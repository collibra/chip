package skills

import (
	"slices"
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

func names(skills []*Skill) []string {
	out := make([]string, len(skills))
	for i, s := range skills {
		out[i] = s.Name
	}
	return out
}
