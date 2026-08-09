// Desktop draait de hele SURF-desktop als één host-proces — geen QEMU, geen
// device: gewoon `go run ./cmd/desktop` en de web-KVM op http://127.0.0.1:8088/kvm
// (Derek 20-07: "we gebruiken toch niet de display-out maar de KVM-output").
//
// Binnenin is het exact de echte stack: dezelfde compositor, dezelfde
// surfserve en dezelfde app-Drives als op een node — alleen het tamago-bootje
// (netstack, slots) is vervangen door goroutines. Het startmenu start en
// stopt de apps in-proces; taskman kijkt naar een echt cluster zodra
// HOP_ADDR is gezet (HOP_KEY optioneel), en SURF_HOME is de browser-start.
package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/xinix00/hop-os-surf/app/browse"
	"github.com/xinix00/hop-os-surf/app/calc"
	"github.com/xinix00/hop-os-surf/app/clock"
	"github.com/xinix00/hop-os-surf/app/hopapi"
	"github.com/xinix00/hop-os-surf/app/launcher"
	"github.com/xinix00/hop-os-surf/app/taskman"
	"github.com/xinix00/hop-os-surf/app/ui"
	"github.com/xinix00/hop-os-surf/stack/compositor"
	"github.com/xinix00/hop-os-surf/stack/scene"
	"github.com/xinix00/hop-os-surf/stack/surf"
	"github.com/xinix00/hop-os-surf/stack/surfserve"
	"github.com/xinix00/hop-os-surf/stack/window"
	"github.com/xinix00/lean/leanhttp"
)

func main() {
	surfAddr := flag.String("surf", "127.0.0.1:7878", "SURF-listener (apps verbinden hier)")
	httpAddr := flag.String("http", "127.0.0.1:8088", "web-KVM (open /kvm in de browser)")
	flag.Parse()
	logf := log.Printf

	comp := compositor.New(1280, 720)
	srv := surfserve.New(comp, logf)
	ln, err := net.Listen("tcp", *surfAddr)
	if err != nil {
		log.Fatalf("desktop: surf-listener %s: %v", *surfAddr, err)
	}
	go srv.ServeSURF(ln)
	go func() { log.Fatal(leanhttp.ListenAndServe(*httpAddr, srv.Handler())) }()
	logf("desktop: KVM op http://%s/kvm — apps via %s", *httpAddr, *surfAddr)

	// Taskman is de vaste kijker op het cluster: altijd aan zodra er een HOP
	// is om naar te kijken; zonder HOP_ADDR bestaat hij simpelweg niet.
	if hop := os.Getenv("HOP_ADDR"); hop != "" {
		if !strings.Contains(hop, "://") {
			hop = "http://" + hop
		}
		client := &hopapi.Client{Base: hop, Key: os.Getenv("HOP_KEY")}
		conn := scene.Open(*surfAddr, "taskman @ host", 480, 360, logf)
		go func() { logf("taskman: %v", taskman.Drive(conn, client, logf)) }()
		logf("desktop: taskman kijkt naar %s", hop)
	}

	runLauncher(*surfAddr, logf)
	select {} // de server en de apps zijn het proces
}

