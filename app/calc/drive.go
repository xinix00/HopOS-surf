package calc

import (
	"fmt"
	"sync"

	"github.com/xinix00/hop-os-surf/stack/scene"
)

// Drive is de complete calculator achter een scene-verbinding. Daarmee
// gebruiken de Tamago-main en de host-desktop exact dezelfde controller,
// net als browser, clock en taskman.
func Drive(conn *scene.Conn, logf func(string, ...any)) error {
	var mu sync.Mutex
	var c Calc
	var display *scene.Node

	press := func(key byte) {
		mu.Lock()
		c.Press(key)
		line := Line(&c)
		conn.SetText(display, line)
		mu.Unlock()
		logf("calc: key %q -> %s", key, line)
	}

	root, disp := Tree(press)
	display = disp
	conn.OnKey = func(code uint32, down bool) {
		if down {
			if key := Key(code); key != 0 {
				press(key)
			}
		}
	}

	closed := make(chan struct{})
	var closeOnce sync.Once
	conn.OnClose = func() { closeOnce.Do(func() { close(closed) }) }
	if err := conn.Show(root); err != nil {
		return fmt.Errorf("calc: show: %w", err)
	}
	<-closed
	return nil
}
