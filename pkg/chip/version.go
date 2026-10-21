package chip

import (
	"regexp"
	"runtime/debug"
	"strings"
)

// Version is the release version. GoReleaser sets it via -ldflags -X for
// official builds. Otherwise init derives it from the Go build info: the VCS
// tag or commit for a checkout, the module version for `go install`, or "dev".
var Version = ""

func init() {
	if Version == "" {
		Version = buildInfoVersion(debug.ReadBuildInfo())
	}
}

// buildInfoVersion always returns valid semver. Build metadata ("+src") marks
// a build made from source, so it can't be mistaken for an official release
// binary of the same version.
func buildInfoVersion(bi *debug.BuildInfo, ok bool) string {
	if !ok || bi.Main.Version == "" || bi.Main.Version == "(devel)" {
		return "0.0.0-dev+src"
	}
	v := strings.TrimPrefix(bi.Main.Version, "v")
	base, meta, found := strings.Cut(v, "+")
	// Pseudo-versions carry a yyyymmddhhmmss timestamp; the date is enough.
	base = pseudoTimestamp.ReplaceAllString(base, "$1$2-$3")
	if found {
		return base + "+src." + meta
	}
	return base + "+src"
}

var pseudoTimestamp = regexp.MustCompile(`([.-])(\d{8})\d{6}-([0-9a-f]{12})$`)
