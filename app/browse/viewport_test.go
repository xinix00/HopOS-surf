package browse

import (
	"strings"
	"sync"
	"testing"

	"golang.org/x/net/html"
)

func TestCSSContextIsPerLayout(t *testing.T) {
	small := newCSSContext(400, 200)
	small.remPx = 10
	large := newCSSContext(800, 600)
	large.remPx = 20

	type want struct {
		cx             cssContext
		vw, vh, twoRem int
	}
	cases := []want{
		{cx: small, vw: 200, vh: 100, twoRem: 20},
		{cx: large, vw: 400, vh: 300, twoRem: 40},
	}

	var wg sync.WaitGroup
	errs := make(chan string, 2)
	for _, tc := range cases {
		tc := tc
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 1000 {
				vw, okW := cssLen(tc.cx, "50vw")
				vh, okH := cssLen(tc.cx, "50vh")
				rem, okR := cssLen(tc.cx, "2rem")
				if !okW || !okH || !okR || vw != tc.vw || vh != tc.vh || rem != tc.twoRem {
					errs <- "relatieve CSS-maat lekte tussen layouts"
					return
				}
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}

func TestLayoutViewportUsesItsOwnHeight(t *testing.T) {
	doc, err := html.Parse(strings.NewReader(
		`<html><body><div style="height:50vh;background:#123456"></div></body></html>`,
	))
	if err != nil {
		t.Fatal(err)
	}
	body := findEl(doc, "body")
	short := LayoutViewport(body, Viewport{Width: 320, Height: 200})
	tall := LayoutViewport(body, Viewport{Width: 320, Height: 600})
	if tall.Height-short.Height < 180 {
		t.Fatalf("vh gebruikte niet de meegegeven viewport: kort=%d lang=%d", short.Height, tall.Height)
	}
}
