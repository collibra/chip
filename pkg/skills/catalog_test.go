package skills

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"testing/fstest"
)

func TestLoadFromFS_parsesFrontmatterAndResources(t *testing.T) {
	fsys := fstest.MapFS{
		"files/collibra/lineage/SKILL.md": &fstest.MapFile{Data: []byte(
			`---
description: Trace lineage.
related: collibra/discovery, collibra/index
---

# Body
content
`)},
		"files/collibra/lineage/references/column-lineage.md": &fstest.MapFile{Data: []byte("# Column lineage")},
		"files/collibra/lineage/references/id-bridging.md":    &fstest.MapFile{Data: []byte("# ID bridging")},
		"files/collibra/index/SKILL.md":                       &fstest.MapFile{Data: []byte("---\ndescription: Navigator.\n---\n\n# Index\n")},
	}

	cat, err := loadFromFS(fsys, "files")
	if err != nil {
		t.Fatalf("loadFromFS: %v", err)
	}

	lineage := cat.Get("collibra/lineage")
	if lineage == nil {
		t.Fatal("expected collibra/lineage skill")
	}
	if lineage.Description != "Trace lineage." {
		t.Errorf("description = %q", lineage.Description)
	}
	if !slices.Equal(lineage.Related, []string{"collibra/discovery", "collibra/index"}) {
		t.Errorf("related = %v", lineage.Related)
	}
	wantPaths := []string{"references/column-lineage.md", "references/id-bridging.md"}
	if !slices.Equal(lineage.ResourcePaths(), wantPaths) {
		t.Errorf("resource paths = %v, want %v", lineage.ResourcePaths(), wantPaths)
	}
	if lineage.Resource("references/column-lineage.md") == nil {
		t.Error("expected column-lineage.md resource")
	}
	if lineage.Resource("references/missing.md") != nil {
		t.Error("expected nil for missing resource")
	}

	if cat.Get("collibra/index") == nil {
		t.Error("expected collibra/index skill")
	}
	if cat.Get("collibra/unknown") != nil {
		t.Error("expected nil for unknown skill")
	}
}

func TestCatalogSearch_filtersByQuery(t *testing.T) {
	fsys := fstest.MapFS{
		"files/collibra/lineage/SKILL.md":   &fstest.MapFile{Data: []byte("---\ndescription: Trace lineage.\n---\n\nbody")},
		"files/collibra/discovery/SKILL.md": &fstest.MapFile{Data: []byte("---\ndescription: Find assets.\n---\n\nbody")},
		"files/collibra/index/SKILL.md":     &fstest.MapFile{Data: []byte("---\ndescription: Navigator.\n---\n\nbody")},
	}
	cat, err := loadFromFS(fsys, "files")
	if err != nil {
		t.Fatal(err)
	}

	if got := len(cat.Search("")); got != 3 {
		t.Errorf("empty query returned %d skills, want 3", got)
	}

	results := cat.Search("LINEAGE")
	if len(results) != 1 || results[0].Name != "collibra/lineage" {
		t.Errorf("query %q returned %v", "LINEAGE", names(results))
	}

	results = cat.Search("Find assets")
	if len(results) != 1 || results[0].Name != "collibra/discovery" {
		t.Errorf("description query returned %v", names(results))
	}

	if got := len(cat.Search("nonsense")); got != 0 {
		t.Errorf("nonsense query returned %d results", got)
	}
}

