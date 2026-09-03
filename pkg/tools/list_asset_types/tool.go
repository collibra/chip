// Package list_asset_types implements the list_asset_types MCP tool: it lists
// the asset types configured in Collibra (e.g. Business Term, Data Element,
// Issue), optionally filtered by name, publicId or product.
package list_asset_types

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// OutputStatus is the overall outcome of a list_asset_types call.
type OutputStatus string

const (
	// StatusSuccess means the asset types were returned.
	StatusSuccess OutputStatus = "success"
	// StatusValidationError means the inputs failed validation before any read.
	StatusValidationError OutputStatus = "validation_error"
	// StatusError means the asset types could not be read due to a downstream error.
	StatusError OutputStatus = "error"
)

const defaultLimit = 100

// assetTypeScanCap bounds the client-side scan used to apply publicId/product
// filters, which the Collibra asset types API cannot apply server-side. The
// scan stops after examining this many asset types even if the catalog is
// larger; when it does, the response reports scanExhaustive=false so a
// caller never mistakes the capped result for the complete one.
const assetTypeScanCap = 5000

// assetTypeScanPageSize is the page size used while scanning for a
// publicId/product match; the Collibra API accepts up to 1000 per page.
const assetTypeScanPageSize = 1000

// Input is the tool's typed input. All fields are optional filters/pagination.
type Input struct {
	Name     string `json:"name,omitempty" jsonschema:"Optional. Filter by asset type name — matched case-insensitively as a substring, applied server-side by Collibra. Example: 'Issue' matches 'Issue', 'Data Issue', 'Data Quality Issue'."`
	PublicId string `json:"publicId,omitempty" jsonschema:"Optional. Filter to the asset type whose publicId exactly matches this value (case-insensitive), e.g. 'Issue'. The publicId is a stable identifier distinct from the display name. Applied via a bounded scan of the catalog — see scanExhaustive/scanned in the response."`
	Product  string `json:"product,omitempty" jsonschema:"Optional. Filter to asset types tagged with this exact product name (case-insensitive), e.g. 'Data Helpdesk'. Every asset type — root and subtype — is individually tagged with its own product, so this matches per-record with no type-hierarchy walk. Applied via a bounded scan of the catalog — see scanExhaustive/scanned in the response."`
	Offset   int    `json:"offset,omitempty" jsonschema:"Optional. Pagination offset into the (filtered) results (min 0). Defaults to 0."`
	Limit    int    `json:"limit,omitempty" jsonschema:"Optional. Maximum number of results to return (1-1000). Default: 100."`
}

// AssetType is one returned asset type.
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

// Output is the typed response.
type Output struct {
	Status         OutputStatus `json:"status" jsonschema:"'success' when asset types were returned; 'validation_error' for bad inputs; 'error' for downstream failures."`
	Message        string       `json:"message" jsonschema:"Human-readable summary."`
	Total          int64        `json:"total" jsonschema:"The total number of asset types matching the filters (for pagination). When scanExhaustive is false, this counts only matches found within the scanned prefix of the catalog, not the whole instance."`
	Offset         int64        `json:"offset" jsonschema:"The offset for the results"`
	Limit          int64        `json:"limit" jsonschema:"The maximum number of results returned"`
	AssetTypes     []AssetType  `json:"assetTypes" jsonschema:"The list of asset types"`
	ScanExhaustive bool         `json:"scanExhaustive" jsonschema:"True if the result is a complete answer over the whole catalog. False only when publicId/product filtering hit the scan cap before reaching the end of the catalog — in that case, matching asset types may exist beyond what was scanned, and a caller must not treat 'total'/'assetTypes' as the full answer."`
	Scanned        int64        `json:"scanned" jsonschema:"Number of asset types examined from the underlying catalog to produce this response. Equals the server-reported total when name is the only filter or no filter was given, since that case is fully server-side."`
}

// NewTool returns the registered tool.
func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:  "list_asset_types",
		Title: "List Asset Types",
		Description: "List the asset types configured in Collibra (e.g. Business Term, Data Element, Issue and its subtypes) with their " +
			"properties and metadata. Use this to discover which asset type to pass to create_asset/prepare_create_asset when you don't " +
			"already know its exact publicId or display name. Optional filters: name (case-insensitive substring, e.g. 'Issue' also " +
			"matches 'Data Issue'), publicId (exact match on the stable identifier), and product (exact match on the product an asset " +
			"type is tagged with, e.g. 'Data Helpdesk' — every root type and subtype carries its own product tag). name is applied " +
			"server-side. publicId/product cannot be filtered server-side by the Collibra API, so they are applied by scanning the " +
			"catalog up to a bounded cap; the response's scanExhaustive/scanned fields report whether that scan covered the whole " +
			"catalog or was capped, so a capped, partial result is never mistaken for a complete one. Paginated (offset/limit) over the " +
			"filtered results.",
		Handler:     handler(collibraClient),
		Permissions: []string{},
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, DestructiveHint: chip.Ptr(false), IdempotentHint: true, OpenWorldHint: chip.Ptr(false)},
	}
}

