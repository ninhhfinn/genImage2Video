package textrender

import (
	"os"
	"testing"
)

func TestListTemplates(t *testing.T) {
	list, err := ListTemplates("../assets/templates")
	if err != nil {
		t.Fatalf("ListTemplates: %v", err)
	}
	want := map[string]bool{"daiky": true, "sunset": true}
	got := map[string]bool{}
	for _, tt := range list {
		got[tt.Name] = true
		if tt.DisplayName == "" {
			t.Errorf("%s: empty DisplayName", tt.Name)
		}
	}
	for name := range want {
		if !got[name] {
			t.Errorf("missing template %s", name)
		}
	}
}

func TestRenderAllTemplates(t *testing.T) {
	// Test data simulating a real listing for visual inspection.
	contents := map[string]string{
		"address":    "Ngõ 171 Nguyễn Ngọc Vũ, Cầu Giấy",
		"nickname":   "Daiky home",
		"prices":     "2h 249k · 4h 367k · Qua đêm 449k",
		"amenities":  "Máy chiếu netflix · Bếp nấu · Wc khép kín",
		"listing_id": "ID 12345",
		"watermark":  "@yourchannel",
	}
	for _, name := range []string{"daiky", "sunset"} {
		tmpl, err := LoadTemplate(name, "../assets/templates")
		if err != nil {
			t.Errorf("load %s: %v", name, err)
			continue
		}
		ctx := &RenderContext{VideoWidth: 1080, VideoHeight: 1920, AssetsDir: "../assets"}
		os.MkdirAll("/tmp/img2video_smoke/templates_"+name, 0755)
		for key, text := range contents {
			st, ok := tmpl.Elements[key]
			if !ok {
				continue
			}
			s := st.Clone()
			s.Text = text
			out, err := Render(s, ctx)
			if err != nil {
				t.Errorf("%s/%s: %v", name, key, err)
				continue
			}
			out.Save("/tmp/img2video_smoke/templates_" + name + "/" + key + ".png")
		}
		t.Logf("→ /tmp/img2video_smoke/templates_%s/", name)
	}
}
