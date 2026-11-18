package clients

// The public DGC REST endpoints that read the directory of people and resources: users, user groups,
// roles, role assignments (responsibilities), communities and domains. Each request sends only the
// parameters that are set, so an omitted one keeps DGC's default, except limit
// (DefaultDirectoryLimit). The rows keep what a lookup needs: ids, names and status. They carry no
// email address, as no CHIP tool returns one, and no audit stamps.

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
)

// DefaultDirectoryLimit is sent when the caller gives no limit, in place of DGC's 1000 rows.
const DefaultDirectoryLimit = 20

// DirectoryPage is DGC's paged answer: total is 0 when countLimit is 0.
type DirectoryPage[Row any] struct {
	Total   int64 `json:"total"`
	Offset  int   `json:"offset"`
	Limit   int   `json:"limit"`
	Results []Row `json:"results"`
}

// DirectoryPaging is the paging every directory endpoint takes. A nil Offset or CountLimit is not
// sent; a nil Limit sends DefaultDirectoryLimit.
type DirectoryPaging struct {
	Offset     *int
	Limit      *int
	CountLimit *int
}

// DirectoryUser is a person in the DGC user directory, without contact details.
type DirectoryUser struct {
	ID         string `json:"id"`
	UserName   string `json:"userName,omitempty"`
	FirstName  string `json:"firstName,omitempty"`
	LastName   string `json:"lastName,omitempty"`
	Title      string `json:"title,omitempty"`
	Department string `json:"department,omitempty"`
	Enabled    bool   `json:"enabled"`
	System     bool   `json:"system"`
}

// DirectoryUserGroup is a named set of users.
type DirectoryUserGroup struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	System bool   `json:"system"`
}

// DirectoryRole is a global or resource role. Its permissions are left out: a lookup needs the role
// itself, and a role carries dozens of them.
type DirectoryRole struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Global      bool   `json:"global"`
	Disabled    bool   `json:"disabled"`
	System      bool   `json:"system"`
}

// DirectoryRef points at a resource: its id, its name when DGC gives one, and what kind of resource
// it is.
type DirectoryRef struct {
	ID   string `json:"id"`
	Name string `json:"name,omitempty"`
	Type string `json:"type,omitempty"`
}

// DirectoryCommunity is a community with the community it sits in, if any.
type DirectoryCommunity struct {
	ID          string        `json:"id"`
	Name        string        `json:"name"`
	Description string        `json:"description,omitempty"`
	Parent      *DirectoryRef `json:"parent,omitempty"`
}

// DirectoryDomain is a domain with its type and the community it belongs to.
type DirectoryDomain struct {
	ID          string        `json:"id"`
	Name        string        `json:"name"`
	Description string        `json:"description,omitempty"`
	Type        *DirectoryRef `json:"type,omitempty"`
	Community   *DirectoryRef `json:"community,omitempty"`
}

// DirectoryResponsibility says that Owner (a user or a user group) holds Role, on Resource or, for a
// global role, on the whole instance.
type DirectoryResponsibility struct {
	ID       string        `json:"id"`
	Role     *DirectoryRef `json:"role,omitempty"`
	Owner    *DirectoryRef `json:"owner,omitempty"`
	Resource *DirectoryRef `json:"resource,omitempty"`
}

type SearchUsersRequest struct {
	UserIDs         []string
	Name            string
	GroupID         string
	Title           string
	Department      string
	IncludeDisabled *bool
	DirectoryPaging
}

// SearchUsers calls GET /rest/2.0/users.
func SearchUsers(ctx context.Context, client *http.Client, request SearchUsersRequest) (*DirectoryPage[DirectoryUser], error) {
	params := url.Values{}
	addAll(params, "userId", request.UserIDs)
	addString(params, "name", request.Name)
	addString(params, "groupId", request.GroupID)
	addString(params, "title", request.Title)
	addString(params, "department", request.Department)
	addBool(params, "includeDisabled", request.IncludeDisabled)
	addPaging(params, request.DirectoryPaging)
	return getDirectoryPage[DirectoryUser](ctx, client, "/rest/2.0/users", params)
}

type SearchUserGroupsRequest struct {
	Name          string
	NameMatchMode string
	UserID        string
	DirectoryPaging
}

// SearchUserGroups calls GET /rest/2.0/userGroups.
func SearchUserGroups(ctx context.Context, client *http.Client, request SearchUserGroupsRequest) (*DirectoryPage[DirectoryUserGroup], error) {
	params := url.Values{}
	addString(params, "name", request.Name)
	addString(params, "nameMatchMode", request.NameMatchMode)
	addString(params, "userId", request.UserID)
	addPaging(params, request.DirectoryPaging)
	return getDirectoryPage[DirectoryUserGroup](ctx, client, "/rest/2.0/userGroups", params)
}

