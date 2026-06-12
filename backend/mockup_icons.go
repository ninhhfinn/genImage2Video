package main

import (
	"image/color"
	"math"

	"github.com/fogleman/gg"
)

// Vector icon helpers (monochrome, drawn directly on a gg context) used to
// reproduce the emoji/icons in the reference designs since the text renderer
// cannot draw colour emoji. All sizes are in px on the target canvas.

// drawPinIcon draws a location pin whose bounding box is (cx-w/2 .. cx+w/2,
// topY .. topY+h). col is the body colour; the hole is punched white.
func drawPinIcon(dc *gg.Context, cx, topY, h float64, col color.Color) {
	r := h * 0.34
	cy := topY + r
	dc.SetColor(col)
	dc.DrawCircle(cx, cy, r)
	dc.Fill()
	dc.MoveTo(cx-r*0.78, cy+r*0.5)
	dc.LineTo(cx, topY+h)
	dc.LineTo(cx+r*0.78, cy+r*0.5)
	dc.ClosePath()
	dc.Fill()
	dc.SetColor(color.NRGBA{255, 255, 255, 235})
	dc.DrawCircle(cx, cy, r*0.42)
	dc.Fill()
}

// drawStarIcon draws a filled 5-point star centred at (cx,cy) with outer radius r.
func drawStarIcon(dc *gg.Context, cx, cy, r float64, col color.Color) {
	dc.SetColor(col)
	dc.NewSubPath()
	for i := 0; i < 5; i++ {
		ao := -math.Pi/2 + float64(i)*2*math.Pi/5
		dc.LineTo(cx+r*math.Cos(ao), cy+r*math.Sin(ao))
		ai := ao + math.Pi/5
		dc.LineTo(cx+r*0.42*math.Cos(ai), cy+r*0.42*math.Sin(ai))
	}
	dc.ClosePath()
	dc.Fill()
}

// drawLeafIcon draws a small leaf (pointed oval + midrib) centred at (cx,cy),
// `size` tall, rotated by `deg` degrees.
func drawLeafIcon(dc *gg.Context, cx, cy, size, deg float64, col color.Color) {
	dc.Push()
	dc.RotateAbout(gg.Radians(deg), cx, cy)
	w := size * 0.55
	dc.SetColor(col)
	dc.MoveTo(cx, cy-size/2)
	dc.QuadraticTo(cx+w, cy, cx, cy+size/2)
	dc.QuadraticTo(cx-w, cy, cx, cy-size/2)
	dc.ClosePath()
	dc.Fill()
	// midrib
	dc.SetColor(color.NRGBA{255, 255, 255, 120})
	dc.SetLineWidth(size * 0.05)
	dc.MoveTo(cx, cy-size*0.42)
	dc.LineTo(cx, cy+size*0.42)
	dc.Stroke()
	dc.Pop()
}

// drawHouseIcon draws a simple house glyph in a size×size box centred at (cx,cy).
func drawHouseIcon(dc *gg.Context, cx, cy, size float64, col color.Color) {
	s := size
	dc.SetColor(col)
	// roof
	dc.MoveTo(cx-s*0.5, cy-s*0.05)
	dc.LineTo(cx, cy-s*0.5)
	dc.LineTo(cx+s*0.5, cy-s*0.05)
	dc.ClosePath()
	dc.Fill()
	// body
	dc.DrawRectangle(cx-s*0.36, cy-s*0.05, s*0.72, s*0.5)
	dc.Fill()
	// door (punch)
	dc.SetColor(color.NRGBA{255, 255, 255, 220})
	dc.DrawRectangle(cx-s*0.1, cy+s*0.12, s*0.2, s*0.33)
	dc.Fill()
}

// drawFishIcon draws a small cute fish (head pointing left, tail right) in the
// box (x..x+w, y..y+h). body is the fill colour.
func drawFishIcon(dc *gg.Context, x, y, w, h float64, body color.Color) {
	bw := w * 0.68 // body ellipse width
	cx := x + bw*0.5
	cy := y + h*0.5
	dc.SetColor(body)
	dc.DrawEllipse(cx, cy, bw*0.5, h*0.42)
	dc.Fill()
	// tail fin (triangle on the right)
	dc.MoveTo(x+bw*0.86, cy)
	dc.LineTo(x+w, y+h*0.12)
	dc.LineTo(x+w, y+h*0.88)
	dc.ClosePath()
	dc.Fill()
	// top fin
	dc.MoveTo(cx-bw*0.12, cy-h*0.40)
	dc.QuadraticTo(cx+bw*0.1, cy-h*0.64, cx+bw*0.34, cy-h*0.30)
	dc.ClosePath()
	dc.Fill()
	// eye
	dc.SetColor(color.NRGBA{255, 255, 255, 245})
	dc.DrawCircle(x+bw*0.26, cy-h*0.06, h*0.1)
	dc.Fill()
	dc.SetColor(color.NRGBA{40, 30, 20, 255})
	dc.DrawCircle(x+bw*0.24, cy-h*0.06, h*0.045)
	dc.Fill()
}

// drawDoorIcon draws a small open-door glyph (cam/red) in the box
// (x..x+w, y..y+h) — mimics the orange door emoji used next to prices.
func drawDoorIcon(dc *gg.Context, x, y, w, h float64, body, panel color.Color) {
	dc.SetColor(body)
	dc.DrawRoundedRectangle(x, y, w, h, w*0.12)
	dc.Fill()
	// inner panel
	dc.SetColor(panel)
	dc.DrawRoundedRectangle(x+w*0.16, y+h*0.12, w*0.68, h*0.76, w*0.08)
	dc.Fill()
	// knob
	dc.SetColor(body)
	dc.DrawCircle(x+w*0.7, y+h*0.5, w*0.07)
	dc.Fill()
}
