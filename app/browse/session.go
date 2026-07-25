// Session is het browservenster: pagina's ophalen, parsen en klaarzetten
// voor de layout. De DOM is golang.org/x/net/html (de WHATWG-parser die
// heel Go-land gebruikt), selectors matcht cascadia — allebei puur Go, dus
// ook op tamago. Er zit bewust géén browserframework meer tussen: wij zijn
// zelf de browser, en scripting bestaat hier niet (statische pagina's +
// mechanisme-detectie, zie consentGateURL en de sr-hidden-regels).
package browse

import (
	"fmt"
	"image"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/html"
)

// welkomHTML is de startpagina (geen netwerk nodig): meteen beeld, en een
// mini-zelftest van de layout-engine.
const welkomHTML = `<html><head><title>surf</title></head><body>
<h1>surf browser</h1>
<p>Typ een adres in de balk hierboven en druk op Enter.
Scroll met het wiel, klik links om te volgen.</p>
<hr>
<p><b>x/net/html</b> parset de pagina's; deze layout-engine zet de DOM om in
pixels. Geen scripts &mdash; wel CSS, en vooral: <i>leesbaar</i>.</p>
<ul><li>blokken en woordwrap</li><li>koppen op schaal</li>
<li>links: <a href="about:blank">klikbaar</a></li>
<li><code>code</code> en <pre>  pre met  spaties</pre></li></ul>
</body></html>`

const userAgent = "surf/0.1 (HopOS)" // Wikipedia c.s. weigeren anonieme clients (403)

// pageMaxBytes begrenst één HTML-document over de lijn — zelfde gedachte
// als bij afbeeldingen en stylesheets: bare metal, begrensde heap.
const pageMaxBytes = 8 << 20

// Session is één browservenster.
type Session struct {
	client      *http.Client
	doc         *html.Node
	addr        *url.URL               // adres van de huidige pagina (na redirects)
	history     []string               // verlaten pagina's, oudste eerst — de terug-knop
	base        *url.URL               // addr + <base href>: anker voor relatieve links
	imgs        map[string]image.Image // gedecodeerde <img>'s van de huidige pagina, op raw src
	edits       map[*html.Node]string  // ingetikte veldwaarden (overleven een re-layout)
	icon        image.Image            // apple-touch-icon: vult het logo-slot als de site zelf svg/JS is
	controls    map[ControlID]*html.Node
	controlIDs  map[*html.Node]ControlID
	nextControl ControlID

	resourceMu   sync.Mutex
	resources    map[string]resourceData
	resourceWait map[string]chan struct{}

	// De cascade in twee stappen: matchen is duur en breedte-onafhankelijk
	// (één keer per pagina, in de nav-goroutine), de media-evaluatie is
	// goedkoop en gebeurt per layout-breedte — zo schakelt een resize
	// tussen de mobiele en de desktop-versie van de site.
	matched    []matchedRule
	styleCache map[*html.Node]props
	styleW     int
}

// matchedRule is één gematchte CSS-regel: zijn declaraties, de elementen
// die hij raakt en de media-condities waaronder hij geldt. De volgorde in
// Session.matched ís de cascade-volgorde (specificiteit, dan bron).
type matchedRule struct {
	mq    []string
	decls props
	nodes []*html.Node
}

// NewSession start een venster op de ingebouwde startpagina, met een eigen
// cookie-jar en het gedeelde TLS-transport erachter.
func NewSession() *Session { return newSession(newNetClient()) }

// NewSessionHandler is NewSession met een in-process http.Handler in plaats
// van het echte netwerk — voor de host-tests, zonder poorten of sockets.
func NewSessionHandler(h http.Handler) *Session {
	return newSession(&http.Client{Transport: handlerTransport{h}, Jar: newJar(), Timeout: 20 * time.Second})
}

func newSession(c *http.Client) *Session {
	s := &Session{client: c}
	s.doc, _ = html.Parse(strings.NewReader(welkomHTML))
	s.addr, _ = url.Parse("about:blank")
	s.base = s.addr
	return s
}

// --- navigatie ---------------------------------------------------------------

// Go navigeert naar een adresbalk-invoer; een kaal adres ("hop.local",
// "10.0.0.7:8080/status") krijgt http:// ervoor. Bij een fout blijft de
// huidige pagina staan.
func (s *Session) Go(addr string) error {
	if addr == "" {
		return nil
	}
	if !hasScheme(addr) {
		addr = "http://" + addr
	}
	return s.navigate(addr)
}

// Follow navigeert naar een aangeklikte href; relatieve paden ("/x",
// "page2.html", "#anker") resolven tegen de huidige pagina (incl. <base>).
func (s *Session) Follow(href string) error {
	return s.navigate(href)
}

// Back navigeert naar de laatst verlaten pagina (de terug-knop). Een
// mislukte terug-hop laat de historie intact; zonder historie is het een
// nette fout voor de statusbalk.
func (s *Session) Back() error {
	if len(s.history) == 0 {
		return fmt.Errorf("geen vorige pagina")
	}
	prev := s.history[len(s.history)-1]
	if err := s.navigate(prev); err != nil {
		return err
	}
	// navigate pushte de zojuist verlaten pagina: die én de bestemming
	// zelf van de stapel — terug is geen vooruit.
	s.history = s.history[:max(len(s.history)-2, 0)]
	return nil
}

