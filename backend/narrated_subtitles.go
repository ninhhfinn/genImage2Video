package main

// Engine phụ đề ASS cho mode Thuyết minh AI — tái tạo style TikTok đã đo từ
// video mẫu (xem plan quiet-meandering-newt):
//   - karaoke: cụm 2–6 từ hiện nguyên khối tại y≈83%, từ đang đọc phóng 1.5×
//     + màu accent theo template + glow; từ đọc xong quay về trắng.
//   - typewriter: gõ từng ký tự theo câu (kiểu lovebox), Montserrat trắng.
//   - hook title 2 dòng typewriter hiện hết cảnh 1 rồi biến mất.
//   - tim bay vector {\p1} quanh sticker (không cần asset).
// Toàn bộ chữ động nằm trong MỘT file .ass render bằng libass
// (filter `subtitles`), vì PNG-per-word cần hàng trăm input và không layout
// được dòng có 1 từ phóng to.
//
// Timing từng từ KHÔNG có thật (TTS không trả timestamp) — ước lượng bằng cách
// chia thời lượng audio đo được của từng đoạn cho các từ theo trọng số
// rune+2 (tiếng Việt đơn âm tiết nên rất sát), trừ khoảng lặng đầu/cuối đo
// bằng silencedetect.

import (
	"fmt"
	"io"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

// ── Accent theo template ──

type accentColor struct {
	Highlight string // màu từ đang đọc / cụm nhấn hook
	Glow      string // màu viền glow
}

// templateAccent — màu chủ đạo từng template video (lấy từ palette trong
// chrome_overlay.go / video_templates.go / assets/templates/*.json).
var templateAccent = map[string]accentColor{
	"amorex":     {"#FF87CD", "#FF4FAE"}, // hồng — đúng màu video mẫu ocean
	"editorial":  {"#FFA7C4", "#FF5FA2"},
	"chillgreen": {"#A8D94B", "#6FA22E"},
	"goldserif":  {"#FFE94A", "#FFC700"},
	"marquee":    {"#FFBF1F", "#C2452D"},
	"creampill":  {"#FF6A3D", "#FF4004"},
	"staycation": {"#FFD9A0", "#E8912D"},
	"ntgroom":    {"#FFAF3B", "#E07B00"},
	"daiky":      {"#E8B98A", "#A8743F"},
	"sunset":     {"#FF9A5C", "#B3471F"},
}

func accentForTemplate(name string) accentColor {
	if ac, ok := templateAccent[strings.TrimSpace(strings.ToLower(name))]; ok {
		return ac
	}
	return accentColor{"#FF87CD", "#FF4FAE"}
}

// normalizeSubtitleStyle — mọi giá trị lạ đều về "karaoke".
func normalizeSubtitleStyle(s string) string {
	if strings.TrimSpace(strings.ToLower(s)) == "typewriter" {
		return "typewriter"
	}
	return "karaoke"
}

// ── Timing từng từ ──

type wordTiming struct {
	Word       string
	Start, End float64 // giây tuyệt đối trong video
}

// distributeWordTimings chia (voiceDur − lead − tail) cho các từ của narration,
// bắt đầu tại voiceAt+lead. Trọng số = số rune + 2. Mỗi từ ≥0.12s (clamp rồi
// renormalize 1 lần để tổng khớp). End từ i nối liền Start từ i+1.
func distributeWordTimings(narration string, voiceAt, voiceDur, lead, tail float64) []wordTiming {
	words := strings.Fields(narration)
	if len(words) == 0 {
		return nil
	}
	usable := voiceDur - lead - tail
	if usable < 0.2 {
		lead = 0
		usable = voiceDur
		if usable <= 0 {
			usable = 0.2 * float64(len(words))
		}
	}
	weights := make([]float64, len(words))
	sumW := 0.0
	for i, w := range words {
		weights[i] = float64(utf8.RuneCountInString(w)) + 2
		sumW += weights[i]
	}
	durs := make([]float64, len(words))
	sumD := 0.0
	for i := range words {
		d := usable * weights[i] / sumW
		if d < 0.12 {
			d = 0.12
		}
		durs[i] = d
		sumD += d
	}
	scale := usable / sumD
	out := make([]wordTiming, len(words))
	t := voiceAt + lead
	for i, w := range words {
		d := durs[i] * scale
		out[i] = wordTiming{Word: w, Start: t, End: t + d}
		t += d
	}
	return out
}

// segmentSpeechBounds ước lượng khoảng lặng đầu/cuối của 1 file TTS bằng
// ffmpeg silencedetect (khử padding Google TTS nối chunk). Lỗi bất kỳ →
// fallback (0.10, 0.15).
func segmentSpeechBounds(path string, dur float64) (lead, tail float64) {
	lead, tail = 0.10, 0.15
	if path == "" || dur <= 0 {
		return
	}
	if _, err := os.Stat(path); err != nil {
		return
	}
	cmd := exec.Command("ffmpeg", "-hide_banner", "-i", path,
		"-af", "silencedetect=n=-35dB:d=0.25", "-f", "null", "-")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return
	}
	reStart := regexp.MustCompile(`silence_start:\s*([\d.]+)`)
	reEnd := regexp.MustCompile(`silence_end:\s*([\d.]+)`)
	var starts, ends []float64
	for _, ln := range strings.Split(string(out), "\n") {
		if m := reStart.FindStringSubmatch(ln); m != nil {
			if v, e := strconv.ParseFloat(m[1], 64); e == nil {
				starts = append(starts, v)
			}
		}
		if m := reEnd.FindStringSubmatch(ln); m != nil {
			if v, e := strconv.ParseFloat(m[1], 64); e == nil {
				ends = append(ends, v)
			}
		}
	}
	// Khoảng lặng mở đầu: silence_start ≈ 0 → lead = silence_end đầu tiên.
	if len(starts) > 0 && starts[0] <= 0.05 && len(ends) > 0 {
		if l := ends[0]; l > lead && l < dur/2 {
			lead = l
		}
	}
	// Khoảng lặng cuối: silence_start cuối gần hết file (không cần end).
	if len(starts) > 0 {
		last := starts[len(starts)-1]
		if last > dur/2 {
			if tl := dur - last; tl > tail && tl < dur/2 {
				tail = tl
			}
		}
	}
	return
}

