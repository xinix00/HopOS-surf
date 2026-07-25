package browse

import (
	"strconv"
	"strings"
)

// cssRule is één selector met zijn declaraties, klaar om te matchen. mq
// zijn de omhullende @media-condities — die worden pas bij het cascaden
// geëvalueerd, tegen de échte framebreedte (mobile óf desktop).
type cssRule struct {
	sel   string
	spec  int // versimpelde specificiteit: id·100 + class/attr/pseudo·10 + tag
	seq   int // bronvolgorde (tiebreaker: later wint)
	decls props
	mq    []string
}

// parseCSS vouwt een stylesheet uit tot regels. Tolerant: commentaar en
// onbekende @-blokken (met hun hele inhoud) verdwijnen, kapotte regels ook.
// Selector-groepen ("h1, h2") splitsen in losse regels met eigen
// specificiteit.
func parseCSS(src string, seq0 int) []cssRule { return parseCSSm(src, seq0, nil) }

// parseCSSm is parseCSS met omhullende media-condities (het media=""-
// attribuut van de sheet, en verderop geneste @media-blokken).
func parseCSSm(src string, seq0 int, mq []string) []cssRule {
	src = stripComments(src)
	var rules []cssRule
	for i := 0; i < len(src); {
		open := strings.IndexByte(src[i:], '{')
		if open < 0 {
			break
		}
		sel := strings.TrimSpace(src[i : i+open])
		body, next := block(src, i+open)
		i = next
		if sel == "" {
			continue
		}
		if sel[0] == '@' {
			// @media: de query reist mee met de geneste regels; welke tak
			// geldt beslist de framebreedte bij het cascaden. Queries die
			// op géén enkele breedte kunnen matchen (print, prefers-light)
			// vallen hier al af. Andere @-blokken blijven genegeerd.
			if strings.HasPrefix(sel, "@media") {
				if q := sel[len("@media"):]; mediaAnyWidth(q) {
					sub := append(append([]string{}, mq...), q)
					rules = append(rules, parseCSSm(body, seq0+len(rules), sub)...)
				}
			}
			// @supports: half het web wikkelt zijn grid/flex-layout hierin —
			// die blokken overslaan verliest regels voor dingen die we WÉL
			// kunnen. De conditie evalueert tegen supportedProp: dezelfde
			// waarheid als de regel-filter, geen tweede lijst.
			if strings.HasPrefix(sel, "@supports") {
				if supportsCond(sel[len("@supports"):]) {
					rules = append(rules, parseCSSm(body, seq0+len(rules), mq)...)
				}
			}
			continue
		}
		decls := parseDecls(body)
		if len(decls) == 0 {
			continue
		}
		for _, one := range strings.Split(sel, ",") {
			one = simplifySelector(strings.TrimSpace(one))
			if one == "" || deadSelector(one) {
				continue
			}
			rules = append(rules, cssRule{
				sel: one, spec: specificity(one), seq: seq0 + len(rules), decls: decls, mq: mq,
			})
		}
	}
	return rules
}

// supportsCond evalueert een @supports-conditie: (prop: waarde) is waar
// als wij de property dragen — supportedProp is de enige waarheid, er
// komt geen tweede lijst. and/or/not en geneste haakjes zoals de spec ze
// schrijft; onbekende vormen (selector(...), font-format(...)) zijn niet
// waar — net als in een browser die ze niet kent.
func supportsCond(c string) bool {
	c = strings.TrimSpace(c)
	if parts := splitCond(c, " or "); len(parts) > 1 {
		for _, p := range parts {
			if supportsCond(p) {
				return true
			}
		}
		return false
	}
	if parts := splitCond(c, " and "); len(parts) > 1 {
		for _, p := range parts {
			if !supportsCond(p) {
				return false
			}
		}
		return true
	}
	if strings.HasPrefix(strings.ToLower(c), "not") {
		return !supportsCond(c[len("not"):])
	}
	if len(c) > 1 && c[0] == '(' && closeParen(c, 0) == len(c)-1 {
		inner := strings.TrimSpace(c[1 : len(c)-1])
		if i := strings.IndexByte(inner, ':'); i >= 0 && !strings.ContainsAny(inner[:i], "()") {
			return supportedProp(strings.ToLower(strings.TrimSpace(inner[:i])))
		}
		return supportsCond(inner) // geneste conditie: ((a) or (b))
	}
	return false
}

