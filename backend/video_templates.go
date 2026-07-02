package main

// Video template v2 — vẽ overlay bằng Go-code (gg + textrender) thay vì JSON
// element rời, để bản render thật có đủ icon vector, hiệu ứng extrude 3D,
// headline ghép nhiều phần và bảng giá nhóm — đúng như mockup đã duyệt trong
// template-mockups/. Mockup test (preview2_test.go) gọi CHÍNH các hàm vẽ này
// nên mockup luôn = output production.
//
// Mỗi template vẽ lên một canvas trong suốt cỡ video rồi trả về 1 OverlayPlan
// full-frame; ffmpeg composite như mọi overlay khác.

import (
	"fmt"
	"image"
	"image/color"
	"path/filepath"
	"strings"

	"github.com/fogleman/gg"

	"img2video/textrender"
)

// videoTemplateNames — các template render bằng Go-code.
var videoTemplateNames = map[string]bool{
	"staycation": true, "creampill": true, "goldserif": true,
	"editorial": true, "marquee": true, "chillgreen": true,
}

// renderVideoTemplateComposite vẽ overlay cho template Go-code. ok=false khi
// template không thuộc nhóm này (caller rơi về đường JSON cũ).
func renderVideoTemplateComposite(cfg Config, tmpDir string) (plans []OverlayPlan, ok bool, err error) {
	name := strings.TrimSpace(strings.ToLower(cfg.Template))
	if !videoTemplateNames[name] {
		return nil, false, nil
	}
	W, H := cfg.Width, cfg.Height
	if W <= 0 || H <= 0 {
		return nil, true, fmt.Errorf("video template %s: thiếu kích thước video", name)
	}
	ctx := &textrender.RenderContext{VideoWidth: W, VideoHeight: H, AssetsDir: assetsDir(),
		// Vùng an toàn TikTok: chừa cột nút phải 12%, caption đáy 18%, status đỉnh 9%.
		InsetLeftFrac: 0.05, InsetRightFrac: 0.12, InsetTopFrac: 0.09, InsetBottomFrac: 0.18}

	dc := gg.NewContext(W, H)
	dc.SetRGBA(0, 0, 0, 0)
	dc.Clear()

	switch name {
	case "staycation":
		drawStaycationVideo(dc, ctx, cfg)
	case "creampill":
		drawCreampillVideo(dc, ctx, cfg)
	case "goldserif":
		drawGoldSerifVideo(dc, ctx, cfg)
	case "editorial":
		drawEditorialVideo(dc, ctx, cfg)
	case "marquee":
		drawMarqueeVideo(dc, ctx, cfg)
	case "chillgreen":
		drawChillVideo(dc, ctx, cfg)
	}

	// Watermark + listing_id giữ trong VÙNG AN TOÀN TikTok (canh giữa, ~0.74/0.70H).
	// Vị trí cũ (góc dưới-phải 0.86H / đáy 0.93H) bị caption · nav · cột nút che.
	// Editorial vẫn dùng watermark làm brand trên đỉnh (bỏ qua ở đây).
	if wm := strings.TrimSpace(cfg.Watermark); wm != "" && name != "editorial" {
		img, m := renderEl(&textrender.ElementStyle{
			Text: wm, FontFile: "BeVietnamPro-Regular.ttf", SizePct: 0.02, Color: "#FFFFFF",
			Shadow: &textrender.ShadowStyle{Color: "#000000", Alpha: 0.6, Blur: 4, OffsetX: 1, OffsetY: 1},
			Align:  "center",
		}, ctx)
		drawCX(dc, img, W, int(0.74*float64(H)), m)
	}
	if id := strings.TrimSpace(cfg.ListingID); id != "" {
		img, m := renderEl(&textrender.ElementStyle{
			Text: id, FontFile: "BeVietnamPro-Bold.ttf", SizePct: 0.022, Color: "#FFFFFF",
			Shadow: &textrender.ShadowStyle{Color: "#000000", Alpha: 0.6, Blur: 4, OffsetX: 0.7, OffsetY: 1.4},
			Align:  "center",
		}, ctx)
		drawCX(dc, img, W, int(0.70*float64(H)), m)
	}

	path := filepath.Join(tmpDir, "composite.png")
	if err := gg.SavePNG(path, dc.Image()); err != nil {
		return nil, true, err
	}
	plans = append(plans, OverlayPlan{PNGPath: path, X: 0, Y: 0})

	// Tiêu đề intro (hiện 0..TitleDuration giây) — render rời để giữ hành vi cũ.
	if t := strings.TrimSpace(cfg.Title); t != "" {
		td := cfg.TitleDuration
		if td <= 0 {
			td = 3
		}
		st := &textrender.ElementStyle{
			Text: t, FontFile: "BeVietnamPro-Bold.ttf", SizePct: 0.05, Color: "#FFFFFF",
			Shadow:   &textrender.ShadowStyle{Color: "#000000", Alpha: 0.6, Blur: 8, OffsetY: 3},
			Align:    "center", MaxWidthPct: 0.9,
			Position: textrender.Position{X: "center", Y: "0.45"},
		}
		if cfg.TitleFontFile != "" {
			st.FontFile = cfg.TitleFontFile
		}
		adaptFont(st)
		out, rerr := textrender.Render(st, ctx)
		if rerr == nil && out != nil {
			tp := filepath.Join(tmpDir, "intro-title.png")
			if out.Save(tp) == nil {
				plans = append(plans, OverlayPlan{PNGPath: tp, X: out.X, Y: out.Y,
					EnableExpr: fmt.Sprintf("between(t,0,%.3f)", td)})
			}
		}
	}
	return plans, true, nil
}

