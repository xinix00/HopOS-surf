package browse

import (
	"strings"

	"golang.org/x/net/html"
)

// loadIcon haalt het officiële site-icoon op als vervanger voor logo's die
// pas door JavaScript of een niet-gerasterde webcomponent worden getekend.
func (s *Session) loadIcon() {
	s.icon = nil
	if s.addr == nil || (s.addr.Scheme != "http" && s.addr.Scheme != "https") {
		return
	}
	href := ""
	var walk func(n *html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "link" {
			rel, _ := attr(n, "rel")
			rel = strings.ToLower(rel)
			h, ok := attr(n, "href")
			if ok && h != "" {
				if strings.Contains(rel, "apple-touch-icon") {
					href = h
				} else if href == "" && strings.Contains(rel, "icon") &&
					strings.Contains(strings.ToLower(h), ".png") {
					href = h
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(s.doc)
	if href == "" {
		href = "/apple-touch-icon.png"
	}
	s.icon = s.fetchImage(href)
}
