package main

import (
	"os"
	"testing"
)

// TestThumbnailVisual renders a sample collage to /tmp so it can be eyeballed
// against the approved HTML mockup. Uses the repo-root mock images.
func TestThumbnailVisual(t *testing.T) {
	photos := []string{
		"../clean-bg.png",            // hero (clean bedroom)
		"../mock-assets/pol-screen.jpg",
		"../mock-assets/pol-kitchen.jpg",
		"../mock-assets/pol-dining.jpg",
	}
	for _, p := range photos {
		if _, err := os.Stat(p); err != nil {
			t.Skipf("missing sample image %s: %v", p, err)
		}
	}

	cfg := ThumbnailConfig{
		Title:     "Sunset",
		Address:   "83C Pháo Đài Láng",
		Prices:    []string{"2h 249k", "4h 367k", "Qua đêm 449k"},
		Amenities: []string{"Máy chiếu netflix", "Bếp nấu", "Cửa sổ thoáng", "Gương checkin", "Wc khép kín", "Gửi xe free"},
		Watermark: "@tranhouse_hanoi",
	}

	data, err := buildThumbnailImage(cfg, photos)
	if err != nil {
		t.Fatalf("buildThumbnailImage: %v", err)
	}
	out := "/tmp/thumb-test.jpg"
	if err := os.WriteFile(out, data, 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	t.Logf("wrote %s (%d bytes)", out, len(data))
}