// ── Chunk 2–6 từ ──

type subChunk struct{ Words []wordTiming }

// chunkWords cắt tại dấu câu trước, rồi cap maxWords/maxRunes; chunk mồ côi
// 1 từ được gộp về trước (hoặc mượn 1 từ của chunk trước nếu trước đã đầy).
func chunkWords(ws []wordTiming, maxWords, maxRunes int) []subChunk {
	if len(ws) == 0 {
		return nil
	}
	endsWithPunct := func(w string) bool {
		trimmed := strings.TrimRight(w, `"'”’)»`)
		if trimmed == "" {
			return false
		}
		r, _ := utf8.DecodeLastRuneInString(trimmed)
		return strings.ContainsRune(",.!?…;:", r)
	}
	var chunks []subChunk
	var cur []wordTiming
	runes := 0
	flush := func() {
		if len(cur) > 0 {
			chunks = append(chunks, subChunk{Words: cur})
			cur = nil
			runes = 0
		}
	}
	for _, w := range ws {
		wr := utf8.RuneCountInString(w.Word) + 1
		if len(cur) > 0 && (len(cur) >= maxWords || runes+wr > maxRunes) {
			flush()
		}
		cur = append(cur, w)
		runes += wr
		if endsWithPunct(w.Word) {
			flush()
		}
	}
	flush()
	// Gộp chunk mồ côi 1 từ.
	for i := 1; i < len(chunks); i++ {
		if len(chunks[i].Words) != 1 {
			continue
		}
		prev := &chunks[i-1]
		if len(prev.Words) < maxWords {
			prev.Words = append(prev.Words, chunks[i].Words...)
			chunks = append(chunks[:i], chunks[i+1:]...)
			i--
		} else {
			// Mượn từ cuối của chunk trước.
			n := len(prev.Words)
			chunks[i].Words = append([]wordTiming{prev.Words[n-1]}, chunks[i].Words...)
			prev.Words = prev.Words[:n-1]
		}
	}
	return chunks
}

// ── Helpers ASS ──

// assColorStyle: "#RRGGBB" → "&H00BBGGRR" (định dạng cột màu trong [V4+ Styles]).
func assColorStyle(hex string) string {
	r, g, b := parseHexRGB(hex)
	return fmt.Sprintf("&H00%02X%02X%02X", b, g, r)
}

// assColorTag: "#RRGGBB" → "&HBBGGRR&" (dùng trong override tag \c \3c).
func assColorTag(hex string) string {
	r, g, b := parseHexRGB(hex)
	return fmt.Sprintf("&H%02X%02X%02X&", b, g, r)
}

func parseHexRGB(hex string) (r, g, b uint8) {
	h := strings.TrimPrefix(strings.TrimSpace(hex), "#")
	if len(h) != 6 {
		return 255, 255, 255
	}
	v, err := strconv.ParseUint(h, 16, 32)
	if err != nil {
		return 255, 255, 255
	}
	return uint8(v >> 16), uint8(v >> 8), uint8(v)
}

// assTime: giây → "H:MM:SS.CC".
func assTime(sec float64) string {
	if sec < 0 {
		sec = 0
	}
	cs := int(sec*100 + 0.5)
	h := cs / 360000
	cs -= h * 360000
	m := cs / 6000
	cs -= m * 6000
	s := cs / 100
	cs -= s * 100
	return fmt.Sprintf("%d:%02d:%02d.%02d", h, m, s, cs)
}

