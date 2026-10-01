package skills

import (
	"fmt"

	"github.com/collibra/chip/pkg/chip"
)

// requirementsMet reports whether every capability named in a skill's
// `requires:` frontmatter is enabled on the running server. A skill with no
// `requires:` is always served. Resolving a name to a config field is
// chip.ServerToolConfig.CapabilityEnabled's job; this is the frontmatter
// loop around it.
//
// The gate is declared in the skill rather than by a list of skill names in
// Go for two reasons: a skill supplied through --skills-dir can gate itself,
// and renaming or moving a skill cannot silently un-gate it.
//
// An unrecognized capability name is an error, not a warning: a skill
// requiring something chip does not know about would otherwise be served
// unconditionally, which is the opposite of what its author asked for. The
// error fails catalog load and so aborts startup — unlike an unknown
// --experimental name, which only warns. That asymmetry is deliberate and is
// documented for operators in docs/CONFIG.md and for contributors in
// docs/TOOL_CONTRIBUTION_STANDARDS.md 3.5.
func requirementsMet(requires []string, toolConfig *chip.ServerToolConfig) (bool, error) {
	for _, name := range requires {
		enabled, err := toolConfig.CapabilityEnabled(name)
		if err != nil {
			return false, fmt.Errorf("requires: %w", err)
		}
		if !enabled {
			return false, nil
		}
	}
	return true, nil
}