func TestLoadFromFS_sharedResourcesAttachedDeclaratively(t *testing.T) {
	fsys := fstest.MapFS{
		"files/collibra/_shared/rich-text.md": &fstest.MapFile{Data: []byte("# Shared rich-text rules")},
		"files/collibra/asset-create/SKILL.md": &fstest.MapFile{Data: []byte(
			"---\ndescription: Create.\nshared: rich-text.md\n---\n\nbody")},
		"files/collibra/asset-edit/SKILL.md": &fstest.MapFile{Data: []byte(
			"---\ndescription: Edit.\nshared: rich-text.md\n---\n\nbody")},
		"files/collibra/discovery/SKILL.md": &fstest.MapFile{Data: []byte(
			"---\ndescription: Search.\n---\n\nbody")},
	}

	cat, err := loadFromFS(fsys, "files")
	if err != nil {
		t.Fatalf("loadFromFS: %v", err)
	}

	for _, name := range []string{"collibra/asset-create", "collibra/asset-edit"} {
		s := cat.Get(name)
		if s == nil {
			t.Fatalf("missing skill %s", name)
		}
		r := s.Resource("shared/rich-text.md")
		if r == nil {
			t.Errorf("%s: expected shared/rich-text.md", name)
			continue
		}
		if r.Content != "# Shared rich-text rules" {
			t.Errorf("%s: content = %q", name, r.Content)
		}
	}

	if s := cat.Get("collibra/discovery"); s == nil || len(s.Resources) != 0 {
		t.Errorf("discovery should not receive shared resources, got %v", s.ResourcePaths())
	}
}

func TestLoadFromFS_missingSharedResourceIsError(t *testing.T) {
	fsys := fstest.MapFS{
		"files/collibra/asset-create/SKILL.md": &fstest.MapFile{Data: []byte(
			"---\ndescription: Create.\nshared: nope.md\n---\n\nbody")},
	}
	if _, err := loadFromFS(fsys, "files"); err == nil {
		t.Fatal("expected error for missing shared resource")
	}
}

func TestLoadFromFS_skillWithoutFrontmatter(t *testing.T) {
	fsys := fstest.MapFS{
		"files/collibra/raw/SKILL.md": &fstest.MapFile{Data: []byte("# No frontmatter\nbody")},
	}
	cat, err := loadFromFS(fsys, "files")
	if err != nil {
		t.Fatal(err)
	}
	skill := cat.Get("collibra/raw")
	if skill == nil {
		t.Fatal("expected collibra/raw skill")
	}
	if skill.Description != "" || skill.Related != nil {
		t.Errorf("expected empty metadata, got desc=%q related=%v", skill.Description, skill.Related)
	}
	if skill.Body == "" {
		t.Error("body should be preserved verbatim when no frontmatter")
	}
}

func TestEmbeddedCatalog_loads(t *testing.T) {
	cat, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	required := []string{
		"collibra/index",
		"collibra/discovery",
		"collibra/lineage",
		"collibra/asset-create",
		"collibra/asset-edit",
	}
	for _, name := range required {
		if cat.Get(name) == nil {
			t.Errorf("missing embedded skill: %s", name)
		}
	}
	if r := cat.Get("collibra/lineage").Resource("references/column-lineage-workaround.md"); r == nil {
		t.Error("missing column-lineage-workaround.md resource")
	}
	if r := cat.Get("collibra/asset-create").Resource("shared/rich-text-markdown.md"); r == nil {
		t.Error("missing shared/rich-text-markdown.md on asset-create")
	}
	if r := cat.Get("collibra/asset-edit").Resource("shared/rich-text-markdown.md"); r == nil {
		t.Error("missing shared/rich-text-markdown.md on asset-edit")
	}
}

func TestLoadWith_emptyDirReturnsEmbeddedOnly(t *testing.T) {
	withDir, err := LoadWith("")
	if err != nil {
		t.Fatalf("LoadWith(\"\"): %v", err)
	}
	embedded, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !slices.Equal(names(withDir.List()), names(embedded.List())) {
		t.Errorf("LoadWith(\"\") differs from Load()")
	}
}