// runLauncher is het startmenu met de host-catalogus: dezelfde launcher.Menu
// als op het device, maar een start is een goroutine in plaats van een job —
// en de "clusterstatus" is gewoon wat er in dit proces draait. Stoppen doet
// het rode stoplichtje: elke instantie meldt zijn eigen einde op doneC, en
// meerdere van hetzelfde mag (Derek 21-07).
func runLauncher(surfAddr string, logf func(string, ...any)) {
	doneC := make(chan int, 8) // instantie-einde: nooit lossy, altijd gedraind

	// Een starter zet één instantie op en meldt via doneC wanneer die sterft
	// (Drive keert terug op de WM-CLOSE; calc's leeslus vuurt OnClose).
	starters := map[string]func(id int, title string){
		"clock": func(id int, title string) {
			win, err := window.Open(surfAddr, title, clock.Size, clock.Size, logf)
			if err != nil {
				doneC <- id
				return
			}
			go func() { logf("clock: %v", clock.Drive(win, logf)); doneC <- id }()
		},
		"browser": func(id int, title string) {
			win, err := window.Open(surfAddr, title, 480, 360, logf)
			if err != nil {
				doneC <- id
				return
			}
			go func() { logf("browser: %v", browse.Drive(win, os.Getenv("SURF_HOME"), logf)); doneC <- id }()
		},
		"calc": func(id int, title string) {
			conn := scene.Open(surfAddr, title, 240, 320, logf)
			go func() { logf("calc: %v", calc.Drive(conn, logf)); doneC <- id }()
		},
		"goboy": func(id int, title string) {
			// De eerste port (apps/ports/goboy) is een eigen module en start
			// daarom als subprocess — zoals hij op het cluster een eigen job
			// is; het window komt toch via SURF binnen. Vergt een gedraaide
			// tools/prepare-goboy.sh en de desktop in de repo-wortel. De ROM
			// komt uit GOBOY_ROM (pad op de host), anders blargg's testcart
			// uit de upstream-checkout.
			rom := os.Getenv("GOBOY_ROM")
			if rom == "" {
				// roms/ is de cartridge-la van de port (gitignored: images
				// gaan nooit mee online): alfabetisch de eerste .gb/.gbc,
				// en zonder cartridge blargg's testcart uit de upstream-
				// checkout. GOBOY_ROM wint altijd.
				if m, _ := filepath.Glob("apps/ports/goboy/roms/*.gb*"); len(m) > 0 {
					rom = m[0]
				} else {
					rom = "apps/ports/goboy/build/goboy-latest/roms/blargg/cpu_instrs.gb"
				}
			}
			if abs, err := filepath.Abs(rom); err == nil {
				rom = abs // het subprocess draait in de port-map, niet in de desktop-cwd
			}
			cmd := exec.Command("go", "run", "./cmd/goboy-hopos", "-surf", surfAddr, rom)
			cmd.Dir = "apps/ports/goboy"
			cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
			go func() {
				if err := cmd.Run(); err != nil {
					logf("goboy: %v (prepare-goboy.sh gedraaid? desktop in de repo-wortel gestart?)", err)
				}
				doneC <- id
			}()
		},
	}
	apps := []launcher.App{{Name: "clock"}, {Name: "browser"}, {Name: "calc"}, {Name: "goboy"}}

	conn := scene.Open(surfAddr, "launcher @ host", 320, 420, logf)
	conn.Role = surf.RoleMenu
	m := launcher.NewMenu(conn, apps)

	refreshC := make(chan struct{}, 1)
	actC := make(chan string, 2)
	m.Refresh = func() { ui.PostLatest(refreshC, struct{}{}) }
	m.OnStart = func(a launcher.App) { ui.PostLatest(actC, a.Name) }
	conn.OnKey = m.Key
	if err := m.Start(); err != nil {
		logf("launcher: %v", err)
		return
	}

	// De worker bezit running/seq alleen zelf: alle mutaties via de kanalen.
	go func() {
		running := map[int]string{} // instantie-id → appnaam
		seq := map[string]int{}     // volgnummer per app (voor de titel)
		nextID := 0
		status := func() hopapi.Status {
			placed := map[string]int{}
			for _, name := range running {
				placed[name]++
			}
			return hopapi.Status{
				ClusterName: "host (go run)",
				Agents:      1,
				Jobs:        len(placed),
				TotalPlaced: len(running),
				Placed:      placed,
			}
		}
		m.SetData(status())
		for {
			select {
			case <-refreshC:
			case name := <-actC:
				starter := starters[name]
				if starter == nil {
					break
				}
				nextID++
				seq[name]++
				title := name + " @ host"
				if seq[name] > 1 {
					title = fmt.Sprintf("%s #%d @ host", name, seq[name])
				}
				running[nextID] = name
				starter(nextID, title)
				logf("launcher: %s gestart (%s)", name, title)
			case id := <-doneC:
				logf("launcher: %s gestopt (rood stoplichtje)", running[id])
				delete(running, id)
			}
			m.SetData(status())
		}
	}()
}
