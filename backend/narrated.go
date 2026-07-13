package main

import (
	"fmt"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"strings"

	"img2video/textrender"
)

// ─── Mode "narrated": ghép video có lời kể + phụ đề + nhạc nền duck ──────────

const (
	narratedCross = 0.5 // crossfade giữa các cảnh (giây)
	narratedPad   = 1.6 // đệm mỗi cảnh: 0.8 dẫn + 0.8 đuôi (≥ 2·cross + 0.2)
)

// filepathJoinTemp trả về đường dẫn trong thư mục temp hệ thống.
func filepathJoinTemp(name string) string {
	return filepath.Join(os.TempDir(), name)
}

// reportProgress cập nhật tiến trình cho /api/status (và in ra stderr).
func reportProgress(msg string) {
	fmt.Println(msg)
	if state != nil {
		state.mu.Lock()
		state.progress = msg
		state.mu.Unlock()
	}
}

// narratePreflight kiểm tra điều kiện TRƯỚC khi tốn API: có nguồn Claude nào
// khả dụng, và có key TTS cho provider đã chọn.
func narratePreflight(cfg Config) error {
	if !claudeCLIAvailable() && os.Getenv("ANTHROPIC_API_KEY") == "" {
		return fmt.Errorf("Chưa có nguồn AI để viết kịch bản: cần Claude Code đăng nhập (lệnh `claude`) HOẶC đặt biến môi trường ANTHROPIC_API_KEY trên máy chạy backend")
	}
	switch strings.ToLower(strings.TrimSpace(cfg.TTSProvider)) {
	case "google", "free":
		// Google TTS miễn phí, không cần key.
	case "fpt":
		if os.Getenv("FPT_TTS_API_KEY") == "" {
			return fmt.Errorf("Chưa có FPT_TTS_API_KEY — cần key FPT.AI để lồng giọng (hoặc đổi sang Google free / ElevenLabs)")
		}
	default:
		if os.Getenv("ELEVENLABS_API_KEY") == "" {
			return fmt.Errorf("Chưa có ELEVENLABS_API_KEY — cần key ElevenLabs để lồng giọng (hoặc đổi sang Google free / FPT.AI)")
		}
	}
	return nil
}

// narratedTimeline chứa toàn bộ thời gian đã tính (đã quantize theo fps).
type narratedTimeline struct {
	Frames   []int     // số frame zoompan mỗi cảnh
	ClipDur  []float64 // "advance" mỗi cảnh d_i (spacing cho xfade offset)
	Offsets  []float64 // offset xfade cho cảnh i (i>=1); Offsets[0] không dùng
	VoiceAt  []float64 // thời điểm bắt đầu giọng đọc mỗi cảnh (tuyệt đối)
	CapStart []float64 // phụ đề cảnh i bật lúc
	CapEnd   []float64 // phụ đề cảnh i tắt lúc
	Total    float64   // tổng thời lượng video sau trim
	Cross    float64
}

// computeNarratedTimeline tính lịch từ độ dài giọng đọc từng cảnh. Hàm thuần
// (không I/O) để unit-test được.
//
//	F_i     = round((v_i + pad + cross) * fps)   // số frame zoompan
//	d_i     = F_i/fps − cross                     // advance đã quantize
//	O_i     = Σ_{j<i} d_j − i·cross               // offset xfade (i≥1)
//	T       = Σ d_j − (n−1)·cross                 // tổng
//	W_i     = (i==0)? 0 : O_i + cross             // cảnh i "một mình"
//	E_i     = (i==n−1)? T : O_{i+1}
//	A_i     = W_i + max(0.15, (E_i − W_i − v_i)/2) // giọng đặt giữa cửa sổ
func computeNarratedTimeline(voiceDur []float64, cross, pad float64, fps int) narratedTimeline {
	n := len(voiceDur)
	tl := narratedTimeline{
		Frames:   make([]int, n),
		ClipDur:  make([]float64, n),
		Offsets:  make([]float64, n),
		VoiceAt:  make([]float64, n),
		CapStart: make([]float64, n),
		CapEnd:   make([]float64, n),
		Cross:    cross,
	}
	if n == 0 {
		return tl
	}
	ff := float64(fps)
	cumD := 0.0
	for i := 0; i < n; i++ {
		F := int(math.Round((voiceDur[i] + pad + cross) * ff))
		if F < int((pad+cross)*ff) {
			F = int((pad + cross) * ff)
		}
		d := float64(F)/ff - cross
		tl.Frames[i] = F
		tl.ClipDur[i] = d
		if i >= 1 {
			tl.Offsets[i] = cumD - float64(i)*cross
		}
		cumD += d
	}
	tl.Total = cumD - float64(n-1)*cross

	for i := 0; i < n; i++ {
		var w, e float64
		if i == 0 {
			w = 0
		} else {
			w = tl.Offsets[i] + cross
		}
		if i == n-1 {
			e = tl.Total
		} else {
			e = tl.Offsets[i+1]
		}
		tl.CapStart[i] = w
		tl.CapEnd[i] = e
		lead := (e - w - voiceDur[i]) / 2
		if lead < 0.15 {
			lead = 0.15
		}
		tl.VoiceAt[i] = w + lead
	}
	return tl
}

