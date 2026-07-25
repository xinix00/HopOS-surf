package browse

import (
	"image"

	"golang.org/x/net/html"
)

// columns legt één rij cellen naast elkaar: elke cel zijn eigen
// sub-layouter op kolombreedte, daarna verschoven naar zijn kolom-x. De
// rij wordt zo hoog als de hoogste cel; justify-content verdeelt de vrije
// rijruimte (als de kolommen de rij niet vullen) en align-items/-self
// bepaalt waar een lagere cel verticaal hangt. Eerst wordt speculatief
// gelegd: zijn de celhoogtes wild uit balans, dan is dit geen kaartenrij
// maar pagina-steigerwerk (een titelblokje naast een eindeloze
// nieuwskolom) — dan géén commit (false) en stapelt de aanroeper gewoon.
func (l *layouter) columns(cells []*html.Node, colW []int, gap int, st style, jc, ai, ji string, vast bool) bool {
	subs := make([]*layouter, len(cells))
	maxH, minH := 0, 1<<30
	for i, cell := range cells {
		if cell == nil {
			continue // een rowspan-gat: de kolom blijft open maar leeg
		}
		sub := l.subLayout(cell, colW[i%len(colW)], st, false)
		subs[i] = sub
		if sub.y > maxH {
			maxH = sub.y
		}
		if sub.y < minH {
			minH = sub.y
		}
	}
	// Balans-check: kaartenrijen en teasers zijn (ruwweg) even hoog; een
	// kolom die torenhoog boven de rest uitsteekt hoort niet naast maar
	// boven/onder de rest. Kleine rijen zijn altijd goed — en een rij uit
	// een expliciete areas-template ook: main-naast-sidebar ís de site
	// (tweakers' "editorial-content editorial-content sidebar").
	if !vast && maxH > 700 && maxH > 3*minH {
		return false
	}
	l.breakLine()
	l.flushGap()
	x0 := l.lineLeft(st.indent)
	// justify-content: kolommen met vaste maten vullen de rij niet — de
	// vrije ruimte wordt verdeeld (rijen uit flexTracks hebben die zelden).
	total := gap * (len(cells) - 1)
	for i := range cells {
		total += colW[i%len(colW)]
	}
	if free := l.lineRight(st.rIndent) - x0 - total; free > 0 {
		n := len(cells)
		switch jc {
		case "center":
			x0 += free / 2
		case "flex-end", "end", "right":
			x0 += free
		case "space-between":
			if n > 1 {
				gap += free / (n - 1)
			}
		case "space-around":
			x0 += free / (2 * n)
			gap += free / n
		case "space-evenly":
			x0 += free / (n + 1)
			gap += free / (n + 1)
		}
	}
	y0 := l.y
	cx := x0
	for i, sub := range subs {
		if sub == nil {
			cx += colW[i%len(colW)] + gap
			continue
		}
		// align-items (align-self van de cel wint): waar hangt een lagere
		// cel in de rij? De default (stretch) rekt zijn kaartvlak op.
		// vertical-align is de tabel-taal voor hetzelfde (cellen!).
		a := ai
		switch l.propsOf(cells[i])["vertical-align"] {
		case "middle":
			a = "center"
		case "bottom":
			a = "flex-end"
		case "top":
			a = "start"
		}
		if v, ok := l.propsOf(cells[i])["align-self"]; ok && v != "auto" {
			a = v
		}
		dy := 0
		switch a {
		case "center":
			dy = (maxH - sub.y) / 2
		case "flex-end", "end", "baseline", "last baseline":
			dy = maxH - sub.y
		}
		// align-items: stretch (de flex-default): een kaartvlak dat de
		// hele cel besloeg groeit mee tot de rijhoogte — gelijke kaarten.
		if a == "" || a == "stretch" || a == "normal" {
			for k := range sub.boxes {
				b := &sub.boxes[k]
				if b.Text == "" && b.Img == nil && (b.HasBG || b.HasBrd || b.Tile != nil) &&
					b.R.Min.Y <= 4 && b.R.Max.Y >= sub.y-4 {
					b.R.Max.Y = maxH
				}
			}
		}
		// justify-items (justify-self van de cel wint): bij center/end
		// krimpt het item naar zijn inhoud (shrink-to-fit) en schuift het
		// binnen zijn cel — grid-knoppen midden of tegen de celrand.
		j, shrink := ji, true
		if v, ok := l.propsOf(cells[i])["justify-self"]; ok && v != "auto" {
			j = v
		}
		// space-between/end: het láátste item raakt de containerrand (echte
		// flex) — de gemeten cel houdt anders zijn pad-marges als kier.
		// Zonder klemmen: een kaart die zijn cel vult blijft gewoon staan.
		if j == "" && i == len(subs)-1 && (jc == "space-between" || jc == "flex-end" || jc == "end" || jc == "right") {
			j, shrink = "end", false
		}
		dx := 0
		if j == "center" || j == "end" || j == "flex-end" || j == "right" {
			m0, m1 := subExtent(sub, !shrink)
			if free := colW[i%len(colW)] - (m1 - m0); free > 0 {
				dx = free
				if j == "center" {
					dx = free / 2
				}
			}
		}
		// sub begint op zijn eigen pad-marge
		l.adopt(sub, image.Pt(cx-sub.pad+dx, y0+dy), false)
		cx += colW[i%len(colW)] + gap
	}
	l.y = y0 + maxH
	l.x, l.lineH, l.space = 0, 0, false
	l.line0 = len(l.boxes)
	return true
}
