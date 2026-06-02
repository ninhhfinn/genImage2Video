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
}
