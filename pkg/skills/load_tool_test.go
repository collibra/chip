package skills

import (
	"strings"
	"testing"
)

func TestLoadHandler_fullBodyAppendsTrailer(t *testing.T) {
	cat := newTestCatalog(t)
	out, err := loadHandler(cat)(t.Context(), loadInput{SkillName: "collibra/lineage"})
	if err != nil {
		t.Fatal(err)
	}
	if !out.Found {
		t.Fatal("expected found=true")
	}
	if !strings.Contains(out.Content, "body") {
		t.Errorf("content missing body: %q", out.Content)
	}
	if !strings.Contains(out.Content, "Bundled resources") {
		t.Error("content missing resources trailer")
	}
	if !strings.Contains(out.Content, "Related skills:") {
		t.Error("content missing related-skills trailer")
	}
}

func TestLoadHandler_headerOnlyOmitsBody(t *testing.T) {
	cat := newTestCatalog(t)
	out, err := loadHandler(cat)(t.Context(), loadInput{SkillName: "collibra/lineage", HeaderOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	if !out.Found {
		t.Fatal("expected found=true")
	}
	if out.Content != "" {
		t.Errorf("header-only should not include content, got %q", out.Content)
	}
	if out.Description == "" || len(out.Related) == 0 || len(out.Resources) == 0 {
		t.Errorf("header-only missing metadata: %+v", out)
	}
}

func TestLoadHandler_resourcePathReturnsResourceContent(t *testing.T) {
	cat := newTestCatalog(t)
	out, err := loadHandler(cat)(t.Context(), loadInput{
		SkillName:    "collibra/lineage",
		ResourcePath: "references/notes.md",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !out.Found {
		t.Fatal("expected found=true")
	}
	if out.Content != "notes" {
		t.Errorf("content = %q, want %q", out.Content, "notes")
	}
}

func TestLoadHandler_unknownSkillReturnsError(t *testing.T) {
	cat := newTestCatalog(t)
	out, err := loadHandler(cat)(t.Context(), loadInput{SkillName: "collibra/missing"})
	if err != nil {
		t.Fatal(err)
	}
	if out.Found {
		t.Fatal("expected found=false")
	}
	if out.Error == "" {
		t.Error("expected error message")
	}
}

func TestLoadHandler_unknownResourceListsAvailable(t *testing.T) {
	cat := newTestCatalog(t)
	out, err := loadHandler(cat)(t.Context(), loadInput{
		SkillName:    "collibra/lineage",
		ResourcePath: "references/nope.md",
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.Found {
		t.Fatal("expected found=false")
	}
	if !strings.Contains(out.Error, "references/notes.md") {
		t.Errorf("error should list available resources, got %q", out.Error)
	}
}

func TestLoadHandler_resourcePathOnSkillWithoutResources(t *testing.T) {
	cat := newTestCatalog(t)
	out, err := loadHandler(cat)(t.Context(), loadInput{
		SkillName:    "collibra/discovery",
		ResourcePath: "references/anything.md",
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.Found {
		t.Fatal("expected found=false")
	}
	if !strings.Contains(out.Error, "Available: none") {
		t.Errorf("error should say 'Available: none' when skill has no resources, got %q", out.Error)
	}
}

func TestLoadHandler_resourcePathTakesPrecedenceOverHeaderOnly(t *testing.T) {
	cat := newTestCatalog(t)
	out, err := loadHandler(cat)(t.Context(), loadInput{
		SkillName:    "collibra/lineage",
		ResourcePath: "references/notes.md",
		HeaderOnly:   true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !out.Found {
		t.Fatal("expected found=true")
	}
	if out.Content != "notes" {
		t.Errorf("resourcePath should win over headerOnly; content = %q, want %q", out.Content, "notes")
	}
}

func TestLoadHandler_skillWithoutRelatedOrResources(t *testing.T) {
	cat := newTestCatalog(t)
	out, err := loadHandler(cat)(t.Context(), loadInput{SkillName: "collibra/discovery"})
	if err != nil {
		t.Fatal(err)
	}
	if !out.Found {
		t.Fatal("expected found=true")
	}
	if strings.Contains(out.Content, "Bundled resources") {
		t.Error("should not include resources trailer when no resources exist")
	}
	if strings.Contains(out.Content, "Related skills:") {
		t.Error("should not include related-skills trailer when no related skills exist")
	}
}
