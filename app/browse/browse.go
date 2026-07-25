// Package browse is de browser achter cmd/browser: x/net/html levert de
// DOM (de WHATWG-parser), cascadia de selectors, de Session het netwerk —
// dit pakket doet de rest: layout en pixels. Eén flow-layout op het
// Spleen-font — blokken, woordwrap, koppen, links, floats, gepinde
// headers — genoeg om echte pagina's leesbaar te maken en links klikbaar.
// Los van main zodat de host-tests de hele keten (HTML → boxes → pixels →
// hit-test) kunnen draaien; net als calc/ is alleen de main tamago-only.
package browse

import (
	"image"
	"image/color"
	"sort"
	"strings"

	"golang.org/x/net/html"

	"github.com/xinix00/hop-os-surf/stack/pixel"
)

// BarH is de hoogte van de adresbalk boven de pagina; StatusH die van de
// statusbalk eronder ("wat doet hij?" — laden, fouten, klaar).
const (
	BarH    = 24
	StatusH = 18
)

// faceFor kiest het Spleen-font voor een layout-schaal: 1 = lopende tekst
// (6x12), 2 = koppen (8x16), 3+ = h1 (8x16 op 2x). charW/charH zijn de
// celmaten waarop de hele layout rekent (voorheen het 8x8-grid).
func faceFor(scale int) (pixel.Face, int) {
	switch {
	case scale <= 1:
		return pixel.F12, 1
	case scale == 2:
		return pixel.F16, 1
	default:
		return pixel.F16, 2
	}
}

func charW(scale int) int           { f, s := faceFor(scale); return f.W * s }
func charH(scale int) int           { f, s := faceFor(scale); return f.H * s }
func textW(t string, scale int) int { return len(t) * charW(scale) }

func drawTxt(img *image.RGBA, x, y, scale int, col color.RGBA, t string) {
	f, s := faceFor(scale)
	pixel.DrawText(img, x, y, f, s, col, t)
}

func drawTxtCentered(img *image.RGBA, r image.Rectangle, scale int, col color.RGBA, t string) {
	f, s := faceFor(scale)
	pixel.DrawTextCentered(img, r, f, s, col, t)
}

// Papier-look: pagina's zijn gemaakt voor zwart-op-wit; de chrome sluit aan
// bij het instrumentenpaneel van de rest van de desktop.
var (
	colBar      = color.RGBA{0x18, 0x22, 0x36, 0xFF}
	colBarTxt   = color.RGBA{0xF0, 0xF4, 0xFF, 0xFF}
	colPage     = color.RGBA{0xFC, 0xFC, 0xF8, 0xFF}
	colText     = color.RGBA{0x20, 0x20, 0x24, 0xFF}
	colBold     = color.RGBA{0x00, 0x00, 0x00, 0xFF}
	colLink     = color.RGBA{0x1A, 0x4F, 0xC4, 0xFF}
	colCode     = color.RGBA{0x6A, 0x2A, 0x8A, 0xFF}
	colRule     = color.RGBA{0xB0, 0xB0, 0xB8, 0xFF}
	colErrBar   = color.RGBA{0xFF, 0x8A, 0x7A, 0xFF} // fouttekst op de donkere statusbalk
	colScrTrack = color.RGBA{0xE4, 0xE4, 0xE0, 0xFF}
	colScrThumb = color.RGBA{0x8A, 0x96, 0xB0, 0xFF}
	colFieldBG  = color.RGBA{0xFF, 0xFF, 0xFF, 0xFF} // invoerveld
	colBtnFace  = color.RGBA{0xE2, 0xE6, 0xEE, 0xFF} // knop
	colFocus    = color.RGBA{0x2D, 0x6C, 0xDF, 0xFF} // rand van het veld met focus
)

const (
	pad   = 6  // paginamarge
	lead  = 4  // interlinie
	inset = 16 // inspringing per lijst/quote-niveau
)

const defaultViewportHeight = 600

// Viewport is het CSS-contentvenster voor één layout-run. Height is dus de
// paginahoogte tussen de browserbalken, niet de volledige SURF-surface.
// Een expliciet type voorkomt dat meerdere browservensters procesglobale
// vw/vh/rem-state delen.
type Viewport struct {
	Width, Height int
}

func (v Viewport) normalized() Viewport {
	if v.Width < 1 {
		v.Width = mobileWidth
	}
	if v.Height < 1 {
		v.Height = defaultViewportHeight
	}
	return v
}

// Box is één gelayoute tekstrun, afbeelding of <hr>-lijn in document-
// coördinaten: (0,0) is de top van de pagina, los van scroll en adresbalk.
type Box struct {
	R      image.Rectangle
	Text   string
	Scale  int
	Col    color.RGBA
	Href   string      // niet-leeg: klikbaar (nog onopgeloste href uit de pagina)
	Rule   bool        // <hr>: R vullen i.p.v. Text tekenen
	Img    *image.RGBA // <img>: al geschaald naar R — teken i.p.v. Text
	Tile   *image.RGBA // background-image: herhaald over R (tegels — nooit een reuze-alloc)
	Bold   bool        // pseudo-vet (dubbelgetekend)
	Under  bool        // onderstreept (text-decoration; de UA-default voor links)
	Strike bool        // doorgestreept (line-through — de oude prijs)
	Z      int         // z-index: sorteersleutel van de late (absolute) laag
	BG     color.RGBA  // achtergrondvlak achter de run (of het blok)
	HasBG  bool
	Border color.RGBA // blokrand (kaarten, panelen)
	HasBrd bool
	BrdW   int  // randdikte in px (0/1 = de klassieke 1px-lijn)
	Rad    int  // border-radius: hoekstraal in px (-1 = helemaal rond, klemt bij het tekenen)
	Field  int  // >0: invoerveld/knop — index+1 in Page.Fields
	Pin    bool // hoort bij de gepinde header (zie Page.PinY0/PinY1)
}

