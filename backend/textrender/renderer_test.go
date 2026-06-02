package textrender

import (
	"os"
	"testing"
)

func TestRenderDaikyPill(t *testing.T) {
	style := &ElementStyle{
		Text:     "Daiky home",
		FontFile: "PlayfairDisplay-Variable.ttf",
		SizePct:  0.085,
		Color:    "#FFFFFF",
		Bg: &BgStyle{
			Color:   "#9B7D5B",
			Alpha:   0.92,
			Radius:  32,
			Padding: [2]float64{20, 48},
		},
		Shadow: &ShadowStyle{
			Color:   "#000000",
			Alpha:   0.30,
			Blur:    12,
			OffsetX: 0,
			OffsetY: 6,
		},
		Position:    Position{X: "center", Y: "0.45"},
		Align:       "center",
		MaxWidthPct: 0.85,
	}
	ctx := &RenderContext{
		VideoWidth:  1080,
		VideoHeight: 1920,
		AssetsDir:   "../assets",
	}
	out, err := Render(style, ctx)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if out == nil {
		t.Fatal("Render returned nil")
	}
	os.MkdirAll("/tmp/img2video_smoke", 0755)
	if err := out.Save("/tmp/img2video_smoke/textrender_daiky.png"); err != nil {
		t.Fatal(err)
	}
	t.Logf("→ /tmp/img2video_smoke/textrender_daiky.png  %dx%d at (%d,%d)",
		out.Width, out.Height, out.X, out.Y)
}

func TestRenderVietnameseAddress(t *testing.T) {
	style := &ElementStyle{
		Text:     "Ngõ 171 Nguyễn Ngọc Vũ",
		FontFile: "PlayfairDisplay-Variable.ttf",
		SizePct:  0.030,
		Color:    "#FFFFFF",
		Bg: &BgStyle{
			Color:   "#9B7D5B",
			Alpha:   0.85,
			Radius:  24,
			Padding: [2]float64{12, 28},
		},
		Position: Position{X: "center", Y: "0.38"},
		Align:    "center",
	}
	ctx := &RenderContext{
		VideoWidth: 1080, VideoHeight: 1920,
		AssetsDir: "../assets",
	}
	out, err := Render(style, ctx)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	out.Save("/tmp/img2video_smoke/textrender_vn_address.png")
	t.Logf("→ /tmp/img2video_smoke/textrender_vn_address.png")
}
