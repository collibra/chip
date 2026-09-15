package list_asset_types

import (
	"context"
	"net/http"
	"strings"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// defaultLimit is applied when the caller omits Limit.
const defaultLimit = 100

// serverBatchSize is the page size used while scanning for a publicId/product
// match. The Core API's documented maximum limit is 1000.
const serverBatchSize = 1000

// maxScan bounds a single tool call to 5 requests worst case.
const maxScan = 5000

// OutputStatus is the overall outcome of a list_asset_types call.
type OutputStatus string

const (
	// StatusSuccess means asset types were returned (possibly zero).
	StatusSuccess OutputStatus = "success"
	// StatusValidationError means the inputs failed validation before any read.
	StatusValidationError OutputStatus = "validation_error"
)

type Input struct {
	Limit    int    `json:"limit,omitempty" jsonschema:"Optional. Maximum number of results to return. The maximum allowed limit is 1000. Default: 100."`
	Offset   int    `json:"offset,omitempty" jsonschema:"Optional. Index of first result (pagination offset). Default: 0."`
	Name     string `json:"name,omitempty" jsonschema:"Optional. Filters server-side on the asset type's name, case-insensitive substring match (e.g. 'Issue'). Combine with publicId/product for a narrower, faster scan."`
	PublicId string `json:"publicId,omitempty" jsonschema:"Optional. Client-side filter: only asset types whose publicId contains this value, case-insensitive (e.g. 'Issue' matches 'DataIssue'). Applied over a bounded scan; check resultsTruncated."`
	Product  string `json:"product,omitempty" jsonschema:"Optional. Client-side filter: only asset types whose product field contains this value, case-insensitive (e.g. 'HELPDESK', 'GLOSSARY'). Useful to separate real product types from test/decoy types. Applied over a bounded scan; check resultsTruncated. Asset types with no product never match."`
}

type Output struct {
	Status           OutputStatus `json:"status" jsonschema:"'success' when the call completed; 'validation_error' for bad inputs."`
	Message          string       `json:"message,omitempty" jsonschema:"Human-readable summary, populated for validation_error."`
	Total            int64        `json:"total" jsonschema:"The total number of asset types matching the search criteria. On a publicId/product-filtered call, this is the number of matches found during the scan, not the instance-wide asset type count."`
	Offset           int64        `json:"offset" jsonschema:"The offset for the results"`
	Limit            int64        `json:"limit" jsonschema:"The maximum number of results returned"`
	AssetTypes       []AssetType  `json:"assetTypes" jsonschema:"The list of asset types"`
	ResultsTruncated bool         `json:"resultsTruncated" jsonschema:"True when this result may be incomplete: a publicId/product scan stopped at the scan cap, so there may be further matches beyond 'scanned'. Narrow with name to get a complete answer. Always false when neither publicId nor product was supplied, since the API applies name and pagination itself."`
	Scanned          int64        `json:"scanned" jsonschema:"Number of asset types chip examined to apply the publicId/product filters. 0 when no scan was needed (unfiltered or name-only calls)."`
}

type AssetType struct {
	ID                 string `json:"id" jsonschema:"The unique identifier of the asset type"`
	Name               string `json:"name" jsonschema:"The name of the asset type"`
	Description        string `json:"description,omitempty" jsonschema:"The description of the asset type"`
	PublicId           string `json:"publicId,omitempty" jsonschema:"The public id of the asset type"`
	DisplayNameEnabled bool   `json:"displayNameEnabled" jsonschema:"Whether display name is enabled for assets of this type"`
	RatingEnabled      bool   `json:"ratingEnabled" jsonschema:"Whether rating is enabled for assets of this type"`
	FinalType          bool   `json:"finalType" jsonschema:"Whether the ability to create child asset types is locked"`
	System             bool   `json:"system" jsonschema:"Whether this is a system asset type"`
	Product            string `json:"product,omitempty" jsonschema:"The product to which this asset type is linked"`
}

func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:  "list_asset_types",
		Title: "List Asset Types",
		Description: "List asset types available in Collibra with their properties and metadata. Optional filters: name (server-side, " +
			"case-insensitive substring), publicId and product (both client-side, case-insensitive substring, e.g. product: 'HELPDESK' or 'GLOSSARY'). " +
			"publicId/product filters scan a bounded window of the catalog; check resultsTruncated before treating a filtered result as complete.",
		Handler:     handler(collibraClient),
		Permissions: []string{},
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, DestructiveHint: chip.Ptr(false), IdempotentHint: true, OpenWorldHint: chip.Ptr(false)},
	}
}

