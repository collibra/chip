package get_assessment

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
	"github.com/collibra/chip/pkg/tools/validation"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Input has two modes. A DIRECT lookup (assessmentId or assessmentReviewId)
// fetches a single assessment and can't be combined with anything else. A
// FILTERED lookup uses any combination of the remaining fields and returns a
// list (paginated). With no fields set it lists the assessments you can see.
type Input struct {
	// Direct lookups (single result; mutually exclusive; not combinable with filters).
	AssessmentID       string `json:"assessmentId,omitempty" jsonschema:"Direct lookup: the assessment's own UUID (e.g. the id from an /assessments/conduct?id= URL). Fetches exactly one; cannot be combined with other fields."`
	AssessmentReviewID string `json:"assessmentReviewId,omitempty" jsonschema:"Direct lookup: the UUID of the Assessment Review catalog asset linked to the assessment. Fetches exactly one; cannot be combined with other fields."`

	// Filters (combinable; return a list).
	Name             string `json:"name,omitempty" jsonschema:"Filter: case-insensitive, matches assessments whose name contains this text (partial names work)."`
	Status           string `json:"status,omitempty" jsonschema:"Filter: DRAFT, SUBMITTED, or OBSOLETE."`
	TemplateID       string `json:"templateId,omitempty" jsonschema:"Filter: UUID of the template the assessment was created from."`
	TemplateVersion  string `json:"templateVersion,omitempty" jsonschema:"Filter: template version to match — an exact version or 'LATEST'. Requires templateId."`
	AssetID          string `json:"assetId,omitempty" jsonschema:"Filter: UUID of the asset the assessment was conducted on."`
	LastModifiedFrom string `json:"lastModifiedFrom,omitempty" jsonschema:"Filter: ISO-8601 timestamp; include assessments last modified at or after this time (e.g. 2026-07-01T00:00:00.000Z)."`
	LastModifiedTo   string `json:"lastModifiedTo,omitempty" jsonschema:"Filter: ISO-8601 timestamp; include assessments last modified before this time."`
	Limit            int    `json:"limit,omitempty" jsonschema:"Filter: max results to return (1-50, default 10)."`
	Cursor           string `json:"cursor,omitempty" jsonschema:"Filter: pagination cursor from a previous response's nextCursor."`
}

type Output struct {
	// Assessment is set for a direct lookup, or a filtered lookup with exactly one match.
	Assessment *clients.Assessment `json:"assessment,omitempty" jsonschema:"the resolved assessment when a direct lookup is used or a filtered lookup matches exactly one. Includes id, name, status, template, content (each question's id, name, description, current answer {type,value} and comments), asset, assessmentReview, assignees, owner, isVisibleToEveryone and timestamps."`
	// Assessments is set for a filtered lookup that matches more than one.
	Assessments []clients.Assessment `json:"assessments,omitempty" jsonschema:"the matching assessments when a filtered lookup returns more than one. Each has the same shape as the assessment field."`
	NextCursor  string               `json:"nextCursor,omitempty" jsonschema:"cursor for the next page of a filtered lookup, if more results exist; pass it back as 'cursor'."`
	Error       string               `json:"error,omitempty" jsonschema:"error message if no assessment was found or an API error occurred"`
	Found       bool                 `json:"found" jsonschema:"whether at least one assessment was found"`
}

func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:  "get_assessment",
		Title: "Get Assessment",
		Description: "Retrieve conducted assessment(s) so their questions, answers, and fields can be read. Assessments are conducted objects (from /assessments/conduct), NOT catalog assets — use this instead of get_asset_details for them. " +
			"Two modes: (1) a direct lookup of a single assessment via assessmentId (its own UUID) or assessmentReviewId (the linked Assessment Review asset UUID); or (2) a filtered lookup combining any of name (partial, case-insensitive), status (DRAFT/SUBMITTED/OBSOLETE), templateId (+templateVersion), assetId, and a lastModifiedFrom/lastModifiedTo range, with limit/cursor for paging. " +
			"A direct id can't be combined with filters. With no fields at all it lists the assessments you can see. Returns each assessment's status, template, full question/answer content, assignees, owner, visibility, and timestamps.",
		Handler:     handler(collibraClient),
		Permissions: []string{},
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, DestructiveHint: chip.Ptr(false), IdempotentHint: true, OpenWorldHint: chip.Ptr(false)},
	}
}

