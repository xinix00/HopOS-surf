package browse

import (
	"image"
	"strconv"
	"strings"

	"golang.org/x/net/html"
)

// fillAbs herkent de aspect-ratio-vulling: absolute op top:0;left:0 (in
// een wrapper met padding-top-percentage, die wij op 0 klemmen). Dat is
// geen overlay maar gewoon de inhoud — die hoort in de flow, anders
// schuiven foto's over elkaar heen.
func fillAbs(cx cssContext, cp props) bool {
	zero := func(k string) bool {
		v, ok := cssLen(cx, cp[k])
		return cp[k] != "" && ok && v == 0
	}
	return zero("top") && zero("left")
}

// absolute haalt een element uit de flow: sub-layout, geplaatst op zijn
// ankers t.o.v. de containing block (de dichtstbijzijnde gepositioneerde
// voorouder, anders de pagina) en ná de flow geschilderd — badges, labels,
// overlays. Zonder expliciete width is hij shrink-to-fit: zo breed als zijn
// inhoud (een badge is geen balk). bottom-ankers komen hier via flushAbs,
// mét de dan bekende voorouder-onderkant (containerBottom; -1 = onbekend,
// dan valt bottom terug op de static position).
func (l *layouter) absolute(el *html.Node, cp props, st style, containerBottom int) {
	o := absOrigin{w: l.width - 2*l.pad} // zonder voorouder: de pagina
	if n := len(l.origins); n > 0 {
		o = l.origins[n-1]
	}
	l.absoluteAt(el, cp, st, containerBottom, o)
}

// fixedPanel legt een position:fixed-element tegen de viewport — dé
// containing block van fixed: (0,0), paginabreed, viewH hoog. De hoogte
// zelf (100%-calc of het top+bottom-paar) rekent cssMinExtent al tegen
// viewH. Gescrold reist het paneel mee met het document: de eerlijke
// statische lezing van "hangt in het venster".
func (l *layouter) fixedPanel(el *html.Node, cp props, st style) {
	// De containing block van fixed ís de viewport — zonder paginamarge:
	// left:0 + width:100% is écht randje-tot-randje (tweakers' menubalk).
	l.absoluteAt(el, cp, st, l.css.viewH, absOrigin{p: image.Pt(0, 0), w: l.width, h: l.css.viewH})
}

// absoluteAt is absolute() met een expliciete containing block (voor
// fixed: de viewport).
func (l *layouter) absoluteAt(el *html.Node, cp props, st style, containerBottom int, o absOrigin) {
	x := l.lineLeft(st.indent)
	if v, ok := cssLenSignedPct(l.css, cp["left"], o.w); ok {
		x = o.p.X + v
	}
	w := 0
	if v, ok := cssLenPct(l.css, cp["width"], o.w); ok && v > 0 {
		w = v
	}
	wExplicit := w > 0
	if v, ok := cssLenSignedPct(l.css, cp["right"], o.w); ok && cp["left"] == "" && wExplicit {
		x = o.p.X + o.w - v - w // rechts geankerd op de containing block
	}
	if w <= 0 {
		w = l.width - l.pad - x
	}
	if x < 0 {
		x = 0
	}
	if x >= l.width {
		return
	}
	// Een leeg absoluut element mét vlak en maten ís een vlak: de vulling
	// van een progressiebalk (gethop: width-% + inset-ankers in een
	// relative spoor), badges, accentstrepen. Alleen met een expliciete
	// breedte — anders zou elke lege overlay-backdrop paginabreed kleuren.
	if wExplicit && l.emptyContent(el) {
		if bgc, ok := cssColor(cp["background-color"]); ok {
			h := 0
			if v, ok := cssLenPct(l.css, cp["height"], o.h); ok && v > 0 {
				h = v
			} else if t, ok := anchorLen(l.css, cp["top"], o.h); ok && o.h > 0 {
				if b, ok2 := anchorLen(l.css, cp["bottom"], o.h); ok2 && o.h-t-b > 0 {
					h = o.h - t - b
				}
			}
			if h >= 2 && h <= 600 && w >= 4 {
				y := o.p.Y
				if v, ok := anchorLen(l.css, cp["top"], o.h); ok {
					y = o.p.Y + v
				} else if v, ok := anchorLen(l.css, cp["bottom"], o.h); ok && containerBottom >= 0 {
					y = containerBottom - v - h
				}
				box := Box{R: image.Rect(x, y, x+w, y+h), BG: bgc, HasBG: true, Rad: cssRadius(l.css, cp["border-radius"])}
				if z, err := strconv.Atoi(strings.TrimSpace(cp["z-index"])); err == nil {
					box.Z = z
				}
				l.late = append(l.late, box)
			}
		}
		return
	}
	if w < 24 {
		return // niets zinnigs te leggen
	}
	sub := l.subLayout(el, w, st, true)
	uMin, uMax := subExtent(sub, wExplicit)
	if uMax <= uMin {
		return // leeg element
	}
	natW := uMax - uMin
	if v, ok := cssLenSignedPct(l.css, cp["right"], o.w); ok && cp["left"] == "" && !wExplicit {
		x = o.p.X + o.w - v - natW // rechts geankerd op de gemeten breedte
		if x < 0 {
			x = 0
		}
	}
	y := l.y
	if v, ok := anchorLen(l.css, cp["top"], o.h); ok {
		y = o.p.Y + v
	} else if v, ok := anchorLen(l.css, cp["bottom"], o.h); ok && containerBottom >= 0 {
		y = containerBottom - v - sub.y // onderkant tegen de voorouder-onderkant
	}
	if y < 0 {
		y = 0
	}
	n0 := len(l.late)
	l.adopt(sub, image.Pt(x-uMin, y), true)
	// z-index: de sorteersleutel van de late laag — layoutStyled sorteert
	// stabiel, dus zonder declaratie blijft het bronvolgorde.
	if z, err := strconv.Atoi(strings.TrimSpace(cp["z-index"])); err == nil {
		for i := n0; i < len(l.late); i++ {
			l.late[i].Z = z
		}
	}
}

