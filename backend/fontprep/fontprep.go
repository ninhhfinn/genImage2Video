// Package fontprep inspects and repairs user-uploaded fonts so they render
// correctly through the gg/freetype-go pipeline in backend/textrender.
//
// It exists because freetype-go has two silent failure modes that an arbitrary
// "bring-your-own" font routinely triggers:
//
//   - Variable fonts: freetype-go ignores the `wght` axis and rasterizes the
//     default (usually thin) master. We detect the `fvar` table and bake a
//     static Bold instance via fonttools (`varLib.instancer`).
//   - Missing glyphs: a Latin font with no Vietnamese diacritics renders □
//     tofu. We probe the cmap for the full Vietnamese letter set + digits.
//   - CFF outlines: freetype-go can only rasterize `glyf` (TrueType) outlines,
//     not CFF/PostScript. We flag those as unrenderable.
//
// This package is deliberately separate from textrender (which stays pure-Go at
// render time): instancing shells out to fonttools, the same way the app
// already shells out to ffmpeg/ImageMagick for other ingest steps.
package fontprep

import (
	"encoding/binary"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/image/font/sfnt"
)

// vietnamese is the complete set of precomposed Vietnamese letters with
// diacritics (plain ASCII a/e/i/o/u/y are omitted — every Latin font has those;
// it's the accented forms and đ/Đ that go missing). Digits are checked
// separately because listings lean on prices/areas/counts.
const vietnamese = "àáảãạăằắẳẵặâầấẩẫậđèéẻẽẹêềếểễệìíỉĩịòóỏõọôồốổỗộơờớởỡợùúủũụưừứửữựỳýỷỹỵ" +
	"ÀÁẢÃẠĂẰẮẲẴẶÂẦẤẨẪẬĐÈÉẺẼẸÊỀẾỂỄỆÌÍỈĨỊÒÓỎÕỌÔỒỐỔỖỘƠỜỚỞỠỢÙÚỦŨỤƯỪỨỬỮỰỲÝỶỸỴ"

const digits = "0123456789"

// Axis is one variation axis read from the fvar table.
type Axis struct {
	Tag string  `json:"tag"`
	Min float64 `json:"min"`
	Def float64 `json:"def"`
	Max float64 `json:"max"`
}

// Report is the structural verdict on a font, returned to the upload UI.
type Report struct {
	Family   string `json:"family"`
	Outline  string `json:"outline"`  // "glyf" | "cff" | "glyf+cff" | "unknown"
	Variable bool   `json:"variable"` // has an fvar table
	Axes     []Axis `json:"axes,omitempty"`

	RenderableByGG bool `json:"renderable_by_gg"` // glyf outlines + parseable cmap
	VietnameseOK   bool `json:"vietnamese_ok"`

	MissingVietnamese string `json:"missing_vietnamese,omitempty"` // the actual □ runes
	MissingDigits     string `json:"missing_digits,omitempty"`

	Warnings []string `json:"warnings,omitempty"`
}

