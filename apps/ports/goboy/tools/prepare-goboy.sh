#!/bin/sh
# Zet een bouwbare goboy klaar in build/goboy-latest — áltijd de nieuwste
# upstream (default branch), geen pin. Zelfde patroon als prepare-cloudflared
# bij de buurman (easy/hop/apps/cloudflared), met twee verschillen:
#
#  1. LATEST in plaats van een gepinde versie: goboy is een hobbyproject
#     zonder release-cadans, dus we volgen de branch en laten dit script (en
#     daarna tools/build.sh) luid falen wanneer upstream iets breekt.
#     Daarom een git-clone en niet de modulecache: de proxy loopt achter op
#     de branch-HEAD.
#  2. De patch is puur additief: pkg/gb/fromdata.go (ROM uit bytes — tamago
#     heeft geen bestandssysteem). De audio-uitgang (oto, cgo) gaat niet met
#     een diff maar met een stub-module via de replace in go.mod — nul regels
#     wijziging in upstream, dus niets dat bij een nieuwe HEAD kan schuiven.
#
# Idempotent: gewoon opnieuw draaien om bij te werken naar de nieuwste HEAD.
#
#   tools/prepare-goboy.sh          # klaarzetten / bijwerken
#   tools/prepare-goboy.sh --clean  # weggooien
set -e
cd "$(dirname "$0")/.."

DEST="build/goboy-latest"
REPO="${GOBOY_REPO:-https://github.com/Humpheh/goboy}"

if [ "${1:-}" = "--clean" ]; then
	rm -rf build
	echo "OK: build/ weg (go-commando's in deze module falen tot je dit script weer draait)" >&2
	exit 0
fi

rm -rf "$DEST"
mkdir -p build
git clone --quiet --depth 1 "$REPO" "$DEST"
COMMIT=$(git -C "$DEST" rev-parse --short HEAD)
# Geen geneste repo in de werkboom laten slingeren; build/ is al gitignored.
rm -rf "$DEST/.git"
echo "goboy @ $COMMIT (branch-HEAD van $REPO)" >&2

# Onze additieve bestanden erin — bestaat er al een upstream-variant, dan is
# de patch mogelijk overbodig geworden (of botst hij): luid melden.
for pair in "patch/gb/fromdata.go:pkg/gb"; do
	file=${pair%%:*}
	dir=${pair##*:}
	target="$DEST/$dir/$(basename "$file")"
	if [ -e "$target" ]; then
		echo "!! $dir/$(basename "$file") bestaat al upstream — patch mogelijk overbodig, controleer" >&2
	fi
	cp "$file" "$target"
	echo "  + $dir/$(basename "$file")" >&2
done

# De verse HEAD kan andere requires hebben: go.sum bijtrekken. Dit raakt
# alleen deze module; wijzigt go.mod/go.sum, commit dat dan mee.
GOWORK=off GOFLAGS=-mod=mod go mod tidy

echo "OK: $DEST klaar — nu bouwt tools/build.sh (en go test ./...)" >&2
