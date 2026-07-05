package main

// Render frame THẬT: lấy dữ liệu + ảnh THẬT từ API (testdata/listings.json) → build
// Config y hệt app (App.jsx buildRenderCfg: prices=price_lines, nickname=nickname||name,
// address, amenities=curate) → vẽ overlay qua CHÍNH renderVideoTemplateComposite trên
// nền ảnh thật. Mục đích: soi bug khi gặp data thật (text dài, combo 147 ký tự, 5 dòng giá,
// nickname null/dài, địa chỉ dài) với cả 8 template.
//   GENAPI=1 go test -count=1 -timeout 600s -run TestGenRealAPIFrames .

import (
	"image/color"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/disintegration/imaging"
	"github.com/fogleman/gg"
)

// curateAmenitiesVI tái hiện curateAmenities (App.jsx) trên amenities ĐÃ DỊCH (info.Amenities).
func curateAmenitiesVI(items []string) []string {
	norm := func(s string) string {
		s = strings.ToLower(s)
		s = strings.NewReplacer("á", "a", "à", "a", "ả", "a", "ã", "a", "ạ", "a",
			"ă", "a", "ắ", "a", "ằ", "a", "ẳ", "a", "ẵ", "a", "ặ", "a",
			"â", "a", "ấ", "a", "ầ", "a", "ẩ", "a", "ẫ", "a", "ậ", "a",
			"é", "e", "è", "e", "ẻ", "e", "ẽ", "e", "ẹ", "e",
			"ê", "e", "ế", "e", "ề", "e", "ể", "e", "ễ", "e", "ệ", "e",
			"í", "i", "ì", "i", "ỉ", "i", "ĩ", "i", "ị", "i",
			"ó", "o", "ò", "o", "ỏ", "o", "õ", "o", "ọ", "o",
			"ô", "o", "ố", "o", "ồ", "o", "ổ", "o", "ỗ", "o", "ộ", "o",
			"ơ", "o", "ớ", "o", "ờ", "o", "ở", "o", "ỡ", "o", "ợ", "o",
			"ú", "u", "ù", "u", "ủ", "u", "ũ", "u", "ụ", "u",
			"ư", "u", "ứ", "u", "ừ", "u", "ử", "u", "ữ", "u", "ự", "u",
			"ý", "y", "ỳ", "y", "ỷ", "y", "ỹ", "y", "ỵ", "y",
			"đ", "d").Replace(s)
		return strings.TrimSpace(s)
	}
	whitelist := []struct {
		label string
		kw    []string
	}{
		{"Bếp", []string{"bep", "kitchen", "stove", "induction"}},
		{"Máy chiếu", []string{"may chieu", "projector"}},
		{"Tivi", []string{"tivi", "ti vi", "tv", "television"}},
		{"Netflix", []string{"netflix"}},
		{"Bồn tắm", []string{"bon tam", "bathtub", "hot tub", "jacuzzi"}},
		{"Thang máy", []string{"thang may", "elevator", "lift"}},
		{"Máy giặt", []string{"may giat", "washer"}},
	}
	normed := make([]string, len(items))
	for i, a := range items {
		normed[i] = norm(a)
	}
	var out []string
	for _, w := range whitelist {
		for _, a := range normed {
			hit := false
			for _, k := range w.kw {
				if strings.Contains(a, k) {
					hit = true
					break
				}
			}
			if hit {
				out = append(out, w.label)
				break
			}
		}
	}
	return out
}

// downloadPhoto tải 1 ảnh (cache theo path). Trả path nếu OK.
func downloadPhoto(url, dst string) (string, error) {
	if st, err := os.Stat(dst); err == nil && st.Size() > 1000 {
		return dst, nil
	}
	cl := &http.Client{Timeout: 30 * time.Second}
	resp, err := cl.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", os.ErrNotExist
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 20<<20))
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(dst, b, 0o644); err != nil {
		return "", err
	}
	return dst, nil
}

func TestGenRealAPIFrames(t *testing.T) {
	if os.Getenv("GENAPI") == "" {
		t.Skip("set GENAPI=1")
	}
	const W, H = 1080, 1920
	data, err := os.ReadFile("testdata/listings.json")
	if err != nil {
		t.Fatal(err)
	}
	infos := parseListings(data)
	if len(infos) == 0 {
		t.Fatal("no listings parsed")
	}
	photoDir := "testdata/realphotos"
	os.MkdirAll(photoDir, 0o755)
	outDir := "../realapi-out"
	os.MkdirAll(outDir, 0o755)

	// 5 listing khó nhất (theo dump): combo dài, nickname dài, địa chỉ dài, nickname null.
	stress := []int{6, 15, 2, 5, 13}
	templates := []string{"goldserif", "creampill", "staycation", "chillgreen", "marquee", "editorial", "amorex", "ntgroom"}

	for _, idx := range stress {
		if idx >= len(infos) {
			continue
		}
		in := infos[idx]
		// tải ảnh nền (ảnh đầu, dùng thumb_720 cho nhẹ)
		bg := refV1
		if len(in.ThumbURLs) > 0 {
			dst := filepath.Join(photoDir, in.ID+"-0.jpg")
			if p, derr := downloadPhoto(in.ThumbURLs[0], dst); derr == nil {
				bg = p
			} else {
				t.Logf("download fail %s: %v (fallback)", in.ID, derr)
			}
		}
		nick := in.Nickname
		if nick == "" {
			nick = in.Name
		}
		amen := curateAmenitiesVI(in.Amenities)
		amenLine := strings.Join(amen, ", ")
		safeNick := strings.NewReplacer("/", "_", " ", "_").Replace(nick)
		if len(safeNick) > 18 {
			safeNick = safeNick[:18]
		}
		for _, tmpl := range templates {
			cfg := Config{
				Template: tmpl, Width: W, Height: H,
				Nickname: nick,
				Address:  in.Address,
				Prices:   in.PriceLines,
			}
			if amenLine != "" {
				cfg.Amenities = []string{amenLine}
			}
			if tmpl == "editorial" { // editorial dùng watermark làm brand đỉnh (app lấy từ ô nhập)
				cfg.Watermark = "Dayladau House"
			}
			dc := gg.NewContext(W, H)
			img, oerr := imaging.Open(bg, imaging.AutoOrientation(true))
			if oerr != nil {
				dc.SetColor(color.NRGBA{40, 30, 26, 255})
				dc.DrawRectangle(0, 0, W, H)
				dc.Fill()
			} else {
				dc.DrawImage(imaging.Fill(img, W, H, imaging.Center, imaging.Lanczos), 0, 0)
			}
			tmp := t.TempDir()
			plans, ok, perr := renderVideoTemplateComposite(cfg, tmp)
			if perr != nil || !ok {
				t.Fatalf("%d-%s composite: ok=%v err=%v", idx, tmpl, ok, perr)
			}
			for _, p := range plans {
				ov, e := imaging.Open(p.PNGPath)
				if e != nil {
					continue
				}
				dc.DrawImage(ov, p.X, p.Y)
			}
			name := filepath.Join(outDir, safeNick+"--"+tmpl+".jpg")
			if e := imaging.Save(dc.Image(), name, imaging.JPEGQuality(88)); e != nil {
				t.Fatalf("save %s: %v", name, e)
			}
		}
		t.Logf("✓ #%d %s (%d photos) -> 8 frames", idx, nick, len(in.PhotoURLs))
	}
}
