package main

// Vòng lặp render→so sánh cho CẢ 8 template: kiểm tra bảng giá mỗi template khớp
// y hệt mockup Canva (nhãn + đơn vị). Render overlay (no-veil) trên nền tối, ghép
// 8 panel thành 1 montage. Output ../scratch/alltmpl/.
//   ALLTMPL=1 go test -count=1 -timeout 300s -run TestAllTemplatePrices .

import (
	"image"
	"image/color"
	"os"
	"path/filepath"
	"testing"

	"github.com/disintegration/imaging"
)

func TestAllTemplatePrices(t *testing.T) {
	if os.Getenv("ALLTMPL") == "" {
		t.Skip("set ALLTMPL=1")
	}
	const W, H = 1080, 1920
	data, err := os.ReadFile("testdata/api_thanhxuan.json")
	if err != nil {
		t.Fatal(err)
	}
	infos := parseListings(data)
	var in ListingInfo
	for _, x := range infos {
		if x.ID == "lss5wnncbl" {
			in = x
			break
		}
	}
	if in.ID == "" {
		t.Fatal("không tìm thấy VTP501")
	}
	t.Logf("=== VTP501 bảng giá theo template ===")
	for _, tpl := range allTemplateNames {
		t.Logf("  %-11s → %v", tpl, in.PriceLinesByTemplate[tpl])
	}

	outDir := "../scratch/alltmpl"
	os.MkdirAll(outDir, 0o755)

	// Thu nhỏ mỗi panel để ghép (thumb width 360).
	const tw, th = 360, 640
	montage := imaging.New(tw*4+30, th*2+10, color.NRGBA{18, 18, 22, 255})
	amen := "Máy chiếu Netflix, bồn tắm, boardgame"

	for i, tpl := range allTemplateNames {
		cfg := Config{
			Template: tpl, Width: W, Height: H,
			Nickname:  in.Nickname,
			Address:   in.Address,
			Prices:    in.PriceLinesByTemplate[tpl],
			Amenities: []string{amen},
		}
		if tpl == "editorial" {
			cfg.Watermark = "Dayladau House"
		}
		// Mirror production (renderVideoTemplateComposite): nén giá + dọn địa chỉ.
		cfg.Prices = compactVideoPrices(cfg.Prices)
		cfg.Address = cleanVideoAddress(cfg.Address)
		body := chromeTemplateBody(tpl, cfg)
		if body == "" {
			t.Fatalf("%s: body rỗng", tpl)
		}
		tmp := t.TempDir()
		htmlPath := filepath.Join(tmp, "o.html")
		os.WriteFile(htmlPath, []byte(chromePageNoVeil(cfg, body)), 0o644)
		png := filepath.Join(tmp, "o.png")
		if err := runChromeShot(t, htmlPath, png, W, H); err != nil {
			t.Fatalf("%s: %v", tpl, err)
		}
		ov, err := imaging.Open(png)
		if err != nil {
			t.Fatal(err)
		}
		dark := imaging.New(W, H, color.NRGBA{44, 30, 40, 255})
		full := imaging.Overlay(dark, ov, image.Pt(0, 0), 1.0)
		imaging.Save(full, filepath.Join(outDir, tpl+".png"), imaging.PNGCompressionLevel(1))
		thumb := imaging.Resize(full, tw, th, imaging.Lanczos)
		col := i % 4
		row := i / 4
		montage = imaging.Paste(montage, thumb, image.Pt(col*(tw+10), row*(th+10)))
	}
	imaging.Save(montage, filepath.Join(outDir, "montage.jpg"), imaging.JPEGQuality(90))
	t.Logf("✓ montage → %s/montage.jpg", outDir)
}
