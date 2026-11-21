package main

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
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

func TestServerWideCredentialsResolveTheServiceAccount(t *testing.T) {
	const serviceAccountID = "00000000-0000-0000-0000-0000000005e1"
	serviceAccountAuth := "Basic " + base64.StdEncoding.EncodeToString([]byte("svc:secret"))
	dgc := newFakeDGC(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != serviceAccountAuth {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		_, _ = fmt.Fprintf(w, `{"id":%q}`, serviceAccountID)
	})
	config := &Config{Api: CollibraApiConfig{Url: dgc.server.URL, Username: "svc", Password: "secret"}}
	client := &http.Client{Transport: newCollibraTransport(config, chip.NewCollibraClient(http.DefaultTransport))}

	callTool(t, client, http.Header{"Authorization": {aliceAuth}})
	callTool(t, client, http.Header{"Authorization": {bobAuth}})

	lookups := dgc.lookups()
	if len(lookups) != 1 {
		t.Fatalf("expected 1 lookup, got %d", len(lookups))
	}
	if got := lookups[0].Get("Authorization"); got != serviceAccountAuth {
		t.Errorf("lookup must use the configured credentials, got %q", got)
	}
	for _, request := range dgc.toolRequests() {
		if got := request.Get(userIDHeader); got != serviceAccountID {
			t.Errorf("expected %s %q, got %q", userIDHeader, serviceAccountID, got)
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

func TestDefinitiveLookupFailureIsCachedForTenMinutes(t *testing.T) {
	cases := map[string]http.HandlerFunc{
		"401":      statusHandler(http.StatusUnauthorized),
		"403":      statusHandler(http.StatusForbidden),
		"404":      statusHandler(http.StatusNotFound),
		"empty id": bodyHandler(`{"id":"","userName":"alice"}`),
	}
	for name, handler := range cases {
		t.Run(name, func(t *testing.T) {
			dgc := newFakeDGC(t, handler)
			lookups := &lookupTransport{}
			clock := &fakeClock{now: time.Unix(0, 0)}
			client := &http.Client{Transport: newInterceptedTransport(dgc, lookups, clock)}

			callTool(t, client, http.Header{"Authorization": {aliceAuth}})
			clock.Advance(userIDCacheTTL - time.Second)
			callTool(t, client, http.Header{"Authorization": {aliceAuth}})
			if got := lookups.count(); got != 1 {
				t.Fatalf("expected 1 lookup within 10 minutes, got %d", got)
			}

			clock.Advance(2 * time.Second)
			callTool(t, client, http.Header{"Authorization": {aliceAuth}})
			if got := lookups.count(); got != 2 {
				t.Errorf("expected a second lookup after 10 minutes, got %d lookups", got)
			}
			assertNoUserIDHeader(t, dgc)
		})
	}
}

func TestTransientLookupFailureIsCachedForThirtySeconds(t *testing.T) {
	cases := map[string]struct {
		handler   http.HandlerFunc
		intercept func(*http.Request) (*http.Response, error)
	}{
		"timeout":           {handler: blockingHandler(5 * time.Second)},
		"transport error":   {intercept: func(*http.Request) (*http.Response, error) { return nil, errors.New("connection reset") }},
		"500":               {handler: statusHandler(http.StatusInternalServerError)},
		"503":               {handler: statusHandler(http.StatusServiceUnavailable)},
		"408":               {handler: statusHandler(http.StatusRequestTimeout)},
		"429":               {handler: statusHandler(http.StatusTooManyRequests)},
		"malformed body":    {handler: bodyHandler(`{"id":`)},
		"oversized body":    {handler: bodyHandler(oversizedUserBody())},
		"padded past limit": {handler: bodyHandler(`{"id":"` + aliceID + `"}` + strings.Repeat(" ", userIDResponseLimit))},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			dgc := newFakeDGC(t, tc.handler)
			lookups := &lookupTransport{intercept: tc.intercept}
			clock := &fakeClock{now: time.Unix(0, 0)}
			transport := newInterceptedTransport(dgc, lookups, clock)
			transport.userIDs.lookupTimeout = 50 * time.Millisecond
			client := &http.Client{Transport: transport}

			callTool(t, client, http.Header{"Authorization": {aliceAuth}})
			clock.Advance(userIDTransientFailureTTL - time.Second)
			callTool(t, client, http.Header{"Authorization": {aliceAuth}})
			if got := lookups.count(); got != 1 {
				t.Fatalf("expected 1 lookup within 30 seconds, got %d", got)
			}

			clock.Advance(2 * time.Second)
			callTool(t, client, http.Header{"Authorization": {aliceAuth}})
			if got := lookups.count(); got != 2 {
				t.Errorf("expected a second lookup after 30 seconds, got %d lookups", got)
			}
			assertNoUserIDHeader(t, dgc)
		})
	}
}

func TestSlowLookupIsAbandonedAfterTimeout(t *testing.T) {
	if userIDLookupTimeout != 500*time.Millisecond {
		t.Fatalf("expected a 500ms lookup timeout, got %s", userIDLookupTimeout)
	}
	dgc := newFakeDGC(t, blockingHandler(3*time.Second))
	client := newTestClient(dgc)

	started := time.Now()
	callTool(t, client, http.Header{"Authorization": {aliceAuth}})
	if elapsed := time.Since(started); elapsed > 2*time.Second {
		t.Errorf("expected the lookup to be abandoned after %s, the call took %s", userIDLookupTimeout, elapsed)
	}
	if got := len(dgc.toolRequests()); got != 1 {
		t.Fatalf("expected the DGC request to be sent, got %d", got)
	}
	assertNoUserIDHeader(t, dgc)
}

func TestLookupFailureLogLevel(t *testing.T) {
	cases := map[string]struct {
		handler http.HandlerFunc
		level   slog.Level
	}{
		"401":     {handler: statusHandler(http.StatusUnauthorized), level: slog.LevelDebug},
		"403":     {handler: statusHandler(http.StatusForbidden), level: slog.LevelDebug},
		"404":     {handler: statusHandler(http.StatusNotFound), level: slog.LevelWarn},
		"500":     {handler: statusHandler(http.StatusInternalServerError), level: slog.LevelWarn},
		"timeout": {handler: blockingHandler(5 * time.Second), level: slog.LevelWarn},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			logs := captureLogs(t)
			dgc := newFakeDGC(t, tc.handler)
			transport := newTestTransport(dgc)
			transport.userIDs.lookupTimeout = 50 * time.Millisecond

			callTool(t, &http.Client{Transport: transport}, http.Header{"Authorization": {aliceAuth}})

			levels := logs.levelsOf("Could not resolve the current user")
			if len(levels) != 1 || levels[0] != tc.level {
				t.Errorf("expected one %s record, got %v", tc.level, levels)
			}
		})
	}
}