// Inspect reports a font's structure and coverage. Pure-Go, no shell-out, and
// recovers from panics in the parser so a malformed upload can't crash a
// request handler.
func Inspect(data []byte) (rep Report) {
	defer func() {
		if r := recover(); r != nil {
			rep = Report{Outline: "unknown", Warnings: []string{fmt.Sprintf("không đọc được font: %v", r)}}
		}
	}()

	hasGlyf := tableData(data, "glyf") != nil
	hasCFF := tableData(data, "CFF ") != nil || tableData(data, "CFF2") != nil
	rep.Variable = tableData(data, "fvar") != nil

	switch {
	case hasGlyf && hasCFF:
		rep.Outline = "glyf+cff"
	case hasGlyf:
		rep.Outline = "glyf"
	case hasCFF:
		rep.Outline = "cff"
	default:
		rep.Outline = "unknown"
	}

	if rep.Variable {
		rep.Axes = parseFvarAxes(tableData(data, "fvar"))
	}

	fnt, err := sfnt.Parse(data)
	if err != nil {
		rep.RenderableByGG = false
		rep.Warnings = append(rep.Warnings, "không phân tích được font (cmap hỏng?)")
		return rep
	}

	// freetype-go (gg) only rasterizes glyf outlines. CFF-only → unusable.
	rep.RenderableByGG = hasGlyf
	if hasCFF && !hasGlyf {
		rep.Warnings = append(rep.Warnings, "font dùng outline CFF/PostScript — freetype-go không render được")
	}

	var buf sfnt.Buffer
	if name, err := fnt.Name(&buf, sfnt.NameIDFamily); err == nil {
		rep.Family = name
	}

	rep.MissingVietnamese = missingGlyphs(fnt, &buf, vietnamese)
	rep.MissingDigits = missingGlyphs(fnt, &buf, digits)
	rep.VietnameseOK = rep.MissingVietnamese == ""

	if !rep.VietnameseOK {
		rep.Warnings = append(rep.Warnings, "thiếu glyph tiếng Việt — chữ có dấu sẽ ra ô vuông")
	}
	if rep.MissingDigits != "" {
		rep.Warnings = append(rep.Warnings, "thiếu glyph chữ số")
	}
	return rep
}

// Prepare runs the full ingest pipeline: inspect, then (if variable) bake a
// static Bold instance freetype-go can render. Returns the bytes to store
// (instanced when changed), an updated report, and whether instancing ran.
//
// It does NOT reject fonts missing Vietnamese glyphs — that's a warning the UI
// surfaces, not a hard error (a user may legitimately want a Latin-only title).
// It DOES error on outlines gg cannot rasterize, since storing those is futile.
func Prepare(data []byte) (out []byte, rep Report, instanced bool, err error) {
	rep = Inspect(data)
	if rep.Outline == "cff" {
		return nil, rep, false, fmt.Errorf("font dùng outline CFF/PostScript, không hỗ trợ (cần .ttf TrueType)")
	}
	if !rep.RenderableByGG {
		return nil, rep, false, fmt.Errorf("font không render được qua freetype-go")
	}

	if !rep.Variable {
		return data, rep, false, nil
	}

	staticData, pins, ierr := InstanceStatic(data, rep.Axes)
	if ierr != nil {
		// Keep the original bytes; the caller can still store it but the title
		// will render thin. Surface the failure as a warning.
		rep.Warnings = append(rep.Warnings, "không tạo được bản static (font sẽ render mảnh): "+ierr.Error())
		return data, rep, false, nil
	}

	// Re-inspect the instanced font so the report reflects reality (fvar gone).
	rep2 := Inspect(staticData)
	rep2.Family = rep.Family // family name can drop during instancing; keep original
	rep2.Warnings = append(rep2.Warnings, fmt.Sprintf("đã tạo bản static từ font variable (%s)", pinSummary(pins)))
	return staticData, rep2, true, nil
}

// HasFontTools reports whether the `fonttools` CLI is available for instancing.
func HasFontTools() bool {
	_, err := exec.LookPath("fonttools")
	return err == nil
}

