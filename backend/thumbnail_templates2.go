package main

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"strings"

	"github.com/disintegration/imaging"
	"github.com/fogleman/gg"
	"img2video/textrender"
)

// 5 layout templates bổ sung, dựng từ ảnh mockup người dùng cung cấp:
//   cento     — lưới 2×2 + nhãn combo ở 4 góc + thanh thông tin nâu ở giữa
//   amber     — 1 ảnh hero, panel giá góc trên-trái, tiêu đề serif lớn + pill địa chỉ
//   strip     — tiêu đề trên cùng + dải 3 ảnh ngang ở giữa + giá phía dưới
//   creamgrid — nền kem, tiêu đề serif đỏ, lưới 2×2 bo góc, giá serif đỏ
//   filmstrip — 3 dải ảnh xếp dọc, dải giữa phủ chữ (tiêu đề + tagline + giá)
//
// Mỗi hàm vẽ trực tiếp lên canvas gg rồi encode JPEG. Chữ đi qua textrender
// (renderEl) để đồng bộ pill/stroke/shadow với phần còn lại của hệ thống.

func isExtraThumbTemplate(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "cento", "amber", "strip", "creamgrid", "filmstrip":
		return true
	}
	return false
}

func buildExtraThumbnail(cfg ThumbnailConfig, photos []string) ([]byte, error) {
	W, H := cfg.Width, cfg.Height
	dc := gg.NewContext(W, H)
	ctx := &textrender.RenderContext{VideoWidth: W, VideoHeight: H, AssetsDir: assetsDir()}

	switch strings.ToLower(strings.TrimSpace(cfg.Template)) {
	case "cento":
		drawCento(dc, cfg, ctx, photos)
	case "amber":
		drawAmber(dc, cfg, ctx, photos)
	case "strip":
		drawStrip(dc, cfg, ctx, photos)
	case "creamgrid":
		drawCreamGrid(dc, cfg, ctx, photos)
	case "filmstrip":
		drawFilmstrip(dc, cfg, ctx, photos)
	}
	drawGridWatermark(dc, cfg, ctx)
	drawThumbListingID(dc, cfg, ctx)

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, dc.Image(), &jpeg.Options{Quality: 92}); err != nil {
		return nil, fmt.Errorf("thumbnail extra: encode: %v", err)
	}
	return buf.Bytes(), nil
}

// ─────────────────────────── helpers ─────────────────────────────────────────

// photoCell crop-fills a photo into a rounded rectangle at (x,y,w,h).
func photoCell(dc *gg.Context, path string, x, y, w, h, radius float64, sat, bri float64) {
	img, err := imaging.Open(path, imaging.AutoOrientation(true))
	if err != nil {
		return
	}
	filled := imaging.Fill(img, int(w), int(h), imaging.Center, imaging.Lanczos)
	if sat != 0 {
		filled = imaging.AdjustSaturation(filled, sat)
	}
	if bri != 0 {
		filled = imaging.AdjustBrightness(filled, bri)
	}
	if radius > 0 {
		dc.DrawRoundedRectangle(x, y, w, h, radius)
		dc.Clip()
		dc.DrawImage(filled, int(x), int(y))
		dc.ResetClip()
		return
	}
	dc.DrawImage(filled, int(x), int(y))
}

// drawAt composites a textrender PNG so its CONTENT top-left lands at (x,y).
func drawAt(dc *gg.Context, img image.Image, x, y, m int) {
	if img == nil {
		return
	}
	dc.DrawImage(img, x-m, y-m)
}

// drawAtClamped vẽ như drawAt nhưng kéo hộp NỘI DUNG vào trong khung an toàn:
// mép phải ≤ W-insetR, mép dưới ≤ H-insetB (và không âm). Dùng cho các phần tử
// composite căn mép/lệch tay có thể tràn ra ngoài khung video.
func drawAtClamped(dc *gg.Context, img image.Image, x, y, m, W, H, insetR, insetB int) {
	if img == nil {
		return
	}
	cw := img.Bounds().Dx() - 2*m
	ch := img.Bounds().Dy() - 2*m
	if x+cw > W-insetR {
		x = W - insetR - cw
	}
	if x < 0 {
		x = 0
	}
	if y+ch > H-insetB {
		y = H - insetB - ch
	}
	if y < 0 {
		y = 0
	}
	dc.DrawImage(img, x-m, y-m)
}

