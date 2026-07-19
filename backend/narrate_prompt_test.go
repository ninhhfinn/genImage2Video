package main

import (
	"strings"
	"testing"
)

// ─── Prompt v4: công thức hook + ví dụ mẫu + tự kiểm ─────────────────────────

func TestPickHookFormulaDeterministic(t *testing.T) {
	a := pickHookFormula("L123")
	if strings.TrimSpace(a) == "" {
		t.Fatal("pickHookFormula rỗng")
	}
	if b := pickHookFormula("L123"); b != a {
		t.Errorf("cùng listing phải cùng công thức: %q != %q", a, b)
	}
	if pickHookFormula("") == "" {
		t.Error("listing rỗng vẫn phải trả công thức")
	}
	seen := map[string]bool{}
	for _, id := range []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "k", "m", "n"} {
		seen[pickHookFormula(id)] = true
	}
	if len(seen) < 3 {
		t.Errorf("12 listing chỉ ra %d công thức hook — phải đa dạng hơn", len(seen))
	}
}

func TestNarrationExamplesBlock(t *testing.T) {
	isolateScriptLib(t)
	hh := narrationExamplesBlock("haihuoc")
	if n := strings.Count(hh, "<example>"); n < 3 {
		t.Errorf("haihuoc cần ≥3 ví dụ mẫu, got %d", n)
	}
	if !strings.Contains(hh, "Chào các con vợ") {
		t.Error("ví dụ haihuoc phải mở bằng 'Chào các con vợ'")
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
				t.Errorf("ví dụ %s #%d cần ≥3 đoạn (hook/thân/CTA), got %d", persona, i, len(s.Segments))
			}
			for _, seg := range s.Segments {
				if digitWarn(seg.Narration) || digitWarn(seg.Caption) {
					t.Errorf("ví dụ %s #%d có chữ số trong lời kể/caption: %q", persona, i, seg.Narration)
				}
			}
		}
	}
}

func TestNarrationFullSystemPromptV4(t *testing.T) {
	isolateScriptLib(t)
	cfg := Config{ListingID: "L9", NarrationPersona: "haihuoc"}
	p := narrationFullSystemPrompt(cfg, 13)
	if !strings.Contains(p, pickHookFormula("L9")) {
		t.Error("prompt phải chứa công thức hook được giao cho listing")
	}
	for _, want := range []string{"Chào các con vợ", "TỰ KIỂM", "<example>", "lưu video"} {
		if !strings.Contains(p, want) {
			t.Errorf("prompt haihuoc thiếu %q", want)
		}
	}
	lp := narrationFullSystemPrompt(Config{ListingID: "L9", NarrationPersona: "lichsu"}, 13)
	if strings.Contains(lp, "con vợ") {
		t.Error("prompt lichsu không được lẫn 'con vợ'")
	}
	if !strings.Contains(lp, "TỰ KIỂM") || !strings.Contains(lp, "<example>") {
		t.Error("prompt lichsu vẫn phải có khối TỰ KIỂM + ví dụ mẫu")
	}
	if narrationPromptVersion == "v3" || narrationPromptVersion == "v4" {
		t.Errorf("đã sửa prompt (hook title v5) — phải bump narrationPromptVersion khỏi %s", narrationPromptVersion)
	}
}

// ─── Prompt v5: hook title 2 dòng hiện trên màn hình ─────────────────────────

func TestNarrationPromptV5HookTitle(t *testing.T) {
	isolateScriptLib(t)
	p := narrationFullSystemPrompt(Config{ListingID: "L9", NarrationPersona: "haihuoc"}, 13)
	for _, want := range []string{"hook_line1", "hook_line2", "hook_emphasis"} {
		if !strings.Contains(p, want) {
			t.Errorf("prompt v5 thiếu hướng dẫn %q", want)
		}
	}
	// Rule chữ số phải được scope: hook_line ĐƯỢC dùng chữ số.
	if !strings.Contains(p, "hook_line ĐƯỢC dùng chữ số") && !strings.Contains(p, "hook_line được dùng chữ số") {
		t.Error("prompt v5 phải nói rõ hook_line được dùng chữ số (khác narration)")
	}
	// Schema structured-output phải khai báo 3 field hook.
	sc := narrationSchema()
	props, _ := sc["properties"].(map[string]any)
	for _, k := range []string{"hook_line1", "hook_line2", "hook_emphasis"} {
		if _, ok := props[k]; !ok {
			t.Errorf("narrationSchema thiếu property %q", k)
		}
	}
}

func TestValidateScriptHookFields(t *testing.T) {
	s := &NarrationScript{
		Segments: []NarrationSegment{
			{ImageIndex: 0, Caption: "Phòng", Narration: "Chào các con vợ, căn này xinh lắm."},
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
	// Emphasis không nằm trong dòng nào → phải bị xoá.
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
