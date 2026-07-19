package main

import (
	"bufio"
	"flag"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"

	"img2video/textrender"
)

type Config struct {
	Input         string
	Output        string
	Mode          string
	Total         float64
	Duration      float64
	FPS           int
	ZoomIntensity float64
	InputFPS      float64
	OutputFPS     int
	Audio         string
	Title         string
	TitleDuration float64
	Watermark     string
	TextFont      string
	TextScale     float64
	TextColor     string
	TitleFontFile string // uploaded/custom font file for title + nickname (relative to assets/fonts)
	Width         int
	Height        int
	Tiktok        bool
	Verbose       bool
	DryRun        bool

	// Listing overlay (persistent, hiển thị xuyên suốt video)
	Address       string   // dòng địa chỉ ở đầu video
	Nickname      string   // text lớn ở giữa (vd "6OIYH")
	ListingID     string   // ID hiển thị ở cuối
	Prices        []string // các dòng giá hiển thị giữa video
	Amenities     []string // tiện nghi (Máy chiếu netflix- Bếp nấu- ...)
	EffectType    string   // "zoom-in" | "zoom-out" | "move-left-right" | "move-right-left" | "random"
	EffectTypes   []string // 1 kiểu cố định, hoặc ["random"]: mỗi ảnh random đúng 1 nhóm hiệu ứng
	OverlayStyle  string   // "badge" (Daiky home, brown box) | "bubble" (Sunset, no box, lower-left)
	OverlayFont   string   // "playfair" | "lilita"
	OverlayScale  float64  // scale chữ overlay
	OverlayText   string   // màu chữ nội dung (address/prices/amenities/listing_id)
	OverlayBG     string   // màu nền badge
	OverlayStroke string   // màu viền bubble/title (legacy → shadow)
	TitleColor    string   // màu chữ tiêu đề (title/nickname)
	StrokeColor   string   // màu viền chữ tiêu đề (stroke outline), vd sunset
	TitleBg       string   // màu nền pill tiêu đề (title/nickname)
	BodyBg        string   // màu nền pill nội dung (address/prices/amenities)
	GridIntro     bool     // chèn 1 frame lưới 2x2 tĩnh ở đầu video rồi về Ken Burns

	// Template system (mới — thay thế OverlayStyle dần)
	Template     string                              // "daiky" | "sunset"
	CustomStyles map[string]*textrender.ElementStyle // override per-element

	// Tự đăng social qua webhook (Make.com / n8n) sau khi render xong
	AutoPost   bool     // bật gửi video tới webhook
	WebhookURL string   // URL webhook nhận video + metadata
	Platforms  []string // ["tiktok","facebook"]

	// Thuyết minh AI: lời kể AI (Claude Vision) + giọng đọc TTS + phụ đề.
	// Narrate là cờ ĐỘC LẬP — ghép được với mọi motion mode (kenburns/slideshow).
	Narrate          bool   // bật thuyết minh AI (không còn là 1 giá trị của Mode)
	NarrationPersona string // "haihuoc" (mặc định) | "lichsu"
	TTSProvider      string // "google" | "elevenlabs" | "fpt"
	VoiceID          string // voice id/name cho provider; "" → mặc định env
	MaxSegments      int    // số cảnh tối đa (3–15); 0 → 10
	TargetDuration   int    // narrated: thời lượng mục tiêu (giây); 0 → không giới hạn
	SubtitleStyle    string // narrated: "karaoke" (mặc định) | "typewriter"

	// Kịch bản do user duyệt/sửa từ panel FE (nil = để Claude viết).
	Script *NarrationScript
	// ForceScript: bỏ qua cache kịch bản (nút "Viết lại" trong panel FE).
	ForceScript bool
}

var supportedExts = map[string]bool{
	".jpg": true, ".jpeg": true, ".png": true, ".webp": true, ".bmp": true,
}

type effect struct {
	name string
	z    string
	x    string
	y    string
}

// filterEffects trả về subset hiệu ứng theo loại
func filterEffects(all []effect, kind string) []effect {
	if kind == "" || kind == "mixed" || kind == "subtle" {
		return all
	}
	var out []effect
	for _, e := range all {
		switch kind {
		case "zoom-in":
			if strings.HasPrefix(e.name, "zoom-in") {
				out = append(out, e)
			}
		case "zoom-out":
			if strings.HasPrefix(e.name, "zoom-out") {
				out = append(out, e)
			}
		case "pan":
			if strings.HasPrefix(e.name, "pan") {
				out = append(out, e)
			}
		case "move-left-right", "pan-right":
			if strings.HasPrefix(e.name, "pan →right") {
				out = append(out, e)
			}
		case "move-right-left", "pan-left":
			if strings.HasPrefix(e.name, "pan ←left") {
				out = append(out, e)
			}
		case "drift":
			if strings.HasPrefix(e.name, "zoom-in + drift") {
				out = append(out, e)
			}
		}
	}
	return out
}

func filterEffectsForKinds(all []effect, kinds []string, fallback string) []effect {
	if len(kinds) == 0 && fallback != "" {
		kinds = []string{fallback}
	}
	if len(kinds) == 0 {
		return all
	}

	seen := map[string]bool{}
	var out []effect
	for _, kind := range kinds {
		for _, e := range filterEffects(all, kind) {
			if seen[e.name] {
				continue
			}
			seen[e.name] = true
			out = append(out, e)
		}
	}
	if len(out) == 0 {
		return all
	}
	return out
}

func effectIsRandom(cfg Config) bool {
	if strings.EqualFold(cfg.EffectType, "random") {
		return true
	}
	for _, k := range cfg.EffectTypes {
		if strings.EqualFold(k, "random") {
			return true
		}
	}
	return false
}

// pickRandomVideoTemplate trả về tên 1 template video ngẫu nhiên trong
// assets/templates (đúng danh sách mà picker hiển thị). Dùng khi user chọn
// "random": mỗi video sẽ tự bốc 1 mẫu. Fallback "daiky" nếu không đọc được.
func pickRandomVideoTemplate() string {
	list, err := textrender.ListTemplates(assetsTemplatesDir())
	if err != nil || len(list) == 0 {
		return "daiky"
	}
	names := make([]string, 0, len(list))
	for _, t := range list {
		if strings.EqualFold(t.Name, "random") {
			continue
		}
		names = append(names, t.Name)
	}
	if len(names) == 0 {
		return "daiky"
	}
	return names[rand.Intn(len(names))]
}

