package browse

import (
	"net/url"
	"sort"
	"strings"
	"sync"

	"github.com/andybalholm/cascadia"
	"golang.org/x/net/html"
)

// --- CSS laden en matchen -----------------------------------------------------

// Géén inhouds-limieten meer op CSS (Derek 23-07: elke cap kostte stilletjes
// de helft van een echte site — eerst de header-regel in sheet #8, toen de
// mobiele overrides achter de rules-cap, toen de match-deadline halverwege).
// Wat blijft is één anti-oneindige-stream-slot per fetch; de nettimeout
// (de Session-client, 20s) begrenst de tijd.
const cssMaxBytes = 8 << 20 // per sheet over de lijn — ver boven elke echte sheet

// loadStyles verzamelt de <style>-blokken en <link rel=stylesheet>-sheets,
// parset ze tot regels en matcht elke regel één keer met cascadia over de
// boom, in cascade-volgorde (specificiteit, bron). Het resultaat is
// Session.matched — breedte-onafhankelijk; stylesFor rekent daar per
// framebreedte de computed props uit. Draait in de nav-goroutine.
func (s *Session) loadStyles() {
	s.matched, s.styleCache, s.styleW = nil, nil, 0
	var rules []cssRule
	// media=""-attribuut van de sheet: reist als conditie met de regels mee,
	// net als een omhullend @media-blok.
	sheetMQ := func(n *html.Node) ([]string, bool) {
		m, ok := attr(n, "media")
		if !ok || strings.TrimSpace(m) == "" {
			return nil, true
		}
		if !mediaAnyWidth(m) {
			return nil, false // print e.d.: kan nooit gelden
		}
		return []string{m}, true
	}
	// Eerst verzamelen (de cascade-volgorde is heilig), dan de externe
	// sheets parallel over de lijn, dan in bronvolgorde parsen.
	type bron struct {
		tekst string
		href  string
		mq    []string
	}
	var bronnen []*bron
	eachEl(s.doc, func(n *html.Node) {
		switch n.Data {
		case "style":
			if mq, ok := sheetMQ(n); ok {
				bronnen = append(bronnen, &bron{tekst: textContent(n), mq: mq})
			}
		case "link":
			rel, _ := attr(n, "rel")
			href, _ := attr(n, "href")
			if strings.EqualFold(strings.TrimSpace(rel), "stylesheet") && href != "" {
				if mq, ok := sheetMQ(n); ok {
					bronnen = append(bronnen, &bron{href: href, mq: mq})
				}
			}
		}
	})
	var wg sync.WaitGroup
	sem := make(chan struct{}, 6)
	for _, b := range bronnen {
		if b.href == "" {
			continue
		}
		wg.Add(1)
		go func(b *bron) {
			defer wg.Done()
			sem <- struct{}{}
			b.tekst = s.fetchText(b.href)
			<-sem
		}(b)
	}
	wg.Wait()
	for _, b := range bronnen {
		// @import: sheets die sheets laden — relatieve verwijzingen resolven
		// tegen de importerende sheet, niet tegen de pagina.
		base := s.base
		if b.href != "" {
			if u, err := s.resolve(b.href); err == nil {
				base = u
			}
		}
		b.tekst = s.expandImports(b.tekst, base, 0)
		rules = append(rules, parseCSSm(b.tekst, len(rules), b.mq)...)
	}
	if len(rules) == 0 {
		return
	}
	sort.SliceStable(rules, func(i, j int) bool {
		if rules[i].spec != rules[j].spec {
			return rules[i].spec < rules[j].spec
		}
		return rules[i].seq < rules[j].seq
	})
	for _, r := range rules {
		sel, err := cascadia.Parse(r.sel)
		if err != nil {
			continue // selector die cascadia niet kent: regel vervalt
		}
		nodes := cascadia.QueryAll(s.doc, sel)
		if len(nodes) == 0 {
			continue
		}
		s.matched = append(s.matched, matchedRule{mq: r.mq, decls: r.decls, nodes: nodes})
	}
}

