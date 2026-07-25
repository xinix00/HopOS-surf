package browse

import (
	"image"
	"image/color"
	"image/draw"

	"github.com/xinix00/hop-os-surf/app/ui"
	"github.com/xinix00/hop-os-surf/stack/pixel"
)

// --- view: chrome + scroll + hit-test --------------------------------------

// View is de zichtbare toestand van het browserwindow: adresbalk, de
// gelayoute pagina, de scrollpositie en de statusregel. main houdt er één
// bij en rendert hem na elk event.
type View struct {
	Addr   string // adresbalk-inhoud (bewerkt door toetsen)
	Status string // statusbalk onderin: "go …", een fout, "" = niets
	Err    bool   // Status is een fout — kleur hem als zodanig
	Page   Page
	Scroll int
	Focus  int // >0: Page.Fields[Focus-1] heeft de toetsen; 0 = de adresbalk

	// Frame-identiteit voor RenderScrolled: alleen als ditzelfde buffer al
	// vol getekend is met dezelfde pagina en maat is verschuiven veilig.
	fbPix    *uint8
	fbRect   image.Rectangle
	fbScroll int
	fbBoxes  *Box
}

// Focused geeft het veld met focus, of nil.
func (v *View) Focused() *Field {
	if v.Focus > 0 && v.Focus <= len(v.Page.Fields) {
		return &v.Page.Fields[v.Focus-1]
	}
	return nil
}

// Render tekent adresbalk + pagina + statusbalk over het hele beeld. De
// balken gaan als laatste over de content heen — dat ís de clipping: op de
// statusbalk rendert nooit pagina-inhoud.
func (v *View) Render(img *image.RGBA) {
	b := img.Bounds()
	canvas := colPage
	if v.Page.HasBG {
		canvas = v.Page.BG // donkere site: ook het canvas donker, niet alleen het body-vlak
	}
	pixel.Fill(img, b, canvas)
	y0 := b.Min.Y + BarH
	pinned := v.pinnedNow()
	for i := range v.Page.Boxes {
		bx := &v.Page.Boxes[i]
		if pinned && bx.Pin {
			continue // komt zo bovenop, op zijn gepinde plek
		}
		v.drawBox(img, bx, y0+bx.R.Min.Y-v.Scroll, y0+bx.R.Max.Y-v.Scroll, y0)
	}
	if pinned {
		// De gepinde header: bovenin, over de gescrolde content heen. Eerst
		// een dekkende strook in de canvaskleur — een header zonder eigen
		// achtergrond zou anders transparant over de tekst zweven.
		strip := image.Rect(b.Min.X, y0, b.Max.X, y0+v.Page.PinY1-v.Page.PinY0)
		pixel.Fill(img, strip, canvas)
		for i := range v.Page.Boxes {
			bx := &v.Page.Boxes[i]
			if !bx.Pin {
				continue
			}
			v.drawBox(img, bx, y0+bx.R.Min.Y-v.Page.PinY0, y0+bx.R.Max.Y-v.Page.PinY0, y0)
		}
	}
	v.RenderBar(img)
	v.RenderStatus(img)
	v.renderScrollbar(img)
	v.noteFrame(img)
}

// noteFrame onthoudt wat er in dit buffer getekend staat — de basis
// waarop RenderScrolled mag verschuiven.
func (v *View) noteFrame(img *image.RGBA) {
	if len(img.Pix) == 0 {
		return
	}
	v.fbPix, v.fbRect, v.fbScroll = &img.Pix[0], img.Bounds(), v.Scroll
	v.fbBoxes = nil
	if len(v.Page.Boxes) > 0 {
		v.fbBoxes = &v.Page.Boxes[0]
	}
}

