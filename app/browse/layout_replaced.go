package browse

import (
	"image"
	"strings"

	"golang.org/x/net/html"
)

// layoutReplaced handelt elementtypen af die niet via de gewone child-flow
// lopen: logo-slot, regels, media en form-controls.
func (l *layouter) layoutReplaced(el *html.Node, cp props, st style) (handled bool) {
	tag := el.Data
	handled = true
	// Het logo-slot: een voorpagina-link zonder renderbare inhoud (het
	// logo is svg of een webcomponent) — het alt-tekst-principe, met het
	// site-eigen icoon als vulling. Zo staat het logo wáár de site hem
	// heeft staan, niet in een verzonnen balk.
	// Géén verzonnen naam naast het icoon: het echte wordmark bevat de
	// merknaam al — kunnen we die niet renderen (tweakers hangt hem pas
	// met JS in de DOM), dan is alléén het icoon de eerlijke weergave.
	if tag == "a" && l.icon != nil && !l.iconUsed && l.emptyContent(el) {
		if href, ok := attr(el, "href"); ok && isRootHref(href) {
			l.iconUsed = true
			st.href = href
			l.imageSized(l.icon, 28, 28, st, false)
			l.space = true
			return
		}
	}
	if st.inline {
		// In een menu-context (flex/nav) hoort lucht tussen de items, ook
		// als de bron geen witruimte heeft ("</a><a>") — flex-gap, arm.
		l.space = true
	}
	switch tag {
	case "br":
		l.breakLine()
		return
	case "hr":
		l.breakLine()
		l.blockGap(lead)
		l.flushGap()
		l.boxes = append(l.boxes, Box{
			R: image.Rect(l.pad, l.y, l.width-l.pad, l.y+1), Col: colRule, Rule: true,
		})
		l.y++
		l.blockGap(lead)
		return
	case "img":
		if src := imgSrc(el); src != "" && l.imgs[src] != nil {
			m := l.imgs[src]
			avail := l.lineRight(st.rIndent) - l.lineLeft(st.indent)
			w, h := imgSize(l.css, el, cp, m.Bounds().Dx(), m.Bounds().Dy(), avail)
			st.rad = cssRadius(l.css, cp["border-radius"]) // ronde avatars
			// display:block + margin:auto: het klassiek gecentreerde plaatje.
			if mar := cssEdgesOf(l.css, cp, "margin", 96); mar.autoL && mar.autoR {
				st.center = true
			}
			fl := cp["float"]
			if fl != "left" && fl != "right" && st.inline && l.fL.w == 0 && l.fR.w == 0 &&
				w*2 <= l.lineRight(st.rIndent)-l.lineLeft(st.indent) {
				// Teaser-patroon: in een flex-rij gaat het (eerste) plaatje
				// naar links en stroomt de kop ernaast — zonder dit stapelt
				// alles onder elkaar en lijkt geen nieuwssite op zichzelf.
				// Alleen voor échte thumbnails: een width:100%-beeld ís de
				// inhoud, daar valt niets naast te laten stromen (en de
				// float-halvering zou hem juist klein maken).
				fl = "left"
			}
			if fl == "left" || fl == "right" {
				l.floatImage(m, w, h, st, fl == "right")
			} else {
				l.imageSized(m, w, h, st, cp["object-fit"] == "cover")
			}
			return
		}
		// alt="" betekent in HTML: decoratief — dan ook géén placeholder.
		// Zonder dit werd elk icoontje (svg, lazy geladen, mislukt) een
		// grijze "[img]" en verzoop de pagina in de ruis.
		alt, hasAlt := attr(el, "alt")
		if alt = strings.TrimSpace(alt); alt == "" {
			if hasAlt {
				return
			}
			alt = "img"
		}
		l.word("["+alt+"]", style{scale: st.scale, col: colRule, href: st.href, indent: st.indent})
		l.space = true
		return
	case "video":
		// Zonder afspeler is de poster het eerlijke beeld van een video;
		// zonder poster doet de fallback-inhoud (tekst) gewoon zijn ding.
		if v, ok := attr(el, "poster"); ok && l.imgs[v] != nil {
			m := l.imgs[v]
			avail := l.lineRight(st.rIndent) - l.lineLeft(st.indent)
			w, h := imgSize(l.css, el, cp, m.Bounds().Dx(), m.Bounds().Dy(), avail)
			l.imageSized(m, w, h, st, cp["object-fit"] == "cover")
			return
		}
	case "input":
		l.input(el, st)
		return
	case "textarea":
		val := textContent(el)
		if v, ok := l.edits[el]; ok {
			val = v
		}
		l.widget(el, val, false, st)
		return
	}

	handled = false
	return
}