func imageOrientation(path string) (string, error) {
	w, h, err := imageDimensions(path)
	if err != nil {
		return "", err
	}
	if w > h {
		return "landscape", nil
	}
	if h > w {
		return "portrait", nil
	}
	return "square", nil
}

func imageDimensions(path string) (int, int, error) {
	f, err := os.Open(path)
	if err == nil {
		defer f.Close()
		cfg, _, decodeErr := image.DecodeConfig(f)
		if decodeErr == nil && cfg.Width > 0 && cfg.Height > 0 {
			return cfg.Width, cfg.Height, nil
		}
	}

	if _, lookErr := exec.LookPath("ffprobe"); lookErr == nil {
		out, probeErr := exec.Command(
			"ffprobe",
			"-v", "error",
			"-select_streams", "v:0",
			"-show_entries", "stream=width,height",
			"-of", "csv=s=x:p=0",
			path,
		).Output()
		if probeErr == nil {
			parts := strings.Split(strings.TrimSpace(string(out)), "x")
			if len(parts) == 2 {
				w, wErr := strconv.Atoi(parts[0])
				h, hErr := strconv.Atoi(parts[1])
				if wErr == nil && hErr == nil && w > 0 && h > 0 {
					return w, h, nil
				}
			}
		}
	}

	if err != nil {
		return 0, 0, err
	}
	return 0, 0, fmt.Errorf("không đọc được kích thước ảnh")
}

// buildEffects tạo danh sách hiệu ứng Ken Burns thực sự:
// zoom và pan xảy ra ĐỒNG THỜI, tạo cảm giác mắt đang di chuyển qua ảnh.
//
// Có 2 nhóm:
//
//	A) Zoom-in vào một góc/cạnh cụ thể (focal point cố định — zoom kinh điển)
//	B) Zoom cố định + pan ngang/dọc qua ảnh  (như camera đang lia)
//
// buildEffects tạo các hiệu ứng Ken Burns ổn định dùng LINEAR INTERPOLATION.
// Thay vì cộng dồn per-frame (gây rung), dùng công thức:
//
//	position = start + (end - start) * progress
//
// với progress = on/d (0.0 → 1.0 trong suốt clip)
//
// zDelta: tổng lượng zoom (0.08–0.23 tuỳ intensity)
// frames: số frame của clip
func buildEffects(zDelta float64, frames int) []effect {
	d := float64(frames)

	// Zoom tuyến tính: z = 1.0 + zDelta * (on/d)
	zIn := fmt.Sprintf("1+(%.5f*on/%d)", zDelta, frames)
	// Zoom-out tuyến tính: z = (1+zDelta) - zDelta*(on/d)
	zOut := fmt.Sprintf("max(1.001,(%.3f)-(%.5f*on/%d))", 1.0+zDelta, zDelta, frames)
	// Zoom cố định
	zFix := fmt.Sprintf("%.4f", 1.0+zDelta)

	// Helper: lerp(a, b, t) = a + (b-a)*t  với t = on/d
	lerp := func(a, b, t string) string {
		return fmt.Sprintf("(%s)+((%s)-(%s))*(%s)", a, b, a, t)
	}
	t := fmt.Sprintf("on/%d", frames) // progress 0→1

	// ── FIX BOUNCE: ──
	// Khi zoom = z, FFmpeg crop ra vùng có size = iw/z * ih/z
	// → vị trí pan x hợp lệ: 0 → (iw - iw/z) = iw*(1 - 1/z) = iw*(z-1)/z
	// Với z = 1+zDelta: max pan = iw * zDelta / (1+zDelta)
	// Áp dụng safety margin 0.95 để không bao giờ chạm cạnh → KHÔNG còn bounce
	safety := 0.92
	zEnd := 1.0 + zDelta
	maxPanXf := 1080.0 * zDelta / zEnd * safety
	maxPanYf := 1920.0 * zDelta / zEnd * safety
	maxPanX := fmt.Sprintf("%.2f", maxPanXf)
	maxPanY := fmt.Sprintf("%.2f", maxPanYf)

	// Center x/y khi zoom = zEnd: (iw - iw/z)/2 = iw*(z-1)/(2*z)
	cxf := 1080.0 * zDelta / (2.0 * zEnd)
	cyf := 1920.0 * zDelta / (2.0 * zEnd)
	cx := fmt.Sprintf("%.2f", cxf)
	cy := fmt.Sprintf("%.2f", cyf)

	// Cho zoom-in (z = 1 → 1+zDelta), center thay đổi theo z:
	// Tại on=0: z≈1, center≈0
	// Tại on=d: z=1+zDelta, center=iw*zDelta/(2*(1+zDelta))
	// Dùng linear approximation để tránh nhảy: center scaled by progress
	cxIn := fmt.Sprintf("(%.2f)*(on/%d)", cxf, frames)
	cyIn := fmt.Sprintf("(%.2f)*(on/%d)", cyf, frames)

	// Góc cho zoom-in: vị trí cố định (góc), không nhảy
	cornerX0 := "0"                                // góc trái
	cornerY0 := "0"                                // góc trên
	cornerXR := fmt.Sprintf("%.2f", maxPanXf*1.05) // góc phải (gần cạnh nhưng có safety)
	cornerYB := fmt.Sprintf("%.2f", maxPanYf*1.05) // góc dưới

	_ = d

	return []effect{
		// ── Nhóm A: zoom-in vào điểm cố định ────────────
		{
			"zoom-in → center",
			zIn,
			cxIn, cyIn, // center "tăng dần" theo zoom → không nhảy
		},
		{
			"zoom-in → top-left",
			zIn,
			cornerX0, cornerY0,
		},
		{
			"zoom-in → top-right",
			zIn,
			cornerXR, cornerY0,
		},
		{
			"zoom-in → bottom-left",
			zIn,
			cornerX0, cornerYB,
		},
		{
			"zoom-in → bottom-right",
			zIn,
			cornerXR, cornerYB,
		},
		// ── Zoom-out từ trung tâm (cùng center logic) ─────────
		{
			"zoom-out ← center",
			zOut,
			// Tại on=0 z=zEnd, center=cxf; tại on=d z≈1, center≈0
			// Đảo ngược progress
			fmt.Sprintf("(%.2f)*(1-on/%d)", cxf, frames),
			fmt.Sprintf("(%.2f)*(1-on/%d)", cyf, frames),
		},
		// ── Nhóm B: zoom cố định + pan tuyến tính (đã có safety) ──
		{
			"pan →right",
			zFix,
			lerp("0", maxPanX, t),
			cy,
		},
		{
			"pan ←left",
			zFix,
			lerp(maxPanX, "0", t),
			cy,
		},
		{
			"pan ↓down",
			zFix,
			cx,
			lerp("0", maxPanY, t),
		},
		{
			"pan ↑up",
			zFix,
			cx,
			lerp(maxPanY, "0", t),
		},
		// ── Nhóm C: zoom-in + pan đồng thời ──────────────────
		// Pan range scale theo zoom progress để không vượt cạnh
		{
			"zoom-in + drift →right",
			zIn,
			fmt.Sprintf("(%.2f)*(on/%d)", maxPanXf, frames),
			cyIn,
		},
		{
			"zoom-in + drift ←left",
			zIn,
			fmt.Sprintf("(%.2f)*(1-on/%d)", maxPanXf, frames),
			cyIn,
		},
		{
			"zoom-in + drift ↓down",
			zIn,
			cxIn,
			fmt.Sprintf("(%.2f)*(on/%d)", maxPanYf, frames),
		},
		{
			"zoom-in + drift ↑up",
			zIn,
			cxIn,
			fmt.Sprintf("(%.2f)*(1-on/%d)", maxPanYf, frames),
		},
	}
}

