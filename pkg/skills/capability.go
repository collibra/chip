package skills

import (
	"fmt"

	"github.com/collibra/chip/pkg/chip"
)

// requirementsMet reports whether every capability named in a skill's
// `requires:` frontmatter is enabled on the running server. A skill with no
// `requires:` is always served.
//
// The gate is declared in the skill rather than by a list of skill names in
// Go for two reasons: a skill supplied through --skills-dir can gate itself,
// and renaming or moving a skill cannot silently un-gate it.
//
// An unrecognized capability name is an error, not a warning: a skill
// requiring something chip does not know about would otherwise be served
// unconditionally, which is the opposite of what its author asked for.
func requirementsMet(requires []string, toolConfig *chip.ServerToolConfig) (bool, error) {
	for _, name := range requires {
		enabled, err := capabilityEnabled(name, toolConfig)
		if err != nil {
			return false, err
		}
		if !enabled {
			return false, nil
		}
	}
	return true, nil
}

// capabilityEnabled resolves one `requires:` value against the server
// configuration. data-quality is the only capability a skill can require
// today; add a case here when a second capability flag appears.
func capabilityEnabled(name string, toolConfig *chip.ServerToolConfig) (bool, error) {
	switch name {
	case chip.DataQualityCapabilityName:
		return toolConfig.DataQuality, nil
	default:
		return false, fmt.Errorf(
			"unknown requires value %q in skill frontmatter (known capabilities: %s)",
			name, chip.DataQualityCapabilityName,
		)
	}
}