// assEscape: khử ký tự điều khiển ASS trong text người dùng (tên/lời kể từ
// dữ liệu Dayladau — KHÔNG tin cậy). '\' phải đổi trước tiên: một dấu '\' đứng
// trước '{' của tag do code sinh sẽ escape mất dấu '{' → chèn override/vỡ layout.
func assEscape(s string) string {
	s = strings.ReplaceAll(s, `\`, "/")
	s = strings.ReplaceAll(s, "{", "(")
	s = strings.ReplaceAll(s, "}", ")")
	s = strings.ReplaceAll(s, "\r", "")
	s = strings.ReplaceAll(s, "\n", `\N`)
	return s
}

// escapeFilterPath chuẩn bị path cho giá trị option trong -filter_complex
// (bọc ngoài bằng nháy đơn ở subtitlesFilterArg).
func escapeFilterPath(p string) string {
	p = filepath.ToSlash(p)
	p = strings.ReplaceAll(p, `\`, `/`)
	p = strings.ReplaceAll(p, `:`, `\:`)
	// Giá trị nằm trong nháy đơn ('...') ở subtitlesFilterArg; nháy đơn phải
	// thoát kiểu '\'' (đóng quote, escaped-quote, mở lại) mới sống qua lớp bọc.
	p = strings.ReplaceAll(p, `'`, `'\''`)
	return p
}

func subtitlesFilterArg(assPath, fontsDir string) string {
	return fmt.Sprintf("subtitles=filename='%s':fontsdir='%s'",
		escapeFilterPath(assPath), escapeFilterPath(fontsDir))
}

// stickerAssetPath trả đường dẫn assets/stickers/<name> nếu tồn tại, "" nếu
// không — thiếu asset thì render tự bỏ qua trang trí đó, KHÔNG fail.
func stickerAssetPath(name string) string {
	p := filepath.Join(assetsDir(), "stickers", name)
	if _, err := os.Stat(p); err != nil {
		return ""
	}
	return p
}

// ── Hook title fallback (khi script không có hook AI viết) ──

// Giá trong dữ liệu tới từ nhiều format tuỳ template/nguồn: "199.000 đ" (canonical
// price_line_items), "449,000" (goldserif), "189 cá" (amorex/editorial), "199k"
// (ntgroom), "21h - 9h (Qua đêm): 359" (chillgreen — số trần cuối dòng, đơn vị nghìn).
var (
	reGroupedAmt = regexp.MustCompile(`\d{1,3}(?:[.,]\d{3})+`) // 199.000 / 449,000
	reKUnit      = regexp.MustCompile(`(\d+)\s*[kK]\b`)        // 199k
	// "cá" (đơn vị giá template amorex) phải là RANH GIỚI TỪ: theo sau bởi
	// ký-tự-không-phải-chữ hoặc hết dòng — kẻo "2 các"/"2 cách" bị đọc nhầm là
	// giá "2 cá". (\b của Go regexp chỉ tính ASCII nên không dùng được với "á".)
	reCaUnit      = regexp.MustCompile(`(\d+)\s*cá(?:[^\p{L}]|$)`)
	reTrailingAmt = regexp.MustCompile(`:\s*(\d{2,4})\s*$`)             // "10h - 13h: 220"
	reHoursFirst  = regexp.MustCompile(`(\d+)\s*(?:giờ\s*đầu|h\s*đầu)`) // "3 giờ đầu" / "2h đầu"
)

// priceHitsInLine bắt mọi giá trị (đơn vị NGHÌN đồng, làm tròn) trong 1 dòng giá.
func priceHitsInLine(line string) []int {
	var out []int
	for _, m := range reGroupedAmt.FindAllString(line, -1) {
		if n, err := strconv.Atoi(strings.NewReplacer(".", "", ",", "").Replace(m)); err == nil && n > 0 {
			out = append(out, (n+500)/1000)
		}
	}
	for _, m := range reKUnit.FindAllStringSubmatch(line, -1) {
		if n, err := strconv.Atoi(m[1]); err == nil && n > 0 {
			out = append(out, n)
		}
	}
	for _, m := range reCaUnit.FindAllStringSubmatch(line, -1) {
		if n, err := strconv.Atoi(m[1]); err == nil && n > 0 {
			out = append(out, n)
		}
	}
	if m := reTrailingAmt.FindStringSubmatch(strings.TrimSpace(line)); m != nil {
		if n, err := strconv.Atoi(m[1]); err == nil && n > 0 {
			out = append(out, n)
		}
	}
	return out
}

// composeHookTitle ghép tiêu đề hook fallback từ dữ liệu listing (khi Claude
// không trả hook): dòng 1 = nickname/address, dòng 2 = "Chỉ <giá mồi>". Giá mồi
// = giá RẺ NHẤT trong các dòng (loại "thêm giờ" — dễ gây hiểu lầm); nếu là giá
// giờ đầu và biết số giờ thì kèm "/N tiếng", qua đêm thì "/đêm" — KHÔNG bịa
// đơn vị không có trong dữ liệu.
func composeHookTitle(cfg Config) (l1, l2, emph string) {
	l1 = strings.TrimSpace(cfg.Nickname)
	if l1 == "" {
		l1 = strings.TrimSpace(cfg.Address)
	}
	if l1 == "" {
		l1 = "Homestay xinh xỉu"
	}
	l1 = truncRunes(l1, 28)

	bestVal, bestHours := -1, 0
	bestNight := false
	for _, line := range cfg.Prices {
		low := strings.ToLower(line)
		if strings.Contains(low, "thêm giờ") {
			continue
		}
		for _, v := range priceHitsInLine(line) {
			if v < 30 { // lọc số rác (không giá phòng nào < 30k)
				continue
			}
			if bestVal >= 0 && v >= bestVal {
				continue
			}
			bestVal = v
			bestHours = 0
			if m := reHoursFirst.FindStringSubmatch(low); m != nil {
				bestHours, _ = strconv.Atoi(m[1])
			}
			bestNight = strings.Contains(low, "qua đêm")
		}
	}
	if bestVal > 0 {
		tok := fmt.Sprintf("%dk", bestVal)
		switch {
		case bestHours > 0:
			tok += fmt.Sprintf("/%d tiếng", bestHours)
		case bestNight:
			tok += "/đêm"
		}
		l2 = truncRunes("Chỉ "+tok, 28)
		emph = tok
	}
	return l1, l2, emph
}

// hookPriceMismatch kiểm tra grounding: mọi token GIÁ tường minh trong hook
// (dạng "199k"/"189 cá"/"199.000") phải khớp (±1k làm tròn) một giá parse được
// từ prices. Số trần ("2 người", "3 tiếng") KHÔNG tính là giá. Trả về token
// lệch đầu tiên nếu có.
func hookPriceMismatch(hook string, prices []string) (string, bool) {
	hook = strings.TrimSpace(hook)
	if hook == "" {
		return "", false
	}
	var have []int
	for _, l := range prices {
		have = append(have, priceHitsInLine(l)...)
	}
	check := func(tok string, valK int) bool {
		if valK < 30 { // <30k không phải giá phòng thật → không soi (tránh false-positive)
			return true
		}
		for _, h := range have {
			if valK-h <= 1 && h-valK <= 1 {
				return true
			}
		}
		return false
	}
	for _, m := range reGroupedAmt.FindAllString(hook, -1) {
		if n, err := strconv.Atoi(strings.NewReplacer(".", "", ",", "").Replace(m)); err == nil && n > 0 {
			if !check(m, (n+500)/1000) {
				return m, true
			}
		}
	}
	for _, re := range []*regexp.Regexp{reKUnit, reCaUnit} {
		for _, m := range re.FindAllStringSubmatch(hook, -1) {
			if n, err := strconv.Atoi(m[1]); err == nil && n > 0 {
				if !check(m[0], n) {
					return m[0], true
				}
			}
		}
	}
	return "", false
}

func truncRunes(s string, max int) string {
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	words := strings.Fields(s)
	out := ""
	for _, w := range words {
		next := strings.TrimSpace(out + " " + w)
		if utf8.RuneCountInString(next) > max {
			break
		}
		out = next
	}
	if out == "" {
		out = string([]rune(s)[:max])
	}
	return out
}

// ── Sinh file ASS ──

type narratedSubtitleSpec struct {
	Style      string // đã normalize: "karaoke" | "typewriter"
	Script     *NarrationScript
	Timeline   narratedTimeline
	VoiceDur   []float64
	VoicePaths []string
	Accent     accentColor
	HookEnd    float64
	HookLine1  string
	HookLine2  string
	HookEmph   string
	Amenities  []string // nuôi dòng giữa card serif (typewriter)
	Hearts     bool
	Seed       int64
}

// Vị trí đo từ video mẫu (trên 1080×1920).
const (
	karaokeY   = 1600 // 83.3% chiều cao
	hookLine1Y = 455
	hookLine2Y = 540
	typeY      = 576 // ~30% như lovebox; câu trong cửa sổ hook card né xuống 1100
)

// writeNarratedASS ghi subs.ass + stage tmpDir/fonts chỉ chứa đúng TTF cần
// (Baloo2 trùng family/style với bản Bold, Montserrat name-table hỏng → trỏ
// thẳng assets/fonts sẽ khiến libass chọn nhầm font).
func writeNarratedASS(spec narratedSubtitleSpec, tmpDir string) (assPath, fontsDir string, err error) {
	style := normalizeSubtitleStyle(spec.Style)
	ac := spec.Accent
	if ac.Highlight == "" {
		ac = accentForTemplate("")
	}

	var b strings.Builder
	b.WriteString("[Script Info]\n")
	b.WriteString("ScriptType: v4.00+\n")
	b.WriteString("PlayResX: 1080\nPlayResY: 1920\n")
	b.WriteString("WrapStyle: 0\nScaledBorderAndShadow: yes\n\n")

	b.WriteString("[V4+ Styles]\n")
	b.WriteString("Format: Name, Fontname, Fontsize, PrimaryColour, SecondaryColour, OutlineColour, BackColour, Bold, Italic, Underline, StrikeOut, ScaleX, ScaleY, Spacing, Angle, BorderStyle, Outline, Shadow, Alignment, MarginL, MarginR, MarginV, Encoding\n")
	glowStyle := assColorStyle(ac.Glow)
	// Karaoke: trắng, viền màu glow (blur inline), không shadow, căn giữa (an5).
	fmt.Fprintf(&b, "Style: Karaoke,Baloo 2,60,&H00FFFFFF,&H00FFFFFF,%s,&H00000000,0,0,0,0,100,100,0,0,1,3,0,5,10,10,10,1\n", glowStyle)
	// Hook: SecondaryColour alpha FF → \ko ẩn ký tự chưa gõ.
	fmt.Fprintf(&b, "Style: Hook,Baloo 2,52,&H00FFFFFF,&HFFFFFFFF,%s,&H00000000,0,0,0,0,100,100,0,0,1,2.5,0,5,10,10,10,1\n", glowStyle)
	if style == "typewriter" {
		// Type: trắng ĐẬM TO fs76 (user yêu cầu "to rõ ràng"), KHÔNG outline/shadow
		// trong style — bóng đen thật là layer event thứ 2 đồng bộ \ko (Shadow
		// field bị \ko leak thành bóng ma cho ký tự chưa gõ).
		b.WriteString("Style: Type,Montserrat,76,&H00FCFEFF,&HFFFFFFFF,&H00000000,&H00000000,0,0,0,0,100,100,0,0,1,0,0,8,10,10,10,1\n")
		// TypeBox: pill đen ~90% (BorderStyle 3, Outline = padding hộp).
		b.WriteString("Style: TypeBox,Montserrat,76,&H00FCFEFF,&HFFFFFFFF,&H1A080505,&H1A080505,0,0,0,0,100,100,0,0,3,20,0,8,10,10,10,1\n")
		// Card serif kiểu lovebox (hook đầu video): tên trắng / dòng phụ giãn chữ
		// (Spacing 6) / giá HỒNG NEON #FF00A9. Card tĩnh (không \ko) nên dùng
		// Shadow field thật được, không lo bóng ma.
		b.WriteString("Style: CardTitle,Playfair Display Bold,105,&H00FFFFFF,&H00FFFFFF,&H00000000,&H99000000,0,0,0,0,100,100,0,0,1,0,2,5,10,10,10,1\n")
		b.WriteString("Style: CardSub,Playfair Display Bold,38,&H00FFFFFF,&H00FFFFFF,&H00000000,&H99000000,0,0,0,0,100,100,6,0,1,0,1.5,5,10,10,10,1\n")
		b.WriteString("Style: CardPrice,Playfair Display Bold,95,&H00A900FF,&H00A900FF,&H00000000,&H60580090,0,0,0,0,100,100,0,0,1,0,3,5,10,10,10,1\n")
	}
	// Heart: màu hồng nhạt, vẽ vector.
	fmt.Fprintf(&b, "Style: Heart,Baloo 2,20,%s,%s,%s,&H00000000,0,0,0,0,100,100,0,0,1,0,0,7,0,0,0,1\n",
		assColorStyle("#F9A0CC"), assColorStyle("#F9A0CC"), assColorStyle("#F9A0CC"))
	b.WriteString("\n[Events]\n")
	b.WriteString("Format: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text\n")

	// Timing từng từ cho từng đoạn.
	tl := spec.Timeline
	perSeg := make([][]wordTiming, len(spec.Script.Segments))
	for i, seg := range spec.Script.Segments {
		if i >= len(spec.VoiceDur) {
			break
		}
		path := ""
		if i < len(spec.VoicePaths) {
			path = spec.VoicePaths[i]
		}
		lead, tail := segmentSpeechBounds(path, spec.VoiceDur[i])
		voiceAt := 0.0
		if i < len(tl.VoiceAt) {
			voiceAt = tl.VoiceAt[i]
		}
		perSeg[i] = distributeWordTimings(seg.Narration, voiceAt, spec.VoiceDur[i], lead, tail)
	}

	switch style {
	case "typewriter":
		buildTypewriterEvents(&b, spec.Script, perSeg, tl, spec.HookEnd)
		// Hook = card serif kiểu lovebox (thay hook chữ hồng); không tim bay.
		buildHookCardEvents(&b, spec)
	default:
		for _, ws := range perSeg {
			buildKaraokeEvents(&b, chunkWords(ws, 6, 20), ac)
		}
		if strings.TrimSpace(spec.HookLine1)+strings.TrimSpace(spec.HookLine2) != "" {
			buildHookEvents(&b, spec.HookLine1, spec.HookLine2, spec.HookEmph, spec.HookEnd, ac)
		}
		if spec.Hearts && spec.HookEnd > 1.0 {
			buildHeartEvents(&b, spec.HookEnd, spec.Seed)
		}
	}

	assPath = filepath.Join(tmpDir, "subs.ass")
	if err = os.WriteFile(assPath, []byte(b.String()), 0o644); err != nil {
		return "", "", err
	}

	fontsDir = filepath.Join(tmpDir, "fonts")
	if err = os.MkdirAll(fontsDir, 0o755); err != nil {
		return "", "", err
	}
	need := []string{"Baloo2-ExtraBold.ttf"}
	if style == "typewriter" {
		need = append(need, "Montserrat-Bold.ttf", "PlayfairDisplay-Bold.ttf")
	}
	srcFonts := filepath.Join(assetsDir(), "fonts")
	for _, f := range need {
		if cerr := copyFile(filepath.Join(srcFonts, f), filepath.Join(fontsDir, f)); cerr != nil {
			return "", "", fmt.Errorf("stage font %s: %v", f, cerr)
		}
	}
	return assPath, fontsDir, nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

// buildKaraokeEvents: 1 Dialogue mỗi khoảng-từ; cả cụm hiện, từ active phóng
// 1.5× (\fs92) + màu accent + glow mạnh; từ đọc xong trở về trắng (base style).
func buildKaraokeEvents(b *strings.Builder, chunks []subChunk, ac accentColor) {
	hl := assColorTag(ac.Highlight)
	glow := assColorTag(ac.Glow)
	for ci, c := range chunks {
		// Đuôi 0.10s cho từ cuối, nhưng không được đè lên chunk kế tiếp
		// (cụm cũ phải biến mất đúng lúc cụm mới hiện).
		nextStart := -1.0
		if ci+1 < len(chunks) && len(chunks[ci+1].Words) > 0 {
			nextStart = chunks[ci+1].Words[0].Start
		}
		for j, w := range c.Words {
			end := w.End
			if j == len(c.Words)-1 {
				end += 0.10
				if nextStart >= 0 && end > nextStart {
					end = nextStart
				}
			}
			var parts []string
			for k, other := range c.Words {
				word := assEscape(other.Word)
				if k == j {
					parts = append(parts, fmt.Sprintf(`{\fs92\bord5\blur16\c%s\3c%s}%s{\r\blur5}`, hl, glow, word))
				} else {
					parts = append(parts, word)
				}
			}
			fmt.Fprintf(b, "Dialogue: 0,%s,%s,Karaoke,,0,0,0,,{\\pos(540,%d)\\blur5}%s\n",
				assTime(w.Start), assTime(end), karaokeY, strings.Join(parts, " "))
		}
	}
}

// koLine sinh chuỗi typewriter per-rune {\koN} cho 1 dòng; emphSpan (nếu có,
// case-insensitive) được viết HOA + màu accent.
func koLine(line, emph string, koVal int, ac accentColor) string {
	hl := assColorTag(ac.Highlight)
	glow := assColorTag(ac.Glow)
	emphStart, emphEnd := -1, -1
	if e := strings.TrimSpace(emph); e != "" {
		if idx := strings.Index(strings.ToLower(line), strings.ToLower(e)); idx >= 0 {
			emphStart = idx
			emphEnd = idx + len(e)
		}
	}
	var sb strings.Builder
	for bi, r := range line {
		inEmph := emphStart >= 0 && bi >= emphStart && bi < emphEnd
		k := koVal
		if r == ' ' {
			k = 1
		}
		if bi == emphStart {
			fmt.Fprintf(&sb, `{\c%s\3c%s\bord4\blur10}`, hl, glow)
		}
		if emphEnd >= 0 && bi == emphEnd {
			fmt.Fprintf(&sb, `{\c&HFFFFFF&\3c%s\bord2.5\blur6}`, glow)
		}
		ch := string(r)
		if inEmph {
			ch = strings.ToUpper(ch)
		}
		fmt.Fprintf(&sb, `{\ko%d}%s`, k, assEscape(ch))
	}
	return sb.String()
}

// buildHookEvents: 2 dòng hook typewriter; dòng 2 (dưới, chứa giá) gõ trước
// dòng 1 ~0.55s như video mẫu; cả hai biến mất tức thì tại hookEnd.
func buildHookEvents(b *strings.Builder, line1, line2, emph string, hookEnd float64, ac accentColor) {
	glow := assColorTag(ac.Glow)
	emit := func(line string, y float64, start float64) {
		line = strings.TrimSpace(line)
		if line == "" || start >= hookEnd {
			return
		}
		n := utf8.RuneCountInString(line)
		koVal := 3 // ~33 ký tự/giây
		if float64(n)*0.03 > 0.9 {
			koVal = int(90/float64(n) + 0.5)
			if koVal < 1 {
				koVal = 1
			}
		}
		fmt.Fprintf(b, "Dialogue: 0,%s,%s,Hook,,0,0,0,,{\\pos(540,%.0f)\\blur6\\3c%s}%s\n",
			assTime(start), assTime(hookEnd), y, glow, koLine(line, emph, koVal, ac))
	}
	emit(line2, hookLine2Y, 0.30)
	emit(line1, hookLine1Y, 0.85)
}

type typeSentence struct {
	Text       string
	Start, End float64
}

// typewriterSentences gom các wordTiming thành câu, NGẮT khi TỪ kết thúc bằng
// dấu kết câu (. ! ? …). Làm việc trực tiếp trên wordTiming (đã có thời gian
// từng từ) thay vì re-split narration rồi đối chiếu số từ — nên "199.000" hay
// "2.5" (dấu chấm GIỮA từ, không phải cuối từ) KHÔNG bị xé và không mất chữ.
func typewriterSentences(ws []wordTiming) []typeSentence {
	endsSentence := func(w string) bool {
		trimmed := strings.TrimRight(w, `"'”’)»`)
		if trimmed == "" {
			return false
		}
		r, _ := utf8.DecodeLastRuneInString(trimmed)
		return strings.ContainsRune(".!?…", r)
	}
	var out []typeSentence
	var words []string
	var start float64
	flush := func(end float64) {
		if len(words) == 0 {
			return
		}
		out = append(out, typeSentence{Text: strings.Join(words, " "), Start: start, End: end})
		words = nil
	}
	for i, w := range ws {
		if len(words) == 0 {
			start = w.Start
		}
		words = append(words, w.Word)
		if endsSentence(w.Word) || i == len(ws)-1 {
			flush(w.End)
		}
	}
	return out
}

