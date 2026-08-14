package main

// Mockup preview generator (run manually, NOT part of CI):
//
//   GENMOCKUP=1 go test -run TestGenVideoMockups ./...
//   GENMOCKUP=1 go test -run TestGenThumbMockups ./...
//
// Renders each new template through the REAL production path
// (renderTextOverlays / buildThumbnailImage) over blurred reference photos and
// writes JPGs to ../template-mockups/ so the designs can be reviewed before
// finalizing. Skipped unless GENMOCKUP is set.

import (
	"fmt"
	"image"
	"image/color"
	"os"
	"path/filepath"
	"testing"

	"github.com/disintegration/imaging"
)

const mockOut = "../template-mockups"

func mockHome() string { h, _ := os.UserHomeDir(); return h }

func refPath(name string) string { return filepath.Join(mockHome(), "Downloads", name) }

// Reference photos (the user-supplied designs we are reproducing).
var (
	refV1 = refPath("2aOboQc1aSKdJWMzs0LwsjOxOH0BqsPpigD3zYHo.jpg")
	refV2 = refPath("2aOboQc1aRUFmdKutnUuwklHFSGs51hnslHmpiU4.jpg")
	refV3 = refPath("2aOboQc1aQsLsvkifDANV9Q0ITZ4ASrWnsGpdyCm.jpg")
	refV4 = refPath("2aOboQbqUXlYgpJUwER5Y2abvLPqRRNmtOz8eeDQ(1).jpg")
	refV5 = refPath("2aOboQbqSToYIWEjKIYpuE3ZijDLrvmAnqKj9JLM(1).jpg")
	refV6 = refPath("2aOboQbqSSScx29rf8yDHFSGNJTWVIpBlFX0GcJk.jpg")

	refT1 = refPath("2aOboQbqSRYRSRn0rnKeSZfKHmx0JR69VY3MXmPw(1).jpg")
	refT2 = refPath("2aOboQbqSTAxnUAERme5A81ueIeGp6R5vlLFqRVY(1).jpg")
	refT3 = refPath("2aOboQc1ac22JbBdCnm7xEcmF7dLr2Uqai6IS4Ui.jpg")
	refT4 = refPath("2aOboQc1abJjty7dvMyRugv5mLmV7C5YT18DIn56.jpg")
	refT5 = refPath("2aOboQc1aaa4La6O1fa5KLkXx7g7apMhERfYMbUO.jpg")
)

// blurredBg loads a reference photo, fills it to w×h, then mosaics + blurs it so
// the baked-in text from the original design is fully destroyed (leaving only an
// abstract color field of the room), dimming it a touch so overlay text reads.
func blurredBg(path string, w, h int, blur, dim float64) image.Image {
	img, err := imaging.Open(path, imaging.AutoOrientation(true))
	if err != nil {
		return imaging.New(w, h, color.NRGBA{46, 34, 28, 255})
	}
	out := imaging.Fill(img, w, h, imaging.Center, imaging.Lanczos)
	small := imaging.Resize(out, 34, 0, imaging.Linear) // mosaic: nuke any text
	out = imaging.Resize(small, w, h, imaging.Linear)
	out = imaging.Blur(out, blur)
	if dim != 0 {
		out = imaging.AdjustBrightness(out, dim)
	}
	return out
}

// prepCleanPhotos fills each reference photo to a standard cell size and softly
// blurs it (so the originals' baked-in text is unreadable at collage scale) into
// temp files, returning their paths — used as placeholder photos for thumbnail
// collage mockups.
func prepCleanPhotos(t *testing.T, paths []string) []string {
	dir := t.TempDir()
	var out []string
	for i, p := range paths {
		img, err := imaging.Open(p, imaging.AutoOrientation(true))
		if err != nil {
			continue
		}
		c := imaging.Fill(img, 760, 1000, imaging.Center, imaging.Lanczos)
		c = imaging.Blur(c, 8) // strong enough to erase the originals' baked-in text
		dst := filepath.Join(dir, fmt.Sprintf("p%d.jpg", i))
		if imaging.Save(c, dst, imaging.JPEGQuality(90)) == nil {
			out = append(out, dst)
		}
	}
	return out
}

// cropFrac crops a fractional box [x0,y0]-[x1,y1] (0..1) out of an image.
func cropFrac(img image.Image, x0, y0, x1, y1 float64) image.Image {
	b := img.Bounds()
	W, H := b.Dx(), b.Dy()
	return imaging.Crop(img, image.Rect(b.Min.X+int(x0*float64(W)), b.Min.Y+int(y0*float64(H)),
		b.Min.X+int(x1*float64(W)), b.Min.Y+int(y1*float64(H))))
}

// roomCrop describes one clean room photo extracted from a reference collage.
type roomCrop struct {
	src                string
	x0, y0, x1, y1     float64
}

