package main

import (
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// ─── Unit test thuần cho timeline (không cần API/ffmpeg) ────────────────────

func TestComputeNarratedTimeline(t *testing.T) {
	cases := [][]float64{
		{2.0, 3.0, 1.5},
		{1.0},
		{4.2, 0.8, 2.5, 3.3, 1.1},
	}
	const cross, pad = 0.5, 1.6
	const fps = 30
	for _, voice := range cases {
		tl := computeNarratedTimeline(voice, cross, pad, fps)
		n := len(voice)

		// 1) Frames khớp lưới fps: (ClipDur+cross)*fps == Frames.
		for i := 0; i < n; i++ {
			got := int(math.Round((tl.ClipDur[i] + cross) * fps))
			if got != tl.Frames[i] {
				t.Fatalf("case %v: Frames[%d]=%d nhưng (ClipDur+cross)*fps=%d", voice, i, tl.Frames[i], got)
			}
		}

		// 2) Tổng đúng công thức Σd − (n−1)·cross.
		sumD := 0.0
		for _, d := range tl.ClipDur {
			sumD += d
		}
		wantTotal := sumD - float64(n-1)*cross
		if math.Abs(tl.Total-wantTotal) > 1e-6 {
			t.Fatalf("case %v: Total=%.4f want %.4f", voice, tl.Total, wantTotal)
		}

		// 3) Offsets tăng dần (i>=1) và offset_i + cross <= offset_{i+1}.
		for i := 2; i < n; i++ {
			if tl.Offsets[i] <= tl.Offsets[i-1] {
				t.Fatalf("case %v: Offsets không tăng: [%d]=%.3f [%d]=%.3f", voice, i-1, tl.Offsets[i-1], i, tl.Offsets[i])
			}
		}

		// 4) Cửa sổ phụ đề không chồng nhau và đủ dài cho giọng.
		for i := 0; i < n; i++ {
			win := tl.CapEnd[i] - tl.CapStart[i]
			if win < voice[i]-1e-6 {
				t.Fatalf("case %v: cửa sổ cảnh %d (%.3f) < giọng (%.3f)", voice, i, win, voice[i])
			}
			// giọng nằm trọn trong cửa sổ
			if tl.VoiceAt[i] < tl.CapStart[i]-1e-6 {
				t.Fatalf("case %v: VoiceAt[%d]=%.3f < CapStart=%.3f", voice, i, tl.VoiceAt[i], tl.CapStart[i])
			}
			if tl.VoiceAt[i]+voice[i] > tl.CapEnd[i]+1e-6 {
				t.Fatalf("case %v: giọng cảnh %d kết thúc %.3f > CapEnd %.3f", voice, i, tl.VoiceAt[i]+voice[i], tl.CapEnd[i])
			}
			if i < n-1 && tl.CapEnd[i] > tl.CapStart[i+1]+1e-6 {
				t.Fatalf("case %v: phụ đề %d chồng %d (%.3f > %.3f)", voice, i, i+1, tl.CapEnd[i], tl.CapStart[i+1])
			}
		}

		// 5) n==1: Total == ClipDur[0], không có offset.
		if n == 1 && math.Abs(tl.Total-tl.ClipDur[0]) > 1e-6 {
			t.Fatalf("n=1: Total=%.3f != ClipDur[0]=%.3f", tl.Total, tl.ClipDur[0])
		}
	}
}

// ─── End-to-end gated bằng GENNARRATED=1 (tự chứa, không cần key/mạng) ──────
//
// Sinh ảnh màu bằng ffmpeg, stub genScript + synthTTS (mp3 câm), rồi chạy thật
// buildNarrated → runFFmpeg và kiểm tra output là A/V hợp lệ.

func TestGenNarratedVideo(t *testing.T) {
	if os.Getenv("GENNARRATED") == "" {
		t.Skip("đặt GENNARRATED=1 để chạy end-to-end narrated")
	}
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("không có ffmpeg")
	}

	tmp := t.TempDir()
	// 3 ảnh: landscape, portrait, landscape.
	imgs := []string{
		makeColorImage(t, tmp, "0", "red", 1200, 800),
		makeColorImage(t, tmp, "1", "green", 800, 1200),
		makeColorImage(t, tmp, "2", "blue", 1200, 800),
	}

	// Stub script + TTS; khôi phục sau test.
	origScript, origTTS := genScript, synthTTS
	defer func() { genScript, synthTTS = origScript, origTTS }()

	genScript = func(cfg Config, images []string) (*NarrationScript, error) {
		return &NarrationScript{Segments: []NarrationSegment{
			{ImageIndex: 0, Caption: "Phòng khách", Narration: "Chào các con vợ, đây là phòng khách siêu xinh."},
			{ImageIndex: 1, Caption: "Giường ngủ", Narration: "Cái giường này nệm êm ru, nằm là muốn ngủ luôn."},
			{ImageIndex: 2, Caption: "Ban công", Narration: "Cuối cùng là ban công thoáng mát, nhớ lưu video nhé."},
		}}, nil
	}
	synthTTS = func(cfg Config, text string) ([]byte, error) {
		sec := 1.0 + float64(len([]rune(text)))/40.0
		return silentMP3(t, tmp, sec), nil
	}

	// Preflight cần key TTS + Claude; claude CLI có sẵn, giả lập key TTS.
	os.Setenv("ELEVENLABS_API_KEY", "test-dummy")
	defer os.Unsetenv("ELEVENLABS_API_KEY")

	out := filepath.Join(tmp, "out.mp4")
	cfg := Config{
		Mode: "narrated", Output: out,
		Width: 1080, Height: 1920, FPS: 30, ZoomIntensity: 0.4,
		TTSProvider: "elevenlabs", NarrationPersona: "haihuoc",
		MaxSegments: 10,
		Template:    "", // không template → tập trung vào core + captions + audio
	}

	args, err := buildNarrated(cfg, imgs)
	if err != nil {
		t.Fatalf("buildNarrated: %v", err)
	}
	if err := runFFmpeg(args, cfg); err != nil {
		t.Fatalf("runFFmpeg: %v", err)
	}

	st, err := os.Stat(out)
	if err != nil || st.Size() == 0 {
		t.Fatalf("output rỗng/không tồn tại: %v", err)
	}

	// Kiểm tra có cả video + audio và duration hợp lý.
	dur, err := audioDurationSec(out)
	if err != nil || dur < 3 || dur > 60 {
		t.Fatalf("duration bất thường: %.2f (err=%v)", dur, err)
	}
	if !hasStream(t, out, "audio") {
		t.Fatalf("output thiếu audio stream")
	}
	if !hasStream(t, out, "video") {
		t.Fatalf("output thiếu video stream")
	}
	t.Logf("✅ narrated OK: %s (%.1fs, %.1f KB)", out, dur, float64(st.Size())/1024)
}

