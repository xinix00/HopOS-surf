#!/bin/sh
# Host-tests + het slot-image, zelfde recept als tools/test.sh in de wortel:
# logica host-getest, de main door de tamago-gate. Vereist een klaargezette
# upstream (tools/prepare-goboy.sh — eenmalig, of om bij te werken naar de
# nieuwste HEAD).
set -e
cd "$(dirname "$0")/.."

if [ ! -d build/goboy-latest ]; then
	echo "!! build/goboy-latest ontbreekt — draai eerst tools/prepare-goboy.sh" >&2
	exit 1
fi

# De host-tests draaien de emulatorkern écht (een frame NOP's) — de gate die
# bewijst dat de verse upstream-HEAD nog met onze patch samenwerkt. patch/
# blijft buiten schot: die bestanden bouwen alleen ín de upstream-boom.
GOWORK=off go test ./internal/...
GOWORK=off go vet ./internal/... ./cmd/...
GOWORK=off go build -o /dev/null ./cmd/goboy-hopos # de host-main

TAMAGO="${TAMAGO:-$HOME/tamago-go/bin/go}"
if [ ! -x "$TAMAGO" ]; then
	echo "tamago-gate OVERGESLAGEN ($TAMAGO ontbreekt)" >&2
	exit 0
fi
mkdir -p out
# Canonieke app-link (zie HopOS docs/app.md): één artifact voor elk slot.
printf "  %-12s" "goboy.elf"
GOWORK=off GOTOOLCHAIN=local GOOS=tamago GOOSPKG=github.com/usbarmory/tamago GOARCH=arm64 \
	"$TAMAGO" build -tags linkcpuinit -trimpath \
	-ldflags "-w -T 0x50010000 -R 0x1000" -o out/goboy.elf ./cmd/goboy-hopos
du -h out/goboy.elf | cut -f1
# De lnetonet-variant moet blijven bouwen (opt-in netstack, zie HopOS).
GOWORK=off GOTOOLCHAIN=local GOOS=tamago GOOSPKG=github.com/usbarmory/tamago GOARCH=arm64 \
	"$TAMAGO" build -tags "lnetonet linkcpuinit" -o /dev/null ./cmd/goboy-hopos

echo "OK: host-tests groen, out/goboy.elf gebouwd" >&2
