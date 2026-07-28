package edit_assessment

import (
	"reflect"
	"testing"
)

func TestBuildAnswer(t *testing.T) {
	tests := []struct {
		name       string
		answerType string
		value      string
		items      []ItemInput
		want       any
		wantErr    bool
	}{
		{name: "text passthrough", answerType: "TEXT", value: "hello", want: "hello"},
		{name: "html passthrough", answerType: "HTML", value: "<b>hi</b>", want: "<b>hi</b>"},
		{name: "expression passthrough", answerType: "EXPRESSION", value: "a+b", want: "a+b"},
		{name: "number ok", answerType: "NUMBER", value: "42.5", want: 42.5},
		{name: "number trims", answerType: "NUMBER", value: " 7 ", want: 7.0},
		{name: "number bad", answerType: "NUMBER", value: "twelve", wantErr: true},
		{name: "boolean true", answerType: "BOOLEAN", value: "true", want: true},
		{name: "boolean false", answerType: "BOOLEAN", value: "false", want: false},
		{name: "boolean bad", answerType: "BOOLEAN", value: "yes", wantErr: true},
		{name: "date ok", answerType: "DATE", value: "2026-07-09", want: "2026-07-09"},
		{name: "date bad format", answerType: "DATE", value: "07/09/2026", wantErr: true},
		{name: "case insensitive type", answerType: "number", value: "1", want: 1.0},
		{name: "items ok defaults value to id", answerType: "ITEMS", items: []ItemInput{{ID: "Internal"}}, want: []answerItem{{ID: "Internal", Value: "Internal"}}},
		{name: "items ok explicit value", answerType: "ITEMS", items: []ItemInput{{ID: "yes", Value: "Yes"}}, want: []answerItem{{ID: "yes", Value: "Yes"}}},
		{name: "items empty", answerType: "ITEMS", items: nil, wantErr: true},
		{name: "items missing id", answerType: "ITEMS", items: []ItemInput{{Value: "x"}}, wantErr: true},
		{name: "assets unsupported", answerType: "ASSETS", value: "x", wantErr: true},
		{name: "userorgroups unsupported", answerType: "USERORGROUPS", value: "x", wantErr: true},
		{name: "attachments unsupported", answerType: "ATTACHMENTS", value: "x", wantErr: true},
		{name: "empty type", answerType: "", value: "x", wantErr: true},
		{name: "unknown type", answerType: "MYSTERY", value: "x", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := buildAnswer(tt.answerType, tt.value, tt.items)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got value %v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("got %v (%T), want %v (%T)", got, got, tt.want, tt.want)
			}
		})
	}
}

func TestResolveAnswerType(t *testing.T) {
	tests := []struct {
		name     string
		existing string
		caller   string
		want     string
		wantErr  bool
	}{
		{name: "existing wins", existing: "HTML", caller: "TEXT", want: "HTML"},
		{name: "existing only", existing: "NUMBER", want: "NUMBER"},
		{name: "caller fills blank", existing: "", caller: "items", want: "ITEMS"},
		{name: "caller trimmed+upper", existing: "", caller: "  boolean  ", want: "BOOLEAN"},
		{name: "neither -> error", existing: "", caller: "", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveAnswerType(tt.existing, tt.caller)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}
