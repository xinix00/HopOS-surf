// SVG: heel veel logo's (tweakers, NRC) en iconen zijn vectors —
// tdewolff/canvas parseert en rastert ze (echte gradients, strokes,
// transforms; puur Go, dus ook op tamago). Drie routes komen hier samen:
// inline <svg> in de pagina, <img src="*.svg"> en svg-iconen/
// achtergronden via de Session.
package browse

import (
	"bytes"
	"image"
	"image/draw"
	"strings"

	"github.com/tdewolff/canvas"
	"github.com/tdewolff/canvas/renderers/rasterizer"
	"golang.org/x/net/html"
)

// rasterSVG rastert een SVG naar precies w×h. SVG's uit het wilde web
// kunnen de parser laten struikelen (filters, CSS-in-SVG): elke fout of
// panic is gewoon "geen beeld" — de aanroeper valt dan stil terug.
func rasterSVG(data []byte, w, h int) (m *image.RGBA) {
	defer func() {
		if recover() != nil {
			m = nil
		}
	}()
	if w < 1 || h < 1 || w > imgMaxDim || h > imgMaxDim {
		return nil
	}
	// LET OP: canvas rekt een viewBox x/y-apart de width/height-attr-doos
	// in (negeert preserveAspectRatio) — een use-symbol met een andere
	// verhouding dan zijn host-doos (NRC's logo: 491:147 in 110x55) werd
	// zo uitgerekt. De attrs eraf: de viewBox is dé verhouding, en het
	// passen in de doos doet de letterbox hieronder.
	c, err := canvas.ParseSVG(bytes.NewReader(stripRootSize(data)))
	if err != nil || c.W <= 0 || c.H <= 0 {
		return nil
	}
	// preserveAspectRatio (default xMidYMid meet): een svg wordt pássend
	// gemaakt in zijn doos (uniforme schaal, gecentreerd op transparant),
	// niet uitgerekt — tenzij de bron expliciet "none" zegt.
	s := float64(w) / c.W
	if s2 := float64(h) / c.H; s2 < s {
		s = s2
	}
	img := rasterizer.Draw(c, canvas.DPMM(s), canvas.DefaultColorSpace)
	if img == nil || img.Bounds().Dx() < 1 || img.Bounds().Dy() < 1 {
		return nil
	}
	if bytes.Contains(bytes.ToLower(svgHead(data)), []byte(`preserveaspectratio="none"`)) {
		return scaleTo(img, w, h) // rekken is hier de bedoeling
	}
	if img.Bounds().Dx() == w && img.Bounds().Dy() == h {
		return img
	}
	out := image.NewRGBA(image.Rect(0, 0, w, h))
	off := image.Pt((w-img.Bounds().Dx())/2, (h-img.Bounds().Dy())/2)
	draw.Draw(out, img.Bounds().Add(off), img, img.Bounds().Min, draw.Over)
	return out
}

// rasterSVGNatural rastert op de eigen maat: width/height uit de bron, of
// — met alléén een viewBox — de CSS default object size (max 300x150 op
// verhouding). Proportioneel gecapt op maxDim.
func rasterSVGNatural(data []byte, maxDim int) (m *image.RGBA) {
	defer func() {
		if recover() != nil {
			m = nil
		}
	}()
	c, err := canvas.ParseSVG(bytes.NewReader(data))
	if err != nil || c.W <= 0 || c.H <= 0 {
		return nil
	}
	// Met width/height-attributen zijn c.W/c.H die pixelmaten; met alleen
	// een viewBox een geschaalde lezing — dan telt enkel de verhouding.
	w, h := int(c.W+0.5), int(c.H+0.5)
	if w < 1 || h < 1 {
		return nil
	}
	if head := svgHead(data); !bytes.Contains(head, []byte("width")) || !bytes.Contains(head, []byte("height")) {
		w, h = defaultObjectSize(w, h, maxDim)
	}
	if w > maxDim {
		h, w = h*maxDim/w, maxDim
	}
	if h > maxDim {
		w, h = w*maxDim/h, maxDim
	}
	return rasterSVG(data, w, h)
}

