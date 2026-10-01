//go:build live

package get_dq_job_run_logs_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"testing"

	tools "github.com/collibra/chip/pkg/tools/get_dq_job_run_logs"
)

// TestLive calls a real Collibra instance. Run with:
//
//	COLLIBRA_MCP_API_URL=... COLLIBRA_MCP_API_USR=... COLLIBRA_MCP_API_PWD=... LIVE_RUN_ID=... \
//	  go test -tags live -run TestLive -v ./pkg/tools/get_dq_job_run_logs/
//
// LIVE_INPUT optionally overrides the tool input as JSON (run_id is filled from LIVE_RUN_ID).
func TestLive(t *testing.T) {
	base, runID := os.Getenv("COLLIBRA_MCP_API_URL"), os.Getenv("LIVE_RUN_ID")
	if base == "" || runID == "" {
		t.Skip("COLLIBRA_MCP_API_URL and LIVE_RUN_ID must be set")
	}
	var in tools.Input
	if raw := os.Getenv("LIVE_INPUT"); raw != "" {
		if err := json.Unmarshal([]byte(raw), &in); err != nil {
			t.Fatalf("bad LIVE_INPUT: %v", err)
		}
	}
	in.RunID = runID
	client := &http.Client{Transport: &basicAuth{base: base, usr: os.Getenv("COLLIBRA_MCP_API_USR"), pwd: os.Getenv("COLLIBRA_MCP_API_PWD")}}

	out, err := tools.NewTool(client).Handler(t.Context(), in)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.MarshalIndent(out, "", "  ")
	compact, _ := json.Marshal(out) // what the MCP SDK sends
	fmt.Println(string(body))
	fmt.Printf("RESPONSE_CHARS=%d\n", len(compact))
}

type basicAuth struct{ base, usr, pwd string }

func (b *basicAuth) RoundTrip(r *http.Request) (*http.Response, error) {
	u, err := url.Parse(b.base)
	if err != nil {
		return nil, err
	}
	c := r.Clone(r.Context())
	c.URL.Scheme, c.URL.Host = u.Scheme, u.Host
	c.SetBasicAuth(b.usr, b.pwd)
	return http.DefaultTransport.RoundTrip(c)
}
