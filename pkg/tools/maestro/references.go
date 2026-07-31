// Package maestro holds the input checks the Maestro Agent write tools share.
// create_maestro_agent and edit_maestro_agent accept the same nested blocks and
// have to judge them identically, so the rules live here rather than in either
// tool.
package maestro

import (
	"github.com/collibra/chip/pkg/clients"
	"github.com/collibra/chip/pkg/tools/validation"
)

// ValidateReferences checks the UUID-shaped references in an agent's sharing and
// ownership blocks. A malformed UUID otherwise reaches the API as an opaque 400,
// so it is worth catching locally, naming the field that is wrong.
//
// Everything else is left to the server, which owns those rules and reports them
// with an actionable error code: reserved handles and colors, length and count
// caps, unsupported tool names, and whether the referenced users, groups and roles
// exist at all.
func ValidateReferences(sharing *clients.MaestroSharing, ownership *clients.MaestroOwnership) error {
	if sharing != nil {
		if err := validation.UUIDs("sharing.roles", sharing.Roles); err != nil {
			return err
		}
		if err := validation.UUIDs("sharing.groups", sharing.Groups); err != nil {
			return err
		}
		if err := validation.UUIDs("sharing.users", sharing.Users); err != nil {
			return err
		}
	}
	if ownership != nil {
		if err := validation.UUIDs("ownership.users", ownership.Users); err != nil {
			return err
		}
	}
	return nil
}
