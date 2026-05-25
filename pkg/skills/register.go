package skills

import (
	"github.com/collibra/chip/pkg/chip"
)

// FeatureName is the experimental-feature identifier used to gate skills.
const FeatureName = "skills"

// Tool names exposed by this package. Declared as constants so the gate in
// Enabled and the registrations in RegisterAll cannot drift.
const (
	listToolName = "list_collibra_skills"
	loadToolName = "load_collibra_skill"
)

// Instructions is the full initialize-instructions text shipped when the
// skills feature is enabled. It replaces the default navigator
// (pkg/chip/instructions.md) instead of appending to it: with skills on,
// the workflow recipes, tool categories, and key patterns the default
// navigator carries also live inside the skill bodies, so eagerly shipping
// them duplicates context and competes with the "discover skills first"
// directive. The text mirrors Datadog's pattern — orientation, the
// existence-of-skills claim, the discovery protocol, and the fall-back
// exception — and deliberately omits per-tool recipes.
const Instructions = `# Collibra MCP Server

This server connects you to Collibra, an enterprise data governance platform — the authoritative catalog for what data the organization has, what it means, how it relates, and who governs it. Reach for these tools when the user asks about **discovering, understanding, or governing data**: "what customer data do we have?", "what does this metric measure?", "which columns hold PII?", "where does this KPI come from?". These tools operate on metadata and governance, not on the underlying row-level data.

## Key concepts

- **Assets**: any item in the catalog — its ` + "`assetType`" + ` (Table, Column, Business Term, KPI, Report, Policy, …) determines what it represents.
- **Business Terms**: standardized definitions for business concepts.
- **Data Contracts**: agreements defining data product interfaces.
- **Classifications**: data classes applied to assets to categorize them — most commonly for data sensitivity (PII, PHI), but also for arbitrary taxonomies.

## Skills

This server ships skill guides — short Markdown docs that document the right tool sequences, ID-bridging rules (e.g. lineage entity IDs are not DGC asset UUIDs), permission requirements, and known limitations (e.g. column-level lineage) for each Collibra domain (discovery, lineage, asset create/edit, data products, classification). Skill content is not visible in tool names or schemas.

Before composing a multi-step workflow, run skill discovery: call ` + "`list_collibra_skills`" + ` (optionally with a ` + "`query`" + ` matching the user's intent) and load the matching ` + "`collibra/*`" + ` skill via ` + "`load_collibra_skill`" + `. Start with ` + "`collibra/index`" + ` if unsure which skill applies. Load related skills the index or a loaded skill points to when they apply.

Exceptions: a single tool call whose intent is obvious from the tool's own description does not need a skill. A follow-up call in a domain whose skill you have already loaded this session does not need another discovery round. If no skill clearly matches, proceed with the tool descriptions.

When a tool returns a permission error, surface to the user which scope is needed (e.g. ` + "`dgc.ai-copilot`" + `, ` + "`dgc.classify`" + ` + ` + "`dgc.catalog`" + `).`

// Enabled reports whether the skills feature is active for the given
// config. Both tool names must be enabled; the bootstrap snippet describes
// a workflow that needs both list_collibra_skills and load_collibra_skill,
// so disabling either takes the feature down as a unit. This is the
// canonical gate — callers use it for both the instructions snippet and
// tool registration so they cannot diverge.
func Enabled(toolConfig *chip.ServerToolConfig) bool {
	if !toolConfig.IsExperimentalEnabled(FeatureName) {
		return false
	}
	return toolConfig.IsToolEnabled(listToolName) && toolConfig.IsToolEnabled(loadToolName)
}

// RegisterAll loads the embedded catalog and registers the two skill tools.
// Callers must check Enabled first; RegisterAll does not re-gate.
func RegisterAll(server *chip.Server) error {
	catalog, err := Load()
	if err != nil {
		return err
	}
	chip.RegisterTool(server, NewListTool(catalog))
	chip.RegisterTool(server, NewLoadTool(catalog))
	return nil
}
