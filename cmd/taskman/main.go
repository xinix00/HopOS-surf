// Taskman is de HOP-taskmanager — sinds 20-07 een scene-app (P2): de app
// stuurt bomen en PATCHes, de display rendert; en mét de weg helemaal
// omlaag: cluster → job → task → live logstaart (SSE), waar elke logregel
// één PATCH van regel-lengte is. HOP_ADDR mag naar elke agent wijzen (die
// proxyt /v1 door), HOP_KEY is de cluster-API-key (leeg = geen auth).
//
// De app zelf is taskman.Drive — deze main is alleen het tamago-bootje
// (netstack, env, exit); de host-desktop (cmd/desktop) rijdt dezelfde Drive.
package main

import (
	"fmt"
	"strings"

	"hop-os/metal/app/applib"
	"hop-os/metal/app/applib/appnet"

	"github.com/xinix00/hop-os-surf/app/hopapi"
	"github.com/xinix00/hop-os-surf/app/taskman"
	"github.com/xinix00/hop-os-surf/stack/scene"
	"github.com/xinix00/hop-os-surf/stack/surf"
)

func main() {
	app := applib.Init()

	if _, err := appnet.Up(app); err != nil {
		app.Logf("net: %v", err)
		app.Exit(1)
	}
	addr := surf.Addr(app.Env) // SURF_ADDR, anders de eigen node (HOPOS_HOST:7878)
	if addr == "" {
		app.Logf("taskman: SURF_ADDR not set and no HOPOS_HOST (want <display-node>:7878)")
		app.Exit(1)
	}
	// De agent woont op élke node op het vaste interne adres; HOP_ADDR is
	// alleen nodig om een ándere node aan te wijzen.
	hopAddr := app.Env("HOP_ADDR")
	if hopAddr == "" {
		hopAddr = "10.100.0.1:8080"
	}
	if !strings.Contains(hopAddr, "://") {
		hopAddr = "http://" + hopAddr
	}
	client := &hopapi.Client{Base: hopAddr, Key: app.Env("HOP_KEY")}

	conn := scene.Open(addr, fmt.Sprintf("taskman @ slot %d", app.Slot), 480, 360, app.Logf)
	app.Logf("taskman: scene on %s, hop on %s", addr, hopAddr)

	app.Logf("taskman: %v", taskman.Drive(conn, client, app.Logf))
	app.Exit(1)
}
