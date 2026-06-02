package textrender

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// LoadTemplate reads <templatesDir>/<name>.json and unmarshals it.
func LoadTemplate(name, templatesDir string) (*Template, error) {
	name = strings.TrimSpace(strings.ToLower(name))
	if name == "" {
		return nil, fmt.Errorf("textrender: empty template name")
	}
	path := filepath.Join(templatesDir, name+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("textrender: load template %s: %v", name, err)
	}
	var t Template
	if err := json.Unmarshal(data, &t); err != nil {
		return nil, fmt.Errorf("textrender: parse template %s: %v", name, err)
	}
	if t.Name == "" {
		t.Name = name
	}
	if t.Elements == nil {
		t.Elements = map[string]*ElementStyle{}
	}
	return &t, nil
}

// ListTemplates returns metadata for every *.json file in templatesDir, sorted by name.
// Element bodies are stripped — this is only for UI listing.
func ListTemplates(templatesDir string) ([]Template, error) {
	entries, err := os.ReadDir(templatesDir)
	if err != nil {
		return nil, err
	}
	var out []Template
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".json")
		t, err := LoadTemplate(name, templatesDir)
		if err != nil {
			continue
		}
		out = append(out, Template{
			Name:        t.Name,
			DisplayName: t.DisplayName,
			Description: t.Description,
			// Elements omitted intentionally for listing
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// Merge returns a copy of base with elements from override applied
// (override takes precedence; missing keys in override fall through to base).
func Merge(base *Template, override map[string]*ElementStyle) *Template {
	if base == nil {
		return &Template{Elements: copyElements(override)}
	}
	out := &Template{
		Name:        base.Name,
		DisplayName: base.DisplayName,
		Description: base.Description,
		Elements:    map[string]*ElementStyle{},
		Stack:       base.Stack,
	}
	for k, v := range base.Elements {
		out.Elements[k] = cloneStyle(v)
	}
	for k, v := range override {
		out.Elements[k] = cloneStyle(v)
	}
	return out
}

// Clone returns a deep copy of an ElementStyle.
func (s *ElementStyle) Clone() *ElementStyle {
	return cloneStyle(s)
}

func cloneStyle(s *ElementStyle) *ElementStyle {
	if s == nil {
		return nil
	}
	c := *s
	if s.Bg != nil {
		bg := *s.Bg
		c.Bg = &bg
	}
	if s.Shadow != nil {
		sh := *s.Shadow
		c.Shadow = &sh
	}
	if s.Stroke != nil {
		st := *s.Stroke
		c.Stroke = &st
	}
	return &c
}

func copyElements(src map[string]*ElementStyle) map[string]*ElementStyle {
	out := map[string]*ElementStyle{}
	for k, v := range src {
		out[k] = cloneStyle(v)
	}
	return out
}
