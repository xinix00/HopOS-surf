// Calc is de tweede GUI-app — en sinds 20-07 een scene-app (P2): de boom
// met knoppen reist één keer, de display rendert en hit-test, en de app
// krijgt "knop 7 geklikt" plus rauwe toetsen terug. Elke toetsaanslag is
// daarmee één PATCH van het display-Value — bytes in plaats van de oude
// pixel-damage. Muisklikken bewijzen de EVENT-terugweg, het toetsenbord de
// key-doorvoer (web-KVM en straks USB-HID).
package main

import (
	"fmt"

	"hop-os/metal/app/applib"
	"hop-os/metal/app/applib/appnet"

	"github.com/xinix00/hop-os-surf/app/calc"
	"github.com/xinix00/hop-os-surf/stack/scene"
)

func main() {
	app := applib.Init()

	if _, err := appnet.Up(app); err != nil {
		app.Logf("net: %v", err)
		app.Exit(1)
	}
	addr := app.Env("SURF_ADDR")
	if addr == "" {
		app.Logf("calc: SURF_ADDR not set (want <display-node>:7878)")
		app.Exit(1)
	}

	conn := scene.Open(addr, fmt.Sprintf("calc @ slot %d", app.Slot), 240, 320, app.Logf)
	app.Logf("calc: %v", calc.Drive(conn, app.Logf))
	app.Exit(1)
}