// RenderScrolled is Render voor het geval dat er alléén gescrold is: het
// overlevende deel van het frame schuift in het buffer zelf (memmove) en
// alleen de blootgelegde strook wordt echt getekend — scrollen kost zo
// een strook in plaats van een vol frame, zonder één pixel extra opslag.
// De layout was al gecached; dit hergebruikt ook de pixels. Alles wat
// niet puur schuiven is (ander buffer, andere pagina, de header die
// (los)gepind raakt) valt terug op een volle Render.
func (v *View) RenderScrolled(img *image.RGBA) {
	b := img.Bounds()
	sameBoxes := v.fbBoxes == nil && len(v.Page.Boxes) == 0 ||
		len(v.Page.Boxes) > 0 && v.fbBoxes == &v.Page.Boxes[0]
	if len(img.Pix) == 0 || v.fbPix != &img.Pix[0] || v.fbRect != b || !sameBoxes {
		v.Render(img)
		return
	}
	d := v.Scroll - v.fbScroll
	if d == 0 {
		return
	}
	pinned := v.pinnedNow()
	wasPinned := v.Page.Pinned() && v.fbScroll > v.Page.PinY0
	top := b.Min.Y + BarH
	if pinned {
		// De pinstrook is chroom: schuift niet mee — plus één rij, want
		// het bg-vlak van de header vult 1px lucht buiten zijn rand.
		top += v.Page.PinY1 - v.Page.PinY0 + 1
	}
	bot := b.Max.Y - StatusH
	if pinned != wasPinned || d >= bot-top || -d >= bot-top || bot-top < 8 {
		v.Render(img)
		return
	}
	row := func(y int) []uint8 {
		o := img.PixOffset(b.Min.X, y)
		return img.Pix[o : o+4*b.Dx()]
	}
	var strip image.Rectangle
	if d > 0 {
		// omlaag gescrold: het beeld schuift omhoog, onderin komt bloot
		for y := top; y < bot-d; y++ {
			copy(row(y), row(y+d))
		}
		strip = image.Rect(b.Min.X, bot-d, b.Max.X, bot)
	} else {
		for y := bot - 1; y >= top-d; y-- {
			copy(row(y), row(y+d))
		}
		strip = image.Rect(b.Min.X, top, b.Max.X, top-d)
	}
	// De blootgelegde strook: canvas + elke box die hem raakt, geclipt op
	// de strook zelf (SubImage: tekenen buiten de bounds is een no-op) —
	// zonder clip zou een deels zichtbare kaart zijn vlak over de al
	// geschoven (en correcte) pixels erboven heen vegen.
	canvas := colPage
	if v.Page.HasBG {
		canvas = v.Page.BG
	}
	sub, ok := img.SubImage(strip).(*image.RGBA)
	if !ok {
		v.Render(img)
		return
	}
	pixel.Fill(sub, strip, canvas)
	y0 := b.Min.Y + BarH // dezelfde basis als Render
	for i := range v.Page.Boxes {
		bx := &v.Page.Boxes[i]
		if pinned && bx.Pin {
			continue
		}
		v.drawBox(sub, bx, y0+bx.R.Min.Y-v.Scroll, y0+bx.R.Max.Y-v.Scroll, strip.Min.Y)
	}
	v.renderScrollbar(img)
	v.noteFrame(img)
}

// pinnedNow: is er een header én zijn we er voorbij gescrold? Daarvóór
// staat hij gewoon in de flow op precies dezelfde plek.
func (v *View) pinnedNow() bool {
	return v.Page.Pinned() && v.Scroll > v.Page.PinY0
}

// eachCorner loopt de vier kwart-hoekvlakken van r af (straal rad) en
// roept f aan met de pixel én zijn afstand-index (i,j) vanaf de rechte
// rand — de gedeelde lus onder vullen, omranden en maskeren.
func eachCorner(r image.Rectangle, rad int, f func(x, y, i, j int)) {
	for j := 0; j < rad; j++ {
		for i := 0; i < rad; i++ {
			f(r.Min.X+rad-1-i, r.Min.Y+rad-1-j, i, j)
			f(r.Max.X-rad+i, r.Min.Y+rad-1-j, i, j)
			f(r.Min.X+rad-1-i, r.Max.Y-rad+j, i, j)
			f(r.Max.X-rad+i, r.Max.Y-rad+j, i, j)
		}
	}
}