func main() {
	// Kiểm tra --web trước khi parse flags đầy đủ
	for _, arg := range os.Args[1:] {
		if arg == "--web" || arg == "-web" {
			port := 8080
			// Đọc PORT từ env (Railway, Fly.io, Render tự set)
			if envPort := os.Getenv("PORT"); envPort != "" {
				if p, err := strconv.Atoi(envPort); err == nil {
					port = p
				}
			}
			// --port flag ghi đè env
			for i, a := range os.Args[1:] {
				if (a == "--port" || a == "-port") && i+1 < len(os.Args)-1 {
					if p, err := strconv.Atoi(os.Args[i+2]); err == nil {
						port = p
					}
				}
			}
			if err := startWebServer(port); err != nil {
				fmt.Fprintf(os.Stderr, "❌  %v\n", err)
				os.Exit(1)
			}
			return
		}
	}
	cfg := parseFlags()
	if err := run(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "\n❌  Lỗi: %v\n", err)
		os.Exit(1)
	}
}

func parseFlags() Config {
	cfg := Config{}
	flag.StringVar(&cfg.Input, "input", "", "Thư mục ảnh (bắt buộc)")
	flag.StringVar(&cfg.Output, "output", "output.mp4", "File video ra")
	flag.StringVar(&cfg.Mode, "mode", "kenburns", "slideshow | kenburns | timelapse | narrated")
	flag.Float64Var(&cfg.Total, "total", 40, "Tổng thời gian video (giây)")
	flag.Float64Var(&cfg.Duration, "duration", 0, "Giây mỗi ảnh (tự tính nếu dùng --total)")
	flag.IntVar(&cfg.FPS, "fps", 30, "FPS")
	flag.Float64Var(&cfg.ZoomIntensity, "zoom-intensity", 0.5, "Cường độ zoom 0.0–1.0")
	flag.Float64Var(&cfg.InputFPS, "input-fps", 1, "FPS ảnh vào (timelapse)")
	flag.IntVar(&cfg.OutputFPS, "output-fps", 30, "FPS ra (timelapse)")
	flag.StringVar(&cfg.Audio, "audio", "", "Nhạc nền mp3/aac/wav")
	flag.StringVar(&cfg.Title, "title", "", "Tiêu đề đầu video")
	flag.Float64Var(&cfg.TitleDuration, "title-duration", 3, "Số giây hiển thị tiêu đề đầu video")
	flag.StringVar(&cfg.Watermark, "watermark", "", "Watermark góc dưới phải")
	flag.IntVar(&cfg.Width, "width", 1080, "Chiều rộng")
	flag.IntVar(&cfg.Height, "height", 1920, "Chiều cao (1920 = TikTok 9:16)")
	flag.BoolVar(&cfg.Tiktok, "tiktok", true, "9:16 cho TikTok/Reels")
	flag.BoolVar(&cfg.Verbose, "verbose", false, "In lệnh FFmpeg")
	flag.BoolVar(&cfg.DryRun, "dry-run", false, "Chỉ in lệnh, không chạy")
	flag.BoolVar(&cfg.Narrate, "narrate", false, "bật thuyết minh AI (ghép với motion mode)")
	flag.StringVar(&cfg.NarrationPersona, "narration-persona", "haihuoc", "narrated: haihuoc | lichsu")
	flag.StringVar(&cfg.TTSProvider, "tts-provider", "google", "narrated: google | elevenlabs | fpt")
	flag.StringVar(&cfg.VoiceID, "voice-id", "", "narrated: voice id/name (bỏ trống = mặc định)")
	flag.IntVar(&cfg.MaxSegments, "max-segments", 10, "narrated: số cảnh tối đa (3–15)")
	flag.IntVar(&cfg.TargetDuration, "target-duration", 60, "narrated: thời lượng mục tiêu giây (0 = không giới hạn)")
	flag.StringVar(&cfg.SubtitleStyle, "subtitle-style", "karaoke", "narrated: karaoke | typewriter")
	flag.Usage = printUsage
	flag.Parse()
	if cfg.Input == "" {
		fmt.Fprintln(os.Stderr, "❌  Thiếu --input")
		flag.Usage()
		os.Exit(1)
	}
	if cfg.Tiktok {
		cfg.Width, cfg.Height = 1080, 1920
	}
	return cfg
}

func printUsage() {
	fmt.Print(`
╔══════════════════════════════════════════════════════╗
║        img2video — Ảnh → Video TikTok/Reels          ║
╚══════════════════════════════════════════════════════╝

Nhanh nhất (30 giây, 9:16, ken burns):
  img2video --input ./photos --output reel.mp4

Đặt thời gian:
  img2video --input ./photos --output reel.mp4 --total 60

Có nhạc:
  img2video --input ./photos --output reel.mp4 --audio nhac.mp3

Zoom mạnh hơn:
  img2video --input ./photos --output reel.mp4 --zoom-intensity 0.8

16:9 YouTube:
  img2video --input ./photos --output video.mp4 --tiktok=false --width 1920 --height 1080

`)
	flag.PrintDefaults()
}

