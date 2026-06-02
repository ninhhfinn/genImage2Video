package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/disintegration/imaging"
	"github.com/fogleman/gg"
	"img2video/textrender"
)

// ─── Thumbnail (static collage image) ──────────────────────────────────────
//
// Builds a vertical 3:4 social-post collage from a listing's photos + info:
//   • a full-bleed hero photo (photo #1), darkened for mood + legibility
//   • a right-side column of 3 white-bordered "polaroid" photos (photos #2-4)
//   • a big rounded title (Baloo 2) with an adjustable colored outline
//   • a bottom-left data block: address · price · amenities, each line a pill
//     whose translucent background hugs the line's own width
//   • a small @handle watermark over the hero
//
// All text is drawn through the existing textrender package; the photo
// compositing is done here in pure Go (imaging + gg). No ffmpeg / ImageMagick.

// ThumbnailConfig is the resolved (defaults-applied) input to buildThumbnailImage.
type ThumbnailConfig struct {
	Width, Height int

	Title     string   // big rounded title ("Sunset")
	Address    string   // address line (───→ "Tại <address>")
	Prices     []string // compact price strings, joined onto one line
	Amenities  []string // amenities, wrapped ~3 per line, max 2 lines
	Watermark  string   // @handle shown over the hero

	// Title styling
	TitleFont   string  // default Baloo2-Variable.ttf
	TitleColor  string  // default #FFFFFF
	StrokeColor string  // outline color, default #b3471f (brick)
	StrokeWidth float64 // outline width in px @ 1080w, default 6

	// Data styling
	BodyFont  string  // default BeVietnamPro-Bold.ttf
	BodyColor string  // default #FFFFFF
	PillColor string  // pill background color, default #000000
	PillAlpha float64 // pill opacity 0..1, default 0.32

	// Text-size multipliers (UI sliders). 1 = default.
	TitleScale float64
	DataScale  float64

	// Layout template: "" / "classic" = hero+polaroid (mặc định cũ);
	// "daiky" | "valey" | "peony" | "tiger" = lưới 2×2 + khối chữ (mockup duyệt).
	Template   string
	PriceTable []ThumbPriceRow // bảng giá cho template "valey"
	ListingID  string          // dòng "Link ID" cho template "tiger"
}

// ThumbPriceRow = 1 dòng bảng giá Valey (khung giờ + giá trong tuần / cuối tuần).
type ThumbPriceRow struct {
	Slot    string `json:"slot"`
	Weekday string `json:"weekday"`
	Weekend string `json:"weekend"`
}

// thumbDefaults fills any unset field with a sensible default.
func (c *ThumbnailConfig) applyDefaults() {
	if c.Width <= 0 {
		c.Width = 960
	}
	if c.Height <= 0 {
		c.Height = 1280
	}
	if strings.TrimSpace(c.TitleFont) == "" {
		// Static Bold (wght=700) instance — the variable font renders at its
		// default master (wght=400, thin) under freetype, unlike the CSS mockup.
		c.TitleFont = "Baloo2-Bold.ttf"
	}
	if strings.TrimSpace(c.TitleColor) == "" {
		c.TitleColor = "#FFFFFF"
	}
	if strings.TrimSpace(c.StrokeColor) == "" {
		c.StrokeColor = "#b3471f"
	}
	if c.StrokeWidth <= 0 {
		c.StrokeWidth = 6
	}
	if strings.TrimSpace(c.BodyFont) == "" {
		c.BodyFont = "BeVietnamPro-Bold.ttf"
	}
	if strings.TrimSpace(c.BodyColor) == "" {
		c.BodyColor = "#FFFFFF"
	}
	if strings.TrimSpace(c.PillColor) == "" {
		c.PillColor = "#000000"
	}
	if c.PillAlpha <= 0 {
		c.PillAlpha = 0.32
	}
	if c.TitleScale <= 0 {
		c.TitleScale = 1
	}
	if c.DataScale <= 0 {
		c.DataScale = 1
	}
}

// ─── HTTP ───────────────────────────────────────────────────────────────