// clampRad klemt een hoekstraal op de halve korte zijde van r; -1 (een
// procent of pil-waarde) betekent: precies de halve korte zijde.
func clampRad(r image.Rectangle, rad int) int {
	m := r.Dx()
	if r.Dy() < m {
		m = r.Dy()
	}
	if rad < 0 || rad > m/2 {
		rad = m / 2
	}
	return rad
}

// inCorner: ligt hoekpixel (i,j) — geteld vanaf de rechte rand naar de
// hoek toe — binnen de kwartcirkel met straal rad? (midden-van-pixel-test)
func inCorner(i, j, rad int) bool {
	dx, dy := 2*i+1, 2*j+1
	return dx*dx+dy*dy <= 4*rad*rad
}

// fillRounded vult r met kleur c en afgeronde hoeken (border-radius):
// drie rechte banen plus vier kwartcirkels van losse pixels.
func fillRounded(img *image.RGBA, r image.Rectangle, c color.RGBA, rad int) {
	rad = clampRad(r, rad)
	if rad <= 0 {
		pixel.Fill(img, r, c)
		return
	}
	pixel.Fill(img, image.Rect(r.Min.X+rad, r.Min.Y, r.Max.X-rad, r.Max.Y), c)
	pixel.Fill(img, image.Rect(r.Min.X, r.Min.Y+rad, r.Min.X+rad, r.Max.Y-rad), c)
	pixel.Fill(img, image.Rect(r.Max.X-rad, r.Min.Y+rad, r.Max.X, r.Max.Y-rad), c)
	eachCorner(r, rad, func(x, y, i, j int) {
		if inCorner(i, j, rad) {
			img.SetRGBA(x, y, c)
		}
	})
}

// outlineRounded tekent één randlijn met afgeronde hoeken: rechte stukken
// tussen de hoeken, en op de hoeken de cirkelring (binnen rad, buiten
// rad-1).
func outlineRounded(img *image.RGBA, r image.Rectangle, c color.RGBA, rad int) {
	rad = clampRad(r, rad)
	if rad <= 0 {
		pixel.Outline(img, r, c)
		return
	}
	pixel.Fill(img, image.Rect(r.Min.X+rad, r.Min.Y, r.Max.X-rad, r.Min.Y+1), c)
	pixel.Fill(img, image.Rect(r.Min.X+rad, r.Max.Y-1, r.Max.X-rad, r.Max.Y), c)
	pixel.Fill(img, image.Rect(r.Min.X, r.Min.Y+rad, r.Min.X+1, r.Max.Y-rad), c)
	pixel.Fill(img, image.Rect(r.Max.X-1, r.Min.Y+rad, r.Max.X, r.Max.Y-rad), c)
	eachCorner(r, rad, func(x, y, i, j int) {
		if inCorner(i, j, rad) && !inCorner(i, j, rad-1) {
			img.SetRGBA(x, y, c)
		}
	})
}

// maskRounded maakt de hoeken van een (al geschaalde) afbeelding
// doorzichtig — border-radius op een <img>: de ronde avatar. rad 0 = niets.
func maskRounded(m *image.RGBA, rad int) {
	if rad == 0 || m == nil {
		return
	}
	r := m.Bounds()
	rad = clampRad(r, rad)
	eachCorner(r, rad, func(x, y, i, j int) {
		if !inCorner(i, j, rad) {
			m.SetRGBA(x, y, color.RGBA{})
		}
	})
}

