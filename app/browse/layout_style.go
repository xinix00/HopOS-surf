package browse

import (
	"strconv"
	"strings"

	"golang.org/x/net/html"
)

// applyElementStyle voegt de UA-laag en oude presentatie-attributen samen
// met de al berekende author-CSS, en vertaalt die cascade naar de compacte
// stijl die de renderer door de boom draagt.
func (l *layouter) applyElementStyle(el *html.Node, cp props, st style) (props, style) {
	tag := el.Data

	// UA-defaults liggen onder de author-props. Een h1 is niet speciaal —
	// hij heeft alleen defaults in dezelfde taal als de site-CSS.
	if ua, ok := uaProps[tag]; ok {
		m := make(props, len(ua)+len(cp))
		for k, v := range ua {
			m[k] = v
		}
		for k, v := range cp {
			m[k] = v
		}
		cp = m
	}
	if tag == "a" {
		// Alleen een <a> met href is een :link.
		if href, ok := attr(el, "href"); ok && href != "" {
			st.href = href
			if _, ok := cp["color"]; !ok {
				st.col = colLink
			}
			if _, ok := cp["text-decoration"]; !ok {
				st.under = true
			}
		}
	}

	// Oud web: presentatie-attributen, buiten het CSS-pad.
	if tag == "font" {
		if v, ok := attr(el, "color"); ok {
			if c, ok := cssColor(strings.ToLower(v)); ok {
				st.col = c
			}
		}
	}
	if v, ok := attr(el, "bgcolor"); ok {
		if c, ok := cssColor(strings.ToLower(v)); ok {
			st.bg, st.hasBG = c, true
		}
	}

	if v, ok := cp["color"]; ok {
		if c, ok := cssColor(v); ok {
			st.col = c
		}
	}

	// inherit is letterlijk de reeds meegedragen stijl. currentColor kan
	// pas worden genormaliseerd nadat de cascade-kleur bekend is. cp komt
	// uit de stylecache, dus alleen schrijven in een kopie.
	norm := false
	for _, v := range cp {
		if v == "inherit" || strings.Contains(v, "currentcolor") {
			norm = true
			break
		}
	}
	if norm {
		clone := make(props, len(cp))
		for k, v := range cp {
			switch {
			case v == "inherit":
			case strings.Contains(v, "currentcolor"):
				clone[k] = strings.ReplaceAll(v, "currentcolor", hexCSS(st.col))
			default:
				clone[k] = v
			}
		}
		cp = clone
	}
	if v, ok := cp["background-color"]; ok {
		if c, ok := cssColor(v); ok {
			st.bg, st.hasBG = c, true
		}
	}
	if v, ok := cp["font-weight"]; ok {
		if b, known := boldWeight(v); known {
			st.bold = b
		}
	}
	if v, ok := cp["font-size"]; ok {
		if strings.HasPrefix(v, "clamp(") || strings.HasPrefix(v, "min(") || strings.HasPrefix(v, "max(") {
			if n, ok := cssLenPct(l.css, v, 16); ok && n > 0 {
				v = strconv.Itoa(n) + "px"
			}
		}
		st.scale = fontScale(l.css, v, st.scale)
	}
	if v, ok := cp["text-align"]; ok {
		st.center = v == "center"
		st.right = v == "right" || v == "end"
	}
	if v, ok := cp["white-space"]; ok {
		st.pre = v == "pre"
	}
	if v, ok := cp["line-height"]; ok {
		st.lead = leadFor(l.css, v, st.scale, cp)
	}
	if v, ok := cp["text-decoration"]; ok {
		st.under = strings.Contains(v, "underline")
		st.strike = strings.Contains(v, "line-through")
	}
	if v, ok := cp["text-decoration-line"]; ok {
		st.under = strings.Contains(v, "underline")
		st.strike = strings.Contains(v, "line-through")
	}
	switch tag {
	case "sup":
		st.rise = -3
	case "sub":
		st.rise = 2
	}
	if v, ok := cp["text-transform"]; ok {
		st.xform = v
	}

	switch tag {
	case "ul", "menu", "dir":
		st.marker = "-"
	case "ol":
		st.marker = "1"
		n := 0
		st.list = &n
	}
	if v, ok := cp["list-style"]; ok {
		st.marker = markerType(v, st.marker)
	}
	if v, ok := cp["list-style-type"]; ok {
		st.marker = markerType(v, st.marker)
	}

	if st.hasBG {
		st.on, st.hasOn = st.bg, true
	}
	return cp, st
}
