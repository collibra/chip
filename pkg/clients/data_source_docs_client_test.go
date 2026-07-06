package clients_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/collibra/chip/pkg/clients"
	"github.com/collibra/chip/pkg/tools/testutil"
)

const selectorFixture = `<html><body>
<ul id="interactiveElement">
<li><div id="snowflake" class="catalogConnector" data-mc-conditions="data-sources/catalog-connector-types.big-data-and-nosql"><img class="catalogConnectorIcon"/><div>Snowflake</div></div></li>
<li><div id="postgresql" class="catalogConnector" data-mc-conditions="data-sources/catalog-connector-types.rdbms"><img class="catalogConnectorIcon"/><div>PostgreSQL</div></div></li>
<li><div class="hidden catalogConnector" onclick="showPopup()"><img class="catalogConnectorIcon"/></div></li>
</ul>
</body></html>`

const dataSourceInfoFixture = `<html><body>
<div id="mc-main-content">
<h4>The JDBC connection string:</h4>
<div id="catcon-jdbc-url"><p><code>jdbc:snowflake://&lt;accountname&gt;.snowflakecomputing.com</code></p></div>
<h4>The JDBC driver class name:</h4>
<div><code id="catcon-driver">net.snowflake.client.jdbc.SnowflakeDriver</code></div>
<h4>The Collibra Marketplace listing URL:</h4>
<div><span id="catcon-marketplace"><a href="https://marketplace.collibra.com/listings/snowflake">link</a></span></div>
<h4>The connection prerequisites:</h4>
<div id="catcon-prerequisites"><li>Download the driver from Marketplace.</li></div>
</div>
</body></html>`

const connectionPropertiesFixture = `<html><body>
<table><tbody><tr><td>
<label class="radio-button"><input value="data-sources/catalog-connector-subselectors.credentials" name="jdbc-driver-authentication" type="radio"/><span>Username and password</span></label>
<label class="radio-button"><input value="data-sources/catalog-connector-subselectors.key-pair" type="radio"/><span>Key pair (encrypted)</span></label>
</td></tr></tbody></table>
<div data-mc-conditions="data-sources/catalog-connector-subselectors.credentials,data-sources/edge-jobserver.edge">
<p>To establish a connection using username and password authentication.</p>
<table><thead><tr><th><p>Required?</p></th><th><p>Name</p></th><th><p>Type</p></th><th><p>Value Type</p></th><th><p>Value</p></th></tr></thead>
<tbody>
<tr><td><img src="ok.png" alt="Yes"/></td><td>Password</td><td>Text</td><td>Secret</td><td>The password.</td></tr>
<tr><td><img src="close.png" alt="No"/></td><td>role</td><td>Text</td><td>Plaintext</td><td>The role.</td></tr>
</tbody></table>
</div>
<div data-mc-conditions="data-sources/catalog-connector-subselectors.key-pair,data-sources/edge-jobserver.edge">
<table><thead><tr><th><p>Required?</p></th><th><p>Name</p></th><th><p>Type</p></th><th><p>Value Type</p></th><th><p>Value</p></th></tr></thead>
<tbody>
<tr><td><img src="ok.png" alt="Yes"/></td><td>private_key_file</td><td>File</td><td>Plaintext</td><td>The key file.</td></tr>
</tbody></table>
</div>
</body></html>`

func newDocsTestServer(t *testing.T) *http.Client {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /docs/catalog-connectors/Content/Resources/Snippets/CatalogSnippets/CatalogConnectors/_all-data-sources/selector/selector.htm", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(selectorFixture))
	})
	mux.HandleFunc("GET /docs/catalog-connectors/Content/Resources/Snippets/CatalogSnippets/CatalogConnectors/snowflake/data-source-information.htm", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(dataSourceInfoFixture))
	})
	mux.HandleFunc("GET /docs/catalog-connectors/Content/Resources/Snippets/CatalogSnippets/CatalogConnectors/snowflake/minimum-connection-properties-edge.htm", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(connectionPropertiesFixture))
	})
	mux.HandleFunc("GET /docs/catalog-connectors/Content/Resources/Snippets/CatalogSnippets/CatalogConnectors/does-not-exist/data-source-information.htm", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	return testutil.NewClient(httptest.NewServer(mux))
}

func TestListDataSources(t *testing.T) {
	client := newDocsTestServer(t)
	sources, err := clients.ListDataSources(t.Context(), client)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if len(sources) != 2 {
		t.Fatalf("expected 2 data sources (hidden placeholder excluded), got %d: %+v", len(sources), sources)
	}
	if sources[0].ID != "snowflake" || sources[0].Name != "Snowflake" {
		t.Fatalf("unexpected first source: %+v", sources[0])
	}
}

func TestGetDataSourceInfo(t *testing.T) {
	client := newDocsTestServer(t)
	info, err := clients.GetDataSourceInfo(t.Context(), client, "snowflake")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if info.DriverClassName != "net.snowflake.client.jdbc.SnowflakeDriver" {
		t.Fatalf("unexpected driver class: %s", info.DriverClassName)
	}
	if info.ConnectionStringFormat != "jdbc:snowflake://<accountname>.snowflakecomputing.com" {
		t.Fatalf("unexpected connection string: %s", info.ConnectionStringFormat)
	}
	if info.MarketplaceURL != "https://marketplace.collibra.com/listings/snowflake" {
		t.Fatalf("unexpected marketplace url: %s", info.MarketplaceURL)
	}
	if info.Prerequisites == "" {
		t.Fatalf("expected prerequisites to be populated")
	}
}

func TestGetDataSourceInfo_NotFound(t *testing.T) {
	client := newDocsTestServer(t)
	_, err := clients.GetDataSourceInfo(t.Context(), client, "does-not-exist")
	if err == nil {
		t.Fatalf("expected an error for a 404")
	}
}

func TestGetConnectionProperties(t *testing.T) {
	client := newDocsTestServer(t)
	methods, err := clients.GetConnectionProperties(t.Context(), client, "snowflake")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if len(methods) != 2 {
		t.Fatalf("expected 2 auth methods, got %d: %+v", len(methods), methods)
	}

	byName := map[string]clients.AuthMethodProperties{}
	for _, m := range methods {
		byName[m.Name] = m
	}

	userPass, ok := byName["Username and password"]
	if !ok {
		t.Fatalf("expected 'Username and password' auth method, got: %+v", methods)
	}
	if len(userPass.Properties) != 2 {
		t.Fatalf("expected 2 properties, got: %+v", userPass.Properties)
	}
	if !userPass.Properties[0].Required || userPass.Properties[0].Name != "Password" || userPass.Properties[0].ValueType != "Secret" {
		t.Fatalf("unexpected Password property: %+v", userPass.Properties[0])
	}
	if userPass.Properties[1].Required {
		t.Fatalf("expected role to be not required: %+v", userPass.Properties[1])
	}

	keyPair, ok := byName["Key pair (encrypted)"]
	if !ok {
		t.Fatalf("expected 'Key pair (encrypted)' auth method, got: %+v", methods)
	}
	if len(keyPair.Properties) != 1 || keyPair.Properties[0].Name != "private_key_file" || keyPair.Properties[0].Type != "File" {
		t.Fatalf("unexpected key pair properties: %+v", keyPair.Properties)
	}
}
