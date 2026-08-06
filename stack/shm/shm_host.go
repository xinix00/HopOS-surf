//go:build !tamago

package shm

// Op de ontwikkelmachine is er geen HopOS en dus geen grant. De GUI-stack valt
// terug op de pixelweg, en juist daarom blijven de host-tests representatief:
// ze dekken het pad dat óók op een echte node gebruikt wordt zodra de app en de
// display niet op dezelfde node draaien.

func Grant(n int) ([]byte, uint64, error) { return nil, 0, ErrNoGrant }

func View(ipa uint64, n int) ([]byte, error) { return nil, ErrNoGrant }
