package skills

import (
	"regexp"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/collibra/chip/pkg/chip"
)

// dataQualitySkills are the embedded skills that declare
// `requires: data-quality`; they document tools that are only registered when
// the data quality capability is on.
var dataQualitySkills = []string{"collibra/dq-rule-workbench", "collibra/dq-rules"}

// capabilityFreeSkills are the embedded skills that require no capability and
// are therefore served in every configuration.
var capabilityFreeSkills = []string{
	"collibra/asset-create",
	"collibra/asset-edit",
	"collibra/context",
	"collibra/data-product-create",
	"collibra/discovery",
	"collibra/index",
	"collibra/lineage",
}

func TestEmbeddedCatalog_dataQualitySkillsHiddenWhenCapabilityOff(t *testing.T) {
	cat, err := Load(&chip.ServerToolConfig{})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	for _, name := range dataQualitySkills {
		if cat.Get(name) != nil {
			t.Errorf("%s should not be served with DataQuality=false", name)
		}
	}
	for _, name := range capabilityFreeSkills {
		if cat.Get(name) == nil {
			t.Errorf("%s should be served regardless of DataQuality", name)
		}
	}
	// Assert the surface in both directions, so a new skill cannot slip in
	// (or out) unnoticed.
	if got := names(cat.List()); !slices.Equal(got, capabilityFreeSkills) {
		t.Errorf("catalog with DataQuality=false = %v, want %v", got, capabilityFreeSkills)
	}
}

func TestEmbeddedCatalog_dataQualitySkillsServedWhenCapabilityOn(t *testing.T) {
	cat, err := Load(&chip.ServerToolConfig{DataQuality: true})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := append(append([]string{}, capabilityFreeSkills...), dataQualitySkills...)
	slices.Sort(want)
	for _, name := range want {
		if cat.Get(name) == nil {
			t.Errorf("%s should be served with DataQuality=true", name)
		}
	}
	if got := names(cat.List()); !slices.Equal(got, want) {
		t.Errorf("catalog with DataQuality=true = %v, want %v", got, want)
	}
}

func TestLoadFromFS_unknownRequiresValueFailsLoad(t *testing.T) {
	fsys := fstest.MapFS{
		"files/collibra/exotic/SKILL.md": &fstest.MapFile{Data: []byte(
			"---\ndescription: Exotic.\nrequires: warp-drive\n---\n\nbody")},
	}
	_, err := loadFromFS(fsys, "files", &chip.ServerToolConfig{DataQuality: true})
	if err == nil {
		t.Fatal("expected catalog load to fail on an unknown requires value")
	}
	if !strings.Contains(err.Error(), "warp-drive") {
		t.Errorf("error should name the offending value, got: %v", err)
	}
	if !strings.Contains(err.Error(), "collibra/exotic") {
		t.Errorf("error should name the skill, got: %v", err)
	}
}

// An externally supplied skill gates itself on the same capability names, so
// a --skills-dir skill documenting the data quality tools disappears with
// them.
func TestLoadWith_externalSkillGatesItselfOnCapability(t *testing.T) {
	dir := t.TempDir()
	writeSkill(t, dir, "acme/dq-house-rules", "---\ndescription: ACME DQ rules.\nrequires: data-quality\n---\n\n# ACME\n")

	off, err := LoadWith(dir, &chip.ServerToolConfig{})
	if err != nil {
		t.Fatalf("LoadWith (capability off): %v", err)
	}
	if off.Get("acme/dq-house-rules") != nil {
		t.Error("external skill requiring data-quality should not be served with the capability off")
	}

	on, err := LoadWith(dir, &chip.ServerToolConfig{DataQuality: true})
	if err != nil {
		t.Fatalf("LoadWith (capability on): %v", err)
	}
	if on.Get("acme/dq-house-rules") == nil {
		t.Error("external skill requiring data-quality should be served with the capability on")
	}
}

func TestLoadWith_externalUnknownRequiresValueFailsLoad(t *testing.T) {
	dir := t.TempDir()
	writeSkill(t, dir, "acme/mystery", "---\ndescription: Mystery.\nrequires: teleportation\n---\n\n# ACME\n")

	_, err := LoadWith(dir, &chip.ServerToolConfig{DataQuality: true})
	if err == nil {
		t.Fatal("expected catalog load to fail on an unknown requires value in an external skill")
	}
	if !strings.Contains(err.Error(), "teleportation") {
		t.Errorf("error should name the offending value, got: %v", err)
	}
}

var skillRefPattern = regexp.MustCompile(`collibra/[a-z0-9]+(?:-[a-z0-9]+)*`)

// TestEmbeddedCatalog_crossReferencesResolveInEveryState is what catches a
// served skill — collibra/index above all — advertising a skill that the
// running configuration filtered out. Filtering the catalog does not rewrite
// markdown, so every `collibra/*` reference in a served skill's body or
// `related:` header must resolve to a skill that is also served.
func TestEmbeddedCatalog_crossReferencesResolveInEveryState(t *testing.T) {
	for _, dataQuality := range []bool{false, true} {
		cat, err := Load(&chip.ServerToolConfig{DataQuality: dataQuality})
		if err != nil {
			t.Fatalf("Load (DataQuality=%v): %v", dataQuality, err)
		}
		for _, skill := range cat.List() {
			refs := append([]string{}, skill.Related...)
			refs = append(refs, skillRefPattern.FindAllString(skill.Body, -1)...)
			for _, ref := range refs {
				if ref == skill.Name {
					continue
				}
				if cat.Get(ref) == nil {
					t.Errorf("DataQuality=%v: skill %q references %q, which is not served",
						dataQuality, skill.Name, ref)
				}
			}
		}
	}
}
