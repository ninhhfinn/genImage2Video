package main

// Vòng lặp render→so sánh CẢ 8 TEMPLATE với DATA API THẬT (Hà Nội 2026-07-03):
// testdata/api_hanoi_20260703.json, listing chính AC21 (lsim7jam8p — ca user báo
// "tên ngắn bị lệch" + "editorial phải là Standard"). Mỗi template xuất:
//   <tpl>_ac21.png  — overlay chrome NO-VEIL trên nền #3a3a3a (so bố cục/font
//                     với trang mockup_templates.html cùng nền trơn)
//   <tpl>_final.jpg — bản PRODUCTION (renderChromeOverlay, có veil) đè ảnh thật
//                     listing → đúng khung hình app xuất cho user
// Kèm PARITY (data y hệt trang mockup) cho 2 template vừa sửa: parity_ntgroom.png
// + parity_editorial.png — để diff pixel với render trang mockup bên ngoài.
//   REALLOOP=1 go test -count=1 -timeout 600s -run TestRealAPILoop .
// Output ../scratch/realloop/out/

import (
	"fmt"
	"image"
	"image/color"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/disintegration/imaging"
)

// ─── mirror curateAmenities (App.jsx) — harness phải gửi data y hệt FE ─────────

var viFold = strings.NewReplacer(
	"à", "a", "á", "a", "ả", "a", "ã", "a", "ạ", "a", "ă", "a", "ằ", "a", "ắ", "a", "ẳ", "a", "ẵ", "a", "ặ", "a",
	"â", "a", "ầ", "a", "ấ", "a", "ẩ", "a", "ẫ", "a", "ậ", "a", "è", "e", "é", "e", "ẻ", "e", "ẽ", "e", "ẹ", "e",
	"ê", "e", "ề", "e", "ế", "e", "ể", "e", "ễ", "e", "ệ", "e", "ì", "i", "í", "i", "ỉ", "i", "ĩ", "i", "ị", "i",
	"ò", "o", "ó", "o", "ỏ", "o", "õ", "o", "ọ", "o", "ô", "o", "ồ", "o", "ố", "o", "ổ", "o", "ỗ", "o", "ộ", "o",
	"ơ", "o", "ờ", "o", "ớ", "o", "ở", "o", "ỡ", "o", "ợ", "o", "ù", "u", "ú", "u", "ủ", "u", "ũ", "u", "ụ", "u",
	"ư", "u", "ừ", "u", "ứ", "u", "ử", "u", "ữ", "u", "ự", "u", "ỳ", "y", "ý", "y", "ỷ", "y", "ỹ", "y", "ỵ", "y",
	"đ", "d",
)

func curateAmenitiesMirror(list []string) []string {
	rules := []struct {
		label string
		kws   []string
	}{
		{"Bếp", []string{"bep", "kitchen", "stove", "induction"}},
		{"Máy chiếu", []string{"may chieu", "projector"}},
		{"Tivi", []string{"tivi", "ti vi", "tv", "television"}},
		{"Netflix", []string{"netflix"}},
		{"Bồn tắm", []string{"bon tam", "bathtub", "hot tub", "jacuzzi"}},
		{"Thang máy", []string{"thang may", "elevator", "lift"}},
		{"Máy giặt", []string{"may giat", "washer"}},
	}
	var norm []string
	for _, a := range list {
		norm = append(norm, viFold.Replace(strings.ToLower(a)))
	}
	var out []string
	for _, r := range rules {
		hit := false
		for _, a := range norm {
			for _, k := range r.kws {
				if strings.Contains(a, k) {
					hit = true
					break
				}
			}
			if hit {
				break
			}
		}
		if hit {
			out = append(out, r.label)
		}
	}
	return out
}

// renderOverlayOnFlat render body chrome NO-VEIL rồi đè lên nền trơn #3a3a3a
// (same nền trang mockup) → so bố cục/font trực tiếp với mockup_templates.html.
func renderOverlayOnFlat(t *testing.T, cfg Config, outPNG string) {
	t.Helper()
	body := chromeTemplateBody(strings.ToLower(cfg.Template), cfg)
	if body == "" {
		t.Fatalf("%s: body rỗng", cfg.Template)
	}
	tmp := t.TempDir()
	htmlPath := filepath.Join(tmp, "o.html")
	if err := os.WriteFile(htmlPath, []byte(chromePageNoVeil(cfg, body)), 0o644); err != nil {
		t.Fatal(err)
	}
	png := filepath.Join(tmp, "o.png")
	if err := runChromeShot(t, htmlPath, png, cfg.Width, cfg.Height); err != nil {
		t.Fatalf("%s: %v", cfg.Template, err)
	}
	ov, err := imaging.Open(png)
	if err != nil {
		t.Fatal(err)
	}
	flat := imaging.New(cfg.Width, cfg.Height, color.NRGBA{0x3a, 0x3a, 0x3a, 255})
	full := imaging.Overlay(flat, ov, image.Pt(0, 0), 1.0)
	if err := imaging.Save(full, outPNG, imaging.PNGCompressionLevel(1)); err != nil {
		t.Fatal(err)
	}
}

