#!/bin/sh
# Host-tests + tamago-compile-gate voor de SURF-stack (zelfde recept als
# HopOS' tools/test.sh: logica host-getest, mains door de tamago-gate).
#
# Extra argumenten gaan naar go test door: tools/test.sh -run EndToEnd -v
set -e
cd "$(dirname "$0")/.."

# De mains (cmd/*) kunnen niet op de host: applib is tamago-only. De
# bibliotheek-packages wel — inclusief de end-to-end-keten in surfserve.
go test "$@" ./stack/... ./app/...

# De host-desktop (go run ./cmd/desktop) is de enige host-main: meebouwen.
go build -o /dev/null ./cmd/desktop

TAMAGO="${TAMAGO:-$HOME/tamago-go/bin/go}"
if [ ! -x "$TAMAGO" ]; then
	echo "tamago-gate OVERGESLAGEN ($TAMAGO ontbreekt)" >&2
	exit 0
fi
mkdir -p out
# Canonieke app-link (zie HopOS docs/app.md): één artifact voor elk slot.
#
# nodefaultstack is GEEN optie maar een eis: zonder die tag linkt go-net zijn
# gVisor-implementatie onvoorwaardelijk mee (gvisor.go), naast de lneto-stack
# die appnet daadwerkelijk opzet. Gemeten 09-08 op display.elf: 3649
# gvisor-symbolen naast 671 lneto-symbolen, en de apps waren er 5-15% groter
# van. HopOS' eigen tools/test.sh gebruikt de tag al overal — deze gate liep
# achter, en dat is precies waarom de apps de footprint-winst misliepen.
#
# De lnetonet-variant is hier weg: die tag koos ooit lneto boven gVisor in
# appnet, en sinds de flip is lneto de enige implementatie (up_lneto.go draagt
# geen build-tag meer). Hij bouwde dus twee keer hetzelfde.
for app in display clock calc browser taskman dash launcher; do
	GOWORK=off GOTOOLCHAIN=local GOOS=tamago GOOSPKG=github.com/usbarmory/tamago GOARCH=arm64 \
		"$TAMAGO" build -tags "linkcpuinit nodefaultstack" -trimpath \
		-ldflags "-w -T 0x50010000 -R 0x1000" -o "out/$app.elf" "./cmd/$app"
done
echo "OK: host-tests groen, out/{display,clock,calc,browser,taskman,dash,launcher}.elf gebouwd" >&2
