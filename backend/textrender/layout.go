package textrender

import (
	"image/color"
	"strconv"
	"strings"

	"github.com/fogleman/gg"
)

// parseHexColor accepts "#RRGGBB", "#RRGGBBAA", or "RRGGBB". Returns black on bad input.
func parseHexColor(hex string) color.RGBA {
	s := strings.TrimPrefix(strings.TrimSpace(hex), "#")
	if len(s) == 6 {
		s += "FF"
	}
	if len(s) != 8 {
		return color.RGBA{0, 0, 0, 255}
	}
	r, _ := strconv.ParseUint(s[0:2], 16, 8)
	g, _ := strconv.ParseUint(s[2:4], 16, 8)
	b, _ := strconv.ParseUint(s[4:6], 16, 8)
	a, _ := strconv.ParseUint(s[6:8], 16, 8)
	return color.RGBA{uint8(r), uint8(g), uint8(b), uint8(a)}
}

// withAlpha multiplies the given color's alpha by mul (0..1).
func withAlpha(c color.RGBA, mul float64) color.RGBA {
	if mul <= 0 {
		mul = 1
	}
	if mul > 1 {
		mul = 1
	}
	out := c
	out.A = uint8(float64(c.A) * mul)
	return out
}

// wrapText splits text into lines that fit within maxWidth pixels.
// Uses gg measurer; respects existing newlines. If maxWidth<=0, returns
// the input split only on existing newlines.
func wrapText(dc *gg.Context, text string, maxWidth float64) []string {
	if maxWidth <= 0 {
		return strings.Split(text, "\n")
	}
	var out []string
	for _, paragraph := range strings.Split(text, "\n") {
		paragraph = strings.TrimSpace(paragraph)
		if paragraph == "" {
			out = append(out, "")
			continue
		}
		words := strings.Fields(paragraph)
		current := ""
		for _, w := range words {
			candidate := w
			if current != "" {
				candidate = current + " " + w
			}
			cw, _ := dc.MeasureString(candidate)
			if cw > maxWidth && current != "" {
				out = append(out, current)
				current = w
				continue
			}
			current = candidate
		}
		if current != "" {
			out = append(out, current)
		}
	}
	return out
}

// measureLines returns (max line width, total height, single line height) for a
// slice of lines at the currently-loaded font + given lineSpacing multiplier.
func measureLines(dc *gg.Context, lines []string, lineSpacing float64) (float64, float64, float64) {
	if lineSpacing <= 0 {
		lineSpacing = 1.2
	}
	var maxW float64
	var lineH float64
	for _, ln := range lines {
		w, h := dc.MeasureString(ln)
		if w > maxW {
			maxW = w
		}
		if h > lineH {
			lineH = h
		}
	}
	totalH := lineH * lineSpacing * float64(len(lines))
	return maxW, totalH, lineH
}

// resolvePosition converts a string spec ("left"|"center"|"right"|"0.05"|"12px")
// to an absolute pixel coordinate (top-left of the rendered element).
//
// For X: "left"=0, "center"=(canvas-w)/2, "right"=canvas-w.
// For Y: "top"=0, "center"=(canvas-h)/2, "bottom"=canvas-h.
// A fractional string ("0.55") is treated as a percentage of the video dimension
// (so "0.55" puts the TOP of the element at 55% of canvas height).
// A "12px" string is an absolute pixel offset.
func resolvePosition(p Position, elemW, elemH, canvasW, canvasH int) (int, int) {
	x := resolveAxis(p.X, elemW, canvasW, true)
	y := resolveAxis(p.Y, elemH, canvasH, false)
	return x, y
}

func resolveAxis(spec string, elemSize, canvasSize int, isX bool) int {
	s := strings.TrimSpace(spec)
	if s == "" {
		return 0
	}
	switch strings.ToLower(s) {
	case "left", "top":
		return 0
	case "center", "middle":
		return (canvasSize - elemSize) / 2
	case "right", "bottom":
		return canvasSize - elemSize
	}
	if strings.HasSuffix(s, "px") {
		n, _ := strconv.ParseFloat(strings.TrimSuffix(s, "px"), 64)
		return int(n)
	}
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return int(f * float64(canvasSize))
	}
	return 0
}