// TestNarratedMusicDucking chạy buildNarrated CÓ nhạc nền → xác minh graph
// sidechaincompress hoạt động qua đúng code path (không chỉ test rời).
func TestNarratedMusicDucking(t *testing.T) {
	if os.Getenv("GENNARRATED") == "" {
		t.Skip("đặt GENNARRATED=1")
	}
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("không có ffmpeg")
	}
	tmp := t.TempDir()
	imgs := []string{
		makeColorImage(t, tmp, "0", "orange", 1200, 800),
		makeColorImage(t, tmp, "1", "purple", 800, 1200),
	}
	// nhạc nền = sine 6s
	music := filepath.Join(tmp, "bg.mp3")
	if out, err := exec.Command("ffmpeg", "-y", "-f", "lavfi",
		"-i", "sine=frequency=220:duration=6", "-c:a", "libmp3lame", music).CombinedOutput(); err != nil {
		t.Fatalf("tạo nhạc: %v\n%s", err, out)
	}

	origScript, origTTS := genScript, synthTTS
	defer func() { genScript, synthTTS = origScript, origTTS }()
	genScript = func(cfg Config, images []string) (*NarrationScript, error) {
		return &NarrationScript{Segments: []NarrationSegment{
			{ImageIndex: 0, Caption: "Phòng khách", Narration: "Đây là phòng khách."},
			{ImageIndex: 1, Caption: "Giường ngủ", Narration: "Còn đây là giường ngủ êm ru."},
		}}, nil
	}
	synthTTS = func(cfg Config, text string) ([]byte, error) {
		return silentMP3(t, tmp, 1.0+float64(len([]rune(text)))/40.0), nil
	}
	os.Setenv("ELEVENLABS_API_KEY", "test-dummy")
	defer os.Unsetenv("ELEVENLABS_API_KEY")

	out := filepath.Join(tmp, "out.mp4")
	cfg := Config{
		Mode: "narrated", Output: out, Audio: music,
		Width: 1080, Height: 1920, FPS: 30, ZoomIntensity: 0.4,
		TTSProvider: "elevenlabs", NarrationPersona: "haihuoc", MaxSegments: 10,
	}
	args, err := buildNarrated(cfg, imgs)
	if err != nil {
		t.Fatalf("buildNarrated: %v", err)
	}
	if err := runFFmpeg(args, cfg); err != nil {
		t.Fatalf("runFFmpeg (music): %v", err)
	}
	if !hasStream(t, out, "audio") || !hasStream(t, out, "video") {
		t.Fatalf("output thiếu stream")
	}
	dur, _ := audioDurationSec(out)
	if dur < 2 || dur > 40 {
		t.Fatalf("duration bất thường (music): %.2f", dur)
	}
	t.Logf("✅ music+ducking OK: %.1fs", dur)
}

