package clients

import (
	"net/url"
	"strings"
	"testing"
)

func TestBuildNotificationOptionsDefaultsAndQuantities(t *testing.T) {
	opts, unknown := BuildNotificationOptions(
		[]string{"jobFailed", "scoreBelow", "rowsBelow"},
		map[string]int{"rowsbelow": 500}, // override rowsBelow; scoreBelow uses its default
		nil,
	)
	if len(unknown) != 0 {
		t.Fatalf("unexpected unknown keys: %v", unknown)
	}
	byType := map[string]DqNotificationOption{}
	for _, o := range opts {
		byType[o.NotificationType] = o
	}
	if o, ok := byType["JOB_FAILED"]; !ok || !o.Enabled || o.Quantity != 0 {
		t.Errorf("jobFailed wrong: %+v", o)
	}
	if o := byType["SCORE_LESS_THAN_LIMIT"]; o.Quantity != 75 {
		t.Errorf("expected scoreBelow default quantity 75, got %d", o.Quantity)
	}
	if o := byType["ROWS_LESS_THAN_LIMIT"]; o.Quantity != 500 {
		t.Errorf("expected rowsBelow override quantity 500, got %d", o.Quantity)
	}
}

func TestBuildNotificationOptionsReportsUnknown(t *testing.T) {
	_, unknown := BuildNotificationOptions([]string{"jobFailed", "bogus"}, nil, nil)
	if len(unknown) != 1 || unknown[0] != "bogus" {
		t.Fatalf("expected unknown=[bogus], got %v", unknown)
	}
}

func TestBuildNotificationOptionsPerTypeMessages(t *testing.T) {
	opts, unknown := BuildNotificationOptions(
		[]string{"jobFailed", "scoreBelow"},
		nil,
		map[string]string{"jobfailed": "ping me"}, // per-type message overrides global for jobFailed only
	)
	if len(unknown) != 0 {
		t.Fatalf("unexpected unknown keys: %v", unknown)
	}
	byType := map[string]DqNotificationOption{}
	for _, o := range opts {
		byType[o.NotificationType] = o
	}
	if o := byType["JOB_FAILED"]; o.Message != "ping me" {
		t.Errorf("expected jobFailed message 'ping me', got %q", o.Message)
	}
	if o := byType["SCORE_LESS_THAN_LIMIT"]; o.Message != "" {
		t.Errorf("expected scoreBelow to have no per-type message, got %q", o.Message)
	}
}

func TestDefaultNotificationKeysMatchCatalog(t *testing.T) {
	defaults := map[string]bool{}
	for _, k := range DefaultNotificationKeys() {
		defaults[k] = true
	}
	for _, n := range DqNotificationCatalog() {
		if n.DefaultEnabled != defaults[n.Key] {
			t.Errorf("%s: DefaultEnabled=%v but membership=%v", n.Key, n.DefaultEnabled, defaults[n.Key])
		}
	}
}

// A recipient given as a person's name resolves like a username does; a name
// nobody (or more than one person) answers to still lands in Unresolved, so
// the caller can warn instead of notifying a guessed account.
func TestResolveNotificationRecipientsAcceptsFullNames(t *testing.T) {
	var queries []url.Values
	client := userSearchServer(t, &queries,
		EditAssetUser{ID: "u-1", UserName: "jane.smith", FirstName: "Jane", LastName: "Smith"},
		EditAssetUser{ID: "u-2", UserName: "bjones", FirstName: "Bob", LastName: "Jones"},
		EditAssetUser{ID: "u-3", UserName: "bjones2", FirstName: "Bob", LastName: "Jones"})

	res, err := ResolveNotificationRecipients(t.Context(), client,
		[]string{"Jane Smith", "bjones", "Bob Jones", "Nobody Here"})
	if err != nil {
		t.Fatalf("ResolveNotificationRecipients: %v", err)
	}
	if len(res.UserIDs) != 2 || res.UserIDs[0] != "u-1" || res.UserIDs[1] != "u-2" {
		t.Fatalf("expected [u-1 u-2], got %v", res.UserIDs)
	}
	if len(res.Usernames) != 2 || res.Usernames[0] != "jane.smith" || res.Usernames[1] != "bjones" {
		t.Fatalf("expected [jane.smith bjones], got %v", res.Usernames)
	}
	// "Bob Jones" is shared by two accounts, "Nobody Here" matches none.
	if len(res.Unresolved) != 2 || res.Unresolved[0] != "Bob Jones" || res.Unresolved[1] != "Nobody Here" {
		t.Fatalf("expected [Bob Jones, Nobody Here] unresolved, got %v", res.Unresolved)
	}
	// The two cases need different advice, so only the shared name is ambiguous.
	if len(res.Ambiguous) != 1 || res.Ambiguous[0] != "Bob Jones" {
		t.Fatalf("expected [Bob Jones] ambiguous, got %v", res.Ambiguous)
	}
}

