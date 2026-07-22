package check_user_data_object_access

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type Input struct {
	ObjectIds []string `json:"objectIds" jsonschema:"Required. One or more data object IDs (database, schema, table, view, column, etc.) to check access for. Obtain IDs via search_data_access_objects."`
	UserID    string   `json:"userId,omitempty" jsonschema:"Optional. ID of the user to check access for. Resolve names/emails to a user ID via search_data_access_identities. When omitted along with email, the current user is used."`
	Email     string   `json:"email,omitempty" jsonschema:"Optional. Email of the user to check access for, used when userId is not supplied. When omitted along with userId, the current user is used."`
}

type Output struct {
	Result  *clients.CheckUserDataObjectAccessResult `json:"result,omitempty" jsonschema:"The access check result: the resolved user, per-object access (with granting roles), and any IDs that could not be resolved."`
	Message string                                   `json:"message,omitempty" jsonschema:"Guidance for the agent, set when one or more IDs could not be resolved — ask the user to correct or drop them."`
	Error   string                                   `json:"error,omitempty" jsonschema:"Error message if the access check could not be completed."`
}

func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:        "check_user_data_object_access",
		Description: "Checks if a user has access to a data object (database, schema, table, view, column, etc.) and through which access controls (ie roles). Takes one or more data object IDs (obtain them via search_data_access_objects). For every object it reports whether the user has access, the granted permissions, and the access controls (roles) that grant the access. Checks the current user unless userId or email is supplied (resolve names via search_data_access_identities). IDs that do not correspond to an existing data object are returned in result.unresolved with a message asking the user to correct or drop them.",
		Handler:     handle(collibraClient),
		Permissions: []string{},
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, DestructiveHint: new(false)},
	}
}

func handle(collibraClient *http.Client) chip.ToolHandlerFunc[Input, Output] {
	return func(ctx context.Context, input Input) (Output, error) {
		ids := make([]string, 0, len(input.ObjectIds))
		for _, id := range input.ObjectIds {
			if strings.TrimSpace(id) != "" {
				ids = append(ids, id)
			}
		}
		if len(ids) == 0 {
			return Output{Error: "at least one object ID is required"}, nil
		}

		result, err := clients.CheckUserDataObjectAccess(ctx, collibraClient, ids, input.UserID, input.Email)
		if err != nil {
			return Output{Error: fmt.Sprintf("Failed to check data object access: %s", err.Error())}, nil
		}

		out := Output{Result: result}
		if len(result.Unresolved) > 0 {
			unresolved := make([]string, 0, len(result.Unresolved))
			for _, u := range result.Unresolved {
				unresolved = append(unresolved, fmt.Sprintf("%q (%s)", u.ID, u.Reason))
			}
			out.Message = fmt.Sprintf("Could not resolve %d ID(s) to a data object: %s. Ask the user to correct or drop them, then call again.", len(result.Unresolved), strings.Join(unresolved, ", "))
		}
		return out, nil
	}
}
