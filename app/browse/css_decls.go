package browse

import (
	"image/color"
	"strconv"
	"strings"
)

// srProp is de synthetische property waarmee parseDecls "visueel verborgen
// screenreader-tekst" markeert. Het sr-only-patroon is bewust géén
// display:none (dan zwijgt de screenreader óók): het is 1x1px met clip, of
// absoluut buiten beeld geparkeerd. Wij dragen clip/width/top niet als
// layout, maar herkennen de signatuur bij het parsen — de losse properties
// worden niet bewaard, dus de regel-filter blijft even streng.
const srProp = "-surf-sr-hidden"

// parseDecls parset "prop: waarde; prop: waarde" — alleen de gedragen
// properties blijven over. "background: <kleur>" telt als
// background-color als de waarde een kleur is (de gangbare shorthand).
func parseDecls(s string) props {
	var p props
	var clip, clipPath, w, h, pos, top, left, right, bottom, ti, op string
	var ovf, maxh, xform string
	for _, d := range strings.Split(s, ";") {
		colon := strings.IndexByte(d, ':')
		if colon < 0 {
			continue
		}
		prop := strings.ToLower(strings.TrimSpace(d[:colon]))
		val := strings.ToLower(strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(d[colon+1:]), "!important")))
		if val == "" {
			continue
		}
		switch prop {
		case "clip":
			clip = val
		case "clip-path":
			clipPath = val
		case "width":
			w = val
		case "height":
			h = val
		case "position":
			pos = val
		case "top":
			top = val
		case "left":
			left = val
		case "right":
			right = val
		case "bottom":
			bottom = val
		case "inset":
			// shorthand: 1-4 waarden in CSS-volgorde boven-rechts-onder-links
			if e, ok := expand4(strings.Fields(val)); ok {
				top, right, bottom, left = e[0], e[1], e[2], e[3]
			}
		case "text-indent":
			ti = val
		case "opacity":
			op = val
		case "overflow", "overflow-y":
			if strings.Contains(val, "hidden") || val == "clip" {
				ovf = "hidden"
			}
		case "max-height":
			maxh = val
		case "transform":
			// Off-canvas: een (vrijwel) volledige negatieve translate is de
			// dichte staat van lades en drawers — JS schuift ze pas in
			// beeld. De -50%-centreertruc blijft er expliciet buiten.
			if offCanvas(val) {
				xform = "weg"
			}
		}
		if !supportedProp(prop) {
			continue
		}
		if p == nil {
			p = props{}
		}
		// Een verloop rendert als zijn eerste kleurstop — vlak, maar de
		// juiste kleurfamilie (hero's en headers met een gradient).
		if (prop == "background" || prop == "background-image") && strings.Contains(val, "gradient(") {
			if c := firstColorIn(val[strings.Index(val, "gradient("):]); c != "" {
				p["background-color"] = c
			}
			if prop == "background-image" {
				// Een échte url naast het verloop (wikipedia's
				// "linear-gradient(transparent,transparent), url(sprite.svg)"
				// — de oude svg-fallback-truc): de afbeelding wint.
				if u := cssURL(val); u != "" {
					p["background-image"] = "url(" + u + ")"
				}
				continue
			}
		}
		if prop == "background" {
			// Shorthand uit elkaar trekken: een kleur-token wordt
			// background-color, een url(...) wordt background-image.
			for _, tok := range strings.Fields(val) {
				if _, ok := cssColor(tok); ok {
					p["background-color"] = tok
				}
			}
			if u := cssURL(val); u != "" {
				p["background-image"] = "url(" + u + ")"
			}
			// "background: url(x) center/cover": de maat zit in de shorthand.
			if strings.Contains(val, "cover") {
				p["background-size"] = "cover"
			} else if strings.Contains(val, "contain") {
				p["background-size"] = "contain"
			}
			// var(--x) als hele waarde: bewaren; na var-resolutie wordt het
			// alsnog een kleur (of valt het stil weg).
			if strings.HasPrefix(val, "var(") {
				p["background-color"] = val
			}
			continue
		}
		// flex-flow: de shorthand voor flex-direction + flex-wrap.
		if prop == "flex-flow" {
			for _, tok := range strings.Fields(val) {
				switch tok {
				case "row", "row-reverse", "column", "column-reverse":
					p["flex-direction"] = tok
				case "wrap", "nowrap", "wrap-reverse":
					p["flex-wrap"] = tok
				}
			}
			continue
		}
		// place-items/place-content: de as-shorthands — wij dragen er de
		// align-items- respectievelijk justify-content-kant van.
		if prop == "place-items" {
			p["align-items"] = strings.Fields(val)[0]
			continue
		}
		if prop == "place-content" {
			f := strings.Fields(val)
			p["justify-content"] = f[len(f)-1]
			continue
		}
		// De logische assen (ltr): -inline = links+rechts, -block =
		// boven+onder — modern web schrijft marges bijna alleen nog zo
		// (tweakers' margin-inline:auto centreert zijn menubaan).
		if strings.HasSuffix(prop, "-inline") || strings.HasSuffix(prop, "-block") {
			if base := strings.TrimSuffix(strings.TrimSuffix(prop, "-inline"), "-block"); base == "margin" || base == "padding" {
				f := splitTopLevel(val)
				if len(f) == 1 {
					f = append(f, f[0])
				}
				if len(f) == 2 {
					kanten := [2]string{"-left", "-right"}
					if strings.HasSuffix(prop, "-block") {
						kanten = [2]string{"-top", "-bottom"}
					}
					p[base+kanten[0]] = f[0]
					p[base+kanten[1]] = f[1]
				}
				continue
			}
		}
		// margin/padding: de shorthand schrijft óók zijn vier longhands —
		// dan wint in de cascade gewoon de láátste declaratie, welke vorm
		// die ook had (een margin:0 reset een eerdere margin-left echt).
		if prop == "margin" || prop == "padding" {
			if e, ok := expand4(splitTopLevel(val)); ok {
				for i, kant := range []string{"-top", "-right", "-bottom", "-left"} {
					p[prop+kant] = e[i]
				}
			}
			continue
		}
		p[prop] = val
	}
	if xform == "weg" || srHidden(clip, clipPath, w, h, pos, top, left, ti, op) {
		if p == nil {
			p = props{}
		}
		p[srProp] = "1"
	}
	// Dichtgeklapt: overflow:hidden op (max-)hoogte ~0 is de JS-loze dichte
	// staat van accordeons en uitklapmenu's — die inhoud is er niet. De
	// aspect-ratio-hack (height:0 mét padding-%) is juist een fotolijst en
	// blijft staan.
	if ovf == "hidden" {
		pv := func(k string) string {
			if p == nil {
				return ""
			}
			return p[k]
		}
		if !strings.Contains(pv("padding")+pv("padding-top")+pv("padding-bottom"), "%") {
			for _, hv := range []string{h, maxh} {
				if hv == "" {
					continue
				}
				if n, ok := cssLen(defaultCSSContext(), hv); ok && n <= 2 {
					if p == nil {
						p = props{}
					}
					p[srProp] = "1"
					break
				}
			}
		}
	}
	// Het positionerings-kado: fixed/sticky is chrome (header pinnen,
	// cookiebar weg), absolute gaat uit de flow op zijn coördinaten,
	// relative markeert de containing block.
	switch pos {
	case "fixed", "sticky", "absolute", "relative":
		if p == nil {
			p = props{}
		}
		p["position"] = pos
	}
	// De ankers reizen áltijd mee, ook zónder position in dezelfde regel:
	// wikipedia zet position:absolute en top/right in verschillende regels
	// — de cascade voegt ze pas samen. Zonder position blijven ze inert.
	for k, v := range map[string]string{"top": top, "bottom": bottom, "left": left, "right": right} {
		if v != "" {
			if p == nil {
				p = props{}
			}
			p[k] = v
		}
	}
	return p
}

