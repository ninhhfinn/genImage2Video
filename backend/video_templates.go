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
	"amorex": true, "ntgroom": true,
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
	// Chuẩn hoá data THẬT từ API về dạng video-friendly (nén giá dài/combo, dọn địa
	// chỉ). Data mockup đã ngắn → giữ nguyên (xem video_data.go).
	cfg.Prices = compactVideoPrices(cfg.Prices)
	cfg.Address = cleanVideoAddress(cfg.Address)
	ctx := &textrender.RenderContext{VideoWidth: W, VideoHeight: H, AssetsDir: assetsDir(),
		// Vùng an toàn TikTok: chừa cột nút phải 12%, caption đáy 18%, status đỉnh 9%.
		InsetLeftFrac: 0.05, InsetRightFrac: 0.12, InsetTopFrac: 0.09, InsetBottomFrac: 0.18}

	// ── Đường CHROME (mặc định): dựng overlay từ HTML/CSS mockup đã chốt, render
	// headless Chrome → khớp Canva sát hơn vẽ tay gg (viền -webkit-text-stroke,
	// pill border, emoji màu, SVG cong). Lỗi (thiếu chrome…) → rơi về đường gg cũ.
	path := filepath.Join(tmpDir, "composite.png")
	if cpath, cerr := renderChromeOverlay(cfg, tmpDir); cerr == nil {
		path = cpath
	} else {
		fmt.Printf("[video-template] chrome overlay lỗi, fallback gg: %v\n", cerr)

		dc := gg.NewContext(W, H)
		dc.SetRGBA(0, 0, 0, 0)
		dc.Clear()
		drawVideoVeil(dc, W, H)

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
		case "amorex":
			drawAmorexVideo(dc, ctx, cfg)
		case "ntgroom":
			drawNtgRoomVideo(dc, ctx, cfg)
		}

		// Watermark + listing_id neo ở ĐÁY vùng an toàn TikTok (~0.795/0.75H) để đọc
		// như footer, không lơ lửng giữa khung. Đáy an toàn = 0.82H (caption 18%);
		// vị trí cũ 0.74/0.70H quá cao nên chừa 1/4 khung dưới trống.
		if wm := strings.TrimSpace(cfg.Watermark); wm != "" && name != "editorial" {
			img, m := renderEl(&textrender.ElementStyle{
				Text: wm, FontFile: "BeVietnamPro-Regular.ttf", SizePct: 0.02, Color: "#FFFFFF",
				Shadow: &textrender.ShadowStyle{Color: "#000000", Alpha: 0.6, Blur: 4, OffsetX: 1, OffsetY: 1},
				Align:  "center",
			}, ctx)
			drawCX(dc, img, W, int(0.795*float64(H)), m)
		}
		if id := strings.TrimSpace(cfg.ListingID); id != "" {
			img, m := renderEl(&textrender.ElementStyle{
				Text: id, FontFile: "BeVietnamPro-Bold.ttf", SizePct: 0.022, Color: "#FFFFFF",
				Shadow: &textrender.ShadowStyle{Color: "#000000", Alpha: 0.6, Blur: 4, OffsetX: 0.7, OffsetY: 1.4},
				Align:  "center",
			}, ctx)
			drawCX(dc, img, W, int(0.75*float64(H)), m)
		}

		if err := gg.SavePNG(path, dc.Image()); err != nil {
			return nil, true, err
		}
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

// drawVideoVeil phủ tối nền cho legibility — đồng bộ với cảm giác mockup nhưng
// nằm trong ĐƯỜNG PRODUCTION nên video thật cũng được. Gồm: darken đều toàn khung
// + gradient đậm dần ở ĐÁY (tiêu đề/giá nằm dưới) và nhẹ ở ĐỈNH (panel/address/brand).
func drawVideoVeil(dc *gg.Context, W, H int) {
	fw, fh := float64(W), float64(H)
	// 1. darken đều toàn khung (bù cho việc bg ảnh thật không bị làm tối ở main.go)
	//    — đậm hơn để tiệm cận nền tối mood của mockup, chữ trắng giữa khung nổi rõ.
	dc.SetColor(color.NRGBA{6, 4, 3, 88})
	dc.DrawRectangle(0, 0, fw, fh)
	dc.Fill()
	// 2. gradient đáy: 0 ở 0.38H → rất đậm ở đáy (khối tiêu đề + bảng giá vùng dưới)
	vGradient(dc, 0, fh*0.38, fw, fh*0.62, color.NRGBA{6, 4, 3, 0}, color.NRGBA{6, 4, 3, 140})
	// 3. gradient đỉnh: đậm ở đỉnh → 0 ở 0.26H (brand/address/panel trên)
	vGradient(dc, 0, 0, fw, fh*0.26, color.NRGBA{6, 4, 3, 96}, color.NRGBA{6, 4, 3, 0})
}

