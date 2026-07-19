package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// filterComplexArg trả về giá trị của cờ -filter_complex trong args ffmpeg.
func filterComplexArg(t *testing.T, args []string) string {
	t.Helper()
	for i, a := range args {
		if a == "-filter_complex" && i+1 < len(args) {
			return args[i+1]
		}
	}
	t.Fatal("không thấy -filter_complex trong args")
	return ""
}

// countInputs đếm số cờ -i (số input) trong args.
func countInputs(args []string) int {
	n := 0
	for _, a := range args {
		if a == "-i" {
			n++
		}
	}
	return n
}

// stubTTSForGraph gắn stub genScript + synthTTS cho test đồ thị (không cần API).
// synthTTS vẫn sinh mp3 câm thật (cần ffmpeg) để buildNarrated đo được duration.
func stubTTSForGraph(t *testing.T, script *NarrationScript) {
	t.Helper()
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("cần ffmpeg để sinh mp3 câm")
	}
	origScript, origTTS := genScript, synthTTS
	t.Cleanup(func() { genScript, synthTTS = origScript, origTTS })
	genScript = func(cfg Config, images []string) (*NarrationScript, error) { return script, nil }
	tmp := t.TempDir()
	synthTTS = func(cfg Config, text string) ([]byte, error) {
		return silentMP3(t, tmp, 1.0+float64(len([]rune(text)))/40.0), nil
	}
	os.Setenv("ELEVENLABS_API_KEY", "test-dummy")
	os.Setenv("ANTHROPIC_API_KEY", "test-dummy")
	t.Cleanup(func() { os.Unsetenv("ELEVENLABS_API_KEY"); os.Unsetenv("ANTHROPIC_API_KEY") })
}