// cssLenSignedPct: een (anker)lengte die negatief én een percentage van
// base mag zijn — wikipedia's talencirkel hangt op right:60%/left:60%.
func cssLenSignedPct(cx cssContext, v string, base int) (int, bool) {
	v = strings.TrimSpace(v)
	if strings.HasPrefix(v, "-") {
		if n, ok := cssLenPct(cx, v[1:], base); ok {
			return -n, true
		}
		return 0, false
	}
	return cssLenPct(cx, v, base)
}

// cssMinExtent: de gedeclareerde hoogte van een blok (height of
// min-height, in px; cap tegen 100vh-junk). Voor position:fixed is de
// víewport de containing block: procenten — en dus tweakers'
// calc(100% - var(--site-menu-height)) — rekenen tegen viewH, en een
// top+bottom-paar zónder height is dan óók een hoogte. 0 = niets gezegd.
func cssMinExtent(cx cssContext, cp props) int {
	base := 0
	if cp["position"] == "fixed" {
		base = cx.viewH
	}
	e := 0
	for _, k := range []string{"min-height", "height"} {
		if v, ok := cssLenPct(cx, cp[k], base); ok && v > e {
			e = v
		}
	}
	if e == 0 && base > 0 && cp["bottom"] != "" {
		if t, ok := anchorLen(cx, cp["top"], base); ok {
			if b, ok2 := anchorLen(cx, cp["bottom"], base); ok2 && base-t-b > 0 {
				e = base - t - b
			}
		}
	}
	if e > 600 {
		e = 600
	}
	return e
}

