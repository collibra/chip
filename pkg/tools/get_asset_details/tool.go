package get_asset_details

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type Input struct {
	AssetID                 string `json:"assetId" jsonschema:"the UUID of the asset to retrieve details for"`
	OutgoingRelationsCursor string `json:"outgoingRelationsCursor,omitempty" jsonschema:"Optional. Cursor (asset ID) to fetch the next page of outgoing relations. Use the last relation's target ID from the previous response."`
	IncomingRelationsCursor string `json:"incomingRelationsCursor,omitempty" jsonschema:"Optional. Cursor (asset ID) to fetch the next page of incoming relations. Use the last relation's source ID from the previous response."`
}

type Output struct {
	Asset                  *clients.Asset        `json:"asset,omitempty" jsonschema:"the detailed asset information if found"`
	Responsibilities       []AssetResponsibility `json:"responsibilities,omitempty" jsonschema:"the responsibilities assigned to this asset, including inherited ones"`
	ResponsibilitiesStatus string                `json:"responsibilitiesStatus,omitempty" jsonschema:"status message for responsibilities, e.g. No responsibilities assigned"`
	Link                   string                `json:"link,omitempty" jsonschema:"the link you can navigate to in Collibra to view the asset"`
	Error                  string                `json:"error,omitempty" jsonschema:"error message if asset not found or other error occurred"`
	Found                  bool                  `json:"found" jsonschema:"whether the asset was found"`
}

// AssetResponsibility represents a role assignment (e.g., Owner, Steward) for an asset.
type AssetResponsibility struct {
	RoleName  string `json:"roleName" jsonschema:"the name of the resource role (e.g., Owner, Business Steward)"`
	UserName  string `json:"userName,omitempty" jsonschema:"the username of the assigned user, if the owner is a user"`
	GroupName string `json:"groupName,omitempty" jsonschema:"the name of the assigned group, if the owner is a user group"`
	Inherited bool   `json:"inherited" jsonschema:"true if the responsibility is inherited from a parent resource (domain or community), false if directly assigned to this asset"`
}

func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:        "get_asset_details",
		Description: "Get detailed information about a specific asset by its UUID, including attributes, relations, responsibilities (owners, stewards, and other role assignments), and metadata. Returns up to 100 attributes per type and supports cursor-based pagination for relations (50 per page).",
		Handler:     handler(collibraClient),
		Permissions: []string{},
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}
}

func handler(collibraClient *http.Client) chip.ToolHandlerFunc[Input, Output] {
	return func(ctx context.Context, input Input) (Output, error) {
		assetUUID, err := uuid.Parse(input.AssetID)
		if err != nil {
			return Output{Error: fmt.Sprintf("Invalid asset ID format: %s", err.Error()), Found: false}, nil
		}

		assets, err := clients.GetAssetSummary(ctx, collibraClient, assetUUID, input.OutgoingRelationsCursor, input.IncomingRelationsCursor)
		if err != nil {
			return Output{Error: fmt.Sprintf("Failed to retrieve asset details: %s", err.Error()), Found: false}, nil
		}

		if len(assets) == 0 {
			return Output{Error: "Asset not found", Found: false}, nil
		}

		collibraHost, ok := chip.GetCollibraHost(ctx)
		if !ok {
			slog.WarnContext(ctx, "Collibra instance URL unknown, links will be rendered without host")
		}

		responsibilities, err := clients.GetResponsibilities(ctx, collibraClient, assetUUID.String())
		if err != nil {
			slog.WarnContext(ctx, fmt.Sprintf("Failed to retrieve responsibilities: %s", err.Error()))
		}

		mappedResponsibilities := resolveResponsibilities(ctx, collibraClient, responsibilities, assetUUID.String())
		responsibilitiesStatus := ""
		if len(mappedResponsibilities) == 0 {
			responsibilitiesStatus = "No responsibilities assigned"
		}

		return Output{
			Asset:                  &assets[0],
			Responsibilities:       mappedResponsibilities,
			ResponsibilitiesStatus: responsibilitiesStatus,
			Found:                  true,
			Link:                   fmt.Sprintf("%s/asset/%s", strings.TrimSuffix(collibraHost, "/"), assetUUID),
		}, nil
	}
}

func resolveResponsibilities(ctx context.Context, collibraClient *http.Client, responsibilities []clients.Responsibility, assetID string) []AssetResponsibility {
	if len(responsibilities) == 0 {
		return nil
	}
	ownerNames := resolveOwnerNames(ctx, collibraClient, responsibilities)
	result := make([]AssetResponsibility, 0, len(responsibilities))
	for _, r := range responsibilities {
		entry := AssetResponsibility{}
		if r.Role != nil {
			entry.RoleName = r.Role.Name
		}
		if r.Owner != nil {
			resolved := ownerNames[r.Owner.ID]
			if r.Owner.ResourceDiscriminator == "UserGroup" {
				entry.GroupName = resolved
			} else {
				entry.UserName = resolved
			}
		}
		entry.Inherited = r.BaseResource != nil && r.BaseResource.ID != assetID
		result = append(result, entry)
	}
	return result
}

func resolveOwnerNames(ctx context.Context, collibraClient *http.Client, responsibilities []clients.Responsibility) map[string]string {
	owners := make(map[string]*clients.ResourceRef)
	for _, r := range responsibilities {
		if r.Owner != nil {
			owners[r.Owner.ID] = r.Owner
		}
	}
	names := make(map[string]string, len(owners))
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, owner := range owners {
		wg.Add(1)
		go func(o *clients.ResourceRef) {
			defer wg.Done()
			name := fetchOwnerName(ctx, collibraClient, o)
			mu.Lock()
			names[o.ID] = name
			mu.Unlock()
		}(owner)
	}
	wg.Wait()
	return names
}

func fetchOwnerName(ctx context.Context, collibraClient *http.Client, owner *clients.ResourceRef) string {
	switch owner.ResourceDiscriminator {
	case "User":
		name, err := clients.GetUserName(ctx, collibraClient, owner.ID)
		if err != nil {
			slog.WarnContext(ctx, fmt.Sprintf("Failed to resolve user name for %s: %s", owner.ID, err.Error()))
			return owner.ID
		}
		return name
	case "UserGroup":
		name, err := clients.GetUserGroupName(ctx, collibraClient, owner.ID)
		if err != nil {
			slog.WarnContext(ctx, fmt.Sprintf("Failed to resolve group name for %s: %s", owner.ID, err.Error()))
			return owner.ID
		}
		return name
	default:
		slog.WarnContext(ctx, fmt.Sprintf("Unknown owner type '%s' for %s", owner.ResourceDiscriminator, owner.ID))
		return owner.ID
	}
}
