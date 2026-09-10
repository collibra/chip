package resolve

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/collibra/chip/pkg/clients"
	"github.com/google/uuid"
)

// User is a resolved Collibra user. Only fields that are safe to show back to
// the model are carried: the UUID the APIs need, plus the username and display
// name that let a human confirm the right person was picked. The email address
// is deliberately absent — no CHIP tool returns one.
type User struct {
	ID       string
	UserName string
	FullName string
}

// Hint wording for user resolution. Both are overridable per caller via Hints;
// these apply when the caller has nothing more specific to say.
const (
	userNotFoundHint  = `Accepted forms: the user's UUID, their email address, their username, or their full name as "First Last".`
	userAmbiguityHint = "do NOT pick one — ask which person is meant, then call again with that user's username or UUID"
)

// UserID resolves a user reference to the user's UUID. See UserRef for the
// accepted forms and the order they are tried in.
func UserID(ctx context.Context, client *http.Client, value string, hints Hints) (string, error) {
	user, err := UserRef(ctx, client, value, hints)
	return user.ID, err
}

// UserRef resolves a user reference — a UUID, an email address, a username, or
// a display name such as "Jane Smith" — to that user, in that order of
// precedence. A UUID is returned as-is without any lookup; a value containing
// '@' goes to the exact email endpoint; otherwise one name search is made and
// an exact username wins over a display-name match, so a person whose username
// happens to be someone else's name still resolves to themselves.
//
// A display name matching several enabled users is an error listing each
// candidate with its UUID, never an arbitrary pick: this runs mid-chain to fill
// another call's parameter, and a wrong id writes to the wrong person.
func UserRef(ctx context.Context, client *http.Client, value string, hints Hints) (User, error) {
	v := strings.TrimSpace(value)
	if v == "" {
		return User{}, fmt.Errorf("a user is required. %s", userHints(hints).NotFound)
	}
	if _, err := uuid.Parse(v); err == nil {
		return User{ID: v}, nil
	}
	if strings.Contains(v, "@") {
		return userByEmail(ctx, client, v, hints)
	}
	return userByName(ctx, client, v, hints)
}

// userByEmail resolves an email address through the dedicated exact-match
// endpoint. The list endpoint has no email filter — it silently ignores an
// unknown `emailAddress` param and would return an arbitrary user.
func userByEmail(ctx context.Context, client *http.Client, email string, hints Hints) (User, error) {
	user, err := clients.FindUserByEmail(ctx, client, email)
	if err != nil {
		return User{}, fmt.Errorf("looking up user by email: %w", err)
	}
	if user == nil {
		return User{}, notFoundError("user", email, nil, userHints(hints))
	}
	return toUser(*user), nil
}

// userByName resolves a username or a display name from a single name search:
// an exact username first, then the display-name candidates reduced by
// PickMatch. The search itself is a partial match, so a candidate list is
// exactly what PickMatch expects.
func userByName(ctx context.Context, client *http.Client, name string, hints Hints) (User, error) {
	users, err := clients.FindUsersByName(ctx, client, name)
	if err != nil {
		return User{}, fmt.Errorf("looking up user %q: %w", name, err)
	}
	for _, u := range users {
		if strings.EqualFold(u.UserName, name) {
			return toUser(u), nil
		}
	}

	byID := make(map[string]clients.EditAssetUser, len(users))
	candidates := make([]NamedRef, 0, len(users))
	for _, u := range users {
		full := u.FullName()
		if full == "" {
			continue // a service account with no first/last name has no display name to match
		}
		ref := NamedRef{ID: u.ID, Name: full}
		if u.UserName != "" {
			ref.Ctx = "username: " + u.UserName
		}
		candidates = append(candidates, ref)
		byID[u.ID] = u
	}
	id, err := PickMatch("user", name, candidates, userHints(hints))
	if err != nil {
		return User{}, err
	}
	return toUser(byID[id]), nil
}

// userHints fills in the user-specific wording for whatever the caller left
// blank.
func userHints(hints Hints) Hints {
	if hints.NotFound == "" {
		hints.NotFound = userNotFoundHint
	}
	if hints.Ambiguity == "" {
		hints.Ambiguity = userAmbiguityHint
	}
	return hints
}

// toUser narrows a client user record to the fields CHIP surfaces, dropping the
// email address.
func toUser(u clients.EditAssetUser) User {
	return User{ID: u.ID, UserName: u.UserName, FullName: u.FullName()}
}
