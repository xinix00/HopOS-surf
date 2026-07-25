package browse

import (
	"strconv"
	"strings"
)

// fontScale vertaalt een font-size naar onze schaal 1..3 (8/16/24px):
// wij hebben drie lettermaten, CSS heeft er oneindig veel — afronden dus.
func fontScale(cx cssContext, v string, cur int) int {
	px := 0.0
	switch {
	case strings.HasSuffix(v, "px"):
		px, _ = strconv.ParseFloat(strings.TrimSuffix(v, "px"), 64)
	case strings.HasSuffix(v, "rem"):
		f, _ := strconv.ParseFloat(strings.TrimSuffix(v, "rem"), 64)
		px = f * cx.remPx
	case strings.HasSuffix(v, "em"):
		f, _ := strconv.ParseFloat(strings.TrimSuffix(v, "em"), 64)
		px = f * cx.remPx
	case strings.HasSuffix(v, "%"):
		f, _ := strconv.ParseFloat(strings.TrimSuffix(v, "%"), 64)
		px = f / 100 * cx.remPx
	case v == "xx-large" || v == "xxx-large":
		px = 32
	case v == "x-large":
		px = 24
	case v == "large" || v == "larger":
		px = 18
	case v == "small" || v == "smaller" || v == "x-small" || v == "xx-small":
		px = 12
	case v == "medium":
		px = 16
	default:
		return cur
	}
	switch {
	case px <= 0:
		return cur
	case px >= 28:
		// Alleen échte display-koppen naar 3: op een venster van ~480px is
		// schaal 3 maar ~20 tekens per regel — krantenkoppen (24-26px)
		// lezen op 2 een stuk beter.
		return 3
	case px >= 17:
		return 2
	default:
		return 1
	}
}

// edges is één boxmodel-zijde-set (margin of padding) in pixels; autoL/R
// staan voor "margin: 0 auto" — het klassieke centreer-signaal.
type edges struct {
	t, r, b, l   int
	autoL, autoR bool
	setV, setH   bool // verticaal/horizontaal expliciet gezet (anders: UA-default)
}

// capEdge klemt één zijde: negatieve marges en 100vh-achtige uitschieters
// zijn layout-trucs die ons flow-model alleen maar slopen.
func capEdge(v, max int) int {
	if v < 0 {
		return 0
	}
	if v > max {
		return max
	}
	return v
}

// cssEdgesOf leest margin of padding uit de props: de shorthand (1-4
// waarden, CSS-volgorde boven-rechts-onder-links) plus de losse zijden
// eroverheen. Procenten tellen als 0 (padding-top:56% is de aspect-ratio-
// hack — die wil je echt niet als lege ruimte renderen).
func cssEdgesOf(cx cssContext, cp props, name string, maxPx int) edges {
	e := edges{}
	one := func(v string) (px int, auto, ok bool) {
		v = strings.TrimSpace(v)
		if v == "auto" {
			return 0, true, true
		}
		if strings.HasSuffix(v, "%") {
			return 0, false, true
		}
		if n, ok := cssLen(cx, v); ok {
			return capEdge(n, maxPx), false, true
		}
		return 0, false, false
	}
	// Alleen de longhands: de shorthand is bij het parsen al geëxpandeerd
	// (parseDecls), dus de cascade-volgorde zit dáár al goed.
	for side, dst := range map[string]*int{"-top": &e.t, "-right": &e.r, "-bottom": &e.b, "-left": &e.l} {
		if v, ok := cp[name+side]; ok {
			if px, auto, ok := one(v); ok {
				*dst = px
				switch side {
				case "-top", "-bottom":
					e.setV = true
				case "-left":
					e.autoL, e.setH = auto, true
				case "-right":
					e.autoR, e.setH = auto, true
				}
			}
		}
	}
	return e
}

