package chip

import (
	"net/http"
)

type CollibraClient struct {
	next http.RoundTripper
}

func (c *CollibraClient) RoundTrip(request *http.Request) (*http.Response, error) {
	reqClone := request.Clone(request.Context())
	if reqClone.Header.Get("Content-Type") == "" {
		// If not set, default to application/json, but avoid overriding if it's already set.
		reqClone.Header.Set("Content-Type", "application/json")
	}
	reqClone.Header.Set("User-Agent", "Collibra MCP/"+Version)
	return c.next.RoundTrip(reqClone)
}

func NewCollibraClient(transport http.RoundTripper) *CollibraClient {
	return &CollibraClient{
		next: transport,
	}
}
