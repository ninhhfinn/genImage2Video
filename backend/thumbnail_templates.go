package main

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"path/filepath"
	"strings"

	"github.com/disintegration/imaging"
	"github.com/fogleman/gg"
	"img2video/textrender"
)

// 4 layout templates duyệt từ mockup: lưới 2×2 ảnh + khối chữ ở giữa, mỗi kiểu
// một tông màu + cách bày chữ riêng. Chữ vẽ qua textrender (pill/curve/stroke),
// rồi composite lên canvas gg.

type gridSpec struct {
	name       string
	saturation float64     // imaging.AdjustSaturation
	brightness float64     // imaging.AdjustBrightness
	tint       color.NRGBA // lớp phủ màu lên lưới (NRGBA = alpha KHÔNG premultiplied)
}

func gridSpecFor(name string) gridSpec {
	switch strings.ToLower(name) {
	case "daiky":
		return gridSpec{"daiky", -8, -6, color.NRGBA{120, 80, 40, 40}}
	case "valey":
		return gridSpec{"valey", -8, 6, color.NRGBA{225, 200, 165, 30}}
	case "peony":
		return gridSpec{"peony", -62, 10, color.NRGBA{232, 232, 236, 16}}
	case "tiger":
		return gridSpec{"tiger", 6, -8, color.NRGBA{150, 95, 110, 38}}
	}
	return gridSpec{name: name}
}

func buildGridThumbnail(cfg ThumbnailConfig, photos []string) ([]byte, error) {
	W, H := cfg.Width, cfg.Height
	spec := gridSpecFor(cfg.Template)
	dc := gg.NewContext(W, H)

	if spec.name == "valey" {
		drawValeyBg(dc, cfg, photos)
	} else {
		drawGrid2x2(dc, cfg, photos, spec)
	}

	ctx := &textrender.RenderContext{VideoWidth: W, VideoHeight: H, AssetsDir: assetsDir()}
	switch spec.name {
	case "daiky":
		drawDaikyText(dc, cfg, ctx)
	case "valey":
		drawValeyText(dc, cfg, ctx)
	case "peony":
		drawPeonyText(dc, cfg, ctx)
	case "tiger":
		drawTigerText(dc, cfg, ctx)
	}
	drawGridWatermark(dc, cfg, ctx)
	if spec.name != "tiger" { // tiger đã có khối "Link ID phòng…" riêng
		drawThumbListingID(dc, cfg, ctx)
	}

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, dc.Image(), &jpeg.Options{Quality: 92}); err != nil {
		return nil, fmt.Errorf("thumbnail grid: encode: %v", err)
	}
	return buf.Bytes(), nil
}

// drawGrid2x2 fills the canvas with 4 photos (repeat if fewer) + a color tint.
func drawGrid2x2(dc *gg.Context, cfg ThumbnailConfig, photos []string, spec gridSpec) {
	W, H := cfg.Width, cfg.Height
	cw, ch := W/2, H/2
	at := [4][2]int{{0, 0}, {cw, 0}, {0, ch}, {cw, ch}}
	for i := 0; i < 4; i++ {
		img, err := imaging.Open(photos[i%len(photos)], imaging.AutoOrientation(true))
		if err != nil {
			continue
		}
		img = imaging.Fill(img, cw, ch, imaging.Center, imaging.Lanczos)
		if spec.saturation != 0 {
			img = imaging.AdjustSaturation(img, spec.saturation)
		}
		if spec.brightness != 0 {
			img = imaging.AdjustBrightness(img, spec.brightness)
		}
		dc.DrawImage(img, at[i][0], at[i][1])
	}
	if spec.tint.A > 0 {
		dc.SetColor(spec.tint)
		dc.DrawRectangle(0, 0, float64(W), float64(H))
		dc.Fill()
	}
}