// vGradient paints a vertical gradient rectangle (top→bottom colors).
func vGradient(dc *gg.Context, x, y, w, h float64, top, bot color.Color) {
	g := gg.NewLinearGradient(0, y, 0, y+h)
	g.AddColorStop(0, top)
	g.AddColorStop(1, bot)
	dc.SetFillStyle(g)
	dc.DrawRectangle(x, y, w, h)
	dc.Fill()
}

// ─────────────────────────── cento ───────────────────────────────────────────

func drawCento(dc *gg.Context, cfg ThumbnailConfig, ctx *textrender.RenderContext, photos []string) {
	W, H := float64(cfg.Width), float64(cfg.Height)
	cw, ch := W/2, H/2
	at := [4][2]float64{{0, 0}, {cw, 0}, {0, ch}, {cw, ch}}
	for i := 0; i < 4; i++ {
		photoCell(dc, photos[i%len(photos)], at[i][0], at[i][1], cw, ch, 0, -6, -10)
	}
	// nhẹ tay phủ tím để tông gần ref + tăng tương phản chữ trắng góc
	dc.SetColor(color.NRGBA{40, 30, 60, 26})
	dc.DrawRectangle(0, 0, W, H)
	dc.Fill()

	// nhãn combo 4 góc lấy từ Prices (mỗi dòng "Tiêu đề\nGiá")
	labels := trimNonEmpty(cfg.Prices)
	corners := []struct {
		x, y  float64
		ax, ay float64 // align anchor (0 left/top, 1 right/bottom)
	}{
		{0.05 * W, 0.05 * H, 0, 0},
		{0.95 * W, 0.05 * H, 1, 0},
		{0.05 * W, 0.95 * H, 0, 1}, // dưới-trái: canh ĐÁY (đối xứng nhãn trên), khỏi đè panel/nối ảnh
		{0.95 * W, 0.95 * H, 1, 1}, // dưới-phải: canh ĐÁY
	}
	for i := 0; i < len(labels) && i < 4; i++ {
		al := "left"
		if corners[i].ax == 1 {
			al = "right"
		}
		img, m := renderEl(&textrender.ElementStyle{
			Text: labels[i], FontFile: "BeVietnamPro-Bold.ttf", SizePct: 0.026 * scaleOr(cfg.DataScale),
			Color: "#FFFFFF",
			Shadow: &textrender.ShadowStyle{Color: "#000000", Alpha: 0.6, Blur: 7, OffsetY: 2},
			Align: al, LineSpacing: 1.15, MaxWidthPct: 0.42,
		}, ctx)
		if img == nil {
			continue
		}
		cwid := img.Bounds().Dx() - 2*m
		x := int(corners[i].x)
		if corners[i].ax == 1 {
			x = int(corners[i].x) - cwid
		}
		y := int(corners[i].y)
		if corners[i].ay == 1 { // canh ĐÁY: corners[i].y là mép DƯỚI của nội dung
			y -= contentH(img, m)
		}
		drawAt(dc, img, x, y, m)
	}

	// (Bỏ badge "3/19" hardcode — nó đè lên nhãn giá góc phải và là số giả.)

	// thanh thông tin nâu ở giữa — canh TRÁI, có icon nhà/ghim/sao.
	// Fredoka-Bold KHÔNG có dấu tiếng Việt (tiêu đề bị tofu) → dùng Baloo2-Bold.
	titleImg, tM := renderEl(&textrender.ElementStyle{Text: strings.ToUpper(cfg.Title), FontFile: "Baloo2-Bold.ttf",
		SizePct: 0.038 * scaleOr(cfg.TitleScale), Color: "#FFFFFF", Align: "left", MaxWidthPct: 0.78}, ctx)
	addrImg, aM := renderEl(&textrender.ElementStyle{Text: strings.TrimSpace(cfg.Address),
		FontFile: "BeVietnamPro-Bold.ttf", SizePct: 0.021, Color: "#F0E0CC", Align: "left", MaxWidthPct: 0.78}, ctx)
	hdrImg, hM := renderEl(&textrender.ElementStyle{Text: "Tiện ích:", FontFile: "BeVietnamPro-Bold.ttf",
		SizePct: 0.021, Color: "#FFD24A", Align: "left"}, ctx)
	am := trimNonEmpty(cfg.Amenities)
	var amRows []string
	for i := 0; i < len(am); i += 3 {
		end := i + 3
		if end > len(am) {
			end = len(am)
		}
		var parts []string
		for _, it := range am[i:end] {
			parts = append(parts, "•  "+it)
		}
		amRows = append(amRows, strings.Join(parts, "     "))
	}
	amImg, amM := renderEl(&textrender.ElementStyle{Text: strings.Join(amRows, "\n"),
		FontFile: "BeVietnamPro-Bold.ttf", SizePct: 0.017, Color: "#F4E7D6", Align: "left", LineSpacing: 1.5, MaxWidthPct: 0.94}, ctx)

	th := contentH(titleImg, tM)
	lineH := contentH(addrImg, aM)
	iconGap := int(0.016 * W)
	innerW := maxInt(maxInt(th+iconGap+contentW(titleImg, tM), lineH+iconGap+contentW(addrImg, aM)),
		maxInt(lineH+iconGap+contentW(hdrImg, hM), contentW(amImg, amM)))
	gap := int(0.013 * H)
	padV, padH := int(0.026*H), int(0.045*W)
	total := th + gap + lineH + gap + contentH(hdrImg, hM) + gap + contentH(amImg, amM)
	panelW := float64(innerW + 2*padH)
	panelH := float64(total + 2*padV)
	px := (W - panelW) / 2
	py := H/2 - panelH/2
	// Panel nâu MỜ (alpha 135/255 ≈ 53%) để vẫn thấy ảnh phía sau, không che kín.
	dc.SetColor(color.NRGBA{74, 46, 30, 135})
	dc.DrawRoundedRectangle(px, py, panelW, panelH, 20)
	dc.Fill()

	cl := int(px) + padH
	cy := int(py) + padV
	drawHouseIcon(dc, float64(cl)+float64(th)*0.45, float64(cy)+float64(th)*0.5, float64(th)*0.9, color.NRGBA{255, 209, 74, 255})
	drawAt(dc, titleImg, cl+th+iconGap, cy, tM)
	cy += th + gap
	drawPinIcon(dc, float64(cl)+float64(lineH)*0.4, float64(cy), float64(lineH), color.NRGBA{0xE0, 0x4A, 0x3A, 255})
	drawAt(dc, addrImg, cl+lineH+iconGap, cy, aM)
	cy += lineH + gap
	drawStarIcon(dc, float64(cl)+float64(lineH)*0.42, float64(cy)+float64(lineH)*0.5, float64(lineH)*0.5, color.NRGBA{255, 209, 74, 255})
	drawAt(dc, hdrImg, cl+lineH+iconGap, cy, hM)
	cy += contentH(hdrImg, hM) + gap
	drawAt(dc, amImg, cl, cy, amM)
}

