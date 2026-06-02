package textrender

import (
	"image"
	"image/draw"

	"github.com/disintegration/imaging"
	"github.com/fogleman/gg"
)

// drawShadow paints `paint` into a transparent buffer the same size as dc,
// applies Gaussian blur with sigma=shadow.Blur, tints it shadow.Color@Alpha,
// then composites onto dc at (offsetX, offsetY).
//
// `paint` should draw the silhouette of the element (text + pill) onto its
// own *gg.Context — we only use the result's alpha channel.
func drawShadow(dc *gg.Context, shadow *ShadowStyle, paint func(*gg.Context)) {
	if shadow == nil || shadow.Blur <= 0 && shadow.Alpha <= 0 {
		return
	}
	w, h := dc.Width(), dc.Height()

	// 1. Paint silhouette into a fresh buffer
	sil := gg.NewContext(w, h)
	paint(sil)

	// 2. Convert to tinted alpha-only image: replace RGB with shadow color,
	//    multiply alpha by shadow.Alpha.
	shadowColor := withAlpha(parseHexColor(shadow.Color), shadow.Alpha)
	src := sil.Image().(*image.RGBA)
	tinted := image.NewRGBA(src.Bounds())
	for i := 0; i < len(src.Pix); i += 4 {
		a := src.Pix[i+3]
		if a == 0 {
			continue
		}
		tinted.Pix[i+0] = shadowColor.R
		tinted.Pix[i+1] = shadowColor.G
		tinted.Pix[i+2] = shadowColor.B
		tinted.Pix[i+3] = uint8(float64(a) * float64(shadowColor.A) / 255)
	}

	// 3. Blur
	var blurred *image.NRGBA
	if shadow.Blur > 0 {
		blurred = imaging.Blur(tinted, shadow.Blur)
	} else {
		blurred = imaging.Clone(tinted)
	}

	// 4. Composite onto dc at offset
	dst := dc.Image().(*image.RGBA)
	srcBounds := blurred.Bounds()
	dstPoint := image.Pt(int(shadow.OffsetX), int(shadow.OffsetY))
	draw.Draw(dst, srcBounds.Add(dstPoint), blurred, image.Point{}, draw.Over)
}