// drawValeyBg paints a blurred cream background then a white-framed floating 2×2 grid.
func drawValeyBg(dc *gg.Context, cfg ThumbnailConfig, photos []string) {
	W, H := cfg.Width, cfg.Height
	base := imaging.New(W, H, color.NRGBA{0xec, 0xe2, 0xd2, 255})
	if img, err := imaging.Open(photos[0], imaging.AutoOrientation(true)); err == nil {
		b := imaging.Fill(img, W, H, imaging.Center, imaging.Lanczos)
		b = imaging.Blur(b, 24)
		b = imaging.AdjustBrightness(b, 6)
		b = imaging.AdjustSaturation(b, -12)
		base = imaging.Overlay(base, b, image.Pt(0, 0), 0.88)
	}
	dc.DrawImage(base, 0, 0)

	fW, fH := float64(W), float64(H)
	margin := fW * 0.045
	top := fH * 0.035
	gw := fW - 2*margin
	gh := fH * 0.52 // lưới cao hơn để bảng giá nằm ngay dưới (bớt khoảng trống)
	dc.SetColor(color.RGBA{255, 255, 255, 255})
	dc.DrawRoundedRectangle(margin, top, gw, gh, 16)
	dc.Fill()

	pad := 6.0
	cw := (gw - 3*pad) / 2
	ch := (gh - 3*pad) / 2
	at := [4][2]float64{
		{margin + pad, top + pad}, {margin + 2*pad + cw, top + pad},
		{margin + pad, top + 2*pad + ch}, {margin + 2*pad + cw, top + 2*pad + ch},
	}
	for i := 0; i < 4; i++ {
		img, err := imaging.Open(photos[i%len(photos)], imaging.AutoOrientation(true))
		if err != nil {
			continue
		}
		img = imaging.Fill(img, int(cw), int(ch), imaging.Center, imaging.Lanczos)
		img = imaging.AdjustSaturation(img, -6)
		img = imaging.AdjustBrightness(img, 4)
		dc.DrawImage(img, int(at[i][0]), int(at[i][1]))
	}
}

// ─────────────────────────── text per template ──────────────────────────────

func drawDaikyText(dc *gg.Context, cfg ThumbnailConfig, ctx *textrender.RenderContext) {
	W, H := cfg.Width, cfg.Height
	titlePill := "#6F4A30" // nâu thương hiệu Daiky (applyDefaults đã set PillColor=#000 nên không dùng được)

	tImg, tM := renderEl(&textrender.ElementStyle{
		Text: cfg.Title, FontFile: "PlayfairDisplay-Bold.ttf", SizePct: 0.072 * scaleOr(cfg.TitleScale),
		Color: "#FFFFFF",
		Bg:    &textrender.BgStyle{Color: titlePill, Alpha: 1, Radius: 18, Padding: [2]float64{18, 40}},
		Shadow: &textrender.ShadowStyle{Color: "#000000", Alpha: 0.30, Blur: 12, OffsetY: 5},
		Align: "center", MaxWidthPct: 0.8,
	}, ctx)
	aImg, aM := renderEl(&textrender.ElementStyle{
		Text: strings.TrimSpace(cfg.Address), FontFile: "BeVietnamPro-Bold.ttf", SizePct: 0.0255,
		Color: "#FFFFFF",
		Bg:    &textrender.BgStyle{Color: "#cdb89c", Alpha: 0.92, Radius: 60, Padding: [2]float64{12, 34}},
		Shadow: &textrender.ShadowStyle{Color: "#000000", Alpha: 0.28, Blur: 9, OffsetY: 3},
		Align: "center", Curve: 14, MaxWidthPct: 0.62, // chừa chỗ cho vòng cung uốn
	}, ctx)
	pImg, pM := renderEl(&textrender.ElementStyle{
		Text: strings.Join(trimNonEmpty(cfg.Prices), "- "), FontFile: "BeVietnamPro-Bold.ttf",
		SizePct: 0.023 * scaleOr(cfg.DataScale), Color: "#FFFFFF",
		Bg:    &textrender.BgStyle{Color: "#1a120c", Alpha: 0.66, Radius: 22, Padding: [2]float64{10, 22}},
		Align: "center", MaxWidthPct: 0.86,
	}, ctx)

	ah, th, ph := contentH(aImg, aM), contentH(tImg, tM), contentH(pImg, pM)
	overlapA := int(0.014 * float64(H))
	overlapP := int(0.012 * float64(H))
	total := ah + th + ph - overlapA - overlapP
	aTop := H/2 - total/2
	tTop := aTop + ah - overlapA
	pTop := tTop + th - overlapP
	// z-order: title dưới, address & price phủ lên mép title
	drawCX(dc, tImg, W, tTop, tM)
	drawCX(dc, aImg, W, aTop, aM)
	drawCX(dc, pImg, W, pTop, pM)
}

func drawValeyText(dc *gg.Context, cfg ThumbnailConfig, ctx *textrender.RenderContext) {
	W, H := cfg.Width, cfg.Height
	// title ở giữa lưới (~28% chiều cao)
	tImg, tM := renderEl(&textrender.ElementStyle{
		Text: cfg.Title, FontFile: "BalooBhaijaan2-Bold.ttf", SizePct: 0.085 * scaleOr(cfg.TitleScale),
		Color: orStr(cfg.TitleColor, "#FFFFFF"),
		Stroke: &textrender.StrokeStyle{Color: "#d8bf98", Width: 7},
		Shadow: &textrender.ShadowStyle{Color: "#8a6e3c", Alpha: 0.35, Blur: 10, OffsetY: 4},
		Align: "center", MaxWidthPct: 0.7,
	}, ctx)
	drawCX(dc, tImg, W, int(float64(H)*0.295)-contentH(tImg, tM)/2, tM)

	drawValeyTable(dc, cfg)
}

