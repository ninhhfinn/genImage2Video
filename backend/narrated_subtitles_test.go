package main

// Test engine phụ đề ASS cho mode Thuyết minh AI (karaoke/typewriter + hook).
// Toàn bộ hàm thuần — không cần ffmpeg/API key.

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

func TestNormalizeSubtitleStyle(t *testing.T) {
	cases := map[string]string{
		"":           "karaoke",
		"karaoke":    "karaoke",
		" KARAOKE ":  "karaoke",
		"typewriter": "typewriter",
		"Typewriter": "typewriter",
		"vớ vẩn":     "karaoke",
	}
	for in, want := range cases {
		if got := normalizeSubtitleStyle(in); got != want {
			t.Errorf("normalizeSubtitleStyle(%q) = %q, muốn %q", in, got, want)
		}
	}
}

func TestDistributeWordTimings(t *testing.T) {
	ws := distributeWordTimings("một hai ba bốn năm", 10, 5, 0.2, 0.3)
	if len(ws) != 5 {
		t.Fatalf("muốn 5 từ, được %d", len(ws))
	}
	if d := ws[0].Start - 10.2; d > 1e-9 || d < -1e-9 {
		t.Errorf("từ đầu phải bắt đầu tại voiceAt+lead=10.2, được %.4f", ws[0].Start)
	}
	usableEnd := 10.0 + 5.0 - 0.3
	if d := ws[4].End - usableEnd; d > 1e-6 || d < -1e-6 {
		t.Errorf("từ cuối phải kết thúc tại %.2f, được %.4f", usableEnd, ws[4].End)
	}
	for i, w := range ws {
		if w.End-w.Start < 0.12-1e-9 {
			t.Errorf("từ %d (%q) ngắn hơn 0.12s: %.4f", i, w.Word, w.End-w.Start)
		}
		if i > 0 {
			if d := w.Start - ws[i-1].End; d > 1e-9 || d < -1e-9 {
				t.Errorf("từ %d không nối liền từ trước (Start=%.4f, prev End=%.4f)", i, w.Start, ws[i-1].End)
			}
		}
	}
	if got := distributeWordTimings("   ", 0, 1, 0, 0); got != nil {
		t.Errorf("narration rỗng phải trả nil, được %v", got)
	}
	// Từ dài hơn được nhiều thời gian hơn từ ngắn (trọng số rune).
	ws2 := distributeWordTimings("a thườnggggg", 0, 2, 0, 0)
	if len(ws2) == 2 && ws2[1].End-ws2[1].Start <= ws2[0].End-ws2[0].Start {
		t.Errorf("từ dài phải được nhiều thời gian hơn: %v", ws2)
	}
}

func mkWords(t *testing.T, sentence string) []wordTiming {
	t.Helper()
	return distributeWordTimings(sentence, 0, float64(len(strings.Fields(sentence))), 0, 0)
}

func TestChunkWords(t *testing.T) {
	ws := mkWords(t, "ôi trời ơi, chuyện là anh mới vớ phải cô người yêu.")
	chunks := chunkWords(ws, 6, 20)
	if len(chunks) < 2 {
		t.Fatalf("phải tách ≥2 chunk, được %d", len(chunks))
	}
	// Dấu phẩy phải cắt chunk: chunk đầu kết thúc ở "ơi,".
	first := chunks[0].Words
	if !strings.HasSuffix(first[len(first)-1].Word, ",") {
		t.Errorf("chunk đầu phải kết thúc tại dấu phẩy, được %q", first[len(first)-1].Word)
	}
	total := 0
	for ci, c := range chunks {
		if len(c.Words) == 0 || len(c.Words) > 6 {
			t.Errorf("chunk %d có %d từ (phải 1..6)", ci, len(c.Words))
		}
		if len(c.Words) == 1 && ci > 0 {
			t.Errorf("chunk %d mồ côi 1 từ — phải gộp vào chunk trước", ci)
		}
		total += len(c.Words)
	}
	if total != len(ws) {
		t.Errorf("mất từ khi chunk: %d != %d", total, len(ws))
	}
}

