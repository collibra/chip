package skills

import (
	"strings"
	"testing"

	"github.com/collibra/chip/pkg/chip"
)

func TestEnabled(t *testing.T) {
	tests := []struct {
		name string
		cfg  chip.ServerToolConfig
		want bool
	}{
		{
			name: "experimental off",
			cfg:  chip.ServerToolConfig{},
			want: false,
		},
		{
			name: "experimental on, no tool restrictions",
			cfg:  chip.ServerToolConfig{Experimental: []string{FeatureName}},
			want: true,
		},
		{
			name: "list tool disabled",
			cfg: chip.ServerToolConfig{
				Experimental:  []string{FeatureName},
				DisabledTools: []string{listToolName},
			},
			want: false,
		},
		{
			name: "load tool disabled",
			cfg: chip.ServerToolConfig{
				Experimental:  []string{FeatureName},
				DisabledTools: []string{loadToolName},
			},
			want: false,
		},
		{
			name: "enabledTools allowlist excludes skill tools",
			cfg: chip.ServerToolConfig{
				Experimental: []string{FeatureName},
				EnabledTools: []string{"some_other_tool"},
			},
			want: false,
		},
		{
			name: "enabledTools allowlist includes both skill tools",
			cfg: chip.ServerToolConfig{
				Experimental: []string{FeatureName},
				EnabledTools: []string{listToolName, loadToolName},
			},
			want: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := Enabled(&tc.cfg); got != tc.want {
				t.Errorf("Enabled() = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestEnabledToolNamesMatchRegisteredTools guards against drift between the
// constants Enabled gates on and the Name fields the tools actually register
// with. If these diverge, Enabled would return true while RegisterAll
// registers tools under different names — or vice versa.
func TestEnabledToolNamesMatchRegisteredTools(t *testing.T) {
	catalog, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if name := NewListTool(catalog).Name; name != listToolName {
		t.Errorf("list tool name drift: registered %q, gated on %q", name, listToolName)
	}
	if name := NewLoadTool(catalog).Name; name != loadToolName {
		t.Errorf("load tool name drift: registered %q, gated on %q", name, loadToolName)
	}
}

func TestRegisterAll_emptyExternalDirSucceeds(t *testing.T) {
	server := chip.NewServer()
	if err := RegisterAll(server, ""); err != nil {
		t.Fatalf("RegisterAll(server, \"\"): %v", err)
	}
}

func TestRegisterAll_badExternalDirReturnsError(t *testing.T) {
	server := chip.NewServer()
	badPath := "/this/path/should/not/exist/abc123"
	err := RegisterAll(server, badPath)
	if err == nil {
		t.Fatal("expected error for missing external dir")
	}
	if !strings.Contains(err.Error(), badPath) {
		t.Errorf("error should cite the path, got: %v", err)
	}
}
