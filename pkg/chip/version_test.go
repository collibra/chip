package chip

import (
	"regexp"
	"runtime/debug"
	"testing"
)

// Official semver.org regular expression.
var semver = regexp.MustCompile(`^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)` +
	`(?:-((?:0|[1-9]\d*|\d*[a-zA-Z-][0-9a-zA-Z-]*)(?:\.(?:0|[1-9]\d*|\d*[a-zA-Z-][0-9a-zA-Z-]*))*))?` +
	`(?:\+([0-9a-zA-Z-]+(?:\.[0-9a-zA-Z-]+)*))?$`)

func TestBuildInfoVersion(t *testing.T) {
	info := func(v string) *debug.BuildInfo {
		return &debug.BuildInfo{Main: debug.Module{Version: v}}
	}

	tests := []struct {
		name string
		bi   *debug.BuildInfo
		ok   bool
		want string
	}{
		{"no build info", nil, false, "0.0.0-dev+src"},
		{"empty version", info(""), true, "0.0.0-dev+src"},
		{"devel placeholder", info("(devel)"), true, "0.0.0-dev+src"},
		{"tagged checkout", info("v0.0.50"), true, "0.0.50+src"},
		{"tagged checkout with local changes", info("v0.0.50+dirty"), true, "0.0.50+src.dirty"},
		{"after a tag", info("v0.0.66-0.20261021190308-571e6b101ec6"), true, "0.0.66-0.20261021-571e6b101ec6+src"},
		{"after a tag with local changes", info("v0.0.66-0.20261021190308-571e6b101ec6+dirty"), true, "0.0.66-0.20261021-571e6b101ec6+src.dirty"},
		{"after a prerelease tag", info("v1.2.3-rc.1.0.20261021190308-571e6b101ec6"), true, "1.2.3-rc.1.0.20261021-571e6b101ec6+src"},
		{"no tags in clone", info("v0.0.0-20261021094702-318a62fbeeca"), true, "0.0.0-20261021-318a62fbeeca+src"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildInfoVersion(tt.bi, tt.ok)
			if got != tt.want {
				t.Fatalf("buildInfoVersion() = %q, want %q", got, tt.want)
			}
			if !semver.MatchString(got) {
				t.Fatalf("%q is not valid semver", got)
			}
		})
	}
}