// cssLenPct: een lengte die ook een percentage (van avail), een simpele
// calc() of een min()/max()/clamp() mag zijn.
func cssLenPct(cx cssContext, v string, avail int) (int, bool) {
	v = strings.TrimSpace(v)
	if strings.HasPrefix(v, "calc(") {
		end := closeParen(v, len("calc(")-1)
		if end < 0 {
			return 0, false
		}
		return cssCalc(cx, v[len("calc("):end], avail)
	}
	for _, fn := range []string{"min(", "max(", "clamp("} {
		if strings.HasPrefix(v, fn) {
			end := closeParen(v, len(fn)-1)
			if end < 0 {
				return 0, false
			}
			return cssMinMax(cx, fn, splitArgs(v[len(fn):end]), avail)
		}
	}
	if strings.HasSuffix(v, "%") {
		f, err := strconv.ParseFloat(strings.TrimSuffix(v, "%"), 64)
		if err != nil {
			return 0, false
		}
		return int(f / 100 * float64(avail)), true
	}
	return cssLen(cx, v)
}

// cssCalc rekent een calc-expressie uit: lengtes, percentages en kale
// getallen met + - * / ertussen (spaties om + en -, zoals de spec eist),
// haakjes-groepen incluis — tweakers' page-grid rekent
// "(3 - 1) * 1rem + ( 3 * 344px )". * en / gaan vóór + en -.
func cssCalc(cx cssContext, expr string, avail int) (int, bool) {
	t, ok := calcEval(cx, expr, avail)
	if !ok || !t.px {
		return 0, false
	}
	return int(t.v), true
}

// calcTerm is één calc-waarde: een kaal getal (schaal) of een lengte (px).
type calcTerm struct {
	v  float64
	px bool
}

func calcEval(cx cssContext, expr string, avail int) (calcTerm, bool) {
	toks := splitTopLevel(expr)
	if len(toks) == 0 || len(toks)%2 == 0 {
		return calcTerm{}, false
	}
	term := func(tok string) (calcTerm, bool) {
		if strings.HasPrefix(tok, "(") && strings.HasSuffix(tok, ")") {
			return calcEval(cx, tok[1:len(tok)-1], avail)
		}
		if strings.HasPrefix(tok, "calc(") {
			end := closeParen(tok, len("calc(")-1)
			if end < 0 {
				return calcTerm{}, false
			}
			return calcEval(cx, tok[len("calc("):end], avail)
		}
		if f, err := strconv.ParseFloat(tok, 64); err == nil {
			return calcTerm{v: f}, true
		}
		if n, ok := cssLenPct(cx, tok, avail); ok {
			return calcTerm{v: float64(n), px: true}, true
		}
		return calcTerm{}, false
	}
	// Eerst alle termen oplossen, dan * en /, dan + en -.
	vals := make([]calcTerm, 0, (len(toks)+1)/2)
	ops := make([]string, 0, len(toks)/2)
	for i, tok := range toks {
		if i%2 == 1 {
			if tok != "+" && tok != "-" && tok != "*" && tok != "/" {
				return calcTerm{}, false
			}
			ops = append(ops, tok)
			continue
		}
		v, ok := term(tok)
		if !ok {
			return calcTerm{}, false
		}
		vals = append(vals, v)
	}
	for i := 0; i < len(ops); {
		a, b := vals[i], vals[i+1]
		switch ops[i] {
		case "*":
			if a.px && b.px {
				return calcTerm{}, false // px maal px bestaat niet
			}
			vals[i] = calcTerm{v: a.v * b.v, px: a.px || b.px}
		case "/":
			if b.v == 0 || b.px {
				return calcTerm{}, false
			}
			vals[i] = calcTerm{v: a.v / b.v, px: a.px}
		default:
			i++
			continue
		}
		vals = append(vals[:i+1], vals[i+2:]...)
		ops = append(ops[:i], ops[i+1:]...)
	}
	total := vals[0]
	for i, op := range ops {
		b := vals[i+1]
		if total.px != b.px {
			return calcTerm{}, false // px plus schaal is geen maat
		}
		if op == "+" {
			total.v += b.v
		} else {
			total.v -= b.v
		}
	}
	return total, true
}

