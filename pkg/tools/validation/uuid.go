// Package validation provides shared input validation helpers for CHIP tools.
// Helpers return Go errors that the MCP SDK wraps as tool execution errors
// (isError: true), letting the calling model see the problem and self-correct.
package validation

import (
	"fmt"
	"regexp"

	"github.com/google/uuid"
)

// UUID verifies value is a well-formed UUID. fieldName is surfaced in the
// returned error so the calling model can identify which field failed.
func UUID(fieldName, value string) error {
	if _, err := uuid.Parse(value); err != nil {
		return fmt.Errorf("invalid UUID for %q: %s", fieldName, err.Error())
	}
	return nil
}

// UUIDOptional verifies value is a well-formed UUID when non-empty. Empty
// strings pass. Use for optional UUID fields.
func UUIDOptional(fieldName, value string) error {
	if value == "" {
		return nil
	}
	return UUID(fieldName, value)
}

// UUIDs verifies every entry in values is a well-formed UUID. An empty slice
// passes. The error identifies the offending element's index.
func UUIDs(fieldName string, values []string) error {
	for i, v := range values {
		if _, err := uuid.Parse(v); err != nil {
			return fmt.Errorf("invalid UUID for %q[%d]: %s", fieldName, i, err.Error())
		}
	}
	return nil
}

// dashedUUID is the 8-4-4-4-12 form of a UUID, in either case. UUID accepts other forms too (no
// dashes, braces, urn:uuid:) that DGC's REST API rejects with a 400 when they are sent as given.
var dashedUUID = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// DashedUUID verifies value is a UUID in the dashed form, so a tool can send it to DGC as given.
func DashedUUID(fieldName, value string) error {
	if !dashedUUID.MatchString(value) {
		return fmt.Errorf("invalid UUID for %q: %q is not in the form 00000000-0000-0000-0000-000000000000", fieldName, value)
	}
	return nil
}

// DashedUUIDOptional verifies value is a UUID in the dashed form when non-empty. Empty strings pass.
func DashedUUIDOptional(fieldName, value string) error {
	if value == "" {
		return nil
	}
	return DashedUUID(fieldName, value)
}

// DashedUUIDs verifies every entry in values is a UUID in the dashed form. An empty slice passes. The
// error identifies the offending element's index.
func DashedUUIDs(fieldName string, values []string) error {
	for i, v := range values {
		if err := DashedUUID(fmt.Sprintf("%s[%d]", fieldName, i), v); err != nil {
			return err
		}
	}
	return nil
}
