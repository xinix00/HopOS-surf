package browse

import (
	"image"
	"image/draw"
	"sort"
	"strconv"
	"strings"

	"golang.org/x/net/html"
)

// clampBlock legt een blok met een regelbudget: de inhoud in een
// sub-layout, na N regels afgekapt met "..." — de teaser-kap. Past de
// inhoud binnen het budget, dan is er niets te doen (false: de gewone
// flow neemt het over, inclusief marges en decoratie).
func (l *layouter) clampBlock(el *html.Node, cp props, st style, lines int) bool {
	availW := l.lineRight(st.rIndent) - l.lineLeft(st.indent)
	if v, ok := cssLenPct(l.css, cp["width"], availW); ok && v >= 64 && v < availW {
		availW = v
	}
	if availW < 64 {
		return false
	}
	sub := l.subLayout(el, availW, st, false)
	// De regelstarts van de tekst; alles vanaf regel N+1 vervalt.
	var starts []int
	seen := map[int]bool{}
	for _, b := range sub.boxes {
		if b.Text != "" && !seen[b.R.Min.Y] {
			seen[b.R.Min.Y] = true
			starts = append(starts, b.R.Min.Y)
		}
	}
	if len(starts) <= lines {
		return false // past al: geen kap nodig
	}
	sort.Ints(starts)
	cut := starts[lines]
	kept := sub.boxes[:0]
	for _, b := range sub.boxes {
		if b.R.Min.Y >= cut {
			continue
		}
		if b.R.Max.Y > cut {
			b.R.Max.Y = cut // een vlak dat doorliep kapt mee af
		}
		kept = append(kept, b)
	}
	sub.boxes = kept
	// Het beletselteken aan het einde van de laatste zichtbare regel.
	var last *Box
	for i := range sub.boxes {
		if b := &sub.boxes[i]; b.Text != "" && b.R.Min.Y == starts[lines-1] {
			last = b
		}
	}
	if last != nil {
		w := textW("...", last.Scale)
		x0 := last.R.Max.X + charW(last.Scale)
		if x0+w > availW-sub.pad {
			x0 = availW - sub.pad - w
		}
		sub.boxes = append(sub.boxes, Box{
			R:    image.Rect(x0, last.R.Min.Y, x0+w, last.R.Max.Y),
			Text: "...", Scale: last.Scale, Col: last.Col, Href: last.Href,
		})
	}
	sub.y = cut
	l.breakLine()
	// De blokmarge van dit element komt uit de cascade (uaProps of de site).
	mar := cssEdgesOf(l.css, l.propsOf(el), "margin", 96)
	l.blockGap(mar.t)
	l.flushGap()
	l.adopt(sub, image.Pt(l.lineLeft(st.indent)-sub.pad, l.y), false)
	l.y += sub.y
	l.blockGap(mar.b)
	return true
}

// inlineBlock legt een display:inline-block met expliciete breedte als
// mini-blok in de regelflow: een eigen sub-layout, geplaatst als een
// (groot) woord — naast elkaar zolang het past (tegels, taalvakken).
func (l *layouter) inlineBlock(el *html.Node, w int, st style) {
	sub := l.subLayout(el, w, st, false)
	if sub.y < 1 || len(sub.boxes) == 0 {
		return
	}
	l.flushGap()
	sp := 0
	if l.space && l.x > 0 {
		sp = charW(st.scale)
	}
	if l.x > 0 && l.x+sp+w > l.lineRight(st.rIndent) {
		l.breakLine()
		sp = 0
	}
	if l.x == 0 {
		l.x = l.lineLeft(st.indent)
	}
	x := l.x + sp
	l.adopt(sub, image.Pt(x-sub.pad, l.y), false)
	l.x = x + w
	if sub.y > l.lineH {
		l.lineH = sub.y
	}
	l.space = false
	l.alignLine(st)
}

// input legt één <input> in de flow; hidden doet niet mee, knoppen en
// tekstvelden worden widgets, checkbox/radio (v0) een kaal vinkje.
func (l *layouter) input(el *html.Node, st style) {
	typ, _ := attr(el, "type")
	typ = strings.ToLower(strings.TrimSpace(typ))
	val, _ := attr(el, "value")
	if v, ok := l.edits[el]; ok {
		val = v
	}
	switch typ {
	case "hidden":
		return
	case "submit", "button", "reset":
		if val == "" {
			val = "OK"
		}
		l.widget(el, val, true, st)
	case "checkbox", "radio":
		mark := "[ ]"
		if _, ok := attr(el, "checked"); ok {
			mark = "[x]"
		}
		l.word(mark, st) // tonen wel, togglen (nog) niet
		l.space = true
	default: // text, search, email, url, ...
		l.widget(el, val, false, st)
	}
}

