// CSS is bewust een zichtbare, begrensde subset: cascade en mediaqueries,
// tekst, boxmodel, positionering en de bruikbare delen van flex/grid. De
// selector-kant gebruikt cascadia; onbekende declaraties, animaties en
// andere niet-renderbare mechanismen vallen stil weg.
package browse

import (
	"image/color"
	"strings"
)

// props zijn de computed properties van één element — alleen de gedragen
// subset, lowercase prop → lowercase waarde.
type props map[string]string

// cssContext bevat alle relatieve maatstaven voor één layout-run. CSS-
// lengtes mogen nooit op procesglobale viewportstate leunen: de host-
// desktop kan meerdere browservensters met verschillende maten tegelijk
// draaien. De context reist daarom met de layouter mee.
type cssContext struct {
	viewW, viewH int
	remPx        float64
}

func defaultCSSContext() cssContext {
	return cssContext{viewW: mobileWidth, viewH: 600, remPx: 16}
}

func newCSSContext(viewW, viewH int) cssContext {
	if viewW < 1 {
		viewW = mobileWidth
	}
	if viewH < 1 {
		viewH = 600
	}
	return cssContext{viewW: viewW, viewH: viewH, remPx: 16}
}

// supportedProp: regels zonder één van deze properties worden al bij het
// parsen weggegooid — het gros van echte stylesheets blijft junk (fonts,
// animaties, schaduwen), en elke overgebleven regel kost een match-ronde.
// Sinds de box-engine horen de boxmodel-properties er ook bij.
func supportedProp(p string) bool {
	if strings.HasPrefix(p, "--") {
		return true // custom property: voer voor var()-resolutie
	}
	switch p {
	case "display", "visibility", "color", "background-color", "background",
		"background-image", "background-position", "font-weight", "font-size", "text-align",
		"border", "border-color", "border-top", "border-right", "border-bottom",
		"border-left", "flex-direction", "float", "clear",
		"margin", "margin-top", "margin-right", "margin-bottom", "margin-left",
		"padding", "padding-top", "padding-right", "padding-bottom", "padding-left",
		"margin-inline", "margin-block", "padding-inline", "padding-block",
		"border-radius",
		"width", "max-width", "min-width", "height", "min-height", "gap", "column-gap", "row-gap",
		"list-style", "list-style-type",
		"background-size", "grid-template-columns", "grid-column", "grid-auto-flow", "flex-wrap",
		"grid-template-areas", "grid-area", "justify-items", "justify-self",
		"white-space", "flex", "flex-grow", "flex-basis", "flex-flow", "order",
		"object-fit", "aspect-ratio",
		"justify-content", "align-items", "align-self", "place-items", "place-content",
		"text-transform", "text-decoration", "text-decoration-line",
		"line-height", "-webkit-line-clamp", "line-clamp", "text-overflow",
		"vertical-align", "z-index":
		return true
	}
	return false
}

// cssBorder leest een border(-color)-waarde: aan/uit, de kleur (grijs als
// er alleen "1px solid" staat) en de dikte in px (1 zonder maat, cap 8).
// "none", "0" en varianten zijn uit — en "transparent" óók: dat is een
// doorzichtige rand (de ruimte-truc tegen verspringen bij hover), geen
// grijze lijn.
func cssBorder(cx cssContext, v string) (color.RGBA, int, bool) {
	if v == "" || v == "none" || v == "0" || strings.HasPrefix(v, "0 ") ||
		strings.HasPrefix(v, "0px") || strings.Contains(v, "none") ||
		strings.Contains(v, "transparent") {
		return color.RGBA{}, 0, false
	}
	col, w := colRule, 1 // default: de rustige grijze 1px-lijn
	for _, tok := range strings.Fields(v) {
		if c, ok := cssColor(tok); ok {
			col = c
		} else if n, ok := cssLen(cx, tok); ok && n > 0 {
			w = capEdge(n, 8)
		}
	}
	return col, w, true
}
