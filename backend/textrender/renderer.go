package textrender

import (
	"bytes"
	"fmt"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"strings"

	"github.com/fogleman/gg"
	"github.com/golang/freetype/truetype"
)

// Render paints one styled text element to a transparent PNG.
// Returns nil (no error) when style.Text is empty — callers can skip silently.
func Render(style *ElementStyle, ctx *RenderContext) (*RenderedElement, error) {
	if style == nil || strings.TrimSpace(style.Text) == "" {
		return nil, nil
	}
	if ctx == nil || ctx.VideoWidth <= 0 || ctx.VideoHeight <= 0 {
		return nil, fmt.Errorf("textrender: invalid render context")
	}

	fontPath := filepath.Join(ctx.AssetsDir, "fonts", style.FontFile)
	if _, err := os.Stat(fontPath); err != nil {
		return nil, fmt.Errorf("textrender: font %s: %v", style.FontFile, err)
	}
	fontSize := style.SizePct * float64(ctx.VideoWidth)
	if fontSize < 6 {
		fontSize = 6
	}

	// ── 1. Measure on a 1x1 throwaway ──
	measure := gg.NewContext(1, 1)
	if err := measure.LoadFontFace(fontPath, fontSize); err != nil {
		return nil, fmt.Errorf("textrender: load %s: %v", fontPath, err)
	}
	maxLineW := 0.0
	if style.MaxWidthPct > 0 {
		maxLineW = style.MaxWidthPct * float64(ctx.VideoWidth)
	} else if !style.NoWrap {
		// No explicit width: wrap at a generous 0.90W so a genuinely over-wide
		// line wraps instead of running off-canvas. Fit-to-width callers set
		// NoWrap to keep their intentional single-line behavior.
		maxLineW = 0.90 * float64(ctx.VideoWidth)
	}
	lines := wrapText(measure, style.Text, maxLineW)
	textW, textH, lineH := measureLines(measure, lines, style.LineSpacing)

	// gg's line box under-reports glyphs whose ink rises above the nominal
	// ascent — Vietnamese stacked tone marks (ồ ắ ễ…) in Latin-metric fonts
	// like Playfair/Prata. drawLines puts the first baseline at lineH from the
	// box top, so anything taller clips at the canvas edge and the marks
	// silently vanish. Extend the box upward to the real ink top.
	extraTop := math.Max(0, glyphInkTop(fontPath, fontSize, style.Text)-lineH)
	textH += extraTop

	padV, padH := 0.0, 0.0
	if style.Bg != nil {
		padV = style.Bg.Padding[0]
		padH = style.Bg.Padding[1]
	}

	// Shadow extent: blur radius (≈3σ) + offset magnitude on each side
	shadowExtent := 0.0
	if style.Shadow != nil {
		extent := style.Shadow.Blur*3 + math.Max(math.Abs(style.Shadow.OffsetX), math.Abs(style.Shadow.OffsetY))
		if extent > shadowExtent {
			shadowExtent = extent
		}
	}

	// Stroke extends each glyph outward by its width on every side.
	strokeExtent := 0.0
	if style.Stroke != nil && style.Stroke.Width > 0 {
		strokeExtent = style.Stroke.Width
	}

	// Outer transparent margin must clear whichever effect reaches furthest.
	margin := math.Max(shadowExtent, strokeExtent)

	contentW := textW + padH*2
	contentH := textH + padV*2
	canvasW := int(math.Ceil(contentW + margin*2))
	canvasH := int(math.Ceil(contentH + margin*2))
	if canvasW < 1 {
		canvasW = 1
	}
	if canvasH < 1 {
		canvasH = 1
	}

	originX := margin // top-left of content box (pill or text bg)
	originY := margin

	// ── 2. Real canvas ──
	dc := gg.NewContext(canvasW, canvasH)
	dc.SetRGBA(0, 0, 0, 0)
	dc.Clear()

	if err := dc.LoadFontFace(fontPath, fontSize); err != nil {
		return nil, fmt.Errorf("textrender: load font (render): %v", err)
	}

	// ── 3. Shadow pass (composites silhouette of pill+text, blurred, before main draw) ──
	if style.Shadow != nil {
		drawShadow(dc, style.Shadow, func(sil *gg.Context) {
			sil.SetRGBA(1, 1, 1, 1) // any opaque color — we use alpha only
			if style.Bg != nil {
				r := style.Bg.Radius
				if r > contentW/2 {
					r = contentW / 2
				}
				if r > contentH/2 {
					r = contentH / 2
				}
				if r < 0 {
					r = 0
				}
				sil.DrawRoundedRectangle(originX, originY, contentW, contentH, r)
				sil.Fill()
			} else {
				_ = sil.LoadFontFace(fontPath, fontSize)
				drawLines(sil, lines, style, originX+padH, originY+padV+extraTop, textW)
			}
		})
	}

	// ── 4. Background pill ──
	if style.Bg != nil {
		drawPill(dc, style.Bg, originX, originY, contentW, contentH)
	}

	// ── 4b. Stroke outline: stamp the glyphs around a circle of radius=width in
	//    the stroke color, so the fill drawn next leaves only an outward ring. ──
	if strokeExtent > 0 {
		sc := parseHexColor(style.Stroke.Color)
		alpha := float64(sc.A) / 255
		if style.Stroke.Alpha > 0 {
			alpha *= style.Stroke.Alpha
		}
		dc.SetRGBA(float64(sc.R)/255, float64(sc.G)/255, float64(sc.B)/255, alpha)
		n := int(math.Ceil(2 * math.Pi * strokeExtent / 1.5))
		if n < 12 {
			n = 12
		}
		if n > 96 {
			n = 96
		}
		for i := 0; i < n; i++ {
			ang := 2 * math.Pi * float64(i) / float64(n)
			drawLines(dc, lines, style,
				originX+padH+strokeExtent*math.Cos(ang),
				originY+padV+extraTop+strokeExtent*math.Sin(ang), textW)
		}
	}

	// ── 5. Text ──
	textColor := parseHexColor(style.Color)
	dc.SetRGBA(
		float64(textColor.R)/255,
		float64(textColor.G)/255,
		float64(textColor.B)/255,
		float64(textColor.A)/255,
	)
	drawLines(dc, lines, style, originX+padH, originY+padV+extraTop, textW)

	// ── 6. Optional curve: bend the whole composite along an arc. Because the
	//    warp changes the bounding box and re-centers the content, the shadow
	//    extent nudge below is skipped for curved elements. ──
	finalImg := dc.Image()
	curved := math.Abs(style.Curve) > 0.01
	if curved {
		finalImg = arcWarp(finalImg, style.Curve)
	}
	finalW := finalImg.Bounds().Dx()
	finalH := finalImg.Bounds().Dy()

	// ── 7. Optional rotation: blit onto a larger canvas, rotated about its center.
	//    Done as a post-process so pill/shadow/text geometry stays simple. ──
	if math.Abs(style.Rotation) > 0.01 {
		theta := style.Rotation * math.Pi / 180
		cos, sin := math.Abs(math.Cos(theta)), math.Abs(math.Sin(theta))
		newW := float64(finalW)*cos + float64(finalH)*sin
		newH := float64(finalW)*sin + float64(finalH)*cos
		rdc := gg.NewContext(int(math.Ceil(newW)), int(math.Ceil(newH)))
		rdc.SetRGBA(0, 0, 0, 0)
		rdc.Clear()
		rdc.Translate(newW/2, newH/2)
		rdc.Rotate(theta)
		rdc.DrawImageAnchored(finalImg, 0, 0, 0.5, 0.5)
		finalImg = rdc.Image()
		finalW = rdc.Width()
		finalH = rdc.Height()
	}

	// ── 8. Resolve position on video canvas using the final bounding box.
	//    For non-curved elements, subtract the original shadow extent so the
	//    element origin still anchors where the template author intended —
	//    shadow naturally extends beyond. ──
	x, y := resolvePosition(style.Position, finalW, finalH, ctx.VideoWidth, ctx.VideoHeight)
	if !curved {
		x -= int(margin)
		y -= int(margin)
	}

	// ── 8b. Safe-area clamp: keep the element box inside the TikTok safe rectangle
	//    (right action rail / bottom caption / top status band). No-op when the
	//    context sets no insets (static thumbnails). ──
	if ctx.InsetLeftFrac > 0 || ctx.InsetRightFrac > 0 || ctx.InsetTopFrac > 0 || ctx.InsetBottomFrac > 0 {
		il := int(ctx.InsetLeftFrac * float64(ctx.VideoWidth))
		ir := int(ctx.InsetRightFrac * float64(ctx.VideoWidth))
		it := int(ctx.InsetTopFrac * float64(ctx.VideoHeight))
		ib := int(ctx.InsetBottomFrac * float64(ctx.VideoHeight))
		x = clampPos(x, finalW, ctx.VideoWidth, il, ir)
		y = clampPos(y, finalH, ctx.VideoHeight, it, ib)
	}

	// ── 9. Encode PNG ──
	var buf bytes.Buffer
	if err := png.Encode(&buf, finalImg); err != nil {
		return nil, fmt.Errorf("textrender: encode: %v", err)
	}

	return &RenderedElement{
		PNG:    buf.Bytes(),
		Width:  finalW,
		Height: finalH,
		X:      x,
		Y:      y,
	}, nil
}