// GetUserGroup calls GET /rest/2.0/userGroups/{id}; DGC answers 404 for an unknown id.
func GetUserGroup(ctx context.Context, client *http.Client, groupID string) (*DirectoryUserGroup, error) {
	path := "/rest/2.0/userGroups/" + url.PathEscape(groupID)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to build GET %s: %w", path, err)
	}
	request.Header.Set("Accept", "application/json")
	body, err := executeCollibraRequest(client, request)
	if err != nil {
		return nil, err
	}
	var group DirectoryUserGroup
	if err := json.Unmarshal(body, &group); err != nil {
		return nil, fmt.Errorf("failed to parse the response of GET %s: %w", path, err)
	}
	return &group, nil
}

type SearchRolesRequest struct {
	Name                string
	NameMatchMode       string
	Description         string
	Type                string
	OwnerID             string
	RoleInheritanceMode string
	IncludeDisabled     *bool
	DirectoryPaging
}

// SearchRoles calls GET /rest/2.0/roles.
func SearchRoles(ctx context.Context, client *http.Client, request SearchRolesRequest) (*DirectoryPage[DirectoryRole], error) {
	params := url.Values{}
	addString(params, "name", request.Name)
	addString(params, "nameMatchMode", request.NameMatchMode)
	addString(params, "description", request.Description)
	addString(params, "type", request.Type)
	addString(params, "ownerId", request.OwnerID)
	addString(params, "roleInheritanceMode", request.RoleInheritanceMode)
	addBool(params, "includeDisabled", request.IncludeDisabled)
	addPaging(params, request.DirectoryPaging)
	return getDirectoryPage[DirectoryRole](ctx, client, "/rest/2.0/roles", params)
}

type SearchResponsibilitiesRequest struct {
	ResourceIDs        []string
	OwnerIDs           []string
	RoleIDs            []string
	IncludeInherited   *bool
	GlobalOnly         *bool
	ExcludeEmptyGroups *bool
	DirectoryPaging
}

// SearchResponsibilities calls GET /rest/2.0/responsibilities.
func SearchResponsibilities(ctx context.Context, client *http.Client, request SearchResponsibilitiesRequest) (*DirectoryPage[DirectoryResponsibility], error) {
	params := url.Values{}
	addAll(params, "resourceIds", request.ResourceIDs)
	addAll(params, "ownerIds", request.OwnerIDs)
	addAll(params, "roleIds", request.RoleIDs)
	addBool(params, "includeInherited", request.IncludeInherited)
	addBool(params, "globalOnly", request.GlobalOnly)
	addBool(params, "excludeEmptyGroups", request.ExcludeEmptyGroups)
	addPaging(params, request.DirectoryPaging)
	page, err := getDirectoryPage[wireResponsibility](ctx, client, "/rest/2.0/responsibilities", params)
	if err != nil {
		return nil, err
	}
	rows := make([]DirectoryResponsibility, len(page.Results))
	for i, row := range page.Results {
		rows[i] = row.toDirectory()
	}
	return &DirectoryPage[DirectoryResponsibility]{Total: page.Total, Offset: page.Offset, Limit: page.Limit, Results: rows}, nil
}

// wireResponsibility is a responsibility as DGC sends it: references carry their kind as
// resourceType or resourceDiscriminator.
type wireResponsibility struct {
	ID           string   `json:"id"`
	Role         wireRef  `json:"role"`
	Owner        wireRef  `json:"owner"`
	BaseResource *wireRef `json:"baseResource"`
}

type wireRef struct {
	ID                    string `json:"id"`
	Name                  string `json:"name"`
	ResourceType          string `json:"resourceType"`
	ResourceDiscriminator string `json:"resourceDiscriminator"`
}

func (r wireResponsibility) toDirectory() DirectoryResponsibility {
	row := DirectoryResponsibility{ID: r.ID, Role: r.Role.toDirectory(), Owner: r.Owner.toDirectory()}
	if r.BaseResource != nil {
		row.Resource = r.BaseResource.toDirectory()
	}
	return row
}

func (r wireRef) toDirectory() *DirectoryRef {
	if r.ID == "" {
		return nil
	}
	kind := r.ResourceDiscriminator
	if kind == "" {
		kind = r.ResourceType
	}
	return &DirectoryRef{ID: r.ID, Name: r.Name, Type: kind}
}