func TestLoadWith_externalSkillIsAdded(t *testing.T) {
	dir := t.TempDir()
	writeSkill(t, dir, "acme/internal-policy", "---\ndescription: ACME policy.\n---\n\n# ACME body\n")

	cat, err := LoadWith(dir)
	if err != nil {
		t.Fatalf("LoadWith: %v", err)
	}
	added := cat.Get("acme/internal-policy")
	if added == nil {
		t.Fatal("expected external skill acme/internal-policy to be added")
	}
	if added.Description != "ACME policy." {
		t.Errorf("description = %q", added.Description)
	}
	if !strings.Contains(added.Body, "ACME body") {
		t.Errorf("body missing marker: %q", added.Body)
	}
	if cat.Get("collibra/index") == nil {
		t.Error("embedded skills should still be present after merge")
	}
}

func TestLoadWith_externalOverridesEmbedded(t *testing.T) {
	dir := t.TempDir()
	writeSkill(t, dir, "collibra/lineage", "---\ndescription: Overridden lineage.\nrelated: collibra/index\n---\n\n# Custom lineage body\n")
	writeFile(t, dir, "collibra/lineage/references/custom-note.md", "# Custom note")

	cat, err := LoadWith(dir)
	if err != nil {
		t.Fatalf("LoadWith: %v", err)
	}
	lineage := cat.Get("collibra/lineage")
	if lineage == nil {
		t.Fatal("expected collibra/lineage skill")
	}
	if lineage.Description != "Overridden lineage." {
		t.Errorf("description not overridden: %q", lineage.Description)
	}
	if !slices.Equal(lineage.Related, []string{"collibra/index"}) {
		t.Errorf("related not overridden: %v", lineage.Related)
	}
	if !strings.Contains(lineage.Body, "Custom lineage body") {
		t.Errorf("body not overridden: %q", lineage.Body)
	}
	if lineage.Resource("references/column-lineage-workaround.md") != nil {
		t.Error("embedded resource leaked into overridden skill")
	}
	if lineage.Resource("references/custom-note.md") == nil {
		t.Error("external resource missing on overridden skill")
	}
}

func TestLoadWith_missingDirIsError(t *testing.T) {
	_, err := LoadWith("/nope/does/not/exist/skills")
	if err == nil {
		t.Fatal("expected error for missing dir")
	}
	if !strings.Contains(err.Error(), "/nope/does/not/exist/skills") {
		t.Errorf("error should cite the path, got: %v", err)
	}
}

func TestMerge_orderStaysSorted(t *testing.T) {
	base := fstest.MapFS{
		"files/collibra/a/SKILL.md": &fstest.MapFile{Data: []byte("---\ndescription: A.\n---\n\nbody")},
		"files/collibra/c/SKILL.md": &fstest.MapFile{Data: []byte("---\ndescription: C.\n---\n\nbody")},
	}
	overlay := fstest.MapFS{
		"collibra/b/SKILL.md": &fstest.MapFile{Data: []byte("---\ndescription: B.\n---\n\nbody")},
		"collibra/a/SKILL.md": &fstest.MapFile{Data: []byte("---\ndescription: A2.\n---\n\nbody")},
	}
	cat, err := loadFromFS(base, "files")
	if err != nil {
		t.Fatalf("base load: %v", err)
	}
	ext, err := loadFromFS(overlay, ".")
	if err != nil {
		t.Fatalf("overlay load: %v", err)
	}
	cat.merge(ext)
	got := names(cat.List())
	want := []string{"collibra/a", "collibra/b", "collibra/c"}
	if !slices.Equal(got, want) {
		t.Errorf("order = %v, want %v", got, want)
	}
	if cat.Get("collibra/a").Description != "A2." {
		t.Errorf("override did not replace description, got %q", cat.Get("collibra/a").Description)
	}
}

func writeSkill(t *testing.T, dir, name, content string) {
	t.Helper()
	writeFile(t, dir, filepath.Join(name, "SKILL.md"), content)
}

func writeFile(t *testing.T, dir, rel, content string) {
	t.Helper()
	full := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", full, err)
	}
}

func names(skills []*Skill) []string {
	out := make([]string, len(skills))
	for i, s := range skills {
		out[i] = s.Name
	}
	return out
}
