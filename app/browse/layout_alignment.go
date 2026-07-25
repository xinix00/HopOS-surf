package browse

import (
	"image"
	"sort"

	"golang.org/x/net/html"
)

// rowGroup is één flex-kind op een inline rij: zijn boxrange en zijn
// omhullende rechthoek (een knop-doos mét zijn tekst is één groep).
type rowGroup struct {
	i0, i1 int
	r      image.Rectangle
}

// rowGroups bouwt de kind-groepen van een inline flex-rij uit de
// startmarkers. nil als de rij niet op één regel bleef (elke groep hoort
// bovenaan dezelfde regel te beginnen) — gewrapte regels zijn al vol.
func (l *layouter) rowGroups(from int, starts []int) []rowGroup {
	if len(starts) == 0 {
		return nil
	}
	gs := make([]rowGroup, len(starts))
	for gi, g0 := range starts {
		end := len(l.boxes)
		if gi+1 < len(starts) {
			end = starts[gi+1]
		}
		r := l.boxes[g0].R
		for i := g0 + 1; i < end; i++ {
			r = r.Union(l.boxes[i].R)
		}
		gs[gi] = rowGroup{i0: g0, i1: end, r: r}
	}
	top := gs[0].r.Min.Y
	for _, g := range gs {
		if g.r.Min.Y != top {
			return nil
		}
	}
	return gs
}

// justify verdeelt de vrije regelruimte van een inline flex-rij volgens
// justify-content: center schuift de rij op, flex-end tegen de rechterrand,
// space-between/around/evenly spreiden de kinderen.
func (l *layouter) justify(jc string, gs []rowGroup, st style) {
	maxX := 0
	for _, g := range gs {
		if g.r.Max.X > maxX {
			maxX = g.r.Max.X
		}
	}
	free := l.lineRight(st.rIndent) - maxX
	if free <= 0 {
		return
	}
	shift := func(g rowGroup, d int) {
		for i := g.i0; i < g.i1; i++ {
			l.boxes[i].R = l.boxes[i].R.Add(image.Pt(d, 0))
		}
	}
	n := len(gs)
	switch jc {
	case "center":
		for _, g := range gs {
			shift(g, free/2)
		}
		l.x += free / 2
	case "flex-end", "end", "right":
		for _, g := range gs {
			shift(g, free)
		}
		l.x += free
	case "space-between", "space-around", "space-evenly":
		if n < 2 && jc == "space-between" {
			return
		}
		pos := func(i int) int {
			switch jc {
			case "space-between":
				return free * i / (n - 1)
			case "space-around":
				return free * (2*i + 1) / (2 * n)
			default: // space-evenly
				return free * (i + 1) / (n + 1)
			}
		}
		for i, g := range gs {
			shift(g, pos(i))
		}
		l.x += pos(n - 1)
	}
}

// alignRow: align-items op een inline flex-rij — het hoogste kind bepaalt
// de rijhoogte (dat doet de regel al), de rest centreert of hangt aan de
// onderkant; align-self per kind wint. baseline benadert flex-end: bij één
// fontfamilie liggen de baselines onderin.
func (l *layouter) alignRow(ai string, gs []rowGroup, selves []string) {
	rowH := 0
	for _, g := range gs {
		if h := g.r.Dy(); h > rowH {
			rowH = h
		}
	}
	for gi, g := range gs {
		a := ai
		if gi < len(selves) && selves[gi] != "" && selves[gi] != "auto" {
			a = selves[gi]
		}
		d := 0
		switch a {
		case "center":
			d = (rowH - g.r.Dy()) / 2
		case "flex-end", "end", "baseline", "last baseline":
			d = rowH - g.r.Dy()
		}
		if d > 0 {
			for i := g.i0; i < g.i1; i++ {
				l.boxes[i].R = l.boxes[i].R.Add(image.Pt(0, d))
			}
		}
	}
}

// flexOrder sorteert flex/grid-items op hun order-property (stabiel, de
// DOM-volgorde als tiebreaker). Vrijwel altijd een no-op.
func (l *layouter) flexOrder(items []*html.Node) []*html.Node {
	ordered := false
	for _, it := range items {
		if cssOrder(l.propsOf(it)) != 0 {
			ordered = true
			break
		}
	}
	if !ordered {
		return items
	}
	out := append([]*html.Node{}, items...)
	sort.SliceStable(out, func(i, j int) bool {
		return cssOrder(l.propsOf(out[i])) < cssOrder(l.propsOf(out[j]))
	})
	return out
}
