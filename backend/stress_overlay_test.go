package main

// Stress mockup: render MỌI template thumbnail + video với dữ liệu xấu nhất
// (địa chỉ dài wrap nhiều dòng + 24 tiện ích + giá nhiều dòng + listing_id) trên
// ảnh giả tự sinh, để soi overlap/đè chữ và kiểm tra listing_id hiển thị.
//
//   STRESS=1 go test -run 'TestStress' .
//
// Ảnh ra ở /tmp/img2video_stress/. Bỏ qua trừ khi đặt STRESS=1.

import (
	"fmt"
	"image"
	"image/color"
	"os"
	"path/filepath"
	"testing"

	"github.com/disintegration/imaging"
	"github.com/fogleman/gg"
)

// drawSafeZoneGuide vẽ khung vùng an toàn TikTok (bảo thủ, theo ảnh High Social:
// trên 13.1%, dưới 36.7%, trái 11.1%, phải 22.2% = cột nút like/comment). Khung
// ĐỎ = ranh giới chặt; vạch CAM ngang ~0.78H = mép dưới "nới lỏng" nhiều creator
// dùng. Chữ NẰM TRONG khung đỏ là chắc chắn an toàn.
func drawSafeZoneGuide(base image.Image, W, H int) image.Image {
	dc := gg.NewContextForImage(base)
	x0, y0 := 0.111*float64(W), 0.131*float64(H)
	x1, y1 := 0.778*float64(W), 0.633*float64(H)
	dc.SetRGBA(1, 0, 0, 0.95)
	dc.SetLineWidth(5)
	dc.DrawRectangle(x0, y0, x1-x0, y1-y0)
	dc.Stroke()
	dc.SetRGBA(1, 0.6, 0, 0.85)
	dc.SetLineWidth(3)
	dc.DrawLine(0, 0.78*float64(H), float64(W), 0.78*float64(H))
	dc.Stroke()
	return dc.Image()
}

const stressOut = "/tmp/img2video_stress"
const stressAddr = "1 P. Lê Văn Hưu, Phan Chu Trinh, Hai Bà Trưng, Hà Nội, Vietnam"
const stressID = "6OIYH"

var stressAmenities = []string{
	"Dao và thớt", "Bàn ăn", "Nước rửa chén", "Gia vị đầy đủ", "Gia vị cơ bản",
	"Nồi cơm điện", "Ấm đun nước", "Bếp từ", "Tủ lạnh", "Dụng cụ nấu ăn cơ bản",
	"Bếp", "Móc treo quần áo", "Ổ cắm điện", "Máy điều hòa", "Bàn chải",
	"Tăm dụng", "Máy nước nóng", "Sữa tắm", "Dầu gội", "Khăn tắm",
	"Máy sấy tóc", "Dầu xả", "TV", "Wifi",
}

func stressPhotos(t *testing.T, n int) []string {
	dir := t.TempDir()
	cols := []color.NRGBA{
		{120, 140, 160, 255}, {150, 120, 100, 255}, {100, 150, 120, 255},
		{160, 140, 110, 255}, {110, 120, 150, 255},
	}
	var out []string
	for i := 0; i < n; i++ {
		img := imaging.New(760, 1000, cols[i%len(cols)])
		p := filepath.Join(dir, fmt.Sprintf("p%d.jpg", i))
		if imaging.Save(img, p, imaging.JPEGQuality(85)) == nil {
			out = append(out, p)
		}
	}
	return out
}

func TestStressThumbs(t *testing.T) {
	if os.Getenv("STRESS") == "" {
		t.Skip("set STRESS=1")
	}
	os.MkdirAll(stressOut, 0o755)
	photos := stressPhotos(t, 5)
	const W, H = 960, 1280
	tpls := []string{"", "daiky", "valey", "peony", "tiger", "cento", "amber", "strip", "creamgrid", "filmstrip"}
	// 2 biến thể tiêu đề: tiếng Việt có dấu, và tên DÀI 1 TỪ (không wrap được) để
	// kiểm tra tràn viền (vd "mangoapartment").
	titles := []struct{ suffix, title string }{
		{"", "Giấc mơ trưa"},
		{"-long", "mangoapartment"},
	}
	for _, tpl := range tpls {
		for _, tt := range titles {
			cfg := ThumbnailConfig{
				Template: tpl, Width: W, Height: H,
				Title:     tt.title,
				Address:   stressAddr,
				Caption:   "Disco Room",
				Prices:    []string{"2N1Đ 450k", "Qua đêm 90k", "2h đầu 250k"},
				Amenities: stressAmenities,
				ListingID: stressID,
			}
			if tpl == "valey" {
				cfg.PriceTable = []ThumbPriceRow{
					{Slot: "Giờ đầu", Weekday: "250k", Weekend: "250k"},
					{Slot: "2N1Đ", Weekday: "450k", Weekend: "550k"},
					{Slot: "Qua đêm", Weekday: "90k", Weekend: "120k"},
				}
			}
			data, err := buildThumbnailImage(cfg, photos)
			if err != nil {
				t.Errorf("%s: %v", tpl, err)
				continue
			}
			name := tpl
			if name == "" {
				name = "classic"
			}
			dst := filepath.Join(stressOut, "thumb-"+name+tt.suffix+".jpg")
			os.WriteFile(dst, data, 0o644)
			t.Logf("✓ %s", dst)
		}
	}
}

func TestStressVideos(t *testing.T) {
	if os.Getenv("STRESS") == "" {
		t.Skip("set STRESS=1")
	}
	os.MkdirAll(stressOut, 0o755)
	const W, H = 1080, 1920
	tpls := []string{"staycation", "creampill", "goldserif", "editorial", "marquee", "chillgreen", "daiky", "sunset"}
	for _, tpl := range tpls {
		cfg := Config{
			Template: tpl, Width: W, Height: H,
			Address:   stressAddr,
			Nickname:  "Giấc mơ trưa",
			ListingID: stressID,
			Prices:    []string{"2N1Đ: 450.000đ", "Qua đêm: 90.000đ", "2h đầu: 250.000đ"},
			Amenities: stressAmenities,
			Watermark: "Dayladau Homestay",
		}
		tmp, _ := os.MkdirTemp("", "stress_*")
		plans, err := renderTextOverlays(cfg, tmp)
		if err != nil {
			os.RemoveAll(tmp)
			t.Errorf("%s: %v", tpl, err)
			continue
		}
		base := imaging.New(W, H, color.NRGBA{60, 70, 80, 255})
		for _, p := range plans {
			ov, err := imaging.Open(p.PNGPath)
			if err != nil {
				continue
			}
			base = imaging.Overlay(base, ov, image.Pt(p.X, p.Y), 1.0)
		}
		os.RemoveAll(tmp)
		guided := drawSafeZoneGuide(base, W, H)
		dst := filepath.Join(stressOut, "video-"+tpl+".jpg")
		imaging.Save(guided, dst, imaging.JPEGQuality(88))
		t.Logf("✓ %s (%d overlays)", dst, len(plans))
	}
}