// buildTypewriterEvents: mỗi câu gõ per-rune \ko với BÓNG ĐEN THẬT = 2 layer
// đồng bộ (layer 0 = cùng text + cùng nhịp \ko, fill đen mờ lệch +4/+5px —
// bóng "gõ" cùng từng ký tự; Shadow field thì bị \ko leak bóng ma). Câu đầu
// tiên dùng pill đen (TypeBox, không cần layer bóng). Câu nằm trong cửa sổ
// hook card đẩy xuống y=1100 (né card, đúng hành vi lovebox khi phần trên bị
// chiếm).
func buildTypewriterEvents(b *strings.Builder, script *NarrationScript, perSeg [][]wordTiming, tl narratedTimeline, hookEnd float64) {
	first := true
	for i := range script.Segments {
		if i >= len(perSeg) || len(perSeg[i]) == 0 {
			continue
		}
		ws := perSeg[i]
		spans := typewriterSentences(ws)
		for si, sp := range spans {
			start := sp.Start - 0.15
			if start < 0 {
				start = 0
			}
			end := tl.CapEnd[i]
			if si+1 < len(spans) {
				end = spans[si+1].Start - 0.02
			}
			if end <= start {
				continue
			}
			styleName := "Type"
			if first {
				styleName = "TypeBox"
			}
			first = false
			n := utf8.RuneCountInString(sp.Text)
			// Gõ xong trong 80% thời lượng đọc câu, tốc độ nền ~28 ký tự/s.
			typeTime := float64(n) / 28.0
			if lim := 0.8 * (sp.End - sp.Start); typeTime > lim {
				typeTime = lim
			}
			koVal := 1
			if n > 0 {
				koVal = int(typeTime*100/float64(n) + 0.5)
				if koVal < 1 {
					koVal = 1
				}
			}
			text := typewriterWrap(sp.Text, 22)
			ko := koLineNoEmph(text, koVal) // sinh MỘT lần, dùng chung 2 layer cho đồng bộ
			y := typeY
			if start < hookEnd {
				y = 1100
			}
			if styleName == "Type" {
				// Layer 0: bóng đen mờ lệch +4/+5px, gõ cùng nhịp.
				fmt.Fprintf(b, "Dialogue: 0,%s,%s,Type,,0,0,0,,{\\pos(544,%d)\\1c&H000000&\\1a&H73&\\blur2.5}%s\n",
					assTime(start), assTime(end), y+5, ko)
			}
			fmt.Fprintf(b, "Dialogue: 1,%s,%s,%s,,0,0,0,,{\\pos(540,%d)\\blur0.6}%s\n",
				assTime(start), assTime(end), styleName, y, ko)
		}
	}
}