// splitTopLevel splitst op spaties van het buitenste niveau: alles binnen
// haakjes blijft één token ("repeat(3, minmax(0, 1fr))").
func splitTopLevel(s string) []string {
	var out []string
	depth, start := 0, -1
	for i := 0; i < len(s); i++ {
		switch {
		case s[i] == '(':
			depth++
		case s[i] == ')':
			depth--
		case (s[i] == ' ' || s[i] == '\t') && depth == 0:
			if start >= 0 {
				out = append(out, s[start:i])
				start = -1
			}
			continue
		}
		if start < 0 {
			start = i
		}
	}
	if start >= 0 {
		out = append(out, s[start:])
	}
	return out
}

// cssMinMax rekent min()/max()/clamp() uit over de oplosbare argumenten
// (een vw-term valt gewoon af). clamp(a, x, b) klemt x op [a, b]; is de
// middenterm onoplosbaar dan is het midden van a en b de beste gok.
func cssMinMax(cx cssContext, fn string, args []string, avail int) (int, bool) {
	if fn == "clamp(" && len(args) == 3 {
		lo, okLo := cssLenPct(cx, args[0], avail)
		mid, okMid := cssLenPct(cx, args[1], avail)
		hi, okHi := cssLenPct(cx, args[2], avail)
		switch {
		case okMid:
			if okLo && mid < lo {
				mid = lo
			}
			if okHi && mid > hi {
				mid = hi
			}
			return mid, true
		case okLo && okHi:
			return (lo + hi) / 2, true
		}
		return 0, false
	}
	best, ok := 0, false
	for _, a := range args {
		v, okA := cssLenPct(cx, a, avail)
		if !okA {
			continue
		}
		if !ok || (fn == "min(" && v < best) || (fn == "max(" && v > best) {
			best, ok = v, true
		}
	}
	return best, ok
}

// splitArgs splitst functie-argumenten op de komma's van het buitenste
// niveau (geneste haakjes blijven heel).
func splitArgs(s string) []string {
	var out []string
	depth, start := 0, 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '(':
			depth++
		case ')':
			depth--
		case ',':
			if depth == 0 {
				out = append(out, strings.TrimSpace(s[start:i]))
				start = i + 1
			}
		}
	}
	return append(out, strings.TrimSpace(s[start:]))
}

// cssRatio parst een aspect-ratio-waarde: "16 / 9", "16/9" of "1.5".
func cssRatio(v string) (num, den float64, ok bool) {
	parts := strings.SplitN(strings.ReplaceAll(v, " ", ""), "/", 2)
	num, err := strconv.ParseFloat(parts[0], 64)
	if err != nil || num <= 0 {
		return 0, 0, false
	}
	den = 1
	if len(parts) == 2 {
		den, err = strconv.ParseFloat(parts[1], 64)
		if err != nil || den <= 0 {
			return 0, 0, false
		}
	}
	return num, den, true
}

// markerType vertaalt list-style(-type) naar ons lijstteken: "" (geen),
// "1" (tellen — ook voor letter/romeinse lijsten) of "-" (elk bolletje).
func markerType(v, cur string) string {
	for _, tok := range strings.Fields(v) {
		switch tok {
		case "none":
			return ""
		case "decimal", "decimal-leading-zero", "lower-alpha", "upper-alpha",
			"lower-latin", "upper-latin", "lower-roman", "upper-roman":
			return "1"
		case "disc", "circle", "square", "disclosure-closed", "disclosure-open":
			return "-"
		}
	}
	return cur
}

// firstColorIn zoekt de eerste kleur in een gradient-waarde — onze vlakke
// benadering van een verloop is zijn eerste kleurstop.
func firstColorIn(v string) string {
	for _, tok := range strings.FieldsFunc(v, func(r rune) bool {
		return r == ',' || r == ' ' || r == '(' || r == ')'
	}) {
		if _, ok := cssColor(tok); ok {
			return tok
		}
	}
	return ""
}

