package outcome_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/collibra/chip/pkg/tools/outcome"
)

func TestDescribe_KeepsDGCsWordsAndPicksTheNextStepFromTheHTTPStatus(t *testing.T) {
	for _, tc := range []struct {
		err, wantGuidance string
	}{
		{"HTTP 400 [badRequest]: Name is required", "rejected the request"},
		{"HTTP 403 [permissionDenied]: Not allowed", "not allowed"},
		{"HTTP 404 [notFound]: User group not found.", "does not know"},
		{"HTTP 409 [conflict]: Name taken", "conflict"},
		{"HTTP 500: boom", "could not carry out"},
		{"failed to make request: connection refused", "Check the connection"},
	} {
		message, guidance := outcome.Describe("create the group", errors.New(tc.err))
		if !strings.Contains(message, tc.err) || !strings.HasPrefix(message, "Could not create the group: ") {
			t.Errorf("%q: the message must keep DGC's words, got %q", tc.err, message)
		}
		if !strings.Contains(guidance, tc.wantGuidance) {
			t.Errorf("%q: expected guidance containing %q, got %q", tc.err, tc.wantGuidance, guidance)
		}
	}
}

func TestChunks_SplitsWithoutLosingOrder(t *testing.T) {
	chunks := outcome.Chunks([]string{"a", "b", "c", "d", "e"}, 2)
	if len(chunks) != 3 || len(chunks[0]) != 2 || len(chunks[2]) != 1 || chunks[2][0] != "e" {
		t.Fatalf("unexpected chunks: %v", chunks)
	}
	if got := outcome.Chunks(nil, 2); len(got) != 0 {
		t.Fatalf("expected no chunks for no values, got %v", got)
	}
}