// ─── helpers dữ liệu ──────────────────────────────────────────────────────────

func videoNickname(cfg Config) string {
	return strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(cfg.Nickname), "Homestay "))
}

// districtFromAddress lấy phần sau dấu phẩy cuối ("Đường X, Cầu Giấy" → "Cầu Giấy").
func districtFromAddress(addr string) string {
	parts := strings.Split(addr, ",")
	for i := len(parts) - 1; i >= 0; i-- {
		if p := strings.TrimSpace(parts[i]); p != "" {
			return p
		}
	}
	return ""
}

// priceGroups tách cfg.Prices thành các nhóm dòng: mỗi phần tử mảng là một
// nhóm, dòng trong nhóm tách bằng "\n". Dùng cho bảng giá có khoảng thở giữa
// nhóm (marquee/chillgreen) — drawPriceDoors nhận "" làm dòng giãn cách.
func priceGroups(prices []string) []string {
	var out []string
	for _, p := range prices {
		var lines []string
		for _, ln := range strings.Split(p, "\n") {
			if ln = strings.TrimSpace(ln); ln != "" {
				lines = append(lines, ln)
			}
		}
		if len(lines) == 0 {
			continue
		}
		if len(out) > 0 {
			out = append(out, "")
		}
		out = append(out, lines...)
	}
	return out
}

func videoBodyScale(cfg Config) float64 { return scaleOr(cfg.OverlayScale) }

func softVShadow() *textrender.ShadowStyle {
	return &textrender.ShadowStyle{Color: "#000000", Alpha: 0.5, Blur: 9, OffsetY: 3}
}
func strongVShadow() *textrender.ShadowStyle {
	return &textrender.ShadowStyle{Color: "#000000", Alpha: 0.62, Blur: 6, OffsetY: 2}
}

// drawRight vẽ img sao cho mép PHẢI nội dung tại xRight, mép trên tại yTop.
func drawRight(dc *gg.Context, img image.Image, xRight, yTop, m int) {
	if img == nil {
		return
	}
	cw := img.Bounds().Dx() - 2*m
	dc.DrawImage(img, xRight-cw-m, yTop-m)
}