func handler(collibraClient *http.Client) chip.ToolHandlerFunc[Input, Output] {
	return func(ctx context.Context, input Input) (Output, error) {
		input.Name = strings.TrimSpace(input.Name)
		input.PublicId = strings.TrimSpace(input.PublicId)
		input.Product = strings.TrimSpace(input.Product)

		if input.Limit == 0 {
			input.Limit = defaultLimit
		}
		if input.Limit < 1 || input.Limit > 1000 {
			return Output{Status: StatusValidationError, Message: "limit must be between 1 and 1000."}, nil
		}

		if input.PublicId == "" && input.Product == "" {
			return listServerSide(ctx, collibraClient, input)
		}
		return listWithScan(ctx, collibraClient, input)
	}
}

// listServerSide handles the unfiltered or name-only path: one request, the
// API applies name and pagination itself.
func listServerSide(ctx context.Context, collibraClient *http.Client, input Input) (Output, error) {
	response, err := clients.ListAssetTypes(ctx, collibraClient, input.Limit, input.Offset, input.Name)
	if err != nil {
		return Output{}, err
	}

	return Output{
		Status:     StatusSuccess,
		Total:      response.Total,
		Offset:     response.Offset,
		Limit:      response.Limit,
		AssetTypes: toAssetTypes(response.Results),
	}, nil
}

// listWithScan handles the publicId/product-filtered path: publicId and
// product aren't filterable server-side, so this scans pages (optionally
// narrowed server-side by name) up to maxScan and matches client-side.
// offset/limit are applied as a window over the accumulated matches, not
// over the raw scan.
func listWithScan(ctx context.Context, collibraClient *http.Client, input Input) (Output, error) {
	var matches []clients.AssetTypeDetails
	var scanned int64
	var truncated bool

	for offset := 0; ; {
		page, err := clients.ListAssetTypes(ctx, collibraClient, serverBatchSize, offset, input.Name)
		if err != nil {
			return Output{}, err
		}
		scanned += int64(len(page.Results))
		for _, at := range page.Results {
			if matchesScannedFilters(at, input) {
				matches = append(matches, at)
			}
		}
		if len(page.Results) == 0 || scanned >= page.Total {
			break
		}
		offset += len(page.Results)
		if scanned >= maxScan {
			truncated = true
			break
		}
	}

	return Output{
		Status:           StatusSuccess,
		Total:            int64(len(matches)),
		Offset:           int64(input.Offset),
		Limit:            int64(input.Limit),
		AssetTypes:       toAssetTypes(windowAssetTypes(matches, input.Offset, input.Limit)),
		ResultsTruncated: truncated,
		Scanned:          scanned,
	}, nil
}

// matchesScannedFilters applies the publicId/product client-side filters
// that the Core API does not support server-side.
func matchesScannedFilters(at clients.AssetTypeDetails, input Input) bool {
	if input.PublicId != "" && !containsFold(at.PublicId, input.PublicId) {
		return false
	}
	if input.Product != "" && !containsFold(at.Product, input.Product) {
		return false
	}
	return true
}

func containsFold(value, substr string) bool {
	return strings.Contains(strings.ToLower(value), strings.ToLower(substr))
}

// windowAssetTypes applies offset/limit over the accumulated matches.
func windowAssetTypes(matches []clients.AssetTypeDetails, offset, limit int) []clients.AssetTypeDetails {
	if offset >= len(matches) {
		return nil
	}
	end := offset + limit
	if end > len(matches) {
		end = len(matches)
	}
	return matches[offset:end]
}

func toAssetTypes(details []clients.AssetTypeDetails) []AssetType {
	assetTypes := make([]AssetType, len(details))
	for i, at := range details {
		assetTypes[i] = AssetType{
			ID:                 at.ID,
			Name:               at.Name,
			Description:        at.Description,
			PublicId:           at.PublicId,
			DisplayNameEnabled: at.DisplayNameEnabled,
			RatingEnabled:      at.RatingEnabled,
			FinalType:          at.FinalType,
			System:             at.System,
			Product:            at.Product,
		}
	}
	return assetTypes
}
