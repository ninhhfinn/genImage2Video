package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
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
	// turbo_v2_5 chứ KHÔNG phải multilingual_v2: multilingual_v2 không hỗ trợ
	// tiếng Việt (29 thứ tiếng, thiếu vi) — đọc text Việt ra tiếng lạ.
	elevenModelID = "eleven_turbo_v2_5"
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
// Provider rỗng → Google free (không cần key), tránh chết render vì thiếu key.
func ttsSynthesize(cfg Config, text string) ([]byte, error) {
	switch strings.ToLower(strings.TrimSpace(cfg.TTSProvider)) {
	case "", "google", "free":
		return googleTTS(text)
	case "fpt":
		return fptTTS(text, resolveVoiceID(cfg))
	default: // elevenlabs
		return elevenLabsTTS(text, resolveVoiceID(cfg))
	}
}

// resolveVoiceID: cfg.VoiceID → env → default theo provider.
func resolveVoiceID(cfg Config) string {
	if p := strings.ToLower(strings.TrimSpace(cfg.TTSProvider)); p == "" || p == "google" || p == "free" {
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

// errElevenUnusable đánh dấu lỗi "ElevenLabs không dùng được" (thiếu key, sai
// key, hết quota, gói free) — synthSegments bắt lỗi này để tự chuyển sang
// giọng Google free. Lỗi tạm (5xx/mạng) KHÔNG wrap để không trộn giọng vô cớ.
var errElevenUnusable = errors.New("ElevenLabs không dùng được")

func elevenLabsTTS(text, voiceID string) ([]byte, error) {
	key := strings.TrimSpace(os.Getenv("ELEVENLABS_API_KEY"))
	if key == "" {
		return nil, fmt.Errorf("thiếu ELEVENLABS_API_KEY: %w", errElevenUnusable)
	}
	endpoint := fmt.Sprintf("https://api.elevenlabs.io/v1/text-to-speech/%s?output_format=mp3_44100_128", url.PathEscape(voiceID))
	body, _ := json.Marshal(map[string]any{
		"text":     text,
		"model_id": elevenModelID,
		// Ép tiếng Việt: giọng Anh + câu ngắn dễ bị đoán nhầm ngôn ngữ.
		"language_code": "vi",
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
			return nil, fmt.Errorf("Gói ElevenLabs free không dùng được giọng thư viện qua API (nâng cấp Starter ~$5/tháng hoặc clone giọng riêng rồi điền Voice ID): %w", errElevenUnusable)
		}
		lastErr = fmt.Errorf("ElevenLabs HTTP %d: %s", resp.StatusCode, firstN(string(data), 200))
		if resp.StatusCode == 401 || resp.StatusCode == 403 {
			return nil, fmt.Errorf("%v: %w", lastErr, errElevenUnusable) // key sai/hết hạn
		}
		if resp.StatusCode >= 400 && resp.StatusCode < 500 && resp.StatusCode != 429 {
			break // lỗi client khác (vd sai voice) — không retry
		}
	}
	if lastErr != nil && strings.Contains(lastErr.Error(), "HTTP 429") {
		return nil, fmt.Errorf("%v: %w", lastErr, errElevenUnusable) // hết quota sau khi retry
	}
	return nil, lastErr
}

// ─── FPT.AI ─────────────────────────────────────────────────────────────────

// fptRateWaits: gói FPT free giới hạn tần suất theo phút → khi dính HTTP 429
// chờ lâu dần rồi thử lại (không tính vào ttsMaxAttempts). Tổng chờ tối đa ~2.5'.
var fptRateWaits = []time.Duration{15 * time.Second, 30 * time.Second, 45 * time.Second, 60 * time.Second}

// fptTTS: 2 nền tảng FPT, tự nhận theo dạng key:
//   - key "sk-..." → FPT AI Marketplace (mkp-api.fptcloud.com, OpenAI-compatible,
//     model FPT.AI-VITs, giọng std_kimngan..., $16.5/1M ký tự, trả thẳng audio)
//   - key khác     → FPT.AI cũ (api.fpt.ai/hmi/tts/v5, giọng banmai..., async URL)
func fptTTS(text, voice string) ([]byte, error) {
	key := strings.TrimSpace(os.Getenv("FPT_TTS_API_KEY"))
	if key == "" {
		return nil, fmt.Errorf("thiếu FPT_TTS_API_KEY")
	}
	if strings.HasPrefix(key, "sk-") {
		return fptMarketplaceTTS(key, text, voice)
	}
	var asyncURL string
	var lastErr error
	rl := 0 // số lần đã chờ vì rate limit
	for attempt := 0; attempt < ttsMaxAttempts; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt) * time.Second)
		}
		req, err := http.NewRequest("POST", "https://api.fpt.ai/hmi/tts/v5", strings.NewReader(text))
		if err != nil {
			return nil, fmt.Errorf("tạo request FPT.AI: %v", err)
		}
		req.Header.Set("api-key", key)
		req.Header.Set("api_key", key) // docs FPT mới ghi api_key — gửi cả hai cho chắc
		req.Header.Set("voice", voice)
		req.Header.Set("speed", "")
		resp, err := (&http.Client{Timeout: ttsHTTPTimeout}).Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		data, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode == 429 && rl < len(fptRateWaits) {
			// Gói free bị giới hạn tần suất — chờ đủ lâu rồi thử lại.
			reportProgress(fmt.Sprintf("⏳ FPT free bị giới hạn tần suất — chờ %.0fs rồi đọc tiếp…", fptRateWaits[rl].Seconds()))
			time.Sleep(fptRateWaits[rl])
			rl++
			attempt-- // không tính lần chờ vào số attempt thường
			continue
		}
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
	// Poll URL async cho tới khi có mp3. Gói free "request is queued" nên có
	// thể chờ lâu hơn 60s → cho hẳn 3 phút.
	deadline := time.Now().Add(3 * time.Minute)
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

