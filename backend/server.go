package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"img2video/textrender"
)

// ─── State ────────────────────────────────────────────────────────────────

type RenderState struct {
	mu       sync.Mutex
	running  bool
	progress string
	done     bool
	err      string
	output   string
}

var state = &RenderState{}

type RenderRequest struct {
	Mode          string  `json:"mode"`
	Total         float64 `json:"total"`
	Duration      float64 `json:"duration"`
	ZoomIntensity float64 `json:"zoom_intensity"`
	FPS           int     `json:"fps"`
	Tiktok        bool    `json:"tiktok"`
	Width         int     `json:"width"`
	Height        int     `json:"height"`
	Title         string  `json:"title"`
	TitleDuration float64 `json:"title_duration"`
	Watermark     string  `json:"watermark"`
	TextFont      string  `json:"text_font"`
	TextScale     float64 `json:"text_scale"`
	TextColor     string  `json:"text_color"`
	TitleFontFile string  `json:"title_font_file"` // uploaded/custom font for title+nickname

	// Listing overlay
	Address       string   `json:"address"`
	Nickname      string   `json:"nickname"`
	ListingID     string   `json:"listing_id"`
	Prices        []string `json:"prices"`
	Amenities     []string `json:"amenities"`
	EffectType    string   `json:"effect_type"`   // một kiểu cố định hoặc "random"
	EffectTypes   []string `json:"effect_types"`  // thường 1 phần tử; hoặc ["random"]
	OverlayStyle  string   `json:"overlay_style"` // "badge" | "bubble"
	OverlayFont   string   `json:"overlay_font"`
	OverlayScale  float64  `json:"overlay_scale"`
	OverlayText   string   `json:"overlay_text"`
	OverlayBG     string   `json:"overlay_bg"`
	OverlayStroke string   `json:"overlay_stroke"`
	TitleColor    string   `json:"title_color"`
	StrokeColor   string   `json:"stroke_color"`
	TitleBg       string   `json:"title_bg"`
	BodyBg        string   `json:"body_bg"`
	GridIntro     bool     `json:"grid_intro"`

	// Template system (mới)
	Template     string                              `json:"template"`
	CustomStyles map[string]*textrender.ElementStyle `json:"custom_styles,omitempty"`

	// Tự đăng social qua webhook (Make.com / n8n)
	AutoPost   bool     `json:"auto_post"`
	WebhookURL string   `json:"webhook_url"`
	Platforms  []string `json:"platforms"` // ["tiktok","facebook"]

	// Mode "narrated": lời kể AI + giọng đọc TTS + phụ đề + nhạc nền
	Narrate          bool   `json:"narrate"`
	NarrationPersona string `json:"narration_persona"`
	TTSProvider      string `json:"tts_provider"`
	VoiceID          string `json:"voice_id"`
	MaxSegments      int    `json:"max_segments"`
	TargetDuration   int    `json:"target_duration"` // giây; 0 = không giới hạn
	SubtitleStyle    string `json:"subtitle_style"`  // "karaoke" (mặc định) | "typewriter"
	Music            string `json:"music"`           // tên file nhạc trong music dir; "" = không nhạc
	IntroClip        bool   `json:"intro_clip"`      // chèn cảnh đi đường mở đầu (assets/intro/street.mp4)
	Audience         string `json:"audience"`        // "couple" (mặc định) | "genz"

	// Kịch bản user đã duyệt/sửa từ panel "Xem kịch bản" (nil = Claude tự viết).
	Script *NarrationScript `json:"script,omitempty"`
}

// ─── Start server ─────────────────────────────────────────────────────────