// buildNarrated dựng lệnh ffmpeg cho mode "narrated".
func buildNarrated(cfg Config, images []string) ([]string, error) {
	if err := narratePreflight(cfg); err != nil {
		return nil, err
	}

	// Cap số ảnh theo MaxSegments (mặc định 10).
	maxSeg := cfg.MaxSegments
	if maxSeg <= 0 {
		maxSeg = 10
	}
	if maxSeg > 20 {
		maxSeg = 20
	}
	if len(images) > maxSeg {
		images = images[:maxSeg]
	}
	if len(images) == 0 {
		return nil, fmt.Errorf("không có ảnh để dựng video")
	}
	cfg.GridIntro = false // narrated không dùng lưới intro

	// 1) Kịch bản (Claude).
	reportProgress("🧠 Đang viết kịch bản (Claude nhìn ảnh)…")
	script, err := genScript(cfg, images)
	if err != nil {
		return nil, err
	}

	// Chỉ giữ đúng những ảnh có lời kể, theo thứ tự script.
	segImages := make([]string, len(script.Segments))
	for i, seg := range script.Segments {
		segImages[i] = images[seg.ImageIndex]
	}
	n := len(segImages)
	if n == 0 {
		return nil, fmt.Errorf("kịch bản rỗng")
	}

	// 2) Giọng đọc TTS.
	voicePaths, voiceDur, err := synthSegments(cfg, script)
	if err != nil {
		return nil, err
	}

	// 3) Timeline đồng bộ.
	tl := computeNarratedTimeline(voiceDur, narratedCross, narratedPad, cfg.FPS)

	reportProgress(fmt.Sprintf("🎬 Dựng video %d cảnh · %.1fs", n, tl.Total))

	// Hiệu ứng Ken Burns cho từng cảnh (theo hướng ảnh, per-clip vì zoompan nhúng
	// số frame vào biểu thức).
	zDelta := 0.08 + cfg.ZoomIntensity*0.15
	perEffect := make([]effect, n)
	staticFx := effect{name: "static", z: "1", x: "iw/2-(iw/zoom/2)", y: "ih/2-(ih/zoom/2)"}
	randomKinds := []string{"zoom-in", "zoom-out", "move-left-right", "move-right-left"}

	// Kiểu chuyển động do motion mode + chips 'Kiểu chuyển động' quyết định:
	//   slideshow → ảnh tĩnh (chỉ crossfade + giọng đọc + phụ đề)
	//   kenburns  → theo chip đã chọn (EffectType); "random"/rỗng → tự theo hướng ảnh
	//   timelapse/khác → coi như kenburns (auto theo hướng ảnh)
	chosenKind := strings.TrimSpace(cfg.EffectType)
	if chosenKind == "" && len(cfg.EffectTypes) > 0 {
		chosenKind = strings.TrimSpace(cfg.EffectTypes[0])
	}
	// Chỉ Ken Burns mới honor chip 'Kiểu chuyển động'; slideshow=tĩnh (xử lý riêng
	// bên dưới), timelapse/khác → auto theo hướng ảnh (chip UI ẩn ở các mode này
	// nhưng FE vẫn gửi effect_type mặc định, nên phải gate theo mode ở đây).
	chipFixed := cfg.Mode == "kenburns" && chosenKind != "" && chosenKind != "random" && chosenKind != "mixed"

	for i := 0; i < n; i++ {
		if cfg.Mode == "slideshow" {
			perEffect[i] = staticFx
			continue
		}
		all := buildEffects(zDelta, tl.Frames[i])
		var pool []effect
		if chipFixed {
			pool = filterEffects(all, chosenKind) // honor chip 'Kiểu chuyển động'
		} else {
			k := randomKinds[rand.Intn(len(randomKinds))]
			if orient, oerr := imageOrientation(segImages[i]); oerr == nil {
				switch orient {
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
			pool = filterEffects(all, k)
		}
		if len(pool) == 0 {
			pool = all
		}
		perEffect[i] = pool[rand.Intn(len(pool))]
	}

	// ── Inputs: ảnh, template PNG, caption PNG, giọng mp3, (nhạc) ──
	var args []string
	for i := 0; i < n; i++ {
		t := tl.ClipDur[i] + tl.Cross + 0.2
		args = append(args, "-loop", "1", "-t", fmt.Sprintf("%.3f", t), "-i", segImages[i])
	}

	// Template overlay (thông tin listing xuyên suốt) — tái dùng hệ có sẵn.
	overlayPlans, _, err := prepareTextOverlayPlans(cfg)
	if err != nil {
		return nil, fmt.Errorf("textrender overlays: %v", err)
	}
	templateFirstIdx := n
	for _, p := range overlayPlans {
		args = append(args, "-i", p.PNGPath)
	}

	// Caption PNG per-segment.
	capTmp, err := os.MkdirTemp("", "img2video_caption_*")
	if err != nil {
		return nil, err
	}
	captionPlans, err := renderCaptionPlans(cfg, script, tl, capTmp)
	if err != nil {
		return nil, fmt.Errorf("render phụ đề: %v", err)
	}
	captionFirstIdx := n + len(overlayPlans)
	for _, p := range captionPlans {
		args = append(args, "-i", p.PNGPath)
	}

	// Giọng đọc mp3.
	voiceFirstIdx := captionFirstIdx + len(captionPlans)
	for i := 0; i < n; i++ {
		args = append(args, "-i", voicePaths[i])
	}

	// Nhạc nền (tuỳ chọn).
	musicIdx := -1
	if cfg.Audio != "" {
		if _, serr := os.Stat(cfg.Audio); serr == nil {
			musicIdx = voiceFirstIdx + n
			args = append(args, "-i", cfg.Audio)
		} else {
			fmt.Fprintf(os.Stderr, "⚠️  Không thấy nhạc: %s\n", cfg.Audio)
		}
	}

	// ── Filtergraph video ──
	var fc strings.Builder
	for i := 0; i < n; i++ {
		ef := perEffect[i]
		fc.WriteString(fmt.Sprintf(
			"[%d:v]%s,zoompan=z='%s':x='%s':y='%s':d=%d:s=%dx%d:fps=%d[zp%d];\n",
			i, scaleCrop(cfg.Width, cfg.Height),
			ef.z, ef.x, ef.y,
			tl.Frames[i], cfg.Width, cfg.Height, cfg.FPS, i,
		))
	}
	if n == 1 {
		fc.WriteString("[zp0]copy[vout]")
	} else {
		prev := "zp0"
		for i := 1; i < n; i++ {
			out := fmt.Sprintf("xf%d", i)
			if i == n-1 {
				out = "vout"
			}
			fc.WriteString(fmt.Sprintf(
				"[%s][zp%d]xfade=transition=fade:duration=%.2f:offset=%.3f[%s];\n",
				prev, i, tl.Cross, tl.Offsets[i], out))
			prev = out
		}
	}
	filterStr := strings.TrimRight(fc.String(), ";\n")
	filterStr += fmt.Sprintf(";\n[vout]trim=duration=%.3f,setpts=PTS-STARTPTS[vtrim]", tl.Total)
	mapTarget := "[vtrim]"

	if chain := buildOverlayFilterChain(overlayPlans, mapTarget, "[vtpl]", templateFirstIdx); chain != "" {
		filterStr += chain
		mapTarget = "[vtpl]"
	}
	if chain := buildOverlayFilterChain(captionPlans, mapTarget, "[vfinal]", captionFirstIdx); chain != "" {
		filterStr += chain
		mapTarget = "[vfinal]"
	}

	// ── Filtergraph audio ──
	filterStr += fmt.Sprintf(";\nanullsrc=r=44100:cl=stereo,atrim=duration=%.3f[abase]", tl.Total)
	voiceLabels := []string{"[abase]"}
	for i := 0; i < n; i++ {
		delayMs := int(tl.VoiceAt[i] * 1000)
		lbl := fmt.Sprintf("[d%d]", i)
		filterStr += fmt.Sprintf(";\n[%d:a]aformat=sample_rates=44100:channel_layouts=stereo,adelay=%d|%d%s",
			voiceFirstIdx+i, delayMs, delayMs, lbl)
		voiceLabels = append(voiceLabels, lbl)
	}
	filterStr += fmt.Sprintf(";\n%samix=inputs=%d:duration=first:normalize=0[voice]",
		strings.Join(voiceLabels, ""), len(voiceLabels))

	audioOut := "[aout]"
	if musicIdx >= 0 {
		filterStr += fmt.Sprintf(";\n[%d:a]aloop=loop=-1:size=2147483647,atrim=duration=%.3f,volume=0.55[bgm]", musicIdx, tl.Total)
		filterStr += ";\n[voice]asplit=2[vc1][vc2]"
		filterStr += ";\n[bgm][vc1]sidechaincompress=threshold=0.05:ratio=10:attack=20:release=500[duck]"
		filterStr += ";\n[duck][vc2]amix=inputs=2:duration=first:normalize=0[aout]"
	} else {
		filterStr += ";\n[voice]alimiter=limit=0.95[aout]"
	}

	args = append(args,
		"-filter_complex", filterStr,
		"-map", mapTarget,
		"-map", audioOut,
		"-c:v", "libx264", "-preset", "fast", "-crf", "20",
		"-pix_fmt", "yuv420p",
		"-c:a", "aac", "-b:a", "192k",
		"-movflags", "+faststart",
	)
	return append(args, "-y", cfg.Output), nil
}

// renderCaptionPlans vẽ 1 PNG phụ đề pill cho mỗi cảnh, bật/tắt theo cửa sổ
// [CapStart, CapEnd]. Dùng textrender (KHÔNG dùng Chrome — Chrome nướng sẵn veil
// tối full-frame + chậm).
func renderCaptionPlans(cfg Config, script *NarrationScript, tl narratedTimeline, tmpDir string) ([]OverlayPlan, error) {
	ctx := &textrender.RenderContext{
		VideoWidth:  cfg.Width,
		VideoHeight: cfg.Height,
		AssetsDir:   assetsDir(),
	}
	var plans []OverlayPlan
	for i, seg := range script.Segments {
		text := strings.TrimSpace(seg.Caption)
		if text == "" {
			continue
		}
		style := &textrender.ElementStyle{
			Text:     text,
			FontFile: "BeVietnamPro-Bold.ttf",
			SizePct:  0.036,
			Color:    "#FFFFFF",
			Bg: &textrender.BgStyle{
				Color:   "#000000",
				Alpha:   0.55,
				Radius:  20,
				Padding: [2]float64{12, 28},
			},
			Position:    textrender.Position{X: "center", Y: "0.72"},
			Align:       "center",
			MaxWidthPct: 0.82,
		}
		out, err := textrender.Render(style, ctx)
		if err != nil || out == nil {
			continue
		}
		path := filepath.Join(tmpDir, fmt.Sprintf("cap%02d.png", i))
		if err := out.Save(path); err != nil {
			continue
		}
		enable := fmt.Sprintf("between(t,%.3f,%.3f)", tl.CapStart[i], tl.CapEnd[i])
		plans = append(plans, OverlayPlan{PNGPath: path, X: out.X, Y: out.Y, EnableExpr: enable})
	}
	return plans, nil
}
