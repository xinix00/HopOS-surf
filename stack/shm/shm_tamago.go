//go:build tamago

package shm

import "github.com/xinix00/HopOS/metal/app/applib"

// Grant vraagt HopOS om een vensterbuffer van n bytes die de display-houder
// read-only mag lezen. Geeft de buffer en het adres waarop de DISPLAY hem ziet
// — dat getal stuurt de app zelf door in een MAP-bericht.
//
// De buffer is groter dan gevraagd: HopOS verleent per 2MB-blok. Dat is geen
// verspilling die je hier moet compenseren — gebruik de eerste n bytes.
func Grant(n int) (pix []byte, ipa uint64, err error) {
	a := applib.Self()
	if a == nil {
		return nil, 0, ErrNoGrant
	}
	s, err := a.GrantSurface(n)
	if err != nil {
		return nil, 0, err
	}
	return s.Pix[:n], s.IPA, nil
}

// View is de andere kant: de display maakt van een doorgestuurd adres een
// leesbare slice. Het adres komt uit een ánder proces, dus applib toetst het
// tegen het surface-venster vóór er iets van gelezen wordt.
func View(ipa uint64, n int) ([]byte, error) { return applib.ViewSurface(ipa, n) }