// ControlID is de stabiele koppeling tussen een gelayout veld en de DOM-
// control die Session bezit. De Page zelf houdt geen DOM-pointers vast.
type ControlID uint32

// Field is één formulierveld of -knop op de pagina. Alles wat paint en
// hit-test nodig hebben staat hier; Session vertaalt ID terug naar de DOM.
type Field struct {
	ID          ControlID
	R           image.Rectangle // klik-doel in documentcoördinaten
	Name        string
	Value       string
	Placeholder string
	Submit      bool
}

// Page is het layout-resultaat voor één breedte; bij een resize opnieuw
// layouten (de WM bepaalt de maat, dus dit is de gewone gang van zaken).
type Page struct {
	Boxes  []Box
	Fields []Field
	Height int        // documenthoogte in pixels (voor scroll-klemmen)
	BG     color.RGBA // paginacanvas (body-achtergrond); HasBG=false → papierwit
	HasBG  bool
	// De gepinde header (position:fixed/sticky aan de bovenrand): de boxes
	// met Pin=true beslaan documentregels [PinY0, PinY1); voorbij PinY0
	// gescrold tekent View ze bovenin, zoals de site het vraagt.
	PinY0, PinY1 int
}

// Pinned: is er een header om vast te houden?
func (p *Page) Pinned() bool { return p.PinY1 > p.PinY0 }

// style is de geërfde tekststijl tijdens de DOM-wandeling. CSS voedt
// dezelfde velden als de tag-defaults — de cascade ís deze struct.
type style struct {
	scale    int
	col      color.RGBA
	href     string
	indent   int
	pre      bool
	bold     bool   // pseudo-vet: glyph dubbel getekend met 1px offset
	under    bool   // onderstreept (text-decoration door de cascade)
	strike   bool   // doorgestreept (text-decoration: line-through — prijzen!)
	rise     int    // verticale offset t.o.v. de regel (sub/sup)
	xform    string // text-transform: uppercase/lowercase/capitalize
	center   bool   // text-align:center / <center>
	right    bool   // text-align:right — prijzen, datums
	marker   string // het lijstteken voor li's: "-", "1" (tellen) of "" (geen)
	list     *int   // de teller van de omvattende <ol>
	inline   bool   // in een flex/inline-context: blokken breken hier niet
	blockify bool   // direct kind van een grid/flex-kolom: word een blok (ook een <a>)
	lead     int    // line-height als interlinie in px (0 = de default van 4)
	rad      int    // border-radius voor vervangen inhoud (ronde avatars) — niet geërfd, per element gezet
	rIndent  int    // inspringing vanaf rechts (marges/padding van blokken)
	bg       color.RGBA
	hasBG    bool
	on       color.RGBA // effectieve achtergrond ónder de tekst (contrastbewaking)
	hasOn    bool
}

// flt is één actieve float: een afbeelding aan de kant waar de tekst langs
// stroomt. w is de opgeëiste breedte (incl. marge), bot de onderkant in
// documentcoördinaten, depth de blokdiepte waarop hij ontstond (voor het
// impliciete clearen als dat blok sluit).
type flt struct {
	w, bot, depth int
}

type layouter struct {
	width    int
	pad      int // zijmarge van dít canvas: de paginamarge op de wortel, 0 in subs
	css      cssContext
	x, y     int // x=0 betekent: nog niets op deze regel
	lineH    int
	boxes    []Box
	fields   []Field
	space    bool // er hoort witruimte vóór het volgende woord
	gap      int  // opgespaarde blokmarge (collapsing): pas toe bij het volgende woord
	imgs     map[string]image.Image
	styles   map[*html.Node]props
	edits    map[*html.Node]string // door de gebruiker ingetikte veldwaarden
	control  func(*html.Node) ControlID
	line0    int  // index van de eerste box op de huidige regel (voor centreren)
	lineLead int  // interlinie van de huidige regel (line-height; 0 = default)
	lineTxt  bool // er staat tekst op deze regel (interlinie is van tekst)
	center   bool // deze regel centreren bij breakLine
	right    bool // deze regel rechts uitlijnen bij breakLine
	lineR    int  // rechterrand van de uit te lijnen regel (0 = paginabreed)
	fL, fR   flt  // actieve floats links en rechts
	depth    int  // blokdiepte tijdens de wandeling

	pageBG    color.RGBA // body-achtergrond: het paginacanvas (Page.BG)
	hasPageBG bool
	pin       pinState

	origins  []absOrigin // gepositioneerde voorouders (containing blocks)
	rootEl   *html.Node  // celwortel van een sub-layout: zijn width is al verrekend
	pend     []pendAbs   // bottom-verankerde absolutes: wachten op de voorouderhoogte
	late     []Box       // absolute boxes: geschilderd ná de flow (erbovenop)
	absEl    *html.Node  // absolute() legt dit element zelf — geen recursie
	icon     image.Image // site-icoon (apple-touch-icon) voor het logo-slot
	iconUsed bool        // één logo-slot per pagina: het eerste (de header)
	svgN     int         // gerasterde inline-svg's deze layout (budget)
}

