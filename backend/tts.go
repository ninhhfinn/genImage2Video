package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// ─── Text-to-Speech: 2 nhà cung cấp, chọn trong UI ──────────────────────────
//
//	ElevenLabs — giọng tự nhiên, biểu cảm (mặc định).
//	FPT.AI     — thuần Việt, rẻ, đọc số chuẩn.
//
// Mỗi đoạn lời kể → 1 file mp3, cache theo hash(provider+model+voice+text).

const (
	elevenModelID = "eleven_multilingual_v2"
	// Giọng tiếng Việt mặc định theo persona (giọng thư viện ElevenLabs — cần
	// gói trả phí để dùng qua API; gói free sẽ bị 402, xem thông báo lỗi bên dưới).
	elevenVoiceHaihuoc = "a3AkyqGG4v8Pg7SWQ0Y3" // Ngan — dễ thương, tươi tắn
	elevenVoiceLichsu  = "UsgbMVmY3U59ijwK5mdh" // Trieu Duong — trầm, bình tĩnh
	fptDefaultVoice    = "banmai"
	ttsHTTPTimeout     = 60 * time.Second
	ttsMaxAttempts     = 3
)

// synthTTS là test seam — test gán stub sinh mp3 câm để chạy không cần key.
var synthTTS = ttsSynthesize

// ttsSynthesize tạo mp3 cho 1 đoạn text theo provider đã chọn.
func ttsSynthesize(cfg Config, text string) ([]byte, error) {
	switch strings.ToLower(strings.TrimSpace(cfg.TTSProvider)) {
	case "google", "free":
		return googleTTS(text)
	case "fpt":
		return fptTTS(text, resolveVoiceID(cfg))
	default: // elevenlabs
		return elevenLabsTTS(text, resolveVoiceID(cfg))
	}
}

// resolveVoiceID: cfg.VoiceID → env → default theo provider.
func resolveVoiceID(cfg Config) string {
	if p := strings.ToLower(strings.TrimSpace(cfg.TTSProvider)); p == "google" || p == "free" {
		return "vi" // Google TTS không dùng voice id
	}
	if v := strings.TrimSpace(cfg.VoiceID); v != "" {
		return v
	}
	if strings.EqualFold(strings.TrimSpace(cfg.TTSProvider), "fpt") {
		if v := strings.TrimSpace(os.Getenv("FPT_TTS_VOICE")); v != "" {
			return v
		}
		return fptDefaultVoice
	}
	if v := strings.TrimSpace(os.Getenv("ELEVENLABS_VOICE_ID")); v != "" {
		return v
	}
	// Mặc định theo persona: giọng Việt tươi tắn (hài) / trầm ấm (lịch sự).
	if strings.EqualFold(strings.TrimSpace(cfg.NarrationPersona), "lichsu") {
		return elevenVoiceLichsu
	}
	return elevenVoiceHaihuoc
}

// ─── ElevenLabs ─────────────────────────────────────────────────────────────

func elevenLabsTTS(text, voiceID string) ([]byte, error) {
	key := strings.TrimSpace(os.Getenv("ELEVENLABS_API_KEY"))
	if key == "" {
		return nil, fmt.Errorf("thiếu ELEVENLABS_API_KEY")
	}
	endpoint := fmt.Sprintf("https://api.elevenlabs.io/v1/text-to-speech/%s?output_format=mp3_44100_128", url.PathEscape(voiceID))
	body, _ := json.Marshal(map[string]any{
		"text":     text,
		"model_id": elevenModelID,
	})

	var lastErr error
	for attempt := 0; attempt < ttsMaxAttempts; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt) * time.Second)
		}
		req, err := http.NewRequest("POST", endpoint, bytes.NewReader(body))
		if err != nil {
			return nil, fmt.Errorf("voice id không hợp lệ: %v", err)
		}
		req.Header.Set("xi-api-key", key)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "audio/mpeg")

		resp, err := (&http.Client{Timeout: ttsHTTPTimeout}).Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		data, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode == 200 && len(data) > 0 {
			return data, nil
		}
		// Gói free ElevenLabs không dùng được giọng thư viện qua API → báo rõ ràng.
		if resp.StatusCode == 402 || strings.Contains(string(data), "paid_plan_required") {
			return nil, fmt.Errorf("Gói ElevenLabs free không dùng được giọng thư viện qua API. Cách xử lý: (1) nâng cấp ElevenLabs (Starter ~$5/tháng), HOẶC (2) tự clone 1 giọng riêng rồi điền Voice ID của giọng đó, HOẶC (3) đổi sang FPT.AI trong phần cài đặt")
		}
		lastErr = fmt.Errorf("ElevenLabs HTTP %d: %s", resp.StatusCode, firstN(string(data), 200))
		if resp.StatusCode >= 400 && resp.StatusCode < 500 && resp.StatusCode != 429 {
			break // lỗi client (sai key/voice) — không retry
		}
	}
	return nil, lastErr
}

