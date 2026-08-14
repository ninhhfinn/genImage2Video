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

	// Health check
	mux.HandleFunc("/api/health", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"status": "ok", "version": "2.0.0"})
	})

	// ── Module Dọn dẹp (admin.quanlyhomestay.com) ──
	// Một binary phục vụ hai web: video.… là công cụ dựng video, admin.… là app
	// checklist + chấm công cho cô dọn dẹp. Tách bằng tên miền chứ không phải
	// đường dẫn, để cô dọn dẹp không bao giờ nhìn thấy công cụ video.
	hkApp, err := NewHKApp(hkDataDir())
	if err != nil {
		// Không chặn cả server: công cụ video đang chạy sản xuất, không được sập
		// chỉ vì module mới mở DB lỗi. Ghi log rõ rồi chạy tiếp thiếu module này.
		fmt.Printf("⚠️  Không khởi động được module Dọn dẹp: %v\n", err)
	} else {
		hkApp.Register(mux)
		fmt.Printf("🧹  Module Dọn dẹp: dữ liệu tại %s\n", hkDataDir())
	}

	// Serve React frontend từ dist/ (production)
	serveSPA := makeSPAHandler("dist")
	serveAdminSPA := makeSPAHandler("dist-admin")

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// `/assets-admin/…` phải về app dọn dẹp dù đứng ở đường dẫn nào: trình
		// duyệt xin tài nguyên bằng đường dẫn tuyệt đối nên nó không mang theo
		// tiền tố /unixstay.
		if isAdminHost(r) || strings.HasPrefix(r.URL.Path, hkPathPrefix) ||
			strings.HasPrefix(r.URL.Path, "/assets-admin/") {
			if serveAdminSPA != nil {
				serveAdminSPA(w, r)
				return
			}
			http.Error(w, "Chưa build app dọn dẹp. Chạy: cd frontend-admin && npm run build", 503)
			return
		}
		if serveSPA != nil {
			serveSPA(w, r)
		}
	})

	addr := fmt.Sprintf(":%d", port)
	fmt.Printf("\n🚀  Backend API: http://localhost%s\n", addr)
	fmt.Printf("    Frontend:    http://localhost:5173\n")
	fmt.Println("    Nhấn Ctrl+C để dừng")
	return http.ListenAndServe(addr, handler)
}

// hkPathPrefix — app dọn dẹp phục vụ dưới đường dẫn con này.
//
// Dùng đường dẫn con thay vì tên miền phụ để đi chung tunnel Cloudflare sẵn có
// của video.quanlyhomestay.com, khỏi phải khai báo route mới.
const hkPathPrefix = "/unixstay"

// isAdminHost vẫn nhận tên miền bắt đầu bằng "admin." — giữ lại để sau này muốn
// tách sang admin.quanlyhomestay.com thì chỉ phải thêm route tunnel, không phải
// sửa code. Hiện tại không tên miền nào trỏ vào đây nên nhánh này im lặng.
func isAdminHost(r *http.Request) bool {
	host := r.Host
	if i := strings.IndexByte(host, ':'); i >= 0 {
		host = host[:i]
	}
	return strings.HasPrefix(strings.ToLower(host), "admin.")
}

// makeSPAHandler phục vụ một thư mục build của Vite kèm SPA fallback. Trả nil khi
// thư mục chưa tồn tại, để chỗ gọi báo lỗi cho người dùng biết cần build.
//
// Đường dẫn được làm sạch trước khi ghép: request "/../../etc/passwd" mà ghép
// thẳng vào filepath.Join là đọc được file ngoài thư mục dist.
func makeSPAHandler(distDir string) http.HandlerFunc {
	if _, err := os.Stat(distDir); err != nil {
		return nil
	}
	fs := http.FileServer(http.Dir(distDir))
	indexPath := filepath.Join(distDir, "index.html")
	return func(w http.ResponseWriter, r *http.Request) {
		clean := filepath.Clean("/" + strings.TrimPrefix(r.URL.Path, hkPathPrefix))
		if clean == "/" {
			http.ServeFile(w, r, indexPath)
			return
		}
		if _, err := os.Stat(filepath.Join(distDir, clean)); os.IsNotExist(err) {
			// Đường dẫn nội bộ của SPA (VD /cleaning/session/abc) → trả index.html
			// để React tự định tuyến.
			http.ServeFile(w, r, indexPath)
			return
		}
		r2 := *r
		u := *r.URL
		u.Path = clean
		r2.URL = &u
		fs.ServeHTTP(w, &r2)
	}
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
			// App quản lý dọn dẹp chạy dev ở cổng riêng để build app này không phải
			// dừng công cụ video đang chạy.
			"http://localhost:5174",
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
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-HK-Token")
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
		}
		if cfg.Mode == "" {
			cfg.Mode = "kenburns"
		}

		go func() {
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
			switch cfg.Mode {
			case "slideshow":
				args, err = buildSlideshow(cfg, images)
			case "timelapse":
				args, err = buildTimelapse(cfg, images)
			default:
				args, err = buildKenBurns(cfg, images)
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
