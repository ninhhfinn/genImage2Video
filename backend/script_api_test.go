package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// ─── Preview/sửa kịch bản trước render (C) ───────────────────────────────────

func TestCachedScriptUnlessForced(t *testing.T) {
	key := "unitforce0001"
	t.Cleanup(func() { os.Remove(scriptCachePath(key)) })
	saveCachedScript(key, &NarrationScript{Segments: []NarrationSegment{
		{ImageIndex: 0, Caption: "x", Narration: "một hai ba"},
	}})
	if _, ok := cachedScriptUnlessForced(Config{}, key); !ok {
		t.Fatal("không force phải ăn cache")
	}
	if _, ok := cachedScriptUnlessForced(Config{ForceScript: true}, key); ok {
		t.Fatal("ForceScript phải bỏ qua cache (nút Viết lại)")
	}
}

func TestScriptForConfigUsesProvided(t *testing.T) {
	orig := genScript
	defer func() { genScript = orig }()

	// 1) cfg.Script có sẵn → dùng luôn, KHÔNG gọi Claude; segment lệch index bị loại.
	genScript = func(cfg Config, images []string) (*NarrationScript, error) {
		t.Fatal("không được gọi genScript khi đã có cfg.Script")
		return nil, nil
	}
	cfg := Config{Script: &NarrationScript{Segments: []NarrationSegment{
		{ImageIndex: 0, Caption: "Mở đầu", Narration: "Chào các con vợ, căn Test nè"},
		{ImageIndex: 99, Caption: "rác", Narration: "ngoài phạm vi"},
	}}}
	s, err := scriptForConfig(cfg, []string{"a.jpg", "b.jpg"})
	if err != nil {
		t.Fatalf("scriptForConfig lỗi: %v", err)
	}
	if len(s.Segments) != 1 {
		t.Fatalf("segment index 99 phải bị loại, còn %d segment", len(s.Segments))
	}

	// 2) Script gửi lên toàn segment hỏng → lỗi rõ ràng.
	cfgBad := Config{Script: &NarrationScript{Segments: []NarrationSegment{{ImageIndex: 5, Narration: "x"}}}}
	if _, err := scriptForConfig(cfgBad, []string{"a.jpg"}); err == nil {
		t.Fatal("script không có cảnh hợp lệ phải trả lỗi")
	}

	// 3) Không có cfg.Script → đi qua genScript như cũ.
	called := false
	genScript = func(cfg Config, images []string) (*NarrationScript, error) {
		called = true
		return &NarrationScript{Segments: []NarrationSegment{{ImageIndex: 0, Narration: "ok nè"}}}, nil
	}
	if _, err := scriptForConfig(Config{}, []string{"a.jpg"}); err != nil || !called {
		t.Fatalf("phải gọi genScript khi không có script: err=%v called=%v", err, called)
	}
}

func TestCapNarratedImages(t *testing.T) {
	imgs := make([]string, 15)
	for i := range imgs {
		imgs[i] = fmt.Sprintf("%d.jpg", i)
	}
	if got := capNarratedImages(Config{}, imgs); len(got) != 10 {
		t.Errorf("mặc định phải cap 10 ảnh, got %d", len(got))
	}
	// 60s giọng google chỉ đủ 8 cảnh (khớp TestNarrationBudget).
	if got := capNarratedImages(Config{MaxSegments: 12, TargetDuration: 60, TTSProvider: "google"}, imgs); len(got) != 8 {
		t.Errorf("60s google phải cap 8 ảnh, got %d", len(got))
	}
}

func TestScriptPreviewHandler(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "dummy") // qua check nguồn Claude dù không có CLI
	up := t.TempDir()
	os.WriteFile(filepath.Join(up, "0001.jpg"), []byte("img-a"), 0644)
	os.WriteFile(filepath.Join(up, "0002.jpg"), []byte("img-b"), 0644)

	orig := genScript
	defer func() { genScript = orig }()
	var gotCfg Config
	var gotImgs int
	genScript = func(cfg Config, images []string) (*NarrationScript, error) {
		gotCfg = cfg
		gotImgs = len(images)
		return &NarrationScript{Segments: []NarrationSegment{
			{ImageIndex: 0, Caption: "Mở đầu", Narration: "Chào các con vợ nè"},
			{ImageIndex: 1, Caption: "Chốt phòng", Narration: "Chốt đơn lẹ nha"},
		}}, nil
	}

	body, _ := json.Marshal(map[string]any{
		"narration_persona": "haihuoc",
		"tts_provider":      "google",
		"target_duration":   60,
		"max_segments":      10,
		"nickname":          "Căn Test",
		"listing_id":        "L1",
		"force":             true,
	})
	rr := httptest.NewRecorder()
	scriptPreviewHandler(up)(rr, httptest.NewRequest("POST", "/api/script", bytes.NewReader(body)))
	if rr.Code != 200 {
		t.Fatalf("status %d: %s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Segments []NarrationSegment `json:"segments"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("response không phải JSON: %v", err)
	}
	if len(resp.Segments) != 2 {
		t.Fatalf("cần 2 đoạn, got %d", len(resp.Segments))
	}
	if !gotCfg.ForceScript {
		t.Error("force=true phải truyền vào cfg.ForceScript")
	}
	if gotCfg.Nickname != "Căn Test" || gotCfg.ListingID != "L1" {
		t.Errorf("thiếu dữ liệu listing trong cfg: nickname=%q listing=%q", gotCfg.Nickname, gotCfg.ListingID)
	}
	if gotImgs != 2 {
		t.Errorf("phải đưa 2 ảnh vào genScript, got %d", gotImgs)
	}

	rr2 := httptest.NewRecorder()
	scriptPreviewHandler(up)(rr2, httptest.NewRequest("GET", "/api/script", nil))
	if rr2.Code != 405 {
		t.Errorf("GET phải trả 405, got %d", rr2.Code)
	}
}
