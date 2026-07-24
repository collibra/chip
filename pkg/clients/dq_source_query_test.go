package clients

import (
	"strings"
	"testing"
)

func TestBuildDqSourceQueryPostgresComposesAll(t *testing.T) {
	q := BuildDqSourceQuery(DqSourceQueryInput{
		DatabaseProduct: "POSTGRES",
		SchemaName:      "sales",
		TableName:       "customers",
		SelectedColumns: []string{"id", "balance"},
		FilterColumn:    "is_active",
		FilterOperator:  "=",
		FilterValue:     "true",
		TimeSliceColumn: "created_at",
		SampleSize:      137,
	})
	for _, want := range []string{
		`"id", "balance"`,          // column subset, escaped
		`FROM "sales"."customers"`, // escaped relation
		`"is_active" = 'true'`,     // filter, value single-quoted
		`"created_at" >= '${rd}' AND "created_at" < '${rdEnd}'`, // time slice
		`RANDOM() < (137::float`,                                // postgres sampling
		`LIMIT 137`,
	} {
		if !strings.Contains(q, want) {
			t.Errorf("postgres query missing %q\n got: %s", want, q)
		}
	}
	// The sample's total_rows count must include the filter+slice (sample of the filtered subset).
	if !strings.Contains(q, `total_rows AS (SELECT COUNT(*) AS total FROM "sales"."customers" WHERE "created_at" >= '${rd}'`) {
		t.Errorf("expected total_rows count to include the WHERE predicate, got: %s", q)
	}
}

func TestBuildDqSourceQueryDialectSampling(t *testing.T) {
	cases := map[string]string{
		"SNOWFLAKE": "TABLESAMPLE (50 ROWS)",
		"SQLSERVER": "SELECT TOP 50",
		"DB2":       "FETCH FIRST 50 ROWS ONLY",
		"MYSQL":     "LIMIT 50",
		"ORACLE":    "ORDER BY DBMS_RANDOM.VALUE",
	}
	for dialect, want := range cases {
		q := BuildDqSourceQuery(DqSourceQueryInput{
			DatabaseProduct: dialect, SchemaName: "s", TableName: "t", SampleSize: 50,
		})
		if !strings.Contains(q, want) {
			t.Errorf("%s: expected %q in query, got: %s", dialect, want, q)
		}
	}
}

func TestBuildDqSourceQueryEscapingAndBigQuery(t *testing.T) {
	// SQL Server bracket-escapes identifiers.
	q := BuildDqSourceQuery(DqSourceQueryInput{DatabaseProduct: "SQLSERVER", SchemaName: "dbo", TableName: "orders", SelectedColumns: []string{"id"}})
	if !strings.Contains(q, "[dbo].[orders]") || !strings.Contains(q, "[id]") {
		t.Errorf("sqlserver escaping wrong: %s", q)
	}
	// BigQuery wraps the relation in backticks and leaves numeric filter values bare.
	q = BuildDqSourceQuery(DqSourceQueryInput{
		DatabaseProduct: "BIGQUERY", SchemaName: "ds", TableName: "evt",
		FilterColumn: "amount", FilterOperator: ">", FilterValue: "100",
	})
	if !strings.Contains(q, "`ds.evt`") {
		t.Errorf("bigquery relation should be backtick-wrapped: %s", q)
	}
	if !strings.Contains(q, "amount > 100") || strings.Contains(q, "amount > '100'") {
		t.Errorf("bigquery numeric filter should be bare: %s", q)
	}
}

func TestBuildDqSourceQueryDoesNotDoubleQuote(t *testing.T) {
	// A caller that pre-quotes the value must not produce ''US'' (the live failure).
	for _, v := range []string{"US", "'US'"} {
		q := BuildDqSourceQuery(DqSourceQueryInput{
			DatabaseProduct: "POSTGRES", SchemaName: "s", TableName: "t",
			FilterColumn: "country", FilterOperator: "=", FilterValue: v,
		})
		if !strings.Contains(q, `"country" = 'US'`) || strings.Contains(q, `''US''`) {
			t.Errorf("value %q: expected single-quoted 'US', got: %s", v, q)
		}
	}
}

func TestBuildDqSourceQueryPlainWhenNoOptions(t *testing.T) {
	q := BuildDqSourceQuery(DqSourceQueryInput{DatabaseProduct: "POSTGRES", SchemaName: "sales", TableName: "customers"})
	if q != `SELECT * FROM "sales"."customers"` {
		t.Errorf("unexpected plain query: %q", q)
	}
}

func TestBuildDqSourceQueryEscapesFilterValueQuotes(t *testing.T) {
	// A value carrying a single quote must NOT break out of the string literal — the quote is doubled.
	q := BuildDqSourceQuery(DqSourceQueryInput{
		DatabaseProduct: "POSTGRES",
		SchemaName:      "sales",
		TableName:       "customers",
		FilterColumn:    "country",
		FilterOperator:  "=",
		FilterValue:     "x' OR '1'='1",
	})
	if !strings.Contains(q, `"country" = 'x'' OR ''1''=''1'`) {
		t.Fatalf("filter value not escaped as a literal: %q", q)
	}
	// The raw un-escaped break-out form must be absent.
	if strings.Contains(q, `'x' OR '1'='1'`) {
		t.Fatalf("query contains an unescaped break-out: %q", q)
	}
}

func TestBuildDqSourceQueryStripsWrappingThenEscapes(t *testing.T) {
	// A caller-wrapped value has its outer pair stripped, then embedded quotes are escaped.
	q := BuildDqSourceQuery(DqSourceQueryInput{
		DatabaseProduct: "POSTGRES", SchemaName: "s", TableName: "t",
		FilterColumn: "name", FilterOperator: "=", FilterValue: "'O'Brien'",
	})
	if !strings.Contains(q, `"name" = 'O''Brien'`) {
		t.Fatalf("expected stripped-then-escaped literal, got %q", q)
	}
}

func TestIsAllowedDqFilterOperator(t *testing.T) {
	for _, ok := range []string{"=", "!=", "<>", ">", ">=", "<", "<=", "LIKE", "like", "IS NULL", "is null", "IS  NOT  NULL"} {
		if !IsAllowedDqFilterOperator(ok) {
			t.Errorf("expected %q to be allowed", ok)
		}
	}
	for _, bad := range []string{"", "==", "= 1 OR 1=1", "IN", "; DROP TABLE t", "LIKE '%x%' OR 1=1"} {
		if IsAllowedDqFilterOperator(bad) {
			t.Errorf("expected %q to be rejected", bad)
		}
	}
}
