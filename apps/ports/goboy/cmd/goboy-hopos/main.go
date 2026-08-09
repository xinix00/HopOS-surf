//go:build tamago

// Goboy-hopos draait humpheh/goboy als HopOS-slot-app: een GameBoy in een
// slot, het scherm als SURF-window op de display-node — speelbaar vanuit elke
// browser via de web-KVM. Kill de node en laat HOP hem elders herstarten: het
// window komt vanzelf terug (wel bij het titelscherm, er is nog geen
// state-overdracht).
//
// Config uit de jobspec-env:
//
//	GOBOY_ROM   http-URL van de .gb/.gbc-ROM (verplicht; tamago heeft geen
//	            schijf, dus de cartridge komt over het net — serveer hem met
//	            een Content-Length, net als een app-artifact)
//	SURF_ADDR   display-node (host:poort); leeg = de eigen node (HOPOS_HOST:7878)
//
// Jobspec-schets:
//
//	{"name":"goboy","driver":"hop",
//	 "artifacts":[{"url":"…/goboy.elf"}],
//	 "env":{"GOBOY_ROM":"http://…/tetris.gb"}}
//
// Toetsen (in het KVM-venster): pijltjes=dpad, Z=A, X=B, Enter=Start,
// Backspace=Select, Escape=pauze, '='=DMG-palet.
package main

import (
	"fmt"
	"io"

	"goboy/internal/gbapp"

	"github.com/xinix00/HopOS/metal/app/applib"
	"github.com/xinix00/HopOS/metal/app/applib/appnet"
	"github.com/xinix00/hop-os-surf/stack/surf"
	"github.com/xinix00/hop-os-surf/stack/window"
	"github.com/xinix00/lean/leanhttp"
)

func main() {
	app := applib.Init()

	if _, err := appnet.Up(app); err != nil {
		app.Logf("net: %v", err)
		app.Exit(1)
	}
	addr := surf.Addr(app.Env) // SURF_ADDR, anders de eigen node (HOPOS_HOST:7878)
	if addr == "" {
		app.Logf("goboy: SURF_ADDR not set and no HOPOS_HOST (want <display-node>:7878)")
		app.Exit(1)
	}
	romURL := app.Env("GOBOY_ROM")
	if romURL == "" {
		app.Logf("goboy: GOBOY_ROM not set (want an http-URL to a .gb/.gbc rom)")
		app.Exit(1)
	}

	resp, err := leanhttp.Get(romURL)
	if err != nil {
		app.Logf("goboy: rom %s: %v", romURL, err)
		app.Exit(1)
	}
	rom, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		app.Logf("goboy: rom %s: read: %v", romURL, err)
		app.Exit(1)
	}
	app.Logf("goboy: rom %s (%d bytes)", romURL, len(rom))

	// Herkomst in de titel: het cluster hoort zichtbaar te zijn in de chrome.
	w, h := gbapp.Hint()
	win, err := window.Open(addr, fmt.Sprintf("goboy @ slot %d", app.Slot), w, h, app.Logf)
	if err != nil {
		app.Logf("goboy: open %s: %v", addr, err)
		app.Exit(1)
	}
	app.Logf("goboy: window open on %s", addr)

	app.Logf("goboy: %v", gbapp.Drive(win, rom, app.Logf))
	app.Exit(1)
}