func startWebServer(port int) error {
	uploadDir := filepath.Join(os.TempDir(), "img2video_uploads")
	outputDir := filepath.Join(os.TempDir(), "img2video_output")
	os.MkdirAll(uploadDir, 0755)
	os.MkdirAll(outputDir, 0755)

	mux := http.NewServeMux()

	// Middleware chain: log every request (outermost) → CORS → routes.
	handler := loggingMiddleware(corsMiddleware(mux))

	// ── API Routes ──
	mux.HandleFunc("/api/upload", uploadHandler(uploadDir))
	mux.HandleFunc("/api/render", renderHandler(uploadDir, outputDir))
	mux.HandleFunc("/api/status", statusHandler())
	mux.HandleFunc("/api/download", downloadHandler(outputDir))
	mux.HandleFunc("/api/video/", videoFileHandler(outputDir))
	mux.HandleFunc("/api/history", historyHandler())
	mux.HandleFunc("/api/export-excel", exportExcelHandler(outputDir, port))
	mux.HandleFunc("/api/parse-listings", parseListingsHandler())
	mux.HandleFunc("/api/dayladau-listings", dayladauListingsHandler())
	mux.HandleFunc("/api/select-listing", selectListingHandler(uploadDir))
	mux.HandleFunc("/api/templates", listTemplatesHandler())
	mux.HandleFunc("/api/render-thumbnail", thumbnailHandler(uploadDir, outputDir))
	mux.HandleFunc("/api/thumbnail-file", thumbnailFileHandler(outputDir))
	mux.HandleFunc("/api/thumbnail-history", thumbnailHistoryHandler())

	// Bring-your-own fonts: validate/instance on upload, list, gg-faithful preview.
	mux.HandleFunc("/api/upload-font", uploadFontHandler())
	mux.HandleFunc("/api/fonts", listFontsHandler())
	mux.HandleFunc("/api/font-preview", fontPreviewHandler())
	mux.HandleFunc("/api/delete-font", deleteFontHandler())

	// Nghe thử giọng TTS (mode narrated) — cache theo provider+voice.
	mux.HandleFunc("/api/tts-preview", ttsPreviewHandler())

	// Xem/sửa kịch bản thuyết minh trước khi render (+ thư viện kịch bản đã lưu).
	mux.HandleFunc("/api/script", scriptPreviewHandler(uploadDir))
	mux.HandleFunc("/api/scripts", listScriptsHandler())
	mux.HandleFunc("/api/like-script", likeScriptHandler())
	mux.HandleFunc("/api/delete-script", deleteScriptHandler())

	// Nhạc nền cho mode narrated (upload / list / delete).
	mux.HandleFunc("/api/upload-music", uploadMusicHandler())
	mux.HandleFunc("/api/music", listMusicHandler())
	mux.HandleFunc("/api/delete-music", deleteMusicHandler())

	// Thư viện clip "cảnh đi đường" (intro) — upload / list / delete / import Drive.
	mux.HandleFunc("/api/upload-intro", uploadIntroHandler())
	mux.HandleFunc("/api/intros", listIntrosHandler())
	mux.HandleFunc("/api/intro-file", introFileHandler())
	mux.HandleFunc("/api/delete-intro", deleteIntroHandler())
	mux.HandleFunc("/api/import-intro-drive", importIntroDriveHandler())
	mux.HandleFunc("/api/intro-import-status", introImportStatusHandler())

	// Health check
	mux.HandleFunc("/api/health", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"status": "ok", "version": "2.0.0"})
	})

	// Serve React frontend từ dist/ (production)
	distDir := "dist"
	if _, err := os.Stat(distDir); err == nil {
		fs := http.FileServer(http.Dir(distDir))
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			// SPA fallback: nếu file không tồn tại → serve index.html
			path := filepath.Join(distDir, r.URL.Path)
			if _, err := os.Stat(path); os.IsNotExist(err) {
				http.ServeFile(w, r, filepath.Join(distDir, "index.html"))
				return
			}
			fs.ServeHTTP(w, r)
		})
	}

	addr := fmt.Sprintf(":%d", port)
	fmt.Printf("\n🚀  Backend API: http://localhost%s\n", addr)
	fmt.Printf("    Frontend:    http://localhost:5173\n")
	fmt.Println("    Nhấn Ctrl+C để dừng")
	return http.ListenAndServe(addr, handler)
}