// ─── FPT.AI ─────────────────────────────────────────────────────────────────

func fptTTS(text, voice string) ([]byte, error) {
	key := strings.TrimSpace(os.Getenv("FPT_TTS_API_KEY"))
	if key == "" {
		return nil, fmt.Errorf("thiếu FPT_TTS_API_KEY")
	}
	var asyncURL string
	var lastErr error
	for attempt := 0; attempt < ttsMaxAttempts; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt) * time.Second)
		}
		req, err := http.NewRequest("POST", "https://api.fpt.ai/hmi/tts/v5", strings.NewReader(text))
		if err != nil {
			return nil, fmt.Errorf("tạo request FPT.AI: %v", err)
		}
		req.Header.Set("api-key", key)
		req.Header.Set("voice", voice)
		req.Header.Set("speed", "")
		resp, err := (&http.Client{Timeout: ttsHTTPTimeout}).Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		data, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != 200 {
			lastErr = fmt.Errorf("FPT.AI HTTP %d: %s", resp.StatusCode, firstN(string(data), 200))
			if resp.StatusCode >= 400 && resp.StatusCode < 500 && resp.StatusCode != 429 {
				break
			}
			continue
		}
		var out struct {
			Async   string `json:"async"`
			Error   int    `json:"error"`
			Message string `json:"message"`
		}
		if err := json.Unmarshal(data, &out); err != nil || out.Async == "" {
			lastErr = fmt.Errorf("FPT.AI phản hồi lạ: %s", firstN(string(data), 200))
			continue
		}
		asyncURL = out.Async
		break
	}
	if asyncURL == "" {
		return nil, lastErr
	}
	// Poll URL async cho tới khi có mp3 (FPT sinh mp3 sau vài giây).
	deadline := time.Now().Add(ttsHTTPTimeout)
	for time.Now().Before(deadline) {
		resp, err := (&http.Client{Timeout: 15 * time.Second}).Get(asyncURL)
		if err == nil {
			data, _ := io.ReadAll(resp.Body)
			ct := resp.Header.Get("Content-Type")
			resp.Body.Close()
			if resp.StatusCode == 200 && len(data) > 0 && (strings.Contains(ct, "audio") || strings.Contains(ct, "octet-stream") || len(data) > 2000) {
				return data, nil
			}
		}
		time.Sleep(1500 * time.Millisecond)
	}
	return nil, fmt.Errorf("FPT.AI: hết thời gian chờ file mp3")
}

// ─── Google Translate TTS (miễn phí, không cần key) ─────────────────────────
//
// Endpoint keyless của Google Translate; giới hạn ~200 ký tự/lần nên chia nhỏ
// theo từ rồi nối các mp3 lại. Chất lượng máy đọc (robot) nhưng đọc tiếng Việt
// rõ, không cần đăng ký — hợp để thử nhanh.