// navigate is de gedeelde landing van Go en Follow: resolven, laden, een
// eventuele consent-muur één keer door, en dan de subresources laden.
func (s *Session) navigate(ref string) error {
	prev := s.URL() // voor de historie: de pagina die we (mogelijk) verlaten
	u, err := s.resolve(ref)
	if err != nil {
		return err
	}
	// Alleen-een-anker: geen nieuwe pagina — laat de huidige staan.
	if u.Fragment != "" && sameDoc(u, s.addr) {
		return nil
	}
	doc, final, err := s.load(u)
	if err != nil {
		return err
	}
	// Consent-muur (DPG's privacy-gate: tweakers.net, nu.nl, ...)? Zonder
	// scripts is die pagina letterlijk leeg — de door-URL staat er wel
	// gewoon in. Die éne hop zet het consent-cookie (jar!); daarna opnieuw
	// naar het gevraagde adres, zodat de adresbalk niet de redirect-junk
	// (?referrer=...) toont. Faalt de hop, dan blijft de muur staan en zie
	// je tenminste wáár je bent. Eén hop, dus nooit een lus.
	if gate := consentGateURL(doc); gate != "" {
		if gu, perr := url.Parse(gate); perr == nil {
			if _, _, gerr := s.load(gu); gerr == nil {
				if doc2, final2, err2 := s.load(u); err2 == nil {
					doc, final = doc2, final2
				}
			}
		}
	}
	// DPG's redirects plakken een ?referrer=... aan het adres (ook zónder
	// gate, als het consent-cookie er al zit): analytics-junk, geen inhoud
	// — niet in de balk en niet als base voor links.
	if q := final.Query(); q.Has("referrer") {
		q.Del("referrer")
		clean := *final
		clean.RawQuery = q.Encode()
		final = &clean
	}
	s.doc, s.addr = doc, final
	// Historie voor de terug-knop: de pagina die we zojuist verlieten
	// (herladen van hetzelfde adres telt niet, de lege start ook niet).
	if prev != "" && prev != "about:blank" && prev != final.String() {
		s.history = append(s.history, prev)
		if len(s.history) > 32 {
			s.history = s.history[1:]
		}
	}
	s.base = pageBase(doc, final)
	s.resetResourceCache()
	s.edits = nil    // nieuwe pagina, verse velden
	s.controls = nil // Page-controls horen bij deze DOM
	s.controlIDs = nil
	s.nextControl = 0
	s.resolveUses() // svg <use> → symbolen inlijmen (sprite-sheets ophalen)
	s.loadStyles()
	s.loadImages()
	s.loadIcon()
	return nil
}

// resolve maakt van een adres of href een absolute http(s)-URL. Spaties
// ín het pad ("waarom 1.webp" — easyflorist) encoderen we zoals elke
// browser dat doet; url.Parse zou er anders op stuklopen.
func (s *Session) resolve(ref string) (*url.URL, error) {
	r, err := url.Parse(strings.ReplaceAll(strings.TrimSpace(ref), " ", "%20"))
	if err != nil {
		return nil, err
	}
	if s.base != nil {
		r = s.base.ResolveReference(r)
	}
	if r.Scheme != "http" && r.Scheme != "https" {
		return nil, fmt.Errorf("geen webadres: %s", ref)
	}
	return r, nil
}

func sameDoc(a, b *url.URL) bool {
	if a == nil || b == nil {
		return false
	}
	ac, bc := *a, *b
	ac.Fragment, bc.Fragment = "", ""
	return ac.String() == bc.String()
}

// pageBase: de basis voor relatieve links — de paginalocatie, tenzij een
// <base href> anders zegt.
func pageBase(doc *html.Node, final *url.URL) *url.URL {
	if b := findEl(doc, "base"); b != nil {
		if href, ok := attr(b, "href"); ok && strings.TrimSpace(href) != "" {
			if r, err := url.Parse(strings.TrimSpace(href)); err == nil {
				return final.ResolveReference(r)
			}
		}
	}
	return final
}