// InstanceStatic pins every axis to a sensible static value (wght→Bold clamped
// to range, ital/slnt→upright, the rest→default) and shells out to fonttools to
// bake a static instance. Tries --remove-overlaps first (cleans seams from
// merged contours); falls back without it if skia-pathops is unavailable.
func InstanceStatic(data []byte, axes []Axis) (out []byte, pins map[string]float64, err error) {
	if !HasFontTools() {
		return nil, nil, fmt.Errorf("chưa cài fonttools (cần để xử lý font variable)")
	}
	if len(axes) == 0 {
		axes = parseFvarAxes(tableData(data, "fvar"))
	}
	if len(axes) == 0 {
		return nil, nil, fmt.Errorf("không có trục fvar để pin")
	}

	dir, err := os.MkdirTemp("", "fontprep_*")
	if err != nil {
		return nil, nil, err
	}
	defer os.RemoveAll(dir)
	inPath := filepath.Join(dir, "in.ttf")
	outPath := filepath.Join(dir, "out.ttf")
	if err := os.WriteFile(inPath, data, 0644); err != nil {
		return nil, nil, err
	}

	pins = map[string]float64{}
	var pinArgs []string
	for _, a := range axes {
		tag := strings.TrimRight(a.Tag, " \x00")
		target := a.Def
		switch tag {
		case "wght":
			target = clampF(700, a.Min, a.Max) // Bold, clamped to the font's range
		case "ital", "slnt":
			target = clampF(0, a.Min, a.Max) // upright
		}
		pins[tag] = target
		pinArgs = append(pinArgs, tag+"="+strconv.FormatFloat(target, 'g', -1, 64))
	}

	run := func(extra ...string) error {
		args := append([]string{"varLib.instancer", inPath}, pinArgs...)
		args = append(args, extra...)
		args = append(args, "-o", outPath)
		out, err := exec.Command("fonttools", args...).CombinedOutput()
		if err != nil {
			return fmt.Errorf("%v: %s", err, lastLine(string(out)))
		}
		return nil
	}
	if err := run("--remove-overlaps"); err != nil {
		if err2 := run(); err2 != nil {
			return nil, nil, err2
		}
	}

	out, err = os.ReadFile(outPath)
	if err != nil {
		return nil, nil, err
	}
	return out, pins, nil
}

// ─────────────────────────── low-level helpers ──────────────────────────────

func missingGlyphs(fnt *sfnt.Font, buf *sfnt.Buffer, runes string) string {
	var miss []rune
	for _, r := range runes {
		gi, err := fnt.GlyphIndex(buf, r)
		if err != nil || gi == 0 {
			miss = append(miss, r)
		}
	}
	return string(miss)
}

func parseFvarAxes(fvar []byte) []Axis {
	if len(fvar) < 16 {
		return nil
	}
	axesOffset := int(binary.BigEndian.Uint16(fvar[4:6]))
	axisCount := int(binary.BigEndian.Uint16(fvar[8:10]))
	axisSize := int(binary.BigEndian.Uint16(fvar[10:12]))
	if axisSize < 20 {
		axisSize = 20
	}
	var out []Axis
	for i := 0; i < axisCount; i++ {
		p := axesOffset + i*axisSize
		if p+20 > len(fvar) {
			break
		}
		out = append(out, Axis{
			Tag: strings.TrimRight(string(fvar[p:p+4]), " \x00"),
			Min: fixed1616(fvar[p+4:]),
			Def: fixed1616(fvar[p+8:]),
			Max: fixed1616(fvar[p+12:]),
		})
	}
	return out
}

func fixed1616(b []byte) float64 {
	return float64(int32(binary.BigEndian.Uint32(b[:4]))) / 65536.0
}

// tableData scans the OpenType table directory for a 4-byte tag and returns
// that table's bytes (nil if absent). Handles single fonts and the first font
// of a .ttc collection. Dependency-free so it works even on fonts sfnt can't
// fully parse.
func tableData(data []byte, tag string) []byte {
	if len(data) < 12 {
		return nil
	}
	off := 0
	if string(data[0:4]) == "ttcf" {
		if len(data) < 16 {
			return nil
		}
		off = int(binary.BigEndian.Uint32(data[12:16]))
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

func clampF(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func pinSummary(pins map[string]float64) string {
	var parts []string
	for _, tag := range []string{"wght", "wdth", "opsz", "slnt", "ital"} {
		if v, ok := pins[tag]; ok {
			parts = append(parts, fmt.Sprintf("%s=%g", tag, v))
		}
	}
	return strings.Join(parts, " ")
}

func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	return lines[len(lines)-1]
}
