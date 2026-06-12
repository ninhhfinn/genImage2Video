package main

// Faithful video mockups v3 — applies the fixes from the comparison workflow:
// prices "299,000" (comma, no đ), comma subtitle separators, no watermark,
// soft veil instead of a hard scrim band, text gathered higher & tighter,
// white stroke on gold title, red 3D extrude on marquee, frosted price panel,
// vector icons (door/leaf/pin). Run: GENMOCKUP=1 go test -run TestGenVideoFaithful .

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"testing"

	"github.com/disintegration/imaging"
	"github.com/fogleman/gg"
	"img2video/textrender"
)

type sh = textrender.ShadowStyle
type stk = textrender.StrokeStyle
type bgp = textrender.BgStyle

func el(ctx *textrender.RenderContext, st *textrender.ElementStyle) (image.Image, int) {
	return renderEl(st, ctx)
}

func drawRight(dc *gg.Context, img image.Image, xRight, yTop, m int) {
	if img == nil {
		return
	}
	cw := img.Bounds().Dx() - 2*m
	dc.DrawImage(img, xRight-cw-m, yTop-m)
}

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
// gradient — no hard band edge (the v2 scrim band was the #1 complaint).
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

func soft() *sh   { return &sh{Color: "#000000", Alpha: 0.5, Blur: 9, OffsetY: 3} }
func strong() *sh { return &sh{Color: "#000000", Alpha: 0.62, Blur: 6, OffsetY: 2} }

func encodeJPG(t *testing.T, dc *gg.Context, name string) {
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, dc.Image(), &jpeg.Options{Quality: 90}); err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	os.WriteFile(filepath.Join(mockOut, name), buf.Bytes(), 0o644)
	t.Logf("✓ %s", name)
}