func TestAssColorHelpers(t *testing.T) {
	if got := assColorStyle("#FF87CD"); got != "&H00CD87FF" {
		t.Errorf("assColorStyle: %q", got)
	}
	if got := assColorTag("#ff87cd"); got != "&HCD87FF&" {
		t.Errorf("assColorTag: %q", got)
	}
	if got := assTime(0); got != "0:00:00.00" {
		t.Errorf("assTime(0): %q", got)
	}
	if got := assTime(3661.5); got != "1:01:01.50" {
		t.Errorf("assTime(3661.5): %q", got)
	}
	if got := assEscape("{a}\nb"); got != "(a)\\Nb" {
		t.Errorf("assEscape: %q", got)
	}
	// [review #3] backslash trong text (tên/lời kể không tin cậy) phải bị vô hiệu
	// hoá, kẻo chèn override ASS làm vỡ phụ đề.
	if got := assEscape(`Căn \x hộ`); strings.Contains(got, `\`) {
		t.Errorf("assEscape để lọt backslash (injection): %q", got)
	}
}

func TestAccentForTemplate(t *testing.T) {
	for name := range videoTemplateNames {
		if _, ok := templateAccent[name]; !ok {
			t.Errorf("templateAccent thiếu %q", name)
		}
	}
	for _, name := range []string{"daiky", "sunset"} {
		if _, ok := templateAccent[name]; !ok {
			t.Errorf("templateAccent thiếu %q", name)
		}
	}
	if got := accentForTemplate("chillgreen").Highlight; got != "#A8D94B" {
		t.Errorf("chillgreen highlight: %q", got)
	}
	if got := accentForTemplate("không-tồn-tại").Highlight; got != "#FF87CD" {
		t.Errorf("template lạ phải về hồng mặc định, được %q", got)
	}
	if got := accentForTemplate("").Glow; got != "#FF4FAE" {
		t.Errorf("template rỗng phải về glow hồng mặc định, được %q", got)
	}
}

func TestSubtitlesFilterArg(t *testing.T) {
	got := subtitlesFilterArg("/tmp/a:b/subs.ass", "/tmp/f'd/fonts")
	if !strings.Contains(got, "subtitles=filename=") || !strings.Contains(got, "fontsdir=") {
		t.Fatalf("thiếu filename/fontsdir: %q", got)
	}
	if strings.Contains(got, "a:b") {
		t.Errorf("dấu ':' trong path chưa được escape: %q", got)
	}
	if !strings.Contains(got, `a\:b`) {
		t.Errorf("muốn 'a\\:b' trong %q", got)
	}
	// [review #8] path chứa nháy đơn phải escape kiểu '\'' (đóng-escape-mở) để
	// sống sót qua lớp bọc nháy đơn của ffmpeg, không thì hỏng render.
	if !strings.Contains(got, `'\''`) {
		t.Errorf("nháy đơn trong path phải thành '\\'' , được %q", got)
	}
}

func TestComposeHookTitle(t *testing.T) {
	cfg := Config{
		Nickname: "Homestay Camellia",
		Prices:   []string{"Qua đêm: 350.000đ", "Khung 2h: chỉ 199k"},
	}
	l1, l2, emph := composeHookTitle(cfg)
	if !strings.Contains(l1, "Camellia") {
		t.Errorf("line1 phải chứa nickname, được %q", l1)
	}
	if !strings.Contains(l2, "199k") {
		t.Errorf("line2 phải chứa giá rẻ nhất 199k, được %q", l2)
	}
	if emph == "" || !strings.Contains(strings.ToLower(l2), strings.ToLower(emph)) {
		t.Errorf("emphasis %q phải nằm trong line2 %q", emph, l2)
	}
}

// Format giá THẬT từ API (price_line_items + price_lines_by_template).
func TestComposeHookTitleRealAPIFormats(t *testing.T) {
	// Canonical price_line_items (đường narrate mới): đủ số giờ, số dấu chấm.
	// "Giá thêm giờ" KHÔNG được chọn làm giá mồi (dễ gây hiểu lầm).
	cfg := Config{
		Nickname: "R203",
		Prices: []string{
			"Giá 2 ngày 1 đêm: Từ 369.000 đ",
			"Qua đêm 9PM-9AM: Từ 295.200 đ",
			"Giá 3 giờ đầu: 239.000 đ",
			"Giá thêm giờ: 50.000 đ",
		},
	}
	_, l2, emph := composeHookTitle(cfg)
	if !strings.Contains(l2, "239k") {
		t.Errorf("phải chọn giá giờ đầu 239k (không phải thêm giờ 50k), được %q", l2)
	}
	if !strings.Contains(l2, "3 tiếng") {
		t.Errorf("giá giờ đầu phải kèm đúng số giờ '3 tiếng', được %q", l2)
	}
	if emph == "" || !strings.Contains(strings.ToLower(l2), strings.ToLower(emph)) {
		t.Errorf("emphasis %q phải nằm trong line2 %q", emph, l2)
	}

	// Format template amorex ("cá") và chillgreen (số trần cuối dòng).
	ca := Config{Nickname: "DBP 701", Prices: []string{"Giá giờ: 189 cá", "Giá qua đêm: 359 cá", "2N1Đ: 449 cá"}}
	if _, l2, _ := composeHookTitle(ca); !strings.Contains(l2, "189k") {
		t.Errorf("amorex 'cá' phải parse được → 189k, được %q", l2)
	}
	cg := Config{Nickname: "TC 303", Prices: []string{"21h - 9h (Qua đêm): 332", "2N1Đ: 439", "10h - 13h: 220"}}
	if _, l2, _ := composeHookTitle(cg); !strings.Contains(l2, "220k") {
		t.Errorf("chillgreen số trần phải parse được → 220k, được %q", l2)
	}
	gs := Config{Nickname: "Gold", Prices: []string{"2N1Đ: 449,000", "Qua đêm: 359,000"}}
	if _, l2, _ := composeHookTitle(gs); !strings.Contains(l2, "359k") {
		t.Errorf("goldserif số phẩy phải parse được → 359k, được %q", l2)
	}
}

// hookPriceMismatch: giá trong hook phải khớp giá trong dữ liệu (grounding).
func TestHookPriceMismatch(t *testing.T) {
	prices := []string{"Giá 2 giờ đầu: 199.000 đ", "Qua đêm 9PM-9AM: Từ 307.300 đ", "2N1Đ: 439k"}
	if tok, bad := hookPriceMismatch("Chỉ 199k/2 tiếng", prices); bad {
		t.Errorf("199k có trong dữ liệu mà vẫn báo lệch (token %q)", tok)
	}
	if tok, bad := hookPriceMismatch("Chỉ 307k/đêm", prices); bad {
		t.Errorf("307k (làm tròn 307.300) phải được chấp nhận, báo lệch token %q", tok)
	}
	if _, bad := hookPriceMismatch("Chỉ 99k/2 người", prices); !bad {
		t.Error("99k KHÔNG có trong dữ liệu — phải báo lệch")
	}
	// "2 người"/"2 tiếng" không phải token giá — không được báo lệch.
	if tok, bad := hookPriceMismatch("Căn hộ xinh cho 2 người", prices); bad {
		t.Errorf("'2 người' không phải giá mà báo lệch (token %q)", tok)
	}
	if _, bad := hookPriceMismatch("", prices); bad {
		t.Error("hook rỗng không được báo lệch")
	}
	// [review #7] "các"/"cách"/"cá nhân" sau chữ số KHÔNG được đọc nhầm là giá "cá".
	if tok, bad := hookPriceMismatch("Xỉu luôn 2 các nàng ơi", prices); bad {
		t.Errorf("'2 các' bị đọc nhầm là giá → báo lệch sai (token %q)", tok)
	}
	if got := priceHitsInLine("2 các nàng"); len(got) != 0 {
		t.Errorf("'2 các' không được coi là giá, được %v", got)
	}
	// Nhưng "189 cá" (đơn vị giá thật của template amorex) vẫn phải bắt được.
	if got := priceHitsInLine("Giá qua đêm: 189 cá"); len(got) == 0 || got[len(got)-1] != 189 {
		t.Errorf("'189 cá' phải = 189, được %v", got)
	}
}

// specFixture dựng spec tổng hợp dùng chung cho các test writeNarratedASS.
func specFixture(t *testing.T, style string) (narratedSubtitleSpec, string) {
	t.Helper()
	script := &NarrationScript{
		Segments: []NarrationSegment{
			{ImageIndex: 0, Caption: "Phòng khách", Narration: "Ôi trời ơi, căn này đẹp điên luôn các vợ ạ."},
			{ImageIndex: 1, Caption: "Góc bếp", Narration: "Bếp nấu xịn xò. Máy chiếu nét căng."},
		},
	}
	voiceDur := []float64{4.0, 3.0}
	tl := computeNarratedTimeline(voiceDur, 0.5, 1.6, 30)
	spec := narratedSubtitleSpec{
		Style:      style,
		Script:     script,
		Timeline:   tl,
		VoiceDur:   voiceDur,
		VoicePaths: []string{"/nonexistent/a.mp3", "/nonexistent/b.mp3"},
		Accent:     accentForTemplate("chillgreen"),
		HookEnd:    tl.Offsets[1],
		HookLine1:  "Căn hộ Camellia",
		HookLine2:  "Chỉ 199k/2 người",
		HookEmph:   "199k/2 người",
		Amenities:  []string{"Máy chiếu", "Bồn tắm", "Bếp nấu"},
		Hearts:     true,
		Seed:       42,
	}
	return spec, t.TempDir()
}

func readASS(t *testing.T, assPath string) string {
	t.Helper()
	b, err := os.ReadFile(assPath)
	if err != nil {
		t.Fatalf("đọc ass: %v", err)
	}
	return string(b)
}

func dialogueLines(content, style string) []string {
	var out []string
	for _, ln := range strings.Split(content, "\n") {
		if strings.HasPrefix(ln, "Dialogue:") && strings.Contains(ln, ","+style+",") {
			out = append(out, ln)
		}
	}
	return out
}

func TestWriteNarratedASSKaraoke(t *testing.T) {
	spec, dir := specFixture(t, "karaoke")
	assPath, fontsDir, err := writeNarratedASS(spec, dir)
	if err != nil {
		t.Fatalf("writeNarratedASS: %v", err)
	}
	content := readASS(t, assPath)

	if !strings.Contains(content, "PlayResX: 1080") || !strings.Contains(content, "PlayResY: 1920") {
		t.Error("thiếu PlayRes 1080x1920")
	}
	if !strings.Contains(content, "Style: Karaoke,Baloo 2,") {
		t.Error("thiếu style Karaoke font Baloo 2")
	}

	// Karaoke: 1 Dialogue mỗi từ.
	totalWords := 0
	for _, seg := range spec.Script.Segments {
		totalWords += len(strings.Fields(seg.Narration))
	}
	karaokeEvents := dialogueLines(content, "Karaoke")
	if len(karaokeEvents) != totalWords {
		t.Errorf("muốn %d event karaoke (1/từ), được %d", totalWords, len(karaokeEvents))
	}
	// Mỗi event đúng 1 từ phóng to \fs92, có màu accent chillgreen.
	hl := assColorTag(spec.Accent.Highlight)
	for i, ev := range karaokeEvents {
		if c := strings.Count(ev, `\fs92`); c != 1 {
			t.Errorf("event %d có %d lần \\fs92 (phải đúng 1): %s", i, c, ev)
		}
		if !strings.Contains(ev, hl) {
			t.Errorf("event %d thiếu màu accent %s", i, hl)
		}
	}

	// Hook: 2 event, dòng 2 bắt đầu trước dòng 1, cùng kết thúc tại HookEnd.
	hookEvents := dialogueLines(content, "Hook")
	if len(hookEvents) != 2 {
		t.Fatalf("muốn 2 event Hook, được %d", len(hookEvents))
	}
	// Chữ trong hook bị xen tag {\ko} giữa từng ký tự → strip tag rồi mới so.
	stripTags := regexp.MustCompile(`\{[^}]*\}`)
	hookPlain := stripTags.ReplaceAllString(strings.Join(hookEvents, "\n"), "")
	if !strings.Contains(hookPlain, "199K/2 NGƯỜI") {
		t.Errorf("cụm emphasis phải được viết HOA trong hook, được: %s", hookPlain)
	}
	wantEnd := assTime(spec.HookEnd)
	for i, ev := range hookEvents {
		if !strings.Contains(ev, wantEnd) {
			t.Errorf("hook event %d phải kết thúc tại %s: %s", i, wantEnd, ev)
		}
		if !strings.Contains(ev, `\ko`) {
			t.Errorf("hook event %d thiếu typewriter \\ko", i)
		}
	}

	// Tim bay: có event Heart chứa vector \p1.
	hearts := dialogueLines(content, "Heart")
	if len(hearts) == 0 {
		t.Error("thiếu event Heart")
	}
	for _, h := range hearts {
		if !strings.Contains(h, `\p1`) {
			t.Errorf("heart thiếu \\p1: %s", h)
			break
		}
	}

	// Fonts staged: chỉ Baloo2-ExtraBold cho karaoke.
	entries, err := os.ReadDir(fontsDir)
	if err != nil {
		t.Fatalf("đọc fontsDir: %v", err)
	}
	names := map[string]bool{}
	for _, e := range entries {
		names[e.Name()] = true
	}
	if !names["Baloo2-ExtraBold.ttf"] {
		t.Errorf("fontsDir thiếu Baloo2-ExtraBold.ttf: %v", names)
	}
	if names["Montserrat-Bold.ttf"] {
		t.Errorf("karaoke không được stage Montserrat: %v", names)
	}
	if names["Baloo2-Bold.ttf"] {
		t.Errorf("không được stage Baloo2-Bold (trùng family/style với ExtraBold): %v", names)
	}
}

func TestKaraokeEventTimesMonotonic(t *testing.T) {
	spec, dir := specFixture(t, "karaoke")
	assPath, _, err := writeNarratedASS(spec, dir)
	if err != nil {
		t.Fatalf("writeNarratedASS: %v", err)
	}
	content := readASS(t, assPath)
	re := regexp.MustCompile(`^Dialogue: \d+,(\d+:\d+:\d+\.\d+),(\d+:\d+:\d+\.\d+),Karaoke,`)
	var prevEnd string
	for _, ln := range strings.Split(content, "\n") {
		m := re.FindStringSubmatch(ln)
		if m == nil {
			continue
		}
		start, end := m[1], m[2]
		if start >= end {
			t.Errorf("event start %s >= end %s", start, end)
		}
		if prevEnd != "" && start < prevEnd {
			t.Errorf("event chồng lấn: start %s < prev end %s", start, prevEnd)
		}
		prevEnd = end
	}
	if prevEnd == "" {
		t.Fatal("không tìm thấy event Karaoke nào")
	}
}

func TestWriteNarratedASSTypewriter(t *testing.T) {
	spec, dir := specFixture(t, "typewriter")
	assPath, fontsDir, err := writeNarratedASS(spec, dir)
	if err != nil {
		t.Fatalf("writeNarratedASS: %v", err)
	}
	content := readASS(t, assPath)

	if evs := dialogueLines(content, "Karaoke"); len(evs) != 0 {
		t.Errorf("typewriter không được có event Karaoke, có %d", len(evs))
	}
	// 3 câu trong fixture: 1 (seg0, TypeBox pill) + 2 (seg1). Mỗi câu thường có
	// THÊM 1 layer bóng đen đồng bộ \ko; câu pill không cần bóng → 2n−1 = 5.
	typeEvents := append(dialogueLines(content, "TypeBox"), dialogueLines(content, "Type")...)
	if len(typeEvents) != 5 {
		t.Errorf("muốn 5 event Type/TypeBox (2 câu thường ×2 layer + 1 pill), được %d", len(typeEvents))
	}
	if len(dialogueLines(content, "TypeBox")) != 1 {
		t.Errorf("câu đầu tiên phải dùng TypeBox (pill đen), được %d event TypeBox", len(dialogueLines(content, "TypeBox")))
	}
	shadowLayers := 0
	for _, ev := range typeEvents {
		if !strings.Contains(ev, `\ko`) {
			t.Errorf("event Type thiếu \\ko: %s", ev)
		}
		if strings.Contains(ev, `\1c&H000000&`) {
			shadowLayers++
		}
	}
	if shadowLayers != 2 {
		t.Errorf("muốn 2 layer bóng đen (\\1c&H000000&) cho 2 câu thường, được %d", shadowLayers)
	}
	if !strings.Contains(content, "Style: Type,Montserrat,76,") {
		t.Error("style Type phải là Montserrat cỡ 76 (to rõ ràng)")
	}
	// Caption trong cửa sổ hook card phải né xuống y=1100; sau đó về 576.
	if !strings.Contains(content, `\pos(540,1100`) {
		t.Error("caption trong cảnh 1 phải đẩy xuống y=1100 (né card)")
	}
	if !strings.Contains(content, `\pos(540,576`) {
		t.Error("caption sau hook phải ở y=576")
	}

	// Typewriter KHÔNG dùng hook chữ hồng + KHÔNG tim bay — thay bằng card serif.
	if len(dialogueLines(content, "Hook")) != 0 {
		t.Error("typewriter không được có event Hook hồng (đã thay bằng card serif)")
	}
	if len(dialogueLines(content, "Heart")) != 0 {
		t.Error("typewriter không được có tim bay")
	}
	for _, st := range []string{"CardTitle", "CardSub", "CardPrice"} {
		if !strings.Contains(content, "Style: "+st+",Playfair Display Bold,") {
			t.Errorf("thiếu style %s font Playfair Display Bold", st)
		}
		if len(dialogueLines(content, st)) != 1 {
			t.Errorf("muốn 1 event %s, được %d", st, len(dialogueLines(content, st)))
		}
	}
	title := dialogueLines(content, "CardTitle")[0]
	if !strings.Contains(title, "CĂN HỘ CAMELLIA") {
		t.Errorf("tier1 phải là HookLine1 viết HOA: %s", title)
	}
	if !strings.Contains(title, `\t(0,400`) || !strings.Contains(title, `\fscx125`) {
		t.Errorf("card phải vào bằng zoom-out 125→100 trong 0.4s: %s", title)
	}
	sub := dialogueLines(content, "CardSub")[0]
	if !strings.Contains(sub, "MÁY CHIẾU & BỒN TẮM") {
		t.Errorf("tier2 phải là 2 tiện nghi đầu viết HOA: %s", sub)
	}
	price := dialogueLines(content, "CardPrice")[0]
	if !strings.Contains(price, "199K/2 NGƯỜI") {
		t.Errorf("tier3 phải là emphasis viết HOA: %s", price)
	}
	if !strings.Contains(content, "A900FF") {
		t.Error("giá card phải màu hồng neon #FF00A9 (&H..A900FF)")
	}

	entries, _ := os.ReadDir(fontsDir)
	names := map[string]bool{}
	for _, e := range entries {
		names[e.Name()] = true
	}
	if !names["Montserrat-Bold.ttf"] {
		t.Error("typewriter phải stage Montserrat-Bold.ttf")
	}
	if !names["PlayfairDisplay-Bold.ttf"] {
		t.Error("typewriter phải stage PlayfairDisplay-Bold.ttf (card serif)")
	}
}

// [review #2] Tách câu typewriter phải theo dấu KẾT CÂU cuối từ, KHÔNG xé
// "199.000" và không làm mất chữ.
func TestTypewriterSentences(t *testing.T) {
	// "199.000" nằm giữa câu, không có dấu kết câu → 1 câu, giữ nguyên.
	ws := distributeWordTimings("chỉ 199.000 đồng thôi", 0, 4, 0, 0)
	ss := typewriterSentences(ws)
	if len(ss) != 1 {
		t.Fatalf("phải 1 câu (không xé 199.000), được %d: %+v", len(ss), ss)
	}
	if !strings.Contains(ss[0].Text, "199.000") {
		t.Errorf("giá 199.000 bị xé: %q", ss[0].Text)
	}

	// 2 câu thật (dấu chấm cuối từ + có khoảng trắng).
	ws2 := distributeWordTimings("Phòng đẹp lắm. Giá tốt nữa.", 0, 6, 0, 0)
	ss2 := typewriterSentences(ws2)
	if len(ss2) != 2 {
		t.Fatalf("phải 2 câu, được %d: %+v", len(ss2), ss2)
	}

	// KHÔNG mất chữ: mọi từ đều xuất hiện trong các câu, thời gian tăng dần.
	ws3 := distributeWordTimings("Giá 2.5 và 3.5 triệu đồng.", 0, 6, 0, 0)
	ss3 := typewriterSentences(ws3)
	joined := ""
	var prevEnd float64
	for _, s := range ss3 {
		joined += s.Text + " "
		if s.Start > s.End {
			t.Errorf("câu có start>end: %+v", s)
		}
		if s.Start < prevEnd-1e-9 {
			t.Errorf("câu chồng lấn thời gian: %+v prevEnd=%.3f", s, prevEnd)
		}
		prevEnd = s.End
	}
	for _, w := range []string{"2.5", "3.5", "triệu"} {
		if !strings.Contains(joined, w) {
			t.Errorf("mất chữ %q khỏi phụ đề: %q", w, joined)
		}
	}
}

func TestCardFS(t *testing.T) {
	if got := cardFS(105, 8, 1700); got != 105 {
		t.Errorf("tên ngắn phải giữ fs gốc 105, được %d", got)
	}
	if got := cardFS(105, 28, 1700); got >= 105 || got < 40 {
		t.Errorf("tên 28 rune phải giảm fs (40..104), được %d", got)
	}
	if got := cardFS(105, 100, 1700); got != 40 {
		t.Errorf("fs không được nhỏ hơn sàn 40, được %d", got)
	}
}

// TestNarratedArgsIncludeSubtitles: buildNarrated (stub script+TTS, không chạy
// ffmpeg render) phải: chèn filter subtitles=, sinh .ass đúng font/màu accent,
// có watermark PNG, và KHÔNG còn caption pill cũ.
func TestNarratedArgsIncludeSubtitles(t *testing.T) {
	isolateScriptLib(t)
	if _, err := exec.LookPath("ffprobe"); err != nil {
		t.Skip("không có ffprobe (synthSegments cần đo thời lượng)")
	}
	tmp := t.TempDir()
	imgs := []string{
		makeColorImage(t, tmp, "0", "red", 1200, 800),
		makeColorImage(t, tmp, "1", "green", 800, 1200),
	}
	origScript, origTTS := genScript, synthTTS
	defer func() { genScript, synthTTS = origScript, origTTS }()
	genScript = func(cfg Config, images []string) (*NarrationScript, error) {
		return &NarrationScript{Segments: []NarrationSegment{
			{ImageIndex: 0, Caption: "Phòng khách", Narration: "Chào các con vợ, phòng khách xinh xỉu luôn."},
			{ImageIndex: 1, Caption: "Giường ngủ", Narration: "Nệm êm ru, nằm là quên lối về."},
		}}, nil
	}
	synthTTS = func(cfg Config, text string) ([]byte, error) {
		return silentMP3(t, tmp, 2.0), nil
	}

	run := func(t *testing.T, subtitleStyle string) (joined, assContent string) {
		cfg := Config{
			Narrate: true, Mode: "kenburns", Output: filepath.Join(tmp, "out_"+subtitleStyle+".mp4"),
			Width: 1080, Height: 1920, FPS: 30, ZoomIntensity: 0.4,
			TTSProvider: "google", NarrationPersona: "haihuoc",
			Template: "chillgreen", Watermark: "@dayladau",
			Nickname: "Camellia", Prices: []string{"Khung 2h: 199k"},
			SubtitleStyle: subtitleStyle,
		}
		args, err := buildNarrated(cfg, imgs)
		if err != nil {
			t.Fatalf("buildNarrated: %v", err)
		}
		joined = strings.Join(args, "\n")
		m := regexp.MustCompile(`subtitles=filename='([^']+)'`).FindStringSubmatch(joined)
		if m == nil {
			t.Fatalf("args thiếu filter subtitles=filename=…:\n%s", joined)
		}
		assFile := strings.ReplaceAll(m[1], `\:`, ":")
		b, err := os.ReadFile(assFile)
		if err != nil {
			t.Fatalf("không đọc được file .ass %s: %v", assFile, err)
		}
		return joined, string(b)
	}

	t.Run("karaoke", func(t *testing.T) {
		joined, ass := run(t, "karaoke")
		if !strings.Contains(ass, "Baloo 2") {
			t.Error(".ass thiếu font Baloo 2")
		}
		// Accent chillgreen #A8D94B → tag &H4BD9A8&.
		if !strings.Contains(ass, "4BD9A8") {
			t.Error(".ass thiếu màu accent chillgreen")
		}
		if !strings.Contains(ass, "Dialogue:") {
			t.Error(".ass không có event nào")
		}
		if len(dialogueLines(ass, "Hook")) != 2 {
			t.Error("thiếu 2 event Hook (fallback composeHookTitle từ Nickname+Prices)")
		}
		if !strings.Contains(joined, "watermark.png") {
			t.Error("args thiếu watermark PNG")
		}
		if strings.Contains(joined, "cap00.png") || strings.Contains(joined, "cap01.png") {
			t.Error("caption pill cũ vẫn còn trong args — phải bỏ")
		}
		// Không còn template overlay (bảng giá composite full-frame).
		if strings.Contains(joined, "overlay.png") || strings.Contains(joined, "composite") {
			t.Errorf("args vẫn còn template overlay:\n%s", joined)
		}
	})
	t.Run("typewriter", func(t *testing.T) {
		joined, ass := run(t, "typewriter")
		if !strings.Contains(ass, "Montserrat") {
			t.Error(".ass typewriter thiếu font Montserrat")
		}
		if !strings.Contains(ass, "Playfair Display Bold") {
			t.Error(".ass typewriter thiếu font card serif Playfair Display Bold")
		}
		if len(dialogueLines(ass, "Karaoke")) != 0 {
			t.Error("typewriter không được có event Karaoke")
		}
		// Typewriter tắt trang trí: không sticker mèo, không khói.
		if strings.Contains(joined, "meme_cat.png") || strings.Contains(joined, "pink_ink.mp4") {
			t.Error("typewriter không được nhận input sticker/khói")
		}
	})
}

