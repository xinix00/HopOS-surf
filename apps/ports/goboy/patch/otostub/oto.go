// Package oto is een lege stand-in voor github.com/hajimehoshi/oto v1: de
// echte is een cgo/platform-audiobibliotheek die niet voor GOOS=tamago
// bestaat, en SURF heeft (nog) geen audiokanaal om het geluid heen te dragen.
// Zolang de emulator zonder WithSound draait komt hier nooit een aanroep —
// alleen het type moet bestaan omdat de APU een *oto.Player-veld heeft.
// Alleen de door goboy gebruikte API is nagebootst.
package oto

// Context komt overeen met oto.NewContext(sampleRate, channels, bytesPerSample, bufferSize).
type Context struct{}

// NewContext geeft een context die nergens heen speelt.
func NewContext(sampleRate, channelNum, bitDepthInBytes, bufferSizeInBytes int) (*Context, error) {
	return &Context{}, nil
}

// NewPlayer geeft een speler die alles meteen "gespeeld" meldt.
func (c *Context) NewPlayer() *Player { return &Player{} }

// Player is de nergens-heen-speler.
type Player struct{}

// Write accepteert samples en gooit ze weg.
func (p *Player) Write(b []byte) (int, error) { return len(b), nil }

// Close is er voor de volledigheid.
func (p *Player) Close() error { return nil }
