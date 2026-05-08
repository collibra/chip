// Package validation provides shared input validation helpers for CHIP tools.
// Helpers return Go errors that the MCP SDK wraps as tool execution errors
// (isError: true), letting the calling model see the problem and self-correct.
package validation

import (
	"fmt"

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
