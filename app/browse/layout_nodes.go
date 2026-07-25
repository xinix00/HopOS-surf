package browse

import (
	"net/url"
	"strings"

	"golang.org/x/net/html"
)

// propsOf: de computed props van een element — de stylesheet-match plus het
// inline style=""-attribuut (dat wint altijd).
func (l *layouter) propsOf(el *html.Node) props {
	cp := l.styles[el]
	if inline, ok := attr(el, "style"); ok && inline != "" {
		if d := parseDecls(inline); d != nil {
			m := make(props, len(cp)+len(d))
			for k, v := range cp {
				m[k] = v
			}
			for k, v := range d {
				m[k] = v
			}
			return m
		}
	}
	return cp
}

// elementChildren geeft de element-kinderen; direct-tekst telt apart.
func elementChildren(el *html.Node) []*html.Node {
	var out []*html.Node
	for c := el.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode && !skip[c.Data] {
			out = append(out, c)
		}
	}
	return out
}

// kids is elementChildren mét display:contents opengevouwen: zo'n element
// maakt géén eigen box — zijn kinderen doen mee in de opmaakcontext van de
// ouder (tweakers hijst zo headline-blocks en de ankeiler-stream het
// voorpagina-rooster in). Begrensd diep: contents-in-contents komt voor,
// een lus niet.
func (l *layouter) kids(el *html.Node) []*html.Node {
	var out []*html.Node
	var add func(*html.Node, int)
	add = func(n *html.Node, depth int) {
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if c.Type != html.ElementNode || skip[c.Data] {
				continue
			}
			if depth < 4 && l.propsOf(c)["display"] == "contents" {
				add(c, depth+1)
				continue
			}
			out = append(out, c)
		}
	}
	add(el, 0)
	return out
}

// renderableText: de tekst die wíj zouden tekenen — alles onder skip-
// elementen (svg, script, style) telt niet mee (een <svg><title> is geen
// zichtbare tekst).
func renderableText(n *html.Node) string {
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(c *html.Node) {
		if c.Type == html.ElementNode && skip[c.Data] {
			return
		}
		if c.Type == html.TextNode {
			b.WriteString(c.Data)
		}
		for k := c.FirstChild; k != nil; k = k.NextSibling {
			walk(k)
		}
	}
	walk(n)
	return b.String()
}

// isRootHref: linkt dit naar de voorpagina ("/", of de site-root)?
// Een kaal "#" (hamburger-triggers) is géén voorpagina-link.
func isRootHref(href string) bool {
	href = strings.TrimSpace(href)
	if href == "" || href == "#" {
		return false
	}
	u, err := url.Parse(href)
	if err != nil {
		return false
	}
	return (u.Path == "" || u.Path == "/") && u.RawQuery == "" && u.Fragment == ""
}

// hasDirectText: staat er échte tekst (geen witruimte) direct in dit
// element? Dan is kolommen maken gevaarlijk — de tekst zou verdwijnen.
func hasDirectText(el *html.Node) bool {
	for c := el.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.TextNode && strings.TrimSpace(c.Data) != "" {
			return true
		}
	}
	return false
}

// emptyContent: zit er niets renderbaars in — geen tekst (buiten svg/
// script om), geen geladen afbeelding, geen formulier-widget?
func (l *layouter) emptyContent(n *html.Node) bool {
	if strings.TrimSpace(renderableText(n)) != "" {
		return false
	}
	found := false
	var w func(*html.Node)
	w = func(c *html.Node) {
		if found {
			return
		}
		if c.Type == html.ElementNode {
			switch c.Data {
			case "img":
				if src, _ := attr(c, "src"); l.imgs[src] != nil {
					found = true
					return
				}
			case "svg":
				// Sinds we svg rasteren is een svg-logo échte inhoud: het
				// logo-slot hoeft hem niet meer te vervangen — mits hij een
				// maat heeft, anders valt er niets te rasteren.
				if svgRenderable(c) {
					found = true
					return
				}
			case "input", "textarea", "select":
				found = true
				return
			}
		}
		for k := c.FirstChild; k != nil && !found; k = k.NextSibling {
			w(k)
		}
	}
	w(n)
	return !found
}

// cellVisible: gaat deze flex-cel iets laten zien? Een verborgen kind
// (display:none-hamburger, sr-only-kop) is geen flex-item; verder telt
// renderbare inhoud, of een vulbaar logo-slot (voorpagina-link +
// site-icoon) — anders filtert de cel weg vóórdat het slot gevuld kan
// worden.
// cellHidden: dit kind is écht onzichtbaar (display:none e.d.) — de lichte
// variant van cellVisible, voor grid-cellen: een leeg-maar-gedecoreerd
// element (stipje, voortgangsbalk) ís daar gewoon een cel.
func (l *layouter) cellHidden(n *html.Node) bool {
	cp := l.propsOf(n)
	return cp["display"] == "none" || cp["visibility"] == "hidden" || cp[srProp] == "1"
}

func (l *layouter) cellVisible(n *html.Node) bool {
	if cp := l.propsOf(n); cp["display"] == "none" || cp["visibility"] == "hidden" || cp[srProp] == "1" {
		return false
	}
	if !l.emptyContent(n) {
		return true
	}
	if l.icon == nil {
		return false
	}
	found := false
	var w func(*html.Node)
	w = func(c *html.Node) {
		if found {
			return
		}
		if c.Type == html.ElementNode && c.Data == "a" {
			if href, ok := attr(c, "href"); ok && isRootHref(href) {
				found = true
				return
			}
		}
		for k := c.FirstChild; k != nil && !found; k = k.NextSibling {
			w(k)
		}
	}
	w(n)
	return found
}
