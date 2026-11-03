// Package create_domain implements the create_domain MCP tool: a single write
// tool that creates a Collibra domain inside an existing community. It is
// built around the confirm checkpoint of docs/TOOL_CONTRIBUTION_STANDARDS.md
// section 5: confirm=false (the default) returns a preview of everything that
// would be written and writes nothing; confirm=true performs the write.
package create_domain

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
	"github.com/collibra/chip/pkg/tools/resolve"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// maxNameLength is the longest domain name Collibra accepts.
const maxNameLength = 255

// OutputStatus is the overall outcome of a create_domain call.
type OutputStatus string

const (
	// StatusPreview means confirm was not set: nothing was written.
	StatusPreview OutputStatus = "preview"
	// StatusSuccess means the domain was created.
	StatusSuccess OutputStatus = "success"
	// StatusValidationError means validation or name resolution failed before
	// any write.
	StatusValidationError OutputStatus = "validation_error"
	// StatusError means Collibra rejected the write.
	StatusError OutputStatus = "error"
)

// Input is the tool's typed input.
type Input struct {
	Name        string `json:"name" jsonschema:"Required. Name of the new domain, 1-255 characters. Must be unique within its community, e.g. 'Wellness Glossary'."`
	Community   string `json:"community" jsonschema:"Required. The existing community that will contain the domain, as a UUID or the community's exact display name (case-insensitive), e.g. 'Employee Wellness'."`
	DomainType  string `json:"domainType" jsonschema:"Required. The kind of domain, as a UUID or the domain type's exact display name (case-insensitive), e.g. 'Glossary' or 'Physical Data Dictionary'. The domain type decides which asset types the domain can hold. If the name does not match, the error lists the valid domain types."`
	Description string `json:"description,omitempty" jsonschema:"Optional. Free-text description of the domain."`
	Confirm     bool   `json:"confirm,omitempty" jsonschema:"Safety checkpoint. false (the default) returns a PREVIEW of the domain and writes nothing, so it can be reviewed with the user. Set true only after the user approved the preview."`
}

// Preview echoes every field that confirm=true would write.
type Preview struct {
	Name           string `json:"name"`
	Description    string `json:"description,omitempty"`
	CommunityID    string `json:"communityId"`
	CommunityName  string `json:"communityName,omitempty"`
	DomainTypeID   string `json:"domainTypeId"`
	DomainTypeName string `json:"domainTypeName,omitempty"`
}

// DomainSummary is the created domain as Collibra returned it.
type DomainSummary struct {
	ID            string `json:"id" jsonschema:"UUID of the newly created domain."`
	Name          string `json:"name"`
	Description   string `json:"description,omitempty"`
	CommunityID   string `json:"communityId,omitempty"`
	CommunityName string `json:"communityName,omitempty"`
	DomainTypeID  string `json:"domainTypeId,omitempty"`
	DomainType    string `json:"domainType,omitempty" jsonschema:"Display name of the domain type."`
}

// Output is the typed response.
type Output struct {
	Status  OutputStatus   `json:"status" jsonschema:"'preview' when confirm was not set (nothing written; review it, then call again with confirm=true); 'success' when the domain was created; 'validation_error' for invalid or unresolvable inputs; 'error' when Collibra rejected the write."`
	Message string         `json:"message" jsonschema:"Human-readable summary."`
	Preview *Preview       `json:"preview,omitempty" jsonschema:"The domain that would be created, when status is 'preview'."`
	Domain  *DomainSummary `json:"domain,omitempty" jsonschema:"The created domain, when status is 'success'."`
}

