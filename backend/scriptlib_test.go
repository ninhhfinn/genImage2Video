package main

import (
	"strings"
	"testing"
)

// ─── Thư viện kịch bản + feedback (D) ────────────────────────────────────────

// isolateScriptLib trỏ thư viện kịch bản sang thư mục tạm cho test — vừa
// hermetic (không dính library.json thật trên máy dev) vừa không ghi rác vào
// backend/assets khi test render/buildNarrated tự lưu entry.
func isolateScriptLib(t *testing.T) {
	t.Helper()
	old := scriptLibDirOverride
	scriptLibDirOverride = t.TempDir()
	t.Cleanup(func() { scriptLibDirOverride = old })
}

func libTestScript(opening string) *NarrationScript {
	return &NarrationScript{Segments: []NarrationSegment{
		{ImageIndex: 0, Caption: "Mở đầu", Narration: opening},
		{ImageIndex: 1, Caption: "Góc bếp", Narration: "Bếp xinh lắm nha mấy con vợ"},
		{ImageIndex: 2, Caption: "Ban công", Narration: "Ban công chill hết nước chấm"},
		{ImageIndex: 3, Caption: "Chốt phòng", Narration: "Inbox tôi giữ phòng liền tay"},
	}}
}

func TestScriptLibRoundTrip(t *testing.T) {
	isolateScriptLib(t)

	if got := loadScriptLib(); len(got) != 0 {
		t.Fatalf("thư viện mới phải rỗng, got %d", len(got))
	}

	cfg := Config{ListingID: "L1", Nickname: "Căn Test", NarrationPersona: "haihuoc"}
	saveScriptEntry(cfg, libTestScript("Chào các con vợ, căn Căn Test giá sốc nè"), false)
	entries := loadScriptLib()
	if len(entries) != 1 || entries[0].Edited {
		t.Fatalf("phải có 1 entry chưa edited, got %+v", entries)
	}

	// Lưu lại y hệt → dedup, vẫn 1 entry.
	saveScriptEntry(cfg, libTestScript("Chào các con vợ, căn Căn Test giá sốc nè"), false)
	if entries = loadScriptLib(); len(entries) != 1 {
		t.Fatalf("kịch bản trùng phải dedup, got %d", len(entries))
	}

	// Bản user sửa tay → entry mới, edited=true.
	saveScriptEntry(cfg, libTestScript("Chào các con vợ, bản đã sửa tay nè"), true)
	entries = loadScriptLib()
	if len(entries) != 2 {
		t.Fatalf("bản sửa khác nội dung phải thành entry mới, got %d", len(entries))
	}

	// Like entry đầu tiên → thành ví dụ few-shot cho persona khớp.
	var firstID string
	for _, e := range entries {
		if !e.Edited {
			firstID = e.ID
		}
	}
	if err := setScriptLiked(firstID, true); err != nil {
		t.Fatalf("setScriptLiked lỗi: %v", err)
	}
	exs := likedScriptExamples("haihuoc", 3)
	if len(exs) != 1 {
		t.Fatalf("phải có 1 ví dụ liked, got %d", len(exs))
	}
	if !strings.Contains(exs[0], "giá sốc") {
		t.Errorf("ví dụ liked phải chứa lời kể gốc, got %s", exs[0])
	}
	s, err := parseScriptJSON(exs[0])
	if err != nil {
		t.Fatalf("ví dụ liked phải là JSON kịch bản hợp lệ: %v", err)
	}
	// Kịch bản dài được nén còn tối đa 3 đoạn (hook/thân/CTA) cho đỡ tốn prompt,
	// image_index đánh lại 0..n để mẫu tự nhất quán.
	if len(s.Segments) > 3 {
		t.Errorf("ví dụ liked phải nén ≤3 đoạn, got %d", len(s.Segments))
	}
	for i, seg := range s.Segments {
		if seg.ImageIndex != i {
			t.Errorf("image_index phải đánh lại liên tục, segment %d có index %d", i, seg.ImageIndex)
		}
	}
	if got := likedScriptExamples("lichsu", 3); len(got) != 0 {
		t.Errorf("persona khác không được ăn ví dụ haihuoc, got %d", len(got))
	}

	// Xoá entry đã sửa.
	var editedID string
	for _, e := range loadScriptLib() {
		if e.Edited {
			editedID = e.ID
		}
	}
	if err := deleteScriptEntry(editedID); err != nil {
		t.Fatalf("deleteScriptEntry lỗi: %v", err)
	}
	if entries = loadScriptLib(); len(entries) != 1 {
		t.Fatalf("sau xoá phải còn 1 entry, got %d", len(entries))
	}
}

func TestExamplesHashChangesWithLiked(t *testing.T) {
	isolateScriptLib(t)

	cfg := Config{ListingID: "L1", NarrationPersona: "haihuoc", Nickname: "Test"}
	hashes := []string{"aaa", "bbb"}
	h1 := narrationExamplesHash("haihuoc")
	k1 := scriptCacheKey(cfg, hashes)
	block1 := narrationExamplesBlock("haihuoc")

	saveScriptEntry(cfg, libTestScript("Chào các con vợ, mở đầu độc nhất vô nhị nè"), false)
	id := loadScriptLib()[0].ID
	if err := setScriptLiked(id, true); err != nil {
		t.Fatalf("like lỗi: %v", err)
	}

	if h2 := narrationExamplesHash("haihuoc"); h2 == h1 {
		t.Error("like kịch bản mới phải đổi examples hash")
	}
	if k2 := scriptCacheKey(cfg, hashes); k2 == k1 {
		t.Error("examples đổi phải đổi cache key (không thì render dùng kịch bản cache theo prompt cũ)")
	}
	if block2 := narrationExamplesBlock("haihuoc"); block2 == block1 || !strings.Contains(block2, "độc nhất vô nhị") {
		t.Error("ví dụ liked phải xuất hiện trong examples block")
	}
}
