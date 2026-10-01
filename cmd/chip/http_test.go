package main

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/collibra/chip/pkg/chip"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	testClientID     = "mcp-chip"
	testClientSecret = "very-secret-client-secret"
	testAccessToken  = "issued-access-token"
	testUsername     = "user"
	testPassword     = "pass"
)

// fakeCollibra serves both the OAuth token endpoint and a catch-all API
// endpoint on one httptest server, so the default token URL derivation is
// exercised end to end.
type fakeCollibra struct {
	*httptest.Server
	tokenStatus int
	tokenCalls  atomic.Int32
	apiCalls    atomic.Int32
	apiAuth     chan string
}

func newFakeCollibra(t *testing.T, tokenStatus int) *fakeCollibra {
	t.Helper()
	f := &fakeCollibra{tokenStatus: tokenStatus, apiAuth: make(chan string, 16)}
	mux := http.NewServeMux()
	mux.HandleFunc(defaultOAuthTokenPath, func(w http.ResponseWriter, r *http.Request) {
		f.tokenCalls.Add(1)
		if r.Method != http.MethodPost {
			t.Errorf("token endpoint method = %s, want POST", r.Method)
		}
		if id, secret, ok := r.BasicAuth(); !ok || id != testClientID || secret != testClientSecret {
			t.Errorf("token endpoint basic auth = (%q, ok=%v), want client credentials in header", id, ok)
		}
		if err := r.ParseForm(); err != nil {
			t.Errorf("token endpoint form parse: %v", err)
		}
		if got := r.PostForm.Get("grant_type"); got != "client_credentials" {
			t.Errorf("grant_type = %q, want client_credentials", got)
		}
		if f.tokenStatus != http.StatusOK {
			w.WriteHeader(f.tokenStatus)
			// Echo the secret back so the test can prove it never reaches the caller.
			_, _ = w.Write([]byte(`{"userMessage":"bad credentials for ` + testClientSecret + `"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"` + testAccessToken + `","token_type":"Bearer","expires_in":3600}`))
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		f.apiCalls.Add(1)
		f.apiAuth <- r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	})
	f.Server = httptest.NewServer(mux)
	t.Cleanup(f.Close)
	return f
}

func newToolContext(header http.Header) context.Context {
	toolRequest := &mcp.CallToolRequest{
		Session: &mcp.ServerSession{},
		Params:  &mcp.CallToolParamsRaw{Name: "test_tool"},
		Extra:   &mcp.RequestExtra{Header: header},
	}
	return chip.SetCallToolRequest(context.Background(), toolRequest)
}

func doAPIRequest(t *testing.T, client *http.Client, ctx context.Context) error {
	t.Helper()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "/rest/2.0/assetTypes", nil)
	if err != nil {
		t.Fatalf("building request: %v", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	return resp.Body.Close()
}

func basicAuthValue(username, password string) string {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(username+":"+password))
}

func TestRoundTrip_authenticationModes(t *testing.T) {
	tests := []struct {
		name           string
		configure      func(apiURL string) *Config
		clientHeader   http.Header
		requests       int
		wantAuth       string
		wantTokenCalls int32
	}{
		{
			name: "oauth client credentials sends bearer token and caches it",
			configure: func(apiURL string) *Config {
				return &Config{Api: CollibraApiConfig{Url: apiURL, OAuth: CollibraOAuthConfig{ClientID: testClientID, ClientSecret: testClientSecret}}}
			},
			requests:       2,
			wantAuth:       "Bearer " + testAccessToken,
			wantTokenCalls: 1,
		},
		{
			name: "basic auth sends basic header and never calls token endpoint",
			configure: func(apiURL string) *Config {
				return &Config{Api: CollibraApiConfig{Url: apiURL, Username: testUsername, Password: testPassword}}
			},
			requests:       2,
			wantAuth:       basicAuthValue(testUsername, testPassword),
			wantTokenCalls: 0,
		},
		{
			name: "no server credentials passes client header through unchanged",
			configure: func(apiURL string) *Config {
				return &Config{Api: CollibraApiConfig{Url: apiURL}}
			},
			clientHeader:   http.Header{"Authorization": []string{basicAuthValue("client", "supplied")}},
			requests:       1,
			wantAuth:       basicAuthValue("client", "supplied"),
			wantTokenCalls: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := newFakeCollibra(t, http.StatusOK)
			client := newCollibraClient(tt.configure(fake.URL))
			ctx := newToolContext(tt.clientHeader)

			for i := 0; i < tt.requests; i++ {
				if err := doAPIRequest(t, client, ctx); err != nil {
					t.Fatalf("request %d: %v", i+1, err)
				}
				if got := <-fake.apiAuth; got != tt.wantAuth {
					t.Errorf("request %d Authorization = %q, want %q", i+1, got, tt.wantAuth)
				}
			}
			if got := fake.tokenCalls.Load(); got != tt.wantTokenCalls {
				t.Errorf("token endpoint calls = %d, want %d", got, tt.wantTokenCalls)
			}
			if got := fake.apiCalls.Load(); got != int32(tt.requests) {
				t.Errorf("api calls = %d, want %d", got, tt.requests)
			}
		})
	}
}

