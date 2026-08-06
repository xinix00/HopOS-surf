package surfserve

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"net"
	"sync/atomic"
	"testing"

	"github.com/xinix00/hop-os-surf/stack/compositor"
	"github.com/xinix00/hop-os-surf/stack/surf"
	"github.com/xinix00/hop-os-surf/stack/window"
)

// De fysieke invoerweg end-to-end: een neppe HopOS-kant schrijft de JSON-regels
// die de USB-driver ook schrijft, en die moeten bij de app landen als gewone
// surf.Input — precies zoals een klik uit de browser-KVM dat doet.
//
// Dat is de eigenlijke bewering van hopinput.go: één vocabulaire, één
// routering. Slaagt dit, dan is een echt toetsenbord niet te onderscheiden van
// de KVM-pagina, en dát is de reden dat HopOS geen eigen invoer-ABI kreeg.
func TestConsumeInputVanHopOS(t *testing.T) {
	comp := compositor.New(320, 200)
	srv := New(comp, t.Logf)

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go srv.ServeSURF(&recListener{Listener: l, conns: make(chan net.Conn, 4)})

	// De HopOS-kant: luisteren en de stroom schrijven, net als gui/usbin.
	hop, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer hop.Close()
	go srv.ConsumeInput(hop.Addr().String())
	c, err := hop.Accept()
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	// Een app met een window, zodat er iets is om invoer heen te routeren.
	win, err := window.Open(l.Addr().String(), "hopinput", 60, 40, t.Logf)
	if err != nil {
		t.Fatal(err)
	}
	defer win.Close()
	green := color.RGBA{0x10, 0xCC, 0x30, 0xFF}
	draw.Draw(win.Image(), win.Image().Bounds(), image.NewUniform(green), image.Point{}, draw.Src)
	if err := win.Present(); err != nil {
		t.Fatal(err)
	}
	var at image.Point
	eventually(t, "window zichtbaar", func() bool {
		p, ok := findColorComp(comp, green)
		at = p
		return ok
	})

	// Klik in het window: dat zet ook de focus voor de toets erna. De
	// coördinaten zijn ABSOLUUT — HopOS houdt de cursor bij (een USB-muis
	// stuurt deltas), deze kant rekent er niets aan.
	fmt.Fprintf(c, "{\"k\":\"btn\",\"c\":0,\"v\":1,\"x\":%d,\"y\":%d}\n", at.X, at.Y)
	eventually(t, "klik uit de HopOS-stroom bereikt de app", func() bool {
		select {
		case ev := <-win.Events():
			return ev.Kind == surf.InputButton && ev.Value == 1
		default:
			return false
		}
	})

	fmt.Fprint(c, "{\"k\":\"key\",\"c\":65,\"v\":1}\n")
	eventually(t, "toets uit de HopOS-stroom bereikt de app", func() bool {
		select {
		case ev := <-win.Events():
			return ev.Kind == surf.InputKey && ev.Code == 65 && ev.Value == 1
		default:
			return false
		}
	})
}

// Een kapotte regel mag niet het hele toetsenbord kosten: één aanslag weg is
// het maximum. Zonder deze eigenschap hangt de invoer na één rare byte — en
// dat is op ijzer niet te onderscheiden van een dood toetsenbord.
func TestConsumeInputSlaatKapotteRegelOver(t *testing.T) {
	srv := New(compositor.New(320, 200), t.Logf)
	var x, y atomic.Int64
	srv.OnPointer(func(px, py int) { x.Store(int64(px)); y.Store(int64(py)) })

	hop, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer hop.Close()
	go srv.ConsumeInput(hop.Addr().String())
	c, err := hop.Accept()
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	fmt.Fprint(c, "dit is geen json\n")
	fmt.Fprint(c, "{\"k\":\"onbekend\"}\n")
	fmt.Fprint(c, "{\"k\":\"move\",\"x\":11,\"y\":22}\n")

	eventually(t, "de beweging ná de troep komt aan", func() bool {
		return x.Load() == 11 && y.Load() == 22
	})
}

// findColorComp zoekt een pixel met deze kleur in de compositie.
func findColorComp(c *compositor.Compositor, want color.RGBA) (image.Point, bool) {
	c.Compose()
	img, _ := c.Snapshot()
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, bl, _ := img.At(x, y).RGBA()
			if uint8(r>>8) == want.R && uint8(g>>8) == want.G && uint8(bl>>8) == want.B {
				return image.Point{X: x, Y: y}, true
			}
		}
	}
	return image.Point{}, false
}
