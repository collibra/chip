// Package find_relation_type looks up a single relation type from the
// cached DGC catalog. Lookup keys are stackable (AND): publicId, role,
// and either/both endpoint asset type publicIds. Returns a
// resolve_domain-style envelope (match | candidates | notFound).
//
// Typical patterns:
//
//   - by stable identifier:        publicId="..."
//   - by role alone:               role="governs"  (likely yields candidates)
//   - by role + endpoint shape:    role="governs", headTypePublicId="ManagedControl",
//                                  tailTypePublicId="Asset"
package find_relation_type

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type Input struct {
	PublicID           string `json:"publicId,omitempty" jsonschema:"Optional. Relation type publicId. Exact match."`
	Role               string `json:"role,omitempty" jsonschema:"Optional. The name of the role that the source plays in the relation, e.g. 'governs'. Exact match (case-sensitive)."`
	HeadTypePublicID   string `json:"headTypePublicId,omitempty" jsonschema:"Optional. publicId of the source (head) asset type. Exact match. Use with role to disambiguate the same role across multiple endpoint pairs."`
	TailTypePublicID   string `json:"tailTypePublicId,omitempty" jsonschema:"Optional. publicId of the target (tail) asset type. Exact match."`
}

type Output struct {
	Match      *RelationType  `json:"match,omitempty" jsonschema:"Set when exactly one relation type matches"`
	Candidates []RelationType `json:"candidates,omitempty" jsonschema:"Set when multiple relation types match the criteria; the caller must pick one"`
	NotFound   bool           `json:"notFound,omitempty" jsonschema:"True when no relation type matches"`
	Reason     string         `json:"reason,omitempty" jsonschema:"Explanation when match is empty (notFound or multi-match)"`
}

type RelationType struct {
	ID          string         `json:"id" jsonschema:"The unique identifier of the relation type"`
	PublicID    string         `json:"publicId,omitempty" jsonschema:"The public id of the relation type"`
	Description string         `json:"description,omitempty" jsonschema:"The description of the relation type"`
	Role        string         `json:"role,omitempty" jsonschema:"The name of the role that the source plays in the relation"`
	CoRole      string         `json:"coRole,omitempty" jsonschema:"The name of the role that the target plays in the relation"`
	System      bool           `json:"system" jsonschema:"Whether this is a system relation type"`
	SourceType  *TypeReference `json:"sourceType,omitempty" jsonschema:"Source asset type reference"`
	TargetType  *TypeReference `json:"targetType,omitempty" jsonschema:"Target asset type reference"`
}

type TypeReference struct {
	ID       string `json:"id" jsonschema:"Asset type UUID"`
	Name     string `json:"name,omitempty" jsonschema:"Asset type display name"`
	PublicID string `json:"publicId,omitempty" jsonschema:"Asset type publicId"`
}

func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:        "find_relation_type",
		Description: "Find a single relation type by publicId, by role alone, or by role + endpoint asset type publicIds. Returns match / candidates / notFound. Reads from the same one-hour cache as list_relation_types.",
		Handler:     handler(collibraClient),
		Permissions: []string{},
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}
}

func handler(collibraClient *http.Client) chip.ToolHandlerFunc[Input, Output] {
	return func(ctx context.Context, input Input) (Output, error) {
		if input.PublicID == "" && input.Role == "" && input.HeadTypePublicID == "" && input.TailTypePublicID == "" {
			return Output{}, errors.New("at least one of publicId, role, headTypePublicId, or tailTypePublicId must be provided")
		}
		all, err := clients.ListAllRelationTypes(ctx, collibraClient)
		if err != nil {
			return Output{}, err
		}
		var matches []RelationType
		for _, rt := range all {
			if !relationTypeMatches(rt, input) {
				continue
			}
			matches = append(matches, projectRelationType(rt))
		}
		switch len(matches) {
		case 0:
			return Output{NotFound: true, Reason: notFoundReason(input)}, nil
		case 1:
			m := matches[0]
			return Output{Match: &m}, nil
		default:
			return Output{Candidates: matches, Reason: "multiple matches; pick one by id and re-call with the publicId or narrow with endpoint asset type publicIds"}, nil
		}
	}
}

func relationTypeMatches(rt clients.RelationTypeDetails, in Input) bool {
	if in.PublicID != "" && rt.PublicID != in.PublicID {
		return false
	}
	if in.Role != "" && rt.Role != in.Role {
		return false
	}
	if in.HeadTypePublicID != "" && (rt.SourceType == nil || rt.SourceType.PublicID != in.HeadTypePublicID) {
		return false
	}
	if in.TailTypePublicID != "" && (rt.TargetType == nil || rt.TargetType.PublicID != in.TailTypePublicID) {
		return false
	}
	return true
}

func projectRelationType(rt clients.RelationTypeDetails) RelationType {
	out := RelationType{
		ID:          rt.ID,
		PublicID:    rt.PublicID,
		Description: rt.Description,
		Role:        rt.Role,
		CoRole:      rt.CoRole,
		System:      rt.System,
	}
	if rt.SourceType != nil {
		out.SourceType = &TypeReference{ID: rt.SourceType.ID, Name: rt.SourceType.Name, PublicID: rt.SourceType.PublicID}
	}
	if rt.TargetType != nil {
		out.TargetType = &TypeReference{ID: rt.TargetType.ID, Name: rt.TargetType.Name, PublicID: rt.TargetType.PublicID}
	}
	return out
}

func notFoundReason(in Input) string {
	var parts []string
	if in.PublicID != "" {
		parts = append(parts, fmt.Sprintf("publicId=%q", in.PublicID))
	}
	if in.Role != "" {
		parts = append(parts, fmt.Sprintf("role=%q", in.Role))
	}
	if in.HeadTypePublicID != "" {
		parts = append(parts, fmt.Sprintf("headTypePublicId=%q", in.HeadTypePublicID))
	}
	if in.TailTypePublicID != "" {
		parts = append(parts, fmt.Sprintf("tailTypePublicId=%q", in.TailTypePublicID))
	}
	return "no relation type matches " + strings.Join(parts, ", ")
}