func TestRoundTrip_tokenEndpointFailure(t *testing.T) {
	fake := newFakeCollibra(t, http.StatusUnauthorized)
	client := newCollibraClient(&Config{Api: CollibraApiConfig{
		Url:   fake.URL,
		OAuth: CollibraOAuthConfig{ClientID: testClientID, ClientSecret: testClientSecret},
	}})

	err := doAPIRequest(t, client, newToolContext(nil))
	if err == nil {
		t.Fatal("expected an error when the token endpoint rejects the client")
	}
	if !strings.Contains(err.Error(), "failed to obtain OAuth access token") {
		t.Errorf("error = %q, want it to mention the OAuth token failure", err)
	}
	if !strings.Contains(err.Error(), "HTTP 401") {
		t.Errorf("error = %q, want it to include the token endpoint status", err)
	}
	if strings.Contains(err.Error(), testClientSecret) {
		t.Errorf("error leaks the client secret: %q", err)
	}
	if got := fake.apiCalls.Load(); got != 0 {
		t.Errorf("api calls = %d, want 0 when no token could be obtained", got)
	}
}

func TestResolveOAuthTokenURL(t *testing.T) {
	tests := []struct {
		name     string
		apiURL   string
		tokenURL string
		want     string
	}{
		{name: "base url without trailing slash", apiURL: "https://example.collibra.com", want: "https://example.collibra.com/rest/oauth/v2/token"},
		{name: "base url with trailing slash", apiURL: "https://example.collibra.com/", want: "https://example.collibra.com/rest/oauth/v2/token"},
		{name: "base url with path prefix", apiURL: "https://example.com/collibra/", want: "https://example.com/collibra/rest/oauth/v2/token"},
		{name: "explicit token url overrides derivation", apiURL: "https://example.collibra.com", tokenURL: "https://auth.example.com/token", want: "https://auth.example.com/token"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolveOAuthTokenURL(tt.apiURL, tt.tokenURL); got != tt.want {
				t.Errorf("resolveOAuthTokenURL(%q, %q) = %q, want %q", tt.apiURL, tt.tokenURL, got, tt.want)
			}
		})
	}
}

func TestValidateOAuthConfig(t *testing.T) {
	tests := []struct {
		name    string
		api     CollibraApiConfig
		wantErr string
	}{
		{name: "no auth configured", api: CollibraApiConfig{}},
		{name: "basic auth only", api: CollibraApiConfig{Username: testUsername, Password: testPassword}},
		{name: "oauth only", api: CollibraApiConfig{OAuth: CollibraOAuthConfig{ClientID: testClientID, ClientSecret: testClientSecret}}},
		{name: "oauth with absolute token url", api: CollibraApiConfig{OAuth: CollibraOAuthConfig{ClientID: testClientID, ClientSecret: testClientSecret, TokenURL: "https://auth.example.com/token"}}},
		{name: "client id without secret", api: CollibraApiConfig{OAuth: CollibraOAuthConfig{ClientID: testClientID}}, wantErr: "both api.oauth.client-id and api.oauth.client-secret"},
		{name: "client secret without id", api: CollibraApiConfig{OAuth: CollibraOAuthConfig{ClientSecret: testClientSecret}}, wantErr: "both api.oauth.client-id and api.oauth.client-secret"},
		{name: "oauth and basic auth together", api: CollibraApiConfig{Username: testUsername, Password: testPassword, OAuth: CollibraOAuthConfig{ClientID: testClientID, ClientSecret: testClientSecret}}, wantErr: "cannot use both OAuth client credentials and basic authentication"},
		{name: "relative token url", api: CollibraApiConfig{OAuth: CollibraOAuthConfig{ClientID: testClientID, ClientSecret: testClientSecret, TokenURL: "/rest/oauth/v2/token"}}, wantErr: "invalid OAuth token URL"},
		{name: "unparseable token url", api: CollibraApiConfig{OAuth: CollibraOAuthConfig{ClientID: testClientID, ClientSecret: testClientSecret, TokenURL: "http://[::1"}}, wantErr: "invalid OAuth token URL"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateOAuthConfig(tt.api)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error = %q, want it to contain %q", err, tt.wantErr)
			}
			if strings.Contains(err.Error(), testClientSecret) {
				t.Errorf("error leaks the client secret: %q", err)
			}
		})
	}
}