// cssRadius: border-radius → de hoekstraal in px; -1 betekent "helemaal
// rond" (een procent, of een pil-waarde als 999px — het tekenen klemt op
// de halve bloklengte). Alleen de eerste waarde telt: hoek-per-hoek is
// verfijning die het 8x8-font toch niet haalt.
func cssRadius(cx cssContext, v string) int {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0
	}
	if f := splitTopLevel(v); len(f) > 0 {
		v = f[0]
	}
	if strings.HasSuffix(v, "%") {
		if f, err := strconv.ParseFloat(strings.TrimSuffix(v, "%"), 64); err == nil && f > 0 {
			return -1
		}
		return 0
	}
	if n, ok := cssLen(cx, v); ok && n > 0 {
		return n
	}
	return 0
}

// gridAreas parst grid-template-areas: elke aanhalingstekens-groep is één
// rij, de woorden erin zijn kolomnamen. nil = geen (rechthoekige) template.
func gridAreas(v string) [][]string {
	var rows [][]string
	for {
		i := strings.IndexAny(v, `"'`)
		if i < 0 {
			break
		}
		q := v[i]
		j := strings.IndexByte(v[i+1:], q)
		if j < 0 {
			break
		}
		if row := strings.Fields(v[i+1 : i+1+j]); len(row) > 0 {
			rows = append(rows, row)
		}
		v = v[i+1+j+1:]
	}
	if len(rows) == 0 {
		return nil
	}
	n := len(rows[0])
	if n < 1 || n > 6 {
		return nil
	}
	for _, r := range rows {
		if len(r) != n {
			return nil
		}
	}
	return rows
}

// expand4 expandeert een 1-4-waarden-shorthand naar boven-rechts-onder-
// links, met de CSS-herhaalregels (1 → alle, 2 → v h, 3 → t h b).
func expand4(f []string) ([4]string, bool) {
	if len(f) < 1 || len(f) > 4 {
		return [4]string{}, false
	}
	idx := map[int][4]int{1: {0, 0, 0, 0}, 2: {0, 1, 0, 1}, 3: {0, 1, 2, 1}, 4: {0, 1, 2, 3}}[len(f)]
	return [4]string{f[idx[0]], f[idx[1]], f[idx[2]], f[idx[3]]}, true
}