// consentGateURL herkent een consent-muur en geeft de "klik hier verder"-URL
// terug ("" als de pagina geen muur is). Het patroon (DPG Media, het halve
// NL-web): een <script> met decodeURIComponent('https%3A%2F%2F...') waarin
// een privacy-bevestigings-URL met authId zit; die URL GET'en — met de
// cookie-jar — telt als de minimale (functionele-cookies) doorklik.
func consentGateURL(doc *html.Node) string {
	const marker = "decodeURIComponent('"
	found := ""
	var walk func(n *html.Node)
	walk = func(n *html.Node) {
		if found != "" {
			return
		}
		if n.Type == html.ElementNode && n.Data == "script" {
			txt := textContent(n)
			for i := 0; ; {
				j := strings.Index(txt[i:], marker)
				if j < 0 {
					break
				}
				start := i + j + len(marker)
				end := strings.IndexByte(txt[start:], '\'')
				if end < 0 {
					break
				}
				u, err := url.QueryUnescape(txt[start : start+end])
				if err == nil && hasScheme(u) && strings.Contains(u, "authId=") &&
					strings.Contains(strings.ToLower(u), "privacy") {
					found = u
					return
				}
				i = start + end
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	if doc != nil {
		walk(doc)
	}
	return found
}

// --- formulieren ------------------------------------------------------------

// Type verwerkt een toets in een invoerveld: een teken erbij, of met bs
// een teken eraf. De waarde leeft in de sessie en overleeft re-layouts.
func (s *Session) Type(f *Field, ch byte, bs bool) {
	if f == nil {
		return
	}
	node := s.controls[f.ID]
	if node == nil {
		return
	}
	if s.edits == nil {
		s.edits = map[*html.Node]string{}
	}
	v, ok := s.edits[node]
	if !ok {
		v = f.Value
	}
	if bs {
		if v != "" {
			v = v[:len(v)-1]
		}
	} else {
		v += string(ch)
	}
	s.edits[node] = v
	f.Value = v
}

// Submit verstuurt het formulier waar dit veld in zit: alle benoemde
// velden als GET-query op de action-URL (POST is een andere klus — de
// zoekmachines van deze wereld zijn GET). De aangeklikte submit-knop doet
// zijn eigen naam mee, die van de andere knoppen niet.
func (s *Session) Submit(f *Field) error {
	if f == nil {
		return nil
	}
	node := s.controls[f.ID]
	if node == nil {
		return nil
	}
	form := ancestorForm(node)
	if form == nil {
		return nil
	}
	if m, _ := attr(form, "method"); strings.EqualFold(strings.TrimSpace(m), "post") {
		return fmt.Errorf("POST-formulier: nog niet gedragen")
	}
	q := url.Values{}
	var collect func(n *html.Node)
	collect = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "input" {
			name, _ := attr(n, "name")
			typ, _ := attr(n, "type")
			typ = strings.ToLower(typ)
			val, _ := attr(n, "value")
			if v, ok := s.edits[n]; ok {
				val = v
			}
			switch {
			case name == "":
			case typ == "submit" || typ == "button" || typ == "image":
				if n == node {
					q.Set(name, val)
				}
			case typ == "checkbox" || typ == "radio":
				if _, checked := attr(n, "checked"); checked {
					q.Set(name, val)
				}
			default: // text, search, hidden, ...
				q.Set(name, val)
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			collect(c)
		}
	}
	for c := form.FirstChild; c != nil; c = c.NextSibling {
		collect(c)
	}
	action, _ := attr(form, "action")
	if action == "" {
		action = s.URL()
	}
	if i := strings.IndexByte(action, '?'); i >= 0 {
		action = action[:i]
	}
	return s.Follow(action + "?" + q.Encode())
}

// ancestorForm zoekt het omvattende <form>-element.
func ancestorForm(n *html.Node) *html.Node {
	for p := n.Parent; p != nil; p = p.Parent {
		if p.Type == html.ElementNode && p.Data == "form" {
			return p
		}
	}
	return nil
}

// --- API voor main ------------------------------------------------------------

// URL is het adres van de huidige pagina (voor de adresbalk na navigatie).
func (s *Session) URL() string {
	if s.addr == nil {
		return ""
	}
	return s.addr.String()
}

// Layout layout de huidige pagina voor deze breedte, inclusief de bij de
// navigatie opgehaalde afbeeldingen, CSS-props en ingetikte veldwaarden.
func (s *Session) Layout(width int) Page {
	return s.LayoutViewport(Viewport{Width: width})
}

// LayoutViewport layout de huidige pagina tegen een expliciet content-
// venster. Drive gebruikt dit pad zodat vh/fixed per browserinstantie en
// tegen de ruimte tussen adres- en statusbalk rekenen.
func (s *Session) LayoutViewport(viewport Viewport) Page {
	viewport = viewport.normalized()
	return layoutStyled(
		findEl(s.doc, "body"), viewport, s.imgs, s.stylesFor(viewport.Width),
		s.edits, s.icon, s.bindControl,
	)
}

func (s *Session) bindControl(n *html.Node) ControlID {
	if n == nil {
		return 0
	}
	if id := s.controlIDs[n]; id != 0 {
		return id
	}
	if s.controlIDs == nil {
		s.controlIDs = map[*html.Node]ControlID{}
		s.controls = map[ControlID]*html.Node{}
	}
	s.nextControl++
	id := s.nextControl
	s.controlIDs[n] = id
	s.controls[id] = n
	return id
}

// hasScheme: "letters://" aan het begin. "host:7878" is géén scheme.
func hasScheme(addr string) bool {
	for i := 0; i < len(addr); i++ {
		c := addr[i]
		switch {
		case c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z':
			continue
		case c == ':':
			return i > 0 && len(addr) > i+2 && addr[i+1] == '/' && addr[i+2] == '/'
		default:
			return false
		}
	}
	return false
}
