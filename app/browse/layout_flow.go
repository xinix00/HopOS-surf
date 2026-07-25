package browse

import (
	"image"
	"strings"
)

// preText behoudt regels en spaties; te lange regels lopen het beeld uit
// (geen wrap — zo doet een terminal het ook).
func (l *layouter) preText(txt string, st style) {
	for i, line := range strings.Split(strings.ReplaceAll(ascii(txt), "\t", "    "), "\n") {
		if i > 0 {
			l.breakLine()
		}
		line = strings.TrimRight(line, " \r")
		if line == "" {
			continue
		}
		l.flushGap()
		if l.x == 0 {
			l.x = l.pad + st.indent
		}
		ww := textW(line, st.scale)
		l.boxes = append(l.boxes, Box{
			R:      image.Rect(l.x, l.y, l.x+ww, l.y+charH(st.scale)),
			Text:   line,
			Scale:  st.scale,
			Col:    st.col,
			Href:   st.href,
			Under:  st.under,
			Strike: st.strike,
		})
		l.x += ww
		if h := charH(st.scale); h > l.lineH {
			l.lineH = h
		}
	}
}

// breakLine sluit de huidige regel af (no-op op een lege regel) en
// centreert hem als er gecentreerde content op stond — centreren kán pas
// hier, als de regelbreedte bekend is.
func (l *layouter) breakLine() {
	if l.x == 0 {
		return
	}
	if l.center || l.right {
		// Uitlijnen binnen de éigen rechterrand: in een gecentreerde
		// smalle container (wikipedia's wordmark-blok) is dat niet de
		// paginarand — anders centreer je dubbel en schuift alles rechts.
		edge := l.lineR
		if edge <= 0 {
			edge = l.width - l.pad
		}
		shift := edge - l.x
		if l.center {
			shift /= 2
		}
		if shift > 0 {
			for i := l.line0; i < len(l.boxes); i++ {
				// Alles wat bij déze regel hoort schuift mee — ook inhoud
				// die een doos-padding omlaag zette (chips!). Boxes van
				// eerdere regels (<hr> e.d.) blijven staan.
				if l.boxes[i].R.Min.Y >= l.y {
					l.boxes[i].R = l.boxes[i].R.Add(image.Pt(shift, 0))
				}
			}
		}
	}
	// line-height: de regel-eigen interlinie (0 = de vaste default).
	// Interlinie is van tekst — een kale beeldregel (de teaser-foto, een
	// logo) sluit strak af op zijn blok, zoals display:block dat vraagt.
	ll := l.lineLead
	if ll == 0 {
		ll = lead
	}
	if !l.lineTxt {
		ll = 0
	}
	l.y += l.lineH + ll
	l.x, l.lineH, l.lineLead, l.lineTxt = 0, 0, 0, false
	l.space = false
	l.line0 = len(l.boxes)
	l.center = false
	l.right = false
	l.lineR = 0
}

// alignLine markeert de huidige regel voor uitlijning volgens de stijl,
// mét de rechterrand van de context — in een gecentreerde smalle container
// is dat niet de paginarand.
func (l *layouter) alignLine(st style) {
	if st.center {
		l.center = true
	}
	if st.right {
		l.right = true
	}
	if st.center || st.right {
		l.lineR = l.lineRight(st.rIndent)
	}
}

// blockGap vraagt om verticale marge; opeenvolgende blokken delen de
// grootste (margin collapsing, het arme-mans-model).
func (l *layouter) blockGap(g int) {
	l.breakLine()
	if g > l.gap {
		l.gap = g
	}
}

func (l *layouter) flushGap() {
	if l.gap > 0 {
		// Ook boven het allereerste blok: een expliciete top-marge
		// (wikipedia's 4rem boven het wordmark) hoort alles omlaag te
		// schuiven — dat is geen witruimte-junk maar vormgeving.
		l.y += l.gap
		l.gap = 0
	}
}

// merge plakt woorden die op dezelfde regel met dezelfde stijl precies één
// spatie uit elkaar staan aan elkaar: minder boxes, minder tekenwerk.
func merge(in []Box) []Box {
	out := in[:0]
	for _, b := range in {
		if n := len(out); n > 0 {
			p := &out[n-1]
			if p.Text != "" && b.Text != "" && !p.Rule && !b.Rule && p.Img == nil && b.Img == nil &&
				p.Field == 0 && b.Field == 0 && p.Tile == nil && b.Tile == nil &&
				p.Scale == b.Scale && p.Col == b.Col && p.Href == b.Href &&
				p.Bold == b.Bold && p.Under == b.Under && p.Strike == b.Strike &&
				p.HasBG == b.HasBG && p.BG == b.BG &&
				p.Pin == b.Pin &&
				p.R.Min.Y == b.R.Min.Y && p.R.Max.X+charW(p.Scale) == b.R.Min.X {
				p.Text += " " + b.Text
				p.R.Max.X = b.R.Max.X
				continue
			}
		}
		out = append(out, b)
	}
	return out
}

func isSpace(b byte) bool { return b == ' ' || b == '\t' || b == '\n' || b == '\r' }