// ThumbnailRequest is the JSON body for POST /api/render-thumbnail.
type ThumbnailRequest struct {
	Title     string   `json:"title"`
	Address   string   `json:"address"`
	Prices    []string `json:"prices"`
	Amenities []string `json:"amenities"`
	Watermark string   `json:"watermark"`

	Width  int `json:"width"`
	Height int `json:"height"`

	TitleFont   string  `json:"title_font"`
	TitleColor  string  `json:"title_color"`
	StrokeColor string  `json:"stroke_color"`
	StrokeWidth float64 `json:"stroke_width"`
	BodyFont    string  `json:"body_font"`
	BodyColor   string  `json:"body_color"`
	PillColor   string  `json:"pill_color"`
	PillAlpha   float64 `json:"pill_alpha"`
	TitleScale  float64 `json:"title_scale"`
	DataScale   float64 `json:"data_scale"`

	// Optional explicit photo order [hero, polaroid1, polaroid2, polaroid3] as
	// indices into the uploaded photos. Empty → natural upload order.
	Photos []int `json:"photos"`

	Template   string          `json:"thumb_template"` // "" classic | daiky | valey | peony | tiger
	PriceTable []ThumbPriceRow `json:"price_table"`    // bảng giá cho valey

	// Batch render sets these so each listing's thumbnail is persisted to the
	// thumbnail history (the single manual preview leaves SaveHistory false).
	ListingName string `json:"listing_name"`
	ListingID   string `json:"listing_id"`
	SaveHistory bool   `json:"save_history"`
}

// thumbnailHandler renders a still collage from the already-uploaded photos and
// the supplied listing text, writes <outputDir>/thumbnail.jpg and returns its URL.
func thumbnailHandler(uploadDir, outputDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "method not allowed", 405)
			return
		}
		var req ThumbnailRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			thumbJSONErr(w, "body không hợp lệ", 400)
			return
		}
		images, err := collectImages(uploadDir)
		if err != nil {
			thumbJSONErr(w, err.Error(), 400)
			return
		}
		photos := selectThumbPhotos(images, req.Photos)

		cfg := ThumbnailConfig{
			Width: req.Width, Height: req.Height,
			Title: req.Title, Address: req.Address, Prices: req.Prices,
			Amenities: req.Amenities, Watermark: req.Watermark,
			TitleFont: req.TitleFont, TitleColor: req.TitleColor,
			StrokeColor: req.StrokeColor, StrokeWidth: req.StrokeWidth,
			BodyFont: req.BodyFont, BodyColor: req.BodyColor,
			PillColor: req.PillColor, PillAlpha: req.PillAlpha,
			TitleScale: req.TitleScale, DataScale: req.DataScale,
			Template: req.Template, PriceTable: req.PriceTable, ListingID: req.ListingID,
		}
		data, err := buildThumbnailImage(cfg, photos)
		if err != nil {
			thumbJSONErr(w, err.Error(), 500)
			return
		}
		os.MkdirAll(outputDir, 0755)
		// Always refresh the single live-preview file (manual "Tạo Thumbnail").
		if err := os.WriteFile(filepath.Join(outputDir, "thumbnail.jpg"), data, 0644); err != nil {
			thumbJSONErr(w, err.Error(), 500)
			return
		}

		resp := map[string]interface{}{
			"status": "ok",
			"url":    fmt.Sprintf("/api/thumbnail-file?ts=%d", time.Now().UnixNano()),
			"size":   len(data),
		}

		// Batch mode: persist a per-listing copy and record it in the thumbnail
		// history so all listings' thumbnails show up together.
		if req.SaveHistory {
			name := req.ListingName
			if name == "" {
				currentListing.mu.Lock()
				name = currentListing.Name
				currentListing.mu.Unlock()
			}
			if fname, err := saveThumbnailFile(data, outputDir, req.ListingID); err == nil {
				addThumbnailToHistory(ThumbnailRecord{
					ListingName: name,
					ListingID:   req.ListingID,
					FileName:    fname,
					RenderTime:  time.Now().Format("02/01/2006 15:04:05"),
				})
				resp["filename"] = fname
				resp["url"] = "/api/thumbnail-file?name=" + fname
			}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}
}

