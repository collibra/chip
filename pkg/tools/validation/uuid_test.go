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

func TestDashedUUID_AcceptsOnlyTheDashedForm(t *testing.T) {
	for _, value := range []string{"00000000-0000-0000-0000-000000900002", "ABCDEF00-1111-2222-3333-444455556666"} {
		if err := DashedUUID("id", value); err != nil {
			t.Errorf("expected %q to pass, got: %v", value, err)
		}
	}
	// uuid.Parse accepts these forms, but DGC's REST API answers them with a 400.
	for _, value := range []string{"", "x", "00000000000000000000000000900002", "{00000000-0000-0000-0000-000000900002}", "urn:uuid:00000000-0000-0000-0000-000000900002"} {
		if err := DashedUUID("id", value); err == nil {
			t.Errorf("expected %q to fail", value)
		}
	}
}

func TestDashedUUIDOptional_EmptyPasses(t *testing.T) {
	if err := DashedUUIDOptional("id", ""); err != nil {
		t.Errorf("expected an empty value to pass, got: %v", err)
	}
	if err := DashedUUIDOptional("id", "x"); err == nil {
		t.Error("expected a malformed value to fail")
	}
}

func TestDashedUUIDs_NamesTheOffendingElement(t *testing.T) {
	if err := DashedUUIDs("ids", nil); err != nil {
		t.Errorf("expected an empty list to pass, got: %v", err)
	}
	err := DashedUUIDs("ids", []string{"00000000-0000-0000-0000-000000900002", "bad"})
	if err == nil || !strings.Contains(err.Error(), "ids[1]") {
		t.Errorf("expected the error to name ids[1], got: %v", err)
	}
}