// [review #1] buildNarrated nhận cfg landscape (khi toggle 9:16 tắt) phải ÉP về
// dọc 1080×1920 — nếu không, nhánh khói pad 1080×1920 blend với khung 1920×1080
// sẽ vỡ toàn bộ render.
func TestNarratedForcesPortrait(t *testing.T) {
	isolateScriptLib(t)
	if _, err := exec.LookPath("ffprobe"); err != nil {
		t.Skip("không có ffprobe")
	}
	tmp := t.TempDir()
	imgs := []string{
		makeColorImage(t, tmp, "0", "red", 1200, 800),
		makeColorImage(t, tmp, "1", "green", 800, 1200),
	}
	origScript, origTTS := genScript, synthTTS
	defer func() { genScript, synthTTS = origScript, origTTS }()
	genScript = func(cfg Config, images []string) (*NarrationScript, error) {
		return &NarrationScript{Segments: []NarrationSegment{
			{ImageIndex: 0, Caption: "A", Narration: "Chào các con vợ, phòng xinh xỉu luôn nha."},
			{ImageIndex: 1, Caption: "B", Narration: "Giường êm ru quên lối về."},
		}}, nil
	}
	synthTTS = func(cfg Config, text string) ([]byte, error) { return silentMP3(t, tmp, 2.0), nil }

	cfg := Config{
		Narrate: true, Mode: "kenburns", Output: filepath.Join(tmp, "out.mp4"),
		Width: 1920, Height: 1080, FPS: 30, ZoomIntensity: 0.4, // landscape (toggle 9:16 tắt)
		TTSProvider: "google", NarrationPersona: "haihuoc",
		Template: "amorex", Watermark: "@x", Nickname: "Camellia", Prices: []string{"Khung 2h: 199k"},
	}
	args, err := buildNarrated(cfg, imgs)
	if err != nil {
		t.Fatalf("buildNarrated: %v", err)
	}
	joined := strings.Join(args, "\n")
	if strings.Contains(joined, "1920x1080") || strings.Contains(joined, "pad=1920:1080") {
		t.Errorf("vẫn còn khung landscape trong filtergraph:\n%s", joined)
	}
	if !strings.Contains(joined, "1080x1920") {
		t.Error("zoompan phải ép về 1080x1920")
	}
	if strings.Contains(joined, "pink_ink") && !strings.Contains(joined, "pad=1080:1920") {
		t.Error("nhánh khói phải pad 1080:1920 khớp khung (nếu có asset khói)")
	}
}