// ─────────────────────────── amber ───────────────────────────────────────────

func drawAmber(dc *gg.Context, cfg ThumbnailConfig, ctx *textrender.RenderContext, photos []string) {
	W, H := float64(cfg.Width), float64(cfg.Height)
	// hero
	if img, err := imaging.Open(photos[0], imaging.AutoOrientation(true)); err == nil {
		hero := imaging.Fill(img, cfg.Width, cfg.Height, imaging.Center, imaging.Lanczos)
		hero = imaging.AdjustBrightness(hero, -10)
		dc.DrawImage(hero, 0, 0)
	}
	// scrim đáy cho tiêu đề + pill địa chỉ
	vGradient(dc, 0, H*0.55, W, H*0.45, color.NRGBA{10, 6, 4, 0}, color.NRGBA{10, 6, 4, 150})

	// panel giá góc trên-trái (panel nâu đặc bo góc, theo bản gốc Lagom)
	if pr := strings.Join(trimNonEmpty(cfg.Prices), "\n"); pr != "" {
		img, m := renderEl(&textrender.ElementStyle{
			Text: pr, FontFile: "BeVietnamPro-Bold.ttf", SizePct: 0.0235 * scaleOr(cfg.DataScale),
			Color: "#FFFFFF",
			Bg:    &textrender.BgStyle{Color: "#7B4A38", Alpha: 0.80, Radius: 16, Padding: [2]float64{16, 24}},
			Align: "left", LineSpacing: 1.6, MaxWidthPct: 0.55,
		}, ctx)
		drawAt(dc, img, int(0.05*W), int(0.045*H), m)
	}

	// cụm góc trên-phải: wordmark thương hiệu + badge số trang (vd "lag" "1/4").
	// Chỉ vẽ khi được cấu hình — mặc định rỗng nên không ảnh hưởng dữ liệu cũ.
	drawAmberPageBadge(dc, cfg, ctx, W, H)

	// tiêu đề serif lớn canh trái-dưới. Dùng fit-to-width để TỰ CO vừa khung:
	// tên dài 1 từ (vd "mangoapartment") không wrap được nên trước đây tràn viền.
	titleImg, tM := renderElFitWidth(&textrender.ElementStyle{
		Text: cfg.Title, FontFile: "Fraunces-DisplaySoft.ttf", SizePct: 0.135 * scaleOr(cfg.TitleScale),
		Color: orStr(cfg.TitleColor, "#FFFFFF"),
		Shadow: &textrender.ShadowStyle{Color: "#000000", Alpha: 0.5, Blur: 12, OffsetY: 4},
		Align: "left",
	}, ctx, 0.90)
	titleTop := int(0.64 * H)
	drawAt(dc, titleImg, int(0.05*W), titleTop, tM)
	titleBottom := titleTop + contentH(titleImg, tM)
	titleRight := int(0.05*W) + contentW(titleImg, tM)

	// pill cam "Room" dưới-phải tiêu đề (Fraunces đồng bộ tiêu đề)
	if roomImg, rm := renderEl(&textrender.ElementStyle{Text: "Room", FontFile: "Fraunces-DisplaySoft.ttf",
		SizePct: 0.05, Color: "#FFFFFF",
		Bg: &textrender.BgStyle{Color: "#E8852A", Alpha: 1, Radius: 26, Padding: [2]float64{10, 26}}, Align: "center"}, ctx); roomImg != nil {
		drawAt(dc, roomImg, titleRight-contentW(roomImg, rm), titleBottom-int(0.012*H), rm)
	}
	// (Bỏ badge "1/4" hardcode — số giả, đè nhãn giá góc phải.)

	// pill địa chỉ xanh ở dưới-giữa (chữ HOA + ghim)
	if addr := strings.TrimSpace(cfg.Address); addr != "" {
		img, m := renderEl(&textrender.ElementStyle{
			Text: "         " + strings.ToUpper(addr), FontFile: "BeVietnamPro-Bold.ttf", SizePct: 0.023,
			Color: "#FFFFFF",
			Bg:    &textrender.BgStyle{Color: "#2E7D52", Alpha: 0.85, Radius: 40, Padding: [2]float64{12, 30}},
			Align: "center", MaxWidthPct: 0.92,
		}, ctx)
		if img != nil {
			lh := contentH(img, m)
			px := (cfg.Width - (img.Bounds().Dx() - 2*m)) / 2
			py := int(0.86 * H) // nâng khỏi mép đáy để địa chỉ wrap 2 dòng không lọt khung
			dc.DrawImage(img, px-m, py-m)
			drawPinIcon(dc, float64(px)+float64(lh)*0.5, float64(py)+float64(lh)*0.13, float64(lh)*0.6, color.NRGBA{255, 255, 255, 255})
		}
	}
}