func handler(collibraClient *http.Client) chip.ToolHandlerFunc[Input, Output] {
	return func(ctx context.Context, input Input) (Output, error) {
		if input.Offset < 0 {
			return Output{Status: StatusValidationError, Message: "offset must be >= 0."}, nil
		}
		limit := input.Limit
		if limit == 0 {
			limit = defaultLimit
		}
		if limit < 1 || limit > 1000 {
			return Output{Status: StatusValidationError, Message: "limit must be between 1 and 1000."}, nil
		}

		name := strings.TrimSpace(input.Name)
		publicID := strings.TrimSpace(input.PublicId)
		product := strings.TrimSpace(input.Product)

		if publicID == "" && product == "" {
			return listServerSide(ctx, collibraClient, name, input.Offset, limit)
		}
		return listByScan(ctx, collibraClient, name, publicID, product, input.Offset, limit)
	}
}

// listServerSide handles the case where every requested filter (at most
// `name`) can be forwarded to the Collibra API directly, so its own
// pagination and total are exact and already exhaustive.
func listServerSide(ctx context.Context, collibraClient *http.Client, name string, offset, limit int) (Output, error) {
	response, err := clients.ListAssetTypes(ctx, collibraClient, limit, offset, name)
	if err != nil {
		return Output{Status: StatusError, Message: fmt.Sprintf("Could not list asset types: %v", err)}, nil
	}

	assetTypes := toAssetTypes(response.Results)
	return Output{
		Status:         StatusSuccess,
		Message:        fmt.Sprintf("Returned %d of %d asset type(s).", len(assetTypes), response.Total),
		Total:          response.Total,
		Offset:         response.Offset,
		Limit:          response.Limit,
		AssetTypes:     assetTypes,
		ScanExhaustive: true,
		Scanned:        response.Total,
	}, nil
}

// listByScan handles publicId/product filters, which Collibra cannot apply
// server-side. It scans the catalog (narrowed by name server-side, if given)
// up to assetTypeScanCap, filters each record client-side, and paginates the
// matches found. The response always reports whether that scan was exhaustive.
func listByScan(ctx context.Context, collibraClient *http.Client, name, publicID, product string, offset, limit int) (Output, error) {
	matches, scanned, exhaustive, err := scanAssetTypes(ctx, collibraClient, name, publicID, product)
	if err != nil {
		return Output{Status: StatusError, Message: fmt.Sprintf("Could not list asset types: %v", err)}, nil
	}

	page := paginate(matches, offset, limit)
	assetTypes := toAssetTypes(page)

	message := fmt.Sprintf("Returned %d of %d matching asset type(s) (scanned %d of the catalog).", len(assetTypes), len(matches), scanned)
	if !exhaustive {
		message += " Scan was capped before reaching the end of the catalog — more matches may exist beyond what was scanned."
	}

	return Output{
		Status:         StatusSuccess,
		Message:        message,
		Total:          int64(len(matches)),
		Offset:         int64(offset),
		Limit:          int64(limit),
		AssetTypes:     assetTypes,
		ScanExhaustive: exhaustive,
		Scanned:        scanned,
	}, nil
}

// scanAssetTypes pages through the Collibra asset types API (server-side
// narrowed by name, if given) up to assetTypeScanCap, and returns every
// record whose publicId/product matches. exhaustive is true only if the scan
// reached the end of the catalog before hitting the cap.
func scanAssetTypes(ctx context.Context, collibraClient *http.Client, name, publicID, product string) ([]clients.AssetTypeDetails, int64, bool, error) {
	var matches []clients.AssetTypeDetails
	var scanned int64
	offset := 0

	for {
		response, err := clients.ListAssetTypes(ctx, collibraClient, assetTypeScanPageSize, offset, name)
		if err != nil {
			return nil, scanned, false, err
		}

		for _, at := range response.Results {
			scanned++
			if matchesAssetType(at, publicID, product) {
				matches = append(matches, at)
			}
		}

		offset += len(response.Results)
		if len(response.Results) == 0 || int64(offset) >= response.Total {
			return matches, scanned, true, nil
		}
		if scanned >= assetTypeScanCap {
			return matches, scanned, false, nil
		}
	}
}

// matchesAssetType reports whether at satisfies the given publicId/product
// filters (each ignored when empty); both compare case-insensitively.
func matchesAssetType(at clients.AssetTypeDetails, publicID, product string) bool {
	if publicID != "" && !strings.EqualFold(at.PublicId, publicID) {
		return false
	}
	if product != "" && !strings.EqualFold(at.Product, product) {
		return false
	}
	return true
}

// paginate slices a filtered match list per the requested offset/limit,
// bounded to what's available.
func paginate(matches []clients.AssetTypeDetails, offset, limit int) []clients.AssetTypeDetails {
	if offset >= len(matches) {
		return nil
	}
	end := offset + limit
	if end > len(matches) {
		end = len(matches)
	}
	return matches[offset:end]
}

// toAssetTypes converts client asset type records to the tool's output shape.
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