func handler(collibraClient *http.Client) chip.ToolHandlerFunc[Input, Output] {
	return func(ctx context.Context, input Input) (Output, error) {
		// Direct lookups are exclusive and can't be combined with filters.
		direct := 0
		if input.AssessmentID != "" {
			direct++
		}
		if input.AssessmentReviewID != "" {
			direct++
		}
		if direct > 1 {
			return Output{}, fmt.Errorf("provide only one of assessmentId or assessmentReviewId")
		}
		if direct == 1 && hasFilters(input) {
			return Output{}, fmt.Errorf("assessmentId/assessmentReviewId fetch a single assessment and cannot be combined with filters (name, status, template, assetId, dates, limit, cursor)")
		}

		// Validate the UUID-valued fields that are present.
		for _, f := range [][2]string{
			{"assessmentId", input.AssessmentID},
			{"assessmentReviewId", input.AssessmentReviewID},
			{"templateId", input.TemplateID},
			{"assetId", input.AssetID},
		} {
			if err := validation.UUIDOptional(f[0], f[1]); err != nil {
				return Output{}, err
			}
		}

		// Direct fetches.
		if input.AssessmentID != "" {
			assessment, err := clients.GetAssessment(ctx, collibraClient, input.AssessmentID)
			if err != nil {
				return notFound(err), nil
			}
			return Output{Assessment: assessment, Found: true}, nil
		}
		if input.AssessmentReviewID != "" {
			assessment, err := clients.GetAssessmentByReview(ctx, collibraClient, input.AssessmentReviewID)
			if err != nil {
				return notFound(err), nil
			}
			return Output{Assessment: assessment, Found: true}, nil
		}

		// Filtered lookup.
		if input.TemplateVersion != "" && input.TemplateID == "" {
			return Output{}, fmt.Errorf("templateVersion requires templateId")
		}
		status := ""
		if input.Status != "" {
			status = strings.ToUpper(strings.TrimSpace(input.Status))
			if status != "DRAFT" && status != "SUBMITTED" && status != "OBSOLETE" {
				return Output{}, fmt.Errorf("status must be DRAFT, SUBMITTED, or OBSOLETE; got %q", input.Status)
			}
		}

		paged, err := clients.ListAssessments(ctx, collibraClient, clients.ListAssessmentsParams{
			Name:             input.Name,
			Status:           status,
			TemplateID:       input.TemplateID,
			TemplateVersion:  input.TemplateVersion,
			AssetID:          input.AssetID,
			LastModifiedFrom: input.LastModifiedFrom,
			LastModifiedTo:   input.LastModifiedTo,
			Limit:            input.Limit,
			Cursor:           input.Cursor,
		})
		if err != nil {
			return notFound(err), nil
		}
		out := fromList(paged.Results, "No assessments matched the given filters")
		out.NextCursor = paged.NextCursor
		return out, nil
	}
}

// hasFilters reports whether any filter field is set.
func hasFilters(in Input) bool {
	return in.Name != "" || in.Status != "" || in.TemplateID != "" || in.TemplateVersion != "" ||
		in.AssetID != "" || in.LastModifiedFrom != "" || in.LastModifiedTo != "" || in.Limit != 0 || in.Cursor != ""
}

// fromList shapes a list lookup: one match returns as the single Assessment,
// several as Assessments, none as a not-found result.
func fromList(results []clients.Assessment, emptyMsg string) Output {
	switch len(results) {
	case 0:
		return Output{Error: emptyMsg, Found: false}
	case 1:
		return Output{Assessment: &results[0], Found: true}
	default:
		return Output{Assessments: results, Found: true}
	}
}

func notFound(err error) Output {
	return Output{Error: fmt.Sprintf("Failed to retrieve assessment: %s", err.Error()), Found: false}
}