// ─── helpers dữ liệu ──────────────────────────────────────────────────────────

func videoNickname(cfg Config) string {
	return strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(cfg.Nickname), "Homestay "))
}

// districtFromAddress rút tên QUẬN từ địa chỉ: phần sau dấu phẩy cuối, nhưng
// nếu phần cuối là tỉnh/thành phố thì lùi một bậc — địa chỉ API đầy đủ luôn
// kết thúc ", Hà Nội" nên lấy phần cuối sẽ ra thành phố thay vì quận
// ("..., Nhật Tân, Tây Hồ, Hà Nội" → "Tây Hồ"; "Đường X, Cầu Giấy" → "Cầu Giấy").
func districtFromAddress(addr string) string {
	var parts []string
	for _, p := range strings.Split(cleanVideoAddress(addr), ",") {
		if p = strings.TrimSpace(p); p != "" {
			parts = append(parts, p)
		}
	}
	if len(parts) == 0 {
		return ""
	}
	if last := parts[len(parts)-1]; len(parts) >= 2 && isProvinceName(last) {
		return parts[len(parts)-2]
	}
	return parts[len(parts)-1]
}

// isProvinceName nhận diện tên tỉnh/thành phố hay gặp ở đuôi địa chỉ.
func isProvinceName(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "hà nội", "ha noi", "hanoi",
		"hồ chí minh", "tp hồ chí minh", "tp. hồ chí minh", "thành phố hồ chí minh", "hcm",
		"đà nẵng", "da nang", "hải phòng", "cần thơ", "việt nam", "vietnam":
		return true
	}
	return false
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

// heroShadow — bóng đậm + nhoè rộng cho TIÊU ĐỀ trắng cỡ lớn không viền
// (staycation/editorial serif). Đảm bảo nổi trên cả nền ảnh thật sáng (trần,
// tường trắng) chứ không chỉ nền tối của mockup.
func heroShadow() *textrender.ShadowStyle {
	return &textrender.ShadowStyle{Color: "#000000", Alpha: 0.6, Blur: 16, OffsetY: 3}
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
	fishBody := color.NRGBA{0x4A, 0x9F, 0xE0, 255} // cá xanh dương (đúng emoji 🐟 trong Canva T4/T5)
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
		SizePct: 0.056, Color: "#FFFFFF", Shadow: softVShadow(), Align: "left"}, ctx)
	drawAt(dc, eyL, xL+int(0.012*W), int(0.265*H), mL)
	if d := districtFromAddress(cfg.Address); d != "" {
		eyR, mR := renderEl(&textrender.ElementStyle{Text: d, FontFile: "DancingScript-Variable.ttf",
			SizePct: 0.056, Color: "#FFFFFF", Shadow: softVShadow(), Align: "left"}, ctx)
		drawRight(dc, eyR, int(0.88*W), int(0.27*H), mR) // mép phải ≤ 0.88W, tránh cột nút TikTok
	}

	titleFont := "YesevaOne-Regular.ttf" // Canva Trang 3: Yeseva One (serif display)
	if cfg.TitleFontFile != "" {
		titleFont = cfg.TitleFontFile
	}
	tSt := &textrender.ElementStyle{Text: videoNickname(cfg), FontFile: titleFont,
		SizePct: 0.062, Color: "#FFFFFF", Shadow: heroShadow(), Align: "left", MaxWidthPct: 0.80}
	adaptFont(tSt)
	title, mT := renderEl(tSt, ctx)
	drawAt(dc, title, xL, int(0.315*H), mT)

	sub, mS := renderEl(&textrender.ElementStyle{Text: strings.Join(trimNonEmpty(cfg.Amenities), ", "),
		FontFile: "YesevaOne-Regular.ttf", SizePct: 0.030 * bs, Color: "#FFFFFF", Shadow: strongVShadow(),
		Align: "left", MaxWidthPct: 0.80}, ctx)
	drawAt(dc, sub, xL+int(0.012*W), int(0.405*H), mS)

	pr, mP := renderEl(&textrender.ElementStyle{
		Text:     strings.Join(priceGroups(cfg.Prices), "\n"),
		FontFile: "YesevaOne-Regular.ttf", SizePct: 0.030 * bs, Color: "#FFFFFF", Shadow: strongVShadow(),
		Align: "left", LineSpacing: 1.45, MaxWidthPct: 0.80}, ctx)
	drawAt(dc, pr, xL+int(0.012*W), int(0.46*H), mP)
}

