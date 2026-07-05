package main

// Harness verify 6 template thumbnail Chrome (canva1..canva6) với DATA API THẬT
// (testdata/api_hanoi_20260703.json), cfg mirror y hệt frontend buildThumbnailCfg
// (App.jsx): title = nickname, prices nén "2N1Đ 519k", amenities curate.
//   GENTHUMB=1 go test -count=1 -timeout 600s -run TestGenChromeThumbnails .
// Output ../../scratch/thumbloop/out/<tag>-canvaN.jpg — 2 listing × 6 template:
// ac21 (tên ngắn, ca chuẩn realloop) + long (title dài nhất, stress fitSpan).
// Ảnh phòng tải về cache ở out/photos/; offline → ảnh trơn màu ấm thay thế.

import (
	"fmt"
	"image/color"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/disintegration/imaging"
)

// ─── mirror App.jsx buildThumbnailCfg: price_line_items → "2N1Đ 519k" ─────────

var thumbKeyLabel = map[string]string{
	"two_day_one_night": "2N1Đ", "overnight": "Qua đêm",
	"hourly": "Giờ đầu", "extra_hour": "Thêm giờ", "hour_combo": "Combo",
}

var (
	// "đ" là đơn vị tiền (không theo sau bởi chữ cái) — tránh "1 đ" trong "1 đêm".
	reThumbAmountD   = regexp.MustCompile(`([\d][\d.]*)\s*đ(\P{L}|$)`)
	reThumbAmountAny = regexp.MustCompile(`[\d][\d.]*`)
	reThumbFirstH    = regexp.MustCompile(`(\d+)\s*giờ đầu`)
)

func thumbPriceLinesMirror(items []PriceLineItem) []string {
	compact := func(text string) string {
		num := ""
		if m := reThumbAmountD.FindStringSubmatch(text); m != nil {
			num = m[1]
		} else {
			num = reThumbAmountAny.FindString(text)
		}
		if num == "" {
			return ""
		}
		n, _ := strconv.Atoi(strings.ReplaceAll(num, ".", ""))
		if n == 0 {
			return ""
		}
		if n >= 1000 {
			return strconv.Itoa(int(math.Round(float64(n)/1000))) + "k"
		}
		return strconv.Itoa(n)
	}
	var out []string
	for _, it := range items {
		label := thumbKeyLabel[it.Key]
		if it.Key == "hourly" {
			if m := reThumbFirstH.FindStringSubmatch(it.Text); m != nil {
				label = m[1] + "h đầu"
			}
		}
		var parts []string
		if label != "" {
			parts = append(parts, label)
		}
		if amt := compact(it.Text); amt != "" {
			parts = append(parts, amt)
		}
		if len(parts) > 0 {
			out = append(out, strings.Join(parts, " "))
		}
	}
	return out
}

// thumbTestTitle mirror FE: nickname || firstWord(name) — firstWord bỏ prefix
// "[OFF 20%]" rồi lấy từ đầu (+ từ 2 nếu là số phòng): "STUDIO 504".
var reBracketPrefix = regexp.MustCompile(`^\s*(\[[^\]]*\]\s*)+`)

func thumbTestTitle(in ListingInfo) string {
	if n := strings.TrimSpace(in.Nickname); n != "" {
		return n
	}
	ws := strings.Fields(reBracketPrefix.ReplaceAllString(in.Name, ""))
	if len(ws) == 0 {
		return ""
	}
	if len(ws) > 1 && strings.ContainsAny(ws[1], "0123456789") {
		return ws[0] + " " + ws[1]
	}
	return ws[0]
}

// thumbTestPhotos tải tối đa n ảnh listing (cache out/photos/); mạng lỗi hết →
// ảnh trơn màu ấm để harness vẫn chạy offline.
func thumbTestPhotos(t *testing.T, tag string, in ListingInfo, n int, outDir string) []string {
	t.Helper()
	dir := filepath.Join(outDir, "photos")
	os.MkdirAll(dir, 0o755)
	var out []string
	for i := 0; i < n && i < len(in.PhotoURLs); i++ {
		dst := filepath.Join(dir, fmt.Sprintf("%s-%d.jpg", tag, i))
		if _, err := os.Stat(dst); err == nil {
			out = append(out, dst)
			continue
		}
		if p, err := downloadPhoto(in.PhotoURLs[i], dst); err == nil {
			out = append(out, p)
		} else {
			t.Logf("%s: tải ảnh %d lỗi (%v)", tag, i, err)
		}
	}
	if len(out) == 0 {
		cols := []color.NRGBA{{110, 84, 66, 255}, {84, 96, 78, 255}, {96, 76, 88, 255}, {70, 82, 96, 255}}
		for i := 0; i < n; i++ {
			dst := filepath.Join(dir, fmt.Sprintf("%s-flat%d.jpg", tag, i))
			if err := imaging.Save(imaging.New(1080, 1080, cols[i%len(cols)]), dst, imaging.JPEGQuality(85)); err != nil {
				t.Fatal(err)
			}
			out = append(out, dst)
		}
	}
	return out
}