// hintLen: een presentational hint (het width/height-attribuut van svg's
// en ouderwetse tabellen) als CSS-lengte — kale getallen zijn pixels,
// procenten en echte lengtes gaan ongemoeid door. "" = niets bruikbaars.
func hintLen(v string) string {
	v = strings.ToLower(strings.TrimSpace(v))
	if v == "" {
		return ""
	}
	if strings.HasSuffix(v, "%") {
		if _, err := strconv.ParseFloat(strings.TrimSuffix(v, "%"), 64); err == nil {
			return v
		}
		return ""
	}
	if f, err := strconv.ParseFloat(v, 64); err == nil && f > 0 {
		return v + "px"
	}
	if _, ok := cssLen(defaultCSSContext(), v); ok {
		return v
	}
	return ""
}

// anchorLen: een anker-lengte voor absolutes — px/em altijd, procenten
// alleen mét een basis (de betreffende maat van de containing block;
// wikipedia's talencirkel: top:20% van de gegeven height).
func anchorLen(cx cssContext, v string, base int) (int, bool) {
	if strings.Contains(v, "%") && base <= 0 {
		return 0, false
	}
	return cssLenSignedPct(cx, v, base)
}

// cssPairSigned: twee (mogelijk negatieve) lengtes ("0 -40px") — voor
// background-position/-size; keywords of procenten zijn niet begrepen.
func cssPairSigned(cx cssContext, v string) (int, int, bool) {
	f := strings.Fields(v)
	if len(f) != 2 {
		return 0, 0, false
	}
	x, ok1 := cssLenSigned(cx, f[0])
	y, ok2 := cssLenSigned(cx, f[1])
	return x, y, ok1 && ok2
}

// cssLenSigned: een lengte die ook negatief mag zijn (badge-offsets).
func cssLenSigned(cx cssContext, v string) (int, bool) {
	if strings.HasPrefix(v, "-") {
		if n, ok := cssLen(cx, v[1:]); ok {
			return -n, true
		}
		return 0, false
	}
	return cssLen(cx, v)
}

// srHidden herkent visueel-verborgen in de losse declaraties: het 1x1px-
// sr-only-doosje, alles weggeknipt, buiten beeld geparkeerd (position +
// negatieve left/top, of text-indent — image replacement), of opacity:0
// (skip-links; zonder JS is dat óók in een grote browser onzichtbaar).
func srHidden(clip, clipPath, w, h, pos, top, left, ti, op string) bool {
	if w == "1px" && h == "1px" {
		return true
	}
	if f, err := strconv.ParseFloat(strings.TrimSuffix(op, "%"), 64); err == nil && f == 0 {
		return true
	}
	if strings.HasPrefix(ti, "-") {
		if n, ok := cssLen(defaultCSSContext(), ti[1:]); ok && n >= 999 {
			return true
		}
	}
	if strings.HasPrefix(clipPath, "inset(50%") || strings.HasPrefix(clipPath, "inset(100%") {
		return true
	}
	if strings.HasPrefix(clip, "rect(") {
		inner := strings.TrimSuffix(clip[len("rect("):], ")")
		all := true
		toks := strings.FieldsFunc(inner, func(r rune) bool { return r == ',' || r == ' ' })
		for _, t := range toks {
			if v, ok := cssLen(defaultCSSContext(), t); !ok || v > 1 {
				all = false
				break
			}
		}
		if all && len(toks) == 4 {
			return true
		}
	}
	if pos == "absolute" || pos == "fixed" {
		// 100px of meer het beeld uit is geen vormgeving meer, dat is
		// verstoppen (tweakers' skip-links: left:-300px).
		for _, v := range []string{top, left} {
			if n, ok := cssLen(defaultCSSContext(), strings.TrimPrefix(v, "-")); ok && strings.HasPrefix(v, "-") && n >= 100 {
				return true
			}
		}
	}
	return false
}