// ─── 2. creampill ─────────────────────────────────────────────────────────────

func drawCreampillVideo(dc *gg.Context, ctx *textrender.RenderContext, cfg Config) {
	W, H := cfg.Width, float64(cfg.Height)
	bs := videoBodyScale(cfg)
	cream := "#F9E9AF" // = màu chữ giá/địa chỉ, = nền pill (đúng Canva Trang 2)

	// Dòng giá trên cùng — chữ kem, VIỀN ĐEN dày (Canva Outline). "Qua đêm: … - Combo: …"
	// Mockup Trang 2 chỉ có 2 mục → với data thật (5 dòng) chọn 2 mục tiêu biểu
	// (ưu tiên Qua đêm + Combo) để không tràn ngang.
	if lines := priceGroups(cfg.Prices); len(lines) > 0 {
		var flat []string
		for _, ln := range lines {
			if ln != "" {
				flat = append(flat, ln)
			}
		}
		flat = pickTwoPriceLines(flat)
		top, mt := renderEl(&textrender.ElementStyle{Text: strings.Join(flat, " - "),
			FontFile: "Baloo2-Bold.ttf", SizePct: 0.052 * bs, Color: cream,
			Stroke: &textrender.StrokeStyle{Color: "#000000", Width: 6, Alpha: 1},
			Shadow: &textrender.ShadowStyle{Color: "#000000", Alpha: 0.4, Blur: 6, OffsetY: 2},
			Align:  "center", MaxWidthPct: 0.96}, ctx)
		drawCX(dc, top, W, int(0.205*H), mt)
	}

	// Tiêu đề đỏ trong pill kem bo tròn (Baloo2 ExtraBold, đậm tròn).
	pillFont := "Baloo2-ExtraBold.ttf"
	if cfg.TitleFontFile != "" {
		pillFont = cfg.TitleFontFile
	}
	pill, mp := renderEl(&textrender.ElementStyle{Text: videoNickname(cfg), FontFile: pillFont,
		SizePct: 0.098, Color: "#FF4004",
		Bg:     &textrender.BgStyle{Color: cream, Alpha: 0.95, Radius: 44, Padding: [2]float64{18, 40}},
		Shadow: &textrender.ShadowStyle{Color: "#000000", Alpha: 0.30, Blur: 12, OffsetY: 5},
		Align:  "center", MaxWidthPct: 0.95}, ctx)
	pillTop := int(0.262 * H)
	drawCX(dc, pill, W, pillTop, mp)
	pillBottom := pillTop + contentH(pill, mp)

	// Địa chỉ — chữ kem, VIỀN ĐEN (Canva Outline dày hơn dòng giá). Chảy DƯỚI pill:
	// tên dài (pill 2 dòng) không còn đè lên địa chỉ.
	addr, ma := renderEl(&textrender.ElementStyle{Text: strings.TrimSpace(cfg.Address),
		FontFile: "Baloo2-Bold.ttf", SizePct: 0.046 * bs, Color: cream,
		Stroke: &textrender.StrokeStyle{Color: "#000000", Width: 7, Alpha: 1},
		Shadow: &textrender.ShadowStyle{Color: "#000000", Alpha: 0.4, Blur: 6, OffsetY: 2},
		Align:  "center", MaxWidthPct: 0.92}, ctx)
	addrY := int(0.374 * H)
	if b := pillBottom + int(0.020*H); b > addrY {
		addrY = b
	}
	drawCX(dc, addr, W, addrY, ma)
}

// ─── 3. goldserif ─────────────────────────────────────────────────────────────

