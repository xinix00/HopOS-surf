package browse

import (
	"bytes"
	"strconv"
	"strings"

	"golang.org/x/net/html"
)

// svgFloat leest een svg-maatattribuut ("24", "1.5em", "32px"; procenten
// tellen niet — daar is geen basis voor).
func svgFloat(el *html.Node, name string) int {
	v, ok := attr(el, name)
	if !ok {
		return 0
	}
	v = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(v), "px"))
	if strings.HasSuffix(v, "%") {
		return 0
	}
	if strings.HasSuffix(v, "em") {
		if f, err := strconv.ParseFloat(strings.TrimSuffix(v, "em"), 64); err == nil && f > 0 {
			return int(f * 16)
		}
		return 0
	}
	if f, err := strconv.ParseFloat(v, 64); err == nil && f > 0 {
		return int(f)
	}
	return 0
}

// svgRenderable: kan deze inline <svg> überhaupt een beeld worden — heeft
// hij een maat (viewBox of width+height) én eigen tekenwerk? Een svg die
// alleen een <use>-referentie is (het sprite-patroon: NRC's logo wijst
// naar een symbol elders in het document) kunnen we niet rasteren — dan
// is hij ook geen "inhoud", en mag het logo-slot hem blijven vervangen.
func svgRenderable(el *html.Node) bool {
	if !svgHasGraphic(el) {
		return false
	}
	if w, h := svgViewBox(el); w > 0 && h > 0 {
		return true
	}
	return svgFloat(el, "width") > 0 && svgFloat(el, "height") > 0
}

// svgHasGraphic: staat er echt tekenwerk in (paths, vormen), of alleen
// verwijzingen? Defs en symbolen tellen niet — die renderen per spec
// alléén via een <use> (anders zou een sprite-vel zelf een beeld worden).
func svgHasGraphic(el *html.Node) bool {
	found := false
	var walk func(n *html.Node)
	walk = func(n *html.Node) {
		if found {
			return
		}
		if n.Type == html.ElementNode {
			switch n.Data {
			case "defs", "symbol":
				return
			case "path", "rect", "circle", "ellipse", "polygon", "polyline", "line", "text", "image":
				found = true
				return
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	for c := el.FirstChild; c != nil; c = c.NextSibling {
		walk(c)
	}
	return found
}

// svgViewBox geeft de viewBox-maat (0,0 zonder bruikbare viewBox).
func svgViewBox(el *html.Node) (int, int) {
	v, ok := attr(el, "viewbox")
	if !ok {
		v, ok = attr(el, "viewBox")
	}
	if !ok {
		return 0, 0
	}
	f := strings.FieldsFunc(v, func(r rune) bool { return r == ' ' || r == ',' })
	if len(f) != 4 {
		return 0, 0
	}
	w, err1 := strconv.ParseFloat(f[2], 64)
	h, err2 := strconv.ParseFloat(f[3], 64)
	if err1 != nil || err2 != nil || w < 1 || h < 1 {
		return 0, 0
	}
	return int(w), int(h)
}

// inlineSVG rastert een inline <svg> op zijn plek in de flow: maat uit de
// CSS, anders de attributen, anders de viewBox (gecapt — een logo hoort
// in de regel te passen). Gerasterd op doelmaat: scherp, geen naschalen.
func (l *layouter) inlineSVG(el *html.Node, st style) {
	if l.svgN >= 24 {
		return // budget: geen icoontjes-lawine op bare metal
	}
	if !svgHasGraphic(el) {
		return // alleen <use>-referenties: daar valt niets te rasteren
	}
	cp := l.propsOf(el)
	if cp["display"] == "none" || cp["visibility"] == "hidden" || cp[srProp] == "1" {
		return // een verborgen sprite-vel (of icoon) is geen beeld
	}
	avail := l.lineRight(st.rIndent) - l.lineLeft(st.indent)
	if avail < 8 {
		return
	}
	w, h := 0, 0
	if v, ok := cssLenPct(l.css, cp["width"], avail); ok && v > 0 {
		w = v
	} else if v := svgFloat(el, "width"); v > 0 {
		w = v
	}
	if v, ok := cssLen(l.css, cp["height"]); ok && v > 0 {
		h = v
	} else if v := svgFloat(el, "height"); v > 0 {
		h = v
	}
	vbW, vbH := svgViewBox(el)
	switch {
	case w > 0 && h > 0:
	case w > 0 && vbW > 0:
		h = vbH * w / vbW
	case h > 0 && vbH > 0:
		w = vbW * h / vbH
	case vbW > 0:
		// Alleen een viewBox: dat is een coördinatenstelsel, geen maat.
		// De CSS default object size geldt: de grootste rechthoek met deze
		// verhouding binnen 300x150 (en hij moet op de regel passen).
		w, h = defaultObjectSize(vbW, vbH, avail)
	default:
		return // geen enkele maat te bekennen
	}
	if w < 4 || h < 4 {
		return
	}
	var buf bytes.Buffer
	if html.Render(&buf, el) != nil {
		return
	}
	data := buf.Bytes()
	// fill/stroke: currentColor — "de kleur van hier": de computed color
	// van de svg zelf, anders de geërfde cascade-kleur. Alomtegenwoordig op
	// iconen; zonder invulling rastert oksvg ze zwart of helemaal niet.
	if bytes.Contains(data, []byte("currentColor")) || bytes.Contains(data, []byte("currentcolor")) {
		col := st.col
		if c, ok := cssColor(cp["color"]); ok {
			col = c
		}
		hex := []byte(hexCSS(col))
		data = bytes.ReplaceAll(data, []byte("currentColor"), hex)
		data = bytes.ReplaceAll(data, []byte("currentcolor"), hex)
	}
	m := rasterSVG(data, w, h)
	if m == nil {
		return
	}
	l.svgN++
	l.imageSized(m, w, h, st, false)
}
