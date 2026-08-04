// Package gbapp is de SURF-kant van de goboy-port: één Drive die de emulator
// achter een window laat draaien — 60 GameBoy-frames per seconde het
// PreparedData-scherm in het window blitten (geheeltallig geschaald,
// gecentreerd), en SURF-toetsevents terug de emulator in. Net als de andere
// app-packages bewust niet aan applib gekoppeld: de cmd-mains (tamago én
// host) rijden dezelfde Drive.
package gbapp

import (
	"image"
	"image/color"
	"image/draw"
	"sync"
	"time"

	"github.com/Humpheh/goboy/pkg/gb"
	"github.com/xinix00/hop-os-surf/stack/surf"
	"github.com/xinix00/hop-os-surf/stack/window"
)

// Scale is de vensterhint: 3× het GameBoy-scherm (160×144 → 480×432). De WM
// mag er wat anders van maken (CONFIGURE is de wet), Drive past zich aan.
const Scale = 3

// Hint is de gevraagde venstermaat.
func Hint() (w, h int) { return gb.ScreenWidth * Scale, gb.ScreenHeight * Scale }

// keymap: JS-keyCodes (surf.InputKey v0, wat de web-KVM stuurt) → GameBoy-
// knoppen. Dezelfde indeling als goboy's eigen pixelbinding: Z=A, X=B,
// Enter=Start, Backspace=Select, pijltjes=dpad; Escape pauzeert, '=' wisselt
// het DMG-palet.
var keymap = map[uint32]gb.Button{
	90: gb.ButtonA,      // Z
	88: gb.ButtonB,      // X
	13: gb.ButtonStart,  // Enter
	8:  gb.ButtonSelect, // Backspace
	37: gb.ButtonLeft,   // ←
	38: gb.ButtonUp,     // ↑
	39: gb.ButtonRight,  // →
	40: gb.ButtonDown,   // ↓

	27:  gb.ButtonPause,         // Escape
	187: gb.ButtonChangePallete, // '='
}

// Drive is de hele GameBoy achter het window: maakt de emulator aan uit de
// ROM-bytes en draait dan het 60Hz-frameritme tot het window sterft — de
// cmd-main beslist over exit. CGB staat aan zoals bij upstream: een
// DMG-cartridge draait er gewoon DMG in.
func Drive(win *window.Window, rom []byte, logf func(string, ...any)) error {
	g, err := gb.NewFromData(rom, "", gb.WithCGBEnabled())
	if err != nil {
		return err
	}
	logf("goboy: rom geladen (%d bytes, cgb=%v)", len(rom), g.IsCGB())

	// De emulator wil per frame één Pressed/Released-lijst (ProcessInput),
	// dus de eventgoroutine spaart toetsen op tot het volgende frame.
	var mu sync.Mutex
	var pending gb.ButtonInput
	resized := false
	go func() {
		for ev := range win.Events() {
			switch {
			case ev.Kind == window.KindResize:
				mu.Lock()
				resized = true
				mu.Unlock()
			case ev.Kind == surf.InputKey:
				if b, ok := keymap[ev.Code]; ok {
					mu.Lock()
					if ev.Value == 1 {
						pending.Pressed = append(pending.Pressed, b)
					} else {
						pending.Released = append(pending.Released, b)
					}
					mu.Unlock()
				}
			}
		}
	}()

	// prev is het vorige GameBoy-frame: alleen het veranderde deel gaat de
	// lijn over (dirtyBox), en met de stream-compressie erachter is een
	// stilstaand scherm dan bijna niets.
	var prev [gb.ScreenWidth][gb.ScreenHeight][3]uint8
	full := true // eerste frame (en elke resize): alles, met randen
	tick := time.NewTicker(time.Second / gb.FramesSecond)
	defer tick.Stop()
	for range tick.C {
		mu.Lock()
		in := pending
		pending = gb.ButtonInput{}
		res := resized
		resized = false
		mu.Unlock()

		g.ProcessInput(in)
		g.Update()

		box, dirty := dirtyBox(&prev, &g.PreparedData)
		prev = g.PreparedData
		full = full || res
		if !dirty && !full {
			continue // gepauzeerd of stilstaand beeld: niets te sturen
		}

		img := win.Image()
		sc, off := fit(img.Bounds())
		if full {
			draw.Draw(img, img.Bounds(), image.NewUniform(color.Black), image.Point{}, draw.Src)
			box = image.Rect(0, 0, gb.ScreenWidth, gb.ScreenHeight)
		}
		blit(img, &g.PreparedData, box, sc, off)
		if full {
			err = win.Present()
		} else {
			r := image.Rect(off.X+box.Min.X*sc, off.Y+box.Min.Y*sc,
				off.X+box.Max.X*sc, off.Y+box.Max.Y*sc).Intersect(img.Bounds())
			err = win.Present(r)
		}
		if err != nil {
			return err
		}
		full = false
	}
	return nil
}

// fit kiest de grootste geheeltallige schaal die in het window past (minimaal
// 1 — een te klein window knipt) en centreert het scherm.
func fit(b image.Rectangle) (sc int, off image.Point) {
	sc = min(b.Dx()/gb.ScreenWidth, b.Dy()/gb.ScreenHeight)
	if sc < 1 {
		sc = 1
	}
	off = image.Pt(b.Min.X+(b.Dx()-gb.ScreenWidth*sc)/2, b.Min.Y+(b.Dy()-gb.ScreenHeight*sc)/2)
	return sc, off
}

// dirtyBox is de kleinste rechthoek (in GameBoy-coördinaten) waarbinnen het
// nieuwe frame van het vorige verschilt.
func dirtyBox(prev, cur *[gb.ScreenWidth][gb.ScreenHeight][3]uint8) (image.Rectangle, bool) {
	minX, minY := gb.ScreenWidth, gb.ScreenHeight
	maxX, maxY := -1, -1
	for x := 0; x < gb.ScreenWidth; x++ {
		for y := 0; y < gb.ScreenHeight; y++ {
			if prev[x][y] != cur[x][y] {
				minX, minY = min(minX, x), min(minY, y)
				maxX, maxY = max(maxX, x), max(maxY, y)
			}
		}
	}
	if maxX < 0 {
		return image.Rectangle{}, false
	}
	return image.Rect(minX, minY, maxX+1, maxY+1), true
}

// blit tekent de GameBoy-pixels uit box als sc×sc-blokken in het window,
// geknipt op de windowranden.
func blit(img *image.RGBA, data *[gb.ScreenWidth][gb.ScreenHeight][3]uint8, box image.Rectangle, sc int, off image.Point) {
	bounds := img.Bounds()
	for x := box.Min.X; x < box.Max.X; x++ {
		for y := box.Min.Y; y < box.Max.Y; y++ {
			px := data[x][y]
			block := image.Rect(off.X+x*sc, off.Y+y*sc, off.X+(x+1)*sc, off.Y+(y+1)*sc).Intersect(bounds)
			for wy := block.Min.Y; wy < block.Max.Y; wy++ {
				i := img.PixOffset(block.Min.X, wy)
				for wx := block.Min.X; wx < block.Max.X; wx++ {
					img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = px[0], px[1], px[2], 0xFF
					i += 4
				}
			}
		}
	}
}
