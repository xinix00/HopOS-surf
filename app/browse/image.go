package browse

import "image"

// scaleCover schaalt src beeldvullend naar w×h (aspect behouden, gecentreerd,
// de rest afgesneden) — background-size: cover, het hero/teaser-patroon.
func scaleCover(src image.Image, w, h int) *image.RGBA {
	sb := src.Bounds()
	sw, sh := sb.Dx(), sb.Dy()
	if sw < 1 || sh < 1 || w < 1 || h < 1 {
		return image.NewRGBA(image.Rect(0, 0, 1, 1))
	}
	// De bron-crop die (op schaal) precies w×h dekt.
	cw, ch := sw, sw*h/w
	if ch > sh || ch < 1 {
		ch = sh
		cw = sh * w / h
		if cw > sw {
			cw = sw
		}
	}
	if cw < 1 {
		cw = 1
	}
	ox, oy := sb.Min.X+(sw-cw)/2, sb.Min.Y+(sh-ch)/2
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		sy := oy + y*ch/h
		for x := 0; x < w; x++ {
			dst.Set(x, y, src.At(ox+x*cw/w, sy))
		}
	}
	return dst
}

// scaleTo schaalt src naar w×h met nearest-neighbor: geen extra dependency,
// en op het 8x8-font-scherm is zachte interpolatie toch niet te zien.
func scaleTo(src image.Image, w, h int) *image.RGBA {
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	sb := src.Bounds()
	for y := 0; y < h; y++ {
		sy := sb.Min.Y + y*sb.Dy()/h
		for x := 0; x < w; x++ {
			dst.Set(x, y, src.At(sb.Min.X+x*sb.Dx()/w, sy))
		}
	}
	return dst
}
