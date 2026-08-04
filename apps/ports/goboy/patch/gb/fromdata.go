package gb

import (
	"fmt"

	"github.com/Humpheh/goboy/pkg/cart"
)

// NewFromData is New, maar met de ROM als bytes in plaats van een pad naar een
// bestand — voor omgevingen zonder bestandssysteem (HopOS/tamago: de ROM komt
// daar uit de jobspec, over HTTP). Puur additief en kandidaat voor een
// upstream-PR: het bytes-gat bestaat al half, cart.NewCart neemt al een
// []byte. name mag leeg zijn; met een naam probeert de cart battery-saves
// naast <name>.sav te lezen/schrijven (op tamago faalt dat stil, er is geen
// schijf).
func NewFromData(rom []byte, name string, opts ...GameboyOption) (*Gameboy, error) {
	if len(rom) < 0x150 {
		return nil, fmt.Errorf("rom too small: %d bytes (want at least the 0x150-byte header)", len(rom))
	}
	gameboy := Gameboy{}
	for _, opt := range opts {
		opt(&gameboy.options)
	}
	gameboy.setup()
	gameboy.memory.Cart = cart.NewCart(rom, name)
	gameboy.cgbMode = gameboy.options.cgbMode && gameboy.memory.Cart.GetMode()&cart.CGB != 0
	return &gameboy, nil
}
