package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/collibra/chip/pkg/chip"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	aliceAuth = "Basic YWxpY2U6c2VjcmV0"
	aliceID   = "00000000-0000-0000-0000-00000000a11c"
	bobAuth   = "Basic Ym9iOnNlY3JldA=="
	bobID     = "00000000-0000-0000-0000-000000000b0b"
)

func TestUserIDHeaderIsSetFromCurrentUserLookup(t *testing.T) {
	dgc := newFakeDGC(t, usersByCredential)
	client := newTestClient(dgc)

	callTool(t, client, http.Header{"Authorization": {aliceAuth}})
	callTool(t, client, http.Header{"Authorization": {aliceAuth}})

	toolRequests := dgc.toolRequests()
	if len(toolRequests) != 2 {
		t.Fatalf("expected 2 DGC requests, got %d", len(toolRequests))
	}
	for _, request := range toolRequests {
		if got := request.Get(userIDHeader); got != aliceID {
			t.Errorf("expected %s %q, got %q", userIDHeader, aliceID, got)
		}
	}
	lookups := dgc.lookups()
	if len(lookups) != 1 {
		t.Fatalf("expected 1 lookup, got %d", len(lookups))
	}
	if values := lookups[0].Values(userIDHeader); len(values) != 0 {
		t.Errorf("lookup request must not carry %s, got %v", userIDHeader, values)
	}
	if got := lookups[0].Get("Authorization"); got != aliceAuth {
		t.Errorf("lookup request must carry the caller's credential, got %q", got)
	}
}

func TestUserIDLookupHappensOnceForSequentialAndConcurrentCalls(t *testing.T) {
	dgc := newFakeDGC(t, func(w http.ResponseWriter, r *http.Request) {
		// Holding the response open lets concurrent callers pile up on the in-flight lookup.
		time.Sleep(50 * time.Millisecond)
		usersByCredential(w, r)
	})
	client := newTestClient(dgc)

	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			callTool(t, client, http.Header{"Authorization": {aliceAuth}})
		}()
	}
	wg.Wait()
	for range 5 {
		callTool(t, client, http.Header{"Authorization": {aliceAuth}})
	}

	if got := len(dgc.lookups()); got != 1 {
		t.Errorf("expected 1 lookup, got %d", got)
	}
	for _, request := range dgc.toolRequests() {
		if got := request.Get(userIDHeader); got != aliceID {
			t.Errorf("expected %s %q, got %q", userIDHeader, aliceID, got)
		}
	}
}

func TestUserIDIsResolvedPerCredential(t *testing.T) {
	dgc := newFakeDGC(t, usersByCredential)
	client := newTestClient(dgc)

	callTool(t, client, http.Header{"Authorization": {aliceAuth}})
	callTool(t, client, http.Header{"Authorization": {bobAuth}})
	callTool(t, client, http.Header{"Authorization": {aliceAuth}})
	callTool(t, client, http.Header{"Authorization": {bobAuth}})

	if got := len(dgc.lookups()); got != 2 {
		t.Errorf("expected 2 lookups, got %d", got)
	}
	expected := map[string]string{aliceAuth: aliceID, bobAuth: bobID}
	for _, request := range dgc.toolRequests() {
		authorization := request.Get("Authorization")
		if got := request.Get(userIDHeader); got != expected[authorization] {
			t.Errorf("request with %q: expected %s %q, got %q", authorization, userIDHeader, expected[authorization], got)
		}
	}
}

func TestUserIDCacheEntryExpiresAfterTTL(t *testing.T) {
	dgc := newFakeDGC(t, usersByCredential)
	clock := &fakeClock{now: time.Unix(0, 0)}
	transport := newTestTransport(dgc)
	transport.userIDs.cache = newUserIDCache(userIDCacheSize, userIDCacheTTL, clock.Now)
	client := &http.Client{Transport: transport}

	callTool(t, client, http.Header{"Authorization": {aliceAuth}})
	clock.Advance(userIDCacheTTL - time.Second)
	callTool(t, client, http.Header{"Authorization": {aliceAuth}})
	if got := len(dgc.lookups()); got != 1 {
		t.Fatalf("expected 1 lookup within the TTL, got %d", got)
	}

	clock.Advance(2 * time.Second)
	callTool(t, client, http.Header{"Authorization": {aliceAuth}})
	if got := len(dgc.lookups()); got != 2 {
		t.Errorf("expected a second lookup once 10 minutes passed, got %d lookups", got)
	}
}