func TestPanickingLookupIsRecoveredAndReleasesWaiters(t *testing.T) {
	dgc := newFakeDGC(t, usersByCredential)
	lookups := &lookupTransport{intercept: func(*http.Request) (*http.Response, error) {
		// Delaying the panic lets the other callers start waiting on this lookup.
		time.Sleep(50 * time.Millisecond)
		panic("lookup transport failed")
	}}
	client := &http.Client{Transport: newInterceptedTransport(dgc, lookups, nil)}

	done := make(chan struct{})
	go func() {
		defer close(done)
		var wg sync.WaitGroup
		for range 5 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				callTool(t, client, http.Header{"Authorization": {aliceAuth}})
			}()
		}
		wg.Wait()
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("callers waiting on a panicking lookup were not released")
	}

	if got := lookups.count(); got != 1 {
		t.Errorf("expected 1 lookup, got %d", got)
	}
	if got := len(dgc.toolRequests()); got != 5 {
		t.Errorf("expected 5 DGC requests, got %d", got)
	}
	assertNoUserIDHeader(t, dgc)
}

func TestCancelledWaiterDoesNotWaitForInFlightLookup(t *testing.T) {
	lookupStarted := make(chan struct{})
	releaseLookup := make(chan struct{})
	var startOnce sync.Once
	dgc := newFakeDGC(t, func(w http.ResponseWriter, r *http.Request) {
		startOnce.Do(func() { close(lookupStarted) })
		select {
		case <-releaseLookup:
		case <-r.Context().Done():
		}
		usersByCredential(w, r)
	})
	transport := newTestTransport(dgc)
	transport.userIDs.lookupTimeout = 10 * time.Second
	baseURL, err := url.Parse(dgc.server.URL)
	if err != nil {
		t.Fatalf("parsing server URL: %v", err)
	}

	owner := make(chan struct{})
	go func() {
		defer close(owner)
		callTool(t, &http.Client{Transport: transport}, http.Header{"Authorization": {aliceAuth}})
	}()
	<-lookupStarted

	ctx, cancel := context.WithCancel(context.Background())
	waiter := make(chan string, 1)
	go func() { waiter <- transport.userIDs.resolve(ctx, baseURL, aliceAuth) }()
	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case id := <-waiter:
		if id != "" {
			t.Errorf("expected no user ID for a cancelled waiter, got %q", id)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("cancelled waiter kept waiting for the in-flight lookup")
	}
	close(releaseLookup)
	<-owner
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

func statusHandler(status int) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, http.StatusText(status), status)
	}
}