// offCanvas: schuift deze transform het element (vrijwel) volledig uit
// beeld? translate/translateX/translateY met een eerste component van
// -90% of erger, of -100px of erger. De centreertruc translate(-50%,-50%)
// haalt die drempels nooit.
func offCanvas(v string) bool {
	i := strings.Index(v, "translate")
	if i < 0 {
		return false
	}
	rest := v[i:]
	open := strings.IndexByte(rest, '(')
	if open < 0 {
		return false
	}
	end := closeParen(rest, open)
	if end < 0 {
		return false
	}
	arg := strings.TrimSpace(splitArgs(rest[open+1 : end])[0])
	if !strings.HasPrefix(arg, "-") {
		return false
	}
	if strings.HasSuffix(arg, "%") {
		f, err := strconv.ParseFloat(strings.TrimSuffix(arg[1:], "%"), 64)
		return err == nil && f >= 90
	}
	n, ok := cssLen(defaultCSSContext(), arg[1:])
	return ok && n >= 100
}

// cssURL haalt de url uit een url(...)-waarde; "" als die er niet is.
// data:-URI's doen niet mee (base64-decoderen is een andere klus).
func cssURL(v string) string {
	i := strings.Index(v, "url(")
	if i < 0 {
		return ""
	}
	rest := v[i+4:]
	j := strings.IndexByte(rest, ')')
	if j < 0 {
		return ""
	}
	u := strings.Trim(strings.TrimSpace(rest[:j]), `"'`)
	if u == "" || strings.HasPrefix(u, "data:") {
		return ""
	}
	return u
}

// resolveVars vervangt var(--x) en var(--x, fallback) door de waarde uit
// vars; een paar rondes diep, want variabelen verwijzen graag naar elkaar
// (gethop.org: --acc → --leaf). Onoplosbaar → lege string (de property
// valt dan stil weg — precies wat je wilt).
func resolveVars(v string, vars map[string]string) string {
	for depth := 0; depth < 4 && strings.Contains(v, "var("); depth++ {
		i := strings.Index(v, "var(")
		rest := v[i+4:]
		j := strings.IndexByte(rest, ')')
		if j < 0 {
			return ""
		}
		inner := rest[:j]
		name, fallback := inner, ""
		if c := strings.IndexByte(inner, ','); c >= 0 {
			name, fallback = inner[:c], strings.TrimSpace(inner[c+1:])
		}
		val, ok := vars[strings.TrimSpace(name)]
		if !ok {
			val = fallback
		}
		v = v[:i] + val + v[i+4+j+1:]
		v = strings.TrimSpace(v)
	}
	if strings.Contains(v, "var(") {
		return ""
	}
	return v
}

func stripComments(s string) string {
	for {
		i := strings.Index(s, "/*")
		if i < 0 {
			return s
		}
		j := strings.Index(s[i+2:], "*/")
		if j < 0 {
			return s[:i]
		}
		s = s[:i] + " " + s[i+2+j+2:]
	}
}

// specificity: ruw maar in de goede volgorde — id's boven classes boven
// tags. Pseudo-elementen (::) tellen niet dubbel.
func specificity(sel string) int {
	n := 0
	for i := 0; i < len(sel); i++ {
		switch sel[i] {
		case '#':
			n += 100
		case '.', '[':
			n += 10
		case ':':
			if i+1 < len(sel) && sel[i+1] == ':' {
				i++
			}
			n += 10
		default:
			if (i == 0 || sel[i-1] == ' ' || sel[i-1] == '>' || sel[i-1] == '+' || sel[i-1] == '~') &&
				sel[i] != '*' && sel[i] != ' ' {
				n++
			}
		}
	}
	return n
}

