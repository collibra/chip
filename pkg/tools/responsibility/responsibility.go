// Package responsibility holds the role-assignment logic shared by the edit
// tools for resources that carry responsibilities (domains, communities):
// assigning a user or group to a resource role, and removing that assignment.
// It reuses Collibra's generic /responsibilities endpoints, which take the
// resource type ("Domain", "Community", ...) alongside the resource id.
package responsibility

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/collibra/chip/pkg/clients"
	"github.com/collibra/chip/pkg/tools/resolve"
)

// Resource types accepted by the responsibilities API.
const (
	ResourceDomain    = "Domain"
	ResourceCommunity = "Community"
)

// Target identifies the resource whose responsibilities are being edited.
type Target struct {
	Type string
	ID   string
}

// Check validates the inputs shared by set and remove, without any request.
func Check(role, userID string) error {
	if strings.TrimSpace(role) == "" {
		return fmt.Errorf("role is required (e.g. 'Steward', 'Owner')")
	}
	if strings.TrimSpace(userID) == "" {
		return fmt.Errorf(`userId is required (a UUID, email address, username, or full name such as "Jane Smith")`)
	}
	return nil
}

// Set assigns the user (or group, by UUID) to the named role on the target and
// returns the new responsibility's id.
func Set(ctx context.Context, client *http.Client, target Target, role, userID string) (string, error) {
	roleID, err := resolveRole(ctx, client, role)
	if err != nil {
		return "", err
	}
	ownerID, err := resolve.UserID(ctx, client, userID, resolve.Hints{})
	if err != nil {
		return "", err
	}
	created, err := clients.CreateResponsibility(ctx, client, clients.EditAssetCreateResponsibilityRequest{
		RoleID:       roleID,
		OwnerID:      ownerID,
		ResourceID:   target.ID,
		ResourceType: target.Type,
	})
	if err != nil {
		return "", err
	}
	return created.ID, nil
}

// Remove deletes the responsibility of the given role and user defined
// directly on the target, returning its id. A responsibility inherited from a
// parent community is reported as such rather than deleted.
func Remove(ctx context.Context, client *http.Client, target Target, role, userID string) (string, error) {
	roleID, err := resolveRole(ctx, client, role)
	if err != nil {
		return "", err
	}
	ownerID, err := resolve.UserID(ctx, client, userID, resolve.Hints{})
	if err != nil {
		return "", err
	}
	responsibilities, err := clients.GetResponsibilities(ctx, client, target.ID)
	if err != nil {
		return "", fmt.Errorf("looking up responsibilities: %w", err)
	}

	matchID, inheritedOnly := findDirect(responsibilities, target.ID, roleID, ownerID)
	if matchID == "" {
		if inheritedOnly {
			return "", fmt.Errorf("the %q responsibility for %q is inherited from a parent community and cannot be removed from this resource; remove it where it is defined", role, userID)
		}
		return "", fmt.Errorf("no %q responsibility found for %q on this resource", role, userID)
	}
	if err := clients.DeleteResponsibility(ctx, client, matchID); err != nil {
		return "", err
	}
	return matchID, nil
}

func findDirect(all []clients.Responsibility, resourceID, roleID, ownerID string) (id string, inheritedOnly bool) {
	for _, r := range all {
		if r.Role == nil || r.Role.ID != roleID || r.Owner == nil || r.Owner.ID != ownerID {
			continue
		}
		if r.BaseResource != nil && r.BaseResource.ID == resourceID {
			return r.ID, false
		}
		inheritedOnly = true
	}
	return "", inheritedOnly
}

func resolveRole(ctx context.Context, client *http.Client, name string) (string, error) {
	roles, err := clients.ListRoles(ctx, client)
	if err != nil {
		return "", fmt.Errorf("listing roles: %w", err)
	}
	candidates := make([]resolve.NamedRef, len(roles))
	for i, r := range roles {
		candidates[i] = resolve.NamedRef{ID: r.ID, Name: r.Name}
	}
	return resolve.PickMatch("role", name, candidates, resolve.Hints{Candidates: "Roles"})
}
