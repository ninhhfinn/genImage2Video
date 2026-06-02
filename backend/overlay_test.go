package main

import (
	"os"
	"testing"
)

func TestRenderListingOverlayPNG_Badge(t *testing.T) {
	cfg := Config{
		Width:        1080,
		Height:       1920,
		Address:      "Ngõ 171 Nguyễn Ngọc Vũ",
		Nickname:     "Daiky home",
		Prices:       []string{"2h 249k", "4h 367k", "Qua đêm 449k"},
		OverlayStyle: "badge",
	}
	path, err := renderListingOverlayPNG(cfg)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if path == "" {
		t.Fatal("expected non-empty path")
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Size() < 1000 {
		t.Errorf("PNG too small: %d bytes", info.Size())
	}
	os.MkdirAll("/tmp/img2video_smoke", 0755)
	data, _ := os.ReadFile(path)
	os.WriteFile("/tmp/img2video_smoke/overlay_badge.png", data, 0644)
	t.Logf("→ /tmp/img2video_smoke/overlay_badge.png")
}

func TestRenderListingOverlayPNG_Bubble(t *testing.T) {
	cfg := Config{
		Width:        1080,
		Height:       1920,
		Nickname:     "Sunset",
		Address:      "Tại 83C Pháo Đài Láng",
		Prices:       []string{"2h 249k", "4h 367k", "Qua đêm 449k"},
		Amenities:    []string{"Máy chiếu netflix", "Bếp nấu", "Cửa sổ thoáng", "Gương checkin", "Wc khép kín", "Gửi xe free"},
		OverlayStyle: "bubble",
	}
	path, err := renderListingOverlayPNG(cfg)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("stat: %v", err)
	}
	data, _ := os.ReadFile(path)
	os.MkdirAll("/tmp/img2video_smoke", 0755)
	os.WriteFile("/tmp/img2video_smoke/overlay_bubble.png", data, 0644)
	t.Logf("→ /tmp/img2video_smoke/overlay_bubble.png")
}

func TestRenderListingOverlayPNG_Empty(t *testing.T) {
	cfg := Config{Width: 1080, Height: 1920}
	path, err := renderListingOverlayPNG(cfg)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if path != "" {
		t.Errorf("expected empty path when no listing data, got %q", path)
	}
}
