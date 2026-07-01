package clients

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// DQ job notifications. Unlike rowFilter/sampleSetting (which are persisted-but-not-applied on the
// scan), the structured `notifications` field is APPLIED: the server's JobNotificationsMapper turns
// each NotificationOption into a real AlertCond (verified). So this is a clean structured feature —
// no SQL/dialect involvement. Contract: JobNotifications/NotificationOption in ui-v1-private-oas-spec.yaml.

// DqNotificationOption is one notification rule (→ one AlertCond server-side).
type DqNotificationOption struct {
	NotificationType string `json:"notificationType"`
	Enabled          bool   `json:"enabled"`
	Message          string `json:"message,omitempty"`
	Quantity         int    `json:"quantity,omitempty"`
}

// DqJobNotifications is the public `notifications` object (dq-v1-public-oas-spec.yaml JobNotifications).
// Recipients are delivered via channels (currently EMAIL) carrying platform USERNAMES — not UUIDs. The
// public schema requires both notificationOptions and channels (>=1) when notifications are configured.
type DqJobNotifications struct {
	NotificationOptions   []DqNotificationOption  `json:"notificationOptions"`
	GlobalMessage         string                  `json:"globalMessage,omitempty"`
	UseIndividualMessages bool                    `json:"useIndividualMessages"`
	Channels              []DqNotificationChannel `json:"channels"`
}

// DqNotificationChannel is one delivery channel and its recipients. Channel is EMAIL today; recipients
// are platform usernames.
type DqNotificationChannel struct {
	Channel    string   `json:"channel"`
	Recipients []string `json:"recipients"`
}

// DqNotificationInfo describes one selectable notification for display/selection.
type DqNotificationInfo struct {
	Key              string `json:"key"`
	Label            string `json:"label"`
	NotificationType string `json:"notificationType"`
	DefaultEnabled   bool   `json:"defaultEnabled"`
	TakesQuantity    bool   `json:"takesQuantity"`
	DefaultQuantity  int    `json:"defaultQuantity,omitempty"`
}

// DqNotificationCatalog is the selectable notifications with wizard defaults (Step Notifications):
// Job failed, Rows<=, Score<=, Run time> ON; Job completed, Runs/Days without data OFF.
func DqNotificationCatalog() []DqNotificationInfo {
	return []DqNotificationInfo{
		{Key: "jobFailed", Label: "Job failed", NotificationType: "JOB_FAILED", DefaultEnabled: true},
		{Key: "rowsBelow", Label: "Rows below limit", NotificationType: "ROWS_LESS_THAN_LIMIT", DefaultEnabled: true, TakesQuantity: true, DefaultQuantity: 1},
		{Key: "scoreBelow", Label: "Score below limit", NotificationType: "SCORE_LESS_THAN_LIMIT", DefaultEnabled: true, TakesQuantity: true, DefaultQuantity: 75},
		{Key: "runTimeAbove", Label: "Run time above (minutes)", NotificationType: "RUN_TIME_MORE_THEN_LIMIT", DefaultEnabled: true, TakesQuantity: true, DefaultQuantity: 60},
		{Key: "jobCompleted", Label: "Job completed", NotificationType: "JOB_COMPLETED", DefaultEnabled: false},
		{Key: "runsWithoutData", Label: "Runs without data", NotificationType: "RUNS_WITHOUT_DATA", DefaultEnabled: false, TakesQuantity: true, DefaultQuantity: 1},
		{Key: "daysWithoutData", Label: "Days without data", NotificationType: "DAYS_WITHOUT_DATA", DefaultEnabled: false, TakesQuantity: true, DefaultQuantity: 1},
	}
}

// NotificationKeys returns every valid notification key, in catalog order.
func NotificationKeys() []string {
	cat := DqNotificationCatalog()
	keys := make([]string, 0, len(cat))
	for _, n := range cat {
		keys = append(keys, n.Key)
	}
	return keys
}

