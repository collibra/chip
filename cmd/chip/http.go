package main

import (
	"crypto/rand"
	"crypto/tls"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"time"

	"github.com/collibra/chip/pkg/chip"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type collibraClient struct {
	config *Config
	next   http.RoundTripper
}

func newCollibraClient(config *Config) *http.Client {
	baseTransport := &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   60 * time.Second,
			KeepAlive: 60 * time.Second,
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
			config: config,
			next:   chip.NewCollibraClient(baseTransport),
		},
	}
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
	if c.config.Api.Username != "" && c.config.Api.Password != "" {
		reqClone.SetBasicAuth(c.config.Api.Username, c.config.Api.Password)
	} else {
		copyHeader(toolRequest, reqClone, "Authorization")
	}
	reqClone.Header.Set("X-MCP-Session-Id", chip.GetSessionId(reqClone.Context()))
	reqClone.Header.Set("X-MCP-Tool-Name", toolRequest.Params.Name)
	reqClone.Header.Set("traceparent", generateTraceParent())
	reqClone.URL.Scheme = baseURL.Scheme
	reqClone.URL.Host = baseURL.Host
	reqClone.URL.Path = path.Join(baseURL.Path, request.URL.Path)
	return c.next.RoundTrip(reqClone)
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
