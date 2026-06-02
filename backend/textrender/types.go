// Package textrender renders text overlays (title, listing fields, watermark)
// as transparent PNGs that FFmpeg can overlay onto a video.
//
// Style is data-driven via Template JSON files; no hardcoded layout.
// Uses fogleman/gg (freetype-go) for text + rounded rects + clipping,
// disintegration/imaging for Gaussian shadow blur. Pure Go — no external bins.
package textrender

// ElementStyle = visual config for a single text element.
type ElementStyle struct {
	Text string `json:"-"` // set at render time, not in template JSON

	// Font
	FontFile string  `json:"font"`     // file name within AssetsDir/fonts
	SizePct  float64 `json:"size_pct"` // size as fraction of video width
	Color    string  `json:"color"`    // hex "#RRGGBB" or "#RRGGBBAA"

	// Background (rounded pill / box). Nil = no background.
	Bg *BgStyle `json:"bg,omitempty"`

	// Drop shadow with Gaussian blur. Nil = no shadow.
	Shadow *ShadowStyle `json:"shadow,omitempty"`

	// Outline drawn around the glyphs. Nil = no outline.
	Stroke *StrokeStyle `json:"stroke,omitempty"`

	// Position of the element on the video canvas.
	Position Position `json:"position"`

	// Layout
	Align       string  `json:"align"`         // "left" | "center" | "right"
	LineSpacing float64 `json:"line_spacing"`  // multiplier; 0 → 1.2
	MaxWidthPct float64 `json:"max_width_pct"` // fraction of video width; 0 → no wrap

	// Rotation in degrees, applied after pill+text+shadow compositing.
	// Negative = counter-clockwise (right edge lifts up). 0 = no rotation.
	Rotation float64 `json:"rotation,omitempty"`

	// Curve bends the whole element (pill+text+shadow) along a circular arc
	// spanning this many degrees across its width. Positive = arch (middle
	// highest), negative = valley. 0 = straight. Applied before Rotation.
	Curve float64 `json:"curve,omitempty"`
}

type BgStyle struct {
	Color   string     `json:"color"`
	Alpha   float64    `json:"alpha"`   // 0..1
	Radius  float64    `json:"radius"`  // pixels
	Padding [2]float64 `json:"padding"` // [vertical, horizontal] pixels
}

type ShadowStyle struct {
	Color   string  `json:"color"`
	Alpha   float64 `json:"alpha"`    // 0..1
	Blur    float64 `json:"blur"`     // Gaussian sigma
	OffsetX float64 `json:"offset_x"` // shadow drops by this many px
	OffsetY float64 `json:"offset_y"`
}

// StrokeStyle = a solid outline traced around each glyph, drawn under the fill.
type StrokeStyle struct {
	Color string  `json:"color"` // hex "#RRGGBB" or "#RRGGBBAA"
	Width float64 `json:"width"` // outline thickness in px, extends outward from the glyph
	Alpha float64 `json:"alpha"` // 0..1; 0 is treated as fully opaque
}

// Position uses string values to allow either anchors or fractions/pixels.
type Position struct {
	X string `json:"x"` // "left" | "center" | "right" | "0.05" | "12px"
	Y string `json:"y"` // "top"  | "center" | "bottom" | "0.55" | "200px"
}

// StackStyle lays out a group of elements as a vertical flow: each item is
// rendered, measured, and placed below the previous one with a fixed Margin —
// so multi-line text never overlaps. Used for the listing-info cluster.
type StackStyle struct {
	Items  []string    `json:"items"`          // element keys, top→bottom
	X      string      `json:"x"`              // "center" | "left" | "right" | "0.05" | "12px"
	Anchor string      `json:"anchor"`         // "top" | "center" | "bottom" (how Y is interpreted)
	Y      string      `json:"y"`              // reference line: fraction of height or "Npx"
	Margin float64     `json:"margin"`         // px gap between items
	Scrim  *ScrimStyle `json:"scrim,omitempty"` // optional dark panel behind the stack
}

// ScrimStyle = a semi-transparent rounded panel drawn behind a stack to keep
// text readable over bright/busy photos.
type ScrimStyle struct {
	Color   string     `json:"color"`   // hex, e.g. "#000000"
	Alpha   float64    `json:"alpha"`   // 0..1
	Radius  float64    `json:"radius"`  // px
	Padding [2]float64 `json:"padding"` // [vertical, horizontal] px around the stack bbox
}

// Template = the full set of element styles for one preset.
type Template struct {
	Name        string                   `json:"name"`
	DisplayName string                   `json:"display_name"`
	Description string                   `json:"description"`
	Elements    map[string]*ElementStyle `json:"elements"`
	Stack       *StackStyle              `json:"stack,omitempty"`
}

// RenderContext = canvas size + asset locations passed to Render.
type RenderContext struct {
	VideoWidth  int
	VideoHeight int
	AssetsDir   string // contains fonts/ and templates/
}

// RenderedElement = the output of Render(): a PNG plus the (x, y) where
// FFmpeg should overlay it on the video.
type RenderedElement struct {
	PNG    []byte
	Width  int
	Height int
	X, Y   int
}
