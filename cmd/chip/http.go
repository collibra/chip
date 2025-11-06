package main

import (
	"fmt"
	"net/http"
	"net/url"
	"path"

	chip "github.com/collibra/chip/app"
)

type collibraClient struct {
	config *chip.Config
	next   http.RoundTripper
}

func (c *collibraClient) RoundTrip(request *http.Request) (*http.Response, error) {
	reqClone := request.Clone(request.Context())
	toolRequest, err := chip.GetCallToolRequest(reqClone.Context())
	if err != nil {
		return nil, err
	}
	if c.config.Api.Url == "" {
		return nil, fmt.Errorf("API URL is not configured")
	}
	if c.config.Api.Username != "" && c.config.Api.Password != "" {
		reqClone.SetBasicAuth(c.config.Api.Username, c.config.Api.Password)
	} else {
		chip.CopyHeader(toolRequest, reqClone, "Authorization")
	}
	reqClone.Header.Set("X-MCP-Session-Id", chip.GetSessionId(toolRequest))
	baseURL, err := url.Parse(c.config.Api.Url)
	if err != nil {
		return nil, fmt.Errorf("invalid API URL configuration: %w", err)
	}
	toolRequest.Extra.Header.Set("collibraUrl", fmt.Sprintf("%s://%s", baseURL.Scheme, baseURL.Host))
	reqClone.URL.Scheme = baseURL.Scheme
	reqClone.URL.Host = baseURL.Host
	reqClone.URL.Path = path.Join(baseURL.Path, request.URL.Path)
	return c.next.RoundTrip(reqClone)
}
