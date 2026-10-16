package main

import (
	"strings"
	"testing"

	"github.com/collibra/chip/pkg/chip"
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

// data-quality is a capability flag (--data-quality), not an experimental
// feature: it must not be accepted as an --experimental name, or a stale
// config would appear to enable the data quality tools without doing so.
func TestDataQualityIsNotAnExperimentalFeature(t *testing.T) {
	if _, ok := knownExperimentalFeatures[chip.DataQualityCapabilityName]; ok {
		t.Errorf("%q must not be a known experimental feature", chip.DataQualityCapabilityName)
	}
	if strings.Contains(knownExperimentalFeaturesList(), chip.DataQualityCapabilityName) {
		t.Errorf("known features list should not mention %q: %s",
			chip.DataQualityCapabilityName, knownExperimentalFeaturesList())
	}
}