// rasterSVGSheet rastert een sprite-vel van geneste <svg id>-sub-logo's
// (wikipedia's portal-sheet: 22 logo's onder elkaar): oksvg plakt zoiets
// tot één dekkende klodder, dus wij rasteren elk sub-svg apart en leggen
// ze op hun x/y in het vel — daarna knipt background-position er gewoon
// zijn plaatjes uit. nil = geen genest vel (de gewone route mag het doen).
func rasterSVGSheet(data []byte, maxDim int) *image.RGBA {
	doc, err := html.Parse(bytes.NewReader(data))
	if err != nil {
		return nil
	}
	root := findEl(doc, "svg")
	if root == nil {
		return nil
	}
	var subs []*html.Node
	for c := root.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode && c.Data == "svg" {
			subs = append(subs, c)
		}
	}
	if len(subs) < 2 {
		return nil
	}
	w, h := svgFloat(root, "width"), svgFloat(root, "height")
	if w < 1 || h < 1 {
		w, h = svgViewBox(root)
	}
	if w < 1 || h < 1 || w > maxDim || h > maxDim {
		return nil
	}
	vel := image.NewRGBA(image.Rect(0, 0, w, h))
	for _, s := range subs {
		sw, sh := svgFloat(s, "width"), svgFloat(s, "height")
		if sw < 1 || sh < 1 {
			sw, sh = svgViewBox(s)
		}
		if sw < 1 || sh < 1 {
			continue
		}
		x, y := svgFloat(s, "x"), svgFloat(s, "y")
		var buf bytes.Buffer
		if html.Render(&buf, s) != nil {
			continue
		}
		if m := rasterSVG(buf.Bytes(), sw, sh); m != nil {
			draw.Draw(vel, image.Rect(x, y, x+sw, y+sh), m, image.Point{}, draw.Over)
		}
	}
	return vel
}

// stripRootSize haalt de width/height-attributen van de wortel-<svg> af
// als er een viewBox is: de aanroeper bepaalt de doelmaat al, en de
// viewBox draagt de verhouding — zo kan de parser niets uitrekken.
func stripRootSize(data []byte) []byte {
	i := bytes.Index(data, []byte("<svg"))
	if i < 0 {
		return data
	}
	end := bytes.IndexByte(data[i:], '>')
	if end < 0 {
		return data
	}
	head := data[i : i+end]
	if !bytes.Contains(bytes.ToLower(head), []byte("viewbox")) {
		return data // zonder viewBox zíjn de attrs de enige maat
	}
	stripped := dropAttr(dropAttr(head, "width"), "height")
	if len(stripped) == len(head) {
		return data
	}
	out := make([]byte, 0, len(data)-len(head)+len(stripped))
	out = append(out, data[:i]...)
	out = append(out, stripped...)
	return append(out, data[i+end:]...)
}

// dropAttr verwijdert één attribuut (naam="waarde") uit een tag-kop —
// grens-bewust: style="width:…" en stroke-width blijven staan.
func dropAttr(head []byte, name string) []byte {
	low := bytes.ToLower(head)
	n := []byte(name)
	for i := 0; ; {
		j := bytes.Index(low[i:], n)
		if j < 0 {
			return head
		}
		j += i
		if j == 0 || !isSpace(low[j-1]) {
			i = j + 1
			continue
		}
		k := j + len(n)
		for k < len(low) && isSpace(low[k]) {
			k++
		}
		if k >= len(low) || low[k] != '=' {
			i = j + 1
			continue
		}
		k++
		for k < len(low) && isSpace(low[k]) {
			k++
		}
		if k < len(low) && (low[k] == '"' || low[k] == '\'') {
			q := low[k]
			m := bytes.IndexByte(low[k+1:], q)
			if m < 0 {
				return head
			}
			k += 1 + m + 1
		} else {
			for k < len(low) && !isSpace(low[k]) && low[k] != '>' {
				k++
			}
		}
		head = append(head[:j-1:j-1], head[k:]...)
		low = bytes.ToLower(head)
		i = j - 1
	}
}

// svgHead: de openings-tag van het svg-element (voor attribuut-detectie).
func svgHead(data []byte) []byte {
	i := bytes.Index(data, []byte("<svg"))
	if i < 0 {
		return nil
	}
	rest := data[i:]
	if j := bytes.IndexByte(rest, '>'); j >= 0 {
		return rest[:j]
	}
	return rest
}

// defaultObjectSize: de CSS-regel voor vervangen inhoud met alleen een
// verhouding — de grootste rechthoek met die verhouding binnen 300x150,
// verder gecapt op de beschikbare breedte.
func defaultObjectSize(rw, rh, avail int) (int, int) {
	w := 300
	h := rh * w / rw
	if h > 150 {
		h = 150
		w = rw * h / rh
	}
	if w > avail && avail > 0 {
		h, w = h*avail/w, avail
	}
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}
	return w, h
}

// looksLikeSVG: is deze (afbeeldings)respons een SVG? Het content-type of
// gewoon de bron zelf zegt het.
func looksLikeSVG(ct string, data []byte) bool {
	if strings.Contains(ct, "svg") {
		return true
	}
	head := data
	if len(head) > 512 {
		head = head[:512]
	}
	return bytes.Contains(head, []byte("<svg"))
}
