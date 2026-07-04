package textrender

import "github.com/fogleman/gg"

// drawPill draws a rounded-rectangle background on dc at (x, y, w, h)
// using the colors/alpha/radius from BgStyle.
func drawPill(dc *gg.Context, bg *BgStyle, x, y, w, h float64) {
	c := withAlpha(parseHexColor(bg.Color), bg.Alpha)
	dc.SetRGBA(
		float64(c.R)/255,
		float64(c.G)/255,
		float64(c.B)/255,
		float64(c.A)/255,
	)
	r := bg.Radius
	if r > w/2 {
		r = w / 2
	}
	if r > h/2 {
		r = h / 2
	}
	if r < 0 {
		r = 0
	}
	dc.DrawRoundedRectangle(x, y, w, h, r)
	dc.Fill()

	// Optional hairline border, traced fully inside the panel so it can't be
	// clipped by a zero-margin canvas. Inset by half the stroke width.
	if bg.BorderWidth > 0 && bg.BorderColor != "" {
		ba := bg.BorderAlpha
		if ba <= 0 {
			ba = 1
		}
		bc := withAlpha(parseHexColor(bg.BorderColor), ba)
		dc.SetRGBA(
			float64(bc.R)/255,
			float64(bc.G)/255,
			float64(bc.B)/255,
			float64(bc.A)/255,
		)
		in := bg.BorderWidth / 2
		br := r - in
		if br < 0 {
			br = 0
		}
		dc.SetLineWidth(bg.BorderWidth)
		dc.DrawRoundedRectangle(x+in, y+in, w-2*in, h-2*in, br)
		dc.Stroke()
	}
}