// widget plaatst een invoerveld of knop als box in de flow en registreert
// hem als Field (het klik/tik-doel). Veldbreedte volgt het size-attribuut
// (default 20 tekens), knopbreedte het label.
func (l *layouter) widget(el *html.Node, val string, submit bool, st style) {
	l.flushGap()
	cp := l.propsOf(el)
	chars := 20
	if submit {
		chars = len(val) + 2
	} else if v, ok := attr(el, "size"); ok {
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && n > 0 {
			chars = n
		}
	}
	w := chars*charW(st.scale) + 8
	max := l.lineRight(st.rIndent) - l.lineLeft(st.indent)
	// De site-CSS bepaalt de veldbreedte als hij dat wil (wikipedia's
	// zoekbalk: width 100%); anders het size-attribuut.
	if v, ok := cssLenPct(l.css, cp["width"], max); ok && v >= 40 {
		w = v
	}
	if w > max {
		w = max
	}
	// ... en de hoogte: height, of padding om de tekstregel heen.
	h := charH(st.scale) + 8
	if v, ok := cssLen(l.css, cp["height"]); ok && v >= charH(st.scale) {
		h = v
	} else if pd := cssEdgesOf(l.css, cp, "padding", 48); pd.setV {
		h = charH(st.scale) + pd.t + pd.b + 2
	}
	sp := 0
	if l.space && l.x > 0 {
		sp = charW(st.scale)
	}
	if l.x > 0 && l.x+sp+w > l.lineRight(st.rIndent) {
		l.breakLine()
		sp = 0
	}
	if l.x == 0 {
		l.x = l.lineLeft(st.indent)
	}
	x := l.x + sp
	r := image.Rect(x, l.y, x+w, l.y+h)
	name, _ := attr(el, "name")
	placeholder, _ := attr(el, "placeholder")
	l.fields = append(l.fields, Field{
		ID: l.control(el), R: r, Name: name, Value: val, Placeholder: placeholder, Submit: submit,
	})
	// De site-CSS kleedt het veld aan (zoekbalken, merk-knoppen); wat er
	// niet staat, vult renderField met de UA-default (wit veld, knopgrijs).
	box := Box{R: r, Scale: st.scale, Field: len(l.fields), Rad: cssRadius(l.css, cp["border-radius"])}
	if c, ok := cssColor(cp["background-color"]); ok {
		box.BG, box.HasBG = c, true
	}
	if c, bw, on := cssBorder(l.css, cp["border"]); on {
		box.Border, box.BrdW, box.HasBrd = c, bw, true
	}
	if c, ok := cssColor(cp["color"]); ok {
		box.Col = c
	}
	l.boxes = append(l.boxes, box)
	l.x = x + w
	if h > l.lineH {
		l.lineH = h
	}
	l.space = false
	l.alignLine(st)
}

// ascii vouwt tekst naar het 8x8-font (ASCII) via de folds-tabel; wat daar
// niet in staat wordt één '?' — zonder dit werd een em-dash drie '?'-en
// (één per UTF-8-byte).
func ascii(s string) string {
	i := 0
	for ; i < len(s); i++ {
		if s[i] >= 0x80 {
			break
		}
	}
	if i == len(s) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	b.WriteString(s[:i])
	for _, r := range s[i:] {
		if r < 0x80 {
			b.WriteRune(r)
		} else if t, ok := folds[r]; ok {
			b.WriteString(t)
		} else {
			b.WriteByte('?')
		}
	}
	return b.String()
}