// splitCond splitst op een keyword (" or ", " and ") van het buitenste
// niveau; haakjes blijven heel. Lengte 1 = geen splitsing.
func splitCond(s, kw string) []string {
	var out []string
	depth, start := 0, 0
	low := strings.ToLower(s)
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '(':
			depth++
		case ')':
			depth--
		}
		if depth == 0 && strings.HasPrefix(low[i:], kw) {
			out = append(out, strings.TrimSpace(s[start:i]))
			start = i + len(kw)
			i += len(kw) - 1
		}
	}
	return append(out, strings.TrimSpace(s[start:]))
}

// mediaProbeWidths: de breedtes waarop we proeven of een query überhaupt
// kán matchen — van telefoon tot breed scherm, plus onze default.
var mediaProbeWidths = []int{320, mobileWidth, 640, 800, 1024, 1280, 1680}

// mediaAnyWidth: kan deze query op énige redelijke framebreedte matchen?
func mediaAnyWidth(q string) bool {
	for _, w := range mediaProbeWidths {
		if mediaMatches(q, w) {
			return true
		}
	}
	return false
}

// ruleMediaOK: gelden alle omhullende media-condities op deze breedte?
func ruleMediaOK(mq []string, w int) bool {
	for _, q := range mq {
		if !mediaMatches(q, w) {
			return false
		}
	}
	return true
}

// mobileWidth is de viewport waar @media-queries tegen geëvalueerd worden.
// De styles worden bij het laden berekend (de windowbreedte is dan nog niet
// bekend) en het venster is 480 breed — wij zíjn gewoon een telefoon.
const mobileWidth = 480

// mediaMatches evalueert een @media-query tegen breedte w. Bewust simpel:
// komma's zijn OR, "and" is AND; gedragen zijn (min-width), (max-width),
// de range-vorm (width <= ...) en de types screen/all. Onbekende features
// en "not" matchen niet — liever een regel te weinig dan desktop-CSS op
// een telefoonvenster.
func mediaMatches(q string, w int) bool {
	for _, branch := range strings.Split(strings.ToLower(q), ",") {
		if mediaBranch(strings.TrimSpace(branch), w) {
			return true
		}
	}
	return false
}

func mediaBranch(b string, w int) bool {
	if b == "" || strings.HasPrefix(b, "not ") || strings.Contains(b, " not ") {
		return false
	}
	for _, part := range strings.Split(b, " and ") {
		part = strings.TrimSpace(part)
		switch part {
		case "screen", "all", "only screen", "only all":
			continue
		}
		if !mediaCond(part, w) {
			return false
		}
	}
	return true
}

// mediaCond: één (feature)-conditie. Zowel de klassieke vorm
// (min-width: 768px) als de range-vorm (480px <= width < 64em).
func mediaCond(c string, w int) bool {
	c = strings.TrimSpace(c)
	c = strings.TrimPrefix(c, "(")
	c = strings.TrimSuffix(c, ")")
	if i := strings.IndexByte(c, ':'); i >= 0 {
		prop, val := strings.TrimSpace(c[:i]), strings.TrimSpace(c[i+1:])
		// Wij zijn een lichte lezer (papierwit canvas): dark-mode-CSS hoort
		// niet te matchen — anders bloedt een donker thema half een lichte
		// pagina in (nu.nl's headerchips). En bewegen doen we niet.
		switch prop {
		case "prefers-color-scheme":
			return val == "light"
		case "prefers-reduced-motion":
			return val == "reduce"
		}
		v, ok := cssLen(defaultCSSContext(), val)
		if !ok {
			return false
		}
		switch prop {
		case "min-width":
			return w >= v
		case "max-width":
			return w <= v
		}
		return false
	}
	c = strings.ReplaceAll(c, " ", "")
	i := strings.Index(c, "width")
	if i < 0 {
		return false
	}
	left, right := c[:i], c[i+len("width"):]
	if lv, op, ok := splitCmp(left, true); ok {
		if !cmpWidth(w, flip(op), lv) {
			return false
		}
	} else if left != "" {
		return false
	}
	if rv, op, ok := splitCmp(right, false); ok {
		if !cmpWidth(w, op, rv) {
			return false
		}
	} else if right != "" {
		return false
	}
	return left != "" || right != ""
}

// splitCmp haalt operator en lengte uit "63em<=" (links van width) of
// "<=63em" (rechts van width).
func splitCmp(s string, leftSide bool) (px int, op string, ok bool) {
	for _, o := range []string{"<=", ">=", "<", ">", "="} {
		if leftSide && strings.HasSuffix(s, o) {
			if v, ok := cssLen(defaultCSSContext(), strings.TrimSuffix(s, o)); ok {
				return v, o, true
			}
			return 0, "", false
		}
		if !leftSide && strings.HasPrefix(s, o) {
			if v, ok := cssLen(defaultCSSContext(), strings.TrimPrefix(s, o)); ok {
				return v, o, true
			}
			return 0, "", false
		}
	}
	return 0, "", false
}

