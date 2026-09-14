package main

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"time"

	"github.com/collibra/chip/pkg/chip"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/clientcredentials"
)

const defaultOAuthTokenPath = "/rest/oauth/v2/token"

type collibraClient struct {
	config      *Config
	tokenSource oauth2.TokenSource // nil unless OAuth client credentials are configured
	next        http.RoundTripper
}

func newCollibraClient(config *Config) *http.Client {
	baseTransport := &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   60 * time.Second,
			KeepAlive: 60 * time.Second,
			DualStack: true,
		}).DialContext,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 10 * time.Second,
	}

	if config.Api.SkipTLSVerify {
		slog.Warn(fmt.Sprintf("Skipping TLS certificate verification for %s", config.Api.Url))
		baseTransport.TLSClientConfig = &tls.Config{InsecureSkipVerify: config.Api.SkipTLSVerify}
	}

	if config.Api.Proxy != "" {
		proxyURL, err := url.Parse(config.Api.Proxy)
		if err != nil {
			slog.Error(fmt.Sprintf("Invalid proxy URL: %s", err))
			os.Exit(1)
		}
		slog.Info(fmt.Sprintf("Using proxy URL: %s", proxyURL))
		baseTransport.Proxy = http.ProxyURL(proxyURL)
	}

	return &http.Client{
		Transport: &collibraClient{
			config:      config,
			tokenSource: newOAuthTokenSource(config, baseTransport),
			next:        chip.NewCollibraClient(baseTransport),
		},
	}
}

// newOAuthTokenSource returns a caching token source for the OAuth 2.0 client
// credentials grant, or nil when OAuth is not configured. The token request
// goes through baseTransport so that proxy and TLS settings apply to it too.
func newOAuthTokenSource(config *Config, baseTransport http.RoundTripper) oauth2.TokenSource {
	if config.Api.OAuth.ClientID == "" || config.Api.OAuth.ClientSecret == "" {
		return nil
	}
	cc := &clientcredentials.Config{
		ClientID:     config.Api.OAuth.ClientID,
		ClientSecret: config.Api.OAuth.ClientSecret,
		TokenURL:     resolveOAuthTokenURL(config.Api.Url, config.Api.OAuth.TokenURL),
		AuthStyle:    oauth2.AuthStyleInHeader, // Collibra expects Basic credentials on the token endpoint
	}
	ctx := context.WithValue(context.Background(), oauth2.HTTPClient, &http.Client{
		Transport: baseTransport,
		Timeout:   30 * time.Second,
	})
	// clientcredentials.Config.TokenSource already wraps the source in
	// oauth2.ReuseTokenSource, which caches the token and refreshes it before expiry.
	return cc.TokenSource(ctx)
}

func resolveOAuthTokenURL(apiURL, tokenURL string) string {
	if tokenURL != "" {
		return tokenURL
	}
	return strings.TrimSuffix(apiURL, "/") + defaultOAuthTokenPath
}

func (c *collibraClient) RoundTrip(request *http.Request) (*http.Response, error) {
	if c.config.Api.Url == "" {
		return nil, fmt.Errorf("API URL is not configured")
	}
	baseURL, err := url.Parse(c.config.Api.Url)
	if err != nil {
		return nil, fmt.Errorf("invalid API URL configuration: %w", err)
	}
	reqClone := request.Clone(request.Context())
	toolRequest, ok := chip.GetCallToolRequest(reqClone.Context())
	if !ok {
		return nil, fmt.Errorf("toolRequest not found in ctx")
	}
	if err := c.setAuthorization(toolRequest, reqClone); err != nil {
		return nil, err
	}
	reqClone.Header.Set("X-MCP-Session-Id", chip.GetSessionId(reqClone.Context()))
	reqClone.Header.Set("X-MCP-Tool-Name", toolRequest.Params.Name)
	reqClone.Header.Set("traceparent", generateTraceParent())
	reqClone.URL.Scheme = baseURL.Scheme
	reqClone.URL.Host = baseURL.Host
	reqClone.URL.Path = path.Join(baseURL.Path, request.URL.Path)
	return c.next.RoundTrip(reqClone)
}

// setAuthorization applies the configured authentication mode, in order of
// precedence: OAuth client credentials, server-wide basic auth, then the
// Authorization header supplied by the MCP client.
func (c *collibraClient) setAuthorization(toolRequest *mcp.CallToolRequest, httpRequest *http.Request) error {
	switch {
	case c.tokenSource != nil:
		token, err := c.tokenSource.Token()
		if err != nil {
			return fmt.Errorf("failed to obtain OAuth access token: %w", sanitizeOAuthError(err))
		}
		token.SetAuthHeader(httpRequest)
	case c.config.Api.Username != "" && c.config.Api.Password != "":
		httpRequest.SetBasicAuth(c.config.Api.Username, c.config.Api.Password)
	default:
		copyHeader(toolRequest, httpRequest, "Authorization")
	}
	return nil
}

// sanitizeOAuthError strips the token endpoint response body from oauth2
// errors so that it can never leak into logs or tool results. Only the HTTP
// status and the RFC 6749 error code are kept.
func sanitizeOAuthError(err error) error {
	var retrieveErr *oauth2.RetrieveError
	if !errors.As(err, &retrieveErr) {
		return err
	}
	msg := "token endpoint request failed"
	if retrieveErr.Response != nil {
		msg = fmt.Sprintf("token endpoint returned HTTP %d", retrieveErr.Response.StatusCode)
	}
	if retrieveErr.ErrorCode != "" {
		msg += fmt.Sprintf(" (%s)", retrieveErr.ErrorCode)
	}
	return errors.New(msg)
}

func generateTraceParent() string {
	traceID := make([]byte, 16)
	spanID := make([]byte, 8)

	_, _ = rand.Read(traceID)
	_, _ = rand.Read(spanID)

	return fmt.Sprintf("00-%x-%x-01", traceID, spanID)
}

func copyHeader(toolRequest *mcp.CallToolRequest, httpRequest *http.Request, header string) {
	extra := toolRequest.GetExtra()
	if extra == nil || extra.Header == nil {
		return
	}
	if values, exists := extra.Header[header]; exists {
		for _, value := range values {
			httpRequest.Header.Add(header, value)
		}
	}
}
