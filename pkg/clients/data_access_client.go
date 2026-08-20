package clients

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/collibra/chip/pkg/chip"
	sdk "github.com/collibra/data-access-go-sdk"
	"github.com/collibra/data-access-go-sdk/services"
	"github.com/collibra/data-access-go-sdk/types"
	"github.com/google/uuid"
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
	Url               string                   `json:"url" jsonschema:"Url in the Collibra UI to view access control"`
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

// GetDataAccessControl retrieves a single data access control by ID, with its WHAT and WHO
// lists.
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

	details := mapToDataAccessControlDetails(ctx, ac)

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

func mapToDataAccessWhatItem(w *types.AccessWhatAccessControlItem) DataAccessWhatItem {
	item := DataAccessWhatItem{
		ExpiresAt: w.ExpiresAt,
	}
	if w.AccessControl != nil {
		item.ID = w.AccessControl.Id
		item.Name = w.AccessControl.Name
		item.State = string(w.AccessControl.State)
		item.Action = string(w.AccessControl.Action)
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
		item.ItemID = v.Id
		item.ItemName = v.Name
		item.Email = v.Email
		item.UserType = string(v.Type)
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

// DataAccessDataSource holds the details of a single Data Access data source.
type DataAccessDataSource struct {
	ID          string    `json:"id" jsonschema:"Unique identifier of the data source"`
	Name        string    `json:"name" jsonschema:"Display name of the data source"`
	Type        string    `json:"type" jsonschema:"Type identifier of the data source, set by the connector during a sync (e.g. snowflake, databricks, bigquery)"`
	Description string    `json:"description" jsonschema:"Description of the data source"`
	ParentID    string    `json:"parentId,omitempty" jsonschema:"Identifier of the parent data source, when this data source has one"`
	CreatedAt   time.Time `json:"createdAt" jsonschema:"Timestamp when the data source was created"`
	ModifiedAt  time.Time `json:"modifiedAt" jsonschema:"Timestamp when the data source was last modified"`
	Url         string    `json:"url" jsonschema:"Url in the Collibra UI to view data source"`
}

// GetDataAccessDataSource retrieves a single Data Access data source by ID.
func GetDataAccessDataSource(ctx context.Context, httpClient *http.Client, id string) (*DataAccessDataSource, error) {
	collibraHost, ok := chip.GetCollibraHost(ctx)
	if !ok {
		return nil, fmt.Errorf("collibra host not configured in context")
	}
	dataAccessURL := strings.TrimSuffix(collibraHost, "/") + "/dataAccess"

	collibraClient, err := sdk.NewClient(dataAccessURL, sdk.WithHTTPClient(httpClient))
	if err != nil {
		return nil, fmt.Errorf("failed to create data access client: %w", err)
	}

	ds, err := collibraClient.DataSource().GetDataSource(ctx, id)
	if err != nil {
		return nil, err
	}

	return mapToDataAccessDataSource(ctx, ds), nil
}

func mapToDataAccessDataSource(ctx context.Context, ds *types.DataSource) *DataAccessDataSource {
	uiURL := buildUiUrl(ctx, "data-sources", ds.Id)
	out := &DataAccessDataSource{
		ID:          ds.Id,
		Name:        ds.Name,
		Type:        ds.Type,
		Description: ds.Description,
		CreatedAt:   ds.CreatedAt,
		ModifiedAt:  ds.ModifiedAt,
		Url:         uiURL,
	}
	if ds.Parent != nil {
		out.ParentID = ds.Parent.Id
	}
	return out
}

// DataAccessIdentity represents a user in Collibra Data Access.
type DataAccessIdentity struct {
	ID    string  `json:"id" jsonschema:"Unique identifier of the user"`
	Name  string  `json:"name" jsonschema:"Display name of the user"`
	Email *string `json:"email,omitempty" jsonschema:"Email address of the user"`
	Type  string  `json:"type" jsonschema:"User type: Human or Machine"`
	Url   string  `json:"url" jsonschema:"Url in the Collibra UI to view identity"`
}

// SearchDataAccessIdentitiesResult holds a page of identities.
type SearchDataAccessIdentitiesResult struct {
	Items []*DataAccessIdentity
}

// SearchDataAccessIdentities searches for Data Access users by name and/or email. Email is an
// exact match, name a contains match; results are capped at pageSize.
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

		identity := mapToDataAccessIdentity(ctx, user)
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
		result.Items = append(result.Items, mapToDataAccessIdentity(ctx, user))
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
	Url                   string                 `json:"url" jsonschema:"Url in the Collibra UI to view the data object"`
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

// SearchDataAccessObjects returns the data objects matching the supplied filters, capped at
// pageSize.
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
		result.Items = append(result.Items, mapToDataAccessObject(ctx, obj))
		if len(result.Items) >= limit {
			break
		}
	}
	return result, nil
}

