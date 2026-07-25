package browse

import (
	"bytes"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"

	_ "golang.org/x/image/webp"
	"golang.org/x/net/html"
)

// --- afbeeldingen -------------------------------------------------------------

// Grenzen voor het afbeeldingen laden: dit draait straks op bare metal, en
// één foto mag de heap niet opblazen. Boven de kaders → alt-tekst, net als
// bij een laadfout. De kaders zijn per afbeelding — een paginateller was
// er ook, maar een krant tóónt gewoon 80 foto's (weg dus, Derek 22-07).
const (
	imgMaxBytes = 8 << 20 // per afbeelding, over de lijn (easyflorist: 4,8MB-webp's)
	imgMaxDim   = 2048    // px, per zijde — wat we bewáren (2048² RGBA = 16MB)
	// De decode-piek die we aandurven: sites serveren rustig 24-megapixel
	// foto's (easyflorist: 6000×4000 webp). jpeg/webp decoderen naar YCbCr
	// (~2 B/px), png/gif naar RGBA (4 B/px) — na de decode schalen we
	// meteen terug naar imgMaxDim, dus dit is een píek, geen bezit.
	imgMaxDecode = 96 << 20 // bytes (easyflorists grootste: 7952×5304 webp ≈ 84MB)
)

// loadImages haalt de <img src>'s van de huidige pagina op en decodeert ze,
// gesleuteld op het rauwe src-attribuut (waar de layout ze op terugvindt).
// Fouten zijn per afbeelding en stil: de layout valt terug op de alt-tekst.
// Draait in de nav-goroutine — de event-lus merkt er niets van.
func (s *Session) loadImages() {
	s.imgs = nil
	seen := map[string]bool{}
	var srcs []string
	load := func(src string) {
		if src == "" || seen[src] {
			return
		}
		seen[src] = true
		srcs = append(srcs, src)
	}
	eachEl(findEl(s.doc, "body"), func(n *html.Node) {
		if n.Data == "img" {
			// Dezelfde bron-keuze als de layout (src/data-src/srcset).
			load(imgSrc(n))
		}
		if n.Data == "video" {
			// De poster is het beeld dat de layout toont.
			if v, ok := attr(n, "poster"); ok {
				load(v)
			}
		}
		// background-image uit een inline style — de layout zoekt hem
		// straks op dezelfde sleutel (de rauwe url) terug.
		if inline, ok := attr(n, "style"); ok {
			if v, ok := parseDecls(inline)["background-image"]; ok {
				load(cssURL(v))
			}
		}
	})
	// background-images uit de stylesheets: uit álle gematchte regels —
	// niet alleen die van de mobiele breedte, anders mist de desktop-
	// layout straks zijn achtergronden.
	for _, r := range s.matched {
		if v, ok := r.decls["background-image"]; ok {
			load(cssURL(v))
		}
	}
	// En dan alles tegelijk over de lijn: het wachten zit in het netwerk,
	// niet in de CPU — zes verbindingen naast elkaar halen een fotorijke
	// pagina in een fractie van de seriële tijd binnen.
	type gehaald struct {
		src string
		m   image.Image
	}
	out := make(chan gehaald)
	sem := make(chan struct{}, 6)
	for _, src := range srcs {
		go func(src string) {
			sem <- struct{}{}
			m := s.fetchImage(src)
			<-sem
			out <- gehaald{src, m}
		}(src)
	}
	for range srcs {
		g := <-out
		if g.m != nil {
			if s.imgs == nil {
				s.imgs = map[string]image.Image{}
			}
			s.imgs[g.src] = g.m
		}
	}
}

// fetchImage haalt één afbeelding op (src opgelost tegen de pagina) en
// decodeert hem; nil bij elke vorm van pech of buiten de kaders.
func (s *Session) fetchImage(src string) image.Image {
	// Eerst begrensd binnenhalen, dan op de bytes DecodeConfig → Decode:
	// zo kost een te groot plaatje nooit meer dan imgMaxBytes.
	data, contentType, err := s.fetchResource(src, imgMaxBytes)
	if err != nil {
		return nil
	}
	// SVG (logo's, iconen): rasteren op eigen maat — daarna is het gewoon
	// een afbeelding als elke andere. Sprite-vellen met geneste <svg id>'s
	// eerst: die zou oksvg tot één klodder plakken.
	if looksLikeSVG(contentType, data) {
		if m := rasterSVGSheet(data, imgMaxDim); m != nil {
			return m
		}
		if m := rasterSVGNatural(data, 1024); m != nil {
			return m
		}
		return nil
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || cfg.Width < 1 || cfg.Height < 1 {
		return nil
	}
	perPix := 4 // png/gif: RGBA-achtig
	if format == "jpeg" || format == "webp" {
		perPix = 2 // YCbCr 4:2:0 ≈ 1,5 B/px, met marge
	}
	if cfg.Width*cfg.Height*perPix > imgMaxDecode {
		return nil // écht te groot: dan liever de alt-tekst
	}
	m, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil
	}
	// Reuzefoto's meteen terugschalen: de decode-piek is tijdelijk, wat we
	// bewaren blijft binnen het kader (≤2048 per zijde) — meer dan zat
	// voor het scherm, en 32 foto's op een pagina blijven zo betaalbaar.
	if b := m.Bounds(); b.Dx() > imgMaxDim || b.Dy() > imgMaxDim {
		w, h := b.Dx(), b.Dy()
		if w > imgMaxDim {
			h, w = h*imgMaxDim/w, imgMaxDim
		}
		if h > imgMaxDim {
			w, h = w*imgMaxDim/h, imgMaxDim
		}
		if w < 1 || h < 1 {
			return nil
		}
		m = scaleTo(m, w, h)
	}
	return m
}