func run(cfg Config) error {
	if err := checkFFmpeg(); err != nil {
		return err
	}
	images, err := collectImages(cfg.Input)
	if err != nil {
		return err
	}
	fmt.Printf("📁  Tìm thấy %d ảnh\n", len(images))

	if cfg.Duration <= 0 {
		cfg.Duration = cfg.Total / float64(len(images))
		fmt.Printf("⏱️   %.0fs ÷ %d ảnh = %.2fs/ảnh\n", cfg.Total, len(images), cfg.Duration)
	}
	if cfg.Duration < 1.5 {
		cfg.Duration = 1.5
		fmt.Printf("⚠️   Tối thiểu 1.5s/ảnh → tổng %.1fs\n", cfg.Duration*float64(len(images)))
	}

	// Tương thích ngược: mode "narrated" cũ → cờ Narrate + motion mặc định kenburns.
	if cfg.Mode == "narrated" {
		cfg.Narrate = true
		cfg.Mode = "kenburns"
	}

	var args []string
	if cfg.Narrate {
		// Thuyết minh AI ghép lên motion mode đã chọn (buildNarrated tôn trọng cfg.Mode).
		args, err = buildNarrated(cfg, images)
	} else {
		switch cfg.Mode {
		case "slideshow":
			args, err = buildSlideshow(cfg, images)
		case "kenburns":
			args, err = buildKenBurns(cfg, images)
		case "timelapse":
			args, err = buildTimelapse(cfg, images)
		default:
			return fmt.Errorf("mode không hợp lệ: %s", cfg.Mode)
		}
	}
	if err != nil {
		return err
	}
	return runFFmpeg(args, cfg)
}

func checkFFmpeg() error {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return fmt.Errorf("không thấy ffmpeg — tải tại https://ffmpeg.org/download.html")
	}
	return nil
}

func collectImages(dir string) ([]string, error) {
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return nil, fmt.Errorf("thư mục không tồn tại: %s", dir)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var imgs []string
	for _, e := range entries {
		if !e.IsDir() && supportedExts[strings.ToLower(filepath.Ext(e.Name()))] {
			imgs = append(imgs, filepath.Join(dir, e.Name()))
		}
	}
	sort.Strings(imgs)
	if len(imgs) == 0 {
		return nil, fmt.Errorf("không có ảnh jpg/png/webp trong %s", dir)
	}
	return imgs, nil
}

func scaleCrop(w, h int) string {
	// scale lên đủ lớn rồi crop chính giữa — không có viền đen
	return fmt.Sprintf("scale=%d:%d:force_original_aspect_ratio=increase,crop=%d:%d", w, h, w, h)
}

func scalePad(w, h int) string {
	return fmt.Sprintf("scale=%d:%d:force_original_aspect_ratio=decrease,pad=%d:%d:(ow-iw)/2:(oh-ih)/2:color=black", w, h, w, h)
}

func appendTextFilters(cfg Config, vf string) string {
	w := cfg.Width
	textFont := selectedTextFontOption(cfg.TextFont, "regular")
	boldFont := selectedTextFontOption(cfg.TextFont, "bold")
	textScale := safeScale(cfg.TextScale)
	textColor := safeColor(cfg.TextColor, "white")

	// helper escape cho drawtext
	esc := func(s string) string {
		s = strings.ReplaceAll(s, "\\", "\\\\")
		s = strings.ReplaceAll(s, ":", "\\:")
		s = strings.ReplaceAll(s, "'", "\\'")
		s = strings.ReplaceAll(s, "%", "\\%")
		return s
	}

	addText := func(text string, fs int, color string, alpha float64, x, y string, fontOpt string, borderW int, enable string) {
		if text == "" {
			return
		}
		enableExpr := ""
		if enable != "" {
			enableExpr = ":enable='" + enable + "'"
		}
		vf += fmt.Sprintf(
			",drawtext=text='%s'%s:fontsize=%d:fontcolor=%s@%.2f:x=%s:y=%s:borderw=%d:bordercolor=black@0.85:shadowcolor=black@0.65:shadowx=2:shadowy=2%s",
			esc(text), fontOpt, fs, color, alpha, x, y, borderW, enableExpr,
		)
	}

	titleDuration := cfg.TitleDuration
	if titleDuration <= 0 {
		titleDuration = 3
	}
	titleEnable := ""
	if strings.TrimSpace(cfg.Title) != "" {
		titleEnable = fmt.Sprintf("between(t,0,%.3f)", titleDuration)
	}
	// Listing overlay (address/title/prices/amenities) giờ render bằng PNG (overlay.go)
	// thay vì drawtext — xem renderListingOverlayPNG + overlayFilter trong buildKenBurns.

	// ── 5. Title custom (đầu video) ──
	if cfg.Title != "" {
		// Hỗ trợ multi-line: split bằng \n hoặc | để xuống dòng
		lines := strings.FieldsFunc(cfg.Title, func(r rune) bool {
			return r == '\n' || r == '|'
		})
		if len(lines) == 0 {
			lines = []string{cfg.Title}
		}
		baseY := 0.10
		titleFs := int(float64(w/18) * textScale)
		lineH := 0.06
		for i, ln := range lines {
			ln = strings.TrimSpace(ln)
			if ln == "" {
				continue
			}
			// Dòng đầu in to hơn
			fs := titleFs
			if i == 0 && len(lines) > 1 {
				fs = int(float64(w/14) * textScale)
			}
			y := fmt.Sprintf("h*%.3f", baseY+float64(i)*lineH)
			addText(ln, fs, textColor, 1.0, "(w-text_w)/2", y, boldFont, 3, titleEnable)
		}
	}

	// ── 6. Watermark (góc phải dưới) ──
	if cfg.Watermark != "" {
		addText(cfg.Watermark,
			int(float64(w/32)*textScale), textColor, 0.75,
			"w-text_w-24", "h-text_h-24",
			textFont, 3, "")
	}

	return vf
}

func safeScale(v float64) float64 {
	if v <= 0 {
		return 1
	}
	if v < 0.6 {
		return 0.6
	}
	if v > 1.6 {
		return 1.6
	}
	return v
}

func safeColor(v, fallback string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return fallback
	}
	if regexp.MustCompile(`^#[0-9a-fA-F]{6}$`).MatchString(v) ||
		regexp.MustCompile(`^[a-zA-Z]+$`).MatchString(v) {
		return v
	}
	return fallback
}