// thumbnailFileHandler serves the last-rendered thumbnail.jpg (no caching so the
// preview refreshes on every render).
func thumbnailFileHandler(outputDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// ?name=<persisted thumbnail> serves a history item (immutable, cacheable);
		// no name → the live single-preview thumbnail.jpg (never cached).
		name := "thumbnail.jpg"
		named := false
		if n := r.URL.Query().Get("name"); n != "" {
			name = filepath.Base(n) // path-safe: strip any directory components
			named = true
		}
		f := filepath.Join(outputDir, name)
		if _, err := os.Stat(f); err != nil {
			http.Error(w, "not ready", 404)
			return
		}
		if named {
			w.Header().Set("Cache-Control", "public, max-age=3600")
		} else {
			w.Header().Set("Cache-Control", "no-store, must-revalidate")
		}
		w.Header().Set("Content-Type", "image/jpeg")
		http.ServeFile(w, r, f)
	}
}

// selectThumbPhotos picks the hero + polaroid photos. With explicit indices it
// follows them (clamped to valid range); otherwise it uses natural upload order.
func selectThumbPhotos(images []string, idx []int) []string {
	if len(idx) == 0 {
		return images
	}
	var out []string
	for _, i := range idx {
		if i >= 0 && i < len(images) {
			out = append(out, images[i])
		}
	}
	if len(out) == 0 {
		return images
	}
	return out
}

func thumbJSONErr(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// buildThumbnailImage applies defaults then dispatches to the chosen layout.
func buildThumbnailImage(cfg ThumbnailConfig, photos []string) ([]byte, error) {
	cfg.applyDefaults()
	if len(photos) == 0 {
		return nil, fmt.Errorf("thumbnail: cần ít nhất 1 ảnh")
	}
	switch strings.ToLower(strings.TrimSpace(cfg.Template)) {
	case "daiky", "valey", "peony", "tiger":
		return buildGridThumbnail(cfg, photos)
	default:
		return buildClassicThumbnail(cfg, photos) // hero + polaroid (mặc định cũ)
	}
}

// buildClassicThumbnail composites the hero + 3-polaroid collage and returns JPEG.
func buildClassicThumbnail(cfg ThumbnailConfig, photos []string) ([]byte, error) {
	W, H := cfg.Width, cfg.Height

	dc := gg.NewContext(W, H)

	// ── 1. Hero: crop-fill the whole canvas, darken a touch ──
	hero, err := imaging.Open(photos[0], imaging.AutoOrientation(true))
	if err != nil {
		return nil, fmt.Errorf("thumbnail: mở ảnh hero: %v", err)
	}
	hero = imaging.Fill(hero, W, H, imaging.Center, imaging.Lanczos)
	hero = imaging.AdjustBrightness(hero, -16)
	hero = imaging.AdjustSaturation(hero, 6)
	dc.DrawImage(hero, 0, 0)

	// ── 2. Legibility gradients (bottom + left scrim) ──
	drawBottomLeftScrim(dc, W, H)

	// ── 3. Polaroid stack on the right (photos #2-4) ──
	drawPolaroids(dc, cfg, photos)

	// ── 4. Text overlays via textrender ──
	ctx := &textrender.RenderContext{VideoWidth: W, VideoHeight: H, AssetsDir: assetsDir()}
	if err := drawThumbnailText(dc, cfg, ctx); err != nil {
		return nil, err
	}

	// ── 5. Encode JPEG ──
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, dc.Image(), &jpeg.Options{Quality: 92}); err != nil {
		return nil, fmt.Errorf("thumbnail: encode: %v", err)
	}
	return buf.Bytes(), nil
}

