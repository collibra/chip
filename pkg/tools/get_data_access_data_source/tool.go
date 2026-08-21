package get_data_access_data_source

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
	sdktypes "github.com/collibra/data-access-go-sdk/types"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type Input struct {
	DataSourceID string `json:"dataSourceId" jsonschema:"Required. ID of the data source to fetch. Data source IDs are returned as dataSourceId on data objects from search_data_access_objects and check_user_data_object_access."`
}

type Output struct {
	DataSource *clients.DataAccessDataSource `json:"dataSource,omitempty" jsonschema:"The resolved data source: its id, name, type, description, parent id, and timestamps."`
	Message    string                        `json:"message,omitempty" jsonschema:"Guidance for the agent, set when the data source ID could not be resolved — ask the user to correct it."`
	Error      string                        `json:"error,omitempty" jsonschema:"Error message if the data source could not be retrieved."`
}

func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:        "get_data_access_data_source",
		Title:       "Get Data Access Data Source",
		Description: "Fetches a Collibra Data Access data source by its ID, returning its name, type, description, parent, and timestamps. Use this to resolve a data source ID (the dataSourceId carried by data objects from search_data_access_objects and check_user_data_object_access) to a human-readable data source name and type. If the ID does not correspond to an existing data source, a message asking the user to correct it is returned.",
		Handler:     handle(collibraClient),
		Permissions: []string{},
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, DestructiveHint: new(false), IdempotentHint: true, OpenWorldHint: new(false)},
	}
}

func handle(collibraClient *http.Client) chip.ToolHandlerFunc[Input, Output] {
	return func(ctx context.Context, input Input) (Output, error) {
		id := strings.TrimSpace(input.DataSourceID)
		if id == "" {
			return Output{Error: "a data source ID is required"}, nil
		}

		dataSource, err := clients.GetDataAccessDataSource(ctx, collibraClient, id)
		if err != nil {
			var notFound *sdktypes.ErrNotFound
			if errors.As(err, &notFound) {
				return Output{Message: fmt.Sprintf("No data source found with ID %q. Ask the user to correct it, then call again.", id)}, nil
			}
			return Output{Error: fmt.Sprintf("Failed to fetch data source: %s", err.Error())}, nil
		}

		return Output{DataSource: dataSource}, nil
	}
}
