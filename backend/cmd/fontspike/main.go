// Command fontspike is a throwaway preflight probe for "bring-your-own font".
//
// It answers the only question that matters for the import-arbitrary-font
// feature: when a user hands us a font file, what will the *gg/freetype-go*
// pipeline actually do with it? An HTML mockup would lie here — the browser
// renders variable fonts and substitutes missing glyphs far more forgivingly
// than freetype-go does. So this drives the real textrender.Render path and
// writes the exact PNG the video would get.
//
// For each font it reports:
//   - outline type (glyf vs CFF) — freetype-go only rasterizes glyf
//   - variable?  (presence of the `fvar` table) — gg pins the default master,
//     so variable fonts render THIN
//   - Vietnamese + digit glyph coverage (via cmap) — missing → tofu boxes
//   - whether gg could load+render it at all
//   - an ink-ratio weight proxy so "renders thin" is quantified, not asserted
//
// Usage:
//   go run ./cmd/fontspike                 # probes a curated set in assets/fonts
//   go run ./cmd/fontspike path/to/x.ttf   # probes specific files
//
// PNGs land in ./fontspike-out/.
package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/image/font/sfnt"

	"img2video/textrender"
)

// previewText exercises the high-risk characters in one realistic listing
// title: Đ ư ờ ệ ự ầ ễ (diacritics that vanish in non-VN fonts) + digits.
const previewText = "Biệt thự 3 tầng – 120m²\nĐường Nguyễn Huệ, Quận 1"

// Vietnamese letters that a generic Latin font is likely to be missing. Plain
// a/e/i/o/u/y are omitted — every font has those; their diacritic forms are
// where tofu shows up. đ/Đ is the single most commonly absent glyph.
var vietSpecial = []rune(
	"ăâđêôơưĂÂĐÊÔƠƯ" +
		"áàảãạắằẳẵặấầẩẫậ" +
		"éèẻẽẹếềểễệ" +
		"íìỉĩị" +
		"óòỏõọốồổỗộớờởỡợ" +
		"úùủũụứừửữự" +
		"ýỳỷỹỵ",
)

var digits = []rune("0123456789")

func main() {
	fonts := os.Args[1:]
	if len(fonts) == 0 {
		// Curated default set — one font per failure mode we expect to see.
		base := "assets/fonts"
		for _, n := range []string{
			"BeVietnamPro-Bold.ttf",  // good: VN-native, static
			"Baloo2-Bold.ttf",        // good: static instance we ship
			"Baloo2-Variable.ttf",    // variable → should render thin
			"PlayfairDisplay-Variable.ttf", // variable
			"Fredoka-Bold.ttf",       // missing VN glyphs → tofu
			"Prata-Regular.ttf",      // static serif, lining figures
		} {
			fonts = append(fonts, filepath.Join(base, n))
		}
	}

	outDir := "fontspike-out"
	if err := os.MkdirAll(outDir, 0755); err != nil {
		fmt.Fprintln(os.Stderr, "mkdir out:", err)
		os.Exit(1)
	}

	fmt.Printf("Probing %d font(s) → previews in ./%s/\n", len(fonts), outDir)
	fmt.Println(strings.Repeat("─", 72))

	for _, fp := range fonts {
		probe(fp, outDir)
	}

	fmt.Println(strings.Repeat("─", 72))
	fmt.Println("Open the PNGs and compare: variable fonts render thin, fonts")
	fmt.Println("without VN coverage show □ boxes. That is what gen-video would get.")
}