// subLayout legt een element in zijn eigen mini-layouter van breedte w —
// dé bouwsteen voor cellen, floats, inline-blokken, metingen en
// absolutes. De tekststijl erft mee (ook text-align, zoals in echte CSS);
// de flow-context (inspringing, inline, pre) reset, en binnenin gedraagt
// alles zich als blok.
func (l *layouter) subLayout(el *html.Node, w int, st style, abs bool) *layouter {
	// Géén paginamarge in een sub: de celbreedte ís de contentbreedte —
	// synthetische zijmarges per nestingniveau vraten 12px per laag
	// (tweakers' hero: 704 → 599 door zeven lagen), en dat is geen CSS.
	sub := &layouter{
		width: w, css: l.css, imgs: l.imgs, styles: l.styles, edits: l.edits,
		control: l.control, icon: l.icon, rootEl: el,
	}
	if abs {
		sub.absEl = el
	}
	st.indent, st.rIndent = 0, 0
	st.inline, st.pre = false, false
	st.blockify = true
	sub.walk(el, st)
	sub.breakLine()
	sub.flushAbs(-1, sub.y)
	return sub
}

// verhuis neemt boxes over op hun plek in de ouder: veld-indexen herbast,
// Pin vervalt (alleen de hoofdlaag pint), alles schuift met off mee.
func verhuis(dst *[]Box, src []Box, off image.Point, fbase int) {
	for _, b := range src {
		if b.Field > 0 {
			b.Field += fbase
		}
		b.Pin = false
		b.R = b.R.Add(off)
		*dst = append(*dst, b)
	}
}

// adopt haalt een complete sub-layout binnen: boxes naar de hoofdlaag en
// late naar late — of álles naar late (absolutes schilderen bovenop).
func (l *layouter) adopt(sub *layouter, off image.Point, late bool) {
	fbase := len(l.fields)
	if late {
		verhuis(&l.late, sub.boxes, off, fbase)
	} else {
		verhuis(&l.boxes, sub.boxes, off, fbase)
	}
	verhuis(&l.late, sub.late, off, fbase)
	for _, f := range sub.fields {
		f.R = f.R.Add(off)
		l.fields = append(l.fields, f)
	}
}