// ─── CORS Middleware ──────────────────────────────────────────────────────

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		// Cho phép localhost dev + production domain
		allowed := []string{
			"http://localhost:5173",
			"http://localhost:3000",
			"http://localhost:8080",
		}
		// Nếu có ALLOWED_ORIGINS trong env (production)
		if env := os.Getenv("ALLOWED_ORIGINS"); env != "" {
			allowed = append(allowed, strings.Split(env, ",")...)
		}
		for _, o := range allowed {
			if origin == o || o == "*" {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				break
			}
		}
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		w.Header().Set("Access-Control-Allow-Credentials", "true")

		if r.Method == "OPTIONS" {
			w.WriteHeader(204)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// ─── Request Logging Middleware ───────────────────────────────────────────

// ANSI colour helpers for readable terminal output.
const (
	cReset  = "\033[0m"
	cDim    = "\033[2m"
	cGreen  = "\033[32m"
	cYellow = "\033[33m"
	cRed    = "\033[31m"
	cCyan   = "\033[36m"
	cBold   = "\033[1m"
)

// visitorTracker remembers which client IPs we've already seen so the first
// request from a new visitor can be highlighted ("👤 người dùng mới").
var visitors = struct {
	sync.Mutex
	seen map[string]time.Time
}{seen: map[string]time.Time{}}

// statusRecorder wraps http.ResponseWriter to capture the status code so the
// logging middleware can report it after the handler runs.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

// clientIP extracts the real client IP, honouring reverse-proxy headers so the
// log shows the visitor rather than the proxy when deployed behind one.
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// First entry is the original client.
		return strings.TrimSpace(strings.Split(xff, ",")[0])
	}
	if xr := r.Header.Get("X-Real-IP"); xr != "" {
		return strings.TrimSpace(xr)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// statusColor picks a colour by HTTP status class (2xx green, 3xx/4xx yellow, 5xx red).
func statusColor(code int) string {
	switch {
	case code >= 500:
		return cRed
	case code >= 300:
		return cYellow
	default:
		return cGreen
	}
}

// isNoise reports whether a request is background/asset traffic we don't want
// cluttering the "who's using the app" log: static assets (js/css/img/fonts),
// the health check, and the ~1s /api/status render-progress polling. The SPA
// document itself ("/" or /index.html) is treated as a real page load.
func isNoise(path string) bool {
	switch path {
	case "/api/health", "/api/status",
		"/api/history", "/api/thumbnail-history":
		return true // uptime pings + render-progress + 4s history polling
	case "/", "/index.html":
		return false // the app page load — a real visit
	}
	// Non-API path with a file extension → static asset (index-xxx.js, logo.png…).
	if !strings.HasPrefix(path, "/api/") {
		return strings.Contains(filepath.Base(path), ".")
	}
	return false
}

// loggingMiddleware prints one line per request to the terminal so you can see
// live who is using the app: time, client IP, method, path, status, duration.
// The first hit from a new IP is called out as a new visitor. Static-asset and
// polling noise is filtered out; set LOG_VERBOSE=1 to log every request.
func loggingMiddleware(next http.Handler) http.Handler {
	verbose := os.Getenv("LOG_VERBOSE") == "1"
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: 200}
		next.ServeHTTP(rec, r)

		// CORS preflight is handled+returned inside corsMiddleware — pure noise.
		if r.Method == "OPTIONS" {
			return
		}
		// Skip asset/health/poll noise unless verbose logging is requested.
		if !verbose && isNoise(r.URL.Path) {
			return
		}

		ip := clientIP(r)

		// New-visitor highlight: first time we've seen this IP.
		visitors.Lock()
		_, known := visitors.seen[ip]
		visitors.seen[ip] = start
		visitors.Unlock()
		if !known {
			fmt.Printf("%s%s👤  Người dùng mới đang truy cập: %s%s%s\n",
				cBold, cCyan, ip, cReset, "")
		}

		dur := time.Since(start)
		sc := rec.status
		fmt.Printf("%s%s%s  %s%s%3d%s  %-6s %s  %s%s%s\n",
			cDim, start.Format("15:04:05"), cReset,
			statusColor(sc), cBold, sc, cReset,
			r.Method, r.URL.Path,
			cDim, dur.Round(time.Millisecond), cReset)
	})
}

