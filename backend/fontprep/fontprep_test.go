package fontprep

import (
	"os"
	"path/filepath"
	"testing"
)

func read(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "assets", "fonts", name))
	if err != nil {
		t.Skipf("font %s không có: %v", name, err)
	}
	return data
}

func TestInspectKnownFonts(t *testing.T) {
	cases := []struct {
		file         string
		wantVariable bool
		wantVietOK   bool
		wantRender   bool
	}{
		{"BeVietnamPro-Bold.ttf", false, true, true}, // VN-native static — the gold standard
		{"Baloo2-Bold.ttf", false, true, true},       // static instance we ship
		{"Baloo2-Variable.ttf", true, true, true},    // variable → renders thin under gg
		{"Fredoka-Bold.ttf", true, false, true},      // missing VN glyphs → tofu
	}
	for _, c := range cases {
		t.Run(c.file, func(t *testing.T) {
			rep := Inspect(read(t, c.file))
			if rep.Variable != c.wantVariable {
				t.Errorf("Variable = %v, want %v", rep.Variable, c.wantVariable)
			}
			if rep.VietnameseOK != c.wantVietOK {
				t.Errorf("VietnameseOK = %v, want %v (missing %q)", rep.VietnameseOK, c.wantVietOK, rep.MissingVietnamese)
			}
			if rep.RenderableByGG != c.wantRender {
				t.Errorf("RenderableByGG = %v, want %v", rep.RenderableByGG, c.wantRender)
			}
			if c.wantVariable && len(rep.Axes) == 0 {
				t.Errorf("variable font reported no axes")
			}
		})
	}
}

func TestInspectGarbage(t *testing.T) {
	// Must not panic on non-font bytes.
	rep := Inspect([]byte("this is definitely not a font file"))
	if rep.RenderableByGG {
		t.Errorf("garbage reported as renderable")
	}
}

func TestPrepareInstancesVariable(t *testing.T) {
	if !HasFontTools() {
		t.Skip("fonttools không có — bỏ qua instancing")
	}
	out, rep, instanced, err := Prepare(read(t, "Baloo2-Variable.ttf"))
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if !instanced {
		t.Fatalf("variable font was not instanced")
	}
	if rep.Variable {
		t.Errorf("instanced font still reports Variable=true")
	}
	if len(out) == 0 {
		t.Errorf("empty instanced output")
	}
	// The instanced bytes must themselves no longer carry an fvar table.
	if tableData(out, "fvar") != nil {
		t.Errorf("instanced output still has fvar table")
	}
}