// drawExtrude renders `text` with a red 3D extrude (offset red copies) under a
// gold fill, centred horizontally at cx, content-top at yTop. Returns bottom y.
func drawExtrude(dc *gg.Context, ctx *textrender.RenderContext, text, font string, size float64, gold, red string, cx float64, yTop int) int {
	redImg, mR := el(ctx, &textrender.ElementStyle{Text: text, FontFile: font, SizePct: size, Color: red, Align: "center"})
	goldImg, mG := el(ctx, &textrender.ElementStyle{Text: text, FontFile: font, SizePct: size, Color: gold, Align: "center"})
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

// drawPriceDoors draws price lines (each with a trailing orange door icon).
// leftAlign=false centres each line around centerX.
func drawPriceDoors(dc *gg.Context, ctx *textrender.RenderContext, lines []string, font string, size float64, col string, leftAlign bool, anchorX, yTop, gap int) int {
	fishBody := color.NRGBA{0xF2, 0x8C, 0x3B, 255}
	cy := yTop
	for _, ln := range lines {
		if ln == "" {
			cy += gap
			continue
		}
		img, m := el(ctx, &textrender.ElementStyle{Text: ln, FontFile: font, SizePct: size, Color: col, Shadow: strong(), Align: "left"})
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

func TestGenVideoFaithful(t *testing.T) {
	if os.Getenv("GENMOCKUP") == "" {
		t.Skip("set GENMOCKUP=1")
	}
	os.MkdirAll(mockOut, 0o755)
	const W, H = 1080, 1920
	ctx := &textrender.RenderContext{VideoWidth: W, VideoHeight: H, AssetsDir: assetsDir()}

	// 1 ── staycation
	{
		dc := gg.NewContext(W, H)
		vbg(dc, refV1, W, H, 10, 0, 0.62, 1, 1)
		softVeil(dc, W, H, 45, 70)
		drawStaycation(dc, ctx, W, H)
		encodeJPG(t, dc, "video-1-staycation.jpg")
	}
	// 2 ── creampill
	{
		dc := gg.NewContext(W, H)
		vbg(dc, refV2, W, H, -8, 0, 0.46, 1, 1)
		softVeil(dc, W, H, 45, 70)
		drawCreampill(dc, ctx, W, H)
		encodeJPG(t, dc, "video-2-creampill.jpg")
	}
	// 3 ── goldserif
	{
		dc := gg.NewContext(W, H)
		vbg(dc, refV3, W, H, 10, 0, 0.54, 1, 1)
		softVeil(dc, W, H, 45, 70)
		drawGoldSerif(dc, ctx, W, H)
		encodeJPG(t, dc, "video-3-goldserif.jpg")
	}
	// 4 ── editorial
	{
		dc := gg.NewContext(W, H)
		vbg(dc, refV4, W, H, -6, 0, 0.56, 1, 1)
		softVeil(dc, W, H, 40, 50)
		drawEditorial(dc, ctx, W, H)
		encodeJPG(t, dc, "video-4-editorial.jpg")
	}
	// 5 ── marquee
	{
		dc := gg.NewContext(W, H)
		vbg(dc, refV4, W, H, -14, 0, 0.56, 1, 1)
		softVeil(dc, W, H, 60, 90)
		drawMarquee(dc, ctx, W, H)
		encodeJPG(t, dc, "video-5-marquee.jpg")
	}
	// 6 ── chillgreen
	{
		dc := gg.NewContext(W, H)
		vbg(dc, refV1, W, H, 6, 0, 0.62, 1, 1)
		softVeil(dc, W, H, 55, 85)
		drawChill(dc, ctx, W, H)
		encodeJPG(t, dc, "video-6-chillgreen.jpg")
	}
}

// ── 1. staycation ─────────────────────────────────────────────────────────────
func drawStaycation(dc *gg.Context, ctx *textrender.RenderContext, W, H int) {
	xL := int(0.07 * float64(W))
	eyL, mL := el(ctx, &textrender.ElementStyle{Text: "Staycation", FontFile: "DancingScript-Variable.ttf",
		SizePct: 0.052, Color: "#FFFFFF", Shadow: soft(), Align: "left"})
	eyR, mR := el(ctx, &textrender.ElementStyle{Text: "Cầu Giấy", FontFile: "DancingScript-Variable.ttf",
		SizePct: 0.052, Color: "#FFFFFF", Shadow: soft(), Align: "left"})
	drawAt(dc, eyL, xL+int(0.012*float64(W)), int(0.265*float64(H)), mL)
	drawRight(dc, eyR, int(0.95*float64(W)), int(0.27*float64(H)), mR)

	title, mT := el(ctx, &textrender.ElementStyle{Text: "Amoureux NK605", FontFile: "YesevaOne-Regular.ttf",
		SizePct: 0.082, Color: "#FFFFFF", Shadow: soft(), Align: "left", MaxWidthPct: 0.9})
	drawAt(dc, title, xL, int(0.315*float64(H)), mT)

	sub, mS := el(ctx, &textrender.ElementStyle{Text: "Máy chiếu Netflix, bồn tắm, boardgame",
		FontFile: "BeVietnamPro-Bold.ttf", SizePct: 0.0265, Color: "#FFFFFF", Shadow: strong(),
		Align: "left", MaxWidthPct: 0.9})
	drawAt(dc, sub, xL+int(0.012*float64(W)), int(0.405*float64(H)), mS)

	pr, mP := el(ctx, &textrender.ElementStyle{
		Text: "2N1Đ: 299,000\nQua đêm: 299,000\nCombo theo giờ: 299,000",
		FontFile: "Prata-Regular.ttf", SizePct: 0.030, Color: "#FFFFFF", Shadow: strong(),
		Align: "left", LineSpacing: 1.45, MaxWidthPct: 0.9})
	drawAt(dc, pr, xL+int(0.012*float64(W)), int(0.46*float64(H)), mP)
}

// ── 2. creampill ──────────────────────────────────────────────────────────────
func drawCreampill(dc *gg.Context, ctx *textrender.RenderContext, W, H int) {
	top, mt := el(ctx, &textrender.ElementStyle{Text: "Qua đêm: 299,000   -   Combo: 299,000",
		FontFile: "BeVietnamPro-Bold.ttf", SizePct: 0.024, Color: "#FFFFFF",
		Shadow: &sh{Color: "#000000", Alpha: 0.75, Blur: 7, OffsetY: 2}, Align: "center", MaxWidthPct: 0.92})
	drawCX(dc, top, W, int(0.165*float64(H)), mt)

	pill, mp := el(ctx, &textrender.ElementStyle{Text: "Amoureux NK605", FontFile: "Inter-Bold.ttf",
		SizePct: 0.064, Color: "#D33B2C",
		Bg:     &bgp{Color: "#FBE6C2", Alpha: 0.97, Radius: 22, Padding: [2]float64{16, 40}},
		Shadow: &sh{Color: "#000000", Alpha: 0.35, Blur: 12, OffsetY: 5}, Align: "center", MaxWidthPct: 0.92})
	drawCX(dc, pill, W, int(0.205*float64(H)), mp)

	addr, ma := el(ctx, &textrender.ElementStyle{Text: "Đường Nguyễn Khang, Cầu Giấy",
		FontFile: "BeVietnamPro-Bold.ttf", SizePct: 0.026, Color: "#FFFFFF",
		Shadow: &sh{Color: "#000000", Alpha: 0.75, Blur: 7, OffsetY: 2}, Align: "center", MaxWidthPct: 0.9})
	drawCX(dc, addr, W, int(0.285*float64(H)), ma)
}

// ── 3. goldserif ──────────────────────────────────────────────────────────────
func drawGoldSerif(dc *gg.Context, ctx *textrender.RenderContext, W, H int) {
	gold := "#FFC107"
	addr, ma := el(ctx, &textrender.ElementStyle{Text: "Đường Nguyễn Khang, Cầu Giấy",
		FontFile: "BeVietnamPro-Bold.ttf", SizePct: 0.024, Color: "#FFFFFF", Shadow: strong(),
		Align: "center", MaxWidthPct: 0.9})
	drawCX(dc, addr, W, int(0.155*float64(H)), ma)

	title, mt := el(ctx, &textrender.ElementStyle{Text: "Amoureux NK605", FontFile: "YesevaOne-Regular.ttf",
		SizePct: 0.078, Color: gold, Stroke: &stk{Color: "#FFFFFF", Width: 4.5, Alpha: 1},
		Shadow: &sh{Color: "#000000", Alpha: 0.5, Blur: 8, OffsetY: 3}, Align: "center", MaxWidthPct: 0.94})
	drawCX(dc, title, W, int(0.175*float64(H)), mt)

	rows := []struct {
		text, col string
		size, y   float64
	}{
		{"Giá theo ngày", gold, 0.027, 0.265},
		{"2N1Đ: 390,000", "#FFFFFF", 0.024, 0.305},
		{"Qua đêm: 290,000", "#FFFFFF", 0.024, 0.340},
		{"Giá theo giờ", gold, 0.027, 0.395},
		{"14h - 20h: 390,000", "#FFFFFF", 0.024, 0.435},
		{"14h - 18h: 290,000", "#FFFFFF", 0.024, 0.470},
	}
	for _, r := range rows {
		img, m := el(ctx, &textrender.ElementStyle{Text: r.text, FontFile: "BeVietnamPro-Bold.ttf",
			SizePct: r.size, Color: r.col, Shadow: strong(), Align: "center", MaxWidthPct: 0.92})
		drawCX(dc, img, W, int(r.y*float64(H)), m)
	}
}

// ── 4. editorial ──────────────────────────────────────────────────────────────
func drawEditorial(dc *gg.Context, ctx *textrender.RenderContext, W, H int) {
	xL := int(0.05 * float64(W))
	// brand top-center
	brand, mBr := el(ctx, &textrender.ElementStyle{Text: "Dayladau House", FontFile: "Prata-Regular.ttf",
		SizePct: 0.026, Color: "#FFFFFF", Shadow: soft(), Align: "center"})
	drawCX(dc, brand, W, int(0.035*float64(H)), mBr)

	big, mb := el(ctx, &textrender.ElementStyle{Text: "Standard", FontFile: "PlayfairDisplay-Variable.ttf",
		SizePct: 0.135, Color: "#FFFFFF", Shadow: soft(), Align: "left", Curve: 22})
	drawAt(dc, big, xL, int(0.115*float64(H)), mb)
	bigBottom := int(0.115*float64(H)) + contentH(big, mb)
	bigRight := xL + contentW(big, mb)

	// script "room" sits BELOW-right, just under Standard's baseline (not on the letters)
	script, ms := el(ctx, &textrender.ElementStyle{Text: "room", FontFile: "DancingScript-Variable.ttf",
		SizePct: 0.078, Color: "#FFFFFF", Shadow: soft(), Align: "left"})
	drawAt(dc, script, bigRight-contentW(script, ms)+int(0.04*float64(W)), bigBottom-int(0.006*float64(H)), ms)

	// address with pin, below the Standard/room headline cluster
	addrY := bigBottom + int(0.055*float64(H))
	addr, ma := el(ctx, &textrender.ElementStyle{Text: "số 4 ngõ 444 Đội Cấn, Ba Đình",
		FontFile: "BeVietnamPro-Bold.ttf", SizePct: 0.022, Color: "#FFFFFF", Shadow: strong(),
		Align: "left", MaxWidthPct: 0.6})
	pinX := float64(xL) + 0.012*float64(W)
	drawPinIcon(dc, pinX, float64(addrY), float64(contentH(addr, ma)), color.NRGBA{255, 255, 255, 255})
	drawAt(dc, addr, xL+int(0.04*float64(W)), addrY, ma)

	// frosted-light price panel on the right, dark text
	panel, mp := el(ctx, &textrender.ElementStyle{
		Text: "Giá giờ: 349K/2H\nGiá đêm/ngày: 589K\nGiá ngày/đêm: 689K",
		FontFile: "BeVietnamPro-Bold.ttf", SizePct: 0.023, Color: "#3A352F",
		Bg:     &bgp{Color: "#F7F1E6", Alpha: 0.82, Radius: 24, Padding: [2]float64{20, 28}},
		Shadow: &sh{Color: "#000000", Alpha: 0.28, Blur: 12, OffsetY: 5}, Align: "left", LineSpacing: 1.6,
		MaxWidthPct: 0.46})
	drawRight(dc, panel, int(0.96*float64(W)), int(0.40*float64(H)), mp)
}

// ── 5. marquee ────────────────────────────────────────────────────────────────
func drawMarquee(dc *gg.Context, ctx *textrender.RenderContext, W, H int) {
	gold, red := "#FFC107", "#B0161A"
	// line 1: Summer  [Date]  Hà Nội
	dateImg, mD := el(ctx, &textrender.ElementStyle{Text: "Date", FontFile: "YesevaOne-Regular.ttf",
		SizePct: 0.10, Color: gold, Align: "center"})
	dW := contentW(dateImg, mD)
	dH := contentH(dateImg, mD)
	dTop := int(0.225 * float64(H))
	drawExtrude(dc, ctx, "Date", "YesevaOne-Regular.ttf", 0.10, gold, red, float64(W)/2, dTop)
	dLeft := (W - dW) / 2
	vCenter := dTop + dH/2
	gap := int(0.022 * float64(W))
	summer, mSu := el(ctx, &textrender.ElementStyle{Text: "Summer", FontFile: "Prata-Regular.ttf",
		SizePct: 0.044, Color: "#F2E2B6", Shadow: soft(), Align: "center"})
	hanoi, mHa := el(ctx, &textrender.ElementStyle{Text: "Hà Nội", FontFile: "Prata-Regular.ttf",
		SizePct: 0.044, Color: "#F2E2B6", Shadow: soft(), Align: "center"})
	drawRight(dc, summer, dLeft-gap, vCenter-contentH(summer, mSu)/2, mSu)
	drawAt(dc, hanoi, dLeft+dW+gap, vCenter-contentH(hanoi, mHa)/2, mHa)

	// line 2: Homestay (extrude), tight under line 1
	homeBottom := drawExtrude(dc, ctx, "Homestay", "YesevaOne-Regular.ttf", 0.115, gold, red, float64(W)/2, dTop+int(0.072*float64(H)))

	sub, mS := el(ctx, &textrender.ElementStyle{Text: "Máy chiếu Netflix – Bếp cooking date",
		FontFile: "BeVietnamPro-Italic.ttf", SizePct: 0.025, Color: "#F2E2B6", Shadow: strong(),
		Align: "center", MaxWidthPct: 0.9})
	drawCX(dc, sub, W, homeBottom+int(0.02*float64(H)), mS)

	yTop := homeBottom + int(0.075*float64(H))
	drawPriceDoors(dc, ctx, []string{
		"21h - 9h (Qua đêm): 314", "2N1Đ: 449", "",
		"10h - 13h: 199", "10h - 20h: 299", "14h - 20h: 279",
	}, "BeVietnamPro-Bold.ttf", 0.024, "#FFFFFF", false, W/2, yTop, int(0.012*float64(H)))
}

// ── 6. chillgreen ─────────────────────────────────────────────────────────────
func drawChill(dc *gg.Context, ctx *textrender.RenderContext, W, H int) {
	green := "#A4C639"
	leaf := color.NRGBA{0x8F, 0xB8, 0x3A, 255}
	addr, ma := el(ctx, &textrender.ElementStyle{Text: "Thanh Xuân - Hà Nội",
		FontFile: "BeVietnamPro-Bold.ttf", SizePct: 0.024, Color: "#FFFFFF", Shadow: strong(),
		Align: "center", MaxWidthPct: 0.9})
	drawCX(dc, addr, W, int(0.20*float64(H)), ma)

	h1, m1 := el(ctx, &textrender.ElementStyle{Text: "Homestay chữa lành siu chill",
		FontFile: "BeVietnamPro-Italic.ttf", SizePct: 0.038, Color: green,
		Stroke: &stk{Color: "#2B4A12", Width: 2.4, Alpha: 1}, Shadow: &sh{Color: "#0c2a06", Alpha: 0.7, Blur: 6, OffsetY: 2},
		Align: "center", MaxWidthPct: 0.84})
	y1 := int(0.255 * float64(H))
	drawCX(dc, h1, W, y1, m1)
	// leaf before line 1 (left of content)
	c1w := contentW(h1, m1)
	c1h := contentH(h1, m1)
	drawLeafIcon(dc, float64((W-c1w)/2)-float64(c1h)*0.55, float64(y1)+float64(c1h)*0.5, float64(c1h)*0.95, -25, leaf)

	h2, m2 := el(ctx, &textrender.ElementStyle{Text: "đầy đủ tiện nghi, máy chiếu Netflix",
		FontFile: "BeVietnamPro-Italic.ttf", SizePct: 0.038, Color: green,
		Stroke: &stk{Color: "#2B4A12", Width: 2.4, Alpha: 1}, Shadow: &sh{Color: "#0c2a06", Alpha: 0.7, Blur: 6, OffsetY: 2},
		Align: "center", MaxWidthPct: 0.84})
	y2 := y1 + c1h + int(0.012*float64(H))
	drawCX(dc, h2, W, y2, m2)
	c2w := contentW(h2, m2)
	c2h := contentH(h2, m2)
	drawLeafIcon(dc, float64((W+c2w)/2)+float64(c2h)*0.55, float64(y2)+float64(c2h)*0.5, float64(c2h)*0.95, 25, leaf)

	// price block, left-aligned, door icons
	yTop := y2 + c2h + int(0.04*float64(H))
	leftX := int(0.28 * float64(W))
	drawPriceDoors(dc, ctx, []string{
		"21h - 9h (Qua đêm): 279", "2N1Đ: 399", "",
		"10h - 13h: 299", "10h - 20h: 299", "14h - 20h: 299",
	}, "BeVietnamPro-Bold.ttf", 0.023, "#FFFFFF", true, leftX, yTop, int(0.011*float64(H)))
}