// TestNarratedEdgeCases phủ n==1 và caption rỗng (captionPlans lệch số với ảnh).
func TestNarratedEdgeCases(t *testing.T) {
	if os.Getenv("GENNARRATED") == "" {
		t.Skip("đặt GENNARRATED=1")
	}
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("không có ffmpeg")
	}
	os.Setenv("ELEVENLABS_API_KEY", "test-dummy")
	defer os.Unsetenv("ELEVENLABS_API_KEY")
	origScript, origTTS := genScript, synthTTS
	defer func() { genScript, synthTTS = origScript, origTTS }()
	synthTTS = func(cfg Config, text string) ([]byte, error) {
		return silentMP3(t, t.TempDir(), 1.5), nil
	}

	t.Run("single-scene", func(t *testing.T) {
		tmp := t.TempDir()
		imgs := []string{makeColorImage(t, tmp, "0", "teal", 1080, 1920)}
		genScript = func(cfg Config, images []string) (*NarrationScript, error) {
			return &NarrationScript{Segments: []NarrationSegment{
				{ImageIndex: 0, Caption: "Toàn cảnh", Narration: "Một cảnh duy nhất."},
			}}, nil
		}
		out := filepath.Join(tmp, "one.mp4")
		cfg := Config{Mode: "narrated", Output: out, Width: 1080, Height: 1920, FPS: 30, TTSProvider: "elevenlabs", MaxSegments: 10}
		args, err := buildNarrated(cfg, imgs)
		if err != nil {
			t.Fatalf("buildNarrated n=1: %v", err)
		}
		if err := runFFmpeg(args, cfg); err != nil {
			t.Fatalf("runFFmpeg n=1: %v", err)
		}
		if !hasStream(t, out, "audio") || !hasStream(t, out, "video") {
			t.Fatalf("n=1 output thiếu stream")
		}
	})

	t.Run("empty-caption", func(t *testing.T) {
		tmp := t.TempDir()
		imgs := []string{
			makeColorImage(t, tmp, "0", "red", 1200, 800),
			makeColorImage(t, tmp, "1", "green", 1200, 800),
			makeColorImage(t, tmp, "2", "blue", 800, 1200),
		}
		// cảnh giữa caption rỗng → chỉ 2 caption PNG cho 3 cảnh (test index alignment)
		genScript = func(cfg Config, images []string) (*NarrationScript, error) {
			return &NarrationScript{Segments: []NarrationSegment{
				{ImageIndex: 0, Caption: "Đầu", Narration: "Cảnh đầu."},
				{ImageIndex: 1, Caption: "", Narration: "Cảnh giữa không có caption."},
				{ImageIndex: 2, Caption: "Cuối", Narration: "Cảnh cuối."},
			}}, nil
		}
		out := filepath.Join(tmp, "cap.mp4")
		cfg := Config{Mode: "narrated", Output: out, Width: 1080, Height: 1920, FPS: 30, TTSProvider: "elevenlabs", MaxSegments: 10}
		args, err := buildNarrated(cfg, imgs)
		if err != nil {
			t.Fatalf("buildNarrated empty-caption: %v", err)
		}
		if err := runFFmpeg(args, cfg); err != nil {
			t.Fatalf("runFFmpeg empty-caption: %v", err)
		}
		if !hasStream(t, out, "video") {
			t.Fatalf("empty-caption output thiếu video")
		}
	})
}

