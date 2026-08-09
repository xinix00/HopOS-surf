package surfserve

// De HTTP-kant draait op apphttp (geen net/http meer in het display-image),
// dus httptest.NewServer kan de Handler niet meer aannemen. Dit is dezelfde
// afspraak in tien regels: een echte listener op een vrije poort, .URL en
// .Close — en het pad dat de tests aflopen is nu precies het pad van de echte
// display, inclusief onze eigen request-parser.

import (
	"net"
	"testing"

	"github.com/xinix00/lean/leanhttp"
)

type webTest struct {
	URL string
	ln  net.Listener
}

func (w *webTest) Close() { w.ln.Close() }

func newWeb(t *testing.T, s *Server) *webTest {
	t.Helper()
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("web-listener: %v", err)
	}
	go leanhttp.Serve(ln, s.Handler())
	return &webTest{URL: "http://" + ln.Addr().String(), ln: ln}
}