// drawExtrude vẽ text với khối 3D đỏ phía sau (các bản sao lệch dần) dưới lớp
// vàng — hiệu ứng marquee. Trả về y đáy nội dung.
func drawExtrude(dc *gg.Context, ctx *textrender.RenderContext, text, font string, size float64, gold, red string, cx float64, yTop int) int {
	redImg, mR := renderEl(&textrender.ElementStyle{Text: text, FontFile: font, SizePct: size, Color: red, Align: "center"}, ctx)
	goldImg, mG := renderEl(&textrender.ElementStyle{Text: text, FontFile: font, SizePct: size, Color: gold, Align: "center"}, ctx)
	if goldImg == nil {
		return yTop
	}
	cw := contentW(goldImg, mG)
	ch := contentH(goldImg, mG)
	left := int(cx) - cw/2
	for _, off := range []int{9, 7, 5, 3, 1} {
		drawAt(dc, redImg, left+off, yTop+off, mR)
	}
	drawAt(dc, goldImg, left, yTop, mG)
	return yTop + ch
}

// drawPriceDoors vẽ các dòng giá, mỗi dòng kèm icon cá cam phía sau.
// Dòng "" = khoảng giãn cách nhóm. leftAlign=false → căn giữa quanh anchorX.
func drawPriceDoors(dc *gg.Context, ctx *textrender.RenderContext, lines []string, font string, size float64, col string, leftAlign bool, anchorX, yTop, gap int, maxWidthPct float64) int {
	fishBody := color.NRGBA{0xF2, 0x8C, 0x3B, 255}
	safeBottom := int(0.82 * float64(ctx.VideoHeight)) // không cho bảng giá lấn vùng caption
	cy := yTop
	for _, ln := range lines {
		if ln == "" {
			cy += gap
			continue
		}
		if cy > safeBottom { // hết chỗ an toàn → dừng, không vẽ tràn xuống đáy
			break
		}
		img, m := renderEl(&textrender.ElementStyle{Text: ln, FontFile: font, SizePct: size, Color: col, Shadow: strongVShadow(), Align: "left", MaxWidthPct: maxWidthPct}, ctx)
		if img == nil {
			continue
		}
		cw := contentW(img, m)
		ch := contentH(img, m)
		fishH := float64(ch) * 0.95
		fishW := fishH * 1.55
		var x int
		if leftAlign {
			x = anchorX
		} else {
			x = anchorX - (cw+16+int(fishW))/2
		}
		drawAt(dc, img, x, cy, m)
		drawFishIcon(dc, float64(x+cw+16), float64(cy)+(float64(ch)-fishH)/2, fishW, fishH, fishBody)
		cy += ch + gap
	}
	return cy
}

// ─── 1. staycation ────────────────────────────────────────────────────────────

func drawStaycationVideo(dc *gg.Context, ctx *textrender.RenderContext, cfg Config) {
	W, H := float64(cfg.Width), float64(cfg.Height)
	bs := videoBodyScale(cfg)
	xL := int(0.07 * W)

	eyL, mL := renderEl(&textrender.ElementStyle{Text: "Staycation", FontFile: "DancingScript-Variable.ttf",
		SizePct: 0.052, Color: "#FFFFFF", Shadow: softVShadow(), Align: "left"}, ctx)
	drawAt(dc, eyL, xL+int(0.012*W), int(0.265*H), mL)
	if d := districtFromAddress(cfg.Address); d != "" {
		eyR, mR := renderEl(&textrender.ElementStyle{Text: d, FontFile: "DancingScript-Variable.ttf",
			SizePct: 0.052, Color: "#FFFFFF", Shadow: softVShadow(), Align: "left"}, ctx)
		drawRight(dc, eyR, int(0.88*W), int(0.27*H), mR) // mép phải ≤ 0.88W, tránh cột nút TikTok
	}

	titleFont := "PlayfairDisplay-Bold.ttf" // serif đậm, số "4" kín (đồng bộ thumbnail)
	if cfg.TitleFontFile != "" {
		titleFont = cfg.TitleFontFile
	}
	tSt := &textrender.ElementStyle{Text: videoNickname(cfg), FontFile: titleFont,
		SizePct: 0.082, Color: "#FFFFFF", Shadow: softVShadow(), Align: "left", MaxWidthPct: 0.80}
	adaptFont(tSt)
	title, mT := renderEl(tSt, ctx)
	drawAt(dc, title, xL, int(0.315*H), mT)

	sub, mS := renderEl(&textrender.ElementStyle{Text: strings.Join(trimNonEmpty(cfg.Amenities), ", "),
		FontFile: "BeVietnamPro-Bold.ttf", SizePct: 0.0265 * bs, Color: "#FFFFFF", Shadow: strongVShadow(),
		Align: "left", MaxWidthPct: 0.80}, ctx)
	drawAt(dc, sub, xL+int(0.012*W), int(0.405*H), mS)

	pr, mP := renderEl(&textrender.ElementStyle{
		Text:     strings.Join(priceGroups(cfg.Prices), "\n"),
		FontFile: "BeVietnamPro-Bold.ttf", SizePct: 0.030 * bs, Color: "#FFFFFF", Shadow: strongVShadow(),
		Align: "left", LineSpacing: 1.45, MaxWidthPct: 0.80}, ctx)
	drawAt(dc, pr, xL+int(0.012*W), int(0.46*H), mP)
}