// TestNarratedRealClaude chạy Claude Code THẬT nhìn ảnh thật (0đ qua gói Max) —
// chứng minh đường vision→kịch bản hoạt động. Gate riêng vì tốn quota + thời gian.
func TestNarratedRealClaude(t *testing.T) {
	if os.Getenv("GENNARRATED_REAL") == "" {
		t.Skip("đặt GENNARRATED_REAL=1 để gọi Claude Code thật")
	}
	if !claudeCLIAvailable() {
		t.Skip("không có claude CLI")
	}
	tmp := t.TempDir()
	imgs := []string{
		makeColorImage(t, tmp, "0", "0x8B4513", 1200, 800), // nâu
		makeColorImage(t, tmp, "1", "0x2E8B57", 800, 1200), // xanh
	}
	cfg := Config{
		NarrationPersona: "haihuoc",
		Nickname:         "Homestay Test",
		Address:          "Thanh Xuân, Hà Nội",
		Prices:           []string{"Giá giờ: 199K/2H", "Qua đêm: 519K"},
		Amenities:        []string{"Máy chiếu", "Bếp nấu", "Ban công"},
	}
	script, err := claudeCodeScript(cfg, imgs)
	if err != nil {
		t.Fatalf("claudeCodeScript thật lỗi: %v", err)
	}
	script = validateScript(script, len(imgs))
	if len(script.Segments) == 0 {
		t.Fatalf("Claude không trả segment nào")
	}
	for i, seg := range script.Segments {
		if strings.TrimSpace(seg.Narration) == "" {
			t.Fatalf("segment %d thiếu narration", i)
		}
		t.Logf("cảnh %d [%s]: %s", seg.ImageIndex, seg.Caption, seg.Narration)
	}
	t.Logf("✅ Claude Code thật trả %d cảnh hợp lệ", len(script.Segments))
}