func selectedTextFontOption(choice, style string) string {
	if err := initFonts(); err == nil {
		if path := selectedFontPath(choice); path != "" {
			return fontFileOption(path)
		}
	}
	return textFontOption(style)
}

func selectedFontPath(choice string) string {
	switch strings.ToLower(strings.TrimSpace(choice)) {
	case "lilita", "lilita-one":
		return lilitaPath
	case "playfair", "playfair-display":
		return playfairPath
	default:
		return ""
	}
}

func fontFileOption(path string) string {
	path = filepath.ToSlash(path)
	path = strings.ReplaceAll(path, "\\", "\\\\")
	path = strings.ReplaceAll(path, ":", "\\:")
	path = strings.ReplaceAll(path, "'", "\\'")
	return ":fontfile='" + path + "'"
}

// wrapJoin nối các pieces bằng sep, xuống dòng khi vượt maxChars; trả tối đa maxLines dòng.
// Nếu một piece riêng lẻ đã dài hơn maxChars, sẽ word-wrap piece đó bằng wrapText để không tràn.
func wrapJoin(pieces []string, sep string, maxChars, maxLines int) []string {
	var lines []string
	current := ""
	capped := func() bool { return maxLines > 0 && len(lines) >= maxLines }
	flush := func() {
		if current != "" {
			lines = append(lines, current)
			current = ""
		}
	}
	for _, p := range pieces {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if len([]rune(p)) > maxChars {
			flush()
			for _, sub := range wrapText(p, maxChars, 0) {
				if capped() {
					return lines
				}
				lines = append(lines, sub)
			}
			continue
		}
		next := p
		if current != "" {
			next = current + sep + p
		}
		if len([]rune(next)) > maxChars && current != "" {
			flush()
			if capped() {
				return lines
			}
			current = p
			continue
		}
		current = next
	}
	if current != "" && !capped() {
		lines = append(lines, current)
	}
	return lines
}

func wrapText(text string, maxChars, maxLines int) []string {
	words := strings.Fields(text)
	if len(words) == 0 {
		return nil
	}
	var lines []string
	current := ""
	for _, word := range words {
		next := word
		if current != "" {
			next = current + " " + word
		}
		if len([]rune(next)) > maxChars && current != "" {
			lines = append(lines, current)
			current = word
			if maxLines > 0 && len(lines) >= maxLines {
				return lines
			}
			continue
		}
		current = next
	}
	if current != "" && (maxLines <= 0 || len(lines) < maxLines) {
		lines = append(lines, current)
	}
	return lines
}

func textFontOption(style string) string {
	var candidates []string
	switch style {
	case "bold":
		candidates = []string{
			`C:\Windows\Fonts\arialbd.ttf`,
			`C:\Windows\Fonts\segoeuib.ttf`,
			`/usr/share/fonts/truetype/dejavu/DejaVuSans-Bold.ttf`,
			`/usr/share/fonts/truetype/liberation2/LiberationSans-Bold.ttf`,
		}
	case "italic":
		candidates = []string{
			`C:\Windows\Fonts\ariali.ttf`,
			`C:\Windows\Fonts\segoeuii.ttf`,
			`/usr/share/fonts/truetype/dejavu/DejaVuSans-BoldOblique.ttf`,
			`/usr/share/fonts/truetype/dejavu/DejaVuSans-Oblique.ttf`,
			`/usr/share/fonts/truetype/liberation2/LiberationSans-BoldItalic.ttf`,
			`/usr/share/fonts/truetype/liberation2/LiberationSans-Italic.ttf`,
		}
	case "serif-bold":
		candidates = []string{
			`C:\Windows\Fonts\timesbd.ttf`,
			`C:\Windows\Fonts\georgiab.ttf`,
			`/usr/share/fonts/truetype/dejavu/DejaVuSerif-Bold.ttf`,
			`/usr/share/fonts/truetype/liberation2/LiberationSerif-Bold.ttf`,
		}
	default:
		candidates = []string{
			`C:\Windows\Fonts\arial.ttf`,
			`C:\Windows\Fonts\segoeui.ttf`,
			`C:\Windows\Fonts\tahoma.ttf`,
			`/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf`,
			`/usr/share/fonts/truetype/liberation2/LiberationSans-Regular.ttf`,
		}
	}
	for _, font := range candidates {
		if _, err := os.Stat(font); err == nil {
			font = filepath.ToSlash(font)
			font = strings.ReplaceAll(font, "\\", "\\\\")
			font = strings.ReplaceAll(font, ":", "\\:")
			font = strings.ReplaceAll(font, "'", "\\'")
			return ":fontfile='" + font + "'"
		}
	}
	return ""
}

// hasTextOverlay = cần thêm drawtext (title hoặc watermark).
// Listing overlay (Address/Nickname/Prices/Amenities) đã được render bằng PNG
// trong overlay.go nên không nằm trong đây.
func hasTextOverlay(cfg Config) bool {
	return cfg.Title != "" || cfg.Watermark != ""
}

func appendAudio(cfg Config, args []string, totalSec float64) []string {
	if cfg.Audio == "" {
		return args
	}
	if _, err := os.Stat(cfg.Audio); err != nil {
		fmt.Fprintf(os.Stderr, "⚠️   Không thấy audio: %s\n", cfg.Audio)
		return args
	}
	fade := totalSec - 2
	if fade < 0 {
		fade = 0
	}
	af := fmt.Sprintf("aloop=loop=-1:size=2147483647,atrim=duration=%.3f,afade=t=out:st=%.3f:d=2", totalSec, fade)
	return append(args, "-i", cfg.Audio, "-filter:a", af, "-shortest")
}

// ─── Slideshow ─────────────────────────────────────────────────────────────