// ─── 2. creampill ─────────────────────────────────────────────────────────────

func drawCreampillVideo(dc *gg.Context, ctx *textrender.RenderContext, cfg Config) {
	W, H := cfg.Width, float64(cfg.Height)
	bs := videoBodyScale(cfg)

	if lines := priceGroups(cfg.Prices); len(lines) > 0 {
		var flat []string
		for _, ln := range lines {
			if ln != "" {
				flat = append(flat, ln)
			}
		}
		top, mt := renderEl(&textrender.ElementStyle{Text: strings.Join(flat, "   -   "),
			FontFile: "BeVietnamPro-Bold.ttf", SizePct: 0.024 * bs, Color: "#FFFFFF",
			Shadow: &textrender.ShadowStyle{Color: "#000000", Alpha: 0.75, Blur: 7, OffsetY: 2},
			Align:  "center", MaxWidthPct: 0.88}, ctx)
		drawCX(dc, top, W, int(0.165*H), mt)
	}

	pillFont := "PlayfairDisplay-Bold.ttf" // serif đậm, số "4" kín (thay Inter "4" hở đỉnh)
	if cfg.TitleFontFile != "" {
		pillFont = cfg.TitleFontFile
	}
	pill, mp := renderEl(&textrender.ElementStyle{Text: videoNickname(cfg), FontFile: pillFont,
		SizePct: 0.064, Color: "#D33B2C",
		Bg:     &textrender.BgStyle{Color: "#FBE6C2", Alpha: 0.80, Radius: 22, Padding: [2]float64{16, 40}},
		Shadow: &textrender.ShadowStyle{Color: "#000000", Alpha: 0.35, Blur: 12, OffsetY: 5},
		Align:  "center", MaxWidthPct: 0.92}, ctx)
	drawCX(dc, pill, W, int(0.205*H), mp)

	addr, ma := renderEl(&textrender.ElementStyle{Text: strings.TrimSpace(cfg.Address),
		FontFile: "BeVietnamPro-Bold.ttf", SizePct: 0.026 * bs, Color: "#FFFFFF",
		Shadow: &textrender.ShadowStyle{Color: "#000000", Alpha: 0.75, Blur: 7, OffsetY: 2},
		Align:  "center", MaxWidthPct: 0.9}, ctx)
	drawCX(dc, addr, W, int(0.285*H), ma)
}

// ─── 3. goldserif ─────────────────────────────────────────────────────────────