// listTemplatesHandler returns metadata for every JSON template under
// assets/templates (sorted by name) — used by the TemplatePicker UI.
func listTemplatesHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		list, err := textrender.ListTemplates(assetsTemplatesDir())
		if err != nil {
			http.Error(w, `{"error":"`+err.Error()+`"}`, 500)
			return
		}
		json.NewEncoder(w).Encode(list)
	}
}

// ─── Handlers ─────────────────────────────────────────────────────────────

func uploadHandler(uploadDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "method not allowed", 405)
			return
		}
		r.ParseMultipartForm(200 << 20)
		files := r.MultipartForm.File["images"]
		os.RemoveAll(uploadDir)
		os.MkdirAll(uploadDir, 0755)
		count := 0
		for _, fh := range files {
			ext := strings.ToLower(filepath.Ext(fh.Filename))
			if !supportedExts[ext] {
				continue
			}
			src, err := fh.Open()
			if err != nil {
				continue
			}
			defer src.Close()
			dst, err := os.Create(filepath.Join(uploadDir, fmt.Sprintf("%04d%s", count, ext)))
			if err != nil {
				continue
			}
			io.Copy(dst, src)
			dst.Close()
			count++
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]int{"count": count})
	}
}

func renderHandler(uploadDir, outputDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "method not allowed", 405)
			return
		}
		state.mu.Lock()
		if state.running {
			state.mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			http.Error(w, `{"error":"already running"}`, 409)
			return
		}
		state.running = true
		state.done = false
		state.err = ""
		state.progress = "Đang chuẩn bị..."
		state.output = ""
		state.mu.Unlock()

		var req RenderRequest
		json.NewDecoder(r.Body).Decode(&req)
		if req.FPS == 0 {
			req.FPS = 30
		}
		if req.Duration == 0 {
			req.Duration = 3
		}
		if req.Total == 0 {
			req.Total = 40
		}
		if req.TitleDuration == 0 {
			req.TitleDuration = 3
		}
		if req.ZoomIntensity == 0 {
			req.ZoomIntensity = 0.4
		}
		if req.Width == 0 {
			req.Width = 1080
		}
		if req.Height == 0 {
			req.Height = 1920
		}
		if req.Tiktok {
			req.Width, req.Height = 1080, 1920
		}

		// "random" → backend tự bốc 1 template video thật (mỗi video 1 mẫu).
		if strings.EqualFold(strings.TrimSpace(req.Template), "random") {
			req.Template = pickRandomVideoTemplate()
			fmt.Printf("🎲  Template random → %s\n", req.Template)
		}

		outputFile := filepath.Join(outputDir, "output.mp4")
		cfg := Config{
			Input:         uploadDir,
			Output:        outputFile,
			Mode:          req.Mode,
			Total:         req.Total,
			Duration:      req.Duration,
			FPS:           req.FPS,
			ZoomIntensity: req.ZoomIntensity,
			Width:         req.Width,
			Height:        req.Height,
			Title:         req.Title,
			TitleDuration: req.TitleDuration,
			Watermark:     req.Watermark,
			TextFont:      req.TextFont,
			TextScale:     req.TextScale,
			TextColor:     req.TextColor,
			TitleFontFile: req.TitleFontFile,
			Address:       req.Address,
			Nickname:      req.Nickname,
			ListingID:     req.ListingID,
			Prices:        req.Prices,
			Amenities:     req.Amenities,
			EffectType:    req.EffectType,
			EffectTypes:   req.EffectTypes,
			OverlayStyle:  req.OverlayStyle,
			OverlayFont:   req.OverlayFont,
			OverlayScale:  req.OverlayScale,
			OverlayText:   req.OverlayText,
			OverlayBG:     req.OverlayBG,
			OverlayStroke: req.OverlayStroke,
			TitleColor:    req.TitleColor,
			StrokeColor:   req.StrokeColor,
			TitleBg:       req.TitleBg,
			BodyBg:        req.BodyBg,
			GridIntro:     req.GridIntro,
			Template:      req.Template,
			CustomStyles:  req.CustomStyles,
			AutoPost:      req.AutoPost,
			WebhookURL:    req.WebhookURL,
			Platforms:     req.Platforms,

			Narrate:          req.Narrate,
			NarrationPersona: req.NarrationPersona,
			TTSProvider:      req.TTSProvider,
			VoiceID:          req.VoiceID,
			MaxSegments:      req.MaxSegments,
			TargetDuration:   req.TargetDuration,
			SubtitleStyle:    req.SubtitleStyle,
			IntroClip:        req.IntroClip,
			Audience:         req.Audience,
			Script:           req.Script,
		}
		if cfg.Mode == "" {
			cfg.Mode = "kenburns"
		}
		// Tương thích ngược: mode "narrated" cũ → cờ Narrate + motion mặc định kenburns.
		if cfg.Mode == "narrated" {
			cfg.Narrate = true
			cfg.Mode = "kenburns"
		}
		// Nhạc nền: tên file → đường dẫn thật trong music dir (dùng cho ducking).
		if p := resolveMusicPath(req.Music); p != "" {
			cfg.Audio = p
		}
		// Preflight cho thuyết minh AI: báo lỗi rõ ràng TRƯỚC khi tốn API.
		if cfg.Narrate {
			if perr := narratePreflight(cfg); perr != nil {
				state.mu.Lock()
				state.err = perr.Error()
				state.running = false
				state.done = true
				state.progress = ""
				state.mu.Unlock()
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(400)
				json.NewEncoder(w).Encode(map[string]string{"error": perr.Error()})
				return
			}
		}

		go func() {
			defer func() {
				if r := recover(); r != nil {
					state.mu.Lock()
					state.err = fmt.Sprintf("Lỗi không mong đợi khi render: %v", r)
					state.running = false
					state.done = true
					state.mu.Unlock()
				}
			}()
			images, err := collectImages(cfg.Input)
			if err != nil {
				state.mu.Lock()
				state.err = err.Error()
				state.running = false
				state.done = true
				state.mu.Unlock()
				return
			}
			if cfg.Duration <= 0 {
				cfg.Duration = cfg.Total / float64(len(images))
			}
			if cfg.Duration < 1.5 {
				cfg.Duration = 1.5
			}

			var args []string
			if cfg.Narrate {
				// Thuyết minh AI ghép lên motion mode đã chọn (buildNarrated tôn trọng cfg.Mode).
				args, err = buildNarrated(cfg, images)
			} else {
				switch cfg.Mode {
				case "slideshow":
					args, err = buildSlideshow(cfg, images)
				case "timelapse":
					args, err = buildTimelapse(cfg, images)
				default:
					args, err = buildKenBurns(cfg, images)
				}
			}
			if err != nil {
				state.mu.Lock()
				state.err = err.Error()
				state.running = false
				state.done = true
				state.mu.Unlock()
				return
			}
			runFFmpegWeb(args, outputFile, outputDir, cfg)
		}()

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "started"})
	}
}

