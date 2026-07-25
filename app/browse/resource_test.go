package browse

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
)

func TestSubresourcesShareOneByteCache(t *testing.T) {
	var imageRequests atomic.Int32
	var cssRequests atomic.Int32

	var picture bytes.Buffer
	m := image.NewRGBA(image.Rect(0, 0, 2, 2))
	m.SetRGBA(0, 0, color.RGBA{R: 255, A: 255})
	if err := png.Encode(&picture, m); err != nil {
		t.Fatal(err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/asset.png", func(w http.ResponseWriter, _ *http.Request) {
		imageRequests.Add(1)
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(picture.Bytes())
	})
	mux.HandleFunc("/shared.css", func(w http.ResponseWriter, _ *http.Request) {
		cssRequests.Add(1)
		w.Header().Set("Content-Type", "text/css")
		_, _ = w.Write([]byte("body{color:#123456}"))
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`<html><head>
<link rel="apple-touch-icon" href="/asset.png">
</head><body><img src="/asset.png" alt="asset"></body></html>`))
	})

	s := NewSessionHandler(mux)
	if err := s.Go("http://example.test/"); err != nil {
		t.Fatal(err)
	}
	// Dezelfde URL wordt zowel als <img> als site-icoon gedecodeerd, maar
	// de bytes horen maar één keer over de transportgrens te gaan.
	if got := imageRequests.Load(); got != 1 {
		t.Fatalf("gedeelde afbeelding %d keer opgehaald, wil 1", got)
	}
	if err := s.Go("http://example.test/twee"); err != nil {
		t.Fatal(err)
	}
	if got := imageRequests.Load(); got != 2 {
		t.Fatalf("cache hoort per pagina te leven: na navigatie %d requests, wil 2", got)
	}

	const callers = 32
	var wg sync.WaitGroup
	errs := make(chan error, callers)
	for range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, err := s.fetchResource("/shared.css", 1<<20)
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if got := cssRequests.Load(); got != 1 {
		t.Fatalf("gelijktijdige subresource %d keer opgehaald, wil 1", got)
	}

	// Een nieuw document krijgt een verse, begrensde cache. Binnen dat
	// document delen <img> en icoon opnieuw één fetch.
	if err := s.Go("http://example.test/"); err != nil {
		t.Fatal(err)
	}
	if got := imageRequests.Load(); got != 3 {
		t.Fatalf("nieuw document deelde de oude bytecache: %d fetches, wil 3", got)
	}
}

func TestSessionsDoNotShareCookieJar(t *testing.T) {
	a, b := NewSession(), NewSession()
	if a.client == b.client {
		t.Fatal("browservensters delen hun http.Client")
	}
	if a.client.Jar == b.client.Jar {
		t.Fatal("browservensters delen hun cookie-jar")
	}
}
