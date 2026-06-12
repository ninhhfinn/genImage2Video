package main

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

// Bản static Bold (bake từ font variable): magick/Pango cũng chỉ lấy master
// Regular của font variable nên badge bị mảnh + số old-style.
//go:embed assets/fonts/PlayfairDisplay-Bold.ttf
var fontPlayfairData []byte

//go:embed assets/fonts/LilitaOne-Regular.ttf
var fontLilitaData []byte

const (
	badgePillBG = "#8C6149"
	badgeTextFG = "#FFFFFF"
)

var (
	fontInitOnce       sync.Once
	fontInitErr        error
	playfairPath       string // dùng trực tiếp qua -font cho badge
	lilitaPath         string
	fontconfigFilePath string // FONTCONFIG_FILE cho Pango (bubble)
)

// initFonts extract embedded TTF ra temp dir + tạo fontconfig isolated
// để Pango / magick tìm thấy mà không cần ghi vào ~/.local/share/fonts.
func initFonts() error {
	fontInitOnce.Do(func() {
		dir, err := os.MkdirTemp("", "img2video_fonts_*")
		if err != nil {
			fontInitErr = err
			return
		}
		playfairPath = filepath.Join(dir, "PlayfairDisplay-Bold.ttf")
		lilitaP := filepath.Join(dir, "LilitaOne-Regular.ttf")
		if err := os.WriteFile(playfairPath, fontPlayfairData, 0644); err != nil {
			fontInitErr = err
			return
		}
		if err := os.WriteFile(lilitaP, fontLilitaData, 0644); err != nil {
			fontInitErr = err
			return
		}
		lilitaPath = lilitaP
		cacheDir := filepath.Join(dir, "cache")
		if err := os.MkdirAll(cacheDir, 0755); err != nil {
			fontInitErr = err
			return
		}
		fcConf := `<?xml version="1.0"?>
<!DOCTYPE fontconfig SYSTEM "fonts.dtd">
<fontconfig>
  <dir>` + dir + `</dir>
  <include ignore_missing="yes">/etc/fonts/conf.d</include>
  <cachedir>` + cacheDir + `</cachedir>
</fontconfig>
`
		fontconfigFilePath = filepath.Join(dir, "fonts.conf")
		if err := os.WriteFile(fontconfigFilePath, []byte(fcConf), 0644); err != nil {
			fontInitErr = err
			return
		}
	})
	return fontInitErr
}

// magickCmd build magick command với FONTCONFIG_FILE env để Pango thấy embedded fonts.
func magickCmd(args ...string) *exec.Cmd {
	cmd := exec.Command("magick", args...)
	if fontconfigFilePath != "" {
		cmd.Env = append(os.Environ(), "FONTCONFIG_FILE="+fontconfigFilePath)
	}
	return cmd
}

func hasListingOverlay(cfg Config) bool {
	return cfg.Address != "" || cfg.Nickname != "" || len(cfg.Prices) > 0 || len(cfg.Amenities) > 0
}

func overlayHash(cfg Config) string {
	h := sha256.New()
	fmt.Fprintf(h, "v4|%s|%s|%s|%v|%v|%dx%d|%s|%.2f|%s|%s|%s",
		cfg.OverlayStyle, cfg.Address, cfg.Nickname,
		cfg.Prices, cfg.Amenities, cfg.Width, cfg.Height,
		cfg.OverlayFont, safeScale(cfg.OverlayScale), cfg.OverlayText, cfg.OverlayBG, cfg.OverlayStroke)
	return hex.EncodeToString(h.Sum(nil))[:16]
}