// folds: niet-ASCII → ASCII. Typografie naar de schrijfmachine-vorm,
// accentletters naar hun kale vorm (ë → e — Nederlandse pagina's staan er
// vol mee: Oekraïne, één, financiën), ligaturen uit elkaar, valuta naar hun
// ISO-code.
var folds = map[rune]string{
	'­': "", // zacht koppelteken: onzichtbaar tot je hem nodig hebt (nooit, bij ons)
	'–': "-", '—': "-", '−': "-", '‐': "-", '‑': "-",
	'‘': "'", '’': "'", '‚': ",", '“': "\"", '”': "\"", '„': "\"",
	'«': "<<", '»': ">>", '‹': "<", '›': ">",
	' ': " ", ' ': " ", ' ': " ", '​': "",
	'•': "-", '·': "-", '…': "...", '×': "x", '÷': "/",
	'©': "(c)", '®': "(r)", '™': "(tm)", '°': "*", '±': "+/-",
	'→': "->", '←': "<-", '↑': "^", '↓': "v",
	'€': "EUR", '£': "GBP", '¥': "JPY", '¢': "c",
	'à': "a", 'á': "a", 'â': "a", 'ã': "a", 'ä': "a", 'å': "a", 'æ': "ae",
	'ç': "c", 'è': "e", 'é': "e", 'ê': "e", 'ë': "e",
	'ì': "i", 'í': "i", 'î': "i", 'ï': "i", 'ñ': "n", 'ð': "d",
	'ò': "o", 'ó': "o", 'ô': "o", 'õ': "o", 'ö': "o", 'ø': "o", 'œ': "oe",
	'ù': "u", 'ú': "u", 'û': "u", 'ü': "u", 'ý': "y", 'ÿ': "y",
	'ß': "ss", 'þ': "th", 'ĳ': "ij",
	'À': "A", 'Á': "A", 'Â': "A", 'Ã': "A", 'Ä': "A", 'Å': "A", 'Æ': "AE",
	'Ç': "C", 'È': "E", 'É': "E", 'Ê': "E", 'Ë': "E",
	'Ì': "I", 'Í': "I", 'Î': "I", 'Ï': "I", 'Ñ': "N", 'Ð': "D",
	'Ò': "O", 'Ó': "O", 'Ô': "O", 'Õ': "O", 'Ö': "O", 'Ø': "O", 'Œ': "OE",
	'Ù': "U", 'Ú': "U", 'Û': "U", 'Ü': "U", 'Ý': "Y", 'Ÿ': "Y", 'Ĳ': "IJ",
	'š': "s", 'Š': "S", 'ž': "z", 'Ž': "Z", 'č': "c", 'Č': "C",
}

// word plaatst één woord, met wrap op de paginabreedte.
func (l *layouter) word(w string, st style) {
	w = ascii(w)
	switch st.xform {
	case "uppercase":
		w = strings.ToUpper(w)
	case "lowercase":
		w = strings.ToLower(w)
	case "capitalize":
		if len(w) > 0 && w[0] >= 'a' && w[0] <= 'z' {
			w = string(w[0]-32) + w[1:]
		}
	}
	l.lineTxt = true
	l.flushGap()
	// Contrastbewaking: tekst die (bijna) wegvalt tegen zijn achtergrond —
	// meestal een link waarvan wij de kleurregel niet dragen, op een donker
	// menuvlak — klapt naar licht of donker. Liever leesbaar dan kleurecht.
	if st.hasOn && absInt(luma(st.col)-luma(st.on)) < 90 {
		if luma(st.on) < 128 {
			st.col = colBarTxt
		} else {
			st.col = colText
		}
	}
	ww := textW(w, st.scale)
	sp := 0
	if l.space && l.x > 0 {
		sp = charW(st.scale)
	}
	if l.x > 0 && l.x+sp+ww > l.lineRight(st.rIndent) {
		l.breakLine()
		sp = 0
	}
	if l.x == 0 {
		l.x = l.lineLeft(st.indent)
		// Past het woord op een verse regel niet naast de float (een kop
		// op schaal 3 naast een foto), spring er dan onder — anders liep
		// hij het beeld uit.
		for l.x+ww > l.lineRight(st.rIndent) {
			bot := 0
			if l.fL.w > 0 && l.y < l.fL.bot {
				bot = l.fL.bot
			}
			if l.fR.w > 0 && l.y < l.fR.bot && (bot == 0 || l.fR.bot < bot) {
				bot = l.fR.bot
			}
			if bot == 0 {
				break
			}
			l.y = bot
			l.x = l.lineLeft(st.indent)
		}
	}
	x := l.x + sp
	// sub/sup: de verticale offset t.o.v. de regelbasis (nooit boven de
	// paginarand uit).
	y := l.y + st.rise
	if y < 0 {
		y = 0
	}
	l.boxes = append(l.boxes, Box{
		R:      image.Rect(x, y, x+ww, y+charH(st.scale)),
		Text:   w,
		Scale:  st.scale,
		Col:    st.col,
		Href:   st.href,
		Bold:   st.bold,
		Under:  st.under,
		Strike: st.strike,
		BG:     st.bg,
		HasBG:  st.hasBG,
	})
	l.x = x + ww
	if h := charH(st.scale); h > l.lineH {
		l.lineH = h
	}
	// line-height: de ruimste tekst op de regel bepaalt de interlinie.
	eff := lead
	if st.lead > 0 {
		eff = st.lead
	}
	if eff > l.lineLead {
		l.lineLead = eff
	}
	l.space = false
	l.alignLine(st)
}

