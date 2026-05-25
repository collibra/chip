package skills

import (
	"strings"
	"testing"
	"testing/fstest"
)

func newTestCatalog(t *testing.T) *Catalog {
	t.Helper()
	fsys := fstest.MapFS{
		"files/collibra/lineage/SKILL.md": &fstest.MapFile{Data: []byte(
			`---
description: Trace lineage.
related: collibra/discovery
---

body`)},
		"files/collibra/lineage/references/notes.md": &fstest.MapFile{Data: []byte("notes")},
		"files/collibra/discovery/SKILL.md":          &fstest.MapFile{Data: []byte("---\ndescription: Find assets.\n---\n\nbody")},
	}
	cat, err := loadFromFS(fsys, "files")
	if err != nil {
		t.Fatal(err)
	}
	return cat
}

func TestListHandler_namesOnlyByDefault(t *testing.T) {
	cat := newTestCatalog(t)
	out, err := listHandler(cat)(t.Context(), listInput{})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Skills) != 2 {
		t.Fatalf("expected 2 skills, got %d", len(out.Skills))
	}
	for _, s := range out.Skills {
		if s.Description != "" || s.Related != nil || s.Resources != nil {
			t.Errorf("expected name-only entry, got %+v", s)
		}
	}
}

func TestListHandler_includeHeader(t *testing.T) {
	cat := newTestCatalog(t)
	out, err := listHandler(cat)(t.Context(), listInput{IncludeHeader: true})
	if err != nil {
		t.Fatal(err)
	}
	var lineage listedSkill
	for _, s := range out.Skills {
		if s.Name == "collibra/lineage" {
			lineage = s
			break
		}
	}
	if lineage.Description != "Trace lineage." {
		t.Errorf("description = %q", lineage.Description)
	}
	if len(lineage.Related) != 1 || lineage.Related[0] != "collibra/discovery" {
		t.Errorf("related = %v", lineage.Related)
	}
	if len(lineage.Resources) != 1 || lineage.Resources[0] != "references/notes.md" {
		t.Errorf("resources = %v", lineage.Resources)
	}
}

func TestListHandler_filtersByQuery(t *testing.T) {
	cat := newTestCatalog(t)
	out, err := listHandler(cat)(t.Context(), listInput{Query: "lineage"})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Skills) != 1 || out.Skills[0].Name != "collibra/lineage" {
		names := make([]string, len(out.Skills))
		for i, s := range out.Skills {
			names[i] = s.Name
		}
		t.Errorf("query filter returned %s", strings.Join(names, ","))
	}
}