func bodyHandler(body string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}
}

// blockingHandler answers only after delay, or not at all if the lookup is abandoned first.
func blockingHandler(delay time.Duration) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(delay):
			usersByCredential(w, r)
		}
	}
}

// oversizedUserBody is valid JSON with an id, padded past userIDResponseLimit.
func oversizedUserBody() string {
	return `{"id":"` + aliceID + `","padding":"` + strings.Repeat("x", userIDResponseLimit) + `"}`
}

func assertNoUserIDHeader(t *testing.T, dgc *fakeDGC) {
	t.Helper()
	for _, request := range dgc.toolRequests() {
		if values := request.Values(userIDHeader); len(values) != 0 {
			t.Errorf("expected no %s, got %v", userIDHeader, values)
		}
	}
}

func newTestClient(dgc *fakeDGC) *http.Client {
	return &http.Client{Transport: newTestTransport(dgc)}
}

func newTestTransport(dgc *fakeDGC) *collibraClient {
	config := &Config{Api: CollibraApiConfig{Url: dgc.server.URL}}
	return newCollibraTransport(config, chip.NewCollibraClient(http.DefaultTransport))
}

// newInterceptedTransport routes DGC traffic through lookups and, when clock is set, ages the
// user ID cache by it.
func newInterceptedTransport(dgc *fakeDGC, lookups *lookupTransport, clock *fakeClock) *collibraClient {
	config := &Config{Api: CollibraApiConfig{Url: dgc.server.URL}}
	transport := newCollibraTransport(config, chip.NewCollibraClient(lookups))
	if clock != nil {
		transport.userIDs.cache = newUserIDCache(userIDCacheSize, userIDCacheTTL, clock.Now)
	}
	return transport
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

// lookupTransport sends requests to DGC and counts the current-user lookups; when intercept is
// set it answers the lookups instead of DGC.
type lookupTransport struct {
	mu        sync.Mutex
	lookups   int
	intercept func(*http.Request) (*http.Response, error)
}

func (l *lookupTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if request.URL.Path == currentUserPath {
		l.mu.Lock()
		l.lookups++
		l.mu.Unlock()
		if l.intercept != nil {
			return l.intercept(request)
		}
	}
	return http.DefaultTransport.RoundTrip(request)
}

func (l *lookupTransport) count() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.lookups
}

// logCapture is an slog handler that records every log record.
type logCapture struct {
	mu      sync.Mutex
	records []slog.Record
}

func captureLogs(t *testing.T) *logCapture {
	capture := &logCapture{}
	previous := slog.Default()
	slog.SetDefault(slog.New(capture))
	t.Cleanup(func() { slog.SetDefault(previous) })
	return capture
}

func (c *logCapture) levelsOf(messagePrefix string) []slog.Level {
	c.mu.Lock()
	defer c.mu.Unlock()
	var levels []slog.Level
	for _, record := range c.records {
		if strings.HasPrefix(record.Message, messagePrefix) {
			levels = append(levels, record.Level)
		}
	}
	return levels
}

func (c *logCapture) Enabled(context.Context, slog.Level) bool { return true }

func (c *logCapture) Handle(_ context.Context, record slog.Record) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.records = append(c.records, record)
	return nil
}

func (c *logCapture) WithAttrs([]slog.Attr) slog.Handler { return c }

func (c *logCapture) WithGroup(string) slog.Handler { return c }
