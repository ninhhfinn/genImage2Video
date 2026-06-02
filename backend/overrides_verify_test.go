package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestApplyConfigOverrides_ProducesDifferentOutput proves the UI knobs now
// actually change the rendered PNGs. We render the template-driven listing
// overlay under several setting combos and verify their PNG hashes differ.
func TestApplyConfigOverrides_ProducesDifferentOutput(t *testing.T) {
	base := Config{
		Width:    1080,
		Height:   1920,
		Template: "daiky",
		Address:  "39 P. Kẻ Vẽ, Đông Ngạc, Bắc Từ Liêm, Hà Nội",
		Nickname: "601",
		Prices:   []string{"2 ngày 1 đêm: 250.000đ", "Qua đêm: 75.000đ", "Giá giờ: 180.000đ"},
		Amenities: []string{
			"balcony", "knife and cutboard", "dinning table",
			"hot water kettle", "refrigerator", "cooking basics",
		},
		ListingID: "601",
	}

	cases := []struct {
		name string
		mut  func(*Config)
	}{
		{"baseline", func(c *Config) {}},
		{"font_lilita", func(c *Config) { c.OverlayFont = "lilita" }},
		{"font_playfair", func(c *Config) { c.OverlayFont = "playfair" }},
		{"scale_up", func(c *Config) { c.OverlayScale = 1.35 }},
		{"scale_down", func(c *Config) { c.OverlayScale = 0.75 }},
		{"color_yellow", func(c *Config) { c.OverlayText = "#FFD24A" }},
		{"title_yellow", func(c *Config) { c.TitleColor = "#FFD24A" }},
		{"style_badge_brown", func(c *Config) {
			c.OverlayStyle = "badge"
			c.OverlayBG = "#8C6149"
		}},
		{"style_badge_navy", func(c *Config) {
			c.OverlayStyle = "badge"
			c.OverlayBG = "#1F3A5F"
		}},
		{"style_bubble", func(c *Config) { c.OverlayStyle = "bubble" }},
	}

	dump := "/tmp/img2video_override_verify"
	_ = os.MkdirAll(dump, 0755)
	hashByElement := map[string]map[string]string{}

	for _, tc := range cases {
		cfg := base
		tc.mut(&cfg)
		plans, tmpDir, err := prepareTextOverlayPlans(cfg)
		if err != nil {
			t.Fatalf("[%s] prepare: %v", tc.name, err)
		}
		if len(plans) == 0 {
			t.Fatalf("[%s] no plans rendered", tc.name)
		}
		for _, p := range plans {
			key := strings.TrimSuffix(filepath.Base(p.PNGPath), ".png")
			data, err := os.ReadFile(p.PNGPath)
			if err != nil {
				t.Fatalf("[%s] read %s: %v", tc.name, key, err)
			}
			h := sha256.Sum256(data)
			sum := hex.EncodeToString(h[:8])
			if hashByElement[key] == nil {
				hashByElement[key] = map[string]string{}
			}
			hashByElement[key][tc.name] = sum
			out := filepath.Join(dump, fmt.Sprintf("%s_%s.png", key, tc.name))
			_ = os.WriteFile(out, data, 0644)
		}
		os.RemoveAll(tmpDir)
	}

	t.Logf("Hash matrix (first 16 hex of SHA256 per PNG):")
	for _, key := range []string{"nickname", "address", "prices", "amenities"} {
		hashes := hashByElement[key]
		if hashes == nil {
			continue
		}
		t.Logf("── %s", key)
		seen := map[string][]string{}
		for _, tc := range cases {
			sum, ok := hashes[tc.name]
			if !ok {
				continue
			}
			seen[sum] = append(seen[sum], tc.name)
			t.Logf("   %-22s %s", tc.name, sum)
		}
		baselineSum := hashes["baseline"]
		// Title (nickname) follows the title color control; body follows the
		// listing font/scale/color controls. Each must respond to its own knob.
		// Badge style is excluded (no-op on flat elements).
		var mustDiffer []string
		if key == "nickname" {
			mustDiffer = []string{"title_yellow"}
		} else {
			mustDiffer = []string{"font_lilita", "scale_up", "scale_down", "color_yellow"}
		}
		for _, name := range mustDiffer {
			got, ok := hashes[name]
			if !ok {
				continue
			}
			if got == baselineSum {
				t.Errorf("[%s] hash matches baseline for %s — override didn't take effect", key, name)
			}
		}
	}

	// daiky is two-tone: TitleBg re-colors the title pill, BodyBg re-colors the
	// address/prices/amenities pills, and the two are independent.
	daiky := base
	daiky.Template = "daiky"
	hashFor := func(plans []OverlayPlan, key string) string {
		for _, p := range plans {
			if strings.TrimSuffix(filepath.Base(p.PNGPath), ".png") == key {
				d, _ := os.ReadFile(p.PNGPath)
				h := sha256.Sum256(d)
				return hex.EncodeToString(h[:8])
			}
		}
		return ""
	}
	render := func(cfg Config) ([]OverlayPlan, string) {
		plans, tmpDir, err := prepareTextOverlayPlans(cfg)
		if err != nil {
			t.Fatalf("render: %v", err)
		}
		return plans, tmpDir
	}
	dBase, dBaseDir := render(daiky)
	defer os.RemoveAll(dBaseDir)
	dTitleBg, dTitleBgDir := render(func() Config { c := daiky; c.TitleBg = "#1F3A5F"; return c }())
	defer os.RemoveAll(dTitleBgDir)
	dBodyBg, dBodyBgDir := render(func() Config { c := daiky; c.BodyBg = "#1F3A5F"; return c }())
	defer os.RemoveAll(dBodyBgDir)

	// TitleBg changes the title pill but not the content pills.
	if a, b := hashFor(dBase, "nickname"), hashFor(dTitleBg, "nickname"); a == b {
		t.Errorf("daiky nickname should change when TitleBg re-colors its pill")
	} else {
		t.Logf("daiky nickname base=%s titleBg=%s (title pill re-color OK)", a, b)
	}
	if a, b := hashFor(dBase, "prices"), hashFor(dTitleBg, "prices"); a != b {
		t.Errorf("TitleBg must NOT affect content pills; prices base=%s titleBg=%s", a, b)
	}

	// BodyBg changes the content pills but not the title pill.
	if a, b := hashFor(dBase, "prices"), hashFor(dBodyBg, "prices"); a == b {
		t.Errorf("daiky prices should change when BodyBg re-colors its pill")
	} else {
		t.Logf("daiky prices base=%s bodyBg=%s (content pill re-color OK)", a, b)
	}
	if a, b := hashFor(dBase, "nickname"), hashFor(dBodyBg, "nickname"); a != b {
		t.Errorf("BodyBg must NOT affect title pill; nickname base=%s bodyBg=%s", a, b)
	}

	// Dump daiky baseline PNGs for visual comparison against target image.
	dumpDir := "/tmp/img2video_target_compare"
	_ = os.MkdirAll(dumpDir, 0755)
	for _, p := range dBase {
		key := strings.TrimSuffix(filepath.Base(p.PNGPath), ".png")
		data, _ := os.ReadFile(p.PNGPath)
		_ = os.WriteFile(filepath.Join(dumpDir, "daiky_"+key+".png"), data, 0644)
	}

	sunset := base
	sunset.Template = "sunset"
	sBase, sBaseDir := render(sunset)
	defer os.RemoveAll(sBaseDir)
	for _, p := range sBase {
		key := strings.TrimSuffix(filepath.Base(p.PNGPath), ".png")
		data, _ := os.ReadFile(p.PNGPath)
		_ = os.WriteFile(filepath.Join(dumpDir, "sunset_"+key+".png"), data, 0644)
	}
	t.Logf("Target-compare PNGs dumped to %s", dumpDir)

	t.Logf("PNG samples dumped to %s", dump)
}