func googleTTS(text string) ([]byte, error) {
	chunks := splitForGoogleTTS(text, 180)
	client := &http.Client{Timeout: ttsHTTPTimeout}
	var buf bytes.Buffer
	for _, ch := range chunks {
		endpoint := "https://translate.google.com/translate_tts?ie=UTF-8&client=tw-ob&tl=vi&q=" + url.QueryEscape(ch)
		var got bool
		var lastErr error
		for attempt := 0; attempt < ttsMaxAttempts; attempt++ {
			if attempt > 0 {
				time.Sleep(time.Duration(attempt) * time.Second)
			}
			req, err := http.NewRequest("GET", endpoint, nil)
			if err != nil {
				return nil, fmt.Errorf("Google TTS request: %v", err)
			}
			req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64)")
			resp, err := client.Do(req)
			if err != nil {
				lastErr = err
				continue
			}
			data, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			if resp.StatusCode == 200 && len(data) > 500 {
				buf.Write(data)
				got = true
				break
			}
			lastErr = fmt.Errorf("Google TTS HTTP %d", resp.StatusCode)
		}
		if !got {
			return nil, fmt.Errorf("Google TTS lỗi (có thể bị giới hạn tần suất): %v", lastErr)
		}
	}
	if buf.Len() == 0 {
		return nil, fmt.Errorf("Google TTS không trả về audio")
	}
	return buf.Bytes(), nil
}

// splitForGoogleTTS chia text thành các mảnh ≤ maxRunes ký tự, cắt theo từ.
func splitForGoogleTTS(text string, maxRunes int) []string {
	words := strings.Fields(text)
	var chunks []string
	var cur strings.Builder
	curRunes := 0
	for _, w := range words {
		wr := len([]rune(w))
		if curRunes > 0 && curRunes+wr+1 > maxRunes {
			chunks = append(chunks, cur.String())
			cur.Reset()
			curRunes = 0
		}
		if curRunes > 0 {
			cur.WriteByte(' ')
			curRunes++
		}
		cur.WriteString(w)
		curRunes += wr
	}
	if cur.Len() > 0 {
		chunks = append(chunks, cur.String())
	}
	if len(chunks) == 0 {
		chunks = []string{text}
	}
	return chunks
}

// ─── Synth toàn bộ segments ─────────────────────────────────────────────────

// synthSegments đọc từng đoạn thành mp3 (cache), trả về đường dẫn + độ dài (giây)
// song song với script.Segments.
func synthSegments(cfg Config, script *NarrationScript) ([]string, []float64, error) {
	n := len(script.Segments)
	paths := make([]string, n)
	durs := make([]float64, n)
	provider := strings.ToLower(strings.TrimSpace(cfg.TTSProvider))
	if provider == "" {
		provider = "elevenlabs"
	}
	voice := resolveVoiceID(cfg)

	for i, seg := range script.Segments {
		reportProgress(fmt.Sprintf("🎙️ Giọng đọc cảnh %d/%d…", i+1, n))
		key := ttsCacheKey(provider, voice, seg.Narration)
		path := filepathJoinTemp("img2video_tts_" + key + ".mp3")

		if st, err := os.Stat(path); err != nil || st.Size() == 0 {
			data, err := synthTTS(cfg, seg.Narration)
			if err != nil {
				return nil, nil, fmt.Errorf("TTS lỗi ở cảnh %d: %v", i+1, err)
			}
			if err := os.WriteFile(path, data, 0644); err != nil {
				return nil, nil, fmt.Errorf("ghi mp3 cảnh %d: %v", i+1, err)
			}
		}

		d, err := audioDurationSec(path)
		if err != nil || d <= 0 {
			return nil, nil, fmt.Errorf("đo độ dài mp3 cảnh %d: %v", i+1, err)
		}
		paths[i] = path
		durs[i] = d
	}
	return paths, durs, nil
}

// audioDurationSec đọc độ dài file audio bằng ffprobe (giây).
func audioDurationSec(path string) (float64, error) {
	if _, err := exec.LookPath("ffprobe"); err != nil {
		return 0, fmt.Errorf("không thấy ffprobe")
	}
	out, err := exec.Command("ffprobe",
		"-v", "error",
		"-show_entries", "format=duration",
		"-of", "default=nokey=1:noprint_wrappers=1",
		path,
	).Output()
	if err != nil {
		return 0, err
	}
	return strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
}

func ttsCacheKey(provider, voice, text string) string {
	h := sha256.New()
	h.Write([]byte("tts-v1|" + provider + "|" + elevenModelID + "|" + voice + "|" + text))
	return fmt.Sprintf("%x", h.Sum(nil))[:16]
}