// buildHookCardEvents: card serif kiểu lovebox thay hook chữ hồng ở kiểu
// typewriter — 3 tầng Playfair: tên UPPER trắng / "- TIỆN NGHI & TIỆN NGHI -"
// giãn chữ / giá UPPER hồng neon #FF00A9. Vào bằng zoom-out 125→100 + fade
// 0.4s (đo từ video mẫu), biến mất phụt tại HookEnd.
func buildHookCardEvents(b *strings.Builder, spec narratedSubtitleSpec) {
	start, end := 0.30, spec.HookEnd
	if end <= start+0.2 {
		return
	}
	const entrance = `{\fad(400,0)\fscx125\fscy125\t(0,400,\fscx100\fscy100)}`
	emit := func(styleName, text string, y, baseFS, budget int) {
		text = strings.TrimSpace(text)
		if text == "" {
			return
		}
		fsTag := ""
		if fs := cardFS(baseFS, utf8.RuneCountInString(text), budget); fs < baseFS {
			fsTag = fmt.Sprintf(`{\fs%d}`, fs)
		}
		fmt.Fprintf(b, "Dialogue: 0,%s,%s,%s,,0,0,0,,{\\pos(540,%d)}%s%s%s\n",
			assTime(start), assTime(end), styleName, y, entrance, fsTag, assEscape(text))
	}
	emit("CardTitle", strings.ToUpper(spec.HookLine1), 470, 105, 1700)
	sub := cardSubLine(spec.Amenities)
	emit("CardSub", sub, 585, 38, 3200)
	price := strings.TrimSpace(spec.HookEmph)
	if price == "" {
		price = strings.TrimSpace(spec.HookLine2)
	}
	priceY := 720
	if sub == "" { // thiếu dòng giữa → kéo giá lên gần tên cho khối card liền lạc
		priceY = 650
	}
	emit("CardPrice", strings.ToUpper(price), priceY, 95, 1800)
}