// image plaatst een afbeelding in de flow, als een (groot) woord: past hij
// nog op de regel dan inline, anders op een nieuwe. Breder dan de pagina →
// proportioneel verkleind; het schalen gebeurt hier (één keer per layout),
// renderen is daarna een kale draw.Draw.
func (l *layouter) image(m image.Image, st style) {
	l.imageSized(m, m.Bounds().Dx(), m.Bounds().Dy(), st, false)
}

// imgSize: de weergavemaat van een <img> — CSS width/height wint van de
// width/height-attributen (kale pixels, zoals HTML ze definieert); één
// gegeven maat schaalt de andere proportioneel mee. Zonder aanwijzing de
// eigen maat. CSS "auto" schakelt het attribuut úit (het klassieke
// img{height:auto}-reset: de verhouding komt uit het beeld) — en een
// hoogte-procent heeft bij ons geen basis (de ouderhoogte is onbekend),
// dus die telt als auto. Bronnen mengen gaf eieren (wikipedia's logo:
// CSS width:57px naast het height=183-attribuut).
func imgSize(cx cssContext, el *html.Node, cp props, iw, ih, avail int) (int, int) {
	side := func(name string, pctBase int) int {
		if v, ok := cp[name]; ok {
			v = strings.TrimSpace(v)
			if v == "auto" || (pctBase <= 0 && strings.Contains(v, "%")) {
				return 0 // afleiden uit de andere maat; het attribuut vervalt
			}
			if n, ok := cssLenPct(cx, v, pctBase); ok && n > 0 {
				return n
			}
		}
		if v, ok := attr(el, name); ok {
			v = strings.TrimSuffix(strings.TrimSpace(v), "px")
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				return n
			}
		}
		return 0
	}
	w, h := side("width", avail), side("height", 0)
	// aspect-ratio maakt de ontbrekende maat af als er maar één gegeven is.
	if v, ok := cp["aspect-ratio"]; ok && (w > 0) != (h > 0) {
		if rn, rd, ok := cssRatio(v); ok {
			if w > 0 {
				h = int(float64(w) * rd / rn)
			} else {
				w = int(float64(h) * rn / rd)
			}
			return w, h
		}
	}
	switch {
	case w > 0 && h > 0:
		return w, h
	case w > 0 && iw > 0:
		return w, ih * w / iw
	case h > 0 && ih > 0:
		return iw * h / ih, h
	}
	return iw, ih
}

// bgReplacement herkent het logo-patroon op dit element: background-image
// met vaste CSS-maat, en geen zichtbare tekst (leeg, of weggeschoven met
// text-indent/sr-only). Geeft de afbeelding en de maat; nil als dit gewoon
// een blok-met-achtergrond is.
func (l *layouter) bgReplacement(el *html.Node, cp props) (image.Image, int, int) {
	src := cssURL(cp["background-image"])
	if src == "" || l.imgs[src] == nil {
		return nil, 0, 0
	}
	w, ok1 := cssLen(l.css, cp["width"])
	h, ok2 := cssLen(l.css, cp["height"])
	if !ok1 || !ok2 || w < 8 || h < 8 || w > l.width || h > 600 {
		return nil, 0, 0
	}
	if cp[srProp] != "1" && strings.TrimSpace(textContent(el)) != "" {
		return nil, 0, 0 // zichtbare tekst op een achtergrond: geen replacement
	}
	m := l.imgs[src]
	// Sprite-sheets: background-position knipt het juiste plaatje eruit
	// (wikipedia's wordmark en zuster-iconen leven in één svg-sheet).
	if crop := spriteCrop(l.css, m, cp, w, h); crop != nil {
		return crop, w, h
	}
	return m, w, h
}