func drawGoldSerifVideo(dc *gg.Context, ctx *textrender.RenderContext, cfg Config) {
	W, H := cfg.Width, float64(cfg.Height)
	bs := videoBodyScale(cfg)
	gold := "#FFC107"

	addr, ma := renderEl(&textrender.ElementStyle{Text: strings.TrimSpace(cfg.Address),
		FontFile: "BeVietnamPro-Bold.ttf", SizePct: 0.024 * bs, Color: "#FFFFFF", Shadow: strongVShadow(),
		Align: "center", MaxWidthPct: 0.9}, ctx)
	drawCX(dc, addr, W, int(0.155*H), ma)

	titleFont := "PlayfairDisplay-Bold.ttf" // serif đậm, số "4" kín (thay YesevaOne)
	if cfg.TitleFontFile != "" {
		titleFont = cfg.TitleFontFile
	}
	tSt := &textrender.ElementStyle{Text: videoNickname(cfg), FontFile: titleFont,
		SizePct: 0.078, Color: gold, Stroke: &textrender.StrokeStyle{Color: "#FFFFFF", Width: 4.5, Alpha: 1},
		Shadow: &textrender.ShadowStyle{Color: "#000000", Alpha: 0.5, Blur: 8, OffsetY: 3}, Align: "center", MaxWidthPct: 0.88}
	adaptFont(tSt)
	title, mt := renderEl(tSt, ctx)
	// Title chảy NGAY DƯỚI address: address dài (wrap 2 dòng) không còn đè title.
	titleTop := 0.175 * H
	if b := 0.155*H + float64(contentH(addr, ma)) + 0.006*H; b > titleTop {
		titleTop = b
	}
	drawCX(dc, title, W, int(titleTop), mt)

	// Dòng không chứa chữ số = header mục (vàng, to hơn); còn lại trắng.
	// Bảng giá chảy dưới title (đẩy xuống nếu title bị address đẩy xuống).
	cy := int(0.265 * H)
	if b := int(titleTop + float64(contentH(title, mt)) + 0.03*H); b > cy {
		cy = b
	}
	first := true
	for _, ln := range priceGroups(cfg.Prices) {
		if ln == "" {
			continue
		}
		if cy > int(0.82*H) { // dừng trước vùng caption đáy
			break
		}
		isHeader := !strings.ContainsAny(ln, "0123456789")
		col, size := "#FFFFFF", 0.024*bs
		if isHeader {
			col, size = gold, 0.027*bs
			if !first {
				cy += int(0.02 * H)
			}
		}
		img, m := renderEl(&textrender.ElementStyle{Text: ln, FontFile: "BeVietnamPro-Bold.ttf",
			SizePct: size, Color: col, Shadow: strongVShadow(), Align: "center", MaxWidthPct: 0.92}, ctx)
		drawCX(dc, img, W, cy, m)
		cy += contentH(img, m) + int(0.013*H)
		first = false
	}
}

// ─── 4. editorial ─────────────────────────────────────────────────────────────

func drawEditorialVideo(dc *gg.Context, ctx *textrender.RenderContext, cfg Config) {
	W, H := float64(cfg.Width), float64(cfg.Height)
	bs := videoBodyScale(cfg)
	xL := int(0.05 * W)

	if wm := strings.TrimSpace(cfg.Watermark); wm != "" {
		brand, mBr := renderEl(&textrender.ElementStyle{Text: wm, FontFile: "Prata-Regular.ttf",
			SizePct: 0.026, Color: "#FFFFFF", Shadow: softVShadow(), Align: "center"}, ctx)
		drawCX(dc, brand, cfg.Width, int(0.035*H), mBr)
	}

	titleFont := "PlayfairDisplay-Bold.ttf"
	if cfg.TitleFontFile != "" {
		titleFont = cfg.TitleFontFile
	}
	big, mb := renderElFitWidth(&textrender.ElementStyle{Text: videoNickname(cfg), FontFile: titleFont,
		SizePct: 0.135, Color: "#FFFFFF", Shadow: softVShadow(), Align: "left", Curve: 22}, ctx, 0.82)
	drawAt(dc, big, xL, int(0.115*H), mb)
	bigBottom := int(0.115*H) + contentH(big, mb)
	bigRight := xL + contentW(big, mb)

	// chữ script "room" ép dưới-phải, ngay dưới baseline tiêu đề (kẹp mép phải ≤ 0.86W)
	script, ms := renderEl(&textrender.ElementStyle{Text: "room", FontFile: "DancingScript-Variable.ttf",
		SizePct: 0.078, Color: "#FFFFFF", Shadow: softVShadow(), Align: "left"}, ctx)
	drawAtClamped(dc, script, bigRight-contentW(script, ms)+int(0.04*W), bigBottom-int(0.006*H), ms,
		int(W), int(H), int(0.14*W), int(0.18*H))

	addrY := bigBottom + int(0.055*H)
	addr, ma := renderEl(&textrender.ElementStyle{Text: strings.TrimSpace(cfg.Address),
		FontFile: "BeVietnamPro-Bold.ttf", SizePct: 0.022 * bs, Color: "#FFFFFF", Shadow: strongVShadow(),
		Align: "left", MaxWidthPct: 0.6}, ctx)
	if addr != nil {
		drawPinIcon(dc, float64(xL)+0.012*W, float64(addrY), float64(contentH(addr, ma)), color.NRGBA{255, 255, 255, 255})
		drawAt(dc, addr, xL+int(0.04*W), addrY, ma)
	}

	panel, mp := renderEl(&textrender.ElementStyle{
		Text:     strings.Join(priceGroups(cfg.Prices), "\n"),
		FontFile: "BeVietnamPro-Bold.ttf", SizePct: 0.023 * bs, Color: "#3A352F",
		Bg:     &textrender.BgStyle{Color: "#F7F1E6", Alpha: 0.72, Radius: 24, Padding: [2]float64{18, 26}},
		Shadow: &textrender.ShadowStyle{Color: "#000000", Alpha: 0.28, Blur: 12, OffsetY: 5}, Align: "left", LineSpacing: 1.4,
		MaxWidthPct: 0.46}, ctx)
	// Mép phải panel kéo về 0.77W (trong vùng an toàn) thay vì 0.96W — tránh cột
	// nút like/comment/share của TikTok che mất bảng giá.
	drawRight(dc, panel, int(0.77*W), int(0.40*H), mp)
}

