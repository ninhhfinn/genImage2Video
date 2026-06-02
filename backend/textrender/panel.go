package textrender

import (
	"bytes"
	"fmt"
	"image/png"

	"github.com/fogleman/gg"
)

// RenderPanel draws a single semi-transparent rounded rectangle filling a
// transparent PNG of size w×h. Used as a "scrim" behind a text stack so the
// text stays readable over busy/bright photos.
func RenderPanel(w, h int, hexColor string, alpha, radius float64) ([]byte, error) {
	if w < 1 || h < 1 {
		return nil, fmt.Errorf("textrender: bad panel size %dx%d", w, h)
	}
	dc := gg.NewContext(w, h)
	dc.SetRGBA(0, 0, 0, 0)
	dc.Clear()

	c := withAlpha(parseHexColor(hexColor), alpha)
	dc.SetRGBA(float64(c.R)/255, float64(c.G)/255, float64(c.B)/255, float64(c.A)/255)

	r := radius
	if r > float64(w)/2 {
		r = float64(w) / 2
	}
	if r > float64(h)/2 {
		r = float64(h) / 2
	}
	if r < 0 {
		r = 0
	}
	dc.DrawRoundedRectangle(0, 0, float64(w), float64(h), r)
	dc.Fill()

	var buf bytes.Buffer
	if err := png.Encode(&buf, dc.Image()); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
