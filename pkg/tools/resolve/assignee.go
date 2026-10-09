package resolve

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/collibra/chip/pkg/clients"
	"github.com/collibra/chip/pkg/tools/validation"
)

// AssigneeRef is an assignee as a caller gave it: a USER as a UUID, email
// address, username or full name, or a GROUP as its UUID.
type AssigneeRef struct {
	ID   string
	Type string
}

// CheckAssignees validates everything about an assignee list that needs no
// request: each type is USER or GROUP, and each GROUP id is a UUID, since group
// names are not resolvable through the user lookup. It returns the list with
// types normalised and USER ids still as given, ready for ResolveAssignees.
//
// Callers run it before any user lookup, so a malformed entry anywhere in the
// list is reported without spending a request.
func CheckAssignees(refs []AssigneeRef) ([]clients.Assignee, error) {
	if len(refs) == 0 {
		return nil, nil
	}
	out := make([]clients.Assignee, len(refs))
	for i, a := range refs {
		t := strings.ToUpper(strings.TrimSpace(a.Type))
		switch t {
		case "USER":
		case "GROUP":
			if err := validation.UUID(fmt.Sprintf("assignees[%d].id", i), a.ID); err != nil {
				return nil, fmt.Errorf("%w (a GROUP assignee must be given as its UUID)", err)
			}
		default:
			return nil, fmt.Errorf("assignees[%d].type must be USER or GROUP, got %q", i, a.Type)
		}
		out[i] = clients.Assignee{ID: a.ID, Type: t}
	}
	return out, nil
}

// ResolveAssignees replaces each USER id in a list returned by CheckAssignees
// with that user's UUID. A UUID passes through with no request; an ambiguous
// name is an error naming every candidate rather than an assignment of a
// guessed person.
func ResolveAssignees(ctx context.Context, client *http.Client, checked []clients.Assignee) ([]clients.Assignee, error) {
	if checked == nil {
		return nil, nil
	}
	out := make([]clients.Assignee, len(checked))
	for i, a := range checked {
		out[i] = a
		if a.Type != "USER" {
			continue
		}
		id, err := UserID(ctx, client, a.ID, Hints{})
		if err != nil {
			return nil, fmt.Errorf("assignees[%d]: %w", i, err)
		}
		out[i].ID = id
	}
	return out, nil
}