// flexItem leest het groeigewicht en de vaste basis (px; -1 = geen) van een
// flex-kind: de losse properties én de flex-shorthand ("flex: 1",
// "flex: 0 0 200px").
func flexItem(cx cssContext, cp props, avail int) (grow float64, basis int) {
	basis = -1
	if v, ok := cp["flex-basis"]; ok {
		if px, ok := cssLenPct(cx, v, avail); ok && px > 0 {
			basis = px
		}
	}
	if v, ok := cp["flex-grow"]; ok {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f >= 0 {
			grow = f
		}
	}
	if v, ok := cp["flex"]; ok && v != "none" && v != "auto" && v != "initial" {
		f := strings.Fields(v)
		if g, err := strconv.ParseFloat(f[0], 64); err == nil && g >= 0 {
			grow = g
		}
		// een lengte-token in de shorthand is de basis (de derde waarde,
		// maar "flex: 0 200px" bestaat ook)
		for _, tok := range f[1:] {
			if px, ok := cssLenPct(cx, tok, avail); ok && px > 0 {
				basis = px
			}
		}
	}
	return grow, basis
}

// gridSpan: hoeveel tracks beslaat dit grid-item? "1 / -1" is de hele rij,
// "span N" is N, "a / b" is b-a. 1 als er niets (begrijpelijks) staat.
func gridSpan(cp props, n int) int {
	clamp := func(s int) int {
		if s < 1 {
			return 1
		}
		if s > n {
			return n
		}
		return s
	}
	v := strings.TrimSpace(cp["grid-column"])
	if v == "" {
		return 1
	}
	span := func(s string) (int, bool) {
		if !strings.HasPrefix(s, "span") {
			return 0, false
		}
		i, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(s, "span")))
		return i, err == nil
	}
	parts := strings.SplitN(v, "/", 2)
	p0 := strings.TrimSpace(parts[0])
	if len(parts) == 2 {
		p1 := strings.TrimSpace(parts[1])
		if s, ok := span(p1); ok {
			return clamp(s)
		}
		a, errA := strconv.Atoi(p0)
		if p1 == "-1" {
			if errA == nil {
				return clamp(n - a + 1)
			}
			return n
		}
		if b, errB := strconv.Atoi(p1); errA == nil && errB == nil && b > a {
			return clamp(b - a)
		}
		return 1
	}
	if s, ok := span(p0); ok {
		return clamp(s)
	}
	return 1
}

// cssGap: de flex/grid-gap in px (gap of column-gap), geklemd.
func cssGap(cx cssContext, cp props) int {
	for _, k := range []string{"column-gap", "gap"} {
		if v, ok := cp[k]; ok {
			// "gap: 12px 8px" → de tweede is de kolom-gap.
			f := strings.Fields(v)
			if n, ok := cssLen(cx, f[len(f)-1]); ok {
				return capEdge(n, 32)
			}
		}
	}
	return 8
}

// cssRowGap: de verticale gap tussen flex/grid-rijen — expliciete row-gap,
// of gap (bij twee waarden is de eerste de rijgap). 0 zonder declaratie.
func cssRowGap(cx cssContext, cp props) int {
	if v, ok := cp["row-gap"]; ok {
		if n, ok := cssLen(cx, v); ok {
			return capEdge(n, 48)
		}
	}
	if v, ok := cp["gap"]; ok {
		if n, ok := cssLen(cx, strings.Fields(v)[0]); ok {
			return capEdge(n, 48)
		}
	}
	return 0
}

