package gbapp

import (
	"image"
	"testing"

	"github.com/Humpheh/goboy/pkg/gb"
)

// De patch-gate: NewFromData (ons additieve pkg/gb-bestand) moet bestaan en
// een draaiende emulator opleveren — een frame NOP's executeren zonder paniek
// is genoeg bewijs dat de verse upstream-HEAD nog met de patch samenwerkt.
func TestNewFromDataRunsAFrame(t *testing.T) {
	rom := make([]byte, 0x8000) // kleinste echte cart: 32K ROM-only, vol NOP's
	g, err := gb.NewFromData(rom, "", gb.WithCGBEnabled())
	if err != nil {
		t.Fatalf("NewFromData: %v", err)
	}
	if cycles := g.Update(); cycles < gb.CyclesFrame {
		t.Fatalf("Update deed %d cycles, wil minstens %d (één frame)", cycles, gb.CyclesFrame)
	}
}

func TestNewFromDataRejectsHeaderlessROM(t *testing.T) {
	if _, err := gb.NewFromData(make([]byte, 16), ""); err == nil {
		t.Fatal("16-byte rom hoort te weigeren (geen header)")
	}
}

func TestFit(t *testing.T) {
	for _, tc := range []struct {
		w, h, sc int
		off      image.Point
	}{
		{480, 432, 3, image.Pt(0, 0)},            // de hint: exact 3×
		{500, 432, 3, image.Pt(10, 0)},           // restruimte: gecentreerd
		{320, 288, 2, image.Pt(0, 0)},            // kleiner window: 2×
		{100, 100, 1, image.Pt(-30, -22)},        // te klein: 1× en knippen
		{1600, 1440, 10, image.Pt(0, 0)},         // groot: 10×
		{1600, 300, 2, image.Pt(640, 6)},         // breed: hoogte beslist
	} {
		sc, off := fit(image.Rect(0, 0, tc.w, tc.h))
		if sc != tc.sc || off != tc.off {
			t.Errorf("fit(%dx%d) = %d,%v; wil %d,%v", tc.w, tc.h, sc, off, tc.sc, tc.off)
		}
	}
}

func TestDirtyBox(t *testing.T) {
	var a, b [gb.ScreenWidth][gb.ScreenHeight][3]uint8
	if _, dirty := dirtyBox(&a, &b); dirty {
		t.Fatal("gelijke frames horen schoon te zijn")
	}
	b[10][20] = [3]uint8{1, 2, 3}
	b[30][40] = [3]uint8{4, 5, 6}
	box, dirty := dirtyBox(&a, &b)
	if !dirty || box != image.Rect(10, 20, 31, 41) {
		t.Fatalf("box = %v (dirty=%v), wil (10,20)-(31,41)", box, dirty)
	}
}

// blit hoort buiten de windowranden te knippen in plaats van te panieken —
// het geval "window kleiner dan het GameBoy-scherm" (fit geeft dan een
// negatieve offset).
func TestBlitClipsSmallWindow(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 100, 100))
	var data [gb.ScreenWidth][gb.ScreenHeight][3]uint8
	data[0][0] = [3]uint8{0xAA, 0xBB, 0xCC}
	sc, off := fit(img.Bounds())
	blit(img, &data, image.Rect(0, 0, gb.ScreenWidth, gb.ScreenHeight), sc, off)
	if c := img.RGBAAt(0, 0); c.A != 0xFF {
		t.Fatalf("linksboven niet gevuld: %v", c)
	}
}