// drawBox tekent één box op de al berekende schermpositie (top/bot) — de
// hoofdlus geeft scroll-coördinaten, de pin-pas de vastgezette.
// drawBox tekent één box op zijn schermpositie [top, bot). clipTop is de
// bovengrens van het te tekenen gebied (de content-start onder de
// adresbalk, of de blootgelegde strook bij RenderScrolled) — puur een
// teken-besparing: de echte clipping zijn de bounds van img zelf.
func (v *View) drawBox(img *image.RGBA, bx *Box, top, bot, clipTop int) {
	b := img.Bounds()
	// ±1 speling: een tekstrun met achtergrond vult 1px rondom, en de
	// onderstreping ligt óp bot — een box die precies op de rand eindigt
	// tekent dus nog nét in het gebied.
	if bot+1 <= clipTop || top-1 >= b.Max.Y {
		return
	}
	if bx.Rule {
		pixel.Fill(img, image.Rect(b.Min.X+bx.R.Min.X, top, b.Min.X+bx.R.Max.X, bot), bx.Col)
		return
	}
	if bx.Img != nil {
		// Over, niet Src: PNG-transparantie hoort het paginawit te tonen.
		r := image.Rect(b.Min.X+bx.R.Min.X, top, b.Min.X+bx.R.Max.X, bot)
		draw.Draw(img, r, bx.Img, bx.Img.Bounds().Min, draw.Over)
		return
	}
	if bx.Field > 0 {
		v.renderField(img, bx, top, bot)
		return
	}
	if bx.Tile != nil || bx.HasBrd {
		// Blok-achtergrond: kleur, dan tegelpatroon, dan de rand.
		r := image.Rect(b.Min.X+bx.R.Min.X, top, b.Min.X+bx.R.Max.X, bot)
		if bx.HasBG {
			fillRounded(img, r, bx.BG, bx.Rad)
		}
		if bx.Tile != nil {
			tw, th := bx.Tile.Bounds().Dx(), bx.Tile.Bounds().Dy()
			for ty := r.Min.Y; ty < r.Max.Y; ty += th {
				for tx := r.Min.X; tx < r.Max.X; tx += tw {
					dst := image.Rect(tx, ty, tx+tw, ty+th).Intersect(r)
					draw.Draw(img, dst, bx.Tile, bx.Tile.Bounds().Min, draw.Over)
				}
			}
		}
		if bx.HasBrd {
			// Niet clippen naar het beeld: SetRGBA buiten beeld is al
			// een no-op, en clippen zou valse randen op de snijlijn
			// tekenen bij een half-zichtbaar blok. Dikte = geneste lijnen.
			for i := 0; i < bx.BrdW || i == 0; i++ {
				if bx.Rad != 0 {
					rad := clampRad(r, bx.Rad)
					outlineRounded(img, r.Inset(i), bx.Border, rad-i)
				} else {
					pixel.Outline(img, r.Inset(i), bx.Border)
				}
			}
		}
		return
	}
	if bx.HasBG {
		// 1px lucht rondom: leest prettiger en dekt de spatie in een
		// samengevoegde run.
		fillRounded(img, image.Rect(b.Min.X+bx.R.Min.X-1, top-1, b.Min.X+bx.R.Max.X+1, bot+1), bx.BG, bx.Rad)
	}
	drawTxt(img, b.Min.X+bx.R.Min.X, top, bx.Scale, bx.Col, bx.Text)
	if bx.Bold {
		// Pseudo-vet: het font heeft geen gewichten — dubbel tekenen met
		// 1px offset is er verrassend dichtbij.
		drawTxt(img, b.Min.X+bx.R.Min.X+1, top, bx.Scale, bx.Col, bx.Text)
	}
	if bx.Under {
		// text-decoration door de cascade: de UA-default voor links, door
		// de site aan of uit te zetten — geen hardgekoppelde href-streep.
		pixel.Fill(img, image.Rect(b.Min.X+bx.R.Min.X, bot, b.Min.X+bx.R.Max.X, bot+1), bx.Col)
	}
	if bx.Strike {
		// line-through: hetzelfde streepje, maar middendoor — de oude prijs.
		mid := (top + bot) / 2
		pixel.Fill(img, image.Rect(b.Min.X+bx.R.Min.X, mid, b.Min.X+bx.R.Max.X, mid+1), bx.Col)
	}
}

