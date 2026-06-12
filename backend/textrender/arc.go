package textrender

import (
	"image"
	"image/draw"
	"math"
)

// arcWarp bends src into a circular arc spanning `deg` degrees across its width,
// mimicking ImageMagick's `-distort Arc`. The horizontal middle of the image
// becomes the top of the arch (middle highest, ends curving down). The whole
// composite (pill + text + shadow) bends together, so a curved title keeps its
// background banner. Areas outside the arc are transparent.
//
// deg > 0  → arch (middle highest).  deg < 0 → valley (middle lowest).
// |deg| < ~0.01 returns the source unchanged.
func arcWarp(src image.Image, deg float64) *image.RGBA {
	srcRGBA := toRGBA(src)
	W := srcRGBA.Bounds().Dx()
	H := srcRGBA.Bounds().Dy()

	A := math.Abs(deg) * math.Pi / 180
	if A < 1e-4 || W <= 0 || H <= 0 {
		return srcRGBA
	}

	// Valley = arch applied to a vertically-flipped source, flipped back.
	if deg < 0 {
		return flipV(arcWarp(flipV(srcRGBA), -deg))
	}

	// R0 = radius of the top edge; its arc length equals the source width.
	R0 := float64(W) / A
	halfA := A / 2

	widthOut := 2 * R0 * math.Sin(halfA)
	heightOut := R0 - (R0-float64(H))*math.Cos(halfA)
	outW := int(math.Ceil(widthOut))
	outH := int(math.Ceil(heightOut))
	if outW < 1 {
		outW = 1
	}
	if outH < 1 {
		outH = 1
	}

	cx := widthOut / 2 // circle center x (top of arch is straight above it)
	cy := R0           // top-center of the source maps to output y=0

	dst := image.NewRGBA(image.Rect(0, 0, outW, outH))
	for oy := 0; oy < outH; oy++ {
		for ox := 0; ox < outW; ox++ {
			dx := float64(ox) + 0.5 - cx
			dy := float64(oy) + 0.5 - cy
			r := math.Hypot(dx, dy)
			theta := math.Atan2(dx, -dy) // 0 = straight up (toward top of arch)
			sx := float64(W)/2 + theta/A*float64(W)
			sy := R0 - r
			if sx < 0 || sx >= float64(W) || sy < 0 || sy >= float64(H) {
				continue
			}
			sampleBilinear(dst, ox, oy, srcRGBA, sx, sy)
		}
	}
	return dst
}

// toRGBA returns src as an *image.RGBA anchored at (0,0).
func toRGBA(src image.Image) *image.RGBA {
	if r, ok := src.(*image.RGBA); ok && r.Bounds().Min.X == 0 && r.Bounds().Min.Y == 0 {
		return r
	}
	b := src.Bounds()
	out := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(out, out.Bounds(), src, b.Min, draw.Src)
	return out
}

// flipV returns a vertically mirrored copy.
func flipV(src *image.RGBA) *image.RGBA {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	out := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		srcRow := src.Pix[(h-1-y)*src.Stride : (h-1-y)*src.Stride+w*4]
		copy(out.Pix[y*out.Stride:y*out.Stride+w*4], srcRow)
	}
	return out
}

// sampleBilinear writes the bilinear sample of src at (fx, fy) into dst[ox,oy].
// image.RGBA stores ALPHA-PREMULTIPLIED channels, so the stored values are
// interpolated directly and written back as-is — converting to straight alpha
// (or re-multiplying) here double-applies alpha and shreds the anti-aliased
// glyph edges into the chalky speckle the editorial title used to show.
// The -0.5 centers the sample on pixel centers (output px ox covers
// [ox,ox+1), its center maps to source index space at sx-0.5).
func sampleBilinear(dst *image.RGBA, ox, oy int, src *image.RGBA, fx, fy float64) {
	W := src.Bounds().Dx()
	H := src.Bounds().Dy()
	fx -= 0.5
	fy -= 0.5
	x0 := int(math.Floor(fx))
	y0 := int(math.Floor(fy))
	tx := fx - float64(x0)
	ty := fy - float64(y0)
	x1 := x0 + 1
	y1 := y0 + 1
	if x1 > W-1 {
		x1 = W - 1
	}
	if y1 > H-1 {
		y1 = H - 1
	}
	if x0 < 0 {
		x0 = 0
	}
	if y0 < 0 {
		y0 = 0
	}

	r00, g00, b00, a00 := premul(src, x0, y0)
	r10, g10, b10, a10 := premul(src, x1, y0)
	r01, g01, b01, a01 := premul(src, x0, y1)
	r11, g11, b11, a11 := premul(src, x1, y1)

	r := bilerp(r00, r10, r01, r11, tx, ty)
	g := bilerp(g00, g10, g01, g11, tx, ty)
	bl := bilerp(b00, b10, b01, b11, tx, ty)
	a := bilerp(a00, a10, a01, a11, tx, ty)

	i := dst.PixOffset(ox, oy)
	dst.Pix[i+0] = clamp8(r * 255)
	dst.Pix[i+1] = clamp8(g * 255)
	dst.Pix[i+2] = clamp8(bl * 255)
	dst.Pix[i+3] = clamp8(a * 255)
}

// premul returns the (already premultiplied) components of src[x,y] in 0..1.
func premul(src *image.RGBA, x, y int) (r, g, b, a float64) {
	i := src.PixOffset(x, y)
	return float64(src.Pix[i+0]) / 255,
		float64(src.Pix[i+1]) / 255,
		float64(src.Pix[i+2]) / 255,
		float64(src.Pix[i+3]) / 255
}

func bilerp(v00, v10, v01, v11, tx, ty float64) float64 {
	top := v00 + (v10-v00)*tx
	bot := v01 + (v11-v01)*tx
	return top + (bot-top)*ty
}

func clamp8(v float64) uint8 {
	if v <= 0 {
		return 0
	}
	if v >= 255 {
		return 255
	}
	return uint8(v + 0.5)
}