func TestUserIDCacheEvictsLeastRecentlyUsedCredential(t *testing.T) {
	dgc := newFakeDGC(t, usersByCredential)
	transport := newTestTransport(dgc)
	transport.userIDs.cache = newUserIDCache(2, userIDCacheTTL, time.Now)
	client := &http.Client{Transport: transport}
	carolAuth := "Basic Y2Fyb2w6c2VjcmV0"

	callTool(t, client, http.Header{"Authorization": {aliceAuth}})
	callTool(t, client, http.Header{"Authorization": {bobAuth}})
	callTool(t, client, http.Header{"Authorization": {aliceAuth}})
	callTool(t, client, http.Header{"Authorization": {carolAuth}})
	if got := dgc.lookupCounts(); got[aliceAuth] != 1 || got[bobAuth] != 1 || got[carolAuth] != 1 {
		t.Fatalf("expected one lookup per credential, got %v", got)
	}

	callTool(t, client, http.Header{"Authorization": {aliceAuth}})
	callTool(t, client, http.Header{"Authorization": {bobAuth}})
	got := dgc.lookupCounts()
	if got[aliceAuth] != 1 {
		t.Errorf("recently used credential should stay cached, got %d lookups", got[aliceAuth])
	}
	if got[bobAuth] != 2 {
		t.Errorf("least recently used credential should be looked up again, got %d lookups", got[bobAuth])
	}
}

func TestFailedUserIDLookupLeavesHeaderOff(t *testing.T) {
	cases := map[string]http.HandlerFunc{
		"unauthorized": func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
		},
		"server error": func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "boom", http.StatusInternalServerError)
		},
		"timeout": func(w http.ResponseWriter, r *http.Request) {
			select {
			case <-r.Context().Done():
			case <-time.After(5 * time.Second):
			}
		},
		"malformed body": func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"id":`))
		},
		"missing id": func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"userName":"alice"}`))
		},
	}
	for name, handler := range cases {
		t.Run(name, func(t *testing.T) {
			dgc := newFakeDGC(t, handler)
			transport := newTestTransport(dgc)
			transport.userIDs.lookupTimeout = 50 * time.Millisecond
			client := &http.Client{Transport: transport}

			callTool(t, client, http.Header{"Authorization": {aliceAuth}})

			toolRequests := dgc.toolRequests()
			if len(toolRequests) != 1 {
				t.Fatalf("expected the DGC request to be sent, got %d", len(toolRequests))
			}
			if values := toolRequests[0].Values(userIDHeader); len(values) != 0 {
				t.Errorf("expected no %s, got %v", userIDHeader, values)
			}
		})
	}
}

func TestFailedUserIDLookupIsCached(t *testing.T) {
	dgc := newFakeDGC(t, func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	})
	client := newTestClient(dgc)

	callTool(t, client, http.Header{"Authorization": {aliceAuth}})
	callTool(t, client, http.Header{"Authorization": {aliceAuth}})

	if got := len(dgc.lookups()); got != 1 {
		t.Errorf("expected 1 lookup, got %d", got)
	}
	for _, request := range dgc.toolRequests() {
		if values := request.Values(userIDHeader); len(values) != 0 {
			t.Errorf("expected no %s, got %v", userIDHeader, values)
		}
	}
}

func TestRequestWithoutAuthorizationMakesNoLookup(t *testing.T) {
	dgc := newFakeDGC(t, usersByCredential)
	client := newTestClient(dgc)

	callTool(t, client, http.Header{})

	if got := len(dgc.lookups()); got != 0 {
		t.Errorf("expected no lookup, got %d", got)
	}
	toolRequests := dgc.toolRequests()
	if len(toolRequests) != 1 {
		t.Fatalf("expected 1 DGC request, got %d", len(toolRequests))
	}
	if values := toolRequests[0].Values(userIDHeader); len(values) != 0 {
		t.Errorf("expected no %s, got %v", userIDHeader, values)
	}
}

