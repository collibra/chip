// Package clients: data_source_docs_client.go fetches connection setup guidance
// (driver class, JDBC connection string format, and per-auth-method connection
// properties) for any of Collibra's ~68 supported data sources, from Collibra's
// public product documentation site.
//
// IMPORTANT — this is a documented gap, not a stable integration:
//
// There is no published/versioned API for this. The URLs and HTML structure below
// were reverse-engineered by reading the JavaScript
// (productresources.collibra.com/docs/catalog-connectors/Content/Resources/JavaScripts/power-catcon.js)
// that Collibra's own docs site uses internally to render its "data source" selector
// widget. Collibra could restructure or remove these pages at any time without
// notice, since they were never intended to be consumed as an API. Treat every
// function here as best-effort: a fetch or parse failure means "fall back to asking
// the user for the connection properties directly," not a hard error condition to
// retry aggressively.
//
// If this breaks: re-derive the URL pattern by fetching
// https://productresources.collibra.com/docs/collibra/latest/Content/Edge/JDBCConnections/ta_create-jdbc-connection.htm?data-source=<slug>
// in a real browser with devtools open, and inspecting network requests fired by
// power-catcon.js's fetchCatConDocs() function.
package clients

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	"golang.org/x/net/html"
)

const catalogConnectorsBaseURL = "https://productresources.collibra.com/docs/catalog-connectors/Content/Resources/Snippets/CatalogSnippets/CatalogConnectors"

// DataSourceSlug identifies one of Collibra's supported data sources in the docs
// site's connector selector (e.g. {ID: "snowflake", Name: "Snowflake"}).
type DataSourceSlug struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// DataSourceInfo is driver/connection-string level documentation for a data source.
type DataSourceInfo struct {
	ID                     string `json:"id"`
	Name                   string `json:"name,omitempty"`
	DriverClassName        string `json:"driverClassName,omitempty"`
	ConnectionStringFormat string `json:"connectionStringFormat,omitempty"`
	MarketplaceURL         string `json:"marketplaceUrl,omitempty"`
	Prerequisites          string `json:"prerequisites,omitempty"`
}

// ConnectionPropertyDoc is one documented connection property row.
type ConnectionPropertyDoc struct {
	Required    bool   `json:"required"`
	Name        string `json:"name"`
	Type        string `json:"type"`
	ValueType   string `json:"valueType"`
	Description string `json:"description,omitempty"`
}

// AuthMethodProperties groups the connection properties documented for one
// authentication method (e.g. "Key pair (unencrypted)", "OAuth").
type AuthMethodProperties struct {
	Name       string                  `json:"name"`
	Properties []ConnectionPropertyDoc `json:"properties"`
}

// ListDataSources fetches the full list of data sources shown in Collibra's docs
// connector selector (68 as of 2026-07-04).
func ListDataSources(ctx context.Context, client *http.Client) ([]DataSourceSlug, error) {
	doc, err := fetchAndParseHTML(ctx, client, catalogConnectorsBaseURL+"/_all-data-sources/selector/selector.htm")
	if err != nil {
		return nil, fmt.Errorf("listing data sources: %w", err)
	}

	var sources []DataSourceSlug
	seen := map[string]bool{}
	for _, div := range htmlFindAll(doc, isElementWithClass("div", "catalogConnector")) {
		id := htmlAttr(div, "id")
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		sources = append(sources, DataSourceSlug{ID: id, Name: strings.TrimSpace(htmlText(div))})
	}

	if len(sources) == 0 {
		return nil, fmt.Errorf("listing data sources: no connectors found — the docs site's page structure may have changed")
	}
	return sources, nil
}

// GetDataSourceInfo fetches driver class name, JDBC connection string format,
// Marketplace listing URL, and prerequisites for a data source, by its slug (as
// returned by ListDataSources).
func GetDataSourceInfo(ctx context.Context, client *http.Client, slug string) (*DataSourceInfo, error) {
	doc, err := fetchAndParseHTML(ctx, client, catalogConnectorsBaseURL+"/"+slug+"/data-source-information.htm")
	if err != nil {
		return nil, fmt.Errorf("getting data source info for %q: %w", slug, err)
	}

	info := &DataSourceInfo{ID: slug}

	if el := htmlFindByID(doc, "catcon-driver"); el != nil {
		info.DriverClassName = strings.TrimSpace(htmlText(el))
	}
	if el := htmlFindByID(doc, "catcon-jdbc-url"); el != nil {
		if code := htmlFind(el, isElement("code")); code != nil {
			info.ConnectionStringFormat = strings.TrimSpace(htmlText(code))
		}
	}
	if el := htmlFindByID(doc, "catcon-marketplace"); el != nil {
		if a := htmlFind(el, isElement("a")); a != nil {
			info.MarketplaceURL = htmlAttr(a, "href")
		}
	}
	if el := htmlFindByID(doc, "catcon-prerequisites"); el != nil {
		info.Prerequisites = collapseWhitespace(htmlText(el))
	}

	if info.DriverClassName == "" && info.ConnectionStringFormat == "" {
		return nil, fmt.Errorf("getting data source info for %q: no recognizable content found — the docs site's page structure may have changed, or %q is not a valid slug", slug, slug)
	}

	return info, nil
}