// DataObjectAccessRole is an access control (a "role") that grants a user access to a data
// object. It is the trimmed form surfaced in NearestAccessControls.
type DataObjectAccessRole struct {
	ID       string                   `json:"id" jsonschema:"Unique identifier of the access control granting the access"`
	Name     string                   `json:"name" jsonschema:"Name of the access control (role) granting the access"`
	Action   string                   `json:"action" jsonschema:"Action type of the access control: Grant, Mask, Filter, Share, Group, or FilterRule"`
	State    string                   `json:"state" jsonschema:"State of the access control: Active, Inactive, or Deleted"`
	Category *DataAccessGrantCategory `json:"category,omitempty" jsonschema:"Grant category details, present only for Grant action type"`
}

// UserDataObjectAccess describes the access a single user has on a single data object, and the
// roles (access controls) that grant it.
type UserDataObjectAccess struct {
	HasAccess         bool                   `json:"hasAccess" jsonschema:"Whether the user has any access to the data object"`
	Permissions       []string               `json:"permissions,omitempty" jsonschema:"Source-system permissions the user has on the data object (e.g. SELECT)"`
	GlobalPermissions []string               `json:"globalPermissions,omitempty" jsonschema:"Global permissions the user has on the data object (e.g. READ)"`
	ExpiresAt         *time.Time             `json:"expiresAt,omitempty" jsonschema:"When the access expires. Only populated when access is granted through a single access control; nil when multiple roles grant access."`
	Roles             []DataObjectAccessRole `json:"roles" jsonschema:"The access controls (roles) that grant the user access to the data object"`
}

// ObjectAccessResult ties a resolved data object to the user's access on it.
type ObjectAccessResult struct {
	DataObject *DataAccessObject     `json:"dataObject" jsonschema:"The resolved data object"`
	Access     *UserDataObjectAccess `json:"access" jsonschema:"The user's access to the data object, including the granting roles"`
}

// UnresolvedObjectID is a requested data object ID that could not be resolved to a data object.
type UnresolvedObjectID struct {
	ID     string `json:"id" jsonschema:"The supplied data object ID that could not be resolved"`
	Reason string `json:"reason" jsonschema:"Why it could not be resolved: not_found (no data object exists with this ID)"`
}

// CheckUserDataObjectAccessResult is the result of checking a user's access to one or more data
// objects identified by ID.
type CheckUserDataObjectAccessResult struct {
	User       *DataAccessIdentity   `json:"user" jsonschema:"The user the access was checked for (the current user when no userId/email was supplied)"`
	Results    []*ObjectAccessResult `json:"results" jsonschema:"Per-object access results for the IDs that resolved to a data object"`
	Unresolved []*UnresolvedObjectID `json:"unresolved,omitempty" jsonschema:"IDs that did not resolve to a data object. Ask the user to correct or drop these."`
}