// drawValeyTable vẽ bảng giá trắng bo góc ở dưới (Khung giờ / Trong tuần / Cuối tuần).
func drawValeyTable(dc *gg.Context, cfg ThumbnailConfig) {
	rows := cfg.PriceTable
	if len(rows) == 0 {
		return
	}
	W, H := cfg.Width, cfg.Height
	fW, fH := float64(W), float64(H)
	margin := fW * 0.045
	left, right := margin, fW-margin
	tableW := right - left
	// Lấp đầy vùng dưới lưới; chừa đáy cho dòng ID. rowH co giãn theo số dòng.
	top := fH * 0.575
	bottom := fH * 0.90 // chừa đáy cho dòng ID (ID pill nâng lên 0.05H)
	total := bottom - top
	rowH := total / float64(len(rows)+1)
	headH := rowH

	dc.SetColor(color.RGBA{255, 255, 255, 255})
	dc.DrawRoundedRectangle(left, top, tableW, total, 12)
	dc.Fill()

	colsX := []float64{left, left + tableW*0.44, left + tableW*0.72, right}
	fontPath := filepath.Join(assetsDir(), "fonts", "BeVietnamPro-Bold.ttf")
	baseSize := fH * 0.0120
	_ = dc.LoadFontFace(fontPath, baseSize)

	// cell vẽ chữ căn theo cột; nếu chuỗi rộng hơn ô thì TỰ CO cỡ chữ cho vừa
	// (giá dài "1.500.000đ" không tràn/đè cột bên cạnh), xong khôi phục cỡ gốc.
	cell := func(txt string, j int, midY float64, head bool) {
		if head {
			dc.SetColor(color.RGBA{74, 53, 33, 255})
		} else {
			dc.SetColor(color.RGBA{90, 70, 49, 255})
		}
		colW := colsX[j+1] - colsX[j]
		avail := colW - 18
		if w, _ := dc.MeasureString(txt); w > avail && avail > 0 {
			_ = dc.LoadFontFace(fontPath, baseSize*avail/w)
			defer dc.LoadFontFace(fontPath, baseSize)
		}
		if j == 0 {
			dc.DrawStringAnchored(txt, colsX[0]+12, midY, 0, 0.42)
		} else {
			dc.DrawStringAnchored(txt, (colsX[j]+colsX[j+1])/2, midY, 0.5, 0.42)
		}
	}
	hdr := []string{"Khung giờ", "Trong tuần", "Cuối tuần"}
	for j, h := range hdr {
		cell(h, j, top+headH/2, true)
	}
	for i, r := range rows {
		ry := top + headH + rowH*float64(i)
		for j, c := range []string{r.Slot, r.Weekday, r.Weekend} {
			cell(c, j, ry+rowH/2, false)
		}
	}
	// đường kẻ — mức "Đậm" đã duyệt: #b0905f, dày hơn cho rõ trên canvas 960px
	dc.SetLineWidth(2.5)
	dc.SetColor(color.RGBA{0xb0, 0x90, 0x5f, 255})
	for i := 1; i <= len(rows); i++ {
		y := top + headH + rowH*float64(i-1)
		dc.DrawLine(left+6, y, right-6, y)
		dc.Stroke()
	}
	dc.DrawLine(left+6, top+headH, right-6, top+headH)
	dc.Stroke()
	for j := 1; j < 3; j++ {
		dc.DrawLine(colsX[j], top+6, colsX[j], bottom-6)
		dc.Stroke()
	}
}