// fptMarketplaceTTS gọi FPT AI Marketplace (OpenAI-compatible /v1/audio/speech).
// Model FPT.AI-VITs nhận cả tên cũ (banmai...) lẫn std_* (std_kimngan...) —
// truyền thẳng; tên sai thì API trả lỗi kèm danh sách giọng hợp lệ.
func fptMarketplaceTTS(key, text, voice string) ([]byte, error) {
	if strings.TrimSpace(voice) == "" {
		voice = "std_banmai"
	}
	body, _ := json.Marshal(map[string]any{
		"model":           "FPT.AI-VITs",
		"input":           text,
		"response_format": "wav",
		"voice":           voice,
	})
	var lastErr error
	for attempt := 0; attempt < ttsMaxAttempts; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt) * time.Second)
		}
		req, err := http.NewRequest("POST", "https://mkp-api.fptcloud.com/v1/audio/speech", bytes.NewReader(body))
		if err != nil {
			return nil, fmt.Errorf("tạo request FPT Marketplace: %v", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+key)
		resp, err := (&http.Client{Timeout: ttsHTTPTimeout}).Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		data, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		// Thành công = audio nhị phân (không phải JSON lỗi) và đủ lớn.
		if resp.StatusCode == 200 && len(data) > 500 && !bytes.HasPrefix(bytes.TrimSpace(data), []byte("{")) {
			return data, nil
		}
		lastErr = fmt.Errorf("FPT Marketplace HTTP %d: %s", resp.StatusCode, firstN(string(data), 200))
		if resp.StatusCode >= 400 && resp.StatusCode < 500 && resp.StatusCode != 429 {
			break // sai key/giọng/hết credit — không retry
		}
	}
	return nil, lastErr
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
		provider = "google" // mặc định giọng free, không cần key
		cfg.TTSProvider = provider
	}
	// Chọn ElevenLabs nhưng chưa có key → khỏi gọi API, chuyển thẳng Google free.
	if provider != "google" && provider != "free" && provider != "fpt" &&
		strings.TrimSpace(os.Getenv("ELEVENLABS_API_KEY")) == "" {
		reportProgress("⚠️ Chưa có key ElevenLabs → dùng giọng Google free")
		provider = "google"
		cfg.TTSProvider = provider
	}
	voice := resolveVoiceID(cfg)

	for i, seg := range script.Segments {
		reportProgress(fmt.Sprintf("🎙️ Giọng đọc cảnh %d/%d…", i+1, n))
		key := ttsCacheKey(provider, voice, seg.Narration)
		path := filepathJoinTemp("img2video_tts_" + key + ".mp3")

		if st, err := os.Stat(path); err != nil || st.Size() == 0 {
			data, err := synthTTS(cfg, seg.Narration)
			if err != nil && errors.Is(err, errElevenUnusable) {
				// ElevenLabs chết (key sai/hết quota/gói free) → chuyển Google free
				// cho cảnh này + các cảnh sau. Tính lại cache key để audio Google
				// không bị lưu nhầm dưới key elevenlabs.
				fmt.Fprintf(os.Stderr, "⚠️  ElevenLabs lỗi ở cảnh %d, chuyển giọng Google free: %v\n", i+1, err)
				reportProgress("⚠️ ElevenLabs không dùng được → chuyển giọng Google free")
				provider = "google"
				cfg.TTSProvider = provider
				voice = resolveVoiceID(cfg)
				key = ttsCacheKey(provider, voice, seg.Narration)
				path = filepathJoinTemp("img2video_tts_" + key + ".mp3")
				if st, statErr := os.Stat(path); statErr == nil && st.Size() > 0 {
					data, err = nil, nil // đã có sẵn trong cache Google
				} else {
					data, err = synthTTS(cfg, seg.Narration)
				}
			}
			if err != nil {
				return nil, nil, fmt.Errorf("TTS lỗi ở cảnh %d: %v", i+1, err)
			}
			if len(data) > 0 {
				if err := os.WriteFile(path, data, 0644); err != nil {
					return nil, nil, fmt.Errorf("ghi mp3 cảnh %d: %v", i+1, err)
				}
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

// ─── Nghe thử giọng (UI chọn giọng cho mode narrated) ───────────────────────

const ttsPreviewText = "Chào các con vợ nhé, hôm nay tôi sẽ giới thiệu căn hộ xinh xỉu này."

// ttsPreviewHandler: GET /api/tts-preview?provider=fpt&voice=std_banmai
// → trả audio mẫu (cache theo provider+voice để không tốn tiền gọi lại).
func ttsPreviewHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		provider := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("provider")))
		cfg := Config{TTSProvider: provider, VoiceID: strings.TrimSpace(r.URL.Query().Get("voice"))}
		voice := resolveVoiceID(cfg)

		key := ttsCacheKey(provider, voice, "preview|"+ttsPreviewText)
		path := filepathJoinTemp("img2video_ttsprev_" + key + ".bin")
		data, err := os.ReadFile(path)
		if err != nil || len(data) == 0 {
			data, err = synthTTS(cfg, ttsPreviewText)
			if err != nil {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(502)
				json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
				return
			}
			_ = os.WriteFile(path, data, 0644)
		}

		if bytes.HasPrefix(data, []byte("RIFF")) {
			w.Header().Set("Content-Type", "audio/wav")
		} else {
			w.Header().Set("Content-Type", "audio/mpeg")
		}
		w.Header().Set("Cache-Control", "max-age=3600")
		w.Write(data)
	}
}