// CheckUserDataObjectAccess reports the access a user has on each of the given data objects,
// and the roles granting it. Unknown object IDs come back in Unresolved.
func CheckUserDataObjectAccess(ctx context.Context, httpClient *http.Client, objectIDs []string, userID, email string) (*CheckUserDataObjectAccessResult, error) {
	collibraHost, ok := chip.GetCollibraHost(ctx)
	if !ok {
		return nil, fmt.Errorf("collibra host not configured in context")
	}
	dataAccessURL := strings.TrimSuffix(collibraHost, "/") + "/dataAccess"

	collibraClient, err := sdk.NewClient(dataAccessURL, sdk.WithHTTPClient(httpClient))
	if err != nil {
		return nil, fmt.Errorf("failed to create data access client: %w", err)
	}

	user, err := resolveDataAccessUser(ctx, collibraClient, userID, email)
	if err != nil {
		return nil, err
	}

	result := &CheckUserDataObjectAccessResult{
		User:       mapToDataAccessIdentity(ctx, user),
		Results:    []*ObjectAccessResult{},
		Unresolved: []*UnresolvedObjectID{},
	}

	for _, id := range objectIDs {
		trimmed := strings.TrimSpace(id)
		if trimmed == "" {
			continue
		}

		obj, err := collibraClient.DataObject().GetDataObject(ctx, trimmed)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch data object %q: %w", trimmed, err)
		}
		if obj == nil || obj.Id == "" {
			result.Unresolved = append(result.Unresolved, &UnresolvedObjectID{
				ID:     trimmed,
				Reason: "not_found",
			})
			continue
		}

		item, err := collibraClient.DataObject().GetUserAccessToDataObject(ctx, obj.Id, user.Id)
		if err != nil {
			return nil, fmt.Errorf("failed to check access for data object %q: %w", trimmed, err)
		}

		result.Results = append(result.Results, &ObjectAccessResult{
			DataObject: mapToDataAccessObject(ctx, obj),
			Access:     mapToUserDataObjectAccess(item),
		})
	}

	return result, nil
}

// resolveDataAccessUser resolves the user to check access for: by ID, then email, then the
// current user.
func resolveDataAccessUser(ctx context.Context, collibraClient *sdk.CollibraClient, userID, email string) (*types.User, error) {
	switch {
	case strings.TrimSpace(userID) != "":
		return collibraClient.User().GetUser(ctx, userID)
	case strings.TrimSpace(email) != "":
		return collibraClient.User().GetUserByEmail(ctx, email)
	default:
		return collibraClient.User().GetCurrentUser(ctx)
	}
}

func mapToUserDataObjectAccess(item *types.GroupedDataAccessReturnItem) *UserDataObjectAccess {
	if item == nil {
		return &UserDataObjectAccess{HasAccess: false, Roles: []DataObjectAccessRole{}}
	}

	access := &UserDataObjectAccess{
		HasAccess:         true,
		Permissions:       derefStrings(item.Permissions),
		GlobalPermissions: derefStrings(item.GlobalPermissions),
		ExpiresAt:         item.ExpiresAt,
		Roles:             make([]DataObjectAccessRole, 0, len(item.NearestAccessControls)),
	}

	for _, ac := range item.NearestAccessControls {
		if ac == nil {
			continue
		}
		role := DataObjectAccessRole{
			ID:     ac.Id,
			Name:   ac.Name,
			Action: string(ac.Action),
			State:  string(ac.State),
		}
		if ac.Category != nil {
			role.Category = &DataAccessGrantCategory{
				ID:         ac.Category.Id,
				Name:       ac.Category.Name,
				NamePlural: ac.Category.NamePlural,
				IsSystem:   ac.Category.IsSystem,
				IsDefault:  ac.Category.IsDefault,
			}
		}
		access.Roles = append(access.Roles, role)
	}

	return access
}

// derefStrings dereferences a slice of string pointers, skipping nils.
func derefStrings(in []*string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, 0, len(in))
	for _, s := range in {
		if s != nil {
			out = append(out, *s)
		}
	}
	return out
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