// stylesFor rekent de computed props uit voor deze framebreedte: de
// gematchte regels langs (cascade-volgorde), media-condities evalueren,
// var()'s oplossen. Goedkoop genoeg om per resize te doen — zo IS een
// breed venster de desktopsite. De laatste breedte is gecachet.
func (s *Session) stylesFor(width int) map[*html.Node]props {
	if s.styleCache != nil && s.styleW == width {
		return s.styleCache
	}
	styles := map[*html.Node]props{}
	// Presentational hints: het width/height-attribuut van svg's en
	// ouderwetse tabellen is per spec een declaratie op de láágste plek in
	// de cascade — elke echte CSS-regel wint er dus van (ze staan hier vóór
	// de matched-lus). <img> houdt bewust zijn eigen attribuut-pad in
	// imgSize: daar hoort de beeldverhouding-regel bij (CSS height:auto
	// schakelt het attribuut uit — wikipedia's ei).
	eachEl(s.doc, func(n *html.Node) {
		switch n.Data {
		case "svg", "td", "th", "table":
			for _, k := range []string{"width", "height"} {
				if v, ok := attr(n, k); ok {
					if hv := hintLen(v); hv != "" {
						p := styles[n]
						if p == nil {
							p = props{}
							styles[n] = p
						}
						p[k] = hv
					}
				}
			}
		}
	})
	vars := map[string]string{} // custom properties, doc-globaal (versimpeld: geen scoping)
	for _, r := range s.matched {
		if !ruleMediaOK(r.mq, width) {
			continue
		}
		for _, n := range r.nodes {
			p := styles[n]
			if p == nil {
				p = props{}
				styles[n] = p
			}
			for k, v := range r.decls {
				p[k] = v
			}
		}
		// --vars van geldende regels (:root, body, body.pg-x) gelden
		// doc-globaal in cascade-volgorde — genoeg voor het gangbare
		// "thema op de body"-patroon.
		for k, v := range r.decls {
			if strings.HasPrefix(k, "--") {
				vars[k] = v
			}
		}
	}
	// var(--x) overal oplossen, ook in de vars zelf (--acc: var(--leaf)).
	for k, v := range vars {
		vars[k] = resolveVars(v, vars)
	}
	for _, p := range styles {
		for k, v := range p {
			if strings.Contains(v, "var(") {
				p[k] = resolveVars(v, vars)
			}
		}
	}
	// Kleuren op <html> gelden voor de pagina (html{background:...} is een
	// gangbaar canvas-patroon), maar de layout wandelt vanaf body — schuif
	// ze door naar body waar die ze zelf niet zet.
	if root := findEl(s.doc, "html"); root != nil {
		if hp := styles[root]; hp != nil {
			if body := findEl(s.doc, "body"); body != nil {
				bp := styles[body]
				if bp == nil {
					bp = props{}
					styles[body] = bp
				}
				for _, k := range []string{"color", "background-color", "background-image"} {
					if _, ok := bp[k]; !ok {
						if v, ok := hp[k]; ok {
							bp[k] = v
						}
					}
				}
			}
		}
	}
	s.styleCache, s.styleW = styles, width
	return styles
}

// expandImports vervangt @import-statements door de inhoud van de
// geïmporteerde sheet — zonder dit bestaan sheets die zo bundelen
// simpelweg niet en blijft de pagina half ongestyled. Een mediaconditie
// op de import wordt een omhullend @media-blok (dezelfde evaluatie als
// elke andere query), een supports(...)-conditie evalueert tegen
// supportedProp. depth is de cyclus-wacht (import-lussen), geen budget.
func (s *Session) expandImports(css string, base *url.URL, depth int) string {
	if depth >= 6 || !strings.Contains(css, "@import") {
		return css
	}
	css = stripComments(css)
	var out strings.Builder
	for i := 0; i < len(css); {
		j := strings.Index(css[i:], "@import")
		if j < 0 {
			out.WriteString(css[i:])
			break
		}
		j += i
		end := strings.IndexByte(css[j:], ';')
		if end < 0 {
			out.WriteString(css[i:])
			break
		}
		out.WriteString(css[i:j])
		stmt := css[j+len("@import") : j+end]
		i = j + end + 1
		ref, mq, ok := importTarget(stmt)
		if !ok || ref == "" || strings.HasPrefix(ref, "data:") || base == nil {
			continue
		}
		if mq != "" && !mediaAnyWidth(mq) {
			continue // print e.d.: kan op geen enkele breedte gelden
		}
		u, err := base.Parse(ref)
		if err != nil {
			continue
		}
		sub := s.expandImports(s.fetchText(u.String()), u, depth+1)
		if mq != "" {
			sub = "@media " + mq + "{" + sub + "}"
		}
		out.WriteString(sub)
	}
	return out.String()
}

// importTarget leest het doel uit een @import-statement: url(...) of een
// string, daarna optioneel layer(...)/layer en supports(...) — de rest is
// de mediaquery. ok=false als een supports-conditie faalt.
func importTarget(stmt string) (ref, mq string, ok bool) {
	stmt = strings.TrimSpace(stmt)
	switch {
	case strings.HasPrefix(strings.ToLower(stmt), "url("):
		end := closeParen(stmt, 3)
		if end < 0 {
			return "", "", false
		}
		ref = strings.Trim(strings.TrimSpace(stmt[4:end]), `"'`)
		stmt = stmt[end+1:]
	case len(stmt) > 1 && (stmt[0] == '"' || stmt[0] == '\''):
		j := strings.IndexByte(stmt[1:], stmt[0])
		if j < 0 {
			return "", "", false
		}
		ref = stmt[1 : 1+j]
		stmt = stmt[j+2:]
	default:
		return "", "", false
	}
	rest := strings.TrimSpace(stmt)
	for {
		low := strings.ToLower(rest)
		switch {
		case strings.HasPrefix(low, "layer("):
			end := closeParen(rest, len("layer(")-1)
			if end < 0 {
				return ref, "", true
			}
			rest = strings.TrimSpace(rest[end+1:])
		case strings.HasPrefix(low, "layer"):
			rest = strings.TrimSpace(rest[len("layer"):])
		case strings.HasPrefix(low, "supports("):
			end := closeParen(rest, len("supports(")-1)
			if end < 0 {
				return ref, "", true
			}
			if !supportsCond(rest[len("supports("):end]) {
				return "", "", false // conditie faalt: de import vervalt
			}
			rest = strings.TrimSpace(rest[end+1:])
		default:
			return ref, rest, true
		}
	}
}

// fetchText haalt één tekst-subresource (stylesheet) begrensd op; "" bij pech.
func (s *Session) fetchText(href string) string {
	data, _, err := s.fetchResource(href, cssMaxBytes)
	if err != nil {
		return ""
	}
	return string(data)
}