func probe(fontPath, outDir string) {
	name := filepath.Base(fontPath)
	fmt.Printf("\n● %s\n", name)

	data, err := os.ReadFile(fontPath)
	if err != nil {
		fmt.Printf("  ✗ read: %v\n", err)
		return
	}

	// ── structural: outline type + variable axes (table directory scan) ──
	hasGlyf := hasTable(data, "glyf")
	hasCFF := hasTable(data, "CFF ") || hasTable(data, "CFF2")
	isVar := hasTable(data, "fvar")

	outline := "glyf (TrueType)"
	switch {
	case hasCFF && !hasGlyf:
		outline = "CFF (PostScript)"
	case hasCFF && hasGlyf:
		outline = "glyf+CFF"
	case !hasGlyf && !hasCFF:
		outline = "unknown"
	}
	fmt.Printf("  outline   : %s\n", outline)
	if hasCFF && !hasGlyf {
		fmt.Printf("            ⚠ freetype-go can't rasterize CFF outlines → gg will fail\n")
	}
	var axes []fvarAxis
	if isVar {
		axes = parseFvarAxes(tableData(data, "fvar"))
		fmt.Printf("  variable  : YES (%s) ⚠ gg pins default master → renders THIN\n", axesSummary(axes))
	} else {
		fmt.Printf("  variable  : no\n")
	}

	// ── coverage: which required runes lack a glyph (cmap lookup) ──
	missVN := missingGlyphs(data, vietSpecial)
	missDig := missingGlyphs(data, digits)
	switch {
	case missVN == nil && missDig == nil:
		fmt.Printf("  coverage  : ✓ full Vietnamese + digits\n")
	default:
		if len(missVN) > 0 {
			fmt.Printf("  coverage  : ✗ missing %d/%d VN glyphs → tofu: %s\n",
				len(missVN), len(vietSpecial), string(missVN))
		}
		if len(missDig) > 0 {
			fmt.Printf("  coverage  : ✗ missing digits: %s\n", string(missDig))
		}
	}

	// ── the truth: render the font AS-IS through the real gg pipeline ──
	stem := strings.TrimSuffix(name, filepath.Ext(name))
	out := filepath.Join(outDir, stem+".png")
	renderErr := renderPreview(data, name, previewText, out, 0.06, true)
	origInk := 0.0
	if renderErr != nil {
		fmt.Printf("  render    : ✗ gg refused: %v\n", renderErr)
	} else {
		origInk, _ = inkRatio(data, name)
		fmt.Printf("  render    : ✓ %s   (ink ≈ %.1f%% — weight proxy)\n", out, origInk*100)
	}

	// ── repair: variable fonts get instanced to a static Bold master that
	//    freetype-go reads correctly. This is the actual fix for the THIN bug. ──
	instanced := false
	if isVar {
		staticPath := filepath.Join(outDir, stem+".static.ttf")
		pins, ierr := instanceStatic(fontPath, staticPath, axes)
		if ierr != nil {
			fmt.Printf("  ↳ instance: ✗ %v\n", ierr)
		} else {
			instanced = true
			sData, _ := os.ReadFile(staticPath)
			stillVar := hasTable(sData, "fvar")
			sName := filepath.Base(staticPath)
			sOut := filepath.Join(outDir, stem+".static.png")
			sErr := renderPreview(sData, sName, previewText, sOut, 0.06, true)
			sInk, _ := inkRatio(sData, sName)
			fmt.Printf("  ↳ instance: %s → %s\n", pinSummary(pins), staticPath)
			fmt.Printf("    re-probe: variable=%v, render %s %s  (ink %.1f%% → %.1f%%, %+.1fpp)\n",
				stillVar, okMark(sErr == nil), sOut, origInk*100, sInk*100, (sInk-origInk)*100)
		}
	}

	fmt.Printf("  verdict   : %s\n", verdict(hasCFF && !hasGlyf, isVar, instanced, missVN, missDig, renderErr))
}

func okMark(ok bool) string {
	if ok {
		return "✓"
	}
	return "✗"
}

func verdict(cffOnly, isVar, instanced bool, missVN, missDig []rune, renderErr error) string {
	switch {
	case renderErr != nil || cffOnly:
		return "FAIL — unusable as-is (reject, or convert outlines server-side)"
	case len(missVN) > 0:
		return "WARN — no Vietnamese support; warn user / block for VN content"
	case isVar && instanced:
		return "FIXED — was variable (thin); use the .static.ttf instance"
	case isVar:
		return "WARN — variable but instancing failed; reject or install fonttools"
	case len(missDig) > 0:
		return "WARN — digit glyphs missing"
	default:
		return "PASS — safe to use directly"
	}
}

// ─────────────────────────── variable-font instancing ───────────────────────

type fvarAxis struct {
	tag           string
	min, def, max float64
}