// renderScrollbar tekent een smalle positie-indicator aan de rechterrand —
// alleen als de pagina langer is dan de viewport. Geen klik-doel (v0),
// puur "waar ben ik": scrollen gaat met wiel of toetsen.
func (v *View) renderScrollbar(img *image.RGBA) {
	b := img.Bounds()
	viewH := b.Dy() - BarH - StatusH
	if v.Page.Height <= viewH || viewH < 16 {
		return
	}
	top, bot := b.Min.Y+BarH, b.Max.Y-StatusH
	thumbH := viewH * viewH / v.Page.Height
	if thumbH < 8 {
		thumbH = 8
	}
	y := top + (viewH-thumbH)*v.Scroll/(v.Page.Height-viewH)
	pixel.Fill(img, image.Rect(b.Max.X-4, top, b.Max.X, bot), colScrTrack)
	pixel.Fill(img, image.Rect(b.Max.X-4, y, b.Max.X, y+thumbH), colScrThumb)
}

// renderField tekent één invoerveld of knop (bx.Field is 1-based).
func (v *View) renderField(img *image.RGBA, bx *Box, top, bot int) {
	f := &v.Page.Fields[bx.Field-1]
	b := img.Bounds()
	r := image.Rect(b.Min.X+bx.R.Min.X, top, b.Min.X+bx.R.Max.X, bot)
	// De site-stijl uit de box; de UA-default vult de gaten.
	face, edge, ink := colFieldBG, colRule, colText
	if f.Submit {
		face = colBtnFace
	}
	if bx.HasBG {
		face = bx.BG
	}
	if bx.HasBrd {
		edge = bx.Border
	}
	if bx.Col != (color.RGBA{}) {
		ink = bx.Col
	}
	if v.Focus == bx.Field {
		edge = colFocus
	}
	fillRounded(img, r, face, bx.Rad)
	outlineRounded(img, r, edge, clampRad(r, bx.Rad))
	txt := ascii(f.Value)
	if txt == "" && !f.Submit && v.Focus != bx.Field {
		// De placeholder is de belofte van het veld — grijs, tot er
		// getikt wordt.
		if f.Placeholder != "" {
			txt = ascii(f.Placeholder)
			ink = colRule
		}
	}
	if v.Focus == bx.Field && !f.Submit {
		txt += "_"
	}
	if max := (r.Dx() - 8) / charW(bx.Scale); len(txt) > max && max > 0 {
		txt = txt[len(txt)-max:] // het einde in beeld: daar wordt getikt
	}
	if f.Submit {
		drawTxtCentered(img, r, bx.Scale, ink, txt)
	} else {
		drawTxt(img, r.Min.X+4, r.Min.Y+(r.Dy()-charH(bx.Scale))/2, bx.Scale, ink, txt)
	}
}

// BackW is de terug-knop links in de adresbalk (hit-vlak én chip).
const BackW = 26

// RenderBar tekent alléén de adresbalk (voor het tik-pad: een strook van
// een paar KB damage per toets in plaats van een vol frame): de terug-knop
// links, het adres ernaast.
func (v *View) RenderBar(img *image.RGBA) {
	b := img.Bounds()
	bar := image.Rect(b.Min.X, b.Min.Y, b.Max.X, b.Min.Y+BarH)
	pixel.Fill(img, bar, colBar)

	chip := image.Rect(b.Min.X+2, b.Min.Y+2, b.Min.X+BackW-2, b.Min.Y+BarH-2)
	pixel.Card(img, chip, pixel.ColRaise, pixel.ColLine)
	pixel.DrawTextCentered(img, chip, pixel.F12, 1, pixel.ColText, "<")

	txt := v.Addr + "_"
	// Houd het einde in beeld: daar wordt getypt.
	if max := (b.Dx() - BackW - 2*pad) / charW(1); len(txt) > max && max > 0 {
		txt = txt[len(txt)-max:]
	}
	drawTxt(img, b.Min.X+BackW+pad, b.Min.Y+(BarH-charH(1))/2, 1, colBarTxt, txt)
}

// HitBack: valt (x,y) — window-lokaal — op de terug-knop in de adresbalk?
func (v *View) HitBack(x, y int) bool {
	return y >= 0 && y < BarH && x >= 0 && x < BackW
}