// flip spiegelt een operator: "63em <= width" is "width >= 63em".
func flip(op string) string {
	switch op {
	case "<=":
		return ">="
	case ">=":
		return "<="
	case "<":
		return ">"
	case ">":
		return "<"
	}
	return op
}

func cmpWidth(w int, op string, v int) bool {
	switch op {
	case "<=":
		return w <= v
	case ">=":
		return w >= v
	case "<":
		return w < v
	case ">":
		return w > v
	case "=":
		return w == v
	}
	return false
}

// rootFontPx: de html-font-size naar pixels — %, em en rem zijn hier van
// de browserdefault 16 (62.5% = 10px).
func rootFontPx(v string) float64 {
	v = strings.TrimSpace(v)
	mul := 1.0
	switch {
	case strings.HasSuffix(v, "%"):
		v, mul = strings.TrimSuffix(v, "%"), 0.16
	case strings.HasSuffix(v, "rem"):
		v, mul = strings.TrimSuffix(v, "rem"), 16
	case strings.HasSuffix(v, "em"):
		v, mul = strings.TrimSuffix(v, "em"), 16
	case strings.HasSuffix(v, "px"):
		v = strings.TrimSuffix(v, "px")
	default:
		return 16
	}
	if f, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil && f*mul >= 4 && f*mul <= 32 {
		return f * mul
	}
	return 16
}

// cssLen: een CSS-lengte naar hele pixels (px; em/rem op de wortelbasis;
// vw/vh tegen het venster — layoutbreedte en vensterhoogte).
func cssLen(cx cssContext, v string) (int, bool) {
	v = strings.TrimSpace(v)
	mul := 1.0
	switch {
	case strings.HasSuffix(v, "rem"):
		v, mul = strings.TrimSuffix(v, "rem"), cx.remPx
	case strings.HasSuffix(v, "em"):
		v, mul = strings.TrimSuffix(v, "em"), cx.remPx
	case strings.HasSuffix(v, "px"):
		v = strings.TrimSuffix(v, "px")
	case strings.HasSuffix(v, "vw"):
		// dvw/svw/lvw (de dynamische viewport-varianten) zijn bij ons
		// hetzelfde venster — er beweegt geen adresbalk.
		for _, sfx := range []string{"dvw", "svw", "lvw", "vw"} {
			if strings.HasSuffix(v, sfx) {
				v, mul = strings.TrimSuffix(v, sfx), float64(cx.viewW)/100
				break
			}
		}
	case strings.HasSuffix(v, "vh"):
		for _, sfx := range []string{"dvh", "svh", "lvh", "vh"} {
			if strings.HasSuffix(v, sfx) {
				v, mul = strings.TrimSuffix(v, sfx), float64(cx.viewH)/100
				break
			}
		}
	case v == "0":
	default:
		return 0, false
	}
	f, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
	if err != nil {
		return 0, false
	}
	return int(f * mul), true
}

// simplifySelector vouwt pseudo's weg die bij ons statisch vaststaan:
// :not(:hover), :not(:focus) enz. zijn áltijd waar (er is geen muis of
// focus — de hele :not vervalt), en :is(X)/:where(X) met één argument
// wordt herschreven naar X-zonder-:is. Belangrijk voor de verberg-regels
// van skip-links (".skip:not(:focus)") en voor selector-engines die
// :is() niet kennen — tweakers' hele component-CSS is
// ".more:is(:is(twk-site-menu>menu)>li)>.dropdown-menu"-taal.
func simplifySelector(sel string) string {
	for pass := 0; pass < 8; pass++ {
		changed := false
		for _, fn := range []string{":not(", ":is(", ":where("} {
			for from := 0; ; {
				i := strings.Index(sel[from:], fn)
				if i < 0 {
					break
				}
				i += from
				end := closeParen(sel, i+len(fn)-1)
				if end < 0 {
					break
				}
				inner := sel[i+len(fn) : end]
				switch {
				case strings.Contains(inner, ","):
					from = i + 1 // meerdere argumenten: laten staan
				case fn == ":not(" && deadSelector(inner):
					sel = sel[:i] + sel[end+1:] // :not(nooit-waar) = altijd waar
					changed = true
					from = i
				case fn != ":not(":
					// C1:is(A > B) betekent: matcht C1 én A > B — het laatste
					// compound van de binnenkant versmelt dus met het compound
					// eromheen, en A> komt ervóór (type-selector voorop).
					if folded, ok := foldIs(sel, i, end, inner); ok {
						sel = folded
						changed = true
						from = i
					} else {
						from = i + 1
					}
				default:
					from = i + 1 // :not(.iets-echts): laten staan
				}
			}
		}
		if !changed {
			break
		}
	}
	return strings.TrimSpace(sel)
}

