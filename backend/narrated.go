package main

import (
	"fmt"
	"hash/fnv"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
)

// ─── Mode "narrated": ghép video có lời kể + phụ đề + nhạc nền duck ──────────

const (
	narratedCross = 0.5 // crossfade giữa các cảnh (giây)
	narratedPad   = 1.6 // đệm mỗi cảnh: 0.8 dẫn + 0.8 đuôi (≥ 2·cross + 0.2)

	narratedMinWords = 13 // đoạn ngắn hơn thế thì cụt, hết hài
)

// narrationRate = tốc độ đọc thực tế (từ/giây) theo TTS provider.
// google ĐO THẬT 2026-07-14: ~2.2-2.4 từ/s (render 8 cảnh ra 57.3s ✓).
// fpt ĐO THẬT: Marketplace VITs std_kimngan 16 từ → 5.4s ≈ 3.0; banmai cũ ≈ 3.6.
// elevenlabs ĐO THẬT 2026-07-15 (George đọc VN): 2.58-3.06 từ/s qua 3 render → 2.8.
func narrationRate(provider string) float64 {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "", "google", "free":
		return 2.2
	case "fpt":
		if strings.HasPrefix(strings.TrimSpace(os.Getenv("FPT_TTS_API_KEY")), "sk-") {
			return 2.6 // FPT AI Marketplace (VITs) — đo render thật: 152 từ/58.1s
		}
		return 3.6 // FPT.AI cũ (banmai...)
	default: // elevenlabs
		return 2.8
	}
}

// narrationBudget tính (số cảnh hiệu dụng, số từ tối đa/đoạn) để video narrated
// vừa khít targetSec với giọng của provider. Từ computeNarratedTimeline:
// Total ≈ Σv_i + n·(pad−cross) + cross, tức overhead 1.1s/cảnh + 0.5s cố định.
// targetSec <= 0 → tắt giới hạn (giữ nguyên n, budget 0 = prompt như cũ).
func narrationBudget(targetSec float64, n int, provider string) (effN, wordsPerSeg int) {
	if targetSec <= 0 || n <= 0 {
		return n, 0
	}
	rate := narrationRate(provider)
	minSpeech := narratedMinWords / rate // giây đọc tối thiểu cho 1 cảnh còn "đáng"
	h := narratedPad - narratedCross     // overhead mỗi cảnh (1.1s)
	nMax := int((targetSec - narratedCross) / (minSpeech + h))
	if nMax < 1 {
		nMax = 1
	}
	effN = n
	if effN > nMax {
		effN = nMax
	}
	speech := (targetSec - narratedCross - float64(effN)*h) / float64(effN)
	wordsPerSeg = int(speech * rate)
	if wordsPerSeg < 10 {
		wordsPerSeg = 10
	}
	return effN, wordsPerSeg
}

// filepathJoinTemp trả về đường dẫn trong thư mục temp hệ thống.
func filepathJoinTemp(name string) string {
	return filepath.Join(os.TempDir(), name)
}

func scriptWordCount(s *NarrationScript) int {
	total := 0
	for _, seg := range s.Segments {
		total += len(strings.Fields(seg.Narration))
	}
	return total
}