// RenderStatus tekent alléén de statusbalk onderin (voor het laad-pad:
// partiële damage — de pagina eronder blijft staan).
func (v *View) RenderStatus(img *image.RGBA) {
	r := v.StatusRect(img)
	pixel.Fill(img, r, colBar)
	txt := v.Status
	if max := (r.Dx() - 2*pad) / charW(1); len(txt) > max && max > 0 {
		txt = txt[:max] // begin in beeld houden: daar staat wát hij doet
	}
	col := colBarTxt
	if v.Err {
		col = colErrBar
	}
	drawTxt(img, r.Min.X+pad, r.Min.Y+(StatusH-charH(1))/2, 1, col, txt)
}

// Bar is de adresbalk-rechthoek in beeldcoördinaten (voor partiële Present).
func (v *View) Bar(img *image.RGBA) image.Rectangle {
	b := img.Bounds()
	return image.Rect(b.Min.X, b.Min.Y, b.Max.X, b.Min.Y+BarH)
}

// StatusRect is de statusbalk-rechthoek in beeldcoördinaten.
func (v *View) StatusRect(img *image.RGBA) image.Rectangle {
	b := img.Bounds()
	return image.Rect(b.Min.X, b.Max.Y-StatusH, b.Max.X, b.Max.Y)
}

// docPoint vertaalt een klik (window-lokaal) naar documentcoördinaten,
// rekening houdend met de gepinde header: een klik op de vaste strook
// hoort bij de header, wáár je ook gescrold bent. inPin zegt of de klik
// op die strook viel (dan telt alleen de header mee als doel).
func (v *View) docPoint(x, y int) (p image.Point, inPin bool) {
	if v.pinnedNow() && y-BarH < v.Page.PinY1-v.Page.PinY0 {
		return image.Pt(x, y-BarH+v.Page.PinY0), true
	}
	return image.Pt(x, y-BarH+v.Scroll), false
}

// Hit vertaalt een klik (window-lokale coördinaten, viewH = windowhoogte)
// naar de href van de link eronder; "" als daar geen link is. Kliks op de
// adres- en statusbalk zijn nooit een link.
func (v *View) Hit(x, y, viewH int) string {
	if y < BarH || y >= viewH-StatusH {
		return ""
	}
	p, inPin := v.docPoint(x, y)
	for _, bx := range v.Page.Boxes {
		if inPin && !bx.Pin {
			continue // de strook dekt de content eronder af
		}
		if bx.Href != "" && p.In(bx.R) {
			return bx.Href
		}
	}
	return ""
}

// HitField geeft het veld (1-based, voor View.Focus) onder een klik; 0 als
// daar geen veld is.
func (v *View) HitField(x, y, viewH int) int {
	if y < BarH || y >= viewH-StatusH {
		return 0
	}
	p, inPin := v.docPoint(x, y)
	for i := range v.Page.Fields {
		r := v.Page.Fields[i].R
		if inPin && (r.Min.Y < v.Page.PinY0 || r.Min.Y >= v.Page.PinY1) {
			continue // veld buiten de header telt niet op de strook
		}
		if p.In(r) {
			return i + 1
		}
	}
	return 0
}

// ScrollBy verschuift en klemt de scrollpositie voor deze viewporthoogte;
// geeft terug of er iets veranderde (zo niet: niet hertekenen).
func (v *View) ScrollBy(delta, viewH int) bool {
	max := v.Page.Height - (viewH - BarH - StatusH)
	if max < 0 {
		max = 0
	}
	s := v.Scroll + delta
	if s < 0 {
		s = 0
	}
	if s > max {
		s = max
	}
	if s == v.Scroll {
		return false
	}
	v.Scroll = s
	return true
}

// --- toetsen ----------------------------------------------------------------

// Rune vertaalt een web-KVM-keyCode naar een teken voor de adresbalk.
// Woont sinds de basis-toolset in ui (elke typende app dezelfde vertaling);
// deze naam blijft omdat hij bij de adresbalk hoort.
func Rune(code uint32, shift bool) byte { return ui.Rune(code, shift) }