// TestNarratedIntroGraph: kiểm tra ĐỒ THỊ ffmpeg khi chèn intro — không render,
// chỉ soi args. Chốt các bất biến dễ vỡ: node intro đúng, ảnh dồn input +1,
// amix đúng số, phụ đề cuối, và fallback khi thiếu intro_narration.
func TestNarratedIntroGraph(t *testing.T) {
	isolateScriptLib(t)
	baseScript := &NarrationScript{
		Segments: []NarrationSegment{
			{ImageIndex: 0, Caption: "Cửa", Narration: "Check-in tự động không gặp ai."},
			{ImageIndex: 1, Caption: "Giường", Narration: "Đệm dày sụ êm ru."},
			{ImageIndex: 2, Caption: "Chốt", Narration: "Book lẹ đi các vợ."},
		},
		HookLine1:      "Studio Mây Trắng",
		HookLine2:      "Chỉ 299k",
		IntroNarration: "Thôi dẹp nhà nghỉ tối om đi các vợ ơi.",
		IntroCaption:   "Đi đường",
	}

	makeImgs := func() []string {
		tmp := t.TempDir()
		return []string{
			makeColorImage(t, tmp, "0", "red", 1200, 800),
			makeColorImage(t, tmp, "1", "green", 800, 1200),
			makeColorImage(t, tmp, "2", "blue", 1200, 800),
		}
	}

	// Asset intro giả (chỉ cần os.Stat thành công — không render nên nội dung kệ).
	introFile := filepath.Join(t.TempDir(), "street.mp4")
	if err := os.WriteFile(introFile, []byte("dummy"), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Run("intro-on", func(t *testing.T) {
		stubTTSForGraph(t, baseScript)
		old := introAssetOverride
		introAssetOverride = introFile
		defer func() { introAssetOverride = old }()

		cfg := Config{
			Mode: "narrated", Narrate: true, IntroClip: true,
			Output: filepath.Join(t.TempDir(), "o.mp4"),
			Width:  1080, Height: 1920, FPS: 30, ZoomIntensity: 0.4,
			TTSProvider: "elevenlabs", NarrationPersona: "haihuoc", MaxSegments: 10,
		}
		args, err := buildNarrated(cfg, makeImgs())
		if err != nil {
			t.Fatalf("buildNarrated: %v", err)
		}
		fc := filterComplexArg(t, args)

		// Node intro = cảnh 0: input [0:v], có settb=1/30 + tpad clone + [zp0].
		if !strings.Contains(fc, "[0:v]") || !strings.Contains(fc, "settb=1/30") ||
			!strings.Contains(fc, "tpad=stop_mode=clone") || !strings.Contains(fc, "[zp0]") {
			t.Errorf("thiếu node intro chuẩn (settb/tpad/zp0):\n%s", firstN(fc, 400))
		}
		// KHÔNG được zoompan trên input 0 (đó là video, không phải ảnh tĩnh).
		if strings.Contains(fc, "[0:v]scale") && strings.Contains(fc, "[0:v]scale=1080:1920:force_original_aspect_ratio=increase,crop=1080:1920,zoompan") {
			t.Error("input 0 (intro) không được zoompan")
		}
		// Ảnh dồn về input 1..3, có zoompan.
		for _, lbl := range []string{"[1:v]", "[2:v]", "[3:v]"} {
			if !strings.Contains(fc, lbl) {
				t.Errorf("thiếu node ảnh %s (ảnh phải dồn sau intro)", lbl)
			}
		}
		if !strings.Contains(fc, "zoompan") {
			t.Error("thiếu zoompan cho các cảnh ảnh")
		}
		// 4 cảnh (intro + 3 ảnh) → voice amix 5 input (abase + 4 giọng).
		if !strings.Contains(fc, "amix=inputs=5:duration=first:normalize=0[voice]") {
			t.Errorf("voice amix phải là 5 input (abase+4 giọng):\n%s", firstN(fc, 600))
		}
		// Phụ đề burn cuối cùng.
		if !strings.Contains(fc, "subtitles=") {
			t.Error("thiếu filter subtitles")
		}
		// xfade nối 4 cảnh.
		if !strings.Contains(fc, "xfade=transition=fade") {
			t.Error("thiếu xfade nối cảnh")
		}
	})

	t.Run("intro-off-no-narration", func(t *testing.T) {
		// Kịch bản KHÔNG có intro_narration → dù bật cờ, phải fallback không intro.
		noIntro := *baseScript
		noIntro.IntroNarration = ""
		stubTTSForGraph(t, &noIntro)
		old := introAssetOverride
		introAssetOverride = introFile
		defer func() { introAssetOverride = old }()

		cfg := Config{
			Mode: "narrated", Narrate: true, IntroClip: true,
			Output: filepath.Join(t.TempDir(), "o.mp4"),
			Width:  1080, Height: 1920, FPS: 30, ZoomIntensity: 0.4,
			TTSProvider: "elevenlabs", NarrationPersona: "haihuoc", MaxSegments: 10,
		}
		args, err := buildNarrated(cfg, makeImgs())
		if err != nil {
			t.Fatalf("buildNarrated: %v", err)
		}
		fc := filterComplexArg(t, args)
		// Không có node intro → không tpad clone; input 0 là ảnh (zoompan).
		if strings.Contains(fc, "tpad=stop_mode=clone") {
			t.Error("thiếu intro_narration nhưng vẫn dựng node intro (tpad clone)")
		}
		// 3 cảnh → voice amix 4 input.
		if !strings.Contains(fc, "amix=inputs=4:duration=first:normalize=0[voice]") {
			t.Errorf("fallback: voice amix phải 4 input:\n%s", firstN(fc, 600))
		}
	})

	t.Run("intro-input-count", func(t *testing.T) {
		stubTTSForGraph(t, baseScript)
		old := introAssetOverride
		introAssetOverride = introFile
		defer func() { introAssetOverride = old }()

		cfg := Config{
			Mode: "narrated", Narrate: true, IntroClip: true,
			Output: filepath.Join(t.TempDir(), "o.mp4"),
			Width:  1080, Height: 1920, FPS: 30, ZoomIntensity: 0.4,
			TTSProvider: "elevenlabs", NarrationPersona: "haihuoc", MaxSegments: 10,
		}
		args, err := buildNarrated(cfg, makeImgs())
		if err != nil {
			t.Fatalf("buildNarrated: %v", err)
		}
		// Input tối thiểu: 1 intro + 3 ảnh + 4 giọng = 8 (chưa kể watermark/sticker/khói).
		if got := countInputs(args); got < 8 {
			t.Errorf("số input = %d, cần ≥8 (intro+3 ảnh+4 giọng)", got)
		}
	})
}

// TestGenNarratedIntroVideo: e2e render THẬT có intro (gated GENNARRATED=1).
// Dùng clip lavfi làm cảnh đi đường; phủ cả case clip NGẮN hơn cảnh (tpad clone).
func TestGenNarratedIntroVideo(t *testing.T) {
	isolateScriptLib(t)
	if os.Getenv("GENNARRATED") == "" {
		t.Skip("đặt GENNARRATED=1 để chạy end-to-end narrated intro")
	}
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("không có ffmpeg")
	}
	os.Setenv("ELEVENLABS_API_KEY", "test-dummy")
	os.Setenv("ANTHROPIC_API_KEY", "test-dummy")
	defer func() { os.Unsetenv("ELEVENLABS_API_KEY"); os.Unsetenv("ANTHROPIC_API_KEY") }()

	origScript, origTTS := genScript, synthTTS
	defer func() { genScript, synthTTS = origScript, origTTS }()
	genScript = func(cfg Config, images []string) (*NarrationScript, error) {
		return &NarrationScript{
			Segments: []NarrationSegment{
				{ImageIndex: 0, Caption: "Cửa", Narration: "Check-in tự động không gặp ai đâu nha."},
				{ImageIndex: 1, Caption: "Giường", Narration: "Đệm dày sụ nằm êm ru luôn."},
			},
			HookLine1:      "Studio Mây Trắng",
			HookLine2:      "Chỉ 299k",
			IntroNarration: "Thôi dẹp nhà nghỉ tối om đi các vợ ơi, book căn này đi.",
			IntroCaption:   "Đi đường",
		}, nil
	}

	makeIntro := func(t *testing.T, dur string) string {
		p := filepath.Join(t.TempDir(), "street.mp4")
		out, err := exec.Command("ffmpeg", "-y", "-f", "lavfi",
			"-i", "testsrc2=size=1080x1920:rate=30", "-t", dur,
			"-c:v", "libx264", "-pix_fmt", "yuv420p", p).CombinedOutput()
		if err != nil {
			t.Fatalf("tạo clip intro: %v\n%s", err, out)
		}
		return p
	}

	run := func(t *testing.T, introDur string) {
		tmp := t.TempDir()
		synthTTS = func(cfg Config, text string) ([]byte, error) {
			return silentMP3(t, tmp, 1.0+float64(len([]rune(text)))/40.0), nil
		}
		old := introAssetOverride
		introAssetOverride = makeIntro(t, introDur)
		defer func() { introAssetOverride = old }()

		imgs := []string{
			makeColorImage(t, tmp, "0", "red", 1200, 800),
			makeColorImage(t, tmp, "1", "green", 800, 1200),
		}
		out := filepath.Join(tmp, "out.mp4")
		cfg := Config{
			Mode: "narrated", Narrate: true, IntroClip: true, Output: out,
			Width: 1080, Height: 1920, FPS: 30, ZoomIntensity: 0.4,
			TTSProvider: "elevenlabs", NarrationPersona: "haihuoc", MaxSegments: 10,
		}
		args, err := buildNarrated(cfg, imgs)
		if err != nil {
			t.Fatalf("buildNarrated: %v", err)
		}
		if err := runFFmpeg(args, cfg); err != nil {
			t.Fatalf("runFFmpeg (intro %s): %v", introDur, err)
		}
		if !hasStream(t, out, "audio") || !hasStream(t, out, "video") {
			t.Fatalf("output thiếu stream (intro %s)", introDur)
		}
		dur, _ := audioDurationSec(out)
		if dur < 4 || dur > 40 {
			t.Fatalf("duration bất thường (intro %s): %.2f", introDur, dur)
		}
		t.Logf("✅ intro %s OK: %.1fs", introDur, dur)
	}

	t.Run("clip-dài", func(t *testing.T) { run(t, "8") })  // clip dài hơn cảnh
	t.Run("clip-ngắn", func(t *testing.T) { run(t, "2") }) // clip ngắn → tpad clone
}
