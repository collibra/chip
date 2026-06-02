package skills

import (
	"context"
	"fmt"
	"strings"

	"github.com/collibra/chip/pkg/chip"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type loadInput struct {
	SkillName    string `json:"skillName" jsonschema:"Required. The skill identifier from list_collibra_skills (e.g. 'collibra/lineage')."`
	HeaderOnly   bool   `json:"headerOnly,omitempty" jsonschema:"If true, return name, description, related skills, and the list of bundled resource paths instead of the full body. Default false."`
	ResourcePath string `json:"resourcePath,omitempty" jsonschema:"Optional. Load a specific bundled resource file (e.g. 'references/column-lineage-workaround.md') instead of the main body. Use headerOnly to discover available paths."`
}

type loadOutput struct {
	Found       bool     `json:"found" jsonschema:"True if the skill (and resource, if requested) exists."`
	Name        string   `json:"name,omitempty" jsonschema:"The resolved skill name."`
	Description string   `json:"description,omitempty" jsonschema:"One-line summary."`
	Related     []string `json:"related,omitempty" jsonschema:"Related skill names worth loading together."`
	Resources   []string `json:"resources,omitempty" jsonschema:"Relative paths of bundled reference files on this skill."`
	Content     string   `json:"content,omitempty" jsonschema:"The Markdown body of the skill or the requested resource. Empty when headerOnly is true."`
	Error       string   `json:"error,omitempty" jsonschema:"Set when found is false. Explains why the skill or resource was not located."`
}

// NewLoadTool returns the load_collibra_skill tool wired to the given catalog.
func NewLoadTool(catalog *Catalog) *chip.Tool[loadInput, loadOutput] {
	return &chip.Tool[loadInput, loadOutput]{
		Name: loadToolName,
		Description: "Load a Collibra skill guide before using related tools (e.g. load " +
			"'collibra/lineage' before calling get_lineage_*, or 'collibra/asset-create' before " +
			"create_asset). Skills document the right tool sequences, ID-bridging rules, and " +
			"known limitations. If you do not already know the exact skill name from a prior " +
			"list_collibra_skills response, call list_collibra_skills first; do not guess names " +
			"from topic keywords. Set headerOnly=true to preview a skill's summary, related " +
			"skills, and bundled resources. Set resourcePath to load a specific bundled " +
			"reference; resourcePath takes precedence over headerOnly.",
		Handler:     loadHandler(catalog),
		Permissions: []string{},
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, IdempotentHint: true},
	}
}

func loadHandler(catalog *Catalog) chip.ToolHandlerFunc[loadInput, loadOutput] {
	return func(_ context.Context, input loadInput) (loadOutput, error) {
		skill := catalog.Get(input.SkillName)
		if skill == nil {
			return loadOutput{
				Error: fmt.Sprintf("unknown skill %q. Call list_collibra_skills to see available names.", input.SkillName),
			}, nil
		}
		if input.ResourcePath != "" {
			return loadResource(skill, input.ResourcePath), nil
		}
		return loadSkillContent(skill, input.HeaderOnly), nil
	}
}

func loadResource(skill *Skill, path string) loadOutput {
	resource := skill.Resource(path)
	if resource == nil {
		available := "none"
		if paths := skill.ResourcePaths(); len(paths) > 0 {
			available = strings.Join(paths, ", ")
		}
		return loadOutput{
			Error: fmt.Sprintf("skill %q has no resource %q. Available: %s",
				skill.Name, path, available),
		}
	}
	return loadOutput{
		Found:   true,
		Name:    skill.Name,
		Content: resource.Content,
	}
}

func loadSkillContent(skill *Skill, headerOnly bool) loadOutput {
	out := loadOutput{
		Found:       true,
		Name:        skill.Name,
		Description: skill.Description,
		Related:     skill.Related,
		Resources:   skill.ResourcePaths(),
	}
	if !headerOnly {
		out.Content = renderBody(skill)
	}
	return out
}

// renderBody appends a trailer with bundled resources and related skills.
// The body stays free of catalog metadata so authors only update one place.
func renderBody(skill *Skill) string {
	var b strings.Builder
	b.WriteString(skill.Body)
	if len(skill.Resources) > 0 {
		b.WriteString("\n\n---\nBundled resources (load via `load_collibra_skill` with `resourcePath`):\n")
		for _, r := range skill.Resources {
			fmt.Fprintf(&b, "- %s\n", r.Path)
		}
	}
	if len(skill.Related) > 0 {
		b.WriteString("\n\n---\nRelated skills: ")
		b.WriteString(strings.Join(skill.Related, ", "))
		b.WriteString("\n")
	}
	return b.String()
}