func drawPeonyText(dc *gg.Context, cfg ThumbnailConfig, ctx *textrender.RenderContext) {
	W, H := cfg.Width, cfg.Height
	var boxLines []string
	if pr := strings.Join(trimNonEmpty(cfg.Prices), "- "); pr != "" {
		boxLines = append(boxLines, "• "+pr)
	}
	if am := strings.Join(trimNonEmpty(cfg.Amenities), "- "); am != "" {
		boxLines = append(boxLines, "• "+am)
	}

	tImg, tM := renderEl(&textrender.ElementStyle{
		Text: cfg.Title, FontFile: "BalooBhaijaan2-Bold.ttf", SizePct: 0.072 * scaleOr(cfg.TitleScale),
		Color: orStr(cfg.TitleColor, "#FFFFFF"),
		Stroke: &textrender.StrokeStyle{Color: "#2a2a2e", Width: 3, Alpha: 0.5},
		Shadow: &textrender.ShadowStyle{Color: "#000000", Alpha: 0.4, Blur: 10, OffsetY: 3},
		Align: "center", MaxWidthPct: 0.8,
	}, ctx)
	aImg, aM := renderEl(&textrender.ElementStyle{
		Text: strings.TrimSpace(cfg.Address), FontFile: "BeVietnamPro-Bold.ttf", SizePct: 0.028,
		Color: "#FFFFFF", Shadow: &textrender.ShadowStyle{Color: "#000000", Alpha: 0.55, Blur: 6, OffsetY: 2},
		Align: "center", MaxWidthPct: 0.8,
	}, ctx)
	var bImg image.Image
	var bM int
	if len(boxLines) > 0 {
		bImg, bM = renderEl(&textrender.ElementStyle{
			Text: strings.Join(boxLines, "\n"), FontFile: "BeVietnamPro-Bold.ttf",
			SizePct: 0.020 * scaleOr(cfg.DataScale), Color: "#FFFFFF",
			Bg:    &textrender.BgStyle{Color: "#28282e", Alpha: 0.62, Radius: 14, Padding: [2]float64{16, 20}},
			Align: "left", LineSpacing: 1.5, MaxWidthPct: 0.82,
		}, ctx)
	}

	gap := int(0.008 * float64(H))
	th, ah, bh := contentH(tImg, tM), contentH(aImg, aM), contentH(bImg, bM)
	total := th + gap + ah + gap + bh
	cur := int(float64(H)*0.57) - total/2
	drawCX(dc, tImg, W, cur, tM)
	cur += th + gap
	drawCX(dc, aImg, W, cur, aM)
	cur += ah + gap
	drawCX(dc, bImg, W, cur, bM)
}

func drawTigerText(dc *gg.Context, cfg ThumbnailConfig, ctx *textrender.RenderContext) {
	W, H := cfg.Width, cfg.Height
	tImg, tM := renderEl(&textrender.ElementStyle{
		Text: cfg.Title, FontFile: "BalooBhaijaan2-Bold.ttf", SizePct: 0.082 * scaleOr(cfg.TitleScale),
		Color: orStr(cfg.TitleColor, "#FFFFFF"),
		Stroke: &textrender.StrokeStyle{Color: orStr(cfg.StrokeColor, "#7a4a2a"), Width: 5},
		Shadow: &textrender.ShadowStyle{Color: "#000000", Alpha: 0.35, Blur: 9, OffsetY: 4},
		Align: "center", MaxWidthPct: 0.85,
	}, ctx)
	pill := func(txt string, size float64) (image.Image, int) {
		if strings.TrimSpace(txt) == "" {
			return nil, 0
		}
		return renderEl(&textrender.ElementStyle{
			Text: txt, FontFile: "BeVietnamPro-Bold.ttf", SizePct: size, Color: "#FFFFFF",
			Bg:    &textrender.BgStyle{Color: "#1a1410", Alpha: 0.62, Radius: 8, Padding: [2]float64{9, 20}},
			Align: "center", MaxWidthPct: 0.9,
		}, ctx)
	}
	aImg, aM := pill(strings.TrimSpace(cfg.Address), 0.022)
	pImg, pM := pill(strings.Join(trimNonEmpty(cfg.Prices), "    "), 0.022*scaleOr(cfg.DataScale))
	var idImg image.Image
	var idM int
	if id := strings.TrimSpace(cfg.ListingID); id != "" {
		idImg, idM = renderEl(&textrender.ElementStyle{
			Text: "Link ID phòng trên Dayladau\n" + id, FontFile: "BeVietnamPro-Italic.ttf",
			SizePct: 0.022, Color: "#FFFFFF",
			Shadow: &textrender.ShadowStyle{Color: "#000000", Alpha: 0.6, Blur: 6, OffsetY: 2},
			Align: "center", LineSpacing: 1.3, MaxWidthPct: 0.9,
		}, ctx)
	}

	gap := int(0.009 * float64(H))
	items := []struct {
		img image.Image
		m   int
	}{{tImg, tM}, {aImg, aM}, {pImg, pM}, {idImg, idM}}
	total := 0
	n := 0
	for _, it := range items {
		if h := contentH(it.img, it.m); h > 0 {
			total += h
			n++
		}
	}
	if n > 1 {
		total += gap * (n - 1)
	}
	cur := H/2 - total/2
	for _, it := range items {
		h := contentH(it.img, it.m)
		if h <= 0 {
			continue
		}
		drawCX(dc, it.img, W, cur, it.m)
		cur += h + gap
	}
}