// [review #1 regression, có render thật] Trước fix: narrate + khung landscape →
// blend khói lệch cỡ → ffmpeg abort, không xuất file. Sau fix: ép dọc, render OK.
func TestNarratedLandscapeRenders(t *testing.T) {
	isolateScriptLib(t)
	if os.Getenv("GENNARRATED") == "" {
		t.Skip("đặt GENNARRATED=1 để render thật")
	}
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("không có ffmpeg")
	}
	tmp := t.TempDir()
	imgs := []string{
		makeColorImage(t, tmp, "0", "red", 1200, 800),
		makeColorImage(t, tmp, "1", "green", 800, 1200),
	}
	origScript, origTTS := genScript, synthTTS
	defer func() { genScript, synthTTS = origScript, origTTS }()
	genScript = func(cfg Config, images []string) (*NarrationScript, error) {
		return &NarrationScript{Segments: []NarrationSegment{
			{ImageIndex: 0, Caption: "A", Narration: "Chào các con vợ, phòng chỉ 199.000 đồng thôi nha."},
			{ImageIndex: 1, Caption: "B", Narration: "Giường êm ru quên lối về."},
		}}, nil
	}
	synthTTS = func(cfg Config, text string) ([]byte, error) { return silentMP3(t, tmp, 2.0), nil }
	os.Setenv("ELEVENLABS_API_KEY", "x")
	defer os.Unsetenv("ELEVENLABS_API_KEY")

	out := filepath.Join(tmp, "land.mp4")
	cfg := Config{
		Narrate: true, Mode: "kenburns", Output: out,
		Width: 1920, Height: 1080, FPS: 30, ZoomIntensity: 0.4, // landscape
		TTSProvider: "elevenlabs", NarrationPersona: "haihuoc",
		Template: "amorex", Watermark: `@x\y`, Nickname: `Căn \hộ Test`, // backslash injection
		Prices: []string{"Khung 2h: 199k"},
	}
	args, err := buildNarrated(cfg, imgs)
	if err != nil {
		t.Fatalf("buildNarrated: %v", err)
	}
	if err := runFFmpeg(args, cfg); err != nil {
		t.Fatalf("runFFmpeg (landscape narrate) vẫn lỗi: %v", err)
	}
	w, h := videoDims(t, out)
	if w != 1080 || h != 1920 {
		t.Errorf("output phải 1080x1920 (ép dọc), được %dx%d", w, h)
	}
}