// cssColor parset #rgb/#rrggbb, rgb(a) en de gangbare namen. transparent
// en currentcolor zijn bewust geen kleur (ok=false): niets mee te tekenen.
func cssColor(v string) (color.RGBA, bool) {
	v = strings.TrimSpace(v)
	if v == "" {
		return color.RGBA{}, false
	}
	if v[0] == '#' {
		return hexColor(v[1:])
	}
	if strings.HasPrefix(v, "rgb(") || strings.HasPrefix(v, "rgba(") {
		f := colorArgs(v)
		if len(f) < 3 {
			return color.RGBA{}, false
		}
		var c [3]uint8
		for i := 0; i < 3; i++ {
			// SCSS-output in het wild: ook fracties ("223.176...") en
			// procenten. Afronden en klemmen, niet afkeuren.
			n, ok := colorNum(f[i], 255)
			if !ok {
				return color.RGBA{}, false
			}
			c[i] = uint8(n)
		}
		// alpha 0 (rgba(...,0)) is géén kleur; deels doorschijnend wordt
		// gewoon de kleur — wij composen niet.
		if len(f) >= 4 {
			if a, ok := colorNum(f[3], 1); ok && a == 0 {
				return color.RGBA{}, false
			}
		}
		return color.RGBA{c[0], c[1], c[2], 0xFF}, true
	}
	if strings.HasPrefix(v, "hsl(") || strings.HasPrefix(v, "hsla(") {
		f := colorArgs(v)
		if len(f) < 3 {
			return color.RGBA{}, false
		}
		h, ok1 := colorNum(f[0], 360)
		s, ok2 := colorNum(f[1], 100)
		li, ok3 := colorNum(f[2], 100)
		if !ok1 || !ok2 || !ok3 {
			return color.RGBA{}, false
		}
		return hslColor(h, s/100, li/100), true
	}
	if c, ok := namedColors[v]; ok {
		return c, true
	}
	return color.RGBA{}, false
}

// colorArgs splitst de argumenten van rgb(a)/hsl(a): komma's, spaties en de
// moderne "/"-alphanotatie zijn allemaal scheiders.
func colorArgs(v string) []string {
	inner := v[strings.IndexByte(v, '(')+1:]
	if j := strings.IndexByte(inner, ')'); j >= 0 {
		inner = inner[:j]
	}
	inner = strings.ReplaceAll(inner, "/", " ")
	return strings.FieldsFunc(inner, func(r rune) bool { return r == ',' || r == ' ' })
}

// colorNum parset één kleurcomponent: kaal getal (ook met fractie), of een
// percentage van max. Geklemd op [0, max]; "deg" mag op een hoek.
func colorNum(s string, max float64) (float64, bool) {
	s = strings.TrimSpace(strings.TrimSuffix(s, "deg"))
	pct := strings.HasSuffix(s, "%")
	f, err := strconv.ParseFloat(strings.TrimSuffix(s, "%"), 64)
	if err != nil {
		return 0, false
	}
	if pct {
		f = f / 100 * max
	}
	if f < 0 {
		f = 0
	}
	if f > max {
		f = max
	}
	return f, true
}

// hslColor: HSL → RGB (CSS Color 3). h in graden, s en l in 0..1.
func hslColor(h, s, l float64) color.RGBA {
	c := (1 - abs64(2*l-1)) * s
	hh := h / 60
	x := c * (1 - abs64(mod64(hh, 2)-1))
	var r, g, b float64
	switch {
	case hh < 1:
		r, g, b = c, x, 0
	case hh < 2:
		r, g, b = x, c, 0
	case hh < 3:
		r, g, b = 0, c, x
	case hh < 4:
		r, g, b = 0, x, c
	case hh < 5:
		r, g, b = x, 0, c
	default:
		r, g, b = c, 0, x
	}
	m := l - c/2
	to := func(f float64) uint8 { return uint8((f + m) * 255) }
	return color.RGBA{to(r), to(g), to(b), 0xFF}
}

func abs64(f float64) float64 {
	if f < 0 {
		return -f
	}
	return f
}

