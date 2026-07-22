package get_data_source_setup_guide_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	tools "github.com/collibra/chip/pkg/tools/get_data_source_setup_guide"
	"github.com/collibra/chip/pkg/tools/testutil"
)

const selectorFixture = `<html><body>
<ul id="interactiveElement">
<li><div id="snowflake" class="catalogConnector"><img class="catalogConnectorIcon"/><div>Snowflake</div></div></li>
<li><div id="postgresql" class="catalogConnector"><img class="catalogConnectorIcon"/><div>PostgreSQL</div></div></li>
<li><div id="postgresql-classic" class="catalogConnector"><img class="catalogConnectorIcon"/><div>PostgreSQL Classic</div></div></li>
</ul>
</body></html>`

const dataSourceInfoFixture = `<html><body>
<h4>The JDBC connection string:</h4>
<div id="catcon-jdbc-url"><p><code>jdbc:snowflake://&lt;accountname&gt;.snowflakecomputing.com</code></p></div>
<h4>The JDBC driver class name:</h4>
<div><code id="catcon-driver">net.snowflake.client.jdbc.SnowflakeDriver</code></div>
</body></html>`

const connectionPropertiesFixture = `<html><body>
<div data-mc-conditions="data-sources/catalog-connector-subselectors.credentials">
<table><thead><tr><th><p>Required?</p></th><th><p>Name</p></th><th><p>Type</p></th><th><p>Value Type</p></th><th><p>Value</p></th></tr></thead>
<tbody><tr><td><img src="ok.png" alt="Yes"/></td><td>Password</td><td>Text</td><td>Secret</td><td>The password.</td></tr></tbody></table>
</div>
</body></html>`

func newTestServer(t *testing.T) *http.Client {
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
	mux.HandleFunc("GET /docs/catalog-connectors/Content/Resources/Snippets/CatalogSnippets/CatalogConnectors/postgresql/data-source-information.htm", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(dataSourceInfoFixture))
	})
	mux.HandleFunc("GET /docs/catalog-connectors/Content/Resources/Snippets/CatalogSnippets/CatalogConnectors/postgresql/minimum-connection-properties-edge.htm", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(connectionPropertiesFixture))
	})
	return testutil.NewClient(httptest.NewServer(mux))
}

func TestGetDataSourceSetupGuide_Success(t *testing.T) {
	tools.DocsClient = newTestServer(t)

	output, err := tools.NewTool(nil).Handler(t.Context(), tools.Input{DataSource: "Snowflake"})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !output.Success {
		t.Fatalf("expected success, got error: %s", output.Error)
	}
	if output.DataSourceID != "snowflake" {
		t.Fatalf("unexpected data source id: %s", output.DataSourceID)
	}
	if output.DriverClassName != "net.snowflake.client.jdbc.SnowflakeDriver" {
		t.Fatalf("unexpected driver class: %s", output.DriverClassName)
	}
	if len(output.AuthMethods) != 1 || len(output.AuthMethods[0].Properties) != 1 {
		t.Fatalf("unexpected auth methods: %+v", output.AuthMethods)
	}
}

func TestGetDataSourceSetupGuide_CaseInsensitiveMatch(t *testing.T) {
	tools.DocsClient = newTestServer(t)

	output, err := tools.NewTool(nil).Handler(t.Context(), tools.Input{DataSource: "snowflake"})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !output.Success {
		t.Fatalf("expected success, got error: %s", output.Error)
	}
}

func TestGetDataSourceSetupGuide_Ambiguous(t *testing.T) {
	tools.DocsClient = newTestServer(t)

	output, err := tools.NewTool(nil).Handler(t.Context(), tools.Input{DataSource: "postgresql"})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	// "postgresql" exact-matches one id but is also a substring of "postgresql-classic";
	// exact match should win outright rather than reporting ambiguity.
	if !output.Success || output.DataSourceID != "postgresql" {
		t.Fatalf("expected exact id match to win, got: %+v", output)
	}
}

func TestGetDataSourceSetupGuide_TrulyAmbiguous(t *testing.T) {
	tools.DocsClient = newTestServer(t)

	output, err := tools.NewTool(nil).Handler(t.Context(), tools.Input{DataSource: "postgres"})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if output.Success {
		t.Fatalf("expected failure due to ambiguity")
	}
	if len(output.Matches) != 2 {
		t.Fatalf("expected 2 candidate matches, got: %+v", output.Matches)
	}
}

func TestGetDataSourceSetupGuide_NotFound(t *testing.T) {
	tools.DocsClient = newTestServer(t)

	output, err := tools.NewTool(nil).Handler(t.Context(), tools.Input{DataSource: "sap-hana"})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if output.Success {
		t.Fatalf("expected failure for unknown data source")
	}
	if len(output.Matches) != 3 {
		t.Fatalf("expected all 3 known sources returned as candidates, got: %+v", output.Matches)
	}
}

func TestGetDataSourceSetupGuide_EmptyInput(t *testing.T) {
	tools.DocsClient = newTestServer(t)

	_, err := tools.NewTool(nil).Handler(t.Context(), tools.Input{DataSource: ""})
	if err == nil {
		t.Fatalf("expected an error for empty dataSource")
	}
}
