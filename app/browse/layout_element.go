package browse

import (
	"strings"

	"golang.org/x/net/html"
)

// prepareElement voert de geordende vroege browsersemantiek uit: HTML-
// zichtbaarheid, replaced content en out-of-flow routes. handled betekent
// dat het element volledig is afgehandeld of bewust verborgen.
func (l *layouter) prepareElement(el *html.Node, st style) (cp props, handled bool) {
	tag := el.Data
	handled = true
	if tag == "svg" {
		// Vóór de skip: een inline <svg> ís vaak het logo — rasteren.
		// (skip houdt hem wel uit de tekst-helpers: <svg><title> is geen
		// zichtbare tekst.)
		l.inlineSVG(el, st)
		return
	}
	if skip[tag] {
		return
	}
	if _, hidden := attr(el, "hidden"); hidden {
		return
	}
	// aria-hidden="true" op structuurelementen: het dichtgeklapte JS-menu
	// (<nav class="full-menu">) en ad-panelen (<aside>) die visueel ook
	// niemand ziet. Bewust níet op content: nu.nl markeert zijn (zichtbare!)
	// teaserfoto's als decoratief — die willen we juist wel.
	if v, ok := attr(el, "aria-hidden"); ok && strings.TrimSpace(v) == "true" {
		switch tag {
		case "nav", "aside", "dialog", "menu":
			return
		}
	}
	// <dialog> zonder open is per spec display:none (cookiebanners!).
	if tag == "dialog" {
		if _, open := attr(el, "open"); !open {
			return
		}
	}
	// Computed props (uit de stylesheets) + inline style="" (wint altijd).
	cp = l.propsOf(el)
	// display:none is de waardevolste property van allemaal: cookiebanners,
	// dichtgeklapte menu's en ander verborgen vuil verdwijnen echt.
	if cp["display"] == "none" || cp["visibility"] == "hidden" {
		return
	}
	// Image replacement ("het logo-patroon"): een element met een
	// background-image op vaste maat, waarvan de tekst leeg is of expres
	// onzichtbaar gemaakt (text-indent:-9999px, sr-only) — dat élement ís
	// de afbeelding. Renderen als plaatje, de weggeschoven tekst vervalt.
	if m, w, h := l.bgReplacement(el, cp); m != nil {
		l.imageSized(m, w, h, st, cp["background-size"] == "cover")
		return
	}
	// srProp is onze eigen vondst uit parseDecls: het sr-only-patroon
	// (1x1px, weggeknipt of buiten beeld) — verborgen zónder display:none.
	if cp[srProp] == "1" {
		return
	}
	// ARIA: een leeg element met role="img" en een aria-label ís een
	// afbeelding met alt-tekst (tweakers' <twk-icon> komt zonder JS leeg
	// over de lijn) — het alt-principe, net als bij een kapotte <img>.
	if v, ok := attr(el, "role"); ok && strings.TrimSpace(v) == "img" && l.emptyContent(el) {
		if lbl, ok := attr(el, "aria-label"); ok && strings.TrimSpace(lbl) != "" {
			l.word("["+strings.TrimSpace(lbl)+"]", style{scale: st.scale, col: colRule, href: st.href, indent: st.indent})
			l.space = true
			return
		}
	}
	// <details> zonder open is dichtgeklapt: alleen de <summary> zichtbaar
	// — het HTML-mechanisme zelf, geen JS voor nodig.
	if tag == "details" {
		if _, open := attr(el, "open"); !open {
			for c := el.FirstChild; c != nil; c = c.NextSibling {
				if c.Type == html.ElementNode && c.Data == "summary" {
					l.walk(c, st)
				}
			}
			return
		}
	}
	// Onderin vastgeplakt (fixed + bottom, geen top): een cookiebar of
	// app-banner. Zonder JS is die niet weg te klikken en hij zou in de
	// flow midden door de pagina renderen — weg ermee.
	if cp["position"] == "fixed" && cp["top"] == "" && cp["bottom"] != "" {
		return
	}
	// position:fixed mét een anker ónder de bovenrand: een zijbalk of
	// paneel tegen het venster geplakt (tweakers' panes: top:48px, right:0,
	// height:calc(100% - 48px)). De containing block is de víewport — niet
	// een voorouder. Wat wél tegen de bovenrand zit blijft de gepinde
	// header (verderop, beginPin).
	if cp["position"] == "fixed" && el != l.absEl {
		if v, ok := anchorLen(l.css, cp["top"], l.css.viewH); ok && v > 8 {
			l.fixedPanel(el, cp, st)
			return
		}
	}
	// position:absolute: uit de flow, op zijn coördinaten t.o.v. de
	// dichtstbijzijnde gepositioneerde voorouder, en bovenop geschilderd
	// (badges, labels, overlays). absEl bewaakt de recursie: absolute()
	// legt dit element daarbinnen als gewoon blok.
	// Alleen mét een anker (top/left/right/bottom) gaat een absolute echt
	// uit de flow — zonder coördinaten zou hij als overlay-junk over zijn
	// broers heen vallen, terwijl de flow-plek precies is waar hij hoort.
	// fillAbs (top:0+left:0) is de fotolijst-vulling en blijft in de flow —
	// maar een léég element is geen content-vulling: dat is de vullingsbalk
	// van een progressiebalk (gethop), die moet juist wél de absolute route.
	if cp["position"] == "absolute" && el != l.absEl && (!fillAbs(l.css, cp) || l.emptyContent(el)) &&
		(cp["top"] != "" || cp["left"] != "" || cp["right"] != "" || cp["bottom"] != "") {
		if cp["top"] == "" && cp["bottom"] != "" {
			// bottom-anker: de voorouderhoogte is er pas bij het sluiten —
			// parkeren; flushAbs legt hem zodra de onderkant bekend is.
			l.pend = append(l.pend, pendAbs{el: el, cp: cp, st: st, oi: len(l.origins) - 1})
			return
		}
		l.absolute(el, cp, st, -1)
		return
	}
	// float: het blok drijft naar links of rechts en de flow stroomt
	// ernaast — het krantenpatroon, ook voor niet-afbeeldingen (kaders,
	// tags). <img> heeft zijn eigen float-pad (floatImage) verderop, en
	// position wint per spec van float.
	if fl := cp["float"]; (fl == "left" || fl == "right") && tag != "img" &&
		el != l.rootEl && el != l.absEl && cp["position"] != "absolute" && cp["position"] != "fixed" {
		if l.floatBlock(el, cp, st, fl == "right") {
			return
		}
	}
	// De regel-kap: -webkit-line-clamp N (teaser-kaarten) of nowrap +
	// text-overflow:ellipsis (éénregel-titels). Zonder kap lopen de kaarten
	// vol en staat elke kolommen-balans scheef — precies wat de site met de
	// clamp voorkómt.
	if n := clampLines(cp); n > 0 && el != l.rootEl && el != l.absEl && !st.inline &&
		cp["position"] != "absolute" && cp["position"] != "fixed" {
		if l.clampBlock(el, cp, st, n) {
			return
		}
	}
	handled = false
	return
}
