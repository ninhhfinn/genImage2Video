package main

import (
	"testing"

	"img2video/textrender"
)

// TestTitleFontFileOverride proves an uploaded/custom title font actually
// reaches the rendered title + nickname elements (the gen-video path), and that
// leaving it empty does not disturb the template's own font.
func TestTitleFontFileOverride(t *testing.T) {
	tmpl, err := textrender.LoadTemplate("daiky", assetsTemplatesDir())
	if err != nil {
		t.Fatalf("load template: %v", err)
	}
	applyConfigOverrides(tmpl, Config{Template: "daiky", TitleFontFile: "BeVietnamPro-Bold.ttf"})
	for _, key := range []string{"title", "nickname"} {
		st := tmpl.Elements[key]
		got := "<nil>"
		if st != nil {
			got = st.FontFile
		}
		if got != "BeVietnamPro-Bold.ttf" {
			t.Errorf("%s FontFile = %q, want BeVietnamPro-Bold.ttf", key, got)
		}
	}

	tmpl2, _ := textrender.LoadTemplate("daiky", assetsTemplatesDir())
	orig := tmpl2.Elements["title"].FontFile
	applyConfigOverrides(tmpl2, Config{Template: "daiky"}) // no TitleFontFile
	if tmpl2.Elements["title"].FontFile != orig {
		t.Errorf("empty TitleFontFile changed title font from %q to %q", orig, tmpl2.Elements["title"].FontFile)
	}
}

// TestFontFileForChoicePassthrough covers the mapping that lets uploaded fonts
// (and raw filenames) work in the existing listing-font selector.
func TestFontFileForChoicePassthrough(t *testing.T) {
	cases := map[string]string{
		"playfair":                     "PlayfairDisplay-Bold.ttf",
		"lilita":                       "LilitaOne-Regular.ttf",
		"uploads/my-font-ab12cd34.ttf": "uploads/my-font-ab12cd34.ttf",
		"Custom.OTF":                   "Custom.OTF",
		"notafont":                     "",
		"":                             "",
	}
	for in, want := range cases {
		if got := fontFileForChoice(in); got != want {
			t.Errorf("fontFileForChoice(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestResolveFontFileGuards confirms the preview/delete path rejects traversal
// and unknown fonts while accepting a shipped font.
func TestResolveFontFileGuards(t *testing.T) {
	if _, err := resolveFontFile("../../etc/passwd"); err == nil {
		t.Errorf("traversal not rejected")
	}
	if _, err := resolveFontFile("definitely-not-here.ttf"); err == nil {
		t.Errorf("unknown font not rejected")
	}
	if got, err := resolveFontFile("Prata-Regular.ttf"); err != nil || got != "Prata-Regular.ttf" {
		t.Errorf("shipped font rejected: got %q err %v", got, err)
	}
}
