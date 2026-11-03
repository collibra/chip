// Package create_community implements the create_community MCP tool: a single
// write tool that creates a Collibra community, optionally nested under an
// existing parent community. It is built around the confirm checkpoint of
// docs/TOOL_CONTRIBUTION_STANDARDS.md section 5: confirm=false (the default)
// returns a preview of everything that would be written and writes nothing;
// confirm=true performs the write.
package create_community

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

// maxNameLength is the longest community name Collibra accepts.
const maxNameLength = 255

// OutputStatus is the overall outcome of a create_community call.
type OutputStatus string

const (
	// StatusPreview means confirm was not set: nothing was written.
	StatusPreview OutputStatus = "preview"
	// StatusSuccess means the community was created.
	StatusSuccess OutputStatus = "success"
	// StatusValidationError means validation or name resolution failed before
	// any write.
	StatusValidationError OutputStatus = "validation_error"
	// StatusError means Collibra rejected the write.
	StatusError OutputStatus = "error"
)

// Input is the tool's typed input.
type Input struct {
	Name            string `json:"name" jsonschema:"Required. Name of the new community, 1-255 characters. Collibra requires community names to be unique across the whole instance, e.g. 'Employee Wellness'."`
	ParentCommunity string `json:"parentCommunity,omitempty" jsonschema:"Optional. The existing community to nest the new one under, as a UUID or the parent's exact display name (case-insensitive). Omit to create a top-level community."`
	Description     string `json:"description,omitempty" jsonschema:"Optional. Free-text description of the community."`
	Confirm         bool   `json:"confirm,omitempty" jsonschema:"Safety checkpoint. false (the default) returns a PREVIEW of the community and writes nothing, so it can be reviewed with the user. Set true only after the user approved the preview."`
}

// Preview echoes every field that confirm=true would write.
type Preview struct {
	Name                string `json:"name"`
	Description         string `json:"description,omitempty"`
	ParentCommunityID   string `json:"parentCommunityId,omitempty"`
	ParentCommunityName string `json:"parentCommunityName,omitempty"`
}

// CommunitySummary is the created community as Collibra returned it.
type CommunitySummary struct {
	ID                string `json:"id" jsonschema:"UUID of the newly created community."`
	Name              string `json:"name"`
	Description       string `json:"description,omitempty"`
	ParentCommunityID string `json:"parentCommunityId,omitempty" jsonschema:"UUID of the parent community, when nested."`
}

// Output is the typed response.
type Output struct {
	Status    OutputStatus      `json:"status" jsonschema:"'preview' when confirm was not set (nothing written; review it, then call again with confirm=true); 'success' when the community was created; 'validation_error' for invalid or unresolvable inputs; 'error' when Collibra rejected the write."`
	Message   string            `json:"message" jsonschema:"Human-readable summary."`
	Preview   *Preview          `json:"preview,omitempty" jsonschema:"The community that would be created, when status is 'preview'."`
	Community *CommunitySummary `json:"community,omitempty" jsonschema:"The created community, when status is 'success'."`
}

// NewTool returns the registered tool.
func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:  "create_community",
		Title: "Create Community",
		Description: "Create a new Collibra community — the top-level organizational container that groups domains (a domain being a container for assets), " +
			"for example 'Finance' or 'Employee Wellness'. A community can be nested under another community by passing parentCommunity, " +
			"which accepts the parent's UUID or its exact display name, so there is no need to look the UUID up first. " +
			"Use this to create a community; to create a domain inside a community use create_domain, and to create assets use create_asset. " +
			"The name must be unique across all communities on the instance. " +
			"Built around a confirm checkpoint: confirm=false (the default) returns a PREVIEW of exactly what would be created and writes nothing — show it to the user; " +
			"call again with confirm=true to create it. " +
			"Returns the new community's id on success. " +
			"Requires permission to create communities (typically an administrator action); without it the write fails with a permission error. " +
			"Example requests: 'create a community called Employee Wellness', 'add a sub-community Payroll under Finance', 'set up a new community for the marketing team'.",
		Handler:     handler(collibraClient),
		Permissions: []string{},
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: false, DestructiveHint: chip.Ptr(false), IdempotentHint: false, OpenWorldHint: chip.Ptr(false)},
	}
}

func handler(collibraClient *http.Client) chip.ToolHandlerFunc[Input, Output] {
	return func(ctx context.Context, input Input) (Output, error) {
		name := strings.TrimSpace(input.Name)
		if msg := validateName(name); msg != "" {
			return Output{Status: StatusValidationError, Message: msg}, nil
		}

		var parent resolve.Ref
		if raw := strings.TrimSpace(input.ParentCommunity); raw != "" {
			var err error
			parent, err = resolve.Community(ctx, collibraClient, "parentCommunity", raw)
			if err != nil {
				return Output{Status: StatusValidationError, Message: fmt.Sprintf("parentCommunity could not be resolved: %v", err)}, nil
			}
		}

		if !input.Confirm {
			return previewOutput(name, input.Description, parent), nil
		}

		resp, err := clients.CreateCommunity(ctx, collibraClient, clients.CreateCommunityRequest{
			Name:        name,
			ParentID:    parent.ID,
			Description: input.Description,
		})
		if err != nil {
			return Output{Status: StatusError, Message: fmt.Sprintf("Could not create community: %v", err)}, nil
		}
		return successOutput(resp, parent), nil
	}
}

func validateName(name string) string {
	switch {
	case name == "":
		return "name is required: provide a community name of 1-255 characters."
	case utf8.RuneCountInString(name) > maxNameLength:
		return fmt.Sprintf("name is %d characters long; Collibra allows at most %d. Shorten it.", utf8.RuneCountInString(name), maxNameLength)
	}
	return ""
}

func previewOutput(name, description string, parent resolve.Ref) Output {
	return Output{
		Status: StatusPreview,
		Message: fmt.Sprintf("Preview only — nothing was created. Will create community %q%s. "+
			"Review this with the user, then call again with confirm=true.", name, nestingSuffix(parent)),
		Preview: &Preview{
			Name:                name,
			Description:         description,
			ParentCommunityID:   parent.ID,
			ParentCommunityName: parent.Name,
		},
	}
}

func successOutput(resp *clients.CreateCommunityResponse, parent resolve.Ref) Output {
	summary := &CommunitySummary{ID: resp.ID, Name: resp.Name, Description: resp.Description}
	if resp.Parent != nil {
		summary.ParentCommunityID = resp.Parent.ID
		parent.Name = resp.Parent.Name
	}
	return Output{
		Status:    StatusSuccess,
		Message:   fmt.Sprintf("Created community %q (id %s)%s.", resp.Name, resp.ID, nestingSuffix(parent)),
		Community: summary,
	}
}

func nestingSuffix(parent resolve.Ref) string {
	switch {
	case parent.Name != "":
		return fmt.Sprintf(" under parent community %q", parent.Name)
	case parent.ID != "":
		return fmt.Sprintf(" under parent community %s", parent.ID)
	}
	return " as a top-level community"
}