// drawAmberPageBadge vẽ cụm góc trên-phải: wordmark thương hiệu + badge số trang
// (vd "lag" và "1/4"), mô phỏng nhãn carousel của bản gốc Lagom. Chỉ vẽ khi có
// cấu hình; cả hai rỗng = không vẽ gì (giữ tương thích dữ liệu cũ).
func drawAmberPageBadge(dc *gg.Context, cfg ThumbnailConfig, ctx *textrender.RenderContext, W, H float64) {
	badge := strings.TrimSpace(cfg.PageBadge)
	brand := strings.TrimSpace(cfg.Brand)
	if badge == "" && brand == "" {
		return
	}
	rightX := int(0.955 * W)
	topY := int(0.038 * H)
	leftEdge := rightX

	// badge số trang: ô bo góc kem, chữ nâu đậm, sát mép phải
	if badge != "" {
		img, m := renderEl(&textrender.ElementStyle{
			Text: badge, FontFile: "BeVietnamPro-Bold.ttf", SizePct: 0.026,
			Color: "#5A3A2A",
			Bg:    &textrender.BgStyle{Color: "#E7D8BE", Alpha: 0.92, Radius: 12, Padding: [2]float64{8, 14}},
			Align: "center",
		}, ctx)
		if img != nil {
			w := contentW(img, m)
			drawAt(dc, img, rightX-w, topY, m)
			leftEdge = rightX - w
		}
	}

	// wordmark thương hiệu: serif trắng có đổ bóng, đặt bên trái badge
	if brand != "" {
		img, m := renderEl(&textrender.ElementStyle{
			Text: brand, FontFile: "PlayfairDisplay-Bold.ttf", SizePct: 0.04,
			Color:  "#FFFFFF",
			Shadow: &textrender.ShadowStyle{Color: "#000000", Alpha: 0.45, Blur: 8, OffsetY: 2},
			Align:  "right",
		}, ctx)
		if img != nil {
			w := contentW(img, m)
			gap := int(0.012 * W)
			drawAt(dc, img, leftEdge-gap-w, topY-int(0.006*H), m)
		}
	}
}

