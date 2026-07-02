// mockup-overlays renders the listing text overlays (address, nickname, prices,
// amenities, listing_id) as standalone PNGs for every (template × scale) combo.
// This is for the mockup HTML so the user can compare cỡ chữ side-by-side at
// pixel-accurate sizes without having to render full videos.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"img2video/textrender"
)

// Listing data used in the mockup — keep stable so frames are comparable.
var listing = map[string]string{
	"address":    "Tại Trung Hòa, Cầu Giấy, Hà Nội",
	"nickname":   "Sunset Stay",
	"prices":     "Giá 2 ngày 1 đêm: Từ 449.000 đ · Qua đêm 9PM–9AM:\nTừ 134.700 đ · Giá 2 giờ đầu: 199.000 đ",
	"amenities":  "Netflix · Máy điều hòa · Wifi · Bàn làm việc · Ổ cắm điện · Chỗ đỗ xe trong nhà",
	"listing_id": "Link ID 1024.876",
}

var stackKeys = []string{"address", "nickname", "prices", "amenities", "listing_id"}

func main() {
	tmplName := flag.String("template", "daiky", "template name (daiky|sunset|...)")
	scaleArg := flag.Float64("scale", 1.0, "uniform size scale")
	assetsDir := flag.String("assets", "assets", "assets dir (contains fonts/ + templates/)")
	outDir := flag.String("out", "mockup-out", "output dir for the rendered PNGs")
	flag.Parse()

	if err := run(*tmplName, *scaleArg, *assetsDir, *outDir); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run(tmplName string, scale float64, assetsDir, outDir string) error {
	tplDir := filepath.Join(assetsDir, "templates")
	tmpl, err := textrender.LoadTemplate(tmplName, tplDir)
	if err != nil {
		return err
	}

	// Apply uniform scale to every element's SizePct AND pill padding / radius /
	// stroke width / shadow blur. Without scaling pill+stroke too, the relative
	// "thickness" looks wrong when shrunk.
	for _, key := range stackKeys {
		st := tmpl.Elements[key]
		if st == nil {
			continue
		}
		st.SizePct *= scale
		if st.Bg != nil {
			st.Bg.Padding[0] *= scale
			st.Bg.Padding[1] *= scale
			st.Bg.Radius *= scale
		}
		if st.Stroke != nil {
			st.Stroke.Width *= scale
		}
		if st.Shadow != nil {
			st.Shadow.Blur *= scale
			st.Shadow.OffsetX *= scale
			st.Shadow.OffsetY *= scale
		}
	}

	ctx := &textrender.RenderContext{
		VideoWidth:  1080,
		VideoHeight: 1920,
		AssetsDir:   assetsDir,
	}

	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}

	for _, key := range stackKeys {
		st := tmpl.Elements[key]
		if st == nil {
			fmt.Printf("  - %s: (no element)\n", key)
			continue
		}
		// Inject the listing text into the style at render time.
		st.Text = listing[key]

		out, err := textrender.Render(st, ctx)
		if err != nil {
			return fmt.Errorf("render %s: %v", key, err)
		}
		if out == nil {
			fmt.Printf("  - %s: (empty)\n", key)
			continue
		}
		dst := filepath.Join(outDir, key+".png")
		if err := os.WriteFile(dst, out.PNG, 0o644); err != nil {
			return err
		}
		fmt.Printf("  ✓ %s  %dx%d → %s\n", key, out.Width, out.Height, dst)
	}

	// Also dump the resolved stack config (for the Python compositor).
	if tmpl.Stack != nil {
		s := tmpl.Stack
		// Margin also scales so spacing tightens proportionally.
		marginScaled := s.Margin * scale
		fp := filepath.Join(outDir, "stack.txt")
		body := fmt.Sprintf("items=%s\nx=%s\nanchor=%s\ny=%s\nmargin=%g\n",
			strings.Join(s.Items, ","), s.X, s.Anchor, s.Y, marginScaled)
		_ = os.WriteFile(fp, []byte(body), 0o644)
	}

	// Dump per-element position info too (for the standalone listing_id, which
	// is NOT in stack and has its own absolute position).
	for _, key := range stackKeys {
		st := tmpl.Elements[key]
		if st == nil {
			continue
		}
		fp := filepath.Join(outDir, key+".pos")
		body := fmt.Sprintf("x=%s\ny=%s\nalign=%s\n", st.Position.X, st.Position.Y, st.Align)
		_ = os.WriteFile(fp, []byte(body), 0o644)
	}

	return nil
}
