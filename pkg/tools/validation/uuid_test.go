package validation

import (
	"strings"
	"testing"
)

const validUUID = "9179b887-04ef-4ce5-ab3a-b5bbd39ea3c8"

func TestUUID_Valid(t *testing.T) {
	if err := UUID("assetId", validUUID); err != nil {
		t.Fatalf("expected valid UUID to pass, got %v", err)
	}
}

func TestUUID_Malformed(t *testing.T) {
	err := UUID("assetId", "not-a-uuid")
	if err == nil {
		t.Fatal("expected error for malformed UUID")
	}
	if !strings.Contains(err.Error(), "assetId") {
		t.Fatalf("expected error to mention field name, got %v", err)
	}
}

func TestUUID_Empty(t *testing.T) {
	if err := UUID("assetId", ""); err == nil {
		t.Fatal("expected error for empty UUID on required field")
	}
}

func TestUUIDOptional_Empty(t *testing.T) {
	if err := UUIDOptional("dgcId", ""); err != nil {
		t.Fatalf("expected empty string to pass for optional UUID, got %v", err)
	}
}

func TestUUIDOptional_Invalid(t *testing.T) {
	if err := UUIDOptional("dgcId", "nope"); err == nil {
		t.Fatal("expected error for malformed optional UUID")
	}
}

func TestUUIDs_AllValid(t *testing.T) {
	if err := UUIDs("assetIds", []string{validUUID, validUUID}); err != nil {
		t.Fatalf("expected all-valid slice to pass, got %v", err)
	}
}

func TestUUIDs_Empty(t *testing.T) {
	if err := UUIDs("assetIds", nil); err != nil {
		t.Fatalf("expected empty slice to pass, got %v", err)
	}
}

func TestUUIDs_OneInvalid(t *testing.T) {
	err := UUIDs("assetIds", []string{validUUID, "nope"})
	if err == nil {
		t.Fatal("expected error when a slice entry is malformed")
	}
	if !strings.Contains(err.Error(), "assetIds") || !strings.Contains(err.Error(), "[1]") {
		t.Fatalf("expected error identifying field and index, got %v", err)
	}
}
