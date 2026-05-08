package clients

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/collibra/chip/pkg/chip"
	sdk "github.com/collibra/data-access-go-sdk"
	"github.com/collibra/data-access-go-sdk/services"
	"github.com/collibra/data-access-go-sdk/types"
)

// DataAccessControlDetails holds the details of a single data access control.
type DataAccessControlDetails struct {
	ID                string                   `json:"id" jsonschema:"Unique identifier of the access control"`
	Name              string                   `json:"name" jsonschema:"Name of the access control"`
	Description       string                   `json:"description" jsonschema:"Detailed description of the access control"`
	State             string                   `json:"state" jsonschema:"State of the access control: ACTIVE, INACTIVE, or DELETED"`
	Action            string                   `json:"action" jsonschema:"Action type of the access control: GRANT, MASK, FILTER, SHARE, GROUP, or FILTERRULE"`
	Category          *DataAccessGrantCategory `json:"category,omitempty" jsonschema:"Grant category details, present only for GRANT action type"`
	External          bool                     `json:"external" jsonschema:"Whether the access control is managed externally in the data source rather than in Collibra Data Access"`
	NamingHint        *string                  `json:"namingHint,omitempty" jsonschema:"Naming hint used for generating names in target systems"`
	PolicyRule        *string                  `json:"policyRule,omitempty" jsonschema:"Policy rule string, used for imported row-level filters and column masks"`
	NotInternalizable bool                     `json:"notInternalizable" jsonschema:"Whether the external access control cannot be internalized"`
	Complete          *bool                    `json:"complete,omitempty" jsonschema:"Whether the external access control is complete (all linked entities known in Collibra Data Access)"`
	WhatUnknown       bool                     `json:"whatUnknown" jsonschema:"Whether the WHAT scope of this access control could not be parsed on import"`
	WhoUnknown        bool                     `json:"whoUnknown" jsonschema:"Whether the WHO scope of this access control could not be parsed on import"`
	CreatedAt         time.Time                `json:"createdAt" jsonschema:"Timestamp when the access control was created"`
	ModifiedAt        time.Time                `json:"modifiedAt" jsonschema:"Timestamp when the access control was last modified"`
	What              []DataAccessWhatItem     `json:"what" jsonschema:"List of access controls that this control applies to (the WHAT scope)"`
	Who               []DataAccessWhoItem      `json:"who" jsonschema:"List of principals (users, access controls, data sources) that are granted access by this control"`
	SyncData          []DataAccessSyncData     `json:"syncData" jsonschema:"Synchronization status per linked data source. Valid sync statuses: Notconnected, Failed, Outofdate, Inprogress, Synced, Outofsync."`
}

// DataAccessSyncData holds the sync status of an access control for a single data source.
type DataAccessSyncData struct {
	DataSourceID   string `json:"dataSourceId" jsonschema:"Unique identifier of the linked data source"`
	DataSourceName string `json:"dataSourceName" jsonschema:"Name of the linked data source"`
	SyncStatus     string `json:"syncStatus" jsonschema:"Sync status for this data source. Valid values: Notconnected, Failed, Outofdate, Inprogress, Synced, Outofsync."`
}

// DataAccessWhatItem represents a single entry in the WHAT list of an access control —
// another access control that this one applies to.
type DataAccessWhatItem struct {
	ID        string     `json:"id" jsonschema:"Unique identifier of the access control in the WHAT list"`
	Name      string     `json:"name" jsonschema:"Name of the access control in the WHAT list"`
	State     string     `json:"state" jsonschema:"State of the access control: Active, Inactive, or Deleted"`
	Action    string     `json:"action" jsonschema:"Action type of the access control: Grant, Mask, Filter, Share, Group, or FilterRule"`
	ExpiresAt *time.Time `json:"expiresAt,omitempty" jsonschema:"Optional expiration time for this WHAT entry"`
}