// instanceStatic pins every axis to a sensible static value (wght→Bold,
// ital/slnt→upright, the rest→default) and shells out to fonttools to bake a
// static instance freetype-go can rasterize at the right weight. Tries
// --remove-overlaps first (cleans seams from merged contours), falls back to a
// plain instance if skia-pathops isn't available.
func instanceStatic(srcPath, outPath string, axes []fvarAxis) (map[string]float64, error) {
	if _, err := exec.LookPath("fonttools"); err != nil {
		return nil, fmt.Errorf("fonttools not installed (needed to instance variable fonts)")
	}
	if len(axes) == 0 {
		return nil, fmt.Errorf("no fvar axes to pin")
	}
	pins := map[string]float64{}
	var pinArgs []string
	for _, a := range axes {
		tag := strings.TrimRight(a.tag, " \x00")
		target := a.def
		switch tag {
		case "wght":
			target = clampF(700, a.min, a.max) // Bold, clamped to the font's range
		case "ital", "slnt":
			target = clampF(0, a.min, a.max) // upright
		}
		pins[tag] = target
		pinArgs = append(pinArgs, tag+"="+strconv.FormatFloat(target, 'g', -1, 64))
	}

	run := func(extra ...string) error {
		args := append([]string{"varLib.instancer", srcPath}, pinArgs...)
		args = append(args, extra...)
		args = append(args, "-o", outPath)
		cmd := exec.Command("fonttools", args...)
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("%v: %s", err, lastLine(stderr.String()))
		}
		return nil
	}
	if err := run("--remove-overlaps"); err != nil {
		if err2 := run(); err2 != nil { // retry without overlap removal
			return nil, err2
		}
	}
	return pins, nil
}

// parseFvarAxes reads axis records straight from the fvar table.
func parseFvarAxes(fvar []byte) []fvarAxis {
	if len(fvar) < 16 {
		return nil
	}
	axesOffset := int(binary.BigEndian.Uint16(fvar[4:6]))
	axisCount := int(binary.BigEndian.Uint16(fvar[8:10]))
	axisSize := int(binary.BigEndian.Uint16(fvar[10:12]))
	if axisSize < 20 {
		axisSize = 20
	}
	var out []fvarAxis
	for i := 0; i < axisCount; i++ {
		p := axesOffset + i*axisSize
		if p+20 > len(fvar) {
			break
		}
		out = append(out, fvarAxis{
			tag: string(fvar[p : p+4]),
			min: fixed1616(fvar[p+4:]),
			def: fixed1616(fvar[p+8:]),
			max: fixed1616(fvar[p+12:]),
		})
	}
	return out
}

func fixed1616(b []byte) float64 {
	return float64(int32(binary.BigEndian.Uint32(b[:4]))) / 65536.0
}

func clampF(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func axesSummary(axes []fvarAxis) string {
	var parts []string
	for _, a := range axes {
		parts = append(parts, fmt.Sprintf("%s %g..%g@%g",
			strings.TrimRight(a.tag, " \x00"), a.min, a.max, a.def))
	}
	return "fvar: " + strings.Join(parts, ", ")
}

func pinSummary(pins map[string]float64) string {
	var parts []string
	for _, tag := range []string{"wght", "wdth", "opsz", "slnt", "ital"} {
		if v, ok := pins[tag]; ok {
			parts = append(parts, fmt.Sprintf("%s=%g", tag, v))
		}
	}
	for tag, v := range pins { // any non-standard axes
		switch tag {
		case "wght", "wdth", "opsz", "slnt", "ital":
		default:
			parts = append(parts, fmt.Sprintf("%s=%g", tag, v))
		}
	}
	return "pin " + strings.Join(parts, " ")
}

func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	return lines[len(lines)-1]
}