type ListCommunitiesRequest struct {
	Name          string
	NameMatchMode string
	ParentID      string
	ExcludeMeta   *bool
	DirectoryPaging
}

// ListCommunities calls GET /rest/2.0/communities.
func ListCommunities(ctx context.Context, client *http.Client, request ListCommunitiesRequest) (*DirectoryPage[DirectoryCommunity], error) {
	params := url.Values{}
	addString(params, "name", request.Name)
	addString(params, "nameMatchMode", request.NameMatchMode)
	addString(params, "parentId", request.ParentID)
	addBool(params, "excludeMeta", request.ExcludeMeta)
	addPaging(params, request.DirectoryPaging)
	page, err := getDirectoryPage[wireNamed](ctx, client, "/rest/2.0/communities", params)
	if err != nil {
		return nil, err
	}
	rows := make([]DirectoryCommunity, len(page.Results))
	for i, row := range page.Results {
		rows[i] = DirectoryCommunity{ID: row.ID, Name: row.Name, Description: row.Description, Parent: row.Parent.toDirectory()}
	}
	return &DirectoryPage[DirectoryCommunity]{Total: page.Total, Offset: page.Offset, Limit: page.Limit, Results: rows}, nil
}

type ListDomainsRequest struct {
	Name                  string
	NameMatchMode         string
	CommunityID           string
	TypeID                string
	IncludeSubCommunities *bool
	ExcludeMeta           *bool
	DirectoryPaging
}

// ListDomains calls GET /rest/2.0/domains.
func ListDomains(ctx context.Context, client *http.Client, request ListDomainsRequest) (*DirectoryPage[DirectoryDomain], error) {
	params := url.Values{}
	addString(params, "name", request.Name)
	addString(params, "nameMatchMode", request.NameMatchMode)
	addString(params, "communityId", request.CommunityID)
	addString(params, "typeId", request.TypeID)
	addBool(params, "includeSubCommunities", request.IncludeSubCommunities)
	addBool(params, "excludeMeta", request.ExcludeMeta)
	addPaging(params, request.DirectoryPaging)
	page, err := getDirectoryPage[wireNamed](ctx, client, "/rest/2.0/domains", params)
	if err != nil {
		return nil, err
	}
	rows := make([]DirectoryDomain, len(page.Results))
	for i, row := range page.Results {
		rows[i] = DirectoryDomain{ID: row.ID, Name: row.Name, Description: row.Description, Type: row.Type.toDirectory(), Community: row.Community.toDirectory()}
	}
	return &DirectoryPage[DirectoryDomain]{Total: page.Total, Offset: page.Offset, Limit: page.Limit, Results: rows}, nil
}

// wireNamed reads a community or a domain: the references the two do not have stay empty.
type wireNamed struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Parent      wireRef `json:"parent"`
	Type        wireRef `json:"type"`
	Community   wireRef `json:"community"`
}

// getDirectoryPage sends the GET and reads DGC's page; a failed call carries DGC's error message
// (executeCollibraRequest).
func getDirectoryPage[Row any](ctx context.Context, client *http.Client, path string, params url.Values) (*DirectoryPage[Row], error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, withQuery(path, params), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to build GET %s: %w", path, err)
	}
	request.Header.Set("Accept", "application/json")
	body, err := executeCollibraRequest(client, request)
	if err != nil {
		return nil, err
	}
	var page DirectoryPage[Row]
	if err := json.Unmarshal(body, &page); err != nil {
		return nil, fmt.Errorf("failed to parse the response of GET %s: %w", path, err)
	}
	return &page, nil
}

func addAll(params url.Values, key string, values []string) {
	for _, value := range values {
		params.Add(key, value)
	}
}

func addString(params url.Values, key string, value string) {
	if value != "" {
		params.Set(key, value)
	}
}

func addBool(params url.Values, key string, value *bool) {
	if value != nil {
		params.Set(key, strconv.FormatBool(*value))
	}
}

func addInt(params url.Values, key string, value *int) {
	if value != nil {
		params.Set(key, strconv.Itoa(*value))
	}
}

func addPaging(params url.Values, paging DirectoryPaging) {
	limit := DefaultDirectoryLimit
	if paging.Limit != nil {
		limit = *paging.Limit
	}
	addInt(params, "offset", paging.Offset)
	addInt(params, "limit", &limit)
	addInt(params, "countLimit", paging.CountLimit)
}

func withQuery(path string, params url.Values) string {
	if len(params) == 0 {
		return path
	}
	return path + "?" + params.Encode()
}