// foldIs herschrijft één :is(inner)/:where(inner) op sel[i:end+1] naar een
// :is-loze vorm. pre is het compound-deel vóór de :is (".more"), anc het
// voorouder-deel van de binnenkant ("twk-site-menu>menu>"), last diens
// laatste compound ("li") — samen: anc + last×pre.
func foldIs(sel string, i, end int, inner string) (string, bool) {
	// het compound waar de :is in staat begint ná de vorige combinator —
	// haakjes-bewust terug, en een :is bínnen andermans haakjes laten we
	// met rust (dat compound is niet los te herschrijven).
	cs, depth := i, 0
	for cs > 0 {
		b := sel[cs-1]
		if b == ')' || b == ']' {
			depth++
		} else if b == '(' || b == '[' {
			depth--
			if depth < 0 {
				return sel, false
			}
		}
		if depth == 0 && isCombByte(b) {
			break
		}
		cs--
	}
	pre := sel[cs:i]
	anc, last := "", inner
	if k := lastTopCombinator(inner); k >= 0 {
		anc, last = inner[:k+1], strings.TrimSpace(inner[k+1:])
	}
	merged, ok := mergeCompound(last, pre)
	if !ok {
		return sel, false
	}
	return sel[:cs] + anc + merged + sel[end+1:], true
}

func isCombByte(b byte) bool { return b == ' ' || b == '>' || b == '+' || b == '~' }

// lastTopCombinator: de index van de laatste combinator op het buitenste
// niveau (haakjes en attribuut-blokken tellen niet mee); -1 = compound.
func lastTopCombinator(s string) int {
	depth, last := 0, -1
	for j := 0; j < len(s); j++ {
		switch s[j] {
		case '(', '[':
			depth++
		case ')', ']':
			depth--
		case ' ', '>', '+', '~':
			if depth == 0 {
				last = j
			}
		}
	}
	return last
}

// mergeCompound voegt twee compounds samen tot één, met de type-selector
// voorop ("li" + ".more" = "li.more"). Twee type-selectors tegelijk kan
// niet — dan laten we de :is staan (regel vervalt bij het parsen).
func mergeCompound(a, b string) (string, bool) {
	if a == "" {
		return b, true
	}
	if b == "" {
		return a, true
	}
	aType := a[0] != '.' && a[0] != '#' && a[0] != ':' && a[0] != '['
	bType := b[0] != '.' && b[0] != '#' && b[0] != ':' && b[0] != '['
	if aType && bType {
		return "", false
	}
	if bType {
		return b + a, true
	}
	return a + b, true
}

// closeParen geeft de index van de ')' die de '(' op sel[open] sluit.
func closeParen(sel string, open int) int {
	depth := 0
	for j := open; j < len(sel); j++ {
		switch sel[j] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return j
			}
		}
	}
	return -1
}

// deadSelector: selectors die bij ons per definitie nooit matchen — geen
// muis-hover, geen focusringen, geen gegenereerde ::before-content, geen
// vendor-pseudo's. Echte stylesheets bestaan hier voor een flink deel uit;
// eruit gooien bij het parsen scheelt evenzoveel QuerySelectorAll-rondes.
var deadPseudos = []string{
	":hover", ":focus", ":active", ":visited", ":target", ":checked",
	":disabled", ":enabled", ":before", ":after", ":placeholder",
	":selection", ":backdrop", ":fullscreen", ":-", "::-",
}

func deadSelector(sel string) bool {
	if !strings.ContainsRune(sel, ':') {
		return false // verreweg de meeste selectors: geen pseudo, klaar
	}
	for _, p := range deadPseudos {
		if strings.Contains(sel, p) {
			return true
		}
	}
	return false
}

// block geeft de inhoud tussen de accolade op src[open] en zijn sluiter
// (genest meegeteld), plus de index erna.
func block(src string, open int) (string, int) {
	depth := 0
	for j := open; j < len(src); j++ {
		switch src[j] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return src[open+1 : j], j + 1
			}
		}
	}
	return src[open+1:], len(src)
}