func drawGoldSerifVideo(dc *gg.Context, ctx *textrender.RenderContext, cfg Config) {
	W, H := cfg.Width, float64(cfg.Height)
	bs := videoBodyScale(cfg)
	gold := "#FFFC00" // vàng Canva Trang 1 (không phải amber #FFC107)
	blackOutline := func(w float64) *textrender.StrokeStyle {
		return &textrender.StrokeStyle{Color: "#000000", Width: w, Alpha: 1}
	}

	addr, ma := renderEl(&textrender.ElementStyle{Text: strings.TrimSpace(cfg.Address),
		FontFile: "Baloo2-Bold.ttf", SizePct: 0.034 * bs, Color: "#FFFFFF",
		Stroke: blackOutline(3),
		Shadow: &textrender.ShadowStyle{Color: "#000000", Alpha: 0.4, Blur: 6, OffsetY: 2},
		Align:  "center", MaxWidthPct: 0.9}, ctx)
	drawCX(dc, addr, W, int(0.205*H), ma)

	// Tiêu đề vàng Baloo2 ExtraBold + VIỀN ĐEN dày (Canva Outline).
	titleFont := "Baloo2-ExtraBold.ttf"
	if cfg.TitleFontFile != "" {
		titleFont = cfg.TitleFontFile
	}
	tSt := &textrender.ElementStyle{Text: videoNickname(cfg), FontFile: titleFont,
		SizePct: 0.086, Color: gold, Stroke: blackOutline(5.5),
		Shadow: &textrender.ShadowStyle{Color: "#000000", Alpha: 0.45, Blur: 7, OffsetY: 3}, Align: "center", MaxWidthPct: 0.9}
	title, mt := renderEl(tSt, ctx)
	// Title chảy NGAY DƯỚI address: address dài (wrap 2 dòng) không còn đè title.
	titleTop := 0.245 * H
	if b := 0.205*H + float64(contentH(addr, ma)) + 0.01*H; b > titleTop {
		titleTop = b
	}
	drawCX(dc, title, W, int(titleTop), mt)

	// Dòng không chứa chữ số = header mục (vàng, to hơn, viền đậm); còn lại trắng.
	cy := int(0.36 * H)
	if b := int(titleTop + float64(contentH(title, mt)) + 0.03*H); b > cy {
		cy = b
	}
	first := true
	for _, ln := range goldSerifGroups(priceGroups(cfg.Prices)) {
		if ln == "" {
			continue
		}
		if cy > int(0.82*H) { // dừng trước vùng caption đáy
			break
		}
		isHeader := !strings.ContainsAny(ln, "0123456789")
		col, size, sw := "#FFFFFF", 0.038*bs, 4.0
		if isHeader {
			col, size, sw = gold, 0.044*bs, 4.5
			if !first {
				cy += int(0.02 * H)
			}
		}
		img, m := renderEl(&textrender.ElementStyle{Text: ln, FontFile: "Baloo2-Bold.ttf",
			SizePct: size, Color: col, Stroke: blackOutline(sw),
			Shadow: &textrender.ShadowStyle{Color: "#000000", Alpha: 0.4, Blur: 6, OffsetY: 2},
			Align:  "center", MaxWidthPct: 0.92}, ctx)
		drawCX(dc, img, W, cy, m)
		cy += contentH(img, m) + int(0.014*H)
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

	titleFont := "DejaVuSerif-Bold.ttf" // Canva Trang 6: DejaVu Serif ("Standard")
	if cfg.TitleFontFile != "" {
		titleFont = cfg.TitleFontFile
	}
	// Chữ cung lớn CỐ ĐỊNH "Standard" như mockup Canva Trang 6 (yêu cầu 2026-07-03)
	// — KHÔNG nhồi tên phòng (đồng bộ đường chrome_overlay.go).
	big, mb := renderElFitWidth(&textrender.ElementStyle{Text: "Standard", FontFile: titleFont,
		SizePct: 0.135, Color: "#FFFFFF", Shadow: heroShadow(), Align: "left", Curve: 22}, ctx, 0.82)
	drawAt(dc, big, xL, int(0.115*H), mb)
	bigBottom := int(0.115*H) + contentH(big, mb)
	bigRight := xL + contentW(big, mb)

	// chữ script "Room" ép dưới-phải, hạ xuống ngang hàng địa chỉ, có HÀO QUANG
	// TRẮNG nhẹ (Canva Trang 6: Room glow). Chữ hoa "Room", to hơn.
	// "Room" = Sacramento (script thanh mảnh) — Breathing là font Canva premium KHÔNG
	// tải tự do được; mockup_templates.html dùng Sacramento làm bản thay → theo mockup.
	script, ms := renderEl(&textrender.ElementStyle{Text: "Room", FontFile: "Sacramento-Regular.ttf",
		SizePct: 0.118, Color: "#FFFFFF",
		Shadow: &textrender.ShadowStyle{Color: "#FFFFFF", Alpha: 0.5, Blur: 5, OffsetX: 0, OffsetY: 0},
		Align:  "left"}, ctx)
	scriptY := bigBottom + int(0.012*H)
	drawAtClamped(dc, script, bigRight-contentW(script, ms)+int(0.02*W), scriptY, ms,
		int(W), int(H), int(0.12*W), int(0.18*H))
	scriptBottom := scriptY + contentH(script, ms)

	// Địa chỉ XUỐNG DƯỚI đáy chữ "Room" (Great Vibes swash dài) — địa chỉ dài (wrap 2
	// dòng) không còn bị "Room" đè như Canva Trang 6 (địa chỉ 1 dòng dưới chữ ký).
	addrY := bigBottom + int(0.055*H)
	if b := scriptBottom + int(0.012*H); b > addrY {
		addrY = b
	}
	addr, ma := renderEl(&textrender.ElementStyle{Text: strings.TrimSpace(cfg.Address),
		FontFile: "Montserrat-SemiBold.ttf", SizePct: 0.023 * bs, Color: "#FFFFFF", Shadow: strongVShadow(),
		Align: "left", MaxWidthPct: 0.6}, ctx)
	if addr != nil {
		drawPinIcon(dc, float64(xL)+0.012*W, float64(addrY), float64(contentH(addr, ma)), color.NRGBA{255, 255, 255, 255})
		drawAt(dc, addr, xL+int(0.04*W), addrY, ma)
	}

	// Bảng giá Canva Trang 6: nền TRONG SUỐT (chỉ hơi tối cho dễ đọc), chữ TRẮNG,
	// VIỀN TRẮNG mảnh bo tròn — không phải panel kem chữ đậm.
	panel, mp := renderEl(&textrender.ElementStyle{
		Text:     strings.Join(priceGroups(cfg.Prices), "\n"),
		FontFile: "Montserrat-SemiBold.ttf", SizePct: 0.028 * bs, Color: "#FFFFFF",
		Bg: &textrender.BgStyle{Color: "#000000", Alpha: 0.12, Radius: 30, Padding: [2]float64{20, 30},
			BorderColor: "#FFFFFF", BorderWidth: 4, BorderAlpha: 0.85},
		Shadow: &textrender.ShadowStyle{Color: "#000000", Alpha: 0.45, Blur: 7, OffsetY: 2}, Align: "left", LineSpacing: 1.45,
		MaxWidthPct: 0.5}, ctx)
	// Mép phải panel kéo về 0.80W (trong vùng an toàn) — tránh cột nút TikTok.
	drawRight(dc, panel, int(0.80*W), int(0.42*H), mp)
}

// ─── 5. marquee ───────────────────────────────────────────────────────────────

func drawMarqueeVideo(dc *gg.Context, ctx *textrender.RenderContext, cfg Config) {
	W, H := cfg.Width, float64(cfg.Height)
	bs := videoBodyScale(cfg)
	gold := "#FFC700"

	// headline chiến dịch: Summer [Date] Hà Nội / Homestay (chất template,
	// không phụ thuộc dữ liệu listing). Canva Trang 5: chữ VÀNG Yeseva + ĐỔ BÓNG
	// MAROON CỨNG (#812c2c, lệch xuống-phải ~10px) — KHÔNG phải khối đùn 3D đỏ.
	maroonDrop := func(text string, yTop int) int {
		img, m := renderEl(&textrender.ElementStyle{Text: text, FontFile: "YesevaOne-Regular.ttf",
			SizePct: 0.111, Color: gold,
			Shadow: &textrender.ShadowStyle{Color: "#812c2c", Alpha: 1, Blur: 1, OffsetX: 10, OffsetY: 11},
			Align:  "center"}, ctx)
		drawCX(dc, img, W, yTop, m)
		return yTop + contentH(img, m)
	}
	dateImg, mD := renderEl(&textrender.ElementStyle{Text: "Date", FontFile: "YesevaOne-Regular.ttf",
		SizePct: 0.111, Color: gold, Align: "center"}, ctx)
	dW := contentW(dateImg, mD)
	dH := contentH(dateImg, mD)
	dTop := int(0.225 * H)
	maroonDrop("Date", dTop)
	dLeft := (W - dW) / 2
	vCenter := dTop + dH/2
	gap := int(0.022 * float64(W))
	summer, mSu := renderEl(&textrender.ElementStyle{Text: "Summer", FontFile: "YesevaOne-Regular.ttf",
		SizePct: 0.037, Color: "#F2E2B6", Shadow: softVShadow(), Align: "center"}, ctx)
	hanoi, mHa := renderEl(&textrender.ElementStyle{Text: "Hà Nội", FontFile: "YesevaOne-Regular.ttf",
		SizePct: 0.037, Color: "#F2E2B6", Shadow: softVShadow(), Align: "center"}, ctx)
	drawRight(dc, summer, dLeft-gap, vCenter-contentH(summer, mSu)/2, mSu)
	drawAt(dc, hanoi, dLeft+dW+gap, vCenter-contentH(hanoi, mHa)/2, mHa)

	homeBottom := maroonDrop("Homestay", dTop+int(0.072*H))

	if am := strings.Join(trimNonEmpty(cfg.Amenities), " – "); am != "" {
		sub, mS := renderEl(&textrender.ElementStyle{Text: am,
			FontFile: "YesevaOne-Regular.ttf", SizePct: 0.030 * bs, Color: "#F2E2B6", Shadow: strongVShadow(),
			Align: "center", MaxWidthPct: 0.9}, ctx)
		drawCX(dc, sub, W, homeBottom+int(0.02*H), mS)
	}

	yTop := homeBottom + int(0.075*H)
	drawPriceDoors(dc, ctx, priceGroups(cfg.Prices), "YesevaOne-Regular.ttf", 0.028*bs, "#FFFFFF", false, W/2, yTop, int(0.012*H), 0.76)
}

// ─── 6. chillgreen ────────────────────────────────────────────────────────────

// splitHeadline tách nickname thành 2 dòng cân chữ (ưu tiên "\n" sẵn có).
func splitHeadline(s string) (string, string) {
	if i := strings.IndexAny(s, "\n"); i >= 0 {
		return strings.TrimSpace(s[:i]), strings.TrimSpace(s[i+1:])
	}
	// Nickname NGẮN (mã phòng "XD 609", "501 - STUDIO") KHÔNG tách 2 dòng — nếu
	// tách sẽ thành "XD"/"609" trông như slogan vỡ. Chỉ câu marketing DÀI mới tách.
	s = strings.TrimSpace(s)
	if len([]rune(s)) <= 16 {
		return s, ""
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
	leaf := color.NRGBA{0x8F, 0xB8, 0x3A, 255}

	addr, ma := renderEl(&textrender.ElementStyle{Text: strings.TrimSpace(cfg.Address),
		FontFile: "Montserrat-SemiBold.ttf", SizePct: 0.030 * bs, Color: "#FFFFFF", Shadow: strongVShadow(),
		Align: "center", MaxWidthPct: 0.9}, ctx)
	drawCX(dc, addr, W, int(0.20*H), ma)

	// Headline là câu marketing trọn vẹn — KHÔNG cắt tiền tố "Homestay " như
	// nickname thường (chữ Homestay thuộc câu, không phải brand prefix).
	// Canva Trang 4: chữ VÀNG nghiêng + ĐỔ BÓNG MAROON cứng (#812c2c), không viền.
	line1, line2 := splitHeadline(strings.TrimSpace(cfg.Nickname))
	hStyle := func(text string) *textrender.ElementStyle {
		return &textrender.ElementStyle{Text: text,
			FontFile: "BeVietnamPro-Italic.ttf", SizePct: 0.035, Color: "#FFC700",
			Shadow: &textrender.ShadowStyle{Color: "#812c2c", Alpha: 1, Blur: 1, OffsetX: 5, OffsetY: 6},
			Align:  "center", MaxWidthPct: 0.86}
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
	drawPriceDoors(dc, ctx, priceGroups(cfg.Prices), "Montserrat-SemiBold.ttf", 0.028*bs, "#FFFFFF", true, leftX, yTop, int(0.011*H), 0.55)
}

// drawPillAddress vẽ pill địa chỉ canh giữa + GHIM ĐỎ ở đầu chữ. Trả về đáy nội dung.
// borderColor="" → không viền.
func drawPillAddress(dc *gg.Context, ctx *textrender.RenderContext, addr, font string, size float64,
	textColor, pillColor string, pillAlpha, radius, padV, padH float64,
	borderColor string, borderW, borderAlpha float64, W, y int) int {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return y
	}
	img, m := renderEl(&textrender.ElementStyle{Text: "   " + addr, FontFile: font, SizePct: size,
		Color: textColor,
		Bg: &textrender.BgStyle{Color: pillColor, Alpha: pillAlpha, Radius: radius, Padding: [2]float64{padV, padH},
			BorderColor: borderColor, BorderWidth: borderW, BorderAlpha: borderAlpha},
		Shadow: &textrender.ShadowStyle{Color: "#000000", Alpha: 0.3, Blur: 8, OffsetY: 3},
		Align:  "center", MaxWidthPct: 0.92}, ctx)
	if img == nil {
		return y
	}
	drawCX(dc, img, W, y, m)
	ch := contentH(img, m)
	contentLeft := (W-img.Bounds().Dx())/2 + m + int(padH)
	pinH := float64(ch) * 0.66
	drawPinIcon(dc, float64(contentLeft)+pinH*0.34, float64(y)+(float64(ch)-pinH)/2, pinH, color.NRGBA{0xE0, 0x33, 0x2B, 255})
	return y + ch
}

// ─── 7. amorex (Canva Trang 7) ─────────────────────────────────────────────────
// Panel giá kem góc trên-trái, tiêu đề đậm trắng cỡ lớn, pill địa chỉ kem + ghim.
func drawAmorexVideo(dc *gg.Context, ctx *textrender.RenderContext, cfg Config) {
	W, H := cfg.Width, float64(cfg.Height)
	bs := videoBodyScale(cfg)
	cream := "#F6E9C9"

	if p := strings.Join(priceGroups(cfg.Prices), "\n"); strings.TrimSpace(p) != "" {
		panel, mp := renderEl(&textrender.ElementStyle{Text: p,
			FontFile: "Poppins-Light.ttf", SizePct: 0.027 * bs, Color: "#812C2C", // bảng giá light (đồng bộ đường Chrome)
			Bg:     &textrender.BgStyle{Color: cream, Alpha: 0.92, Radius: 22, Padding: [2]float64{16, 24}},
			Shadow: &textrender.ShadowStyle{Color: "#000000", Alpha: 0.3, Blur: 10, OffsetY: 4},
			Align:  "left", LineSpacing: 1.4, MaxWidthPct: 0.6}, ctx)
		drawAt(dc, panel, int(0.05*float64(W)), int(0.06*H), mp)
	}

	titleFont := "PaytoneOne-Regular.ttf" // Canva Trang 7: Paytone One (sans đậm tròn)
	if cfg.TitleFontFile != "" {
		titleFont = cfg.TitleFontFile
	}
	title, mt := renderEl(&textrender.ElementStyle{Text: videoNickname(cfg), FontFile: titleFont,
		SizePct: 0.10, Color: "#FFFFFF",
		Shadow: &textrender.ShadowStyle{Color: "#000000", Alpha: 0.5, Blur: 10, OffsetY: 4},
		Align:  "center", MaxWidthPct: 0.9}, ctx)
	drawCX(dc, title, W, int(0.60*H), mt)
	titleBottom := int(0.60*H) + contentH(title, mt)

	drawPillAddress(dc, ctx, cfg.Address, "Montserrat-SemiBold.ttf", 0.026*bs,
		"#812C2C", cream, 0.92, 30, 14, 32, "", 0, 0, W, titleBottom+int(0.022*H))
}

// ─── 8. ntgroom (Canva Trang 8) ────────────────────────────────────────────────
// Panel giá maroon trong + viền trắng nhạt (trên-trái), NTG402 serif trắng to,
// pill "Room" cam đè góc dưới-phải, pill địa chỉ xanh lá viền trắng + ghim.
func drawNtgRoomVideo(dc *gg.Context, ctx *textrender.RenderContext, cfg Config) {
	W, H := cfg.Width, float64(cfg.Height)
	bs := videoBodyScale(cfg)
	xL := int(0.05 * float64(W))

	if p := strings.Join(priceGroups(cfg.Prices), "\n"); strings.TrimSpace(p) != "" {
		panel, mp := renderEl(&textrender.ElementStyle{Text: p,
			FontFile: "Montserrat-Regular.ttf", SizePct: 0.028 * bs, Color: "#FFFFFF",
			Bg: &textrender.BgStyle{Color: "#812C2C", Alpha: 0.42, Radius: 34, Padding: [2]float64{20, 28},
				BorderColor: "#FFFFFF", BorderWidth: 3, BorderAlpha: 0.55},
			Shadow: &textrender.ShadowStyle{Color: "#000000", Alpha: 0.35, Blur: 8, OffsetY: 3},
			Align:  "left", LineSpacing: 1.4, MaxWidthPct: 0.55}, ctx)
		drawAt(dc, panel, xL, int(0.07*H), mp)
	}

	titleFont := "YesevaOne-Regular.ttf"
	if cfg.TitleFontFile != "" {
		titleFont = cfg.TitleFontFile
	}
	// Pill "Room" cỡ cố định — dựng TRƯỚC để biết bề rộng khi xếp tên (vẽ sau, đè lên tên).
	room, mR := renderEl(&textrender.ElementStyle{Text: "Room", FontFile: "YesevaOne-Regular.ttf",
		SizePct: 0.085, Color: "#FFFFFF",
		Bg:     &textrender.BgStyle{Color: "#F3A01C", Alpha: 1, Radius: 60, Padding: [2]float64{8, 54}},
		Shadow: &textrender.ShadowStyle{Color: "#000000", Alpha: 0.25, Blur: 8, OffsetY: 3},
		Align:  "center"}, ctx)
	roomW := contentW(room, mR)
	roomH := contentH(room, mR)

	// Tên là hero serif 1 dòng. mkTitle tạo style mới mỗi lần (renderElFitWidth ghi đè SizePct).
	mkTitle := func() *textrender.ElementStyle {
		return &textrender.ElementStyle{Text: videoNickname(cfg), FontFile: titleFont,
			SizePct: 0.15, Color: "#FFFFFF",
			Shadow: &textrender.ShadowStyle{Color: "#000000", Alpha: 0.5, Blur: 10, OffsetY: 4},
			Align:  "left"}
	}
	ntgY := int(0.50 * H)
	// Lần dựng đầu (cho phép tới 0.72·W). Tên NGẮN (mã phòng "NTG402") không bị co.
	ntg, mN := renderElFitWidth(mkTitle(), ctx, 0.72)
	ntgW := contentW(ntg, mN)
	ntgH := contentH(ntg, mN)

	var roomY int
	if ntgW < int(0.68*float64(W)) {
		// Tên NGẮN (mã phòng): pill đè ĐUÔI y như Canva ("NTG402" → pill đè "402").
		drawAt(dc, ntg, xL, ntgY, mN)
		roomX := xL + ntgW - int(float64(roomW)*0.82)
		if roomX < xL {
			roomX = xL
		}
		roomY = ntgY + ntgH - int(float64(roomH)*0.55)
		drawAt(dc, room, roomX, roomY, mR)
	} else {
		// Tên DÀI ("p602 - phòng có ban công", "Chocolate – Smart Home"): co tên để
		// CÒN CHỖ cho pill đặt SAU tên (không che chữ), pill căn giữa theo tên.
		gap := int(0.012 * float64(W))
		titleFrac := float64(W-2*xL-roomW-gap) / float64(W)
		ntg, mN = renderElFitWidth(mkTitle(), ctx, titleFrac)
		ntgW = contentW(ntg, mN)
		ntgH = contentH(ntg, mN)
		drawAt(dc, ntg, xL, ntgY, mN)
		roomX := xL + ntgW + gap // khe sạch, KHÔNG đè chữ (tên đã co chừa đủ chỗ)
		if roomX+roomW > W-xL {
			roomX = W - xL - roomW
		}
		roomY = ntgY + ntgH/2 - roomH/2
		drawAt(dc, room, roomX, roomY, mR)
	}
	roomBottom := roomY + roomH

	drawPillAddress(dc, ctx, cfg.Address, "Montserrat-SemiBold.ttf", 0.026*bs,
		"#FFFFFF", "#1F6B3A", 1, 40, 16, 70, "#FFFFFF", 6, 1, W, roomBottom+int(0.03*H))
}