func buildSlideshow(cfg Config, images []string) ([]string, error) {
	listFile, err := writeConcatList(images, cfg.Duration)
	if err != nil {
		return nil, err
	}
	total := cfg.Duration * float64(len(images))
	fmt.Printf("🎞️   slideshow | %d × %.1fs = %.0fs | %dx%d @%dfps\n",
		len(images), cfg.Duration, total, cfg.Width, cfg.Height, cfg.FPS)
	plans, _, err := prepareTextOverlayPlans(cfg)
	if err != nil {
		return nil, fmt.Errorf("textrender: %v", err)
	}

	overlayPath := ""
	if len(plans) == 0 {
		overlayPath, err = renderListingOverlayPNG(cfg)
		if err != nil {
			return nil, fmt.Errorf("render overlay PNG: %v", err)
		}
	}
	args := []string{"-y", "-f", "concat", "-safe", "0", "-i", listFile}

	if len(plans) > 0 {
		for _, p := range plans {
			args = append(args, "-i", p.PNGPath)
		}
		filterStr := fmt.Sprintf("[0:v]%s[base]", scalePad(cfg.Width, cfg.Height))
		filterStr += buildOverlayFilterChain(plans, "[base]", "[vfinal]", 1)
		args = append(args,
			"-filter_complex", filterStr,
			"-map", "[vfinal]",
			"-c:v", "libx264", "-preset", "fast", "-crf", "22",
			"-pix_fmt", "yuv420p", "-r", strconv.Itoa(cfg.FPS), "-movflags", "+faststart",
		)
	} else if overlayPath != "" {
		args = append(args, "-i", overlayPath)
		filterStr := fmt.Sprintf("[0:v]%s[base];", scalePad(cfg.Width, cfg.Height))
		enable := ""
		if strings.TrimSpace(cfg.Title) != "" {
			td := cfg.TitleDuration
			if td <= 0 {
				td = 3
			}
			enable = fmt.Sprintf(":enable='gte(t,%.3f)'", td)
		}
		filterStr += fmt.Sprintf("[base][1:v]overlay=0:0%s[overlaid]", enable)
		mapTarget := "[overlaid]"
		if hasTextOverlay(cfg) {
			extra := appendTextFilters(cfg, "")
			filterStr += fmt.Sprintf(";[overlaid]%s[vfinal]", strings.TrimPrefix(extra, ","))
			mapTarget = "[vfinal]"
		}
		args = append(args,
			"-filter_complex", filterStr,
			"-map", mapTarget,
			"-c:v", "libx264", "-preset", "fast", "-crf", "22",
			"-pix_fmt", "yuv420p", "-r", strconv.Itoa(cfg.FPS), "-movflags", "+faststart",
		)
	} else {
		vf := scalePad(cfg.Width, cfg.Height)
		vf = appendTextFilters(cfg, vf)
		args = append(args,
			"-vf", vf, "-c:v", "libx264", "-preset", "fast", "-crf", "22",
			"-pix_fmt", "yuv420p", "-r", strconv.Itoa(cfg.FPS), "-movflags", "+faststart",
		)
	}
	args = appendAudio(cfg, args, total)
	return append(args, cfg.Output), nil
}

func writeConcatList(images []string, duration float64) (string, error) {
	f, err := os.CreateTemp("", "img2video_*.txt")
	if err != nil {
		return "", err
	}
	defer f.Close()
	for _, img := range images {
		abs, _ := filepath.Abs(img)
		fmt.Fprintf(f, "file '%s'\nduration %.3f\n", filepath.ToSlash(abs), duration)
	}
	abs, _ := filepath.Abs(images[len(images)-1])
	fmt.Fprintf(f, "file '%s'\n", filepath.ToSlash(abs))
	return f.Name(), nil
}

// ─── Ken Burns ─────────────────────────────────────────────────────────────

// makeGridCollage builds a static 2x2 grid (w×h) from the first 4 images, each
// cropped to fill a (w/2 × h/2) cell. Returns the grid PNG path.
func makeGridCollage(images []string, w, h int) (string, error) {
	if len(images) < 4 {
		return "", fmt.Errorf("cần ≥4 ảnh cho lưới 2x2, có %d", len(images))
	}
	dir, err := os.MkdirTemp("", "gridintro_*")
	if err != nil {
		return "", err
	}
	cellW, cellH := w/2, h/2
	cells := make([]string, 4)
	for i := 0; i < 4; i++ {
		cell := filepath.Join(dir, fmt.Sprintf("cell%d.png", i))
		cmd := magickCmd(images[i],
			"-resize", fmt.Sprintf("%dx%d^", cellW, cellH),
			"-gravity", "center", "-extent", fmt.Sprintf("%dx%d", cellW, cellH),
			cell)
		if out, err := cmd.CombinedOutput(); err != nil {
			return "", fmt.Errorf("crop cell %d: %v: %s", i, err, out)
		}
		cells[i] = cell
	}
	grid := filepath.Join(dir, "grid.png")
	margs := append([]string{"montage"}, cells...)
	margs = append(margs, "-tile", "2x2", "-geometry", "+0+0", "-background", "black", grid)
	if out, err := magickCmd(margs...).CombinedOutput(); err != nil {
		return "", fmt.Errorf("montage: %v: %s", err, out)
	}
	return grid, nil
}