// TestNarratedGoogleLive dựng video THẬT với giọng Google (miễn phí, không key):
// stub kịch bản (đỡ tốn Claude + xác định), nhưng TTS là Google THẬT → chứng minh
// giọng thật hoạt động end-to-end. Xuất file ra path cố định để mở nghe.
func TestNarratedGoogleLive(t *testing.T) {
	if os.Getenv("GENNARRATED_LIVE") == "" {
		t.Skip("đặt GENNARRATED_LIVE=1 để render video giọng Google thật")
	}
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("không có ffmpeg")
	}
	tmp := t.TempDir()
	imgs := []string{
		makeColorImage(t, tmp, "0", "0xC97B3C", 1200, 800),
		makeColorImage(t, tmp, "1", "0x3C8C6E", 800, 1200),
		makeColorImage(t, tmp, "2", "0x8B5FA8", 1200, 800),
	}
	origScript := genScript
	defer func() { genScript = origScript }()
	genScript = func(cfg Config, images []string) (*NarrationScript, error) {
		return &NarrationScript{Segments: []NarrationSegment{
			{ImageIndex: 0, Caption: "Tông cam ấm", Narration: "Chào các con vợ, mở màn là tông cam ấm áp cực kỳ chill nha."},
			{ImageIndex: 1, Caption: "Góc xanh mát", Narration: "Tiếp theo là mảng xanh mát rượi, nhìn thôi đã thấy thư giãn rồi."},
			{ImageIndex: 2, Caption: "Sắc tím mộng mơ", Narration: "Cuối cùng là sắc tím mộng mơ, nhớ lưu video lại đặt phòng nha các con vợ."},
		}}, nil
	}
	// synthTTS THẬT (không stub) — provider google.

	out := filepath.Join(os.TempDir(), "narrated_google_demo.mp4")
	cfg := Config{
		Mode: "narrated", Output: out,
		Width: 1080, Height: 1920, FPS: 30, ZoomIntensity: 0.4,
		TTSProvider: "google", NarrationPersona: "haihuoc", MaxSegments: 10,
	}
	args, err := buildNarrated(cfg, imgs)
	if err != nil {
		t.Fatalf("buildNarrated (google): %v", err)
	}
	if err := runFFmpeg(args, cfg); err != nil {
		t.Fatalf("runFFmpeg (google): %v", err)
	}
	if !hasStream(t, out, "audio") || !hasStream(t, out, "video") {
		t.Fatalf("output thiếu stream")
	}
	// Xác minh audio KHÔNG câm (là giọng thật): mean volume phải > -50 dB.
	mv := meanVolumeDB(t, out)
	if mv <= -50 {
		t.Fatalf("audio có vẻ câm (mean volume %.1f dB) — giọng thật phải to hơn", mv)
	}
	dur, _ := audioDurationSec(out)
	t.Logf("✅ VIDEO GIỌNG THẬT (Google, free): %s", out)
	t.Logf("   độ dài %.1fs · mean volume %.1f dB (không câm = có giọng thật)", dur, mv)
}

// meanVolumeDB đọc mean_volume của track audio bằng ffmpeg volumedetect.
func meanVolumeDB(t *testing.T, path string) float64 {
	t.Helper()
	out, _ := exec.Command("ffmpeg", "-i", path, "-af", "volumedetect", "-f", "null", "-").CombinedOutput()
	for _, line := range strings.Split(string(out), "\n") {
		if i := strings.Index(line, "mean_volume:"); i >= 0 {
			f := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(line[i+len("mean_volume:"):]), "dB"))
			var v float64
			fmt.Sscanf(f, "%f", &v)
			return v
		}
	}
	return -999
}

// ─── helpers ────────────────────────────────────────────────────────────────

func makeColorImage(t *testing.T, dir, name, color string, w, h int) string {
	t.Helper()
	p := filepath.Join(dir, "img"+name+".png")
	cmd := exec.Command("ffmpeg", "-y", "-f", "lavfi",
		"-i", fmt.Sprintf("color=c=%s:s=%dx%d", color, w, h),
		"-frames:v", "1", p)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("tạo ảnh %s: %v\n%s", name, err, out)
	}
	return p
}

func silentMP3(t *testing.T, dir string, sec float64) []byte {
	t.Helper()
	p := filepath.Join(dir, fmt.Sprintf("sil_%.0f.mp3", sec*1000))
	cmd := exec.Command("ffmpeg", "-y", "-f", "lavfi",
		"-i", "anullsrc=r=44100:cl=mono",
		"-t", fmt.Sprintf("%.3f", sec),
		"-c:a", "libmp3lame", "-q:a", "9", p)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("tạo mp3 câm: %v\n%s", err, out)
	}
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("đọc mp3 câm: %v", err)
	}
	return data
}

func hasStream(t *testing.T, path, kind string) bool {
	t.Helper()
	out, err := exec.Command("ffprobe", "-v", "error",
		"-select_streams", string(kind[0]), // 'a' hoặc 'v'
		"-show_entries", "stream=codec_type",
		"-of", "csv=p=0", path).Output()
	if err != nil {
		return false
	}
	return len(out) > 0
}
