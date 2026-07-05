package main

// Vòng lặp render→so sánh cho template chillgreen (Trang 4) với DATA API THẬT
// (testdata/api_thanhxuan.json, listing Thanh Xuân VTP501). Render overlay bằng
// Chrome (đúng đường chrome_overlay.go), composite lên ảnh mockup gốc để soi lệch,
// và render trên nền tối để xem sạch. Output ../scratch/chill-loop/.
//   CHILLLOOP=1 go test -count=1 -timeout 180s -run TestChillLoop .

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/disintegration/imaging"
)

// runChromeShot chụp transparent 1 file HTML → PNG (giống renderChromeOverlay).
func runChromeShot(t *testing.T, htmlPath, outPath string, w, h int) error {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	userDir := filepath.Join(filepath.Dir(htmlPath), "chrome-profile")
	args := []string{
		"--headless", "--no-sandbox", "--disable-gpu", "--hide-scrollbars",
		"--force-device-scale-factor=1", "--default-background-color=00000000",
		"--run-all-compositor-stages-before-draw", "--virtual-time-budget=5000",
		"--user-data-dir=" + userDir,
		"--screenshot=" + outPath,
		fmt.Sprintf("--window-size=%d,%d", w, h),
		"file://" + htmlPath,
	}
	out, err := exec.CommandContext(ctx, chromeBin(), args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("chrome: %v: %s", err, strings.TrimSpace(string(out)))
	}
	if st, e := os.Stat(outPath); e != nil || st.Size() == 0 {
		return fmt.Errorf("chrome: không tạo được %s", outPath)
	}
	return nil
}

// chromePageNoVeil = chromePage nhưng BỎ lớp veil, để composite lên ảnh ref mà
// không làm tối cả ảnh (chỉ so vị trí/ font/ chữ overlay).
func chromePageNoVeil(cfg Config, body string) string {
	return `<!doctype html><html><head><meta charset="utf-8"><style>` +
		`*{margin:0;padding:0;box-sizing:border-box}html,body{background:transparent}` +
		fontFaceCSS() +
		`.stage{width:1080px;height:1920px;position:relative;overflow:hidden;background:transparent}` +
		`</style></head><body><div class="stage">` + body + autofitScript + `</body></html>`
}

func TestChillLoop(t *testing.T) {
	if os.Getenv("CHILLLOOP") == "" {
		t.Skip("set CHILLLOOP=1")
	}
	const W, H = 1080, 1920
	data, err := os.ReadFile("testdata/api_thanhxuan.json")
	if err != nil {
		t.Fatal(err)
	}
	infos := parseListings(data)
	var in ListingInfo
	for _, x := range infos {
		if x.ID == "lss5wnncbl" { // VTP501, Thanh Xuân
			in = x
			break
		}
	}
	if in.ID == "" {
		t.Fatal("không tìm thấy listing VTP501")
	}

	t.Logf("nick=%q addr=%q", in.Nickname, in.Address)
	t.Logf("districtProvince = %q", districtProvinceLine(in.Address))
	t.Logf("VideoPriceLines = %#v", in.VideoPriceLines)

	cfg := Config{
		Template: "chillgreen", Width: W, Height: H,
		Nickname: in.Nickname,
		Address:  in.Address, // địa chỉ đầy đủ như production → template tự suy "Quận - Tỉnh"
		Prices:   in.VideoPriceLines,
	}

	tmp := t.TempDir()
	body := chromeTemplateBody("chillgreen", cfg)
	if body == "" {
		t.Fatal("body rỗng")
	}
	htmlPath := filepath.Join(tmp, "overlay.html")
	if err := os.WriteFile(htmlPath, []byte(chromePageNoVeil(cfg, body)), 0o644); err != nil {
		t.Fatal(err)
	}
	overlayPNG := filepath.Join(tmp, "overlay.png")
	if err := runChromeShot(t, htmlPath, overlayPNG, W, H); err != nil {
		t.Fatal(err)
	}

	outDir := "../scratch/chill-loop"
	os.MkdirAll(outDir, 0o755)

	ov, err := imaging.Open(overlayPNG)
	if err != nil {
		t.Fatal(err)
	}

	// 1) Overlay trên nền tối ấm (xem sạch chữ).
	dark := imaging.New(W, H, color.NRGBA{40, 26, 18, 255})
	clean := imaging.Overlay(dark, ov, image.Pt(0, 0), 1.0)
	imaging.Save(clean, filepath.Join(outDir, "chill_clean.png"), imaging.PNGCompressionLevel(0))

	// 2) Overlay đè lên ảnh mockup gốc (soi lệch vị trí: chữ trùng = khớp).
	ref, err := imaging.Open("testdata/mockup_ref.jpg")
	if err == nil {
		ref = imaging.Resize(ref, W, H, imaging.Lanczos)
		cmp := imaging.Overlay(ref, ov, image.Pt(0, 0), 0.85)
		imaging.Save(cmp, filepath.Join(outDir, "chill_over_ref.png"), imaging.JPEGQuality(92))

		// Ảnh ghép cạnh nhau: mockup gốc | render mới (nền tối) — để so trực tiếp.
		side := imaging.New(W*2+30, H, color.NRGBA{20, 20, 24, 255})
		side = imaging.Paste(side, ref, image.Pt(0, 0))
		side = imaging.Paste(side, clean, image.Pt(W+30, 0))
		imaging.Save(side, filepath.Join(outDir, "chill_side.png"), imaging.JPEGQuality(90))
	}

	// ── THÀNH PHẨM THẬT: đường production (renderChromeOverlay có veil + compact giá)
	// đè lên 1 ảnh phòng thật của VTP501 → khung hình video y như khi render.
	prodCfg := cfg
	prodCfg.Prices = compactVideoPrices(cfg.Prices) // đúng như renderVideoTemplateComposite
	prodCfg.Address = cleanVideoAddress(cfg.Address)
	if comp, cerr := renderChromeOverlay(prodCfg, tmp); cerr == nil {
		bg := refV1
		photo := "https://r2.dayladau-cdn.com/file/4757d344e6c4748c5cbce1e8c3632373dc9f4e2cdb8b4f772757cdcf75e0118c"
		dst := filepath.Join(tmp, "vtp501.jpg")
		if p, derr := downloadPhoto(photo, dst); derr == nil {
			bg = p
		}
		if bgimg, e := imaging.Open(bg, imaging.AutoOrientation(true)); e == nil {
			frame := imaging.Fill(bgimg, W, H, imaging.Center, imaging.Lanczos)
			if ovimg, e2 := imaging.Open(comp); e2 == nil {
				frame = imaging.Overlay(frame, ovimg, image.Pt(0, 0), 1.0)
			}
			imaging.Save(frame, filepath.Join(outDir, "chill_final_product.jpg"), imaging.JPEGQuality(92))
			t.Logf("✓ thành phẩm → chill_final_product.jpg")
		}
	} else {
		t.Logf("production render lỗi: %v", cerr)
	}

	t.Logf("✓ rendered → %s (chill_clean.png, chill_over_ref.png, chill_side.png)", outDir)
	_ = fmt.Sprint
	_ = strings.TrimSpace
}