// cssOrder: de flex/grid order-property (0 zonder declaratie).
func cssOrder(cp props) int {
	if v, ok := cp["order"]; ok {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return 0
}

// gridTracks vertaalt grid-template-columns naar kolombreedtes voor deze
// beschikbare breedte: px is vast, fr/auto/minmax is gewicht, repeat()
// wordt uitgevouwen, repeat(auto-fill|auto-fit, minmax(Xpx, ...)) rekent
// het aantal kolommen uit de breedte. nil = niet te begrijpen (of één
// kolom): gewoon stapelen.
func gridTracks(cx cssContext, v string, avail, gap int) []int {
	toks := gridTokens(cx, v, avail, gap)
	if len(toks) < 2 || len(toks) > 6 {
		return nil // één kolom is stapelen; meer dan 6 wordt confetti
	}
	fixed, weight := 0, 0.0
	for _, t := range toks {
		if t.px > 0 {
			fixed += t.px
		} else {
			weight += t.fr
		}
	}
	free := avail - fixed - gap*(len(toks)-1)
	if free < 0 {
		return nil // vaste kolommen passen niet: stapelen
	}
	out := make([]int, len(toks))
	for i, t := range toks {
		if t.px > 0 {
			// een gedeclareerd smal spoor (tweakers' 0.75rem-rail) is legitiem
			out[i] = t.px
		} else if weight > 0 {
			out[i] = int(float64(free) * t.fr / weight)
			if out[i] < 60 {
				return nil // een fr-kolom moet nog iets kunnen dragen
			}
		}
	}
	return out
}

type gridTok struct {
	px int     // > 0: vaste breedte
	fr float64 // anders: gewicht
}

func gridTokens(cx cssContext, v string, avail, gap int) []gridTok {
	var out []gridTok
	for _, tok := range splitTopLevel(strings.TrimSpace(v)) {
		switch {
		case strings.HasPrefix(tok, "["):
			// Regelnamen ([content-start]) benoemen lijnen, geen sporen.
			continue
		case strings.HasPrefix(tok, "repeat("):
			end := closeParen(tok, len("repeat(")-1)
			if end < 0 {
				return nil
			}
			inner := tok[len("repeat("):end]
			c := strings.IndexByte(inner, ',')
			if c < 0 {
				return nil
			}
			count, rest := strings.TrimSpace(inner[:c]), strings.TrimSpace(inner[c+1:])
			unit := gridTokens(cx, rest, avail, gap)
			if len(unit) == 0 {
				return nil
			}
			n := 0
			switch count {
			case "auto-fill", "auto-fit":
				// De responsive standaard: zoveel kolommen van minstens
				// minmax-X als er passen.
				min := unit[0].px
				if min <= 0 {
					return nil
				}
				n = (avail + gap) / (min + gap)
				if n < 1 {
					n = 1
				}
				// De kolommen mogen meegroeien: maak ze gewichten.
				unit = []gridTok{{fr: 1}}
			default:
				m, err := strconv.Atoi(count)
				if err != nil || m < 1 || m > 6 {
					return nil
				}
				n = m
			}
			for i := 0; i < n; i++ {
				out = append(out, unit...)
			}
		case strings.HasPrefix(tok, "minmax("):
			end := closeParen(tok, len("minmax(")-1)
			if end < 0 {
				return nil
			}
			inner := tok[len("minmax("):end]
			// minmax(Xpx, 1fr): de min is interessant (voor auto-fill),
			// verder is het gewoon een groeikolom.
			if c := strings.IndexByte(inner, ','); c >= 0 {
				if px, ok := cssLen(cx, strings.TrimSpace(inner[:c])); ok && px > 0 {
					out = append(out, gridTok{px: px, fr: 1})
					continue
				}
			}
			out = append(out, gridTok{fr: 1})
		case strings.HasPrefix(tok, "fit-content("):
			// fit-content(X): inhoud tot maximaal X — voor ons het spoor X.
			end := closeParen(tok, len("fit-content(")-1)
			if end < 0 {
				return nil
			}
			if px, ok := cssLenPct(cx, tok[len("fit-content("):end], avail); ok && px > 0 {
				out = append(out, gridTok{px: px})
			} else {
				out = append(out, gridTok{fr: 1})
			}
		case strings.HasPrefix(tok, "calc("):
			if px, ok := cssLenPct(cx, tok, avail); ok && px > 0 {
				out = append(out, gridTok{px: px})
			} else {
				return nil
			}
		case strings.HasSuffix(tok, "fr"):
			f, err := strconv.ParseFloat(strings.TrimSuffix(tok, "fr"), 64)
			if err != nil || f <= 0 {
				return nil
			}
			out = append(out, gridTok{fr: f})
		case tok == "auto" || tok == "min-content" || tok == "max-content":
			out = append(out, gridTok{fr: 1})
		case strings.HasSuffix(tok, "%"):
			if px, ok := cssLenPct(cx, tok, avail); ok && px > 0 {
				out = append(out, gridTok{px: px})
			} else {
				return nil
			}
		default:
			if px, ok := cssLen(cx, tok); ok && px > 0 {
				out = append(out, gridTok{px: px})
			} else {
				return nil
			}
		}
	}
	return out
}

// gridRailPx herkent het centreer-spoor "1fr <vast> 1fr" (tweakers'
// page-grid): de vaste middenbaan is de inhoudsbreedte, de fr-flanken
// zijn marge — dat is centrering, geen kolommenset. 0 = geen rail.
func gridRailPx(cx cssContext, v string, avail, gap int) int {
	toks := gridTokens(cx, v, avail, gap)
	if len(toks) != 3 || toks[0].px != 0 || toks[2].px != 0 || toks[1].px <= 0 {
		return 0
	}
	if toks[0].fr <= 0 || toks[2].fr <= 0 {
		return 0
	}
	return toks[1].px
}

// leadFor vertaalt line-height naar onze interlinie (px lucht boven de
// volgende regel): een kale factor of procent × de regelhoogte, een
// lengte tegen de gedeclareerde font-size. Dé leesbaarheidsknop — kranten
// zetten 1.6, strakke chrome 1.1. Geklemd op [1, 16]; 0 = default/normal.
func leadFor(cx cssContext, v string, scale int, cp props) int {
	v = strings.TrimSpace(v)
	f := 0.0
	switch {
	case v == "" || v == "normal":
		return 0
	case strings.HasSuffix(v, "%"):
		if n, err := strconv.ParseFloat(strings.TrimSuffix(v, "%"), 64); err == nil {
			f = n / 100
		}
	default:
		if n, err := strconv.ParseFloat(v, 64); err == nil {
			f = n
		} else if px, ok := cssLen(cx, v); ok && px > 0 {
			base := 16.0
			if fs, ok := cssLen(cx, cp["font-size"]); ok && fs > 0 {
				base = float64(fs)
			}
			f = float64(px) / base
		}
	}
	if f <= 0 {
		return 0
	}
	extra := int(f*float64(charH(scale))) - charH(scale)
	if extra < 1 {
		extra = 1
	}
	if extra > 16 {
		extra = 16
	}
	return extra
}

// clampLines: het regelbudget van een element — (-webkit-)line-clamp N,
// of de éénregel-variant white-space:nowrap + text-overflow:ellipsis.
// 0 = geen kap.
func clampLines(cp props) int {
	for _, k := range []string{"-webkit-line-clamp", "line-clamp"} {
		if v, ok := cp[k]; ok {
			if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && n >= 1 && n <= 20 {
				return n
			}
		}
	}
	if strings.Contains(cp["text-overflow"], "ellipsis") && cp["white-space"] == "nowrap" {
		return 1
	}
	return 0
}

// boldWeight: is deze font-weight vet op een font zonder gewichten?
func boldWeight(v string) (bold, known bool) {
	switch v {
	case "bold", "bolder":
		return true, true
	case "normal", "lighter":
		return false, true
	}
	if n, err := strconv.Atoi(v); err == nil {
		return n >= 600, true
	}
	return false, false
}