// renderListingOverlayPNG render PNG 1080x1920 trong suốt với listing overlay.
// Trả về đường dẫn PNG; rỗng nếu không có gì để render.
func renderListingOverlayPNG(cfg Config) (string, error) {
	if !hasListingOverlay(cfg) {
		return "", nil
	}
	if _, err := exec.LookPath("magick"); err != nil {
		return "", fmt.Errorf("ImageMagick `magick` chưa cài: %v", err)
	}
	if err := initFonts(); err != nil {
		return "", fmt.Errorf("init fonts: %v", err)
	}

	w, h := cfg.Width, cfg.Height
	if w <= 0 {
		w = 1080
	}
	if h <= 0 {
		h = 1920
	}
	outPath := filepath.Join(os.TempDir(), fmt.Sprintf("img2video_overlay_%s.png", overlayHash(cfg)))
	tmpDir, err := os.MkdirTemp("", "img2video_pills_*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmpDir)

	style := strings.ToLower(strings.TrimSpace(cfg.OverlayStyle))
	var pills []composedLayer
	if style == "bubble" {
		pills, err = composeBubbleLayers(cfg, w, h, tmpDir)
	} else {
		pills, err = composeBadgeLayers(cfg, w, h, tmpDir)
	}
	if err != nil {
		return "", err
	}

	args := []string{"-size", fmt.Sprintf("%dx%d", w, h), "xc:none"}
	for _, p := range pills {
		args = append(args, p.path, "-geometry", fmt.Sprintf("%+d%+d", p.x, p.y), "-compose", "over", "-composite")
	}
	args = append(args, outPath)
	if out, err := magickCmd(args...).CombinedOutput(); err != nil {
		return "", fmt.Errorf("magick compose: %v\n%s", err, out)
	}
	return outPath, nil
}

type composedLayer struct {
	path string // PNG path
	x, y int    // top-left position on canvas
}

// ─── Badge style ──────────────────────────────────────────────────────────

func composeBadgeLayers(cfg Config, canvasW, canvasH int, tmpDir string) ([]composedLayer, error) {
	type pill struct {
		text     string
		font     string // absolute path
		weight   int    // 400 = regular, 700 = bold
		fontSize int
		padX     int
		padY     int
		radius   int
	}

	scale := safeScale(cfg.OverlayScale)
	fontPath := selectedFontPath(cfg.OverlayFont)
	if fontPath == "" {
		fontPath = playfairPath
	}
	textColor := safeColor(cfg.OverlayText, badgeTextFG)
	bgColor := safeColor(cfg.OverlayBG, badgePillBG)

	addressFs := int(float64(canvasW/30) * scale)
	titleFs := int(float64(canvasW/9) * scale)
	priceFs := int(float64(canvasW/30) * scale)
	amenityFs := int(float64(canvasW/34) * scale)

	var queue []pill
	for _, ln := range wrapText(cfg.Address, 32, 2) {
		queue = append(queue, pill{ln, fontPath, 600, addressFs, 30, 16, 32})
	}
	if cfg.Nickname != "" {
		title := strings.TrimSpace(strings.TrimPrefix(cfg.Nickname, "Homestay "))
		// Title cũng cap nếu quá dài (vd "Phòng deluxe view biển 301") → wrap thành 2 dòng
		titleLines := wrapText(title, 14, 2)
		for _, ln := range titleLines {
			queue = append(queue, pill{ln, fontPath, 700, titleFs, 60, 30, 48})
		}
	}
	for _, ln := range wrapJoin(cfg.Prices, "- ", 30, 3) {
		queue = append(queue, pill{ln, fontPath, 600, priceFs, 28, 14, 30})
	}
	for _, ln := range wrapJoin(cfg.Amenities, "- ", 32, 3) {
		queue = append(queue, pill{ln, fontPath, 600, amenityFs, 24, 12, 26})
	}

	var layers []composedLayer
	y := int(float64(canvasH) * 0.10)
	gap := 16
	for i, p := range queue {
		path := filepath.Join(tmpDir, fmt.Sprintf("pill_%02d.png", i))
		w, h, err := renderRoundedPill(p.text, p.font, p.weight, p.fontSize, textColor, bgColor, p.padX, p.padY, p.radius, path)
		if err != nil {
			return nil, err
		}
		x := (canvasW - w) / 2
		layers = append(layers, composedLayer{path: path, x: x, y: y})
		y += h + gap
	}
	return layers, nil
}

// renderRoundedPill render 1 pill (chữ + nền bo tròn) ra PNG. Trả width, height của PNG.
func renderRoundedPill(text, fontPath string, weight, fontSize int, fg, bg string, padX, padY, radius int, outPath string) (int, int, error) {
	weightStr := strconv.Itoa(weight)
	args := []string{
		"-background", bg,
		"-fill", fg,
		"-font", fontPath,
		"-weight", weightStr,
		"-pointsize", strconv.Itoa(fontSize),
		"-gravity", "center",
		"-kerning", "1",
		"label:" + text,
		"-bordercolor", bg,
		"-border", fmt.Sprintf("%dx%d", padX, padY),
		"-alpha", "set",
		"(",
		"+clone", "-channel", "A", "-evaluate", "set", "0", "+channel",
		"-fill", "white",
		"-draw", fmt.Sprintf("roundrectangle 0,0 %%[fx:w-1],%%[fx:h-1] %d,%d", radius, radius),
		"-alpha", "extract",
		")",
		"-compose", "CopyOpacity", "-composite",
		outPath,
	}
	if out, err := magickCmd(args...).CombinedOutput(); err != nil {
		return 0, 0, fmt.Errorf("renderRoundedPill: %v\n%s", err, out)
	}
	return identifySize(outPath)
}

// ─── Bubble style ─────────────────────────────────────────────────────────

func composeBubbleLayers(cfg Config, canvasW, canvasH int, tmpDir string) ([]composedLayer, error) {
	scale := safeScale(cfg.OverlayScale)
	titleFs := int(float64(canvasW/6) * scale)
	bodyFs := int(float64(canvasW/28) * scale)
	leftX := int(float64(canvasW) * 0.06)
	fontFamily := selectedPangoFontFamily(cfg.OverlayFont)
	textColor := safeColor(cfg.OverlayText, "white")
	strokeColor := safeColor(cfg.OverlayStroke, "black")

	var layers []composedLayer
	y := int(float64(canvasH) * 0.56)
	idx := 0

	addTitle := func(text string, fs, strokeW int) error {
		path := filepath.Join(tmpDir, fmt.Sprintf("bub_%02d.png", idx))
		idx++
		_, h, err := renderBubbleText(text, fontFamily, fs, textColor, strokeColor, strokeW, path)
		if err != nil {
			return err
		}
		layers = append(layers, composedLayer{path: path, x: leftX, y: y})
		y += h + 10
		return nil
	}
	addBody := func(text string, fs, strokeW int) error {
		path := filepath.Join(tmpDir, fmt.Sprintf("bub_%02d.png", idx))
		idx++
		_, h, err := renderBubbleText(text, fontFamily, fs, textColor, strokeColor, strokeW, path)
		if err != nil {
			return err
		}
		layers = append(layers, composedLayer{path: path, x: leftX, y: y})
		y += h + 4
		return nil
	}

	if cfg.Nickname != "" {
		title := strings.TrimSpace(strings.TrimPrefix(cfg.Nickname, "Homestay "))
		if err := addTitle(title, titleFs, 10); err != nil {
			return nil, err
		}
		y += 6
	}
	for _, ln := range wrapText(cfg.Address, 38, 2) {
		if err := addBody(ln, bodyFs, 4); err != nil {
			return nil, err
		}
	}
	bodyCount := 0
	for _, ln := range wrapJoin(cfg.Prices, "- ", 36, 2) {
		if bodyCount >= 6 {
			break
		}
		if err := addBody(ln, bodyFs, 4); err != nil {
			return nil, err
		}
		bodyCount++
	}
	for _, ln := range wrapJoin(cfg.Amenities, "- ", 36, 3) {
		if bodyCount >= 8 {
			break
		}
		if err := addBody(ln, bodyFs, 4); err != nil {
			return nil, err
		}
		bodyCount++
	}
	return layers, nil
}

func selectedPangoFontFamily(choice string) string {
	switch strings.ToLower(strings.TrimSpace(choice)) {
	case "playfair", "playfair-display":
		// family name của bản static bake (đã rename khỏi "Playfair Display"
		// để picker phân biệt với font variable gốc)
		return "Playfair Display Bold"
	default:
		return "Lilita One"
	}
}

// renderBubbleText render chữ bubble với viền đen.
// Dùng pango: (chứ không phải label:) để font fallback Vietnamese hoạt động.
// Viền tạo bằng morphology Dilate trên alpha channel của black text layer.
func renderBubbleText(text, fontFamily string, fontSize int, fill, stroke string, strokeW int, outPath string) (int, int, error) {
	pango := func(color string) string {
		return fmt.Sprintf("pango:<span font='%s %d' foreground='%s'>%s</span>",
			fontFamily, fontSize, color, escapePango(text))
	}
	args := []string{
		"(", "-background", "none", pango(stroke),
		"-channel", "A", "-morphology", "Dilate", fmt.Sprintf("Disk:%d", strokeW), "+channel",
		")",
		"(", "-background", "none", pango(fill), ")",
		"-gravity", "center", "-compose", "over", "-composite",
		outPath,
	}
	if out, err := magickCmd(args...).CombinedOutput(); err != nil {
		return 0, 0, fmt.Errorf("renderBubbleText: %v\n%s", err, out)
	}
	return identifySize(outPath)
}

// escapePango XML-escape cho pango markup.
func escapePango(s string) string {
	r := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		"'", "&apos;",
		"\"", "&quot;",
	)
	return r.Replace(s)
}

// identifySize đọc width/height của PNG.
func identifySize(path string) (int, int, error) {
	out, err := magickCmd("identify", "-format", "%wx%h", path).Output()
	if err != nil {
		return 0, 0, err
	}
	parts := strings.SplitN(strings.TrimSpace(string(out)), "x", 2)
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("identify format: %q", out)
	}
	w, _ := strconv.Atoi(parts[0])
	h, _ := strconv.Atoi(parts[1])
	return w, h, nil
}
