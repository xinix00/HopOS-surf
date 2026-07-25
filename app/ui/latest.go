package ui

// PostLatest zet v op c zonder de aanroeper te blokkeren. Is het kanaal
// vol, dan vervangt de nieuwste waarde de oudste: geschikt voor refreshes
// en UI-intenties waarbij alleen de recentste stand nog betekenis heeft.
func PostLatest[T any](c chan T, v T) {
	select {
	case c <- v:
		return
	default:
	}
	select {
	case <-c:
	default:
		// Een ongebufferd kanaal zonder ontvanger kan niets onthouden.
		return
	}
	select {
	case c <- v:
	default:
		// Een andere producer won de race; de aanroeper blijft non-blocking.
	}
}