// NewTool returns the registered tool.
func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:  "create_domain",
		Title: "Create Domain",
		Description: "Create a new Collibra domain — a container for assets that lives inside a community (a community being the top-level grouping of domains). " +
			"The domainType (for example 'Glossary' for business terms, or 'Physical Data Dictionary' for tables and columns) decides which asset types the domain can hold. " +
			"Both community and domainType accept a UUID or an exact display name, so there is no need to look the UUIDs up first; " +
			"an unknown domainType returns the list of valid ones. " +
			"Use this to create a domain; to create a community use create_community, and to create assets inside a domain use create_asset. " +
			"The name must be unique within the community. " +
			"Built around a confirm checkpoint: confirm=false (the default) returns a PREVIEW of exactly what would be created and writes nothing — show it to the user; " +
			"call again with confirm=true to create it. " +
			"Returns the new domain's id on success. " +
			"Requires permission to create domains in the target community (typically an administrator action); without it the write fails with a permission error. " +
			"Example requests: 'create a glossary domain called Wellness Terms in the Employee Wellness community', 'add a new domain for our data dictionary under Finance', 'I need somewhere to put business terms for HR'.",
		Handler:     handler(collibraClient),
		Permissions: []string{},
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: false, DestructiveHint: chip.Ptr(false), IdempotentHint: false, OpenWorldHint: chip.Ptr(false)},
	}
}

func handler(collibraClient *http.Client) chip.ToolHandlerFunc[Input, Output] {
	return func(ctx context.Context, input Input) (Output, error) {
		name := strings.TrimSpace(input.Name)
		if msg := validate(name, input); msg != "" {
			return Output{Status: StatusValidationError, Message: msg}, nil
		}

		community, err := resolve.Community(ctx, collibraClient, "community", input.Community)
		if err != nil {
			return Output{Status: StatusValidationError, Message: fmt.Sprintf("community could not be resolved: %v", err)}, nil
		}
		domainType, err := resolve.DomainType(ctx, collibraClient, "domainType", input.DomainType)
		if err != nil {
			return Output{Status: StatusValidationError, Message: fmt.Sprintf("domainType could not be resolved: %v", err)}, nil
		}

		if !input.Confirm {
			return previewOutput(name, input.Description, community, domainType), nil
		}

		resp, err := clients.CreateDomain(ctx, collibraClient, clients.CreateDomainRequest{
			Name:        name,
			CommunityID: community.ID,
			TypeID:      domainType.ID,
			Description: input.Description,
		})
		if err != nil {
			return Output{Status: StatusError, Message: fmt.Sprintf("Could not create domain: %v", err)}, nil
		}
		return successOutput(resp), nil
	}
}

func validate(name string, input Input) string {
	switch {
	case name == "":
		return "name is required: provide a domain name of 1-255 characters."
	case utf8.RuneCountInString(name) > maxNameLength:
		return fmt.Sprintf("name is %d characters long; Collibra allows at most %d. Shorten it.", utf8.RuneCountInString(name), maxNameLength)
	case strings.TrimSpace(input.Community) == "":
		return "community is required: pass the UUID or exact name of the community that will contain the domain."
	case strings.TrimSpace(input.DomainType) == "":
		return "domainType is required: pass a domain type UUID or exact name, e.g. 'Glossary'."
	}
	return ""
}

func previewOutput(name, description string, community, domainType resolve.Ref) Output {
	return Output{
		Status: StatusPreview,
		Message: fmt.Sprintf("Preview only — nothing was created. Will create domain %q of type %s in community %s. "+
			"Review this with the user, then call again with confirm=true.", name, label(domainType), label(community)),
		Preview: &Preview{
			Name:           name,
			Description:    description,
			CommunityID:    community.ID,
			CommunityName:  community.Name,
			DomainTypeID:   domainType.ID,
			DomainTypeName: domainType.Name,
		},
	}
}

func successOutput(resp *clients.CreateDomainResponse) Output {
	summary := &DomainSummary{ID: resp.ID, Name: resp.Name, Description: resp.Description}
	if resp.Community != nil {
		summary.CommunityID = resp.Community.ID
		summary.CommunityName = resp.Community.Name
	}
	if resp.Type != nil {
		summary.DomainTypeID = resp.Type.ID
		summary.DomainType = resp.Type.Name
	}
	return Output{
		Status:  StatusSuccess,
		Message: fmt.Sprintf("Created domain %q (id %s).", resp.Name, resp.ID),
		Domain:  summary,
	}
}

// label renders a resolved reference by name, falling back to its UUID when the
// caller supplied a UUID and no name was looked up.
func label(ref resolve.Ref) string {
	if ref.Name != "" {
		return fmt.Sprintf("%q", ref.Name)
	}
	return ref.ID
}
