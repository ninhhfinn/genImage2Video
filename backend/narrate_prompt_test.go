package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ─── Prompt v6: 4 khung hook + 6 yếu tố + ví dụ mẫu + tự kiểm (SPEC Dayladau) ─

func TestPickHookFormulaDeterministic(t *testing.T) {
	a := pickHookFormula("L123")
	if strings.TrimSpace(a) == "" {
		t.Fatal("pickHookFormula rỗng")
	}
	if b := pickHookFormula("L123"); b != a {
		t.Errorf("cùng listing phải cùng khung: %q != %q", a, b)
	}
	if pickHookFormula("") == "" {
		t.Error("listing rỗng vẫn phải trả khung")
	}
	// 4 khung A–D: nhiều listing phải rơi vào ≥2 khung khác nhau (deterministic
	// nhưng đa dạng giữa video).
	seen := map[string]bool{}
	for _, id := range []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "k", "m", "n"} {
		seen[pickHookFormula(id)] = true
	}
	if len(seen) < 2 {
		t.Errorf("12 listing chỉ ra %d khung hook — phải đa dạng hơn", len(seen))
	}
}

func TestPickCTAStyleDeterministic(t *testing.T) {
	if pickCTAStyle("L1") != pickCTAStyle("L1") {
		t.Error("cùng listing phải cùng kiểu CTA")
	}
	if strings.TrimSpace(pickCTAStyle("L1")) == "" {
		t.Error("pickCTAStyle rỗng")
	}
}

func TestNarrationExamplesBlock(t *testing.T) {
	isolateScriptLib(t)
	hh := narrationExamplesBlock("haihuoc")
	if n := strings.Count(hh, "<example>"); n < 2 {
		t.Errorf("haihuoc cần ≥2 ví dụ mẫu (SPEC §5), got %d", n)
	}
	if !strings.Contains(hh, "các vợ") {
		t.Error("ví dụ haihuoc phải có giọng gọi tệp 'các vợ'")
	}
	// Không được còn giọng "tôi/con vợ" đời cũ trong ví dụ v6 (persona đã đổi sang "anh").
	if strings.Contains(hh, "Chào các con vợ") {
		t.Error("ví dụ v6 không được mở bằng 'Chào các con vợ' (đã đổi persona sang 'anh')")
	}
	ls := narrationExamplesBlock("lichsu")
	if n := strings.Count(ls, "<example>"); n < 1 {
		t.Errorf("lichsu cần ≥1 ví dụ mẫu, got %d", n)
	}
	if strings.Contains(ls, "con vợ") {
		t.Error("ví dụ lichsu không được lẫn giọng 'con vợ'")
	}
	// Lời kể trong mẫu phải sạch chữ số (TTS đọc thành tiếng) — JSON scaffolding
	// (image_index) thì được phép có số.
	for _, persona := range []string{"haihuoc", "lichsu"} {
		for i, ex := range narrationStaticExamples(persona) {
			s, err := parseScriptJSON(ex)
			if err != nil {
				t.Fatalf("ví dụ %s #%d không phải JSON kịch bản hợp lệ: %v", persona, i, err)
			}
			if len(s.Segments) < 3 {
				t.Errorf("ví dụ %s #%d cần ≥3 đoạn, got %d", persona, i, len(s.Segments))
			}
			for _, seg := range s.Segments {
				if digitWarn(seg.Narration) || digitWarn(seg.Caption) {
					t.Errorf("ví dụ %s #%d có chữ số trong lời kể/caption: %q", persona, i, seg.Narration)
				}
			}
			if digitWarn(s.IntroNarration) {
				t.Errorf("ví dụ %s #%d có chữ số trong intro_narration: %q", persona, i, s.IntroNarration)
			}
			// Không lẫn từ cấm trong ví dụ mẫu.
			if hits := scriptBannedHits(s); len(hits) > 0 {
				t.Errorf("ví dụ %s #%d chứa từ cấm: %v", persona, i, hits)
			}
		}
	}
}

