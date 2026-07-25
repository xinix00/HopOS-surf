package browse

import (
	"bytes"
	"strings"

	"golang.org/x/net/html"
)

// --- <use>-symbolen ------------------------------------------------------------

// resolveUses lost svg <use>-referenties op vóór de layout: het symbool
// — document-intern (NRC's logo) of uit een externe sprite-sheet
// (tweakers' icons-symbol.svg, 217 symbolen) — wordt op de plek van de
// <use> ingelijmd en de omhullende <svg> erft zijn viewBox. Daarna is het
// gewoon een inline svg die de bestaande route rastert. Draait één keer
// per navigatie, in de nav-goroutine (de sheet-fetch mag blokkeren).
func (s *Session) resolveUses() {
	if s.doc == nil {
		return
	}
	var uses []*html.Node
	eachEl(s.doc, func(n *html.Node) {
		if n.Data == "use" && len(uses) < 64 {
			uses = append(uses, n)
		}
	})
	sheets := map[string]*html.Node{}
	fetched := 0
	for _, u := range uses {
		href, ok := attr(u, "href")
		if !ok {
			href, ok = attr(u, "xlink:href")
		}
		if !ok {
			continue
		}
		ref, id, found := strings.Cut(href, "#")
		if !found || id == "" {
			continue
		}
		root := s.doc
		if ref != "" {
			var seen bool
			if root, seen = sheets[ref]; !seen {
				root = nil
				// Een paar externe sheets per pagina; de gedeelde resource-
				// loader dedupliceert dezelfde URL met CSS/images/iconen.
				if fetched < 4 {
					fetched++
					if data, _, err := s.fetchResource(ref, cssMaxBytes); err == nil &&
						bytes.Contains(data, []byte("<svg")) {
						if d, err := html.Parse(bytes.NewReader(data)); err == nil {
							root = d
						}
					}
				}
				sheets[ref] = root
			}
		}
		if target := findByID(root, id); target != nil {
			injectSymbol(u, target)
		}
	}
}

// findByID: het element met dit id, waar ook in de boom (nil = niet daar).
func findByID(root *html.Node, id string) *html.Node {
	if root == nil {
		return nil
	}
	var res *html.Node
	var walk func(n *html.Node)
	walk = func(n *html.Node) {
		if res != nil {
			return
		}
		if n.Type == html.ElementNode {
			if v, ok := attr(n, "id"); ok && v == id {
				res = n
				return
			}
		}
		for c := n.FirstChild; c != nil && res == nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(root)
	return res
}

// injectSymbol vervangt de <use> door (een kloon van) zijn doel: van een
// <symbol>/<svg> de kínderen (die wikkels renderen zelf niet), van een
// los element (<g id>, <path id>) het element zelf. De omhullende <svg>
// zonder eigen viewBox krijgt die van het symbool — het symbool ís het
// coördinatenstelsel.
func injectSymbol(use, target *html.Node) {
	parent := use.Parent
	if parent == nil {
		return
	}
	if target.Data == "symbol" || target.Data == "svg" {
		for c := target.FirstChild; c != nil; c = c.NextSibling {
			parent.InsertBefore(cloneNode(c), use)
		}
	} else {
		parent.InsertBefore(cloneNode(target), use)
	}
	svg := parent
	for svg != nil && !(svg.Type == html.ElementNode && svg.Data == "svg") {
		svg = svg.Parent
	}
	parent.RemoveChild(use)
	if svg == nil {
		return
	}
	if w, h := svgViewBox(svg); w == 0 || h == 0 {
		if v, ok := attr(target, "viewbox"); ok {
			svg.Attr = append(svg.Attr, html.Attribute{Key: "viewBox", Val: v})
		} else if v, ok := attr(target, "viewBox"); ok {
			svg.Attr = append(svg.Attr, html.Attribute{Key: "viewBox", Val: v})
		}
	}
}

// cloneNode: een diepe kopie — de sheet blijft heel, elke <use> krijgt
// zijn eigen exemplaar.
func cloneNode(n *html.Node) *html.Node {
	c := &html.Node{Type: n.Type, Data: n.Data, DataAtom: n.DataAtom, Namespace: n.Namespace}
	c.Attr = append([]html.Attribute(nil), n.Attr...)
	for k := n.FirstChild; k != nil; k = k.NextSibling {
		c.AppendChild(cloneNode(k))
	}
	return c
}
