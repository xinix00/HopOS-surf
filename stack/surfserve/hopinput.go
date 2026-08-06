package surfserve

import (
	"bufio"
	"encoding/json"
	"net"
	"time"
)

// De fysieke invoerweg: toetsenbord en muis die aan de node zélf hangen.
//
// HopOS bezit de USB-controller (een xHCI is een bus-master en er is geen
// IOMMU, dus dat blok kan niet in een kooi), leest de HID-rapporten en serveert
// ze als stroom. Het adres komt mee met de framebuffer-grant: wie het glas
// krijgt, krijgt het toetsenbord erbij — één zitplaats, één overdracht. In de
// env heet dat INPUT_ADDR, naast de FB_*-velden.
//
// HET IS DEZELFDE TAAL ALS DE BROWSER. Elke regel is precies het JSON-object
// dat POST /input al aanneemt, en het gaat door dezelfde inputMsg.event() en
// dezelfde s.Input(). Een echt toetsenbord is dus niet te onderscheiden van de
// KVM-pagina — dat was de hele reden om HopOS geen eigen invoer-ABI te laten
// verzinnen, en het betekent ook dat één bugfix beide paden raakt.
//
// Coördinaten zijn absoluut: HopOS houdt de cursor bij (een USB-muis stuurt
// deltas, een canvas niet) en clamped hem op de schermmaat van de framebuffer.
// Deze kant hoeft er dus niets aan te rekenen.

// hopInputRetry is hoe lang we wachten voor we opnieuw bellen. HopOS staat er
// al vóór de app start, dus in de praktijk lukt de eerste poging; dit dekt een
// herstart van de node-kant en een display die eerder op is dan de USB-scan.
const hopInputRetry = 2 * time.Second

// ConsumeInput belt HopOS' invoerstroom en voert alles wat binnenkomt de
// gewone input-routering in. Blokkeert; start hem als goroutine. Lege addr =
// dit board serveert geen fysieke invoer (geen USB, of headless) en dan doet
// deze functie niets — geen lus, geen log.
//
// Nooit ophouden: het toetsenbord mag niet stilletjes verdwijnen omdat de
// verbinding één keer wegviel.
func (s *Server) ConsumeInput(addr string) {
	if addr == "" {
		return
	}
	for {
		c, err := net.DialTimeout("tcp", addr, 5*time.Second)
		if err != nil {
			time.Sleep(hopInputRetry)
			continue
		}
		s.readInput(c)
		c.Close()
		time.Sleep(hopInputRetry)
	}
}

// readInput leest regels tot de verbinding valt. Een onleesbare regel is geen
// reden om op te hangen — dan is er één toetsaanslag weg in plaats van het
// hele toetsenbord.
func (s *Server) readInput(c net.Conn) {
	sc := bufio.NewScanner(c)
	for sc.Scan() {
		var m inputMsg
		if err := json.Unmarshal(sc.Bytes(), &m); err != nil {
			continue
		}
		if ev, ok := m.event(); ok {
			s.Input(ev)
		}
	}
}
