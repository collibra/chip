package clients

import "testing"

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
