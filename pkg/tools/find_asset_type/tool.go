// Package find_asset_type looks up a single asset type by publicId or
// exact name from the cached DGC catalog. Returns a resolve_domain-style
// envelope (match | candidates | notFound + reason) so the caller can
// uniformly handle the three outcomes.
package find_asset_type

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type Input struct {
	PublicID string `json:"publicId,omitempty" jsonschema:"Optional. Asset type publicId, e.g. 'ManagedControl'. Exact match. At least one of publicId or name must be provided."`
	Name     string `json:"name,omitempty" jsonschema:"Optional. Asset type display name. Exact match (case-sensitive). At least one of publicId or name must be provided."`
}

type Output struct {
	Match      *AssetType  `json:"match,omitempty" jsonschema:"Set when exactly one asset type matches"`
	Candidates []AssetType `json:"candidates,omitempty" jsonschema:"Set when multiple asset types match the criteria; the caller must pick one"`
	NotFound   bool        `json:"notFound,omitempty" jsonschema:"True when no asset type matches"`
	Reason     string      `json:"reason,omitempty" jsonschema:"Explanation when match is empty (notFound or multi-match)"`
}

type AssetType struct {
	ID                 string `json:"id" jsonschema:"The unique identifier of the asset type"`
	Name               string `json:"name" jsonschema:"The name of the asset type"`
	Description        string `json:"description,omitempty" jsonschema:"The description of the asset type"`
	PublicID           string `json:"publicId,omitempty" jsonschema:"The public id of the asset type"`
	DisplayNameEnabled bool   `json:"displayNameEnabled" jsonschema:"Whether display name is enabled for assets of this type"`
	RatingEnabled      bool   `json:"ratingEnabled" jsonschema:"Whether rating is enabled for assets of this type"`
	FinalType          bool   `json:"finalType" jsonschema:"Whether the ability to create child asset types is locked"`
	System             bool   `json:"system" jsonschema:"Whether this is a system asset type"`
}

func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:        "find_asset_type",
		Description: "Find a single asset type by publicId or exact name. Returns match / candidates / notFound. Reads from the same one-hour cache as list_asset_types.",
		Handler:     handler(collibraClient),
		Permissions: []string{},
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}
}

func handler(collibraClient *http.Client) chip.ToolHandlerFunc[Input, Output] {
	return func(ctx context.Context, input Input) (Output, error) {
		if input.PublicID == "" && input.Name == "" {
			return Output{}, errors.New("at least one of publicId or name must be provided")
		}
		all, err := clients.ListAllAssetTypes(ctx, collibraClient)
		if err != nil {
			return Output{}, err
		}
		var matches []AssetType
		for _, at := range all {
			if input.PublicID != "" && at.PublicId != input.PublicID {
				continue
			}
			if input.Name != "" && at.Name != input.Name {
				continue
			}
			matches = append(matches, AssetType{
				ID:                 at.ID,
				Name:               at.Name,
				Description:        at.Description,
				PublicID:           at.PublicId,
				DisplayNameEnabled: at.DisplayNameEnabled,
				RatingEnabled:      at.RatingEnabled,
				FinalType:          at.FinalType,
				System:             at.System,
			})
		}
		switch len(matches) {
		case 0:
			return Output{NotFound: true, Reason: notFoundReason(input)}, nil
		case 1:
			m := matches[0]
			return Output{Match: &m}, nil
		default:
			return Output{Candidates: matches, Reason: "multiple matches; pick one by id and re-call with the publicId"}, nil
		}
	}
}

func notFoundReason(in Input) string {
	switch {
	case in.PublicID != "" && in.Name != "":
		return fmt.Sprintf("no asset type matches publicId=%q and name=%q", in.PublicID, in.Name)
	case in.PublicID != "":
		return fmt.Sprintf("no asset type matches publicId=%q", in.PublicID)
	default:
		return fmt.Sprintf("no asset type matches name=%q", in.Name)
	}
}
