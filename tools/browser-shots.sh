#!/bin/sh
# Expliciete visuele browsermeting. De gewone testpoort blijft hermetisch;
# deze run schrijft het fixture-contactvel en haalt daarna echte sites op.
set -e
cd "$(dirname "$0")/.."

SPEC_SHEET=1 go test ./app/browse -run '^TestSpec$'
LIVE_SITES=1 go test ./app/browse -run '^TestScreenshotSites$'

echo "OK: docs/browser-spec.png en live browser-schoten ververst" >&2
