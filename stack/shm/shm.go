// Package shm is de dunne naad tussen de GUI-stack en de HopOS-surface-grant
// (docs/gui-ontwerp.md, fase P3): een app deelt zijn vensterbuffer met de
// display in plaats van elke frame de pixels over een socket te sturen.
//
// Waarom een eigen pakketje in plaats van applib rechtstreeks aanroepen: applib
// is tamago-only (het trekt cpu/memlimit en cpu/smp mee) en de hele GUI-stack is
// bewust op de ontwikkelmachine testbaar — surfserve draait daar integraal
// (window ↔ server ↔ compositor). Eén bouwtag-splitsing hier houdt dat heel,
// in plaats van in elk pakket dat de grant wil gebruiken.
//
// Buiten HopOS zijn beide functies een nette fout, en dat is precies goed: het
// grant-pad is een optimalisatie die stil terugvalt op pixels-over-de-socket.
package shm

import "errors"

// ErrNoGrant betekent: hier geen gedeeld geheugen. Een app op een andere node,
// een node zonder display, een kooi die het niet kan (RISC-V/PMP), of gewoon de
// ontwikkelmachine. Geen van die gevallen is een fout in de app.
var ErrNoGrant = errors.New("shm: no surface grant available")
