// Package create_assessment implements the create_assessment MCP tool: a write
// tool that starts a new assessment from a template. Creating an assessment
// only requires a template — the returned assessment carries the template's
// questions (unanswered). The caller then fills those answers in via the
// separate edit_assessment tool; this tool never sets answers itself.
package create_assessment

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
	"github.com/collibra/chip/pkg/tools/validation"
)

// Input is the tool's typed input.
type Input struct {
	Template            string          `json:"template" jsonschema:"Required. The assessment template to create from — either its name (e.g. 'Business Context', resolved to the latest version) or its UUID."`
	Name                string          `json:"name,omitempty" jsonschema:"Optional. Name for the new assessment."`
	AssetID             string          `json:"assetId,omitempty" jsonschema:"Optional. UUID of the asset to conduct the assessment on."`
	Assignees           []InputAssignee `json:"assignees,omitempty" jsonschema:"Optional. Users or groups assigned to the assessment."`
	OwnerID             string          `json:"ownerId,omitempty" jsonschema:"Optional. UUID of the assessment owner (a user)."`
	IsVisibleToEveryone *bool           `json:"isVisibleToEveryone,omitempty" jsonschema:"Optional. When true, the assessment is visible to everyone."`
	Status              string          `json:"status,omitempty" jsonschema:"Optional. Initial status: DRAFT, SUBMITTED, or OBSOLETE. Defaults to the API default (DRAFT) when omitted."`
}

// InputAssignee is one user or group assigned to the assessment.
type InputAssignee struct {
	ID   string `json:"id" jsonschema:"UUID of the user or group."`
	Type string `json:"type" jsonschema:"USER or GROUP."`
}

// Output is the typed response.
type Output struct {
	Assessment *AssessmentSummary `json:"assessment,omitempty" jsonschema:"the created assessment, on success. Its questions are unanswered — fill them in with edit_assessment."`
	Error      string             `json:"error,omitempty" jsonschema:"error message when creation failed."`
}

// AssessmentSummary is the post-create snapshot of the assessment.
type AssessmentSummary struct {
	ID        string      `json:"id" jsonschema:"UUID of the created assessment."`
	Name      string      `json:"name,omitempty"`
	Status    string      `json:"status,omitempty"`
	Template  *RefSummary `json:"template,omitempty" jsonschema:"the template the assessment was created from."`
	Questions []Question  `json:"questions,omitempty" jsonschema:"the assessment's questions, currently unanswered. Answer these via edit_assessment using each question's id."`
}

// RefSummary is the {id, name} reference shape.
type RefSummary struct {
	ID   string `json:"id"`
	Name string `json:"name,omitempty"`
}

// Question is one question the assessment expects an answer for.
type Question struct {
	ID          string `json:"id" jsonschema:"question id — pass this to edit_assessment to set the answer."`
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
}

// NewTool returns the registered tool.
func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:  "create_assessment",
		Title: "Create Assessment",
		Description: "Create a new assessment from an assessment template. " +
			"Creating only needs a template — give its name (resolved to the latest version) or its UUID; optionally attach an asset, assignees, an owner, visibility, and an initial status. " +
			"This tool does NOT set answers — the created assessment comes back with the template's questions unanswered. " +
			"Use the returned question ids with edit_assessment to fill in the answers afterwards.",
		Handler:     handler(collibraClient),
		Permissions: []string{},
	}
}

func handler(collibraClient *http.Client) chip.ToolHandlerFunc[Input, Output] {
	return func(ctx context.Context, input Input) (Output, error) {
		templateID, err := resolveTemplateID(ctx, collibraClient, input.Template)
		if err != nil {
			return Output{}, err
		}
		if err := validation.UUIDOptional("assetId", input.AssetID); err != nil {
			return Output{}, err
		}
		if err := validation.UUIDOptional("ownerId", input.OwnerID); err != nil {
			return Output{}, err
		}
		for i, a := range input.Assignees {
			if err := validation.UUID(fmt.Sprintf("assignees[%d].id", i), a.ID); err != nil {
				return Output{}, err
			}
			switch strings.ToUpper(strings.TrimSpace(a.Type)) {
			case "USER", "GROUP":
			default:
				return Output{}, fmt.Errorf("assignees[%d].type must be USER or GROUP, got %q", i, a.Type)
			}
		}

		req := clients.CreateAssessmentRequest{
			Template: clients.AssessmentRef{ID: templateID},
			Name:     input.Name,
		}
		if input.AssetID != "" {
			req.Asset = &clients.AssessmentRef{ID: input.AssetID}
		}
		if input.OwnerID != "" {
			req.Owner = &clients.AssessmentRef{ID: input.OwnerID}
		}
		if len(input.Assignees) > 0 {
			req.Assignees = make([]clients.Assignee, len(input.Assignees))
			for i, a := range input.Assignees {
				req.Assignees[i] = clients.Assignee{ID: a.ID, Type: strings.ToUpper(strings.TrimSpace(a.Type))}
			}
		}
		req.IsVisibleToEveryone = input.IsVisibleToEveryone
		if s := strings.TrimSpace(input.Status); s != "" {
			req.Status = chip.Ptr(strings.ToUpper(s))
		}

		assessment, err := clients.CreateAssessment(ctx, collibraClient, req)
		if err != nil {
			return Output{Error: fmt.Sprintf("Could not create assessment: %v", err)}, nil
		}

		return Output{Assessment: summarise(assessment)}, nil
	}
}

// resolveTemplateID turns the caller's template input into a template UUID.
// A UUID is used as-is; a name is resolved against the latest version of each
// template (case-insensitive), preferring an exact name match and returning a
// self-correcting error when nothing matches or the name is ambiguous.
func resolveTemplateID(ctx context.Context, client *http.Client, template string) (string, error) {
	t := strings.TrimSpace(template)
	if t == "" {
		return "", fmt.Errorf("template is required (its name or UUID)")
	}
	if validation.UUID("template", t) == nil {
		return t, nil
	}

	latest := true
	page, err := clients.ListTemplates(ctx, client, clients.ListTemplatesParams{
		Name:              t,
		LatestVersionOnly: &latest,
		Limit:             50,
	})
	if err != nil {
		return "", fmt.Errorf("looking up template %q: %w", t, err)
	}

	// Prefer exact (case-insensitive) name matches; fall back to the API's
	// contains-matches. Dedupe by template id.
	byID := map[string]string{}
	for _, tmpl := range page.Results {
		if strings.EqualFold(strings.TrimSpace(tmpl.Name), t) {
			byID[tmpl.ID] = tmpl.Name
		}
	}
	if len(byID) == 0 {
		for _, tmpl := range page.Results {
			byID[tmpl.ID] = tmpl.Name
		}
	}

	switch len(byID) {
	case 0:
		return "", fmt.Errorf("no assessment template found matching %q", template)
	case 1:
		for id := range byID {
			return id, nil
		}
	}
	names := make([]string, 0, len(byID))
	for _, n := range byID {
		names = append(names, n)
	}
	sort.Strings(names)
	return "", fmt.Errorf("template %q is ambiguous; matched: %s — specify the exact name or the template UUID", template, strings.Join(names, ", "))
}

func summarise(a *clients.Assessment) *AssessmentSummary {
	if a == nil {
		return nil
	}
	s := &AssessmentSummary{ID: a.ID, Name: a.Name, Status: a.Status}
	if a.Template != nil {
		s.Template = &RefSummary{ID: a.Template.ID, Name: a.Template.Name}
	}
	for _, q := range a.Content {
		s.Questions = append(s.Questions, Question{ID: q.ID, Name: q.Name, Description: q.Description})
	}
	return s
}