// ─── 5. marquee ───────────────────────────────────────────────────────────────

func drawMarqueeVideo(dc *gg.Context, ctx *textrender.RenderContext, cfg Config) {
	W, H := cfg.Width, float64(cfg.Height)
	bs := videoBodyScale(cfg)
	gold, red := "#FFC107", "#B0161A"

	// headline chiến dịch: Summer [Date] Hà Nội / Homestay (chất template,
	// không phụ thuộc dữ liệu listing)
	dateImg, mD := renderEl(&textrender.ElementStyle{Text: "Date", FontFile: "YesevaOne-Regular.ttf",
		SizePct: 0.10, Color: gold, Align: "center"}, ctx)
	dW := contentW(dateImg, mD)
	dH := contentH(dateImg, mD)
	dTop := int(0.225 * H)
	drawExtrude(dc, ctx, "Date", "YesevaOne-Regular.ttf", 0.10, gold, red, float64(W)/2, dTop)
	dLeft := (W - dW) / 2
	vCenter := dTop + dH/2
	gap := int(0.022 * float64(W))
	summer, mSu := renderEl(&textrender.ElementStyle{Text: "Summer", FontFile: "Prata-Regular.ttf",
		SizePct: 0.044, Color: "#F2E2B6", Shadow: softVShadow(), Align: "center"}, ctx)
	hanoi, mHa := renderEl(&textrender.ElementStyle{Text: "Hà Nội", FontFile: "Prata-Regular.ttf",
		SizePct: 0.044, Color: "#F2E2B6", Shadow: softVShadow(), Align: "center"}, ctx)
	drawRight(dc, summer, dLeft-gap, vCenter-contentH(summer, mSu)/2, mSu)
	drawAt(dc, hanoi, dLeft+dW+gap, vCenter-contentH(hanoi, mHa)/2, mHa)

	homeBottom := drawExtrude(dc, ctx, "Homestay", "YesevaOne-Regular.ttf", 0.115, gold, red, float64(W)/2, dTop+int(0.072*H))

	if am := strings.Join(trimNonEmpty(cfg.Amenities), " – "); am != "" {
		sub, mS := renderEl(&textrender.ElementStyle{Text: am,
			FontFile: "BeVietnamPro-Italic.ttf", SizePct: 0.025 * bs, Color: "#F2E2B6", Shadow: strongVShadow(),
			Align: "center", MaxWidthPct: 0.9}, ctx)
		drawCX(dc, sub, W, homeBottom+int(0.02*H), mS)
	}

	yTop := homeBottom + int(0.075*H)
	drawPriceDoors(dc, ctx, priceGroups(cfg.Prices), "BeVietnamPro-Bold.ttf", 0.024*bs, "#FFFFFF", false, W/2, yTop, int(0.012*H), 0.76)
}

