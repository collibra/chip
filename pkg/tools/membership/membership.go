// Package membership answers, for the users a caller names, which of them exist and which are
// already in a given user group. The tools that add users to a group and remove them from it both
// need that before they write anything.
package membership

import (
	"context"
	"net/http"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
	"github.com/collibra/chip/pkg/tools/outcome"
)

// batchSize is how many user ids go in one request: DGC takes them as repeated query parameters, and a
// longer URL risks a refusal.
const batchSize = 50

// Lookup is what DGC knows about the named users.
type Lookup struct {
	// Found are the named users that exist, in the order they were named.
	Found []clients.DirectoryUser
	// Missing are the named ids DGC does not know.
	Missing []string
	// Members are the ids, among those named, that are in the group.
	Members map[string]bool
}

// Resolve looks up the named users and their membership of the group. Ids named twice count once.
func Resolve(ctx context.Context, client *http.Client, groupID string, userIDs []string) (*Lookup, error) {
	ids := unique(userIDs)
	byID := map[string]clients.DirectoryUser{}
	members := map[string]bool{}
	for _, batch := range outcome.Chunks(ids, batchSize) {
		paging := clients.DirectoryPaging{Limit: chip.Ptr(len(batch))}
		found, err := clients.SearchUsers(ctx, client, clients.SearchUsersRequest{UserIDs: batch, IncludeDisabled: chip.Ptr(true), DirectoryPaging: paging})
		if err != nil {
			return nil, err
		}
		for _, user := range found.Results {
			byID[user.ID] = user
		}
		inGroup, err := clients.SearchUsers(ctx, client, clients.SearchUsersRequest{UserIDs: batch, GroupID: groupID, IncludeDisabled: chip.Ptr(true), DirectoryPaging: paging})
		if err != nil {
			return nil, err
		}
		for _, user := range inGroup.Results {
			members[user.ID] = true
		}
	}
	lookup := &Lookup{Members: members}
	for _, id := range ids {
		if user, ok := byID[id]; ok {
			lookup.Found = append(lookup.Found, user)
		} else {
			lookup.Missing = append(lookup.Missing, id)
		}
	}
	return lookup, nil
}

// NotInGroup are the found users that are not in the group yet.
func (l *Lookup) NotInGroup() []clients.DirectoryUser {
	return l.filter(false)
}

// InGroup are the found users that are in the group.
func (l *Lookup) InGroup() []clients.DirectoryUser {
	return l.filter(true)
}

func (l *Lookup) filter(inGroup bool) []clients.DirectoryUser {
	var users []clients.DirectoryUser
	for _, user := range l.Found {
		if l.Members[user.ID] == inGroup {
			users = append(users, user)
		}
	}
	return users
}

// IDs are the ids of the users.
func IDs(users []clients.DirectoryUser) []string {
	ids := make([]string, len(users))
	for i, user := range users {
		ids[i] = user.ID
	}
	return ids
}

func unique(values []string) []string {
	seen := map[string]bool{}
	var result []string
	for _, value := range values {
		if !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	return result
}