func TestInboundUserIDHeaderIsNotForwarded(t *testing.T) {
	const spoofed = "11111111-1111-1111-1111-111111111111"
	cases := map[string]http.HandlerFunc{
		"lookup succeeds": usersByCredential,
		"lookup fails": func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
		},
	}
	for name, handler := range cases {
		t.Run(name, func(t *testing.T) {
			dgc := newFakeDGC(t, handler)
			client := newTestClient(dgc)

			callTool(t, client, http.Header{"Authorization": {aliceAuth}, "X-Mcp-User-Id": {spoofed}})
			callTool(t, client, http.Header{"X-Mcp-User-Id": {spoofed}})

			for _, request := range dgc.toolRequests() {
				for _, value := range request.Values(userIDHeader) {
					if value == spoofed {
						t.Errorf("client-supplied %s was forwarded to DGC", userIDHeader)
					}
				}
			}
		})
	}
}

// usersByCredential answers the current-user lookup with the UUID of the calling credential.
func usersByCredential(w http.ResponseWriter, r *http.Request) {
	ids := map[string]string{aliceAuth: aliceID, bobAuth: bobID}
	id, ok := ids[r.Header.Get("Authorization")]
	if !ok {
		id = "00000000-0000-0000-0000-000000000000"
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = fmt.Fprintf(w, `{"id":%q,"userName":"user"}`, id)
}

func newTestClient(dgc *fakeDGC) *http.Client {
	return &http.Client{Transport: newTestTransport(dgc)}
}

func newTestTransport(dgc *fakeDGC) *collibraClient {
	config := &Config{Api: CollibraApiConfig{Url: dgc.server.URL}}
	return newCollibraTransport(config, chip.NewCollibraClient(http.DefaultTransport))
}

// callTool sends one DGC request the way a tool does, as part of an MCP tool call whose
// inbound HTTP headers are mcpHeaders.
func callTool(t *testing.T, client *http.Client, mcpHeaders http.Header) {
	t.Helper()
	toolRequest := &mcp.CallToolRequest{
		Session: &mcp.ServerSession{},
		Params:  &mcp.CallToolParamsRaw{Name: "test_tool"},
		Extra:   &mcp.RequestExtra{Header: mcpHeaders},
	}
	ctx := chip.SetCallToolRequest(context.Background(), toolRequest)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "/rest/2.0/assets", nil)
	if err != nil {
		t.Fatalf("building request: %v", err)
	}
	response, err := client.Do(request)
	if err != nil {
		t.Errorf("sending request: %v", err)
		return
	}
	_ = response.Body.Close()
}

// fakeDGC records every request it receives; the current-user endpoint is served by lookup and
// everything else answers 200.
type fakeDGC struct {
	server   *httptest.Server
	mu       sync.Mutex
	requests []recordedRequest
}

type recordedRequest struct {
	path   string
	header http.Header
}

func newFakeDGC(t *testing.T, lookup http.HandlerFunc) *fakeDGC {
	dgc := &fakeDGC{}
	dgc.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		dgc.mu.Lock()
		dgc.requests = append(dgc.requests, recordedRequest{path: r.URL.Path, header: r.Header.Clone()})
		dgc.mu.Unlock()
		if r.URL.Path == currentUserPath {
			lookup(w, r)
			return
		}
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(dgc.server.Close)
	return dgc
}

func (d *fakeDGC) lookups() []http.Header {
	return d.headers(func(path string) bool { return path == currentUserPath })
}

func (d *fakeDGC) toolRequests() []http.Header {
	return d.headers(func(path string) bool { return path != currentUserPath })
}

func (d *fakeDGC) lookupCounts() map[string]int {
	counts := map[string]int{}
	for _, header := range d.lookups() {
		counts[header.Get("Authorization")]++
	}
	return counts
}

func (d *fakeDGC) headers(match func(path string) bool) []http.Header {
	d.mu.Lock()
	defer d.mu.Unlock()
	var headers []http.Header
	for _, request := range d.requests {
		if match(request.path) {
			headers = append(headers, request.header)
		}
	}
	return headers
}

type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}
