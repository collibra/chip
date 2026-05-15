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

// SearchDataAccessIdentitiesResult holds a page of identities.
type SearchDataAccessIdentitiesResult struct {
	Items []*DataAccessIdentity
}

// SearchDataAccessIdentities searches for Data Access users by name and/or email.
// When email is provided, an exact lookup via GetUserByEmail is performed. Name is then applied
// as an optional client-side case-insensitive contains filter on the result.
// When only name is provided, ListUsers is called with the Search filter (case-insensitive
// contains). The returned list is capped at pageSize items (default 25).
func SearchDataAccessIdentities(ctx context.Context, httpClient *http.Client, name, email string, pageSize int) (*SearchDataAccessIdentitiesResult, error) {
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

	filter := &types.UserFilterInput{}
	if name != "" {
		filter.Search = &name
	}

	limit := pageSize
	if limit <= 0 {
		limit = 25
	}

	result := &SearchDataAccessIdentitiesResult{
		Items: make([]*DataAccessIdentity, 0, limit),
	}
	for user, iterErr := range collibraClient.User().ListUsers(ctx, services.WithUserListFilter(filter)) {
		if iterErr != nil {
			return nil, iterErr
		}
		result.Items = append(result.Items, mapToDataAccessIdentity(user))
		if len(result.Items) >= limit {
			break
		}
	}
	return result, nil
}

// DataAccessObject represents a single data object in Collibra Data Access.
type DataAccessObject struct {
	ID                    string                 `json:"id" jsonschema:"Unique identifier of the data object"`
	Name                  string                 `json:"name" jsonschema:"Name of the data object"`
	FullName              string                 `json:"fullName" jsonschema:"Fully qualified name of the data object within its data source"`
	Type                  string                 `json:"type" jsonschema:"Type of the data object (e.g. table, column, schema, view)"`
	DataType              *string                `json:"dataType,omitempty" jsonschema:"Data type of the object (typically used for columns)"`
	Deleted               bool                   `json:"deleted" jsonschema:"Whether the data object is deleted (no longer present in the source)"`
	Description           string                 `json:"description" jsonschema:"Description of the data object"`
	DataSourceID          string                 `json:"dataSourceId,omitempty" jsonschema:"Identifier of the data source the object belongs to"`
	ApplicablePermissions []DataAccessPermission `json:"applicablePermissions,omitempty" jsonschema:"Source-system permissions that can be requested or granted on this data object (and its descendants). Each permission carries its name and description."`
}

// DataAccessPermission is a permission that can be set on a data object.
type DataAccessPermission struct {
	Name        string `json:"name" jsonschema:"Permission name as defined by the data source (e.g. SELECT, INSERT)"`
	Description string `json:"description" jsonschema:"Human-readable description of the permission"`
}

// SearchDataAccessObjectsResult holds a page of data objects.
type SearchDataAccessObjectsResult struct {
	Items []*DataAccessObject `json:"items"`
}

// SearchDataAccessObjects returns a list of data objects matching the supplied filters.
// Name search is case-insensitive contains. The returned list is capped at pageSize items
// (default 25), drawn from the SDK's ListDataObjects iterator.
func SearchDataAccessObjects(ctx context.Context, httpClient *http.Client, name string, dataSources []string, dataObjectTypes []string, parents []string, ancestors []string, includeDeleted bool, pageSize int) (*SearchDataAccessObjectsResult, error) {
	collibraHost, ok := chip.GetCollibraHost(ctx)
	if !ok {
		return nil, fmt.Errorf("collibra host not configured in context")
	}
	dataAccessURL := strings.TrimSuffix(collibraHost, "/") + "/dataAccess"

	collibraClient, err := sdk.NewClient(dataAccessURL, sdk.WithHTTPClient(httpClient))
	if err != nil {
		return nil, fmt.Errorf("failed to create data access client: %w", err)
	}

	filter := &types.DataObjectFilterInput{}
	if name != "" {
		filter.Search = &name
	}
	if len(dataSources) > 0 {
		filter.DataSources = dataSources
	}
	if len(dataObjectTypes) > 0 {
		filter.Types = dataObjectTypes
	}
	if len(parents) > 0 {
		filter.Parents = parents
	}
	if len(ancestors) > 0 {
		filter.Ancestors = ancestors
	}
	if includeDeleted {
		filter.IncludeDeleted = &includeDeleted
	}

	limit := pageSize
	if limit <= 0 {
		limit = 25
	}

	result := &SearchDataAccessObjectsResult{
		Items: make([]*DataAccessObject, 0, limit),
	}
	for obj, iterErr := range collibraClient.DataObject().ListDataObjects(ctx, services.WithDataObjectListFilter(filter)) {
		if iterErr != nil {
			return nil, iterErr
		}
		result.Items = append(result.Items, mapToDataAccessObject(obj))
		if len(result.Items) >= limit {
			break
		}
	}
	return result, nil
}