// drawBottomLeftScrim darkens the lower and left edges so the title/data and
// watermark stay readable over a bright hero.
func drawBottomLeftScrim(dc *gg.Context, W, H int) {
	fw, fh := float64(W), float64(H)

	// bottom: transparent at 42% height → dark at the very bottom
	bottom := gg.NewLinearGradient(0, fh*0.42, 0, fh)
	bottom.AddColorStop(0, color.RGBA{10, 5, 3, 0})
	bottom.AddColorStop(1, color.RGBA{10, 5, 3, 200})
	dc.SetFillStyle(bottom)
	dc.DrawRectangle(0, 0, fw, fh)
	dc.Fill()

	// left: subtle dark at the left edge → transparent past mid-width
	left := gg.NewLinearGradient(0, 0, fw*0.55, 0)
	left.AddColorStop(0, color.RGBA{10, 5, 3, 110})
	left.AddColorStop(1, color.RGBA{10, 5, 3, 0})
	dc.SetFillStyle(left)
	dc.DrawRectangle(0, 0, fw, fh)
	dc.Fill()
}

// drawPolaroids paints up to 3 white-bordered, drop-shadowed photo insets down
// the right side, from photos[1:].
func drawPolaroids(dc *gg.Context, cfg ThumbnailConfig, photos []string) {
	W, H := cfg.Width, cfg.Height
	polW := int(0.333 * float64(W)) // ~360
	polH := int(0.242 * float64(H)) // ~348
	rightMargin := int(0.037 * float64(W))
	polX := W - rightMargin - polW
	border := int(0.012 * float64(W)) // ~13
	radius := 3.0

	tops := []int{
		int(0.056 * float64(H)), // ~80
		int(0.325 * float64(H)), // ~468
		int(0.608 * float64(H)), // ~876
	}

	for i := 0; i < 3 && i+1 < len(photos); i++ {
		polY := tops[i]

		// drop shadow
		if sh := roundedShadow(polW, polH, radius+6, 16, 0.5); sh != nil {
			pad := sh.Bounds().Dx() - polW // = 2*shadowPad
			dc.DrawImage(sh, polX-pad/2, polY-pad/2+12)
		}

		// white frame
		dc.SetColor(color.RGBA{253, 252, 249, 255})
		dc.DrawRoundedRectangle(float64(polX), float64(polY), float64(polW), float64(polH), radius)
		dc.Fill()

		// photo inset (crop-fill the inner area)
		innerW := polW - 2*border
		innerH := polH - 2*border
		if img, err := imaging.Open(photos[i+1], imaging.AutoOrientation(true)); err == nil {
			img = imaging.Fill(img, innerW, innerH, imaging.Center, imaging.Lanczos)
			dc.DrawImage(img, polX+border, polY+border)
		}
	}
}

// roundedShadow returns a blurred black rounded-rect, sized w×h plus padding for
// the blur on every side. alpha is 0..1.
func roundedShadow(w, h int, radius, blur, alpha float64) image.Image {
	pad := int(blur*3) + 2
	sc := gg.NewContext(w+2*pad, h+2*pad)
	sc.SetRGBA(0, 0, 0, 0)
	sc.Clear()
	sc.SetRGBA(0, 0, 0, alpha)
	sc.DrawRoundedRectangle(float64(pad), float64(pad), float64(w), float64(h), radius)
	sc.Fill()
	return imaging.Blur(sc.Image(), blur)
}