// ScriptRequest = POST /api/script: sinh (hoặc lấy từ cache) kịch bản thuyết
// minh cho ảnh đã upload, KHÔNG render — để user xem/sửa trước.
type ScriptRequest struct {
	NarrationPersona string   `json:"narration_persona"`
	TTSProvider      string   `json:"tts_provider"`
	MaxSegments      int      `json:"max_segments"`
	TargetDuration   int      `json:"target_duration"`
	Nickname         string   `json:"nickname"`
	Address          string   `json:"address"`
	ListingID        string   `json:"listing_id"`
	Prices           []string `json:"prices"`
	Amenities        []string `json:"amenities"`
	IntroClip        bool     `json:"intro_clip"` // chèn cảnh đi đường (đổi ngân sách + schema kịch bản)
	Audience         string   `json:"audience"`   // "couple" (mặc định) | "genz"
	Force            bool     `json:"force"`      // true = nút "Viết lại": bỏ qua cache
}

// scriptPreviewHandler sinh kịch bản cho panel "Xem kịch bản". Dùng đúng
// genScript + cache như lúc render, nên duyệt xong bấm render KHÔNG gọi Claude
// lần hai (trừ khi user sửa tay — bản sửa gửi kèm request render).
func scriptPreviewHandler(uploadDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method != http.MethodPost {
			http.Error(w, `{"error":"method not allowed"}`, 405)
			return
		}
		var req ScriptRequest
		json.NewDecoder(r.Body).Decode(&req)

		if !claudeCLIAvailable() && os.Getenv("ANTHROPIC_API_KEY") == "" {
			writeJSONErr(w, 400, "Chưa có nguồn AI để viết kịch bản: cần Claude Code đăng nhập (lệnh `claude`) hoặc ANTHROPIC_API_KEY")
			return
		}
		images, err := collectImages(uploadDir)
		if err != nil || len(images) == 0 {
			writeJSONErr(w, 400, "chưa có ảnh nào — upload ảnh hoặc chọn listing trước")
			return
		}

		cfg := Config{
			Narrate:          true,
			NarrationPersona: req.NarrationPersona,
			TTSProvider:      req.TTSProvider,
			MaxSegments:      req.MaxSegments,
			TargetDuration:   req.TargetDuration,
			Nickname:         req.Nickname,
			Address:          req.Address,
			ListingID:        req.ListingID,
			Prices:           req.Prices,
			Amenities:        req.Amenities,
			IntroClip:        req.IntroClip,
			Audience:         req.Audience,
			ForceScript:      req.Force,
		}
		images = capNarratedImages(cfg, images)

		script, err := genScript(cfg, images)
		if err != nil {
			writeJSONErr(w, 500, err.Error())
			return
		}
		_, wordBudget := narrationBudget(narrTargetSec(cfg), len(images), cfg.TTSProvider)
		// Grounding: cảnh báo ngay trên panel nếu giá trong hook lệch dữ liệu.
		hookWarning := ""
		if tok, bad := hookPriceMismatch(script.HookLine1+" "+script.HookLine2, req.Prices); bad {
			hookWarning = fmt.Sprintf("Giá %q trong hook không khớp dữ liệu giá listing — sửa lại trước khi render", tok)
		}
		// Cảnh báo từ cấm (tone clean SPEC §1.1 + ngôn ngữ quảng cáo §3.5) — soft.
		var warnings []string
		if hits := scriptBannedHits(script); len(hits) > 0 {
			warnings = append(warnings, "Kịch bản chứa từ nên tránh: "+strings.Join(hits, ", "))
		}
		json.NewEncoder(w).Encode(map[string]any{
			"segments":        script.Segments,
			"hook_line1":      script.HookLine1,
			"hook_line2":      script.HookLine2,
			"hook_emphasis":   script.HookEmphasis,
			"intro_narration": script.IntroNarration,
			"intro_caption":   script.IntroCaption,
			"intro_enabled":   introEnabled(cfg),
			"hook_warning":    hookWarning,
			"warnings":        warnings,
			"word_budget":     wordBudget,
		})
	}
}

func statusHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		state.mu.Lock()
		defer state.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"running":  state.running,
			"done":     state.done,
			"progress": state.progress,
			"error":    state.err,
		})
	}
}

func downloadHandler(outputDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		f := filepath.Join(outputDir, "output.mp4")
		if _, err := os.Stat(f); err != nil {
			http.Error(w, "not ready", 404)
			return
		}
		w.Header().Set("Content-Disposition", `attachment; filename="reel.mp4"`)
		w.Header().Set("Content-Type", "video/mp4")
		http.ServeFile(w, r, f)
	}
}

func runFFmpegWeb(args []string, outputFile, outputDir string, cfg Config) {
	if err := checkFFmpeg(); err != nil {
		state.mu.Lock()
		state.err = err.Error()
		state.running = false
		state.done = true
		state.mu.Unlock()
		return
	}

	c := exec.Command("ffmpeg", args...)
	stderr, _ := c.StderrPipe()
	if err := c.Start(); err != nil {
		state.mu.Lock()
		state.err = "Không chạy được ffmpeg: " + err.Error()
		state.running = false
		state.done = true
		state.mu.Unlock()
		return
	}

	frameRe := regexp.MustCompile(`frame=\s*(\d+)`)
	timeRe := regexp.MustCompile(`time=(\d+:\d+:\d+\.\d+)`)
	speedRe := regexp.MustCompile(`speed=\s*([0-9.]+x)`)

	var errTail []string
	scanner := bufio.NewScanner(stderr)
	scanner.Split(splitLines)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) != "" {
			errTail = append(errTail, line)
			if len(errTail) > 8 {
				errTail = errTail[1:]
			}
		}
		if m := frameRe.FindStringSubmatch(line); m != nil {
			ts, speed := "", ""
			if mt := timeRe.FindStringSubmatch(line); mt != nil {
				ts = mt[1]
			}
			if ms := speedRe.FindStringSubmatch(line); ms != nil {
				speed = ms[1]
			}
			state.mu.Lock()
			state.progress = fmt.Sprintf("Frame %s | %s | %s", m[1], ts, speed)
			state.mu.Unlock()
		}
	}

	err := c.Wait()

	// ── Render lỗi ──
	if err != nil {
		msg := "FFmpeg lỗi: " + err.Error()
		if len(errTail) > 0 {
			msg += "\n" + strings.Join(errTail, "\n")
		}
		state.mu.Lock()
		state.running = false
		state.done = true
		state.err = msg
		state.mu.Unlock()
		return
	}

	// ── Render thành công ──
	info, _ := os.Stat(outputFile)
	sizeMB := float64(0)
	if info != nil {
		sizeMB = float64(info.Size()) / 1024 / 1024
	}
	state.mu.Lock()
	state.progress = fmt.Sprintf("Hoàn tất! %.1f MB", sizeMB)
	state.output = outputFile
	state.mu.Unlock()

	currentListing.mu.Lock()
	listingID, listingName, addr, photos :=
		currentListing.ID, currentListing.Name, currentListing.Addr, currentListing.Photos
	currentListing.mu.Unlock()

	if filename, saveErr := saveVideoFile(outputFile, outputDir, listingID); saveErr == nil {
		addToHistory(RenderRecord{
			ListingName: listingName,
			ListingID:   listingID,
			Address:     addr,
			PhotoCount:  photos,
			FileSizeMB:  sizeMB,
			RenderTime:  time.Now().Format("02/01/2006 15:04:05"),
			FileName:    filename,
		})
	}

	// ── Tự đăng social (best-effort, KHÔNG làm hỏng video nếu lỗi) ──
	// Chạy ngoài state.mu để network I/O không chặn /api/status poller.
	finalMsg := fmt.Sprintf("Hoàn tất! %.1f MB", sizeMB)
	if cfg.AutoPost && strings.TrimSpace(cfg.WebhookURL) != "" && len(cfg.Platforms) > 0 {
		state.mu.Lock()
		state.progress = "Đang gửi lên social…"
		state.mu.Unlock()

		meta := SocialMeta{
			Title:     cfg.Title,
			Nickname:  cfg.Nickname,
			Address:   cfg.Address,
			ListingID: cfg.ListingID,
			Prices:    cfg.Prices,
			Amenities: cfg.Amenities,
			Platforms: cfg.Platforms,
		}
		if postErr := postToWebhook(cfg.WebhookURL, outputFile, meta); postErr != nil {
			finalMsg = fmt.Sprintf("Hoàn tất! %.1f MB — ⚠️ gửi đăng lỗi: %s", sizeMB, postErr.Error())
		} else {
			finalMsg = fmt.Sprintf("Hoàn tất! %.1f MB — ✅ đã gửi đăng (%s)", sizeMB, strings.Join(cfg.Platforms, ", "))
		}
	}

	state.mu.Lock()
	state.progress = finalMsg
	state.running = false
	state.done = true
	state.mu.Unlock()
}

func formatSize(b int64) string {
	return strconv.FormatFloat(float64(b)/1024/1024, 'f', 1, 64) + " MB"
}