func TestRealAPILoop(t *testing.T) {
	if os.Getenv("REALLOOP") == "" {
		t.Skip("set REALLOOP=1")
	}
	const W, H = 1080, 1920
	data, err := os.ReadFile("testdata/api_hanoi_20260703.json")
	if err != nil {
		t.Fatal(err)
	}
	infos := parseListings(data)
	var in ListingInfo
	for _, x := range infos {
		if x.ID == "lsim7jam8p" { // Cozie House AC21, Nhật Tân, Tây Hồ
			in = x
			break
		}
	}
	if in.ID == "" {
		t.Fatal("không tìm thấy listing AC21")
	}
	nick := in.Nickname
	if nick == "" {
		nick = in.Name // FE: listing.nickname || listing.name
	}
	amen := curateAmenitiesMirror(in.Amenities)
	t.Logf("nick=%q addr=%q amenities=%v", nick, in.Address, amen)

	outDir := "../../scratch/realloop/out"
	os.MkdirAll(outDir, 0o755)

	// Ảnh phòng thật đầu tiên của AC21 làm nền bản production.
	var bgPath string
	if len(in.PhotoURLs) > 0 {
		if p, derr := downloadPhoto(in.PhotoURLs[0], filepath.Join(outDir, "_ac21_photo.jpg")); derr == nil {
			bgPath = p
		} else {
			t.Logf("tải ảnh listing lỗi (%v) → dùng nền trơn", derr)
		}
	}

	for _, tpl := range allTemplateNames {
		cfg := Config{
			Template: tpl, Width: W, Height: H,
			Nickname:  nick,
			Address:   in.Address,
			Prices:    in.PriceLinesByTemplate[tpl],
			Amenities: amen,
		}
		// Mirror production renderVideoTemplateComposite: nén giá + dọn địa chỉ.
		cfg.Prices = compactVideoPrices(cfg.Prices)
		cfg.Address = cleanVideoAddress(cfg.Address)
		t.Logf("%-11s prices=%v", tpl, cfg.Prices)

		// 1) overlay no-veil trên nền trơn — so với trang mockup.
		renderOverlayOnFlat(t, cfg, filepath.Join(outDir, tpl+"_ac21.png"))

		// 2) bản production (veil) trên ảnh thật.
		tmp := t.TempDir()
		comp, cerr := renderChromeOverlay(cfg, tmp)
		if cerr != nil {
			t.Fatalf("%s: production render: %v", tpl, cerr)
		}
		frame := imaging.New(W, H, color.NRGBA{40, 30, 26, 255})
		if bgPath != "" {
			if bgimg, e := imaging.Open(bgPath, imaging.AutoOrientation(true)); e == nil {
				frame = imaging.Fill(bgimg, W, H, imaging.Center, imaging.Lanczos)
			}
		}
		if ovimg, e := imaging.Open(comp); e == nil {
			frame = imaging.Overlay(frame, ovimg, image.Pt(0, 0), 1.0)
		}
		imaging.Save(frame, filepath.Join(outDir, tpl+"_final.jpg"), imaging.JPEGQuality(92))
	}

	// ── PARITY: data y hệt trang mockup → output phải trùng trang mockup ──────
	mockAddr := "Vũ Tông Phan, Thanh Xuân, Hà Nội"
	parity := []struct {
		tpl    string
		cfg    Config
	}{
		{"ntgroom", Config{Template: "ntgroom", Width: W, Height: H, Nickname: "NTG402",
			Address: mockAddr,
			Prices:  []string{"2N1Đ: 399k", "Qua đêm: 299 k", "Thêm giờ: 80k", "2h đầu: 199k", "Combo giờ: 299k"}}},
		{"editorial", Config{Template: "editorial", Width: W, Height: H, Nickname: "Standard",
			Address: mockAddr,
			Prices:  []string{"Giá giờ: 299 cá", "Giá qua đêm: 299 cá", "2N1Đ: 299 cá"}}},
	}
	for _, p := range parity {
		p.cfg.Prices = compactVideoPrices(p.cfg.Prices)
		p.cfg.Address = cleanVideoAddress(p.cfg.Address)
		renderOverlayOnFlat(t, p.cfg, filepath.Join(outDir, "parity_"+p.tpl+".png"))
	}

	t.Logf("✓ %d template + 2 parity → %s", len(allTemplateNames), outDir)
	_ = fmt.Sprint
}