// ─────────────────────────── strip ───────────────────────────────────────────

func drawStrip(dc *gg.Context, cfg ThumbnailConfig, ctx *textrender.RenderContext, photos []string) {
	W, H := float64(cfg.Width), float64(cfg.Height)
	if img, err := imaging.Open(photos[0], imaging.AutoOrientation(true)); err == nil {
		bg := imaging.Fill(img, cfg.Width, cfg.Height, imaging.Center, imaging.Lanczos)
		bg = imaging.AdjustBrightness(bg, -30)
		bg = imaging.Blur(bg, 3)
		dc.DrawImage(bg, 0, 0)
	}
	leftX := int(0.055 * W)
	shadow := func() *textrender.ShadowStyle {
		return &textrender.ShadowStyle{Color: "#000000", Alpha: 0.6, Blur: 8, OffsetY: 2}
	}

	// tiêu đề + địa chỉ canh TRÁI trên cùng
	titleImg, tM := renderEl(&textrender.ElementStyle{
		Text: cfg.Title, FontFile: "PlayfairDisplay-Bold.ttf", SizePct: 0.076 * scaleOr(cfg.TitleScale),
		Color: orStr(cfg.TitleColor, "#FFFFFF"), Shadow: shadow(), Align: "left", MaxWidthPct: 0.9}, ctx)
	drawAt(dc, titleImg, leftX, int(0.05*H), tM)
	addrImg, aM := renderEl(&textrender.ElementStyle{
		Text: strings.TrimSpace(cfg.Address), FontFile: "BeVietnamPro-Bold.ttf", SizePct: 0.024,
		Color: "#FFFFFF", Shadow: shadow(), Align: "left", MaxWidthPct: 0.9}, ctx)
	addrTop := int(0.05*H) + contentH(titleImg, tM) + int(0.008*H)
	drawAt(dc, addrImg, leftX, addrTop, aM)

	// giá canh TRÁI, ngay dưới địa chỉ
	prImg, prM := renderEl(&textrender.ElementStyle{
		Text: strings.Join(trimNonEmpty(cfg.Prices), "\n"), FontFile: "PlayfairDisplay-Bold.ttf",
		SizePct: 0.030 * scaleOr(cfg.DataScale), Color: "#FFFFFF", Shadow: shadow(),
		Align: "left", LineSpacing: 1.5, MaxWidthPct: 0.9}, ctx)
	drawAt(dc, prImg, leftX, addrTop+contentH(addrImg, aM)+int(0.02*H), prM)

	// dải 3 ảnh ngang ở giữa-dưới
	margin := 0.05 * W
	gap := 0.018 * W
	stripTop := 0.47 * H
	stripH := 0.24 * H
	cellW := (W - 2*margin - 2*gap) / 3
	for i := 0; i < 3; i++ {
		x := margin + float64(i)*(cellW+gap)
		photoCell(dc, photos[i%len(photos)], x, stripTop, cellW, stripH, 10, 0, 0)
	}

	// phụ đề tiện nghi ở ĐÁY, canh trái, dấu phẩy
	amImg, amM := renderEl(&textrender.ElementStyle{
		Text: strings.Join(trimNonEmpty(cfg.Amenities), ", "), FontFile: "BeVietnamPro-Bold.ttf", SizePct: 0.0245,
		Color: "#FFFFFF", Shadow: shadow(), Align: "left", MaxWidthPct: 0.9}, ctx)
	drawAt(dc, amImg, leftX, int(stripTop+stripH)+int(0.045*H), amM)
}

