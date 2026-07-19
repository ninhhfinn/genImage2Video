package main

import (
	"net/http/httptest"
	"testing"
)

// Nghe thử KHÔNG được để browser cache: URL preview chỉ chứa provider+voice,
// không chứa model — nếu browser cache thì đổi model xong vẫn phát audio cũ
// (bug "Adam sửa rồi mà nghe thử vẫn đọc sai"). Server đã có cache đĩa riêng.
func TestTTSPreviewNoBrowserCache(t *testing.T) {
	old := synthTTS
	synthTTS = func(cfg Config, text string) ([]byte, error) { return []byte("ID3fakemp3"), nil }
	defer func() { synthTTS = old }()

	req := httptest.NewRequest("GET", "/api/tts-preview?provider=elevenlabs&voice=test-cache-hdr", nil)
	w := httptest.NewRecorder()
	ttsPreviewHandler()(w, req)

	if w.Code != 200 {
		t.Fatalf("HTTP %d: %s", w.Code, w.Body.String())
	}
	if cc := w.Header().Get("Cache-Control"); cc != "no-store" {
		t.Fatalf("Cache-Control = %q, muốn %q", cc, "no-store")
	}
}