func buildKenBurns(cfg Config, images []string) ([]string, error) {
	// Crossfade dài hơn = chuyển cảnh mượt, giảm cảm giác "nhảy"
	cross := 0.8
	if cfg.Duration < 2.5 {
		cross = cfg.Duration * 0.3
	}

	// zDelta: tổng lượng zoom. Intensity 0.5 → delta ~0.16
	// Subtle dùng zDelta nhỏ hơn cho chuyển động nhẹ nhàng
	zDelta := 0.08 + cfg.ZoomIntensity*0.15
	if cfg.EffectType == "subtle" {
		zDelta = 0.05 + cfg.ZoomIntensity*0.07
	}

	framesPerClip := int((cfg.Duration + cross) * float64(cfg.FPS))
	allEffects := buildEffects(zDelta, framesPerClip)

	// Optional static 2x2 grid intro: prepend a collage frame that plays still
	// (no Ken Burns), then crossfades into the regular single-image motion.
	gridIntro := false
	if cfg.GridIntro && len(images) >= 4 {
		if grid, gerr := makeGridCollage(images, cfg.Width, cfg.Height); gerr == nil {
			images = append([]string{grid}, images...)
			gridIntro = true
			fmt.Printf("🔲  Lưới intro 2x2 từ 4 ảnh đầu\n")
		} else {
			fmt.Printf("⚠️  bỏ qua lưới intro: %v\n", gerr)
		}
	}

	totalSec := cfg.Duration*float64(len(images)) - cross*float64(len(images)-1)

	typesDesc := strings.Join(cfg.EffectTypes, ",")
	if typesDesc == "" {
		typesDesc = cfg.EffectType
	}

	randomKinds := []string{"zoom-in", "zoom-out", "move-left-right", "move-right-left"}

	var perImageEffects []effect
	if effectIsRandom(cfg) {
		typesDesc = "auto theo hướng ảnh"
		for i := range images {
			k := randomKinds[rand.Intn(len(randomKinds))]
			orientation, err := imageOrientation(images[i])
			if err == nil {
				switch orientation {
				case "landscape":
					if rand.Intn(2) == 0 {
						k = "move-left-right"
					} else {
						k = "move-right-left"
					}
				case "portrait":
					if rand.Intn(2) == 0 {
						k = "zoom-in"
					} else {
						k = "zoom-out"
					}
				}
			}
			pool := filterEffects(allEffects, k)
			if len(pool) == 0 {
				pool = allEffects
			}
			ef := pool[rand.Intn(len(pool))]
			perImageEffects = append(perImageEffects, ef)
			if err != nil {
				fmt.Printf("   [%d] auto fallback %s → %s (%v)\n", i+1, k, ef.name, err)
			} else {
				fmt.Printf("   [%d] %s → %s → %s\n", i+1, orientation, k, ef.name)
			}
		}
	} else {
		effects := filterEffectsForKinds(allEffects, cfg.EffectTypes, cfg.EffectType)
		rand.Shuffle(len(effects), func(i, j int) { effects[i], effects[j] = effects[j], effects[i] })
		perImageEffects = make([]effect, len(images))
		for i := range images {
			perImageEffects[i] = effects[i%len(effects)]
			fmt.Printf("   [%d] %s\n", i+1, perImageEffects[i].name)
		}
	}

	// The grid intro frame plays static (no zoom/pan).
	if gridIntro && len(perImageEffects) > 0 {
		perImageEffects[0] = effect{name: "grid-static", z: "1", x: "iw/2-(iw/zoom/2)", y: "ih/2-(ih/zoom/2)"}
	}

	fmt.Printf("🎬  kenburns | %d ảnh × %.2fs | cross=%.1fs | tổng=%.1fs | types=%s | %dx%d @%dfps\n",
		len(images), cfg.Duration, cross, totalSec, typesDesc, cfg.Width, cfg.Height, cfg.FPS)

	var args []string
	for _, img := range images {
		args = append(args, "-loop", "1", "-t", fmt.Sprintf("%.3f", cfg.Duration+cross+0.2), "-i", img)
	}

	// Phase 2 pipeline: template-driven PNG overlays via textrender (if cfg.Template set).
	// Falls back to legacy single-PNG magick path if Template is empty.
	overlayPlans, _, err := prepareTextOverlayPlans(cfg)
	if err != nil {
		return nil, fmt.Errorf("textrender overlays: %v", err)
	}

	overlayPath := ""
	if len(overlayPlans) == 0 {
		overlayPath, err = renderListingOverlayPNG(cfg)
		if err != nil {
			return nil, fmt.Errorf("render overlay PNG: %v", err)
		}
	}
	overlayIdx := -1
	if overlayPath != "" {
		args = append(args, "-i", overlayPath)
		overlayIdx = len(images)
	}
	plansFirstIdx := len(images)
	for _, p := range overlayPlans {
		args = append(args, "-i", p.PNGPath)
	}

	var fc strings.Builder
	for i := range images {
		ef := perImageEffects[i]
		fc.WriteString(fmt.Sprintf(
			"[%d:v]%s,zoompan=z='%s':x='%s':y='%s':d=%d:s=%dx%d:fps=%d[zp%d];\n",
			i, scaleCrop(cfg.Width, cfg.Height),
			ef.z, ef.x, ef.y,
			framesPerClip, cfg.Width, cfg.Height, cfg.FPS, i,
		))
	}

	if len(images) == 1 {
		fc.WriteString("[zp0]copy[vout]")
	} else {
		prev := "zp0"
		for i := 1; i < len(images); i++ {
			out := fmt.Sprintf("xf%d", i)
			if i == len(images)-1 {
				out = "vout"
			}
			offset := float64(i) * (cfg.Duration - cross)
			fc.WriteString(fmt.Sprintf(
				"[%s][zp%d]xfade=transition=fade:duration=%.2f:offset=%.3f[%s];\n",
				prev, i, cross, offset, out))
			prev = out
		}
	}

	filterStr := strings.TrimRight(fc.String(), ";\n")
	// trim chính xác tổng thời gian — tránh video bị dài
	filterStr += fmt.Sprintf(";\n[vout]trim=duration=%.3f,setpts=PTS-STARTPTS[vtrim]", totalSec)
	mapTarget := "[vtrim]"

	// Template-driven overlays (Phase 2)
	if len(overlayPlans) > 0 {
		chain := buildOverlayFilterChain(overlayPlans, mapTarget, "[vfinal]", plansFirstIdx)
		if chain != "" {
			filterStr += chain
			mapTarget = "[vfinal]"
		}
	} else if overlayIdx >= 0 {
		// Legacy magick single-PNG path
		enable := ""
		if strings.TrimSpace(cfg.Title) != "" {
			td := cfg.TitleDuration
			if td <= 0 {
				td = 3
			}
			enable = fmt.Sprintf(":enable='gte(t,%.3f)'", td)
		}
		filterStr += fmt.Sprintf(";\n[vtrim][%d:v]overlay=0:0%s[vlist]", overlayIdx, enable)
		mapTarget = "[vlist]"
	}

	// drawtext cho Title + Watermark (legacy — chỉ chạy nếu không dùng template)
	if cfg.Template == "" && hasTextOverlay(cfg) {
		extra := appendTextFilters(cfg, "")
		filterStr += fmt.Sprintf(";\n%s%s[vtxt]", mapTarget, strings.TrimPrefix(extra, ","))
		mapTarget = "[vtxt]"
	}

	args = append(args,
		"-filter_complex", filterStr,
		"-map", mapTarget,
		"-c:v", "libx264", "-preset", "fast", "-crf", "20",
		"-pix_fmt", "yuv420p", "-movflags", "+faststart",
	)
	args = appendAudio(cfg, args, totalSec)
	return append(args, "-y", cfg.Output), nil
}

// ─── Timelapse ─────────────────────────────────────────────────────────────