// ─────────────────────────── creamgrid ───────────────────────────────────────

func drawCreamGrid(dc *gg.Context, cfg ThumbnailConfig, ctx *textrender.RenderContext, photos []string) {
	W, H := float64(cfg.Width), float64(cfg.Height)
	dc.SetColor(color.NRGBA{0xfd, 0xf3, 0xdc, 255})
	dc.DrawRectangle(0, 0, W, H)
	dc.Fill()

	// Nền kem ⇒ chữ ĐỎ CORAL đậm. applyDefaults đã ép TitleColor=#FFFFFF nên chỉ
	// nhận màu tuỳ biến khi người dùng đặt khác trắng.
	red := "#C0392B"
	if c := strings.TrimSpace(cfg.TitleColor); c != "" && !strings.EqualFold(c, "#FFFFFF") {
		red = c
	}

	// PlayfairDisplay-Bold: bản static bake từ font variable (decompose glyph +
	// số lining) — variable gốc qua freetype-go bị mảnh, số old-style (605 →
	// 6o5); Prata thì metrics Latin làm dấu chồng VN bị cắt (đã vá thêm ở
	// textrender bằng glyphInkTop).
	titleImg, tM := renderElFitWidth(&textrender.ElementStyle{
		Text: cfg.Title, FontFile: "PlayfairDisplay-Bold.ttf", SizePct: 0.105 * scaleOr(cfg.TitleScale),
		Color: red, Align: "center",
	}, ctx, 0.86)
	drawCX(dc, titleImg, cfg.Width, int(0.035*H), tM)
	addrImg, aM := renderEl(&textrender.ElementStyle{
		Text: strings.TrimSpace(cfg.Address), FontFile: "PlayfairDisplay-Bold.ttf", SizePct: 0.031,
		Color: red, Align: "center", MaxWidthPct: 0.9,
	}, ctx)
	addrTop := int(0.035*H) + contentH(titleImg, tM) + int(0.012*H)
	drawCX(dc, addrImg, cfg.Width, addrTop, aM)

	// Mosaic 2×2 lệch cột như bản gốc (trên: trái rộng – phải hẹp; dưới: trái
	// hẹp – phải rộng), cao gần hết khung, GÓC VUÔNG.
	margin := 0.05 * W
	gap := 0.012 * W
	// Lưới bắt đầu NGAY DƯỚI địa chỉ thật: địa chỉ dài (wrap 2 dòng) sẽ đẩy lưới
	// xuống thay vì bị đè lên. Đáy lưới giữ cố định 0.77H để khối tiện ích/giá
	// phía dưới không xê dịch; lưới chỉ co lại khi address chiếm nhiều dòng.
	addrBottom := float64(addrTop + contentH(addrImg, aM))
	gridTop := 0.18 * H
	if minTop := addrBottom + 0.018*H; minTop > gridTop {
		gridTop = minTop
	}
	gridBottom := 0.77 * H
	gridH := gridBottom - gridTop
	availW := W - 2*margin - gap
	cellH := (gridH - gap) / 2
	topL, botL := 0.57*availW, 0.41*availW
	rowY := [2]float64{gridTop, gridTop + cellH + gap}
	cells := [4][3]float64{ // x, y, w
		{margin, rowY[0], topL}, {margin + topL + gap, rowY[0], availW - topL},
		{margin, rowY[1], botL}, {margin + botL + gap, rowY[1], availW - botL},
	}
	for i := 0; i < 4; i++ {
		photoCell(dc, photos[i%len(photos)], cells[i][0], cells[i][1], cells[i][2], cellH, 0, -4, 2)
	}

	// caption serif trắng đè đầu ô phải-trên ("Disco Room" trong bản gốc) +
	// 2 dòng credit li ti kiểu bìa tạp chí ngay dưới — chỉ là texture trang
	// trí, cố ý nhỏ tới mức không đọc được ở cỡ thumbnail.
	if capt := strings.TrimSpace(cfg.Caption); capt != "" {
		capImg, cM := renderElFitWidth(&textrender.ElementStyle{
			Text: capt, FontFile: "PlayfairDisplay-Bold.ttf", SizePct: 0.05,
			Color:  "#FFFFFF",
			Shadow: &textrender.ShadowStyle{Color: "#000000", Alpha: 0.4, Blur: 5, OffsetY: 2},
			Align:  "center",
		}, ctx, (availW-topL)*0.85/W)
		cx := cells[1][0] + (cells[1][2]-float64(contentW(capImg, cM)))/2
		capY := int(rowY[0] + 0.018*H)
		drawAt(dc, capImg, int(cx), capY, cM)

		crImg, crM := renderEl(&textrender.ElementStyle{
			Text:     "COPYRIGHT ALL RIGHTS RESERVED\nPHOTOGRAPHY · RECORDS YOUR DAILY STORIES",
			FontFile: "BeVietnamPro-Regular.ttf", SizePct: 0.0105,
			Color: "#FFFFFFB8", Align: "center", LineSpacing: 1.6,
		}, ctx)
		crX := cells[1][0] + (cells[1][2]-float64(contentW(crImg, crM)))/2
		drawAt(dc, crImg, int(crX), capY+contentH(capImg, cM)+int(0.007*H), crM)
	}

	cy := int(gridTop+gridH) + int(0.022*H)
	amImg, amM := renderEl(&textrender.ElementStyle{
		Text: strings.Join(trimNonEmpty(cfg.Amenities), ", "), FontFile: "PlayfairDisplay-Bold.ttf", SizePct: 0.028,
		Color: red, Align: "center", MaxWidthPct: 0.9,
	}, ctx)
	drawCX(dc, amImg, cfg.Width, cy, amM)
	cy += contentH(amImg, amM) + int(0.016*H)

	prImg, prM := renderEl(&textrender.ElementStyle{
		Text: strings.Join(trimNonEmpty(cfg.Prices), "\n"), FontFile: "PlayfairDisplay-Bold.ttf", SizePct: 0.032 * scaleOr(cfg.DataScale),
		Color: red, Align: "center", LineSpacing: 1.5, MaxWidthPct: 0.9,
	}, ctx)
	drawCX(dc, prImg, cfg.Width, cy, prM)
}