// absOrigin is één containing block voor absolute nazaten: zijn hoekpunt
// én zijn breedte — procent-ankers (wikipedia's right:60%) resolven tegen
// die breedte, niet tegen de pagina.
type absOrigin struct {
	p image.Point
	w int
	h int // gedeclareerde hoogte (0 = onbekend) — de basis voor top/bottom-%
}

// pendAbs is een uitgestelde absolute: bottom-verankerd — die is pas te
// leggen als de onderkant van zijn containing block bekend is, dus bij het
// sluiten van die voorouder (of van de pagina; oi -1).
type pendAbs struct {
	el *html.Node
	cp props
	st style
	oi int // index in origins van de containing block (-1 = de pagina)
}

// pinState volgt de header die gepind gaat worden: tussen beginPin en
// endPin krijgen nieuwe boxes Pin=true, en de y-range wordt Page.PinY0/1.
type pinState struct {
	active, done bool
	box0, y0, y1 int
}

// beginPin: dit element vraagt fixed/sticky aan de bovenrand — pinnen als
// het ook écht bovenin de pagina ligt (een modal halverwege is geen
// header). Eén header per pagina; de eerste wint.
func (l *layouter) beginPin(cp props) bool {
	if l.pin.active || l.pin.done {
		return false
	}
	if v, ok := cssLen(l.css, cp["top"]); cp["top"] != "" && (!ok || v > 8) {
		return false // niet tegen de bovenrand geplakt
	}
	if cp["position"] == "fixed" {
		// fixed is viewport-verankerd: de opgespaarde blokmarge van de
		// flow hoort er niet bij — top:0 ís de bovenrand. En zolang er
		// nog geen échte inhoud boven staat (alleen bg-vlakken en
		// wrapper-padding) mag de balk ook echt naar zijn anker: hij
		// ontsnapt per spec aan zijn wrapper.
		l.gap = 0
		top := 0
		if v, ok := cssLen(l.css, cp["top"]); ok && v > 0 {
			top = v
		}
		clear := true
		for i := range l.boxes {
			if b := &l.boxes[i]; b.Text != "" || b.Img != nil || b.Rule || b.Field > 0 {
				clear = false
				break
			}
		}
		if clear && l.y > top {
			l.y = top
		}
	}
	l.breakLine()
	l.flushGap()
	if l.y > 300 {
		return false
	}
	l.pin = pinState{active: true, box0: len(l.boxes), y0: l.y}
	return true
}

// endPin sluit de header af; te hoog (een fixed paneel of modal) betekent:
// toch niet pinnen — dan scrollt hij gewoon mee.
func (l *layouter) endPin() {
	l.breakLine()
	h := l.y - l.pin.y0
	if h > 0 && h <= 120 && len(l.boxes) > l.pin.box0 {
		for i := l.pin.box0; i < len(l.boxes); i++ {
			l.boxes[i].Pin = true
		}
		l.pin.y1, l.pin.done = l.y, true
	}
	l.pin.active = false
}

// lineLeft/lineRight: de regelgrenzen op de huidige y, mét de actieve
// floats — tekst stroomt er vanzelf langs en valt eronder weer breeduit.
func (l *layouter) lineLeft(indent int) int {
	x := l.pad + indent
	if l.fL.w > 0 && l.y < l.fL.bot {
		x += l.fL.w
	}
	return x
}

func (l *layouter) lineRight(rIndent int) int {
	r := l.width - l.pad - rIndent
	if l.fR.w > 0 && l.y < l.fR.bot {
		r -= l.fR.w
	}
	return r
}

// Layout wandelt de DOM onder body en vouwt hem tot boxes voor deze
// paginabreedte. Onbekende elementen erven gewoon door — een pagina met
// <article> of <custom-tag> blijft leesbaar.
func Layout(body *html.Node, width int) Page {
	return LayoutViewport(body, Viewport{Width: width})
}

// LayoutWithImages is Layout met de opgehaalde afbeeldingen, gesleuteld op
// het rauwe src-attribuut (Session lost de URL's op en haalt ze binnen —
// layout blijft puur en synchroon). Een <img> zonder plaatje valt terug op
// zijn alt-tekst.
func LayoutWithImages(body *html.Node, width int, imgs map[string]image.Image) Page {
	return layoutStyled(body, Viewport{Width: width}.normalized(), imgs, nil, nil, nil, nil)
}

// LayoutViewport is Layout met een expliciete CSS-viewport. De breedte
// bepaalt wrapping en mediaqueries; de hoogte is de basis voor vh en fixed.
func LayoutViewport(body *html.Node, viewport Viewport) Page {
	return layoutStyled(body, viewport.normalized(), nil, nil, nil, nil, nil)
}

