package browse

import "golang.org/x/net/html"

// sizedWrapRows pakt wrap-items met een eigen breedte (width/flex-basis)
// in rijen zoals echte flex-wrap: zoveel als er passen. Elke cel krijgt de
// búitenmaat (breedte + marges) — het item positioneert zich daarbinnen
// met zijn eigen marges, precies zoals in de flow. De gap telt alleen als
// de site hem echt declareert (de 8px-default is van ons; marges zitten
// al in de maat). nil zodra een item geen (bruikbare) maat heeft — dan is
// de kaartmaat-heuristiek aan zet.
func (l *layouter) sizedWrapRows(items []*html.Node, availW int, cp props) []colRow {
	packGap := 0
	if cp["gap"] != "" || cp["column-gap"] != "" {
		packGap = cssGap(l.css, cp)
	}
	type sized struct {
		n   *html.Node
		eff int
	}
	var its []sized
	for _, n := range items {
		icp := l.propsOf(n)
		_, basis := flexItem(l.css, icp, availW)
		if basis < 0 {
			if v, ok := cssLenPct(l.css, icp["width"], availW); ok && v > 0 {
				basis = v
			}
		}
		if basis < 90 || basis > availW {
			return nil
		}
		mar := cssEdgesOf(l.css, icp, "margin", 96)
		its = append(its, sized{n: n, eff: basis + mar.l + mar.r})
	}
	var rows []colRow
	cur := colRow{gap: packGap}
	x := 0
	for _, s := range its {
		adv := s.eff
		if len(cur.cells) > 0 {
			adv += packGap
		}
		if len(cur.cells) > 0 && x+adv > availW {
			rows = append(rows, cur)
			cur, x = colRow{gap: packGap}, 0
			adv = s.eff
		}
		cur.cells = append(cur.cells, s.n)
		cur.w = append(cur.w, s.eff)
		x += adv
	}
	if len(cur.cells) > 0 {
		rows = append(rows, cur)
	}
	return rows
}

// flexTracks: kolombreedtes voor één flex-rij — vaste maten (width of
// flex-basis) eerst, de vrije ruimte naar rato van flex-grow; een item
// zonder maat of gewicht deelt mee als gewicht 1 (zijn content-maat kennen
// we hier niet). sized zegt of er überhaupt een expliciet maat-signaal
// stond; nil als het niet past of te smal wordt.
func (l *layouter) flexTracks(items []*html.Node, availW, gap int) (colW []int, sized bool) {
	out := make([]int, len(items))
	weight := make([]float64, len(items))
	free := availW - gap*(len(items)-1)
	totW := 0.0
	for i, it := range items {
		cp := l.propsOf(it)
		g, basis := flexItem(l.css, cp, availW)
		if basis < 0 {
			if v, ok := cssLenPct(l.css, cp["width"], availW); ok && v > 0 {
				basis = v
			}
		}
		if basis > 0 {
			out[i] = basis
			free -= basis
			sized = true
		}
		weight[i] = g
		if g > 0 {
			sized = true
		}
		totW += weight[i]
	}
	// Zonder maat en zonder grow is een flex-item content-sized (flex-grow
	// is per spec 0!) — meten dus, in plaats van de rij vol te delen: zo
	// blijft er échte vrije ruimte over en heeft justify-content iets te
	// verdelen (easyflorists space-between-header: menu links, knoppen
	// uiterst rechts).
	for i, it := range items {
		if out[i] == 0 && weight[i] == 0 {
			w := l.measureCell(it, free)
			if w < 16 {
				w = 16
			}
			out[i] = w
			free -= w
		}
	}
	if free < 0 {
		return nil, sized // past niet naast elkaar
	}
	for i := range out {
		if weight[i] > 0 && totW > 0 {
			out[i] += int(float64(free) * weight[i] / totW)
			if out[i] < 90 {
				return nil, sized // een grow-kolom hoort ruimte te hebben
			}
		}
	}
	return out, sized
}

// measureCell: de natuurlijke (max-content-achtige) breedte van een
// flex-item — een proeflayout op de beschikbare ruimte, gemeten op wat er
// echt staat. Menu's en knoppenrijtjes zijn zo breed als hun inhoud.
func (l *layouter) measureCell(cell *html.Node, avail int) int {
	if avail < 24 {
		return 0
	}
	sub := l.subLayout(cell, avail, style{scale: 1, col: colText, marker: "-"}, false)
	uMin, uMax := subExtent(sub, false)
	if uMax <= uMin {
		return 0
	}
	// +2×pad: de cel-sub legt zijn inhoud tussen de pad-kantlijnen — die
	// marge moet mee in de celbreedte, anders wrapt de cel-layout nét.
	w := uMax - uMin + 2*sub.pad
	if w > avail {
		w = avail
	}
	return w
}
