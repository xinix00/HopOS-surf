# goboy on HopOS — the first port

[humpheh/goboy](https://github.com/Humpheh/goboy) (a GameBoy emulator in pure
Go) running as a HopOS slot app, its screen a SURF window on the display node —
playable from any browser through the web-KVM. Kill the node and let HOP
restart the job elsewhere: the window comes back by itself (at the title
screen — there is no state handover).

The point of this port is the **pattern**: upstream is never forked and never
pinned. `tools/prepare-goboy.sh` pulls the *latest* branch-HEAD, drops our
additions in, and the build tells us loudly when upstream moved under us. Same
recipe as `apps/cloudflared` next door (easy/hop), with the pin swapped for
a latest-pull.

```
5.8 MB  out/goboy.elf   (arm64, apphttp-clean: no crypto/tls in the image)
```

## What it took

| piece | what | lines changed upstream |
|---|---|---|
| `patch/gb/fromdata.go` | `gb.NewFromData` — the ROM as bytes instead of a file path (tamago has no filesystem). Copied into `pkg/gb/` by the prepare script; upstream-PR candidate | 0 (additive) |
| `patch/otostub/` | empty stand-in for `hajimehoshi/oto` (cgo audio, doesn't exist for tamago) via a `replace` — SURF has no audio channel and the APU never touches the player while sound is off | 0 |
| `internal/gbapp/` | the SURF side: 60 frames/s `PreparedData` → window (integer-scaled, centered, dirty-box presents), KVM key events → `ProcessInput` | — |

## Build & run locally

```sh
tools/prepare-goboy.sh   # pulls the latest goboy HEAD (rerun to update)
tools/build.sh           # host tests (runs the emulator core!) + out/goboy.elf
```

Local play needs no hardware at all — SURF is network-transparent, so the same
Drive draws on the host desktop:

```sh
go run ./cmd/desktop &                      # in the repo root: KVM on :8088
go run ./cmd/goboy-hopos path/to/rom.gb     # here; -surf host:port for elsewhere
```

Open http://127.0.0.1:8088/kvm and click the goboy window. Keys mirror goboy's
own binding: **arrows** = dpad, **Z** = A, **X** = B, **Enter** = Start,
**Backspace** = Select; Escape pauses, `=` cycles the DMG palette. Free test
carts live in `build/goboy-latest/roms/` (blargg, mooneye); the emulator passes
blargg's `cpu_instrs` on the desktop.

## Run it on the cluster

The ROM comes over plain HTTP at boot (`apphttp.Get` — serve it with a
`Content-Length`, like an app artifact):

```json
{"name":"goboy","driver":"hop",
 "artifacts":[{"url":"…/goboy.elf"}],
 "env":{"GOBOY_ROM":"http://…/tetris.gb"}}
```

`SURF_ADDR` picks the display node; unset it and the window appears on the
app's own node (`HOPOS_HOST:7878`).

## Limits

- **No sound.** SURF has no audio channel (yet); the APU runs with output
  disabled, which costs nothing (`Buffer` returns immediately).
- **No battery saves on the device.** The cart's save loop wants a disk; on
  tamago the writes fail silently. On the host desktop saves *do* land next to
  the ROM as `<rom>.sav`.
- **Input is keyboard-only** — whatever the web-KVM forwards (JS keyCodes).

## Why latest instead of a pin

goboy is a hobby project without a release cadence; the interesting commits
land on the branch. The trade: `go.mod`/`go.sum` may shift after a prepare run
(commit what `go mod tidy` makes of it), and an upstream change can break the
build — which is exactly the signal we want, at prepare time, not months later.
The gate is real: `tools/build.sh` runs the emulator core on the host (a frame
of NOPs through `NewFromData`) before anything gets linked for tamago.