// The two failure modes need different advice, so the message keeps them apart.
func TestUnresolvedRecipientsMessageSeparatesAmbiguousFromMissing(t *testing.T) {
	msg := UnresolvedRecipientsMessage(RecipientResolution{
		Unresolved: []string{"Bob Jones", "Nobody Here"},
		Ambiguous:  []string{"Bob Jones"},
	})
	if !strings.Contains(msg, "no active Collibra account: Nobody Here.") {
		t.Fatalf("message does not report the missing recipient on its own: %q", msg)
	}
	if !strings.Contains(msg, "more than one active account") || !strings.Contains(msg, "Bob Jones") {
		t.Fatalf("message does not report the shared name as shared: %q", msg)
	}
	if got := UnresolvedRecipientsMessage(RecipientResolution{}); got != "" {
		t.Fatalf("an all-resolved resolution must render an empty message, got %q", got)
	}
}

// A truncated recipient search knows nothing about the accounts it never
// returned, so it must be reported as "could not be pinned down", never as
// "no active Collibra account" — the same rule the write paths follow.
func TestResolveNotificationRecipientsTreatsTruncatedSearchAsAmbiguous(t *testing.T) {
	var queries []url.Values
	client := userSearchServerWithTotal(t, &queries, 250,
		EditAssetUser{ID: "u-1", UserName: "jsmith", FirstName: "Janet", LastName: "Smithers"})

	res, err := ResolveNotificationRecipients(t.Context(), client, []string{"Jane Smith"})
	if err != nil {
		t.Fatalf("ResolveNotificationRecipients: %v", err)
	}
	if len(res.UserIDs) != 0 {
		t.Fatalf("expected nobody bound from a truncated search, got %v", res.UserIDs)
	}
	if len(res.Unresolved) != 1 || res.Unresolved[0] != "Jane Smith" {
		t.Fatalf("expected [Jane Smith] unresolved, got %v", res.Unresolved)
	}
	if len(res.Ambiguous) != 1 || res.Ambiguous[0] != "Jane Smith" {
		t.Fatalf("expected [Jane Smith] ambiguous, got %v", res.Ambiguous)
	}
	if msg := UnresolvedRecipientsMessage(res); strings.Contains(msg, "no active Collibra account") {
		t.Fatalf("a truncated search must not be reported as no such account: %q", msg)
	}
}

// The companion case: an exact username still resolves over a truncated page.
func TestResolveNotificationRecipientsResolvesUsernameOverTruncatedSearch(t *testing.T) {
	var queries []url.Values
	client := userSearchServerWithTotal(t, &queries, 250,
		EditAssetUser{ID: "u-1", UserName: "jsmith", FirstName: "Janet", LastName: "Smithers"})

	res, err := ResolveNotificationRecipients(t.Context(), client, []string{"jsmith"})
	if err != nil {
		t.Fatalf("ResolveNotificationRecipients: %v", err)
	}
	if len(res.UserIDs) != 1 || res.UserIDs[0] != "u-1" {
		t.Fatalf("expected [u-1] resolved, got %v (unresolved=%v)", res.UserIDs, res.Unresolved)
	}
	if len(res.Unresolved) != 0 || len(res.Ambiguous) != 0 {
		t.Fatalf("expected nothing unresolved, got unresolved=%v ambiguous=%v", res.Unresolved, res.Ambiguous)
	}
}