// drawThumbnailText lays out the title, the per-line data pills and the
// watermark, drawing each as a textrender PNG composited onto dc.
func drawThumbnailText(dc *gg.Context, cfg ThumbnailConfig, ctx *textrender.RenderContext) error {
	W, H := cfg.Width, cfg.Height
	leftX := int(0.052 * float64(W)) // pill box left ~56
	padH := 18.0                     // sunset.json pill padding [8,18]
	padV := 8.0
	gap := int(0.0049 * float64(H)) // ~7
	bottomMargin := int(0.132 * float64(H))
	dataBottom := H - bottomMargin
	textLeft := leftX + int(padH) // text left, also title content-left

	// Keep data pills clear of the polaroid column: long lines wrap instead of
	// running under the photos on the right. (Mirror drawPolaroids' geometry.)
	polW := int(0.333 * float64(W))
	rightMargin := int(0.037 * float64(W))
	polLeft := W - rightMargin - polW
	dataMaxWidthPct := float64(polLeft-18-leftX-int(2*padH)) / float64(W)
	if dataMaxWidthPct < 0.3 {
		dataMaxWidthPct = 0.3
	}

	// ── Data pills: sunset-style dark pill + soft shadow, each hugging its line.
	//    Stacking accounts for the shadow margin baked into each PNG. ──
	lines := buildDataLines(cfg)
	pillShadow := &textrender.ShadowStyle{Color: "#000000", Alpha: 0.6, Blur: 8, OffsetX: 0, OffsetY: 2}
	type pill struct {
		img      image.Image
		x        int // composite x (content-left lands at leftX)
		contentH int
		margin   int
	}
	var pills []pill
	for i, ln := range lines {
		size := 0.0240 * cfg.DataScale // ~26px @ 1080
		if i == 0 {
			size = 0.0250 * cfg.DataScale // address line slightly larger
		}
		st := &textrender.ElementStyle{
			Text:        ln,
			FontFile:    cfg.BodyFont,
			SizePct:     size,
			Color:       cfg.BodyColor,
			Bg:          &textrender.BgStyle{Color: cfg.PillColor, Alpha: cfg.PillAlpha, Radius: 16, Padding: [2]float64{padV, padH}},
			Shadow:      pillShadow,
			Position:    textrender.Position{X: fmt.Sprintf("%dpx", leftX), Y: "0px"},
			Align:       "left",
			LineSpacing: 1.15,
			MaxWidthPct: dataMaxWidthPct,
		}
		out, err := textrender.Render(st, ctx)
		if err != nil || out == nil {
			continue
		}
		img, err := png.Decode(bytes.NewReader(out.PNG))
		if err != nil {
			continue
		}
		m := styleMargin(st)
		pills = append(pills, pill{img, out.X, out.Height - 2*m, m})
	}

	totalH := 0
	for i, p := range pills {
		totalH += p.contentH
		if i < len(pills)-1 {
			totalH += gap
		}
	}
	pillsTop := dataBottom - totalH
	cur := pillsTop
	for _, p := range pills {
		dc.DrawImage(p.img, p.x, cur-p.margin) // content-top lands at cur
		cur += p.contentH + gap
	}

	// ── Title (outline + shadow), placed just above the pills. Auto-shrinks to
	//    fit the width left of the polaroid column so it never overlaps a photo. ──
	if strings.TrimSpace(cfg.Title) != "" {
		shadow := &textrender.ShadowStyle{Color: "#000000", Alpha: 0.45, Blur: 12, OffsetX: 0, OffsetY: 4}
		sizePct := 0.105 * cfg.TitleScale // ~113px @ 1080, smaller default than before
		maxTitleW := float64(polLeft - textLeft - 14)

		mkTitle := func(sp float64) *textrender.ElementStyle {
			return &textrender.ElementStyle{
				Text:     cfg.Title,
				FontFile: cfg.TitleFont,
				SizePct:  sp,
				Color:    cfg.TitleColor,
				Stroke:   &textrender.StrokeStyle{Color: cfg.StrokeColor, Width: cfg.StrokeWidth},
				Shadow:   shadow,
				Position: textrender.Position{X: fmt.Sprintf("%dpx", textLeft), Y: "0px"},
				Align:    "left",
			}
		}

		title := mkTitle(sizePct)
		out, err := textrender.Render(title, ctx)
		if err == nil && out != nil {
			m := styleMargin(title)
			contentW := out.Width - 2*m
			// shrink-to-fit if the title would run under the polaroids
			if float64(contentW) > maxTitleW && contentW > 0 {
				title = mkTitle(sizePct * maxTitleW / float64(contentW))
				out, err = textrender.Render(title, ctx)
				m = styleMargin(title)
			}
		}
		if err == nil && out != nil {
			if img, derr := png.Decode(bytes.NewReader(out.PNG)); derr == nil {
				m := styleMargin(title)
				contentH := out.Height - 2*m
				titleGap := int(0.006 * float64(H))
				desiredContentTop := pillsTop - titleGap - contentH
				dc.DrawImage(img, out.X, desiredContentTop-m)
			}
		}
	}

	// ── Watermark (@handle) + music note, over the hero on the right ──
	if strings.TrimSpace(cfg.Watermark) != "" {
		wmX := int(0.413 * float64(W))
		wmY := int(0.572 * float64(H))
		wm := &textrender.ElementStyle{
			Text:     cfg.Watermark,
			FontFile: cfg.BodyFont,
			SizePct:  0.0241,
			Color:    cfg.BodyColor,
			Shadow:   &textrender.ShadowStyle{Color: "#000000", Alpha: 0.65, Blur: 6, OffsetX: 0, OffsetY: 2},
			Position: textrender.Position{X: fmt.Sprintf("%dpx", wmX), Y: fmt.Sprintf("%dpx", wmY)},
			Align:    "left",
		}
		if out, err := textrender.Render(wm, ctx); err == nil && out != nil {
			if img, derr := png.Decode(bytes.NewReader(out.PNG)); derr == nil {
				noteSize := 0.027 * float64(W)
				drawMusicNote(dc, float64(wmX)-noteSize-9, float64(wmY)-2, noteSize, color.RGBA{255, 255, 255, 255})
				dc.DrawImage(img, out.X, out.Y)
			}
		}
	}
	return nil
}