// renderPreview drives textrender.Render exactly as the video pipeline does:
// font lives in <AssetsDir>/fonts/<name>. We stage it in a temp assets dir so
// this generalizes to an uploaded font sitting anywhere on disk.
func renderPreview(data []byte, name, text, outPath string, sizePct float64, withBg bool) error {
	tmp, err := os.MkdirTemp("", "fontspike_*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	fontsDir := filepath.Join(tmp, "fonts")
	if err := os.MkdirAll(fontsDir, 0755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(fontsDir, name), data, 0644); err != nil {
		return err
	}

	style := &textrender.ElementStyle{
		Text:        text,
		FontFile:    name,
		SizePct:     sizePct,
		Color:       "#FFFFFF",
		Position:    textrender.Position{X: "center", Y: "center"},
		Align:       "center",
		LineSpacing: 1.3,
		MaxWidthPct: 0.9,
	}
	if withBg {
		style.Bg = &textrender.BgStyle{
			Color: "#11151c", Alpha: 0.92, Radius: 28, Padding: [2]float64{28, 56},
		}
	}

	ctx := &textrender.RenderContext{VideoWidth: 1080, VideoHeight: 1350, AssetsDir: tmp}
	el, err := textrender.Render(style, ctx)
	if err != nil {
		return err
	}
	if el == nil {
		return fmt.Errorf("nil render (empty text?)")
	}
	return el.Save(outPath)
}

// inkRatio renders the title with NO background and measures the fraction of
// non-transparent pixels — a rough stroke-weight proxy. A variable font pinned
// to its thin default master inks noticeably less than its bold static sibling.
func inkRatio(data []byte, name string) (float64, error) {
	tmp, err := os.MkdirTemp("", "fontink_*")
	if err != nil {
		return 0, err
	}
	defer os.RemoveAll(tmp)
	fontsDir := filepath.Join(tmp, "fonts")
	os.MkdirAll(fontsDir, 0755)
	os.WriteFile(filepath.Join(fontsDir, name), data, 0644)

	style := &textrender.ElementStyle{
		Text: previewText, FontFile: name, SizePct: 0.06, Color: "#FFFFFF",
		Position: textrender.Position{X: "center", Y: "center"},
		Align:    "center", LineSpacing: 1.3, MaxWidthPct: 0.9,
	}
	ctx := &textrender.RenderContext{VideoWidth: 1080, VideoHeight: 1350, AssetsDir: tmp}
	el, err := textrender.Render(style, ctx)
	if err != nil || el == nil {
		return 0, err
	}
	img, err := png.Decode(strings.NewReader(string(el.PNG)))
	if err != nil {
		return 0, err
	}
	return alphaFill(img), nil
}

func alphaFill(img image.Image) float64 {
	b := img.Bounds()
	total := b.Dx() * b.Dy()
	if total == 0 {
		return 0
	}
	ink := 0
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if _, _, _, a := img.At(x, y).RGBA(); a > 0x4000 { // >25% opaque
				ink++
			}
		}
	}
	return float64(ink) / float64(total)
}

// missingGlyphs returns the subset of runes the font has no glyph for (cmap
// glyph index 0 = .notdef). Returns nil if all are present.
func missingGlyphs(data []byte, runes []rune) []rune {
	fnt, err := sfnt.Parse(data)
	if err != nil {
		return runes // can't parse cmap → treat everything as missing
	}
	var buf sfnt.Buffer
	var miss []rune
	for _, r := range runes {
		gi, err := fnt.GlyphIndex(&buf, r)
		if err != nil || gi == 0 {
			miss = append(miss, r)
		}
	}
	return miss
}

func hasTable(data []byte, tag string) bool { return tableData(data, tag) != nil }

// tableData scans the OpenType table directory for a 4-byte tag and returns
// that table's bytes (nil if absent). Handles single fonts and the first font
// of a .ttc collection. Dependency-free so it works even on fonts
// sfnt/freetype can't fully parse.
func tableData(data []byte, tag string) []byte {
	if len(data) < 12 {
		return nil
	}
	off := 0
	if string(data[0:4]) == "ttcf" { // font collection
		if len(data) < 16 {
			return nil
		}
		off = int(binary.BigEndian.Uint32(data[12:16])) // offset of first font
	}
	if off+12 > len(data) {
		return nil
	}
	num := int(binary.BigEndian.Uint16(data[off+4 : off+6]))
	rec := off + 12
	for i := 0; i < num; i++ {
		p := rec + i*16
		if p+16 > len(data) {
			break
		}
		if string(data[p:p+4]) == tag {
			tOff := int(binary.BigEndian.Uint32(data[p+8 : p+12]))
			tLen := int(binary.BigEndian.Uint32(data[p+12 : p+16]))
			if tOff < 0 || tLen < 0 || tOff+tLen > len(data) {
				return nil
			}
			return data[tOff : tOff+tLen]
		}
	}
	return nil
}