func TestNarrationFullSystemPromptV6(t *testing.T) {
	isolateScriptLib(t)
	cfg := Config{ListingID: "L9", NarrationPersona: "haihuoc"}
	p := narrationFullSystemPrompt(cfg, 13)
	if !strings.Contains(p, pickHookFormula("L9")) {
		t.Error("prompt phải chứa khung hook được giao cho listing")
	}
	for _, want := range []string{
		`Xưng "anh"`, "SÁU YẾU TỐ BẮT BUỘC", "TIỆN ÍCH ANH HÙNG", "ĐẶT THEO GIỜ",
		"TỰ KIỂM", "<example>", "vcl", "hân hạnh", "chia sẻ video",
	} {
		if !strings.Contains(p, want) {
			t.Errorf("prompt haihuoc v6 thiếu %q", want)
		}
	}
	// Tệp couple (mặc định) → gọi "các vợ"; genz → "các em".
	if !strings.Contains(p, "các vợ") {
		t.Error("prompt couple phải gọi 'các vợ'")
	}
	genz := narrationFullSystemPrompt(Config{ListingID: "L9", NarrationPersona: "haihuoc", Audience: "genz"}, 13)
	if !strings.Contains(genz, "các em") {
		t.Error("prompt genz phải gọi 'các em'")
	}

	lp := narrationFullSystemPrompt(Config{ListingID: "L9", NarrationPersona: "lichsu"}, 13)
	if strings.Contains(lp, "con vợ") {
		t.Error("prompt lichsu không được lẫn 'con vợ'")
	}
	if !strings.Contains(lp, "TỰ KIỂM") || !strings.Contains(lp, "<example>") {
		t.Error("prompt lichsu vẫn phải có khối TỰ KIỂM + ví dụ mẫu")
	}
	// Đã sửa prompt lên v6 — phải bump khỏi các version cũ.
	for _, old := range []string{"v3", "v4", "v5"} {
		if narrationPromptVersion == old {
			t.Errorf("đã sửa prompt (v6) — phải bump narrationPromptVersion khỏi %s", narrationPromptVersion)
		}
	}
}

func TestNarrationPromptHookTitle(t *testing.T) {
	isolateScriptLib(t)
	p := narrationFullSystemPrompt(Config{ListingID: "L9", NarrationPersona: "haihuoc"}, 13)
	for _, want := range []string{"hook_line1", "hook_line2", "hook_emphasis"} {
		if !strings.Contains(p, want) {
			t.Errorf("prompt thiếu hướng dẫn %q", want)
		}
	}
	if !strings.Contains(p, "hook_line ĐƯỢC dùng chữ số") {
		t.Error("prompt phải nói rõ hook_line được dùng chữ số (khác narration)")
	}
	// Schema structured-output khai báo 3 field hook + intro.
	sc := narrationSchema(false)
	props, _ := sc["properties"].(map[string]any)
	for _, k := range []string{"hook_line1", "hook_line2", "hook_emphasis", "intro_narration", "intro_caption"} {
		if _, ok := props[k]; !ok {
			t.Errorf("narrationSchema thiếu property %q", k)
		}
	}
	// wantIntro=true → intro_narration bắt buộc.
	req, _ := narrationSchema(true)["required"].([]string)
	if !contains(req, "intro_narration") {
		t.Error("narrationSchema(true) phải require intro_narration")
	}
	if req0, _ := narrationSchema(false)["required"].([]string); contains(req0, "intro_narration") {
		t.Error("narrationSchema(false) KHÔNG được require intro_narration")
	}
}

// ─── Intro cảnh đi đường: guidance chỉ xuất hiện khi bật intro + có asset ─────

