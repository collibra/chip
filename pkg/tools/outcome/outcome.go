// Package outcome holds what the user and group tools share: the statuses of a write tool's answer
// and the reading of a DGC failure into a message and the next step for the caller.
package outcome

import (
	"fmt"
	"regexp"
	"strconv"
)

// Status says what a write tool did. Each tool adds its own done status (created, deleted, ...).
type Status string

const (
	// ConfirmRequired is the answer to a call without confirm: nothing was written, and the answer
	// holds everything that would be.
	ConfirmRequired Status = "confirm_required"
	// NeedsInput is the answer to a call that cannot be carried out as given: nothing was written.
	NeedsInput Status = "needs_input"
	// Error is the answer to a call DGC refused or could not carry out.
	Error Status = "error"
)

var httpStatus = regexp.MustCompile(`^HTTP (\d{3})`)

// Describe turns a failed DGC call into a message and the next step. The message is DGC's own words;
// the next step depends on the HTTP status DGC answered with.
func Describe(action string, err error) (message, guidance string) {
	message = fmt.Sprintf("Could not %s: %s", action, err.Error())
	match := httpStatus.FindStringSubmatch(err.Error())
	if match == nil {
		return message, "Nothing is known to have changed. Check the connection and retry."
	}
	code, _ := strconv.Atoi(match[1])
	switch code {
	case 400, 422:
		return message, "DGC rejected the request as given. Correct the input as the message says and retry."
	case 401, 403:
		return message, "The signed-in user is not allowed to do this. Ask a Collibra administrator for the permission."
	case 404:
		return message, "DGC does not know one of the ids. Look it up again and retry."
	case 409:
		return message, "DGC refused because of a conflict, such as a name that is already taken. Change the input or reuse the existing item."
	default:
		return message, "DGC could not carry out the request. Check whether it took effect before retrying."
	}
}

// Chunks splits values into slices of at most size, so a request carrying many ids stays within
// what a URL or a body can hold.
func Chunks(values []string, size int) [][]string {
	var chunks [][]string
	for start := 0; start < len(values); start += size {
		end := min(start+size, len(values))
		chunks = append(chunks, values[start:end])
	}
	return chunks
}