// drawLines paints `lines` onto dc starting at (x, y) with the font/alignment in style.
// `textW` is the measured max line width — used for right/center alignment.
// Assumes dc already has font + color loaded.
func drawLines(dc *gg.Context, lines []string, style *ElementStyle, x, y, textW float64) {
	lineSpacing := style.LineSpacing
	if lineSpacing <= 0 {
		lineSpacing = 1.2
	}
	var lineH float64
	for _, ln := range lines {
		_, h := dc.MeasureString(ln)
		if h > lineH {
			lineH = h
		}
	}
	step := lineH * lineSpacing
	cursorY := y + lineH // baseline of the first line

	for _, ln := range lines {
		w, _ := dc.MeasureString(ln)
		var lx float64
		switch strings.ToLower(style.Align) {
		case "right":
			lx = x + textW - w
		case "center", "":
			lx = x + (textW-w)/2
		default: // "left"
			lx = x
		}
		dc.DrawString(ln, lx, cursorY)
		cursorY += step
	}
}

// glyphInkTop returns the tallest ink extent above the baseline (in pixels)
// among the runes of text, measured on the actual font outlines. Used to grow
// the canvas for glyphs that rise above the nominal line box (stacked
// Vietnamese tone marks in Latin-metric fonts). Returns 0 on any parse error —
// callers then keep the unadjusted box, which matches the old behavior.
func glyphInkTop(fontPath string, size float64, text string) float64 {
	data, err := os.ReadFile(fontPath)
	if err != nil {
		return 0
	}
	f, err := truetype.Parse(data)
	if err != nil {
		return 0
	}
	face := truetype.NewFace(f, &truetype.Options{Size: size})
	defer face.Close()
	top := 0.0
	for _, r := range text {
		b, _, ok := face.GlyphBounds(r)
		if !ok {
			continue
		}
		if t := -float64(b.Min.Y) / 64; t > top {
			top = t
		}
	}
	return top
}

// Save writes the PNG bytes to disk.
func (r *RenderedElement) Save(path string) error {
	return os.WriteFile(path, r.PNG, 0644)
}