// CreateDataAccessRequestWhatInput describes a single WHAT item (a data object) for a new
// data access request, with optional requested permissions.
type CreateDataAccessRequestWhatInput struct {
	DataObjectID      string   `json:"dataObjectId" jsonschema:"The ID of the data object the requesters want access to. Obtain via search_data_access_objects."`
	Permissions       []string `json:"permissions,omitempty" jsonschema:"Source-system permissions requested on this data object (e.g. SELECT). Should always be empty."`
	GlobalPermissions []string `json:"globalPermissions,omitempty" jsonschema:"Global permissions requested on this data object. Must always be READ."`
}

// CreateDataAccessRequestInput holds the parameters required to create a new data access request.
type CreateDataAccessRequestInput struct {
	Name        *string
	Description string
	UserIDs     []string
	What        []CreateDataAccessRequestWhatInput
}

// DataAccessRequestSummary is the simplified result of creating an access request.
type DataAccessRequestSummary struct {
	ID          string  `json:"id" jsonschema:"Unique identifier of the created access request"`
	Name        *string `json:"name,omitempty" jsonschema:"Display name of the access request"`
	Description string  `json:"description" jsonschema:"Description of the access request"`
	Status      string  `json:"status" jsonschema:"Current status of the access request (e.g. Created, Approval, Implementation, Closed)"`
	Outcome     string  `json:"outcome" jsonschema:"Current outcome of the access request"`
	Url         string  `json:"url" jsonschema:"Url in the Collibra UI to view access request"`
}

// CreateDataAccessRequest creates a new Data Access request via the SDK's AccessRequestClient.
func CreateDataAccessRequest(ctx context.Context, httpClient *http.Client, input CreateDataAccessRequestInput) (*DataAccessRequestSummary, error) {
	collibraHost, ok := chip.GetCollibraHost(ctx)
	if !ok {
		return nil, fmt.Errorf("collibra host not configured in context")
	}
	dataAccessURL := strings.TrimSuffix(collibraHost, "/") + "/dataAccess"

	collibraClient, err := sdk.NewClient(dataAccessURL, sdk.WithHTTPClient(httpClient))
	if err != nil {
		return nil, fmt.Errorf("failed to create data access client: %w", err)
	}

	what := make([]types.AccessRequestWhatInput, 0, len(input.What))
	for _, w := range input.What {
		what = append(what, types.AccessRequestWhatInput{
			DataObject: &types.AccessRequestDataObjectWhatInput{
				Id:                w.DataObjectID,
				Permissions:       w.Permissions,
				GlobalPermissions: w.GlobalPermissions,
			},
		})
	}

	req := types.AccessRequestInput{
		Name:        input.Name,
		Description: &input.Description,
		Who: &types.AccessRequestWhoInput{
			Users: input.UserIDs,
		},
		What: what,
	}

	ar, err := collibraClient.AccessRequest().CreateAccessRequest(ctx, req)
	if err != nil {
		return nil, err
	}

	requestURL := strings.TrimSuffix(collibraHost, "/") + "/data-access/access-requests/" + ar.Id

	return &DataAccessRequestSummary{
		ID:          ar.Id,
		Name:        ar.Name,
		Description: ar.Description,
		Status:      string(ar.Status),
		Outcome:     string(ar.Outcome),
		Url:         requestURL,
	}, nil
}

func mapToDataAccessObject(o *types.DataObject) *DataAccessObject {
	out := &DataAccessObject{
		ID:          o.Id,
		Name:        o.Name,
		FullName:    o.FullName,
		Type:        o.Type,
		DataType:    o.DataType,
		Deleted:     o.Deleted,
		Description: o.Description,
	}
	if o.DataSource != nil {
		out.DataSourceID = o.DataSource.Id
	}
	if len(o.ApplicablePermissions) > 0 {
		out.ApplicablePermissions = make([]DataAccessPermission, 0, len(o.ApplicablePermissions))
		for _, p := range o.ApplicablePermissions {
			out.ApplicablePermissions = append(out.ApplicablePermissions, DataAccessPermission{
				Name:        p.Name,
				Description: p.Description,
			})
		}
	}
	return out
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
