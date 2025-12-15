//go:build release

package chip

import (
	"fmt"
	"os"
	"testing"
)

func TestReleaseVersionCheck(t *testing.T) {
	version := os.Getenv("VERSION")

	if Version != fmt.Sprintf("%s-SNAPSHOT", version) {
		t.Errorf("Release guard failed. Expected version %s, got %s", fmt.Sprintf("v%s-SNAPSHOT", version), Version)
	}
}