func sumFloat(xs []float64) float64 {
	t := 0.0
	for _, x := range xs {
		t += x
	}
	return t
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
	case "", "google", "free":
		// Google TTS miễn phí, không cần key.
	case "fpt":
		if os.Getenv("FPT_TTS_API_KEY") == "" {
			return fmt.Errorf("Chưa có FPT_TTS_API_KEY — cần key FPT.AI để lồng giọng (hoặc đổi sang Google free / ElevenLabs)")
		}
	default:
		// ElevenLabs thiếu key KHÔNG chặn render nữa — synthSegments sẽ tự
		// chuyển sang giọng Google free (có warning trong progress).
		if os.Getenv("ELEVENLABS_API_KEY") == "" {
			reportProgress("⚠️ Chưa có ELEVENLABS_API_KEY — sẽ tự dùng giọng Google free")
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

// capNarratedImages cắt danh sách ảnh theo MaxSegments (mặc định 10, trần 20)
// và theo thời lượng mục tiêu (số cảnh tối đa tuỳ tốc độ giọng provider).
// Dùng chung cho buildNarrated và /api/script để hai nơi cap giống hệt nhau.
func capNarratedImages(cfg Config, images []string) []string {
	maxSeg := cfg.MaxSegments
	if maxSeg <= 0 {
		maxSeg = 10
	}
	if maxSeg > 20 {
		maxSeg = 20
	}
	if effN, _ := narrationBudget(float64(cfg.TargetDuration), maxSeg, cfg.TTSProvider); effN < maxSeg {
		reportProgress(fmt.Sprintf("⏱️ Giảm còn %d cảnh cho vừa ~%ds", effN, cfg.TargetDuration))
		maxSeg = effN
	}
	if len(images) > maxSeg {
		images = images[:maxSeg]
	}
	return images
}

// buildNarrated dựng lệnh ffmpeg cho mode "narrated".
func buildNarrated(cfg Config, images []string) ([]string, error) {
	if err := narratePreflight(cfg); err != nil {
		return nil, err
	}

	images = capNarratedImages(cfg, images)
	if len(images) == 0 {
		return nil, fmt.Errorf("không có ảnh để dựng video")
	}
	cfg.GridIntro = false // narrated không dùng lưới intro

	// Phụ đề ASS + hook + sticker/khói đều được thiết kế cho khung DỌC 9:16
	// (PlayResX/Y=1080/1920, toạ độ px cố định, khói pad 1080×1920). Khung khác
	// 9:16 sẽ (a) làm libass kéo méo phụ đề, (b) khiến blend khói lệch kích thước
	// → HỎNG cả render. Nên ép narrated về đúng 1080×1920 dù toggle 9:16 tắt.
	if cfg.Width*16 != cfg.Height*9 {
		reportProgress("ℹ️ Thuyết minh AI luôn xuất video dọc 9:16 (1080×1920)")
	}
	cfg.Width, cfg.Height = 1080, 1920

	// 1) Kịch bản: bản user đã duyệt/sửa (cfg.Script) hoặc Claude viết.
	reportProgress("🧠 Đang viết kịch bản (Claude nhìn ảnh)…")
	script, err := scriptForConfig(cfg, images)
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

	// Kịch bản sống sót tới TTS = sẽ vào video → lưu vào thư viện (user like
	// bản hay để thành ví dụ few-shot cho các lần viết sau).
	saveScriptEntry(cfg, script, cfg.Script != nil)

	// Log tốc độ đọc thực tế để hiệu chỉnh narrationRate (từ/giây theo provider).
	if totalWords, totalVoice := scriptWordCount(script), sumFloat(voiceDur); totalVoice > 0 {
		fmt.Fprintf(os.Stderr, "ℹ️  Tốc độ đọc thực tế: %.2f từ/s (%d từ / %.1fs, provider %s)\n",
			float64(totalWords)/totalVoice, totalWords, totalVoice, cfg.TTSProvider)
	}

	// 3) Timeline đồng bộ.
	tl := computeNarratedTimeline(voiceDur, narratedCross, narratedPad, cfg.FPS)

	if cfg.TargetDuration > 0 && tl.Total > float64(cfg.TargetDuration)*1.2 {
		fmt.Fprintf(os.Stderr, "⚠️  Video %.1fs vượt mục tiêu %ds (lời kể dài hơn dự kiến)\n", tl.Total, cfg.TargetDuration)
	}

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

	// ── Phụ đề ASS (karaoke/typewriter + hook) — thay bảng giá + caption pill.
	// Narrated KHÔNG dùng template overlay nữa (bảng giá/panel/veil), chỉ giữ
	// watermark; toàn bộ chữ động (hook typewriter, karaoke theo giọng, tim bay)
	// nằm trong 1 file .ass render bằng libass. Xem narrated_subtitles.go.
	// Dọn thư mục subs của lần render trước (chứa subs.ass + ~1.2MB font đã copy)
	// — args trả về còn tham chiếu subsTmp nên không thể defer-remove ngay ở đây;
	// best-effort GC lần trước để không rò rỉ tích luỹ.
	if olds, _ := filepath.Glob(filepath.Join(os.TempDir(), "img2video_subs_*")); olds != nil {
		for _, d := range olds {
			os.RemoveAll(d)
		}
	}
	subsTmp, err := os.MkdirTemp("", "img2video_subs_*")
	if err != nil {
		return nil, err
	}
	hookEnd := tl.Total
	if n > 1 {
		hookEnd = tl.Offsets[1]
	}
	hookL1, hookL2, hookEmph := script.HookLine1, script.HookLine2, script.HookEmphasis
	if strings.TrimSpace(hookL1)+strings.TrimSpace(hookL2) == "" {
		hookL1, hookL2, hookEmph = composeHookTitle(cfg)
	}
	// Grounding: giá hiện trên hook phải khớp giá trong dữ liệu listing (API).
	if tok, bad := hookPriceMismatch(hookL1+" "+hookL2, cfg.Prices); bad {
		reportProgress(fmt.Sprintf("⚠️ Giá %q trên tiêu đề hook KHÔNG khớp dữ liệu giá listing — sửa lại ở panel Xem kịch bản", tok))
	}
	subtitleStyle := normalizeSubtitleStyle(cfg.SubtitleStyle)
	stickerPath := stickerAssetPath("meme_cat.png")
	smokePath := stickerAssetPath("pink_ink.mp4")
	if subtitleStyle == "typewriter" {
		// Kiểu lovebox: sạch — không sticker/tim/khói (chỉ karaoke có trang trí).
		stickerPath, smokePath = "", ""
	} else if stickerPath == "" {
		reportProgress("ℹ️ Không có assets/stickers/meme_cat.png — bỏ qua sticker + tim bay")
	}
	seedH := fnv.New32a()
	seedH.Write([]byte(cfg.ListingID))
	assPath, fontsDir, err := writeNarratedASS(narratedSubtitleSpec{
		Style:      subtitleStyle,
		Script:     script,
		Timeline:   tl,
		VoiceDur:   voiceDur,
		VoicePaths: voicePaths,
		Accent:     accentForTemplate(cfg.Template),
		HookEnd:    hookEnd,
		HookLine1:  hookL1,
		HookLine2:  hookL2,
		HookEmph:   hookEmph,
		Amenities:  cfg.Amenities,
		Hearts:     stickerPath != "",
		Seed:       int64(seedH.Sum32()),
	}, subsTmp)
	if err != nil {
		return nil, fmt.Errorf("sinh phụ đề ASS: %v", err)
	}

	// ── Inputs: ảnh, watermark PNG, sticker, khói, giọng mp3, (nhạc) ──
	var args []string
	for i := 0; i < n; i++ {
		t := tl.ClipDur[i] + tl.Cross + 0.2
		args = append(args, "-loop", "1", "-t", fmt.Sprintf("%.3f", t), "-i", segImages[i])
	}

	var overlayPlans []OverlayPlan
	if wm, werr := renderWatermarkPlan(cfg, subsTmp); werr == nil && wm != nil {
		overlayPlans = append(overlayPlans, *wm)
	}
	templateFirstIdx := n
	for _, p := range overlayPlans {
		args = append(args, "-i", p.PNGPath)
	}

	nextIdx := n + len(overlayPlans)
	stkIdx, smkIdx := -1, -1
	if stickerPath != "" {
		stkIdx = nextIdx
		nextIdx++
		args = append(args, "-i", stickerPath)
	}
	if smokePath != "" && tl.Total > 2.5 {
		smkIdx = nextIdx
		nextIdx++
		args = append(args, "-i", smokePath)
	}

	// Giọng đọc mp3.
	voiceFirstIdx := nextIdx
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

	if chain := buildOverlayFilterChain(overlayPlans, mapTarget, "[vwm]", templateFirstIdx); chain != "" {
		filterStr += chain
		mapTarget = "[vwm]"
	}
	// Sticker mèo: trượt từ mép trên xuống y=240 trong ~0.4s (ease-out), đứng
	// yên trên hook, biến mất cùng hook (cắt phụt như video mẫu).
	if stkIdx >= 0 {
		filterStr += fmt.Sprintf(";\n[%d:v]scale=190:-1[stk]", stkIdx)
		filterStr += fmt.Sprintf(
			";\n%s[stk]overlay=x=(main_w-overlay_w)/2:y='if(lt(t,0.45),240-(240+overlay_h)*pow(1-min(t/0.4,1),2),240)':enable='between(t,0.05,%.3f)'[vstk]",
			mapTarget, hookEnd)
		mapTarget = "[vstk]"
	}
	// Khói hồng screen-blend 0–1.8s (nền đen thuần → sau fade là no-op). Pad
	// đúng kích thước khung (cfg.Width×Height) — blend yêu cầu 2 nhánh cùng cỡ,
	// hardcode 1080×1920 sẽ vỡ nếu khung khác.
	if smkIdx >= 0 {
		filterStr += fmt.Sprintf(
			";\n[%d:v]trim=0:1.8,setpts=PTS-STARTPTS,scale=%d:-2,pad=%d:%d:(ow-iw)/2:450:black,fade=t=out:st=1.3:d=0.5,tpad=stop_mode=add:stop_duration=%.3f,format=gbrp[smk]",
			smkIdx, cfg.Width, cfg.Width, cfg.Height, tl.Total-1.8)
		filterStr += fmt.Sprintf(";\n%sformat=gbrp[vg];\n[vg][smk]blend=all_mode=screen:all_opacity=0.85[vsmk]", mapTarget)
		mapTarget = "[vsmk]"
	}
	// Phụ đề ASS LUÔN CUỐI CÙNG (đè lên mọi lớp).
	filterStr += fmt.Sprintf(";\n%s%s[vsub]", mapTarget, subtitlesFilterArg(assPath, fontsDir))
	mapTarget = "[vsub]"

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

// (renderCaptionPlans đã bỏ: caption pill nhãn phòng được thay bằng phụ đề
// karaoke/typewriter theo giọng đọc trong narrated_subtitles.go.)