func mod64(f, m float64) float64 {
	for f >= m {
		f -= m
	}
	for f < 0 {
		f += m
	}
	return f
}

func hexColor(h string) (color.RGBA, bool) {
	nib := func(b byte) (uint8, bool) {
		switch {
		case b >= '0' && b <= '9':
			return b - '0', true
		case b >= 'a' && b <= 'f':
			return b - 'a' + 10, true
		case b >= 'A' && b <= 'F':
			return b - 'A' + 10, true
		}
		return 0, false
	}
	switch len(h) {
	case 3, 4: // #rgb(a)
		if len(h) == 4 {
			// alpha 0 = volledig doorzichtig: dat is géén kleur (nu.nl's
			// #0000-chips werden dekkend zwart). Deels doorschijnend
			// blijft gewoon de kleur — composen doen we bewust niet.
			if a, ok := nib(h[3]); ok && a == 0 {
				return color.RGBA{}, false
			}
		}
		var c [3]uint8
		for i := 0; i < 3; i++ {
			n, ok := nib(h[i])
			if !ok {
				return color.RGBA{}, false
			}
			c[i] = n<<4 | n
		}
		return color.RGBA{c[0], c[1], c[2], 0xFF}, true
	case 6, 8: // #rrggbb(aa)
		if len(h) == 8 {
			hi, ok1 := nib(h[6])
			lo, ok2 := nib(h[7])
			if ok1 && ok2 && hi<<4|lo == 0 {
				return color.RGBA{}, false
			}
		}
		var c [3]uint8
		for i := 0; i < 3; i++ {
			hi, ok1 := nib(h[i*2])
			lo, ok2 := nib(h[i*2+1])
			if !ok1 || !ok2 {
				return color.RGBA{}, false
			}
			c[i] = hi<<4 | lo
		}
		return color.RGBA{c[0], c[1], c[2], 0xFF}, true
	}
	return color.RGBA{}, false
}

// namedColors: de namen die je in het wild echt tegenkomt.
var namedColors = map[string]color.RGBA{
	"black":   {0x00, 0x00, 0x00, 0xFF},
	"white":   {0xFF, 0xFF, 0xFF, 0xFF},
	"red":     {0xFF, 0x00, 0x00, 0xFF},
	"green":   {0x00, 0x80, 0x00, 0xFF},
	"blue":    {0x00, 0x00, 0xFF, 0xFF},
	"yellow":  {0xFF, 0xFF, 0x00, 0xFF},
	"orange":  {0xFF, 0xA5, 0x00, 0xFF},
	"purple":  {0x80, 0x00, 0x80, 0xFF},
	"gray":    {0x80, 0x80, 0x80, 0xFF},
	"grey":    {0x80, 0x80, 0x80, 0xFF},
	"silver":  {0xC0, 0xC0, 0xC0, 0xFF},
	"maroon":  {0x80, 0x00, 0x00, 0xFF},
	"navy":    {0x00, 0x00, 0x80, 0xFF},
	"teal":    {0x00, 0x80, 0x80, 0xFF},
	"olive":   {0x80, 0x80, 0x00, 0xFF},
	"lime":    {0x00, 0xFF, 0x00, 0xFF},
	"aqua":    {0x00, 0xFF, 0xFF, 0xFF},
	"cyan":    {0x00, 0xFF, 0xFF, 0xFF},
	"fuchsia": {0xFF, 0x00, 0xFF, 0xFF},
	"magenta": {0xFF, 0x00, 0xFF, 0xFF},
	"gold":    {0xFF, 0xD7, 0x00, 0xFF},
	"pink":    {0xFF, 0xC0, 0xCB, 0xFF},
	"brown":   {0xA5, 0x2A, 0x2A, 0xFF},
	"darkred": {0x8B, 0x00, 0x00, 0xFF},
	"tomato":  {0xFF, 0x63, 0x47, 0xFF},
}