func mapToDataAccessObject(ctx context.Context, o *types.DataObject) *DataAccessObject {
	uiURL := buildUiUrl(ctx, "data-objects", o.Id)
	out := &DataAccessObject{
		ID:          o.Id,
		Name:        o.Name,
		FullName:    o.FullName,
		Type:        o.Type,
		DataType:    o.DataType,
		Deleted:     o.Deleted,
		Description: o.Description,
		Url:         uiURL,
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

func mapToDataAccessIdentity(ctx context.Context, u *types.User) *DataAccessIdentity {
	uiURL := buildUiUrl(ctx, "identities", u.Id)
	return &DataAccessIdentity{
		ID:    u.Id,
		Name:  u.Name,
		Email: u.Email,
		Type:  string(u.Type),
		Url:   uiURL,
	}
}

func mapToDataAccessControlDetails(ctx context.Context, ac *types.AccessControl) *DataAccessControlDetails {
	categoryName := "default"
	if ac.GetCategory() != nil {
		categoryName = ac.GetCategory().Name
	}
	uiURL := buildUiUrl(ctx, "access-controls/"+categoryName, ac.Id)
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
		Url:               uiURL,
	}

	if ac.Category != nil {
		details.Category = &DataAccessGrantCategory{
			ID:         ac.Category.Id,
			Name:       ac.Category.Name,
			NamePlural: ac.Category.NamePlural,
			IsSystem:   ac.Category.IsSystem,
			IsDefault:  ac.Category.IsDefault,
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

func buildUiUrl(ctx context.Context, resourceType string, id string) string {
	collibraHost, ok := chip.GetCollibraHost(ctx)
	if !ok {
		return ""
	}
	return strings.TrimSuffix(collibraHost, "/") + "/data-access/" + resourceType + "/" + id
}

// Well-known identifiers of the out-of-the-box Collibra Data Product operating model.
const (
	// DataProductAssetTypeID is the asset type of a Data Product.
	DataProductAssetTypeID = "00000000-0000-0000-0000-000000050000"
	// dataProductHasOutputPortPublicID is the relation type linking a Data Product to its
	// output ports.
	dataProductHasOutputPortPublicID = "DataProductHasOutputPort"
	// dataProductRelationsLimit caps the relations fetched for a single Data Product.
	dataProductRelationsLimit = 1000
	// groupSearchScanLimit caps the access controls scanned when looking up a group by name.
	groupSearchScanLimit = 200
)

// CatalogAssetRef identifies a Collibra catalog asset and its asset type.
type CatalogAssetRef struct {
	ID       string `json:"id" jsonschema:"UUID of the asset"`
	Name     string `json:"name" jsonschema:"Display name of the asset"`
	TypeID   string `json:"typeId" jsonschema:"UUID of the asset's type"`
	TypeName string `json:"typeName" jsonschema:"Name of the asset's type"`
}

// DataProductOutputPort is one output port of a Data Product.
type DataProductOutputPort struct {
	ID   string `json:"id" jsonschema:"UUID of the output port asset"`
	Name string `json:"name" jsonschema:"Display name of the output port"`
}

// CatalogAssetRole is the Data Access role — an access control — linked to a catalog asset.
// It is the WHAT of an access request raised on that asset.
type CatalogAssetRole struct {
	ID       string                   `json:"id" jsonschema:"Unique identifier of the access control (role) linked to the asset"`
	Name     string                   `json:"name" jsonschema:"Name of the role"`
	Action   string                   `json:"action" jsonschema:"Action type of the access control. A role is a Grant."`
	State    string                   `json:"state" jsonschema:"State of the access control: Active, Inactive, or Deleted"`
	Category *DataAccessGrantCategory `json:"category,omitempty" jsonschema:"Grant category of the role, present for Grant action types"`
}

// UnresolvedBeneficiary is a requested beneficiary that could not be mapped into Data Access.
type UnresolvedBeneficiary struct {
	Input  string `json:"input" jsonschema:"The supplied identifier"`
	Reason string `json:"reason" jsonschema:"Why it could not be mapped into Data Access"`
}

// DataAccessGroup is a group of users in Collibra Data Access.
type DataAccessGroup struct {
	ID   string `json:"id" jsonschema:"Unique identifier of the group"`
	Name string `json:"name" jsonschema:"Name of the group"`
}

// CreateAssetAccessRequestInput holds the parameters for an access request on a catalog
// asset.
type CreateAssetAccessRequestInput struct {
	Name                    *string
	Description             string
	UserIDs                 []string
	GroupIDs                []string
	RoleID                  string
	CatalogAsset            CatalogAssetRef
	ImplementationExpiresAt time.Time
}

// GetCatalogAsset fetches a Collibra asset's name and type, or nil when no asset with that id
// exists.
func GetCatalogAsset(ctx context.Context, client *http.Client, assetID string) (*CatalogAssetRef, error) {
	reqURL := fmt.Sprintf("/rest/2.0/assets/%s", url.PathEscape(assetID))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("building get asset request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	body, status, err := executeRequestWithStatus(client, req)
	if status == http.StatusNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("getting asset %s: %w", assetID, err)
	}

	var raw struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		DisplayName string `json:"displayName"`
		Type        struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"type"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("decoding asset response: %w", err)
	}
	if raw.ID == "" {
		return nil, nil
	}

	return &CatalogAssetRef{
		ID:       raw.ID,
		Name:     firstNonEmpty(raw.DisplayName, raw.Name),
		TypeID:   raw.Type.ID,
		TypeName: raw.Type.Name,
	}, nil
}

// ListDataProductOutputPorts returns the output ports of a Data Product.
func ListDataProductOutputPorts(ctx context.Context, client *http.Client, dataProductID string) ([]DataProductOutputPort, error) {
	relations, err := listOutgoingRelations(ctx, client, dataProductID)
	if err != nil {
		return nil, err
	}

	publicIDs := make(map[string]string, 4)
	ports := make([]DataProductOutputPort, 0, 4)
	for _, rel := range relations {
		if rel.Type.ID == "" || rel.Target.ID == "" {
			continue
		}
		publicID, cached := publicIDs[rel.Type.ID]
		if !cached {
			relType, err := GetRelationTypeFull(ctx, client, rel.Type.ID)
			if err != nil {
				return nil, fmt.Errorf("resolving relation type %s: %w", rel.Type.ID, err)
			}
			publicID = relType.PublicID
			publicIDs[rel.Type.ID] = publicID
		}
		if publicID != dataProductHasOutputPortPublicID {
			continue
		}
		ports = append(ports, DataProductOutputPort{
			ID:   rel.Target.ID,
			Name: firstNonEmpty(rel.Target.DisplayName, rel.Target.Name),
		})
	}
	return ports, nil
}

// ListAssetRoles returns the Data Access roles linked to a Collibra asset, empty when it has
// none.
func ListAssetRoles(ctx context.Context, httpClient *http.Client, assetID string) ([]CatalogAssetRole, error) {
	collibraClient, err := newDataAccessSDKClient(ctx, httpClient)
	if err != nil {
		return nil, err
	}

	filter := &types.AccessControlFilterInput{AssetIds: []string{assetID}}

	roles := make([]CatalogAssetRole, 0, 1)
	for accessControl, err := range collibraClient.AccessControl().ListAccessControls(ctx, services.WithAccessControlListFilter(filter)) {
		if err != nil {
			return nil, fmt.Errorf("listing the roles linked to asset %s: %w", assetID, err)
		}
		if accessControl == nil {
			continue
		}
		roles = append(roles, mapToCatalogAssetRole(accessControl))
	}
	return roles, nil
}

func mapToCatalogAssetRole(ac *types.AccessControl) CatalogAssetRole {
	role := CatalogAssetRole{
		ID:     ac.Id,
		Name:   ac.Name,
		Action: string(ac.Action),
		State:  string(ac.State),
	}
	if ac.Category != nil {
		role.Category = &DataAccessGrantCategory{
			ID:         ac.Category.Id,
			Name:       ac.Category.Name,
			NamePlural: ac.Category.NamePlural,
			IsSystem:   ac.Category.IsSystem,
			IsDefault:  ac.Category.IsDefault,
		}
	}
	return role
}

// catalogAssetRelation is the subset of a /rest/2.0/relations result this flow needs.
type catalogAssetRelation struct {
	ID   string `json:"id"`
	Type struct {
		ID string `json:"id"`
	} `json:"type"`
	Target struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		DisplayName string `json:"displayName"`
	} `json:"target"`
}

// listOutgoingRelations returns the relations where the given asset is the head.
func listOutgoingRelations(ctx context.Context, client *http.Client, assetID string) ([]catalogAssetRelation, error) {
	endpoint, err := buildUrl("/rest/2.0/relations", RelationsQueryParams{
		SourceID: assetID,
		Limit:    dataProductRelationsLimit,
	})
	if err != nil {
		return nil, fmt.Errorf("building relations request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("building relations request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	body, err := executeRequest(client, req)
	if err != nil {
		return nil, fmt.Errorf("listing relations of asset %s: %w", assetID, err)
	}

	var response struct {
		Results []catalogAssetRelation `json:"results"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("decoding relations response: %w", err)
	}
	return response.Results, nil
}

// ResolveDataAccessUsers maps Collibra users, given as email addresses or usernames, to Data
// Access users by email. Entries that do not map are returned separately.
func ResolveDataAccessUsers(ctx context.Context, httpClient *http.Client, identifiers []string) ([]*DataAccessIdentity, []UnresolvedBeneficiary, error) {
	collibraClient, err := newDataAccessSDKClient(ctx, httpClient)
	if err != nil {
		return nil, nil, err
	}

	resolved := make([]*DataAccessIdentity, 0, len(identifiers))
	unresolved := make([]UnresolvedBeneficiary, 0)
	seen := make(map[string]struct{}, len(identifiers))

	for _, identifier := range identifiers {
		trimmed := strings.TrimSpace(identifier)
		if trimmed == "" {
			continue
		}

		email, reason, err := resolveCollibraUserEmail(ctx, httpClient, trimmed)
		if err != nil {
			return nil, nil, err
		}
		if reason != "" {
			unresolved = append(unresolved, UnresolvedBeneficiary{Input: trimmed, Reason: reason})
			continue
		}

		user, err := collibraClient.User().GetUserByEmail(ctx, email)
		if err != nil {
			var notFound *types.ErrNotFound
			if errors.As(err, &notFound) {
				unresolved = append(unresolved, UnresolvedBeneficiary{
					Input:  trimmed,
					Reason: fmt.Sprintf("no Data Access user with email address %q", email),
				})
				continue
			}
			return nil, nil, err
		}
		if _, duplicate := seen[user.Id]; duplicate {
			continue
		}
		seen[user.Id] = struct{}{}
		resolved = append(resolved, mapToDataAccessIdentity(ctx, user))
	}

	return resolved, unresolved, nil
}

// resolveCollibraUserEmail turns an email address or Collibra username into the email to look
// up in Data Access. A non-empty reason means it could not be resolved.
func resolveCollibraUserEmail(ctx context.Context, httpClient *http.Client, identifier string) (email string, reason string, err error) {
	if strings.Contains(identifier, "@") {
		return identifier, "", nil
	}

	user, err := FindUserByUsername(ctx, httpClient, identifier)
	if err != nil {
		return "", "", err
	}
	if user == nil {
		return "", fmt.Sprintf("no Collibra user with username %q — supply an email address instead", identifier), nil
	}
	if strings.TrimSpace(user.EmailAddress) == "" {
		return "", fmt.Sprintf("Collibra user %q has no email address, so it cannot be mapped to a Data Access user", identifier), nil
	}
	return user.EmailAddress, "", nil
}

// ResolveDataAccessGroups maps Collibra groups, given as names or UUIDs, to Data Access groups
// by name. Entries that do not map are returned separately.
func ResolveDataAccessGroups(ctx context.Context, httpClient *http.Client, identifiers []string) ([]*DataAccessGroup, []UnresolvedBeneficiary, error) {
	resolved := make([]*DataAccessGroup, 0, len(identifiers))
	unresolved := make([]UnresolvedBeneficiary, 0)
	seen := make(map[string]struct{}, len(identifiers))

	for _, identifier := range identifiers {
		trimmed := strings.TrimSpace(identifier)
		if trimmed == "" {
			continue
		}

		name, reason, err := resolveCollibraGroupName(ctx, httpClient, trimmed)
		if err != nil {
			return nil, nil, err
		}
		if reason != "" {
			unresolved = append(unresolved, UnresolvedBeneficiary{Input: trimmed, Reason: reason})
			continue
		}

		group, reason, err := findDataAccessGroupByName(ctx, httpClient, name)
		if err != nil {
			return nil, nil, err
		}
		if reason != "" {
			unresolved = append(unresolved, UnresolvedBeneficiary{Input: trimmed, Reason: reason})
			continue
		}
		if _, duplicate := seen[group.ID]; duplicate {
			continue
		}
		seen[group.ID] = struct{}{}
		resolved = append(resolved, group)
	}

	return resolved, unresolved, nil
}

// resolveCollibraGroupName turns a group name or Collibra group UUID into the name to look up
// in Data Access. A non-empty reason means it could not be resolved.
func resolveCollibraGroupName(ctx context.Context, httpClient *http.Client, identifier string) (name string, reason string, err error) {
	if _, parseErr := uuid.Parse(identifier); parseErr != nil {
		return identifier, "", nil
	}

	groupName, err := GetUserGroupName(ctx, httpClient, identifier)
	if err != nil {
		return "", fmt.Sprintf("no Collibra group with id %q — supply the group name instead", identifier), nil
	}
	if strings.TrimSpace(groupName) == "" {
		return "", fmt.Sprintf("Collibra group %q has no name, so it cannot be mapped to a Data Access group", identifier), nil
	}
	return groupName, "", nil
}

// findDataAccessGroupByName looks up a Data Access group by its exact name. Groups are the
// access controls with the Group action, and name is the only identifier they share with
// Collibra. A non-empty reason means no single group matched.
func findDataAccessGroupByName(ctx context.Context, httpClient *http.Client, name string) (*DataAccessGroup, string, error) {
	collibraClient, err := newDataAccessSDKClient(ctx, httpClient)
	if err != nil {
		return nil, "", err
	}

	filter := &types.AccessControlFilterInput{
		Actions: []types.AccessControlAction{types.AccessControlActionGroup},
		Search:  &name,
	}

	matches := make([]*DataAccessGroup, 0, 1)
	scanned := 0
	for accessControl, err := range collibraClient.AccessControl().ListAccessControls(ctx, services.WithAccessControlListFilter(filter)) {
		if err != nil {
			return nil, "", fmt.Errorf("searching for Data Access group %q: %w", name, err)
		}
		if accessControl == nil {
			continue
		}
		if strings.EqualFold(accessControl.Name, name) {
			matches = append(matches, &DataAccessGroup{ID: accessControl.Id, Name: accessControl.Name})
		}
		scanned++
		if scanned >= groupSearchScanLimit {
			break
		}
	}

	switch len(matches) {
	case 0:
		return nil, fmt.Sprintf("no Data Access group named %q", name), nil
	case 1:
		return matches[0], "", nil
	default:
		return nil, fmt.Sprintf("several Data Access groups are named %q, so it is unclear which one is meant", name), nil
	}
}

// CreateAssetAccessRequest creates an access request for the role linked to a Collibra asset.
func CreateAssetAccessRequest(ctx context.Context, httpClient *http.Client, input CreateAssetAccessRequestInput) (*DataAccessRequestSummary, error) {
	collibraClient, err := newDataAccessSDKClient(ctx, httpClient)
	if err != nil {
		return nil, err
	}

	assetID, err := uuid.Parse(input.CatalogAsset.ID)
	if err != nil {
		return nil, fmt.Errorf("invalid catalog asset id %q: %w", input.CatalogAsset.ID, err)
	}
	assetTypeID, err := uuid.Parse(input.CatalogAsset.TypeID)
	if err != nil {
		return nil, fmt.Errorf("invalid catalog asset type id %q: %w", input.CatalogAsset.TypeID, err)
	}

	expiresAt := input.ImplementationExpiresAt
	req := types.AccessRequestInput{
		Name:        input.Name,
		Description: &input.Description,
		Who: &types.AccessRequestWhoInput{
			Users:          input.UserIDs,
			AccessControls: input.GroupIDs,
		},
		What: []types.AccessRequestWhatInput{{
			AccessControl: &types.AccessRequestAccessControlWhatInput{Id: input.RoleID},
		}},
		CatalogAsset: &types.CatalogAssetInput{
			AssetId:     assetID,
			AssetTypeId: assetTypeID,
		},
		ImplementationExpiresAt: &expiresAt,
	}

	ar, err := collibraClient.AccessRequest().CreateAccessRequest(ctx, req)
	if err != nil {
		return nil, err
	}

	return &DataAccessRequestSummary{
		ID:          ar.Id,
		Name:        ar.Name,
		Description: ar.Description,
		Status:      string(ar.Status),
		Outcome:     string(ar.Outcome),
		Url:         buildUiUrl(ctx, "access-requests", ar.Id),
	}, nil
}

// newDataAccessSDKClient builds a Data Access SDK client on top of chip's HTTP client.
func newDataAccessSDKClient(ctx context.Context, httpClient *http.Client) (*sdk.CollibraClient, error) {
	collibraHost, ok := chip.GetCollibraHost(ctx)
	if !ok {
		return nil, fmt.Errorf("collibra host not configured in context")
	}
	dataAccessURL := strings.TrimSuffix(collibraHost, "/") + "/dataAccess"

	collibraClient, err := sdk.NewClient(dataAccessURL, sdk.WithHTTPClient(httpClient))
	if err != nil {
		return nil, fmt.Errorf("failed to create data access client: %w", err)
	}
	return collibraClient, nil
}

// firstNonEmpty returns the first value that is not empty after trimming.
func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