// styleMargin replicates textrender's outer-margin math (max of shadow + stroke
// extent) so we can align an element's content box precisely.
func styleMargin(s *textrender.ElementStyle) int {
	shadowExtent := 0.0
	if s.Shadow != nil {
		shadowExtent = s.Shadow.Blur*3 + math.Max(math.Abs(s.Shadow.OffsetX), math.Abs(s.Shadow.OffsetY))
	}
	strokeExtent := 0.0
	if s.Stroke != nil {
		strokeExtent = s.Stroke.Width
	}
	return int(math.Max(shadowExtent, strokeExtent))
}

// buildDataLines turns the listing fields into the bottom-left text lines:
// "Tại <address>", a single price line, then amenities wrapped ~3 per line (max 2).
func buildDataLines(cfg ThumbnailConfig) []string {
	var lines []string

	if addr := strings.TrimSpace(cfg.Address); addr != "" {
		low := strings.ToLower(addr)
		if !strings.HasPrefix(low, "tại") {
			addr = "Tại " + addr
		}
		lines = append(lines, addr)
	}

	var prices []string
	for _, p := range cfg.Prices {
		if p = strings.TrimSpace(p); p != "" {
			prices = append(prices, p)
		}
	}
	if len(prices) > 0 {
		lines = append(lines, strings.Join(prices, "- "))
	}

	var amen []string
	for _, a := range cfg.Amenities {
		if a = strings.TrimSpace(a); a != "" {
			amen = append(amen, a)
		}
	}
	const perLine, maxLines = 3, 2
	var amenLines []string
	for i := 0; i < len(amen) && len(amenLines) < maxLines; i += perLine {
		end := i + perLine
		if end > len(amen) {
			end = len(amen)
		}
		line := strings.Join(amen[i:end], "- ")
		// trailing "-" on a non-final line mimics the "continues below" look
		if end < len(amen) && len(amenLines) < maxLines-1 {
			line += "-"
		}
		amenLines = append(amenLines, line)
	}
	lines = append(lines, amenLines...)
	return lines
}

// drawMusicNote draws a small white eighth-note at (x,y) within a size×size box.
func drawMusicNote(dc *gg.Context, x, y, size float64, col color.Color) {
	dc.Push()
	dc.SetColor(col)
	headR := size * 0.26
	hx := x + headR
	hy := y + size - headR*0.9
	stemX := hx + headR*0.95
	// stem
	dc.DrawRectangle(stemX-size*0.05, y, size*0.10, size-headR*0.9)
	dc.Fill()
	// flag
	dc.MoveTo(stemX, y)
	dc.QuadraticTo(x+size*1.05, y+size*0.16, stemX, y+size*0.42)
	dc.LineTo(stemX-size*0.06, y+size*0.42)
	dc.LineTo(stemX-size*0.06, y)
	dc.ClosePath()
	dc.Fill()
	// note head
	dc.DrawEllipse(hx, hy, headR*1.15, headR*0.92)
	dc.Fill()
	dc.Pop()
}
