// De goboy-port: humpheh/goboy (een GameBoy-emulator in pure Go) als
// HopOS-slot-app met zijn scherm als SURF-window. Een eigen module binnen de
// surf-repo, net als apps/cloudflared bij de buurman (easy/hop): een port
// linkt andermans dependency-graaf en die hoort niet in de graaf van de
// hoofdmodule. Nested modules vallen buiten `./...`, dus tools/test.sh in de
// wortel ziet dit niet.
//
// Drie soorten replaces, en alle drie moeten:
//
//  1. Humpheh/goboy => ./build/goboy-latest — áltijd de nieuwste upstream,
//     geen pin: tools/prepare-goboy.sh clonet de default branch en legt onze
//     additieve patch (pkg/gb/fromdata.go) erin. Tot je dat script draait
//     faalt élk go-commando in deze module met "replacement directory does
//     not exist". Dat is de prijs van niet-forken; zie de README.
//  2. hajimehoshi/oto => ./patch/otostub — goboy's audio-uitgang is een
//     cgo/platform-bibliotheek die niet voor tamago bestaat. SURF heeft geen
//     audiokanaal en de APU raakt de speler nooit aan zolang geluid uitstaat
//     (WithSound ontbreekt), dus een lege stand-in met dezelfde API volstaat —
//     nul regels diff in upstream.
//  3. hop-os-surf => ../../.. — de moederrepo zelf, dus per definitie lokaal.
//     HopOS/metal heeft géén replace: echte GitHub-dep (metal/vX-tag,
//     GitHub in de lead), net als in de wortel-go.mod.
module goboy

go 1.26.4

require (
	github.com/Humpheh/goboy v0.0.0
	github.com/xinix00/HopOS/metal v1.8.4
	github.com/xinix00/hop-os-surf v0.0.0
)

require (
	github.com/google/btree v1.1.2 // indirect
	github.com/hajimehoshi/oto v1.0.1 // indirect
	github.com/soypat/lneto v0.1.1-0.20260609173350-82f946154800 // indirect
	github.com/usbarmory/go-net v0.0.0-20260626130943-dad9ef39fd9b // indirect
	github.com/usbarmory/tamago v1.26.4 // indirect
	golang.org/x/sys v0.45.0 // indirect
	golang.org/x/time v0.14.0 // indirect
	gvisor.dev/gvisor v0.0.0-20250911055229-61a46406f068 // indirect
)

replace (
	github.com/Humpheh/goboy => ./build/goboy-latest
	github.com/hajimehoshi/oto => ./patch/otostub
	github.com/xinix00/hop-os-surf => ../../..
)