// ─────────────────────────── filmstrip ───────────────────────────────────────

func drawFilmstrip(dc *gg.Context, cfg ThumbnailConfig, ctx *textrender.RenderContext, photos []string) {
	W, H := float64(cfg.Width), float64(cfg.Height)
	_ = W
	bandH := H / 3
	for i := 0; i < 3; i++ {
		photoCell(dc, photos[i%len(photos)], 0, float64(i)*bandH, W, bandH, 0, 2, -6)
	}
	// Bản gốc KHÔNG phủ tối dải giữa — chữ tách nền nhờ shadow từng dòng.

	// địa chỉ serif đậm UỐN CONG vòng cung phía TRÊN tiêu đề
	addrImg, aM := renderEl(&textrender.ElementStyle{
		Text: strings.TrimSpace(cfg.Address), FontFile: "PlayfairDisplay-Bold.ttf", SizePct: 0.038,
		Color:  "#F6F2ED",
		Shadow: &textrender.ShadowStyle{Color: "#000000", Alpha: 0.6, Blur: 6, OffsetY: 2},
		Align:  "center", MaxWidthPct: 0.8, Curve: 12, // chừa chỗ cho vòng cung uốn
	}, ctx)
	titleImg, tM := renderElFitWidth(&textrender.ElementStyle{
		Text: cfg.Title, FontFile: "PlayfairDisplay-Bold.ttf", SizePct: 0.10 * scaleOr(cfg.TitleScale),
		Color:  orStr(cfg.TitleColor, "#F9F6F2"),
		Shadow: &textrender.ShadowStyle{Color: "#000000", Alpha: 0.55, Blur: 9, OffsetY: 3},
		Align:  "center",
	}, ctx, 0.84)
	// tagline serif với chuỗi gạch dưới liền chữ ở baseline như bản gốc
	tagImg, tagM := renderEl(&textrender.ElementStyle{
		Text: "___Trạm sạc cảm xúc___", FontFile: "PlayfairDisplay-Bold.ttf", SizePct: 0.026,
		Color:  "#FFFFFF",
		Shadow: &textrender.ShadowStyle{Color: "#000000", Alpha: 0.55, Blur: 6, OffsetY: 2},
		Align:  "center",
	}, ctx)
	prImg, prM := renderEl(&textrender.ElementStyle{
		Text: strings.Join(trimNonEmpty(cfg.Prices), "\n"), FontFile: "PlayfairDisplay-Bold.ttf", SizePct: 0.034 * scaleOr(cfg.DataScale),
		Color:  "#FFFFFF",
		Shadow: &textrender.ShadowStyle{Color: "#000000", Alpha: 0.6, Blur: 6, OffsetY: 2},
		Align:  "center", LineSpacing: 1.55, MaxWidthPct: 0.9,
	}, ctx)

	ah, th, tagh := contentH(addrImg, aM), contentH(titleImg, tM), contentH(tagImg, tagM)
	gap := int(0.013 * H)
	// khối chữ bắt đầu cao (~28.5%H) để tiêu đề vắt qua mép nối dải 1–2 như gốc
	cy := int(0.285 * H)
	drawCX(dc, addrImg, cfg.Width, cy, aM)
	cy += ah + gap
	drawCX(dc, titleImg, cfg.Width, cy, tM)
	cy += th + gap
	drawCX(dc, tagImg, cfg.Width, cy, tagM)
	cy += tagh + int(0.04*H)
	drawCX(dc, prImg, cfg.Width, cy, prM)
}

// ─────────────────────────── small helpers ───────────────────────────────────

func contentW(img image.Image, m int) int {
	if img == nil {
		return 0
	}
	return img.Bounds().Dx() - 2*m
}

// renderElFitWidth renders st on a single line, shrinking SizePct so the
// content fits within maxFrac of the canvas width. textrender only wraps —
// it never scales — so titles that must stay on one line (như bản gốc) need
// this two-pass fit. st.MaxWidthPct is cleared to suppress wrapping.
func renderElFitWidth(st *textrender.ElementStyle, ctx *textrender.RenderContext, maxFrac float64) (image.Image, int) {
	st.MaxWidthPct = 0
	st.NoWrap = true // fit-to-width = một dòng; tắt auto-wrap mặc định của renderer
	img, m := renderEl(st, ctx)
	if img == nil {
		return img, m
	}
	target := maxFrac * float64(ctx.VideoWidth)
	if cw := float64(contentW(img, m)); cw > target {
		st.SizePct *= target / cw
		img, m = renderEl(st, ctx)
	}
	return img, m
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