// DataAccessWhoItem represents a single entry in the WHO list of an access control.
type DataAccessWhoItem struct {
	// Type is either "WhoGrant" (direct access) or "WhoPromise" (pre-approved access on request).
	Type            string     `json:"type" jsonschema:"Grant type: WhoGrant (direct access) or WhoPromise (pre-approved on request)"`
	ExpiresAt       *time.Time `json:"expiresAt,omitempty" jsonschema:"Optional expiration time for this WHO entry"`
	PromiseDuration *int64     `json:"promiseDuration,omitempty" jsonschema:"For WhoPromise: duration in seconds of the grant when access is requested"`
	// ItemType is the GraphQL typename of the item: User, AccessControl, DataShareRecipient, DataSource.
	ItemType string  `json:"itemType" jsonschema:"Type of the granted principal: User, AccessControl, DataShareRecipient, or DataSource"`
	ItemID   string  `json:"itemId,omitempty" jsonschema:"ID of the granted principal (present for User and AccessControl item types)"`
	ItemName string  `json:"itemName,omitempty" jsonschema:"Display name of the granted principal (present for User and AccessControl item types)"`
	Email    *string `json:"email,omitempty" jsonschema:"Email address of the user (present for User item type only)"`
	UserType string  `json:"userType,omitempty" jsonschema:"Whether the user is a Human or Machine user (present for User item type only)"`
}

// DataAccessGrantCategory holds the details of a grant category.
type DataAccessGrantCategory struct {
	ID         string `json:"id" jsonschema:"Unique identifier of the grant category"`
	Name       string `json:"name" jsonschema:"Display name of the grant category"`
	NamePlural string `json:"namePlural" jsonschema:"Plural display name of the grant category"`
	IsSystem   bool   `json:"isSystem" jsonschema:"Whether this grant category is system-defined and cannot be edited or removed"`
	IsDefault  bool   `json:"isDefault" jsonschema:"Whether this is the default grant category for new access controls"`
}

// GetDataAccessControl retrieves a single data access control by ID.
// It creates an sdk.CollibraClient using chip's existing HTTP client via sdk.WithHTTPClient,
// so URL routing and authentication are handled by chip's RoundTripper.
func GetDataAccessControl(ctx context.Context, httpClient *http.Client, id string) (*DataAccessControlDetails, error) {
	collibraHost, ok := chip.GetCollibraHost(ctx)
	if !ok {
		return nil, fmt.Errorf("collibra host not configured in context")
	}
	dataAccessURL := strings.TrimSuffix(collibraHost, "/") + "/dataAccess"

	collibraClient, err := sdk.NewClient(dataAccessURL, sdk.WithHTTPClient(httpClient))
	if err != nil {
		return nil, fmt.Errorf("failed to create data access client: %w", err)
	}

	accessControlClient := collibraClient.AccessControl()

	ac, err := accessControlClient.GetAccessControl(ctx, id)
	if err != nil {
		return nil, err
	}

	details := mapToDataAccessControlDetails(ac)

	for whatItem, err := range accessControlClient.GetAccessControlWhatAccessControlList(ctx, id) {
		if err != nil {
			return nil, fmt.Errorf("failed to retrieve what list: %w", err)
		}
		details.What = append(details.What, mapToDataAccessWhatItem(whatItem))
	}

	for whoItem, err := range accessControlClient.GetAccessControlWhoList(ctx, id) {
		if err != nil {
			return nil, fmt.Errorf("failed to retrieve who list: %w", err)
		}
		details.Who = append(details.Who, mapToDataAccessWhoItem(whoItem))
	}

	return details, nil
}

// SearchDataAccessControlsResult holds a page of access controls and an optional next-page cursor.
type SearchDataAccessControlsResult struct {
	Items      []*DataAccessControlDetails `json:"items"`
	NextCursor *string                     `json:"nextCursor,omitempty"`
}

