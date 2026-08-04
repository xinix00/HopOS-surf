//go:build !tamago

// Host-main — en anders dan cloudflared's host-stub een échte: SURF is
// netwerk-transparant, dus dezelfde Drive tekent net zo lief op de
// host-desktop. Start in de hoofdmodule `go run ./cmd/desktop`, open
// http://127.0.0.1:8088/kvm, en dan hier:
//
//	go run ./cmd/goboy-hopos pad/naar/rom.gb
//
// Alleen het bootje verschilt: de ROM komt van schijf in plaats van over
// HTTP, en applib (tamago-only) is vervangen door flag+log.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"goboy/internal/gbapp"

	"github.com/xinix00/hop-os-surf/stack/window"
)

func main() {
	surfAddr := flag.String("surf", "127.0.0.1:7878", "SURF-listener van de (host-)display")
	flag.Parse()
	if flag.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "gebruik: goboy-hopos [-surf host:poort] <rom.gb>")
		os.Exit(2)
	}
	rom, err := os.ReadFile(flag.Arg(0))
	if err != nil {
		log.Fatalf("goboy: %v", err)
	}

	w, h := gbapp.Hint()
	// ASCII-streepje: het 8x8-font van de titelbalk kent geen em-dash.
	title := "goboy @ host - " + filepath.Base(flag.Arg(0))
	win, err := window.Open(*surfAddr, title, w, h, log.Printf)
	if err != nil {
		log.Fatalf("goboy: open %s: %v", *surfAddr, err)
	}
	log.Fatalf("goboy: %v", gbapp.Drive(win, rom, log.Printf))
}
