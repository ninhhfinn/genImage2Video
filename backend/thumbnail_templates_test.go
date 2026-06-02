package main

import (
	"fmt"
	"os"
	"testing"
)

// TestRenderGridTemplates renders all 4 grid templates with real room photos to
// /tmp for visual comparison against the approved mockup.
func TestRenderGridTemplates(t *testing.T) {
	photoset := func(start int) []string {
		var p []string
		for i := 0; i < 4; i++ {
			fp := fmt.Sprintf("../mockphotos/p%02d.jpg", start+i)
			if _, err := os.Stat(fp); err != nil {
				t.Skipf("mockphotos không có (%s) — bỏ qua render verify", fp)
			}
			p = append(p, fp)
		}
		return p
	}

	table := []ThumbPriceRow{
		{"2h đầu", "249k", "299k"}, {"10h-19h", "549k", "699k"},
		{"Qua đêm 22h-9h", "449k", "549k"}, {"Ngày đêm 14h-11h", "549k", "699k"},
		{"Đêm ngày 22h-19h", "699k", "899k"},
	}
	amen := []string{"Máy chiếu netflix", "Bếp nấu", "Gương checkin", "Wc khép kín", "Gửi xe free"}

	cases := []struct {
		tmpl  string
		cfg   ThumbnailConfig
		start int
	}{
		{"daiky", ThumbnailConfig{Title: "Daiky home", Address: "Ngõ 171 Nguyễn Ngọc Vũ",
			Prices: []string{"2h 249k", "4h 367k", "Qua đêm 449k"}, Watermark: "@tranhouse_hanoi"}, 0},
		{"valey", ThumbnailConfig{Title: "Valey", PriceTable: table, Watermark: "@tranhouse_hanoi"}, 4},
		{"peony", ThumbnailConfig{Title: "Peony", Address: "294 Mỹ Đình",
			Prices: []string{"2h 249k-4h 367k- Qua đêm 449k"}, Amenities: amen, Watermark: "@tranhouse_hanoi"}, 8},
		{"tiger", ThumbnailConfig{Title: "Tiger TL3", Address: "110 P. Trung Liệt, Trung Liệt, Đống Đa, Hà Nội",
			Prices: []string{"Combo: 278,000", "Qua đêm: 449,100"}, ListingID: "lsomdqgwni", Watermark: "@hen.ho.101"}, 12},
	}

	for _, c := range cases {
		cfg := c.cfg
		cfg.Template = c.tmpl
		data, err := buildThumbnailImage(cfg, photoset(c.start))
		if err != nil {
			t.Fatalf("%s: %v", c.tmpl, err)
		}
		out := "/tmp/thumb_" + c.tmpl + ".jpg"
		if err := os.WriteFile(out, data, 0644); err != nil {
			t.Fatal(err)
		}
		t.Logf("%-6s → %s (%d KB)", c.tmpl, out, len(data)/1024)
	}
}