// subExtent meet de inhoud van een sub-layout en krimpt (zonder expliciete
// width) vlakken die de volle sub-breedte besloegen mee — shrink-to-fit,
// met symmetrische binnenmarge. Terug: de linker- en rechterrand van wat
// er echt staat.
func subExtent(sub *layouter, wExplicit bool) (int, int) {
	cMin, cMax := 1<<30, 0
	for _, s := range [][]Box{sub.boxes, sub.late} {
		for _, b := range s {
			if b.Text == "" && b.Img == nil && b.Field == 0 {
				continue
			}
			if b.R.Min.X < cMin {
				cMin = b.R.Min.X
			}
			if b.R.Max.X > cMax {
				cMax = b.R.Max.X
			}
		}
	}
	uMin, uMax := 1<<30, -(1 << 30)
	for _, boxes := range []*[]Box{&sub.boxes, &sub.late} {
		for i := range *boxes {
			b := &(*boxes)[i]
			if !wExplicit && cMax > 0 && b.Text == "" && b.Img == nil && b.R.Max.X > cMax {
				if inzet := cMin - b.R.Min.X; inzet >= 0 && cMax+inzet < b.R.Max.X {
					b.R.Max.X = cMax + inzet
				}
			}
			if b.R.Min.X < uMin {
				uMin = b.R.Min.X
			}
			if b.R.Max.X > uMax {
				uMax = b.R.Max.X
			}
		}
	}
	return uMin, uMax
}

// floatBlock legt een gefloat element neer zoals floatImage een foto:
// tegen de linker- of rechterkant, de lopende flow stroomt ernaast
// (lineLeft/lineRight) en valt eronder weer breed uit. Met een CSS-width
// op maat, anders shrink-to-fit (een tag is geen balk). Nooit meer dan
// ~60% van de regel: er moet tekst naast passen, anders was het geen
// float waard — dan doet de gewone flow het (false).
func (l *layouter) floatBlock(el *html.Node, cp props, st style, right bool) bool {
	l.breakLine()
	l.flushGap()
	// Meetbreedte: 60% van de regel op dit niveau — er moet iets naast
	// kunnen. Actieve floats tellen hier níet mee: hoe breed een knop wil
	// zijn hangt niet af van waar hij landt (of hij pást komt ná het meten).
	base := l.width - l.pad - st.rIndent - l.pad - st.indent
	maxW := base * 3 / 5
	if maxW < 48 {
		return false // te smal om nog iets naast te zetten
	}
	mar := cssEdgesOf(l.css, cp, "margin", 96)
	w, wExplicit := maxW, false
	if v, ok := cssLenPct(l.css, cp["width"], base); ok && v >= 24 && v <= maxW {
		w, wExplicit = v, true
	}
	sub := l.subLayout(el, w, st, false)
	uMin, uMax := subExtent(sub, wExplicit)
	if uMax <= uMin {
		return false // niets gerenderd: laat de gewone flow het proberen
	}
	natW := uMax - uMin
	// Past hij niet meer naast de lopende floats, dan begint hieronder de
	// volgende rij (zo wrapt een te lange knoppenbalk).
	if natW+mar.l+mar.r+8 > l.lineRight(st.rIndent)-l.lineLeft(st.indent) && (l.fL.w > 0 || l.fR.w > 0) {
		l.clearFloats(-1)
	}
	// lineLeft/lineRight rekenen de al-actieve floats mee: de nieuwe komt
	// er gewoon naast — de float-rij (NRC's header: float:left-knoppen in
	// een float:right-balk).
	x := l.lineLeft(st.indent) + mar.l
	if right {
		x = l.lineRight(st.rIndent) - natW - mar.r
	}
	l.adopt(sub, image.Pt(x-uMin, l.y), false)
	f := flt{w: natW + mar.l + mar.r + 8, bot: l.y + sub.y + lead, depth: l.depth}
	old := l.fL
	if right {
		old = l.fR
	}
	if old.w > 0 {
		// De kant was al bezet: één gezamenlijke claim — breder, tot de
		// laagste onderkant, en hij leeft zolang het buitenste blok.
		f.w += old.w
		if old.bot > f.bot {
			f.bot = old.bot
		}
		if old.depth < f.depth {
			f.depth = old.depth
		}
	}
	if right {
		l.fR = f
	} else {
		l.fL = f
	}
	l.space = false
	return true
}

// flushAbs legt de geparkeerde bottom-verankerde absolutes van containing
// block oi, nu zijn onderkant (bottom) bekend is.
func (l *layouter) flushAbs(oi, bottom int) {
	if len(l.pend) == 0 {
		return
	}
	var rest []pendAbs
	for _, p := range l.pend {
		if p.oi == oi {
			l.absolute(p.el, p.cp, p.st, bottom)
		} else {
			rest = append(rest, p)
		}
	}
	l.pend = rest
}
