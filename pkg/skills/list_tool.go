package skills

import (
	"context"

	"github.com/collibra/chip/pkg/chip"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type listInput struct {
	Query         string `json:"query,omitempty" jsonschema:"Optional case-insensitive substring filter across skill name and description."`
	IncludeHeader bool   `json:"includeHeader,omitempty" jsonschema:"If true, include the description, related skills, and bundled resource paths alongside each skill name. Default false."`
}

type listOutput struct {
	Skills []listedSkill `json:"skills" jsonschema:"Matching skills in catalog order."`
}

type listedSkill struct {
	Name        string   `json:"name" jsonschema:"The skill identifier passed to load_collibra_skill."`
	Description string   `json:"description,omitempty" jsonschema:"One-line summary. Present when includeHeader is true."`
	Related     []string `json:"related,omitempty" jsonschema:"Related skill names worth loading together. Present when includeHeader is true."`
	Resources   []string `json:"resources,omitempty" jsonschema:"Relative paths of bundled reference files. Present when includeHeader is true."`
}

// NewListTool returns the list_collibra_skills tool wired to the given catalog.
func NewListTool(catalog *Catalog) *chip.Tool[listInput, listOutput] {
	return &chip.Tool[listInput, listOutput]{
		Name: listToolName,
		Description: "List available Collibra skill guides. Skills document multi-step workflows, " +
			"ID-bridging rules, and required permissions for chip's tools. Call this before " +
			"load_collibra_skill when you do not already know the exact skill name; skill names " +
			"are not predictable from topic words. Use query to filter by substring. Set " +
			"includeHeader=true to see one-line summaries, related skills, and bundled resource " +
			"paths alongside names. Load skills proactively when starting work in a relevant " +
			"Collibra domain, not after errors. Start with `collibra/index` if unsure.",
		Handler:     listHandler(catalog),
		Permissions: []string{},
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, IdempotentHint: true},
	}
}

func listHandler(catalog *Catalog) chip.ToolHandlerFunc[listInput, listOutput] {
	return func(_ context.Context, input listInput) (listOutput, error) {
		matches := catalog.Search(input.Query)
		out := listOutput{Skills: make([]listedSkill, 0, len(matches))}
		for _, s := range matches {
			entry := listedSkill{Name: s.Name}
			if input.IncludeHeader {
				entry.Description = s.Description
				entry.Related = s.Related
				entry.Resources = s.ResourcePaths()
			}
			out.Skills = append(out.Skills, entry)
		}
		return out, nil
	}
}