// DefaultNotificationKeys returns the keys enabled by default.
func DefaultNotificationKeys() []string {
	var keys []string
	for _, n := range DqNotificationCatalog() {
		if n.DefaultEnabled {
			keys = append(keys, n.Key)
		}
	}
	return keys
}

// BuildNotificationOptions turns enabled keys (case-insensitive) into NotificationOptions. quantities
// overrides a key's threshold (keyed by lower-case key; <=0 uses the catalog default). messages sets a
// per-notification message (keyed by lower-case key) that overrides the global message for that one
// notification — pass nil for none. Unknown keys are returned so the caller can reject them.
func BuildNotificationOptions(enabledKeys []string, quantities map[string]int, messages map[string]string) ([]DqNotificationOption, []string) {
	valid := map[string]DqNotificationInfo{}
	for _, n := range DqNotificationCatalog() {
		valid[strings.ToLower(n.Key)] = n
	}
	var opts []DqNotificationOption
	var unknown []string
	for _, k := range enabledKeys {
		lk := strings.ToLower(strings.TrimSpace(k))
		if lk == "" {
			continue
		}
		info, ok := valid[lk]
		if !ok {
			unknown = append(unknown, k)
			continue
		}
		opt := DqNotificationOption{NotificationType: info.NotificationType, Enabled: true}
		if info.TakesQuantity {
			q := quantities[lk]
			if q <= 0 {
				q = info.DefaultQuantity
			}
			opt.Quantity = q
		}
		if m := strings.TrimSpace(messages[lk]); m != "" {
			opt.Message = m
		}
		opts = append(opts, opt)
	}
	return opts, unknown
}

// RecipientResolution is the outcome of resolving notification recipients to active users. UserIDs and
// Usernames are positionally aligned (same resolved user); Usernames feed the public notification
// channels (which take usernames), UserIDs are kept for callers that need the UUID.
type RecipientResolution struct {
	UserIDs    []string
	Usernames  []string
	Unresolved []string
}

// ResolveNotificationRecipients resolves each username/email to an active user's UUID. An entry
// containing '@' is looked up by email, otherwise by username. Not-found or disabled accounts land
// in Unresolved (the list endpoint excludes disabled users and the email lookup 404s), so the
// caller can warn and decide whether to proceed without them. Duplicates are de-duped.
func ResolveNotificationRecipients(ctx context.Context, client *http.Client, recipients []string) (RecipientResolution, error) {
	var res RecipientResolution
	seen := map[string]bool{}
	for _, r := range recipients {
		r = strings.TrimSpace(r)
		if r == "" {
			continue
		}
		var (
			u   *EditAssetUser
			err error
		)
		if strings.Contains(r, "@") {
			u, err = FindUserByEmail(ctx, client, r)
		} else {
			u, err = FindUserByUsername(ctx, client, r)
		}
		if err != nil {
			return res, err
		}
		if u == nil || u.ID == "" {
			res.Unresolved = append(res.Unresolved, r)
			continue
		}
		if !seen[u.ID] {
			seen[u.ID] = true
			res.UserIDs = append(res.UserIDs, u.ID)
			username := strings.TrimSpace(u.UserName)
			if username == "" {
				username = r // fall back to what the caller provided if the account has no username
			}
			res.Usernames = append(res.Usernames, username)
		}
	}
	return res, nil
}

// GetCurrentUser returns the invoking user (GET /rest/2.0/users/current) — the default notification
// recipient.
func GetCurrentUser(ctx context.Context, client *http.Client) (*EditAssetUser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "/rest/2.0/users/current", nil)
	if err != nil {
		return nil, fmt.Errorf("get current user: building request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("get current user: sending request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("get current user: reading response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("get current user: status %d: %s", resp.StatusCode, string(body))
	}
	var user EditAssetUser
	if err := json.Unmarshal(body, &user); err != nil {
		return nil, fmt.Errorf("get current user: decoding response: %w", err)
	}
	return &user, nil
}
