package browse

import (
	"image"
	"strconv"

	"golang.org/x/net/html"
)

// layoutElementChildren legt uitsluitend de inhoud van een geopende doos.
// Kolomplanning, flex-volgorde en de gewone DOM-flow komen hier samen; het
// openen en sluiten van het boxmodel blijft in element.
func (l *layouter) layoutElementChildren(el *html.Node, cp props, st, childSt style, mar edges, isBlock, inlined bool) {
	tag := el.Data

	// margin-left:auto in een rij absorbeert vrije ruimte. Auto aan beide
	// kanten centreert het item.
	pushRight := -1
	pushHalf := false
	if mar.autoL && !isBlock && st.inline {
		pushRight = len(l.boxes)
		pushHalf = mar.autoR
	}

	if tag == "li" && isBlock && !st.blockify && st.marker != "" {
		m := st.marker
		if m == "1" {
			n := 1
			if st.list != nil {
				*st.list++
				n = *st.list
			}
			m = strconv.Itoa(n) + "."
		}
		l.word(m, st)
		l.space = true
	}
	if inlined {
		l.space = true
	}

	jcIdx := -1
	var jcStarts []int
	var jcSelf []string
	if rows, gap := l.columnPlan(el, cp, st, tag); rows != nil {
		rowGap := cssRowGap(l.css, cp)
		for ri, row := range rows {
			if ri > 0 && rowGap > 0 {
				l.y += rowGap
			}
			rowG := gap
			if row.gap >= 0 {
				rowG = row.gap
			}
			if !l.columns(row.cells, row.w, rowG, st, cp["justify-content"], cp["align-items"], cp["justify-items"], row.vast) {
				cst := childSt
				cst.inline, cst.blockify = false, true
				for _, cell := range row.cells {
					l.walk(cell, cst)
				}
			}
		}
	} else if cp["flex-direction"] == "column-reverse" {
		var kids []*html.Node
		for c := el.FirstChild; c != nil; c = c.NextSibling {
			kids = append(kids, c)
		}
		for i := len(kids) - 1; i >= 0; i-- {
			l.walk(kids[i], childSt)
		}
	} else if kids := elementChildren(el); childSt.inline && len(kids) > 0 && !hasDirectText(el) &&
		(cp["display"] == "flex" || cp["display"] == "inline-flex") {
		// Onthoud per flex-kind waar zijn output begon. Lege (vaak SVG-)
		// kinderen doen niet mee aan justify/align.
		jcIdx = len(l.boxes)
		for _, c := range l.flexOrder(kids) {
			jcStarts = append(jcStarts, len(l.boxes))
			jcSelf = append(jcSelf, l.propsOf(c)["align-self"])
			l.walk(c, childSt)
			if n := len(jcStarts); jcStarts[n-1] == len(l.boxes) {
				jcStarts, jcSelf = jcStarts[:n-1], jcSelf[:n-1]
			}
		}
	} else {
		for c := el.FirstChild; c != nil; c = c.NextSibling {
			l.walk(c, childSt)
		}
	}

	if inlined {
		l.space = true
	}
	if pushRight >= 0 && len(l.boxes) > pushRight {
		maxX := 0
		for i := pushRight; i < len(l.boxes); i++ {
			if x := l.boxes[i].R.Max.X; x > maxX {
				maxX = x
			}
		}
		if shift := l.lineRight(st.rIndent) - maxX; shift > 0 {
			if pushHalf {
				shift /= 2
			}
			for i := pushRight; i < len(l.boxes); i++ {
				l.boxes[i].R = l.boxes[i].R.Add(image.Pt(shift, 0))
			}
			l.x += shift
		}
	}
	if jcIdx >= 0 && len(l.boxes) > jcIdx {
		if groups := l.rowGroups(jcIdx, jcStarts); groups != nil {
			if jc := cp["justify-content"]; jc != "" {
				l.justify(jc, groups, st)
			}
			if ai := cp["align-items"]; ai != "" {
				l.alignRow(ai, groups, jcSelf)
			}
		}
	}
}
