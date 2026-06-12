package main

// Mockup video templates — gọi CHÍNH các hàm vẽ production trong
// video_templates.go trên nền ảnh tham chiếu, nên gallery template-mockups/
// luôn phản ánh đúng output thật. Run: GENMOCKUP=1 go test -run TestGenVideoFaithful .

import (
	"bytes"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"testing"

	"github.com/disintegration/imaging"
	"github.com/fogleman/gg"
)

func vbg(dc *gg.Context, path string, W, H int, dim, x0, y0, x1, y1 float64) {
	img, err := imaging.Open(path, imaging.AutoOrientation(true))
	if err != nil {
		dc.SetColor(color.NRGBA{40, 30, 26, 255})
		dc.DrawRectangle(0, 0, float64(W), float64(H))
		dc.Fill()
		return
	}
	if !(x0 == 0 && y0 == 0 && x1 == 1 && y1 == 1) {
		img = imaging.Clone(cropFrac(img, x0, y0, x1, y1))
	}
	out := imaging.Fill(img, W, H, imaging.Center, imaging.Lanczos)
	if dim != 0 {
		out = imaging.AdjustBrightness(out, dim)
	}
	dc.DrawImage(out, 0, 0)
}

// softVeil applies an even gentle darken over the whole frame plus a soft bottom
// gradient — no hard band edge.
func softVeil(dc *gg.Context, W, H int, base, botBoost uint8) {
	if base > 0 {
		dc.SetColor(color.NRGBA{6, 4, 3, base})
		dc.DrawRectangle(0, 0, float64(W), float64(H))
		dc.Fill()
	}
	if botBoost > 0 {
		vGradient(dc, 0, float64(H)*0.40, float64(W), float64(H)*0.60,
			color.NRGBA{6, 4, 3, 0}, color.NRGBA{6, 4, 3, botBoost})
	}
}

func encodeJPG(t *testing.T, dc *gg.Context, name string) {
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, dc.Image(), &jpeg.Options{Quality: 90}); err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	os.WriteFile(filepath.Join(mockOut, name), buf.Bytes(), 0o644)
	t.Logf("✓ %s", name)
}

func TestGenVideoFaithful(t *testing.T) {
	if os.Getenv("GENMOCKUP") == "" {
		t.Skip("set GENMOCKUP=1")
	}
	os.MkdirAll(mockOut, 0o755)
	const W, H = 1080, 1920

	cases := []struct {
		file, ref          string
		dim, y0            float64
		base, botBoost     uint8
		cfg                Config
	}{
		{"video-1-staycation.jpg", refV1, 10, 0.62, 45, 70, Config{
			Template: "staycation", Width: W, Height: H,
			Nickname:  "Amoureux NK605",
			Address:   "Đường Nguyễn Khang, Cầu Giấy",
			Amenities: []string{"Máy chiếu Netflix, bồn tắm, boardgame"},
			Prices:    []string{"2N1Đ: 299,000\nQua đêm: 299,000\nCombo theo giờ: 299,000"},
		}},
		{"video-2-creampill.jpg", refV2, -8, 0.46, 45, 70, Config{
			Template: "creampill", Width: W, Height: H,
			Nickname: "Amoureux NK605",
			Address:  "Đường Nguyễn Khang, Cầu Giấy",
			Prices:   []string{"Qua đêm: 299,000", "Combo: 299,000"},
		}},
		{"video-3-goldserif.jpg", refV3, 10, 0.54, 45, 70, Config{
			Template: "goldserif", Width: W, Height: H,
			Nickname: "Amoureux NK605",
			Address:  "Đường Nguyễn Khang, Cầu Giấy",
			Prices: []string{"Giá theo ngày\n2N1Đ: 390,000\nQua đêm: 290,000",
				"Giá theo giờ\n14h - 20h: 390,000\n14h - 18h: 290,000"},
		}},
		{"video-4-editorial.jpg", refV4, -6, 0.56, 40, 50, Config{
			Template: "editorial", Width: W, Height: H,
			Nickname:  "Standard",
			Address:   "số 4 ngõ 444 Đội Cấn, Ba Đình",
			Watermark: "Dayladau House",
			Prices:    []string{"Giá giờ: 349K/2H\nGiá đêm/ngày: 589K\nGiá ngày/đêm: 689K"},
		}},
		{"video-5-marquee.jpg", refV4, -14, 0.56, 60, 90, Config{
			Template: "marquee", Width: W, Height: H,
			Amenities: []string{"Máy chiếu Netflix – Bếp cooking date"},
			Prices: []string{"21h - 9h (Qua đêm): 314\n2N1Đ: 449",
				"10h - 13h: 199\n10h - 20h: 299\n14h - 20h: 279"},
		}},
		{"video-6-chillgreen.jpg", refV1, 6, 0.62, 55, 85, Config{
			Template: "chillgreen", Width: W, Height: H,
			Nickname: "Homestay chữa lành siu chill\nđầy đủ tiện nghi, máy chiếu Netflix",
			Address:  "Thanh Xuân - Hà Nội",
			Prices: []string{"21h - 9h (Qua đêm): 279\n2N1Đ: 399",
				"10h - 13h: 299\n10h - 20h: 299\n14h - 20h: 299"},
		}},
	}

	for _, c := range cases {
		dc := gg.NewContext(W, H)
		vbg(dc, c.ref, W, H, c.dim, 0, c.y0, 1, 1)
		softVeil(dc, W, H, c.base, c.botBoost)

		// vẽ qua đúng đường production: composite PNG rồi đè lên nền
		tmp := t.TempDir()
		plans, ok, err := renderVideoTemplateComposite(c.cfg, tmp)
		if err != nil || !ok {
			t.Fatalf("%s: composite: ok=%v err=%v", c.file, ok, err)
		}
		for _, p := range plans {
			ov, oerr := imaging.Open(p.PNGPath)
			if oerr != nil {
				continue
			}
			dc.DrawImage(ov, p.X, p.Y)
		}
		encodeJPG(t, dc, c.file)
	}
}