func buildTimelapse(cfg Config, images []string) ([]string, error) {
	fmt.Printf("⏩  timelapse | %d frames | %.1ffps → %dfps\n", len(images), cfg.InputFPS, cfg.OutputFPS)
	pattern, startNum, err := detectPattern(images)
	if err != nil {
		return buildSlideshow(Config{
			Input: cfg.Input, Output: cfg.Output,
			Duration: 1.0 / cfg.InputFPS, FPS: cfg.OutputFPS,
			Width: cfg.Width, Height: cfg.Height,
			Audio: cfg.Audio, Title: cfg.Title, TitleDuration: cfg.TitleDuration, Watermark: cfg.Watermark,
			Verbose: cfg.Verbose, DryRun: cfg.DryRun,
		}, images)
	}
	plans, _, err := prepareTextOverlayPlans(cfg)
	if err != nil {
		return nil, fmt.Errorf("textrender: %v", err)
	}
	overlayPath := ""
	if len(plans) == 0 {
		overlayPath, err = renderListingOverlayPNG(cfg)
		if err != nil {
			return nil, fmt.Errorf("render overlay PNG: %v", err)
		}
	}
	args := []string{"-y",
		"-framerate", fmt.Sprintf("%.2f", cfg.InputFPS),
		"-start_number", strconv.Itoa(startNum),
		"-i", pattern,
	}
	if len(plans) > 0 {
		for _, p := range plans {
			args = append(args, "-i", p.PNGPath)
		}
		filterStr := fmt.Sprintf("[0:v]%s[base]", scalePad(cfg.Width, cfg.Height))
		filterStr += buildOverlayFilterChain(plans, "[base]", "[vfinal]", 1)
		args = append(args,
			"-filter_complex", filterStr,
			"-map", "[vfinal]",
			"-r", strconv.Itoa(cfg.OutputFPS),
			"-c:v", "libx264", "-preset", "fast", "-crf", "20",
			"-pix_fmt", "yuv420p", "-movflags", "+faststart",
		)
	} else if overlayPath != "" {
		args = append(args, "-i", overlayPath)
		filterStr := fmt.Sprintf("[0:v]%s[base];", scalePad(cfg.Width, cfg.Height))
		enable := ""
		if strings.TrimSpace(cfg.Title) != "" {
			td := cfg.TitleDuration
			if td <= 0 {
				td = 3
			}
			enable = fmt.Sprintf(":enable='gte(t,%.3f)'", td)
		}
		filterStr += fmt.Sprintf("[base][1:v]overlay=0:0%s[overlaid]", enable)
		mapTarget := "[overlaid]"
		if hasTextOverlay(cfg) {
			extra := appendTextFilters(cfg, "")
			filterStr += fmt.Sprintf(";[overlaid]%s[vfinal]", strings.TrimPrefix(extra, ","))
			mapTarget = "[vfinal]"
		}
		args = append(args,
			"-filter_complex", filterStr,
			"-map", mapTarget,
			"-r", strconv.Itoa(cfg.OutputFPS),
			"-c:v", "libx264", "-preset", "fast", "-crf", "20",
			"-pix_fmt", "yuv420p", "-movflags", "+faststart",
		)
	} else {
		vf := scalePad(cfg.Width, cfg.Height)
		vf = appendTextFilters(cfg, vf)
		args = append(args,
			"-vf", vf,
			"-r", strconv.Itoa(cfg.OutputFPS),
			"-c:v", "libx264", "-preset", "fast", "-crf", "20",
			"-pix_fmt", "yuv420p", "-movflags", "+faststart",
		)
	}
	totalDur := float64(len(images)) / cfg.InputFPS
	args = appendAudio(cfg, args, totalDur)
	return append(args, cfg.Output), nil
}

func detectPattern(images []string) (string, int, error) {
	re := regexp.MustCompile(`(\D*)(\d+)(\.\w+)$`)
	m := re.FindStringSubmatch(filepath.Base(images[0]))
	if m == nil {
		return "", 0, fmt.Errorf("không nhận diện pattern")
	}
	start, _ := strconv.Atoi(m[2])
	pattern := filepath.Join(filepath.Dir(images[0]),
		fmt.Sprintf("%s%%0%dd%s", m[1], len(m[2]), m[3]))
	return pattern, start, nil
}

// ─── FFmpeg runner ─────────────────────────────────────────────────────────

func runFFmpeg(args []string, cfg Config) error {
	if cfg.Verbose || cfg.DryRun {
		fmt.Println("\n🔧  Lệnh FFmpeg:")
		fmt.Println("   " + shellQuote(append([]string{"ffmpeg"}, args...)))
	}
	if cfg.DryRun {
		fmt.Println("\n✅  Dry-run xong.")
		return nil
	}
	fmt.Printf("\n🚀  Đang render: %s\n", cfg.Output)
	c := exec.Command("ffmpeg", args...)
	stderr, _ := c.StderrPipe()
	if err := c.Start(); err != nil {
		return fmt.Errorf("không chạy được ffmpeg: %v", err)
	}
	scanner := bufio.NewScanner(stderr)
	scanner.Split(splitLines)
	frameRe := regexp.MustCompile(`frame=\s*(\d+)`)
	timeRe := regexp.MustCompile(`time=(\d+:\d+:\d+\.\d+)`)
	for scanner.Scan() {
		line := scanner.Text()
		if cfg.Verbose {
			fmt.Println("  ", line)
			continue
		}
		if m := frameRe.FindStringSubmatch(line); m != nil {
			ts := ""
			if mt := timeRe.FindStringSubmatch(line); mt != nil {
				ts = mt[1]
			}
			fmt.Printf("\r⏳  Frame %-6s  |  %s        ", m[1], ts)
		}
	}
	if err := c.Wait(); err != nil {
		return fmt.Errorf("ffmpeg lỗi: %v", err)
	}
	fmt.Printf("\n\n✅  Xong! → %s", cfg.Output)
	if info, err := os.Stat(cfg.Output); err == nil {
		fmt.Printf("  (%.1f MB)\n", float64(info.Size())/1024/1024)
	}
	return nil
}

func splitLines(data []byte, atEOF bool) (int, []byte, error) {
	for i, b := range data {
		if b == '\n' || b == '\r' {
			return i + 1, data[:i], nil
		}
	}
	if atEOF && len(data) > 0 {
		return len(data), data, nil
	}
	return 0, nil, nil
}

func shellQuote(parts []string) string {
	out := make([]string, len(parts))
	for i, p := range parts {
		if strings.ContainsAny(p, " \t\n'\"\\") {
			if runtime.GOOS == "windows" {
				out[i] = `"` + strings.ReplaceAll(p, `"`, `\"`) + `"`
			} else {
				out[i] = "'" + strings.ReplaceAll(p, "'", `'\''`) + "'"
			}
		} else {
			out[i] = p
		}
	}
	return strings.Join(out, " ")
}