// layoutStyled is de volledige variant: mét de computed CSS-props uit
// Session.stylesFor en de ingetikte veldwaarden. Inline style=""-
// attributen werken altijd, ook zonder die map.
func layoutStyled(body *html.Node, viewport Viewport, imgs map[string]image.Image, styles map[*html.Node]props, edits map[*html.Node]string, icon image.Image, control func(*html.Node) ControlID) Page {
	viewport = viewport.normalized()
	// De rem-basis van deze pagina: html { font-size } (62.5% = 10px).
	cx := newCSSContext(viewport.Width, viewport.Height)
	if body != nil && body.Parent != nil && styles != nil {
		if v, ok := styles[body.Parent]["font-size"]; ok {
			cx.remPx = rootFontPx(v)
		}
	}
	l := &layouter{width: viewport.Width, pad: pad, css: cx, imgs: imgs, styles: styles, edits: edits, icon: icon}
	if control != nil {
		l.control = control
	} else {
		var next ControlID
		l.control = func(*html.Node) ControlID {
			next++
			return next
		}
	}
	if body != nil {
		l.walk(body, style{scale: 1, col: colText, marker: "-"})
	}
	l.breakLine()
	// Bottom-verankerde absolutes zonder gepositioneerde voorouder: hun
	// containing block is de pagina — die onderkant is er nu.
	l.flushAbs(-1, l.y)
	p := Page{Boxes: merge(l.boxes), Fields: l.fields, Height: l.y, BG: l.pageBG, HasBG: l.hasPageBG}
	// Absolute boxes schilderen bovenop de flow (paint-volgorde: laatst),
	// onderling gesorteerd op z-index — stabiel, dus gelijke z blijft
	// bronvolgorde (het schildermechanisme bestond al, dit is de sortering).
	sort.SliceStable(l.late, func(i, j int) bool { return l.late[i].Z < l.late[j].Z })
	for _, b := range l.late {
		p.Boxes = append(p.Boxes, b)
		if b.R.Max.Y > p.Height {
			p.Height = b.R.Max.Y
		}
	}
	if l.pin.done {
		p.PinY0, p.PinY1 = l.pin.y0, l.pin.y1
	}
	return p
}

// hexCSS: een kleur terug naar CSS-hex — de invulling voor currentColor.
func hexCSS(c color.RGBA) string {
	const hx = "0123456789abcdef"
	return string([]byte{'#', hx[c.R>>4], hx[c.R&15], hx[c.G>>4], hx[c.G&15], hx[c.B>>4], hx[c.B&15]})
}

// luma: waargenomen helderheid 0..255 (ITU-R BT.601) — kiest tekstkleuren
// bij de themakleur en bewaakt het contrast op gekleurde vlakken.
func luma(c color.RGBA) int {
	return (299*int(c.R) + 587*int(c.G) + 114*int(c.B)) / 1000
}

func absInt(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// uaProps is onze user-agent-stylesheet, in dezelfde taal als de site-CSS:
// berekende waarden die ónder de author-props gemerged worden. Een h1 is
// dus geen speciaal geval — hij heeft alleen defaults, en de site wint.
var uaProps = map[string]props{
	// Tekstblokken krijgen hun leeslucht hiér, als echte margins — de
	// engine-default is 0, zoals CSS (pure containers als div/a hebben
	// niets; de synthetische blockMargin is 23-07 gesneuveld). Koppen naar
	// rato van hun letterschaal.
	"p":          {"margin-top": "3px", "margin-bottom": "3px"},
	"h1":         {"font-size": "32px", "font-weight": "bold", "color": "#000000", "margin-top": "9px", "margin-bottom": "9px"},
	"h2":         {"font-size": "24px", "font-weight": "bold", "color": "#000000", "margin-top": "6px", "margin-bottom": "6px"},
	"h3":         {"font-size": "20px", "font-weight": "bold", "color": "#000000", "margin-top": "6px", "margin-bottom": "6px"},
	"h4":         {"font-weight": "bold", "color": "#000000", "margin-top": "3px", "margin-bottom": "3px"},
	"h5":         {"font-weight": "bold", "color": "#000000", "margin-top": "3px", "margin-bottom": "3px"},
	"h6":         {"font-weight": "bold", "color": "#000000", "margin-top": "3px", "margin-bottom": "3px"},
	"b":          {"font-weight": "bold"},
	"strong":     {"font-weight": "bold"},
	"th":         {"font-weight": "bold", "text-align": "center"},
	"code":       {"color": "#6a2a8a"},
	"kbd":        {"color": "#6a2a8a", "background-color": "#e2e6ee", "border": "1px solid #b0b0b8"},
	"samp":       {"color": "#6a2a8a"},
	"pre":        {"color": "#6a2a8a", "white-space": "pre", "margin-top": "3px", "margin-bottom": "3px"},
	"mark":       {"background-color": "gold"},
	"center":     {"text-align": "center"},
	"summary":    {"font-weight": "bold"},
	"ul":         {"padding-left": "16px", "margin-top": "3px", "margin-bottom": "3px"},
	"ol":         {"padding-left": "16px", "margin-top": "3px", "margin-bottom": "3px"},
	"dl":         {"margin-top": "3px", "margin-bottom": "3px"},
	"blockquote": {"padding-left": "16px", "border-left": "3px solid #b0b0b8", "margin-top": "3px", "margin-bottom": "3px"},
	"dd":         {"padding-left": "16px"},
	"table":      {"margin-top": "3px", "margin-bottom": "3px"},
	"figure":     {"margin-top": "3px", "margin-bottom": "3px"},
	"fieldset":   {"margin-top": "3px", "margin-bottom": "3px"},
	"address":    {"margin-top": "3px", "margin-bottom": "3px"},
	"button":     {"background-color": "#e2e6ee"},
	"s":          {"text-decoration": "line-through"},
	"del":        {"text-decoration": "line-through"},
	"strike":     {"text-decoration": "line-through"},
	"ins":        {"text-decoration": "underline"},
	"u":          {"text-decoration": "underline"},
	"sub":        {"font-size": "small"},
	"sup":        {"font-size": "small"},
}

// blocks: elementen die een eigen regel (en marge) afdwingen.
var blocks = map[string]bool{
	"address": true, "article": true, "aside": true, "blockquote": true,
	"div": true, "dl": true, "dt": true, "dd": true, "fieldset": true,
	"figure": true, "footer": true, "form": true, "h1": true, "h2": true,
	"h3": true, "h4": true, "h5": true, "h6": true, "header": true,
	"li": true, "main": true, "nav": true, "ol": true, "p": true,
	"pre": true, "section": true, "table": true, "tr": true, "ul": true,
}

// skip: elementen zonder zichtbare inhoud.
var skip = map[string]bool{
	"script": true, "style": true, "head": true, "title": true,
	"meta": true, "link": true, "noscript": true, "template": true,
	"svg": true, "iframe": true, "object": true, "select": true,
}

func (l *layouter) walk(n *html.Node, st style) {
	switch n.Type {
	case html.TextNode:
		txt := n.Data
		if st.pre {
			l.preText(txt, st)
			return
		}
		if len(txt) > 0 && isSpace(txt[0]) {
			l.space = true
		}
		words := strings.Fields(txt)
		for _, w := range words {
			l.word(w, st)
			l.space = true
		}
		if len(words) > 0 && !isSpace(txt[len(txt)-1]) {
			l.space = false
		}
	case html.ElementNode:
		l.element(n, st)
	case html.DocumentNode:
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			l.walk(c, st)
		}
	}
}