// cardSubLine: "- MÁY CHIẾU & BỒN TẮM -" từ 2 tiện nghi đầu của listing.
func cardSubLine(amenities []string) string {
	var parts []string
	for _, a := range amenities {
		if a = strings.TrimSpace(a); a == "" {
			continue
		}
		parts = append(parts, strings.ToUpper(a))
		if len(parts) == 2 {
			break
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return "- " + strings.Join(parts, " & ") + " -"
}

// cardFS co chữ card cho vừa bề ngang (~budget px-per-rune đã quy đổi):
// fs = budget/runes, kẹp [40, base].
func cardFS(base, runes, budget int) int {
	if runes < 1 {
		runes = 1
	}
	fs := budget / runes
	if fs > base {
		fs = base
	}
	if fs < 40 {
		fs = 40
	}
	return fs
}

// typewriterWrap chèn \n tại ranh giới từ mỗi ~maxRunes (tối đa 2 lần xuống dòng).
func typewriterWrap(s string, maxRunes int) string {
	words := strings.Fields(s)
	var lines []string
	cur := ""
	for _, w := range words {
		next := strings.TrimSpace(cur + " " + w)
		if cur != "" && utf8.RuneCountInString(next) > maxRunes && len(lines) < 2 {
			lines = append(lines, cur)
			cur = w
			continue
		}
		cur = next
	}
	if cur != "" {
		lines = append(lines, cur)
	}
	return strings.Join(lines, "\n")
}

func koLineNoEmph(line string, koVal int) string {
	var sb strings.Builder
	for _, r := range line {
		if r == '\n' {
			sb.WriteString(`\N`)
			continue
		}
		k := koVal
		if r == ' ' {
			k = 1
		}
		fmt.Fprintf(&sb, `{\ko%d}%s`, k, assEscape(string(r)))
	}
	return sb.String()
}

// buildHeartEvents: tim vector {\p1} nhấp nháy quanh sticker mèo trong cửa sổ
// hook (x 455–635, y 205–300). Deterministic theo seed (cache/resume an toàn).
func buildHeartEvents(b *strings.Builder, hookEnd float64, seed int64) {
	const heartPath = `m 15 8 b 15 2 24 0 27 6 b 30 12 20 20 15 26 b 10 20 0 12 3 6 b 6 0 15 2 15 8`
	rng := rand.New(rand.NewSource(seed))
	for t := 0.5; t < hookEnd-0.3; t += 0.25 {
		x := 455 + rng.Intn(181)
		y := 205 + rng.Intn(96)
		sc := 60 + rng.Intn(51)
		life := 0.35 + rng.Float64()*0.15
		end := t + life
		if end > hookEnd {
			end = hookEnd
		}
		fmt.Fprintf(b, "Dialogue: 1,%s,%s,Heart,,0,0,0,,{\\pos(%d,%d)\\fad(80,150)\\fscx%d\\fscy%d\\p1}%s{\\p0}\n",
			assTime(t), assTime(end), x, y, sc, sc, heartPath)
	}
}