// ─── 6. chillgreen ────────────────────────────────────────────────────────────

// splitHeadline tách nickname thành 2 dòng cân chữ (ưu tiên "\n" sẵn có).
func splitHeadline(s string) (string, string) {
	if i := strings.IndexAny(s, "\n"); i >= 0 {
		return strings.TrimSpace(s[:i]), strings.TrimSpace(s[i+1:])
	}
	words := strings.Fields(s)
	if len(words) < 2 {
		return s, ""
	}
	total := len(s)
	acc := 0
	for i, w := range words {
		acc += len(w) + 1
		if acc >= total/2 {
			return strings.Join(words[:i+1], " "), strings.Join(words[i+1:], " ")
		}
	}
	return s, ""
}

func drawChillVideo(dc *gg.Context, ctx *textrender.RenderContext, cfg Config) {
	W, H := cfg.Width, float64(cfg.Height)
	bs := videoBodyScale(cfg)
	green := "#A4C639"
	leaf := color.NRGBA{0x8F, 0xB8, 0x3A, 255}

	addr, ma := renderEl(&textrender.ElementStyle{Text: strings.TrimSpace(cfg.Address),
		FontFile: "BeVietnamPro-Bold.ttf", SizePct: 0.024 * bs, Color: "#FFFFFF", Shadow: strongVShadow(),
		Align: "center", MaxWidthPct: 0.9}, ctx)
	drawCX(dc, addr, W, int(0.20*H), ma)

	// Headline là câu marketing trọn vẹn — KHÔNG cắt tiền tố "Homestay " như
	// nickname thường (chữ Homestay thuộc câu, không phải brand prefix).
	line1, line2 := splitHeadline(strings.TrimSpace(cfg.Nickname))
	hStyle := func(text string) *textrender.ElementStyle {
		return &textrender.ElementStyle{Text: text,
			FontFile: "BeVietnamPro-Italic.ttf", SizePct: 0.038, Color: green,
			Stroke: &textrender.StrokeStyle{Color: "#2B4A12", Width: 2.4, Alpha: 1},
			Shadow: &textrender.ShadowStyle{Color: "#0c2a06", Alpha: 0.7, Blur: 6, OffsetY: 2},
			Align:  "center", MaxWidthPct: 0.84}
	}
	h1, m1 := renderEl(hStyle(line1), ctx)
	y1 := int(0.255 * H)
	drawCX(dc, h1, W, y1, m1)
	c1w := contentW(h1, m1)
	c1h := contentH(h1, m1)
	drawLeafIcon(dc, float64((W-c1w)/2)-float64(c1h)*0.55, float64(y1)+float64(c1h)*0.5, float64(c1h)*0.95, -25, leaf)

	y2 := y1 + c1h
	if line2 != "" {
		h2, m2 := renderEl(hStyle(line2), ctx)
		y2 = y1 + c1h + int(0.012*H)
		drawCX(dc, h2, W, y2, m2)
		c2w := contentW(h2, m2)
		c2h := contentH(h2, m2)
		drawLeafIcon(dc, float64((W+c2w)/2)+float64(c2h)*0.55, float64(y2)+float64(c2h)*0.5, float64(c2h)*0.95, 25, leaf)
		y2 += c2h
	} else {
		drawLeafIcon(dc, float64((W+c1w)/2)+float64(c1h)*0.55, float64(y1)+float64(c1h)*0.5, float64(c1h)*0.95, 25, leaf)
	}

	yTop := y2 + int(0.04*H)
	leftX := int(0.28 * float64(W))
	// mw 0.55: leftX(0.28W)+0.55W = 0.83W ≤ 0.88W rail → dòng combo dài wrap, không tràn phải
	drawPriceDoors(dc, ctx, priceGroups(cfg.Prices), "BeVietnamPro-Bold.ttf", 0.023*bs, "#FFFFFF", true, leftX, yTop, int(0.011*H), 0.55)
}