// GetConnectionProperties fetches the documented connection properties for a data
// source, grouped by authentication method (e.g. a Snowflake connection documents
// separate property sets for "Username and password", "Key pair (unencrypted)",
// "OAuth", etc.). Use this to know exactly which named properties to pass as
// create_connection's additionalProperties for a given auth method.
func GetConnectionProperties(ctx context.Context, client *http.Client, slug string) ([]AuthMethodProperties, error) {
	doc, err := fetchAndParseHTML(ctx, client, catalogConnectorsBaseURL+"/"+slug+"/minimum-connection-properties-edge.htm")
	if err != nil {
		return nil, fmt.Errorf("getting connection properties for %q: %w", slug, err)
	}

	const subselectorPrefix = "data-sources/catalog-connector-subselectors."

	// Map auth-method key (e.g. "key-pair-unencrypted") to its human-readable name,
	// read from the radio button selector's <input value="...subselectors.KEY"> and
	// sibling <span>NAME</span>.
	names := map[string]string{}
	for _, input := range htmlFindAll(doc, isElement("input")) {
		value := htmlAttr(input, "value")
		if !strings.HasPrefix(value, subselectorPrefix) {
			continue
		}
		key := strings.TrimPrefix(value, subselectorPrefix)
		if label := input.Parent; label != nil {
			if span := htmlFind(label, isElement("span")); span != nil {
				names[key] = strings.TrimSpace(htmlText(span))
			}
		}
	}

	var methods []AuthMethodProperties
	seen := map[string]bool{}
	for _, div := range htmlFindAll(doc, isElement("div")) {
		cond := htmlAttr(div, "data-mc-conditions")
		if cond == "" {
			continue
		}

		var key string
		for _, tok := range strings.Split(cond, ",") {
			tok = strings.TrimSpace(tok)
			if k, ok := strings.CutPrefix(tok, subselectorPrefix); ok && k != "all" {
				key = k
				break
			}
		}
		if key == "" || seen[key] {
			continue
		}

		table := htmlFind(div, isElement("table"))
		if table == nil {
			continue
		}
		thead := htmlFind(table, isElement("thead"))
		if thead == nil || !strings.Contains(strings.ToLower(htmlText(thead)), "required") {
			// Not a connection-properties table (this div's condition may gate some
			// other kind of content) — skip rather than misparse it.
			continue
		}

		seen[key] = true
		name := names[key]
		if name == "" {
			name = key
		}
		methods = append(methods, AuthMethodProperties{Name: name, Properties: parsePropertyRows(table)})
	}

	if len(methods) == 0 {
		return nil, fmt.Errorf("getting connection properties for %q: no auth-method property tables found — the docs site's page structure may have changed, or this data source may not document any (some sources need no extra properties beyond the driver/connection string)", slug)
	}

	return methods, nil
}

func parsePropertyRows(table *html.Node) []ConnectionPropertyDoc {
	tbody := htmlFind(table, isElement("tbody"))
	if tbody == nil {
		return nil
	}

	var props []ConnectionPropertyDoc
	for _, tr := range directChildren(tbody, "tr") {
		tds := directChildren(tr, "td")
		if len(tds) < 5 {
			continue
		}

		required := false
		if img := htmlFind(tds[0], isElement("img")); img != nil {
			required = strings.EqualFold(htmlAttr(img, "alt"), "yes")
		} else {
			required = strings.EqualFold(strings.TrimSpace(htmlText(tds[0])), "yes")
		}

		props = append(props, ConnectionPropertyDoc{
			Required:    required,
			Name:        strings.TrimSpace(htmlText(tds[1])),
			Type:        strings.TrimSpace(htmlText(tds[2])),
			ValueType:   strings.TrimSpace(htmlText(tds[3])),
			Description: collapseWhitespace(htmlText(tds[4])),
		})
	}
	return props
}

func fetchAndParseHTML(ctx context.Context, client *http.Client, url string) (*html.Node, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("building request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("sending request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("not found (%s)", url)
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
	}

	doc, err := html.Parse(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("parsing HTML: %w", err)
	}
	return doc, nil
}

// --- Minimal HTML tree helpers (avoids pulling in a full CSS-selector library for
// what is otherwise a handful of narrow, structure-specific lookups). ---

func htmlFindAll(n *html.Node, match func(*html.Node) bool) []*html.Node {
	var out []*html.Node
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if match(n) {
			out = append(out, n)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return out
}

func htmlFind(n *html.Node, match func(*html.Node) bool) *html.Node {
	if match(n) {
		return n
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if found := htmlFind(c, match); found != nil {
			return found
		}
	}
	return nil
}

func htmlFindByID(n *html.Node, id string) *html.Node {
	return htmlFind(n, func(n *html.Node) bool {
		return n.Type == html.ElementNode && htmlAttr(n, "id") == id
	})
}

func isElement(tag string) func(*html.Node) bool {
	return func(n *html.Node) bool { return n.Type == html.ElementNode && n.Data == tag }
}

func isElementWithClass(tag, class string) func(*html.Node) bool {
	return func(n *html.Node) bool {
		if n.Type != html.ElementNode || n.Data != tag {
			return false
		}
		for _, c := range strings.Fields(htmlAttr(n, "class")) {
			if c == class {
				return true
			}
		}
		return false
	}
}

// directChildren returns n's immediate element children matching tag (not a
// recursive search) — used for table row/cell parsing where a cell's own content
// (e.g. a collapsible "steps" dropdown) could otherwise be mistaken for sibling rows.
func directChildren(n *html.Node, tag string) []*html.Node {
	var out []*html.Node
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode && c.Data == tag {
			out = append(out, c)
		}
	}
	return out
}

func htmlAttr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

func htmlText(n *html.Node) string {
	var sb strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.TextNode {
			sb.WriteString(n.Data)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return sb.String()
}

func collapseWhitespace(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