// Verified text-free room crops (sharp) used as placeholder photos for the
// thumbnail collages — different angles of the same homestays.
var roomCrops = []roomCrop{
	{refT4, 0.07, 0.215, 0.47, 0.40}, // clean disco wide
	{refV1, 0.0, 0.63, 1.0, 1.0},     // disco bed
	{refV2, 0.0, 0.56, 1.0, 1.0},     // disco bed angle 2
	{refV3, 0.0, 0.60, 1.0, 1.0},     // disco bed angle 3
	{refV4, 0.04, 0.58, 0.99, 0.99},  // beige bed
}

// prepRooms extracts the sharp clean room crops to temp files for use as
// collage photos.
func prepRooms(t *testing.T) []string {
	dir := t.TempDir()
	var out []string
	for i, rc := range roomCrops {
		img, err := imaging.Open(rc.src, imaging.AutoOrientation(true))
		if err != nil {
			continue
		}
		c := cropFrac(img, rc.x0, rc.y0, rc.x1, rc.y1)
		dst := filepath.Join(dir, fmt.Sprintf("r%d.jpg", i))
		if imaging.Save(c, dst, imaging.JPEGQuality(92)) == nil {
			out = append(out, dst)
		}
	}
	return out
}

func TestExtractRooms(t *testing.T) {
	if os.Getenv("GENMOCKUP") == "" {
		t.Skip("set GENMOCKUP=1")
	}
	dir := filepath.Join(mockOut, "_rooms")
	os.MkdirAll(dir, 0o755)
	for i, rc := range roomCrops {
		img, err := imaging.Open(rc.src, imaging.AutoOrientation(true))
		if err != nil {
			t.Logf("open %s: %v", rc.src, err)
			continue
		}
		c := cropFrac(img, rc.x0, rc.y0, rc.x1, rc.y1)
		imaging.Save(c, filepath.Join(dir, fmt.Sprintf("room%02d.jpg", i)), imaging.JPEGQuality(92))
	}
	t.Logf("wrote %d room crops", len(roomCrops))
}

func TestGenVideoMockups(t *testing.T) {
	if os.Getenv("GENMOCKUP") == "" {
		t.Skip("set GENMOCKUP=1 to generate mockups")
	}
	if err := os.MkdirAll(mockOut, 0o755); err != nil {
		t.Fatal(err)
	}
	const W, H = 1080, 1920

	cases := []struct {
		file, ref string
		cfg       Config
	}{
		{"video-1-staycation.jpg", refV1, Config{
			Template: "staycation", Width: W, Height: H,
			Address:   "Cầu Giấy, Hà Nội",
			Nickname:  "Amoureux NK605",
			Amenities: []string{"Máy chiếu Netflix · Bồn tắm · Boardgame"},
			Prices:    []string{"2N1Đ: 299.000đ\nQua đêm: 299.000đ\nCombo theo giờ: 299.000đ"},
			Watermark: "Dayladau Homestay",
		}},
		{"video-2-creampill.jpg", refV2, Config{
			Template: "creampill", Width: W, Height: H,
			Address:   "Đường Nguyễn Khang, Cầu Giấy",
			Nickname:  "Amoureux NK605",
			Prices:    []string{"Qua đêm: 299.000đ    -    Combo: 299.000đ"},
			Watermark: "Dayladau Homestay",
		}},
		{"video-3-goldserif.jpg", refV3, Config{
			Template: "goldserif", Width: W, Height: H,
			Address:   "Đường Nguyễn Khang, Cầu Giấy",
			Nickname:  "Amoureux NK605",
			Prices:    []string{"Giá theo ngày\n2N1Đ: 390.000đ      Qua đêm: 290.000đ\nGiá theo giờ\n14h–20h: 390.000đ      14h–18h: 290.000đ"},
			Watermark: "Dayladau Homestay",
		}},
		{"video-4-editorial.jpg", refV4, Config{
			Template: "editorial", Width: W, Height: H,
			Address:   "số 4 ngõ 444 Đội Cấn, Ba Đình",
			Nickname:  "Amoureux NK605",
			Prices:    []string{"Giá giờ: 349K / 2H\nGiá đêm/ngày: 589K\nGiá ngày/đêm: 689K"},
			Watermark: "Dayladau Homestay",
		}},
		{"video-5-marquee.jpg", refV5, Config{
			Template: "marquee", Width: W, Height: H,
			Nickname:  "Amoureux NK605",
			Amenities: []string{"Máy chiếu Netflix – Bếp cooking date"},
			Prices:    []string{"Qua đêm 21h–9h: 314K · 2N1Đ: 449K\n10h–13h: 199K · 10h–20h: 299K · 14h–20h: 279K"},
			Watermark: "Dayladau Homestay",
		}},
		{"video-6-chillgreen.jpg", refV6, Config{
			Template: "chillgreen", Width: W, Height: H,
			Address:   "Thanh Xuân, Hà Nội",
			Nickname:  "Chữa lành · siu chill · đầy đủ tiện nghi, máy chiếu Netflix",
			Prices:    []string{"Qua đêm 21h–9h: 279K · 2N1Đ: 399K\n10h–20h: 299K"},
			Watermark: "Dayladau Homestay",
		}},
	}

	for _, c := range cases {
		tmp, err := os.MkdirTemp("", "mock_*")
		if err != nil {
			t.Fatal(err)
		}
		plans, err := renderTextOverlays(c.cfg, tmp)
		if err != nil {
			os.RemoveAll(tmp)
			t.Fatalf("%s: render: %v", c.file, err)
		}
		base := imaging.Clone(blurredBg(c.ref, W, H, 9, -10))
		for _, p := range plans {
			ov, err := imaging.Open(p.PNGPath)
			if err != nil {
				continue
			}
			base = imaging.Overlay(base, ov, image.Pt(p.X, p.Y), 1.0)
		}
		os.RemoveAll(tmp)
		dst := filepath.Join(mockOut, c.file)
		if err := imaging.Save(base, dst, imaging.JPEGQuality(90)); err != nil {
			t.Fatalf("%s: save: %v", c.file, err)
		}
		t.Logf("✓ %s  (%d overlays)", c.file, len(plans))
	}
}