// spriteCrop knipt het background-position-venster (w×h) uit een sprite-
// sheet, met de sheet eerst op background-size geschaald als dat er staat.
// nil = geen (bruikbare) positie: dan is het gewoon een achtergrond.
func spriteCrop(cx cssContext, m image.Image, cp props, w, h int) image.Image {
	px, py, ok := cssPairSigned(cx, cp["background-position"])
	if !ok {
		return nil
	}
	sheet := m
	if bw, bh, ok := cssPairSigned(cx, cp["background-size"]); ok && bw > 0 && bh > 0 {
		sheet = scaleTo(m, bw, bh)
	}
	b := sheet.Bounds()
	if px == 0 && py == 0 && b.Dx() <= w && b.Dy() <= h {
		// positie 0 0 op een passend beeld: gewoon een achtergrond. Op een
		// gróter vel is het wél een crop — het eerste plaatje van de sheet
		// staat nu eenmaal op (0,0) (wikipedia's Commons-logo).
		return nil
	}
	r := image.Rect(b.Min.X-px, b.Min.Y-py, b.Min.X-px+w, b.Min.Y-py+h)
	if !r.In(b) {
		return nil
	}
	out := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(out, out.Bounds(), sheet, r.Min, draw.Src)
	return out
}

// imageSized plaatst een afbeelding op een gegeven maat in de flow (image
// replacement geeft de CSS-maat mee; een <img> zijn natuurlijke maat).
// cover=true snijdt beeldvullend bij (object-fit: cover) in plaats van te
// pletten.
func (l *layouter) imageSized(m image.Image, w, h int, st style, cover bool) {
	l.flushGap()
	if w < 1 || h < 1 {
		return
	}
	maxW := l.lineRight(st.rIndent) - l.lineLeft(st.indent)
	if maxW < 8 {
		maxW = 8
	}
	if w > maxW {
		h = h * maxW / w
		if h < 1 {
			h = 1
		}
		w = maxW
	}
	sp := 0
	if l.space && l.x > 0 {
		sp = charW(st.scale)
	}
	if l.x > 0 && l.x+sp+w > l.lineRight(st.rIndent) {
		l.breakLine()
		sp = 0
	}
	if l.x == 0 {
		l.x = l.lineLeft(st.indent)
	}
	x := l.x + sp
	scaled := scaleTo(m, w, h)
	if cover {
		scaled = scaleCover(m, w, h)
	}
	maskRounded(scaled, st.rad)
	l.boxes = append(l.boxes, Box{
		R:    image.Rect(x, l.y, x+w, l.y+h),
		Href: st.href,
		Img:  scaled,
	})
	l.x = x + w
	if h > l.lineH {
		l.lineH = h
	}
	l.space = false
	l.alignLine(st)
}

// floatImage legt een afbeelding als float neer: tegen de linker- of
// rechterkant, de lopende tekst stroomt ernaast (lineLeft/lineRight) en
// valt eronder weer breed uit. Nooit meer dan ~60% van de regel breed —
// er moet tekst naast passen, anders was het geen float waard.
func (l *layouter) floatImage(m image.Image, w, h int, st style, right bool) {
	l.breakLine()
	l.flushGap()
	if w < 1 || h < 1 {
		return
	}
	maxW := (l.width - 2*l.pad - st.indent) / 2
	if maxW < 8 {
		maxW = 8
	}
	if w > maxW {
		h = h * maxW / w
		if h < 1 {
			h = 1
		}
		w = maxW
	}
	x := l.pad + st.indent
	if right {
		x = l.width - l.pad - w
	}
	scaled := scaleTo(m, w, h)
	maskRounded(scaled, st.rad)
	l.boxes = append(l.boxes, Box{R: image.Rect(x, l.y, x+w, l.y+h), Href: st.href, Img: scaled})
	f := flt{w: w + 8, bot: l.y + h + lead, depth: l.depth}
	if right {
		l.fR = f
	} else {
		l.fL = f
	}
	l.space = false
}

// clearFloats sluit de floats die dieper dan blokdiepte d ontstonden: de
// y springt onder de foto. Dít maakt het kaart/teaser-patroon af — de
// volgende teaser hoort ónder de vorige, niet in diens restruimte.
func (l *layouter) clearFloats(d int) {
	bot := 0
	if l.fL.w > 0 && l.fL.depth > d {
		if l.fL.bot > bot {
			bot = l.fL.bot
		}
		l.fL = flt{}
	}
	if l.fR.w > 0 && l.fR.depth > d {
		if l.fR.bot > bot {
			bot = l.fR.bot
		}
		l.fR = flt{}
	}
	if bot > 0 {
		l.breakLine()
		if l.y < bot {
			l.y = bot
		}
	}
}