func videoDims(t *testing.T, path string) (int, int) {
	t.Helper()
	out, err := exec.Command("ffprobe", "-v", "error", "-select_streams", "v:0",
		"-show_entries", "stream=width,height", "-of", "csv=p=0:s=x", path).Output()
	if err != nil {
		t.Fatalf("ffprobe dims: %v", err)
	}
	parts := strings.Split(strings.TrimSpace(string(out)), "x")
	if len(parts) != 2 {
		t.Fatalf("ffprobe dims lạ: %q", out)
	}
	w, _ := strconv.Atoi(parts[0])
	h, _ := strconv.Atoi(parts[1])
	return w, h
}

func TestWriteNarratedASSNoHookFallsBackClean(t *testing.T) {
	spec, dir := specFixture(t, "karaoke")
	spec.HookLine1, spec.HookLine2, spec.HookEmph = "", "", ""
	spec.Hearts = false
	assPath, _, err := writeNarratedASS(spec, dir)
	if err != nil {
		t.Fatalf("writeNarratedASS: %v", err)
	}
	content := readASS(t, assPath)
	if len(dialogueLines(content, "Hook")) != 0 {
		t.Error("không có hook text thì không được sinh event Hook")
	}
	if len(dialogueLines(content, "Heart")) != 0 {
		t.Error("Hearts=false thì không được sinh event Heart")
	}
	_ = filepath.Base(assPath)
}