func TestGenThumbMockups(t *testing.T) {
	if os.Getenv("GENMOCKUP") == "" {
		t.Skip("set GENMOCKUP=1 to generate mockups")
	}
	if err := os.MkdirAll(mockOut, 0o755); err != nil {
		t.Fatal(err)
	}
	const W, H = 960, 1280

	// Sharp clean room photos cropped from the references (text-free).
	pool := prepRooms(t)
	if len(pool) == 0 {
		t.Fatal("no placeholder photos")
	}
	pick := func(idx ...int) []string {
		var out []string
		for _, i := range idx {
			out = append(out, pool[i%len(pool)])
		}
		return out
	}

	cases := []struct {
		file string
		cfg  ThumbnailConfig
		ph   []string
	}{
		{"thumb-1-cento.jpg", ThumbnailConfig{
			Template: "cento", Width: W, Height: H,
			Title:   "Amoureux NK605",
			Address: "68 Cầu Giấy, Hà Nội",
			Prices:  []string{"COMBO 4H\n359K", "COMBO 7H\n499K", "14H–12H\n689K", "22H–10H\n489K"},
			Amenities: []string{"Tự động check-in", "Có máy check cam ẩn", "Cửa sổ thoáng",
				"Máy chiếu Netflix Premium", "Có chỗ để xe", "Free nước lọc"},
			Watermark: "",
		}, pick(0, 1, 2, 3)},
		{"thumb-2-amber.jpg", ThumbnailConfig{
			Template: "amber", Width: W, Height: H,
			Title:     "Amoureux NK605",
			Address:   "30 ngõ 169 Doãn Kế Thiện",
			Prices: []string{"Giá giờ: 369K/2H", "Giá đêm/ngày: 559K", "Giá ngày/đêm: 659K"},
			// Brand/PageBadge là trường riêng của bản Lagom, đã gỡ ở 603fdb1
			// ("bỏ bản Lagom") nhưng test chưa dọn theo — cả gói test không biên
			// dịch được kể từ đó. Bỏ hai dòng này để khôi phục.
			Watermark: "",
		}, pick(2)},
		{"thumb-3-strip.jpg", ThumbnailConfig{
			Template:  "strip", Width: W, Height: H,
			Title:     "Amoureux NK605",
			Address:   "Đường Nguyễn Khang, Cầu Giấy",
			Amenities: []string{"Máy chiếu Netflix, bồn tắm, boardgame"},
			Prices:    []string{"2N1Đ: 299,000", "Qua đêm: 299,000", "combo theo giờ: 299,000"},
			Watermark: "",
		}, pick(0, 1, 2)},
		{"thumb-4-creamgrid.jpg", ThumbnailConfig{
			Template:  "creamgrid", Width: W, Height: H,
			Title:     "Amoureux NK605",
			Address:   "Đường Nguyễn Khang, Cầu Giấy",
			Caption:   "Disco Room",
			Amenities: []string{"Máy chiếu Netflix, bồn tắm, boardgame"},
			Prices:    []string{"2N1Đ: 299,000", "Qua đêm: 299,000", "combo theo giờ: 299,000"},
			Watermark: "",
		}, pick(0, 1, 3, 4)},
		{"thumb-5-filmstrip.jpg", ThumbnailConfig{
			Template:  "filmstrip", Width: W, Height: H,
			Title:     "Amoureux NK605",
			Address:   "Đường Nguyễn Khang, Cầu Giấy",
			Prices:    []string{"2N1Đ: 390,000", "Qua đêm: 290,000", "combo theo giờ: 299,000"},
			Watermark: "",
		}, pick(0)},
	}

	for _, c := range cases {
		data, err := buildThumbnailImage(c.cfg, c.ph)
		if err != nil {
			t.Fatalf("%s: build: %v", c.file, err)
		}
		dst := filepath.Join(mockOut, c.file)
		if err := os.WriteFile(dst, data, 0o644); err != nil {
			t.Fatalf("%s: write: %v", c.file, err)
		}
		t.Logf("✓ %s  (%d bytes)", c.file, len(data))
	}
}