// TestThumbLoopAll — VÒNG LẶP DATA THẬT toàn bộ listing (mirror realloop cho
// thumbnail): render mọi listing trong THUMBSRC (mặc định api_fresh_20260704.json,
// tải từ api.dayladau.com/v1/listings) × 6 template canva → scratch/thumbloop/loop/
// <idx>-<id>-canvaN.jpg. Soi montage từng template, sửa code, lặp lại. Lặp nhanh
// bằng filter: THUMBTPL=canva2,canva4 THUMBIDS=3,16 (index hoặc id).
//   THUMBLOOP=1 go test -count=1 -timeout 1800s -run TestThumbLoopAll .
func TestThumbLoopAll(t *testing.T) {
	if os.Getenv("THUMBLOOP") == "" {
		t.Skip("set THUMBLOOP=1")
	}
	src := os.Getenv("THUMBSRC")
	if src == "" {
		src = "testdata/api_fresh_20260704.json"
	}
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	infos := parseListings(data)
	if len(infos) == 0 {
		t.Fatal("không parse được listing nào")
	}
	tplFilter := map[string]bool{}
	for _, s := range strings.Split(os.Getenv("THUMBTPL"), ",") {
		if s = strings.TrimSpace(s); s != "" {
			tplFilter[s] = true
		}
	}
	idFilter := map[string]bool{}
	for _, s := range strings.Split(os.Getenv("THUMBIDS"), ",") {
		if s = strings.TrimSpace(s); s != "" {
			idFilter[s] = true
		}
	}
	outDir := "../../scratch/thumbloop/loop"
	os.MkdirAll(outDir, 0o755)

	for idx, in := range infos {
		if len(idFilter) > 0 && !idFilter[in.ID] && !idFilter[strconv.Itoa(idx)] {
			continue
		}
		prices := thumbPriceLinesMirror(in.PriceLineItems)
		if len(prices) == 0 {
			prices = in.PriceLines
		}
		amen := curateAmenitiesMirror(in.Amenities)
		photos := thumbTestPhotos(t, in.ID, in, 4, outDir)
		t.Logf("%02d %s: title=%q addr=%q prices=%d amen=%d",
			idx, in.ID, thumbTestTitle(in), in.Address, len(prices), len(amen))

		for i := 1; i <= 6; i++ {
			tpl := fmt.Sprintf("canva%d", i)
			if len(tplFilter) > 0 && !tplFilter[tpl] {
				continue
			}
			cfg := ThumbnailConfig{
				Width: 960, Height: 1280,
				Title: thumbTestTitle(in), Address: in.Address,
				Prices: prices, Amenities: amen, Template: tpl,
			}
			jpg, rerr := buildThumbnailImage(cfg, photos)
			if rerr != nil {
				t.Errorf("%02d %s %s: %v", idx, in.ID, tpl, rerr)
				continue
			}
			out := filepath.Join(outDir, fmt.Sprintf("%02d-%s-%s.jpg", idx, in.ID, tpl))
			if werr := os.WriteFile(out, jpg, 0o644); werr != nil {
				t.Fatal(werr)
			}
		}
	}
}

func TestGenChromeThumbnails(t *testing.T) {
	if os.Getenv("GENTHUMB") == "" {
		t.Skip("set GENTHUMB=1")
	}
	data, err := os.ReadFile("testdata/api_hanoi_20260703.json")
	if err != nil {
		t.Fatal(err)
	}
	infos := parseListings(data)
	var short, long ListingInfo
	for _, x := range infos {
		if x.ID == "lsim7jam8p" { // Cozie House AC21 — ca chuẩn của realloop
			short = x
		}
	}
	if short.ID == "" {
		t.Fatal("không tìm thấy listing AC21")
	}
	for _, x := range infos {
		if x.ID == short.ID {
			continue
		}
		if len([]rune(thumbTestTitle(x))) > len([]rune(thumbTestTitle(long))) {
			long = x
		}
	}

	outDir := "../../scratch/thumbloop/out"
	os.MkdirAll(outDir, 0o755)

	for _, lc := range []struct {
		tag string
		in  ListingInfo
	}{{"ac21", short}, {"long", long}} {
		prices := thumbPriceLinesMirror(lc.in.PriceLineItems)
		if len(prices) == 0 { // FE fallback: price_lines gốc
			prices = lc.in.PriceLines
		}
		amen := curateAmenitiesMirror(lc.in.Amenities)
		photos := thumbTestPhotos(t, lc.tag, lc.in, 4, outDir)
		t.Logf("%s: title=%q addr=%q prices=%v amen=%v photos=%d",
			lc.tag, thumbTestTitle(lc.in), lc.in.Address, prices, amen, len(photos))

		for i := 1; i <= 6; i++ {
			tpl := fmt.Sprintf("canva%d", i)
			cfg := ThumbnailConfig{
				// FE gửi 960×1280 — đường canva bỏ qua, luôn ra 1080×1920.
				Width: 960, Height: 1280,
				Title: thumbTestTitle(lc.in), Address: lc.in.Address,
				Prices: prices, Amenities: amen, Template: tpl,
			}
			jpg, err := buildThumbnailImage(cfg, photos) // đường production đầy đủ
			if err != nil {
				t.Fatalf("%s %s: %v", lc.tag, tpl, err)
			}
			out := filepath.Join(outDir, lc.tag+"-"+tpl+".jpg")
			if err := os.WriteFile(out, jpg, 0o644); err != nil {
				t.Fatal(err)
			}
			t.Logf("  ✓ %s (%dKB)", out, len(jpg)/1024)
		}
	}
}