// SearchDataAccessControls returns a page of data access controls filtered by name, actions, and/or states.
// Name search is case-insensitive contains. Pass cursor from a previous response to fetch the next page.
func SearchDataAccessControls(ctx context.Context, httpClient *http.Client, name string, actions []string, states []string, cursor string, pageSize int) (*SearchDataAccessControlsResult, error) {
	collibraHost, ok := chip.GetCollibraHost(ctx)
	if !ok {
		return nil, fmt.Errorf("collibra host not configured in context")
	}
	dataAccessURL := strings.TrimSuffix(collibraHost, "/") + "/dataAccess"

	collibraClient, err := sdk.NewClient(dataAccessURL, sdk.WithHTTPClient(httpClient))
	if err != nil {
		return nil, fmt.Errorf("failed to create data access client: %w", err)
	}

	filter := &types.AccessControlFilterInput{}
	if name != "" {
		filter.Search = &name
	}
	for _, a := range actions {
		filter.Actions = append(filter.Actions, types.AccessControlAction(a))
	}
	for _, s := range states {
		filter.States = append(filter.States, types.AccessControlState(s))
	}

	opts := []func(*services.AccessControlListOptions){
		services.WithAccessControlListFilter(filter),
	}
	if cursor != "" {
		opts = append(opts, services.WithAccessControlListCursor(cursor))
	}
	if pageSize > 0 {
		opts = append(opts, services.WithAccessControlListPageSize(pageSize))
	}

	items, nextCursor, err := collibraClient.AccessControl().ListAccessControlsPage(ctx, opts...)
	if err != nil {
		return nil, err
	}

	result := &SearchDataAccessControlsResult{
		Items:      make([]*DataAccessControlDetails, 0, len(items)),
		NextCursor: nextCursor,
	}
	for _, ac := range items {
		result.Items = append(result.Items, mapToDataAccessControlDetails(ac))
	}
	return result, nil
}

func mapToDataAccessWhatItem(w *types.AccessWhatAccessControlItem) DataAccessWhatItem {
	item := DataAccessWhatItem{
		ExpiresAt: w.ExpiresAt,
	}
	if w.AccessControl != nil {
		item.ID = w.AccessControl.AccessControl.Id
		item.Name = w.AccessControl.AccessControl.Name
		item.State = string(w.AccessControl.AccessControl.State)
		item.Action = string(w.AccessControl.AccessControl.Action)
	}
	return item
}

func mapToDataAccessWhoItem(w *types.AccessWhoItem) DataAccessWhoItem {
	item := DataAccessWhoItem{
		Type:            string(w.Type),
		ExpiresAt:       w.ExpiresAt,
		PromiseDuration: w.PromiseDuration,
	}

	switch v := w.Item.(type) {
	case *types.AccessWhoItemItemUser:
		item.ItemType = "User"
		item.ItemID = v.User.Id
		item.ItemName = v.User.Name
		item.Email = v.User.Email
		item.UserType = string(v.User.Type)
	case *types.AccessWhoItemItemAccessControl:
		item.ItemType = "AccessControl"
		item.ItemID = v.Id
		item.ItemName = v.Name
	case *types.AccessWhoItemItemDataShareRecipient:
		item.ItemType = "DataShareRecipient"
	case *types.AccessWhoItemItemDataSource:
		item.ItemType = "DataSource"
	default:
		if v != nil {
			if t := w.Item.GetTypename(); t != nil {
				item.ItemType = *t
			}
		}
	}

	return item
}

// DataAccessIdentity represents a user in Collibra Data Access.
type DataAccessIdentity struct {
	ID    string  `json:"id" jsonschema:"Unique identifier of the user"`
	Name  string  `json:"name" jsonschema:"Display name of the user"`
	Email *string `json:"email,omitempty" jsonschema:"Email address of the user"`
	Type  string  `json:"type" jsonschema:"User type: Human or Machine"`
}

// SearchDataAccessIdentitiesResult holds a page of identities and an optional next-page cursor.
type SearchDataAccessIdentitiesResult struct {
	Items      []*DataAccessIdentity
	NextCursor *string
}

