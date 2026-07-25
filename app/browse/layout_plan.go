package browse

import (
	"sort"
	"strconv"
	"strings"

	"golang.org/x/net/html"
)

// colRow is één kolommenrij uit columnPlan: de cellen en hun breedtes —
// per rij, want een grid-cel met een grid-column-span is breder dan zijn
// eigen track.
type colRow struct {
	cells []*html.Node
	w     []int
	gap   int  // -1: de standaard kolom-gap; anders expliciet (maat-rijen: 0)
	vast  bool // uit een expliciete areas-template: de site declareert dit
	// letterlijk — de balans-heuristiek (anti-steigerwerk) blijft er vanaf
}

// columnPlan beslist of dit element als kolommen rendert en hoe breed die
// worden: een tabel (rijen van td/th-cellen), een grid (tracks uit
// grid-template-columns) of een flex-rij met blok-kinderen. nil = gewone
// flow. Menu's (flex-rij vol linkjes) blijven bewust inline.
func (l *layouter) columnPlan(el *html.Node, cp props, st style, tag string) ([]colRow, int) {
	availW := l.lineRight(st.rIndent) - l.lineLeft(st.indent)
	// De 320px-vangrail beschermt de heurístieken (flex-rijen raden); een
	// expliciet grid mét tracks of areas is een uitspraak van de site — die
	// mag ook in een 300px-plank kolommen maken (tweakers' Best Buy Guides).
	// gridTracks bewaakt zelf de minimum-spoorbreedte.
	expliciet := cp["display"] == "grid" &&
		(cp["grid-template-columns"] != "" || cp["grid-template-areas"] != "")
	if availW < 320 && !(expliciet && availW >= 160) {
		return nil, 0 // te smal om te verdelen: stapelen leest beter
	}
	gap := cssGap(l.css, cp)
	equal := func(n int) []int {
		w := (availW - gap*(n-1)) / n
		if w < 100 {
			return nil
		}
		colW := make([]int, n)
		for i := range colW {
			colW[i] = w
		}
		return colW
	}
	sameW := func(rows [][]*html.Node, colW []int) []colRow {
		out := make([]colRow, len(rows))
		for i, r := range rows {
			out[i] = colRow{cells: r, w: colW, gap: -1}
		}
		return out
	}
	switch {
	case tag == "table":
		// Rijen van td/th-cellen; colspan telt mee in de kolomtelling en
		// geeft de cel de breedte van zijn overspannen kolommen; rowspan
		// bezet zijn kolom óók in de rijen eronder (de cel staat één keer,
		// de kolommen eronder blijven leeg maar de uitlijning klopt).
		type trow struct {
			cells []*html.Node
			spans []int
			rspan []int
		}
		var rows []trow
		ncol := 0
		var walkT func(n *html.Node)
		walkT = func(n *html.Node) {
			if n.Type == html.ElementNode && n.Data == "tr" {
				var row trow
				total := 0
				for c := n.FirstChild; c != nil; c = c.NextSibling {
					if c.Type == html.ElementNode && (c.Data == "td" || c.Data == "th") {
						s, rs := 1, 1
						if v, ok := attr(c, "colspan"); ok {
							if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && n > 1 && n <= 6 {
								s = n
							}
						}
						if v, ok := attr(c, "rowspan"); ok {
							if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && n > 1 && n <= 6 {
								rs = n
							}
						}
						row.cells = append(row.cells, c)
						row.spans = append(row.spans, s)
						row.rspan = append(row.rspan, rs)
						total += s
					}
				}
				if len(row.cells) > 0 {
					rows = append(rows, row)
					if total > ncol {
						ncol = total
					}
				}
				return
			}
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				walkT(c)
			}
		}
		walkT(el)
		if len(rows) == 0 || ncol < 2 || ncol > 4 {
			return nil, 0 // één kolom of te veel: stapelen leest beter
		}
		// Gedeclareerde kolombreedtes: een cel met een CSS-width (of het
		// ouderwetse width-attribuut — presentational hint) pint zijn
		// kolom; de overige kolommen delen wat overblijft. Zonder
		// declaraties (of als het niet past): gelijke kolommen.
		decl := make([]int, ncol)
		for _, r := range rows {
			t := 0
			for i, c := range r.cells {
				if r.spans[i] == 1 && t < ncol && decl[t] == 0 {
					if v, ok := cssLenPct(l.css, l.propsOf(c)["width"], availW); ok && v >= 48 && v < availW {
						decl[t] = v
					}
				}
				t += r.spans[i]
			}
		}
		base := equal(ncol)
		nvrij, som := 0, 0
		for _, d := range decl {
			som += d
			if d == 0 {
				nvrij++
			}
		}
		if som > 0 {
			if rest := availW - gap*(ncol-1) - som; nvrij == 0 && rest >= 0 {
				base = decl
			} else if nvrij > 0 && rest/nvrij >= 100 {
				base = make([]int, ncol)
				for i, d := range decl {
					if d == 0 {
						d = rest / nvrij
					}
					base[i] = d
				}
			}
		}
		if base != nil {
			out := make([]colRow, len(rows))
			carry := make([]int, ncol) // resterende rowspan per kolom
			for i, r := range rows {
				cr := colRow{gap: -1}
				t, ci := 0, 0
				for t < ncol {
					if carry[t] > 0 {
						// Bezet door een rowspan van hierboven: een lege cel
						// houdt de kolom open, de rest lijnt gewoon uit.
						carry[t]--
						cr.cells = append(cr.cells, nil)
						cr.w = append(cr.w, base[t])
						t++
						continue
					}
					if ci >= len(r.cells) {
						break
					}
					s := r.spans[ci]
					if s > ncol-t {
						s = ncol - t
					}
					w := gap * (s - 1)
					for k := 0; k < s; k++ {
						w += base[t+k]
						if r.rspan[ci] > 1 {
							carry[t+k] = r.rspan[ci] - 1
						}
					}
					cr.cells = append(cr.cells, r.cells[ci])
					cr.w = append(cr.w, w)
					t += s
					ci++
				}
				out[i] = cr
			}
			return out, gap
		}
	case cp["display"] == "grid":
		// Het centreer-spoor is géén kolommenset: element() schuift de
		// inhoud al naar de middenbaan, de kinderen stapelen daar.
		if gridRailPx(l.css, cp["grid-template-columns"], availW, gap) > 0 {
			return nil, 0
		}
		// grid-template-areas: benoemde gebieden — de rijen komen
		// letterlijk uit de template ("kop kop" / "zij hoofd"), de
		// kolombreedtes uit de tracks; een naam die kolommen herhaalt
		// spant die tracks (het holy-grail-patroon). Een gat ("." of een
		// slot zonder element — tweakers' lege ad-slots) is een lege cel,
		// een rowspan (zelfde naam in een latere rij) houdt zijn kolom
		// bezet maar leeg — net als tabel-rowspan; de inhoud staat één
		// keer, de uitlijning klopt.
		if areas := gridAreas(cp["grid-template-areas"]); areas != nil {
			tracks := gridTracks(l.css, cp["grid-template-columns"], availW, gap)
			if tracks == nil || len(tracks) != len(areas[0]) {
				tracks = equal(len(areas[0]))
			}
			if tracks != nil {
				byName := map[string]*html.Node{}
				for _, it := range l.kids(el) {
					if n := l.propsOf(it)["grid-area"]; n != "" {
						byName[n] = it
					}
				}
				if len(byName) > 0 {
					seen := map[string]int{}
					var rows []colRow
					for ri, row := range areas {
						cr := colRow{gap: -1, vast: true}
						echt := false
						for c := 0; c < len(row); {
							name := row[c]
							span := 1
							for c+span < len(row) && row[c+span] == name {
								span++
							}
							w := gap * (span - 1)
							for k := 0; k < span && c+k < len(tracks); k++ {
								w += tracks[c+k]
							}
							it := byName[name]
							if r0, was := seen[name]; was && r0 != ri {
								it = nil // rowspan-vervolg: kolom bezet, cel leeg
							} else if it != nil {
								seen[name] = ri
								echt = true
							}
							cr.cells = append(cr.cells, it)
							cr.w = append(cr.w, w)
							c += span
						}
						// Rijen zonder één echt element (louter gaten of
						// rowspan-vervolg) nemen geen ruimte.
						if echt {
							rows = append(rows, cr)
						}
					}
					if len(rows) > 0 {
						// Auto-geplaatste kinderen (zonder grid-area) horen
						// in impliciete rijen ónder de template, over
						// dezelfde tracks — tweakers' kleine ankeilers,
						// twee per rij.
						placed := map[*html.Node]bool{}
						for _, r := range rows {
							for _, c := range r.cells {
								if c != nil {
									placed[c] = true
								}
							}
						}
						var vrij []*html.Node
						for _, it := range l.kids(el) {
							if !placed[it] && !l.cellHidden(it) {
								vrij = append(vrij, it)
							}
						}
						for i := 0; i < len(vrij); i += len(tracks) {
							end := i + len(tracks)
							if end > len(vrij) {
								end = len(vrij)
							}
							cr := colRow{gap: -1, vast: true}
							for k := i; k < end; k++ {
								cr.cells = append(cr.cells, vrij[k])
								cr.w = append(cr.w, tracks[k-i])
							}
							rows = append(rows, cr)
						}
						return rows, gap
					}
				}
			}
		}
		// Verborgen kinderen zijn geen grid-cellen (tweakers' dichte
		// overlay-<ul>'s tussen de Best Buy-kaarten); leeg-maar-gedecoreerd
		// (stipjes, voortgangsbalken) blijft gewoon een cel.
		var items []*html.Node
		for _, it := range l.kids(el) {
			if !l.cellHidden(it) {
				items = append(items, it)
			}
		}
		tracks := gridTracks(l.css, cp["grid-template-columns"], availW, gap)
		if tracks == nil {
			// grid-area: <rijnummer> zónder template (tweakers' editorial-
			// content: ankeiler rij 1, de nieuwslijst rij 2): genummerde
			// kinderen krijgen elk hun eigen rij, op nummer; "unset"/namen
			// tellen als ongenummerd en volgen eronder in documentvolgorde.
			// Puur volgorde-werk: één kolom, volle breedte.
			type nummerd struct {
				el *html.Node
				nr int
			}
			var genummerd []nummerd
			var rest []*html.Node
			for _, it := range items {
				ga := strings.TrimSpace(l.propsOf(it)["grid-area"])
				if i := strings.IndexByte(ga, '/'); i >= 0 {
					ga = strings.TrimSpace(ga[:i])
				}
				if n, err := strconv.Atoi(ga); err == nil && n > 0 && n <= 64 {
					genummerd = append(genummerd, nummerd{it, n})
				} else {
					rest = append(rest, it)
				}
			}
			if len(genummerd) > 0 {
				sort.SliceStable(genummerd, func(i, j int) bool { return genummerd[i].nr < genummerd[j].nr })
				var rows []colRow
				for _, g := range genummerd {
					rows = append(rows, colRow{cells: []*html.Node{g.el}, w: []int{availW}, gap: -1, vast: true})
				}
				for _, it := range rest {
					rows = append(rows, colRow{cells: []*html.Node{it}, w: []int{availW}, gap: -1, vast: true})
				}
				return rows, gap
			}
		}
		if tracks == nil && strings.HasPrefix(cp["grid-auto-flow"], "column") && len(items) >= 2 {
			// grid-auto-flow: column zonder template: elk item zijn eigen
			// kolom — één rij naast elkaar (tweakers' categoriebalk). Te
			// smal per cel: dan stapelen (zo doet hun mobiel het ook).
			if w := (availW - gap*(len(items)-1)) / len(items); w >= 48 {
				colW := make([]int, len(items))
				for i := range colW {
					colW[i] = w
				}
				return []colRow{{cells: items, w: colW, gap: -1}}, gap
			}
		}
		if tracks == nil || len(items) < 2 {
			return nil, 0
		}
		// Plaatsen mét grid-column-spans: een cel die (via "1 / -1" of
		// "span N") meer tracks pakt krijgt de breedte van die tracks; past
		// hij niet meer op de lopende rij, dan begint hij een nieuwe.
		spanW := func(t, s int) int {
			w := gap * (s - 1)
			for k := 0; k < s; k++ {
				w += tracks[t+k]
			}
			return w
		}
		var rows []colRow
		cur := colRow{gap: -1}
		t := 0
		for _, it := range items {
			s := gridSpan(l.propsOf(it), len(tracks))
			if t+s > len(tracks) && t > 0 {
				rows = append(rows, cur)
				cur, t = colRow{gap: -1}, 0
			}
			cur.cells = append(cur.cells, it)
			cur.w = append(cur.w, spanW(t, s))
			t += s
			if t >= len(tracks) {
				rows = append(rows, cur)
				cur, t = colRow{gap: -1}, 0
			}
		}
		if len(cur.cells) > 0 {
			rows = append(rows, cur)
		}
		return rows, gap
	case cp["display"] == "flex" || cp["display"] == "inline-flex":
		fd := cp["flex-direction"]
		if fd == "column" || fd == "column-reverse" {
			return nil, 0 // een kolom stapelt — dat doet de gewone flow al
		}
		if hasDirectText(el) {
			return nil, 0
		}
		// Cellen zonder zichtbare inhoud (een svg-logo dat wij niet
		// rasteren) doen niet mee — anders wordt zo'n cel een lege
		// gekleurde doos en staat de rest scheef ernaast geperst.
		var items []*html.Node
		for _, it := range elementChildren(el) {
			if l.cellVisible(it) {
				items = append(items, it)
			}
		}
		items = l.flexOrder(items)
		if fd == "row-reverse" {
			for i, j := 0, len(items)-1; i < j; i, j = i+1, j-1 {
				items[i], items[j] = items[j], items[i]
			}
		}
		if len(items) < 2 {
			return nil, 0
		}
		blockish := false
		for _, it := range items {
			if blocks[it.Data] || it.Data == "img" || it.Data == "picture" || it.Data == "video" {
				blockish = true
			}
		}
		if !blockish {
			return nil, 0 // allemaal linkjes: dat is een menu
		}
		// Wrappen: bij expliciete flex-wrap, of bij veel items mét een
		// eigen maat (width/flex-basis, vaak calc(50% - marge) — NRC): die
		// maat bepaalt hoeveel er per rij passen, echte flex-wrap. Zónder
		// wrap en zonder maten is nowrap de default — veel kale linkjes
		// (tweakers' menubalk) zijn een menu, geen kaartenraster.
		wrapDeclared := strings.HasPrefix(cp["flex-wrap"], "wrap")
		if rows := l.sizedWrapRows(items, availW, cp); (rows != nil && (wrapDeclared || len(items) > 4)) || wrapDeclared {
			if rows == nil {
				cols := availW / 220
				if cols < 2 {
					cols = 2
				}
				if cols > 4 {
					cols = 4
				}
				if cols > len(items) {
					cols = len(items)
				}
				colW := equal(cols)
				if colW == nil {
					return nil, 0
				}
				var chunks [][]*html.Node
				for i := 0; i < len(items); i += cols {
					end := i + cols
					if end > len(items) {
						end = len(items)
					}
					chunks = append(chunks, items[i:end])
				}
				rows = sameW(chunks, colW)
			}
			// wrap-reverse: de rijen stapelen van onder naar boven.
			if cp["flex-wrap"] == "wrap-reverse" {
				for i, j := 0, len(rows)-1; i < j; i, j = i+1, j-1 {
					rows[i], rows[j] = rows[j], rows[i]
				}
			}
			return rows, gap
		}
		// Eén rij: breedtes uit width/flex-basis/flex-grow. Zonder expliciete
		// rij-declaratie én zonder maat-signalen niet committen: inhoud-
		// gemeten items naast elkaar is precies wat de inline-flow al doet
		// (mét auto-marges), en een header is geen kaartenrij.
		explicitRow := fd == "row" || fd == "row-reverse" || cp["flex-wrap"] != ""
		colW, sized := l.flexTracks(items, availW, gap)
		if colW != nil && (explicitRow || sized) {
			return []colRow{{cells: items, w: colW, gap: -1}}, gap
		}
		if explicitRow {
			if colW := equal(len(items)); colW != nil {
				return []colRow{{cells: items, w: colW, gap: -1}}, gap
			}
		}
	}
	return nil, 0
}
