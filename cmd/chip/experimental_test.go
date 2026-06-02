package main

import (
	"strings"
	"testing"

	"github.com/collibra/chip/pkg/skills"
)

func TestKnownExperimentalFeaturesListIsStable(t *testing.T) {
	got := knownExperimentalFeaturesList()
	if !strings.Contains(got, skills.FeatureName) {
		t.Errorf("known features list missing %q: %s", skills.FeatureName, got)
	}
}

func TestFormatExperimentalForHelp_includesAllKnown(t *testing.T) {
	help := formatExperimentalForHelp()
	for name, desc := range knownExperimentalFeatures {
		if !strings.Contains(help, name) {
			t.Errorf("help text missing feature name %q", name)
		}
		if !strings.Contains(help, desc) {
			t.Errorf("help text missing description for %q", name)
		}
	}
}