// SearchDataAccessIdentities searches for Data Access users by name and/or email.
// When email is provided, an exact lookup via GetUserByEmail is performed. Name is then applied
// as an optional client-side case-insensitive contains filter on the result.
// When only name is provided, SearchUsers is called with the Search filter (case-insensitive
// contains). Cursor and pageSize control pagination for name-based searches.
func SearchDataAccessIdentities(ctx context.Context, httpClient *http.Client, name, email, cursor string, pageSize int) (*SearchDataAccessIdentitiesResult, error) {
	collibraHost, ok := chip.GetCollibraHost(ctx)
	if !ok {
		return nil, fmt.Errorf("collibra host not configured in context")
	}
	dataAccessURL := strings.TrimSuffix(collibraHost, "/") + "/dataAccess"

	collibraClient, err := sdk.NewClient(dataAccessURL, sdk.WithHTTPClient(httpClient))
	if err != nil {
		return nil, fmt.Errorf("failed to create data access client: %w", err)
	}

	if email != "" {
		user, err := collibraClient.User().GetUserByEmail(ctx, email)
		if err != nil {
			var notFound *types.ErrNotFound
			if errors.As(err, &notFound) {
				return &SearchDataAccessIdentitiesResult{Items: []*DataAccessIdentity{}}, nil
			}
			return nil, err
		}

		identity := mapToDataAccessIdentity(user)
		if name != "" && !strings.Contains(strings.ToLower(identity.Name), strings.ToLower(name)) {
			return &SearchDataAccessIdentitiesResult{Items: []*DataAccessIdentity{}}, nil
		}
		return &SearchDataAccessIdentitiesResult{Items: []*DataAccessIdentity{identity}}, nil
	}

	// Name-only path: use SearchUsers with the Search filter.
	filter := &types.UserFilterInput{}
	if name != "" {
		filter.Search = &name
	}

	var after *string
	if cursor != "" {
		after = &cursor
	}
	var limit *int
	if pageSize > 0 {
		limit = &pageSize
	}

	users, nextCursor, err := collibraClient.User().SearchUsers(ctx, after, limit, filter)
	if err != nil {
		return nil, err
	}

	result := &SearchDataAccessIdentitiesResult{
		Items:      make([]*DataAccessIdentity, 0, len(users)),
		NextCursor: nextCursor,
	}
	for i := range users {
		result.Items = append(result.Items, mapToDataAccessIdentity(&users[i]))
	}
	return result, nil
}

func mapToDataAccessIdentity(u *types.User) *DataAccessIdentity {
	return &DataAccessIdentity{
		ID:    u.Id,
		Name:  u.Name,
		Email: u.Email,
		Type:  string(u.Type),
	}
}

func mapToDataAccessControlDetails(ac *types.AccessControl) *DataAccessControlDetails {
	details := &DataAccessControlDetails{
		What:              []DataAccessWhatItem{},
		Who:               []DataAccessWhoItem{},
		SyncData:          []DataAccessSyncData{},
		ID:                ac.Id,
		Name:              ac.Name,
		Description:       ac.Description,
		State:             string(ac.State),
		Action:            string(ac.Action),
		External:          ac.External,
		NamingHint:        ac.NamingHint,
		PolicyRule:        ac.PolicyRule,
		NotInternalizable: ac.NotInternalizable,
		Complete:          ac.Complete,
		WhatUnknown:       ac.WhatUnknown,
		WhoUnknown:        ac.WhoUnknown,
		CreatedAt:         ac.CreatedAt,
		ModifiedAt:        ac.ModifiedAt,
	}

	if ac.Category != nil {
		details.Category = &DataAccessGrantCategory{
			ID:         ac.Category.GrantCategory.Id,
			Name:       ac.Category.GrantCategory.Name,
			NamePlural: ac.Category.GrantCategory.NamePlural,
			IsSystem:   ac.Category.GrantCategory.IsSystem,
			IsDefault:  ac.Category.GrantCategory.IsDefault,
		}
	}

	for _, sd := range ac.SyncData {
		ds := sd.GetDataSource()
		details.SyncData = append(details.SyncData, DataAccessSyncData{
			DataSourceID:   ds.GetId(),
			DataSourceName: ds.GetName(),
			SyncStatus:     string(sd.GetSyncStatus()),
		})
	}

	return details
}