func TestNarrationIntroGuidance(t *testing.T) {
	isolateScriptLib(t)
	// Tạo asset intro tạm để introEnabled=true.
	tmp := filepath.Join(t.TempDir(), "street.mp4")
	if err := os.WriteFile(tmp, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	old := introAssetOverride
	introAssetOverride = tmp
	defer func() { introAssetOverride = old }()

	on := narrationFullSystemPrompt(Config{ListingID: "L9", NarrationPersona: "haihuoc", Narrate: true, IntroClip: true}, 13)
	if !strings.Contains(on, "intro_narration") || !strings.Contains(on, "CẢNH MỞ ĐẦU LÀ VIDEO") {
		t.Error("bật intro: prompt phải có hướng dẫn viết intro_narration")
	}
	off := narrationFullSystemPrompt(Config{ListingID: "L9", NarrationPersona: "haihuoc", Narrate: true, IntroClip: false}, 13)
	if strings.Contains(off, "CẢNH MỞ ĐẦU LÀ VIDEO") {
		t.Error("tắt intro: prompt KHÔNG được có hướng dẫn cảnh đi đường")
	}
	if !strings.Contains(off, "ẢNH ĐẦU TIÊN chính là HOOK") {
		t.Error("tắt intro: prompt phải nói ảnh đầu tiên là hook")
	}
}

// ─── validateScript giữ hook + intro fields ──────────────────────────────────

func TestValidateScriptHookFields(t *testing.T) {
	s := &NarrationScript{
		Segments: []NarrationSegment{
			{ImageIndex: 0, Caption: "Phòng", Narration: "Căn này xinh lắm nha các vợ."},
		},
		HookLine1:    "  Căn hộ Camellia siêu xinh giữa lòng Đà Lạt mộng mơ luôn nè  ",
		HookLine2:    "Chỉ 199k/2 người",
		HookEmphasis: "199k/2 người",
	}
	out := validateScript(s, 1, 13, "")
	if got := len([]rune(out.HookLine1)); got > 28 {
		t.Errorf("HookLine1 phải bị cắt ≤28 rune, còn %d: %q", got, out.HookLine1)
	}
	if out.HookLine2 != "Chỉ 199k/2 người" {
		t.Errorf("HookLine2 bị đổi: %q", out.HookLine2)
	}
	if out.HookEmphasis != "199k/2 người" {
		t.Errorf("emphasis hợp lệ phải giữ nguyên: %q", out.HookEmphasis)
	}
	s2 := &NarrationScript{
		Segments:     s.Segments,
		HookLine1:    "Căn hộ Camellia",
		HookLine2:    "Đẹp mê ly",
		HookEmphasis: "999k",
	}
	if out2 := validateScript(s2, 1, 13, ""); out2.HookEmphasis != "" {
		t.Errorf("emphasis lạc dòng phải bị xoá, còn %q", out2.HookEmphasis)
	}
}

func TestValidateScriptKeepsIntro(t *testing.T) {
	s := &NarrationScript{
		Segments:       []NarrationSegment{{ImageIndex: 0, Caption: "Cửa", Narration: "Check-in tự động không gặp ai."}},
		IntroNarration: "Thôi dẹp nhà nghỉ tối om đi các vợ ơi.",
		IntroCaption:   "Đi đường",
	}
	out := validateScript(s, 1, 13, "")
	if out.IntroNarration != s.IntroNarration {
		t.Errorf("intro_narration phải sống sót validateScript, got %q", out.IntroNarration)
	}
	if out.IntroCaption != "Đi đường" {
		t.Errorf("intro_caption phải sống sót, got %q", out.IntroCaption)
	}
}

// ─── Kiểm tra từ cấm theo biên từ ────────────────────────────────────────────

func TestScriptBannedHits(t *testing.T) {
	// "vl" trong "vlog" KHÔNG được tính là từ cấm.
	if containsBannedWord("mình làm vlog du lịch", "vl") {
		t.Error("'vl' không được khớp bên trong 'vlog'")
	}
	if !containsBannedWord("cái này vl luôn", "vl") {
		t.Error("'vl' đứng riêng phải bị bắt")
	}
	s := &NarrationScript{
		Segments:  []NarrationSegment{{ImageIndex: 0, Narration: "Dayladau hân hạnh giới thiệu căn này vcl luôn."}},
		HookLine1: "Căn xịn",
	}
	hits := scriptBannedHits(s)
	if !contains(hits, "hân hạnh") || !contains(hits, "vcl") {
		t.Errorf("phải bắt 'hân hạnh' + 'vcl', got %v", hits)
	}
	// Kịch bản sạch → không hit.
	clean := &NarrationScript{Segments: []NarrationSegment{{ImageIndex: 0, Narration: "Căn này xinh xỉu, book lẹ đi các vợ."}}}
	if hits := scriptBannedHits(clean); len(hits) > 0 {
		t.Errorf("kịch bản sạch không được có hit, got %v", hits)
	}
}

func contains(xs []string, v string) bool {
	for _, x := range xs {
		if x == v {
			return true
		}
	}
	return false
}