// drawThumbListingID vẽ mã ID listing nhỏ, canh giữa-đáy thumbnail trên 1 pill
// tối mờ (đọc rõ trên cả nền kem lẫn nền ảnh). Bỏ qua nếu trống. Tiger tự render
// ID riêng trong khối chữ nên KHÔNG gọi hàm này.
func drawThumbListingID(dc *gg.Context, cfg ThumbnailConfig, ctx *textrender.RenderContext) {
	id := strings.TrimSpace(cfg.ListingID)
	if id == "" {
		return
	}
	W, H := cfg.Width, cfg.Height
	img, m := renderEl(&textrender.ElementStyle{
		Text: "ID: " + id, FontFile: "BeVietnamPro-Bold.ttf", SizePct: 0.021, Color: "#FFFFFF",
		Bg:     &textrender.BgStyle{Color: "#000000", Alpha: 0.5, Radius: 14, Padding: [2]float64{7, 18}},
		Shadow: &textrender.ShadowStyle{Color: "#000000", Alpha: 0.4, Blur: 5, OffsetY: 1},
		Align:  "center", MaxWidthPct: 0.9,
	}, ctx)
	if img == nil {
		return
	}
	y := H - int(0.05*float64(H)) - contentH(img, m) // nâng khỏi mép đáy
	drawCX(dc, img, W, y, m)
}

func drawGridWatermark(dc *gg.Context, cfg ThumbnailConfig, ctx *textrender.RenderContext) {
	if strings.TrimSpace(cfg.Watermark) == "" {
		return
	}
	W, H := cfg.Width, cfg.Height
	img, m := renderEl(&textrender.ElementStyle{
		Text: cfg.Watermark, FontFile: "BeVietnamPro-Bold.ttf", SizePct: 0.021, Color: "#FFFFFF",
		Shadow: &textrender.ShadowStyle{Color: "#000000", Alpha: 0.6, Blur: 6, OffsetY: 2},
		Align: "right",
	}, ctx)
	if img == nil {
		return
	}
	cw, chh := img.Bounds().Dx()-2*m, img.Bounds().Dy()-2*m
	x := W - int(float64(W)*0.05) - cw
	y := H - int(float64(H)*0.05) - chh
	note := float64(W) * 0.026
	drawMusicNote(dc, float64(x)-note-8, float64(y)-2, note, color.RGBA{255, 255, 255, 255})
	dc.DrawImage(img, x-m, y-m)
}

// ─────────────────────────── helpers ────────────────────────────────────────

func renderEl(st *textrender.ElementStyle, ctx *textrender.RenderContext) (image.Image, int) {
	if strings.TrimSpace(st.Text) == "" {
		return nil, 0
	}
	out, err := textrender.Render(st, ctx)
	if err != nil || out == nil {
		return nil, 0
	}
	img, err := png.Decode(bytes.NewReader(out.PNG))
	if err != nil {
		return nil, 0
	}
	return img, styleMargin(st)
}

// contentH = chiều cao phần nội dung (trừ margin shadow/stroke 2 bên).
func contentH(img image.Image, m int) int {
	if img == nil {
		return 0
	}
	return img.Bounds().Dy() - 2*m
}

// drawCX vẽ img sao cho phần nội dung canh giữa ngang, mép-trên nội dung tại y.
func drawCX(dc *gg.Context, img image.Image, W, y, m int) {
	if img == nil {
		return
	}
	dc.DrawImage(img, (W-img.Bounds().Dx())/2, y-m)
}

func trimNonEmpty(xs []string) []string {
	var out []string
	for _, x := range xs {
		if x = strings.TrimSpace(x); x != "" {
			out = append(out, x)
		}
	}
	return out
}

// maxAmenities — số tiện ích tối đa hiển thị để khối tiện ích không tràn/đè (lưới
// an toàn). Frontend đã lọc theo whitelist ưu tiên (≤7 mục); đây là chốt chặn an
// toàn cho cả đường thủ công. Áp ở buildThumbnailImage và renderTextOverlays.
const maxAmenities = 7

// capAmenities trim, bỏ rỗng rồi giữ tối đa n mục đầu tiên.
func capAmenities(xs []string, n int) []string {
	out := trimNonEmpty(xs)
	if n > 0 && len(out) > n {
		out = out[:n]
	}
	return out
}

func orStr(v, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return v
}

func scaleOr(v float64) float64 {
	if v <= 0 {
		return 1
	}
	return v
}