func (l *layouter) element(el *html.Node, st style) {
	tag := el.Data
	cp, handled := l.prepareElement(el, st)
	if handled {
		return
	}
	// Elk gepositioneerd element is de containing block voor zijn
	// absolute nazaten.
	originIdx := -1
	if p := cp["position"]; p == "relative" || p == "absolute" || p == "fixed" || p == "sticky" {
		l.origins = append(l.origins, absOrigin{
			p: image.Pt(l.pad+st.indent, l.y),
			w: l.width - 2*l.pad - st.indent - st.rIndent,
			h: cssMinExtent(l.css, cp),
		})
		originIdx = len(l.origins) - 1
		oi := originIdx
		defer func() {
			// Nu is de onderkant van deze containing block bekend: de
			// geparkeerde bottom-verankerde nazaten kunnen gelegd worden.
			l.flushAbs(oi, l.y)
			l.origins = l.origins[:oi]
		}()
	}
	if l.layoutReplaced(el, cp, st) {
		return
	}
	// Bovenin vastgeplakt (fixed/sticky + top): de site zegt zelf "dit is
	// mijn header, hou hem in beeld" — pinnen dus (zie pinState).
	pinning, pinFixed := false, false
	if p := cp["position"]; p == "fixed" || p == "sticky" {
		pinning = l.beginPin(cp)
		if pinning && p == "fixed" {
			// fixed: de viewport is de containing block — de balk ontsnapt
			// aan de rail, de marges van zijn voorouders én de paginamarge
			// (tweakers' menubalk: top:0; left:0; width:100% = écht rand tot
			// rand). Adem voor de tekst komt uit padding (kaart-default of
			// de site-CSS), niet uit een indent-restje.
			st.indent, st.rIndent = -l.pad, -l.pad
			pinFixed = true
		}
	}

	cp, st = l.applyElementStyle(el, cp, st)

	// "Divs goed zetten": display:inline(-block) haalt een element uit de
	// blok-flow, en de kinderen van een flex/grid-container komen náást
	// elkaar in plaats van onder elkaar — precies genoeg voor menu's,
	// zonder een echte layout-engine te worden.
	isBlock := (blocks[tag] || st.blockify) && !st.inline
	switch cp["display"] {
	case "inline", "inline-block", "inline-flex":
		// De wortel van een sub-layout (inline-block-tegel, kolomcel) is
		// dáár al inline geplaatst — binnenin gedraagt hij zich als blok.
		if el != l.rootEl {
			isBlock = false
		}
	case "block", "list-item", "flex", "grid":
		// flex en grid zíjn blok-niveau — ook op een <span> of een custom
		// element (tweakers' <twk-site-menu>): anders krijgt zo'n container
		// nooit zijn achtergrondvlak en behandelen we divs ongelijk.
		isBlock = !st.inline
	}
	// childInline: krijgen de kínderen een inline-context (menu), en
	// childBlockify: worden ze juist blokken (kaarten)? Flex-rij = menu;
	// grid en flex-kolom = blokken onder elkaar — gethops .doors-kaarten
	// stapelen dan net als bij hun eigen mobiele breakpoint, en een
	// <a class=door> wordt daarbij geblokkificeerd zoals in echte CSS.
	childInline := st.inline
	childBlockify := false
	switch cp["display"] {
	case "flex", "inline-flex":
		if fd := cp["flex-direction"]; fd == "column" || fd == "column-reverse" {
			// Een kolom-flex herstélt de blok-context — ook midden in een
			// flex-rij (figure in een teaser): zijn kinderen stapelen, en
			// figcaption komt dus onder de foto, niet ernaast.
			childBlockify, childInline = true, false
		} else {
			childInline = true
		}
	case "grid":
		childBlockify, childInline = true, false
	}
	if tag == "nav" {
		// UA-vooroordeel: een <nav> ís vrijwel altijd een menu — leg hem
		// plat, ook zonder stylesheet (die staat vol properties die wij
		// toch niet dragen).
		childInline = true
	}

	// Een blok dat inline gezet is (flex-kind, display:inline-li) krijgt
	// lucht om zich heen in plaats van een regelbreuk.
	inlined := blocks[tag] && !isBlock

	// Border: uit de CSS (border/border-color); "none"/"0" is uit.
	var brdCol color.RGBA
	brdW, hasBrd := 1, false
	if v, ok := cp["border"]; ok {
		brdCol, brdW, hasBrd = cssBorder(l.css, v)
	}
	if v, ok := cp["border-color"]; ok {
		if c, ok := cssColor(v); ok {
			brdCol, hasBrd = c, true
		}
	}
	// Zijranden (border-left enz.): het accent-patroon van meldingen,
	// citaten en tabs — elk een eigen gekleurde strook langs het blok.
	type sideBrd struct {
		side int // 0=boven 1=rechts 2=onder 3=links
		col  color.RGBA
		w    int
	}
	var sides []sideBrd
	if isBlock {
		for i, name := range []string{"border-top", "border-right", "border-bottom", "border-left"} {
			if v, ok := cp[name]; ok {
				if c, w, on := cssBorder(l.css, v); on {
					sides = append(sides, sideBrd{side: i, col: c, w: w})
				}
			}
		}
	}

	// --- het boxmodel: marge, padding, breedte -----------------------------
	// Containers zijn containers — button of div, het maakt niet uit: de
	// CSS bepaalt de doos, wij rekenen hem uit.
	mar := cssEdgesOf(l.css, cp, "margin", 96)
	pd := cssEdgesOf(l.css, cp, "padding", 48)
	// Engine-default is 0, zoals CSS: alle leeslucht komt uit de cascade —
	// de UA-stylesheet (uaProps) geeft tekstblokken hun marge, de site wint.
	topGap, botGap := 0, 0
	if mar.setV {
		topGap, botGap = mar.t, mar.b
	}
	if pinFixed {
		topGap = 0 // top:0 begint óp de rand, niet onder een blokmarge
	}
	if isBlock {
		st.indent += mar.l
		st.rIndent += mar.r
		// width/max-width: het blok smaller dan zijn ouder; margin:auto
		// centreert (de klassieke artikel-kolom). De wortel van een
		// kolomcel niet: zijn width bepaalde al de célbreedte — nog eens
		// rekenen zou hem tegen de cel resolven (kwart i.p.v. helft).
		availW := l.width - 2*l.pad - st.indent - st.rIndent
		if availW > 64 && el != l.rootEl {
			target := availW
			if v, ok := cssLenPct(l.css, cp["width"], availW); ok && v >= 64 && v < target {
				target = v
			}
			if v, ok := cssLenPct(l.css, cp["max-width"], availW); ok && v >= 64 && v < target {
				target = v
			}
			// min-width tilt een te smal blok weer op (tot wat er past).
			if v, ok := cssLenPct(l.css, cp["min-width"], availW); ok && v > target {
				target = v
				if target > availW {
					target = availW
				}
			}
			if target < availW {
				extra := availW - target
				if mar.autoL && mar.autoR {
					st.indent += extra / 2
					st.rIndent += extra - extra/2
				} else {
					st.rIndent += extra
				}
			}
		}
		// Het grid-centreer-spoor (1fr <vast> 1fr, tweakers' page-grid):
		// de vaste middenbaan is de inhoud, de fr-flanken zijn marge —
		// hetzelfde als margin: 0 auto.
		if cp["display"] == "grid" {
			avail2 := l.width - 2*l.pad - st.indent - st.rIndent
			if railW := gridRailPx(l.css, cp["grid-template-columns"], avail2, cssGap(l.css, cp)); railW > 0 && railW < avail2 {
				extra := avail2 - railW
				st.indent += extra / 2
				st.rIndent += extra - extra/2
			}
		}
		// De containing block ligt wáár het blok ná zijn marges en
		// margin:auto-centrering terechtkwam — dat weten we nu pas
		// (wikipedia's cirkelcontainer: width + margin:0 auto).
		if originIdx >= 0 {
			l.origins[originIdx] = absOrigin{
				p: image.Pt(l.pad+st.indent, l.y),
				w: l.width - 2*l.pad - st.indent - st.rIndent,
				h: cssMinExtent(l.css, cp),
			}
		}
	}

	tile := l.imgs[cssURL(cp["background-image"])]
	decorated := (isBlock || tag == "body") && (st.hasBG || tile != nil || hasBrd || len(sides) > 0)
	// Padding: bij een gedecoreerd blok kleurt hij mee (binnen het vlak);
	// zonder decoratie is het gewoon lucht. Een kaart zonder expliciete
	// padding krijgt de oude kaart-default, en de rand zelf telt ook mee.
	if decorated && tag != "body" && !pd.setV && !pd.setH {
		pd = edges{t: 4, r: 6, b: 4, l: 6, setV: true, setH: true}
	}
	if hasBrd {
		pd.t, pd.r, pd.b, pd.l = pd.t+brdW, pd.r+brdW, pd.b+brdW, pd.l+brdW
	}
	for _, sb := range sides {
		switch sb.side {
		case 0:
			pd.t += sb.w
		case 1:
			pd.r += sb.w
		case 2:
			pd.b += sb.w
		case 3:
			pd.l += sb.w
		}
	}
	if !decorated && isBlock {
		topGap += pd.t
		botGap += pd.b
		st.indent += pd.l
		st.rIndent += pd.r
	}

	// white-space: nowrap — dit element breekt niet middenin: past hij niet
	// meer op de lopende regel, dan begint hij op een verse (labels,
	// knoppen, prijzen). Langer dan een hele regel: dan wrapt hij alsnog.
	if cp["white-space"] == "nowrap" && l.x > 0 {
		txt := strings.Join(strings.Fields(renderableText(el)), " ")
		if tw := textW(ascii(txt), st.scale); tw > 0 &&
			l.x+charW(st.scale)+tw > l.lineRight(st.rIndent) &&
			tw <= l.lineRight(st.rIndent)-l.lineLeft(st.indent) {
			l.breakLine()
		}
	}

	// display:inline-block mét een expliciete breedte is een mini-blok ín
	// de regelflow — het float:left-gevoel: tegels naast elkaar zolang het
	// past. (De sprite-replacement hierboven ving de lege varianten al.)
	if !isBlock && cp["display"] == "inline-block" {
		availIB := l.lineRight(st.rIndent) - l.lineLeft(st.indent)
		if w, ok := cssLenPct(l.css, cp["width"], availIB); ok && w >= 24 && w <= availIB {
			l.inlineBlock(el, w, st)
			return
		}
	}

	// De knop-link: een inline(-block) element mét doos-eigenschappen
	// (padding, marge, vlak of rand). Geen widget en geen uitzondering:
	// de inhoud wordt inline gelegd en daarna vouwt de doos eromheen —
	// tekst rendert nooit los van de div (of a, of span) waar hij in zit.
	// Alleen bij échte decoratie (vlak of rand): kale padding op menu-
	// links zou elke nav in dozen hakken.
	inlineBox := !isBlock && tag != "body" && (st.hasBG || hasBrd)
	ibIdx := -1
	var ibX0, ibY0 int
	if inlineBox {
		l.flushGap()
		if l.space && l.x > 0 {
			l.x += charW(st.scale)
		}
		if l.x == 0 {
			l.x = l.lineLeft(st.indent)
		}
		l.x += mar.l
		ibIdx = len(l.boxes)
		l.boxes = append(l.boxes, Box{BG: st.bg, HasBG: st.hasBG, Border: brdCol, HasBrd: hasBrd, Rad: cssRadius(l.css, cp["border-radius"])})
		ibX0, ibY0 = l.x, l.y
		l.x += pd.l
		l.space = false
		st.hasBG = false // de inhoud ligt al óp de doos
	}

	blockY0 := 0
	if isBlock {
		l.blockGap(topGap)
		l.depth++
		blockY0 = l.y + l.gap // de blok-top, inclusief de nog hangende marge
	}
	// clear: onder de lopende floats beginnen (footer onder de foto).
	if v := cp["clear"]; v == "both" || v == "left" || v == "right" {
		l.clearFloats(-1)
	}

	// Blok-achtergrond en/of -rand: één vlak (of tegelpatroon) achter het
	// hele blok — body-achtergrond wordt zo vanzelf de paginakleur. Het
	// vlak gaat als placeholder de boxlijst in (paint-volgorde: onder de
	// inhoud) en krijgt zijn rechthoek als de blokhoogte bekend is.
	bgIdx := -1
	var bgY0, bgX0, bgX1 int
	var bgCover image.Image // background-size:cover → bij het sluiten beeldvullend schalen
	if decorated {
		l.breakLine()
		l.flushGap()
		bgIdx = len(l.boxes)
		box := Box{BG: st.bg, HasBG: st.hasBG, Border: brdCol, HasBrd: hasBrd, BrdW: brdW, Rad: cssRadius(l.css, cp["border-radius"])}
		if tile != nil {
			w, h := tile.Bounds().Dx(), tile.Bounds().Dy()
			if w > 0 && h > 0 && w <= imgMaxDim && h <= imgMaxDim {
				box.Tile = scaleTo(tile, w, h) // één RGBA-tegel, nooit een reuze-alloc
				if cp["background-size"] == "cover" {
					bgCover = tile
				}
			}
		}
		l.boxes = append(l.boxes, box)
		bgY0 = l.y
		// Exact de blokgrenzen, net als verticaal: lucht is padding en die
		// is al verrekend — een ±2-uitzet liet het vlak als een halo om een
		// width:100%-afbeelding heen piepen (tweakers' teasers, 23-07).
		bgX0 = l.pad + st.indent
		bgX1 = l.width - l.pad - st.rIndent
		// De containing block van absolute nazaten ligt op het vlák — de
		// eerdere vangst zag de blokmarge nog niet en hing de vulling van
		// een progressiebalk 3px boven zijn spoor.
		if originIdx >= 0 {
			l.origins[originIdx].p.Y = l.y
		}
		if tag == "body" {
			bgX0, bgX1 = 0, l.width
			if st.hasBG {
				// De body-kleur is het paginacanvas: ook onder de content en
				// in de marge — een donkere site is dan echt donker.
				l.pageBG, l.hasPageBG = st.bg, true
			}
		} else {
			l.y += pd.t
			st.indent += pd.l
			st.rIndent += pd.r
		}
		st.hasBG = false // de kinderen liggen al óp het vlak: geen run-vulling meer nodig
	}

	// <button> is een container als elke andere — de site-CSS bepaalt de
	// look; alleen het UA-default-knopvlak (als er niets gezet is) en het
	// klikdoel zijn van ons.
	fieldStart := -1
	if tag == "button" {
		l.space = true
		fieldStart = len(l.boxes)
	}

	childSt := st
	childSt.inline = childInline
	childSt.blockify = childBlockify

	l.layoutElementChildren(el, cp, st, childSt, mar, isBlock, inlined)
	if ibIdx >= 0 {
		if len(l.boxes) == ibIdx+1 {
			// Leeg maar mét een eigen maat: dan ís het vlak de inhoud —
			// carrousel-stipjes, statuslampjes, kleurstalen.
			w, wok := cssLen(l.css, cp["width"])
			h, hok := cssLen(l.css, cp["height"])
			if wok && hok && w > 0 && h > 0 {
				doos := image.Rect(ibX0, ibY0, ibX0+w, ibY0+h)
				l.boxes[ibIdx].R = doos
				l.x = doos.Max.X + mar.r
				if h > l.lineH {
					l.lineH = h
				}
				l.space = true
			} else {
				l.boxes = l.boxes[:ibIdx] // lege doos: weg ermee
			}
		} else {
			r := l.boxes[ibIdx+1].R
			for _, b := range l.boxes[ibIdx+2:] {
				r = r.Union(b.R)
			}
			if pd.t > 0 {
				// ruimte voor de padding-boven: de inhoud een stukje omlaag
				for i := ibIdx + 1; i < len(l.boxes); i++ {
					l.boxes[i].R = l.boxes[i].R.Add(image.Pt(0, pd.t))
				}
				r = r.Add(image.Pt(0, pd.t))
			}
			doos := image.Rect(ibX0, ibY0, r.Max.X+pd.r, r.Max.Y+pd.b)
			l.boxes[ibIdx].R = doos
			if l.x < doos.Max.X {
				l.x = doos.Max.X
			}
			l.x += mar.r
			if h := doos.Max.Y - l.y; h > l.lineH {
				l.lineH = h
			}
			l.space = true
		}
	}
	if fieldStart >= 0 && len(l.boxes) > fieldStart {
		r := l.boxes[fieldStart].R
		for _, b := range l.boxes[fieldStart+1:] {
			r = r.Union(b.R)
		}
		name, _ := attr(el, "name")
		value, _ := attr(el, "value")
		if value == "" {
			value = strings.TrimSpace(textContent(el))
		}
		l.fields = append(l.fields, Field{
			ID: l.control(el), R: r.Inset(-2), Name: name, Value: value, Submit: true,
		})
	}
	if isBlock {
		l.depth--
		// Impliciet clearen: floats die ín dit blok ontstonden eindigen
		// hier — echte sites clearfixen hun kaarten toch.
		l.clearFloats(l.depth)
		// height/min-height reserveren óók zonder decoratie ruimte:
		// wikipedia's talencirkel-container (alle kinderen absoluut, de
		// hoogte komt uit de CSS) en kale spacer-divs. Nooit inkrimpen.
		// Gedecoreerde blokken doen dit bij hun vlak (hieronder).
		if bgIdx < 0 {
			if minE := cssMinExtent(l.css, cp); minE > 0 {
				l.breakLine()
				l.flushGap()
				if l.y < blockY0+minE {
					l.y = blockY0 + minE
				}
			}
		}
		l.blockGap(botGap)
	}
	if bgIdx >= 0 {
		l.breakLine()
		if len(l.boxes) == bgIdx+1 && tag != "body" && l.boxes[bgIdx].Tile == nil && cssMinExtent(l.css, cp) == 0 {
			// Er is niets ín het blok beland (een logo-div vol svg): geen
			// vlak achterlaten — een lege gekleurde doos is alleen maar
			// ruis. Mét een gedeclareerde hoogte is het vlak juist de
			// bedoeling: het spoor van een progressiebalk, een kleurstrook.
			l.boxes = l.boxes[:bgIdx]
			l.y = bgY0
		} else {
			if tag == "body" {
				bgX1 = l.width
			} else {
				// min-height (en een expliciete height) rekken het vlak op —
				// hero's en stroken met ademruimte. Nooit inkrimpen (inhoud
				// wint), met een cap tegen 100vh-achtige uitschieters.
				if minE := cssMinExtent(l.css, cp); minE > 0 && l.y+pd.b < bgY0+minE {
					l.y = bgY0 + minE - pd.b
				}
				l.y += pd.b
			}
			// Verticaal exact de blokgrenzen: de binnenmarge zit er al in,
			// en ±2 zou aangrenzende kaarten laten overlappen.
			l.boxes[bgIdx].R = image.Rect(bgX0, bgY0, bgX1, l.y)
			// Zijranden: gekleurde stroken langs de vlakranden (de padding
			// hierboven hield er al ruimte voor vrij).
			for _, sb := range sides {
				s := l.boxes[bgIdx].R
				switch sb.side {
				case 0:
					s.Max.Y = s.Min.Y + sb.w
				case 1:
					s.Min.X = s.Max.X - sb.w
				case 2:
					s.Min.Y = s.Max.Y - sb.w
				case 3:
					s.Max.X = s.Min.X + sb.w
				}
				l.boxes = append(l.boxes, Box{R: s, Col: sb.col, Rule: true})
			}
			// cover: nu de vlakmaat bekend is beeldvullend schalen — één
			// keer per layout, renderen blijft een kale draw (de tegel past
			// dan precies één keer). Reuze-vlakken blijven tegels.
			if w, h := bgX1-bgX0, l.y-bgY0; bgCover != nil && w >= 8 && h >= 8 && w <= 1600 && h <= 800 {
				l.boxes[bgIdx].Tile = scaleCover(bgCover, w, h)
			}
		}
	}
	if pinning {
		l.endPin()
	}
}
