package clients

import (
	"fmt"
	"strconv"
	"strings"
)

// ─────────────────────────────────────────────────────────────────────────────
// Dialect-aware DQ source-query builder — PORTED from the wizard.
//
// WHY THIS EXISTS / WHY IT'S A DUPLICATE:
// The DQ engine scans from the job's `sourceQuery` (opt_load.query). The structured
// rowFilter / sampleSetting fields on the create request are persisted to job_settings
// for UI re-display but are NOT compiled into the scan (verified live: a structured
// rowFilter never reaches opt_load.filter; sampleSetting never reaches opt_load.sample).
// So column-selection, row-filter, time-slice and sampling only take effect if they are
// written into `sourceQuery`. The ONLY complete implementation that composes all four,
// per database dialect, is the frontend wizard's `applyTimeSliceAndSampling`.
//
// This is a faithful Go port of that function (+ its helpers). It is a THIRD copy of the
// same logic and WILL drift unless maintained. Keep it in sync with — or, better, delete it
// once the server composes the query — these canonical sources:
//   - FE: frontend/libs/bound-shared/data-quality/src/job-explorer/utils/utils.ts
//         (applyTimeSliceAndSampling, createColumnsPart, createTimeSlicePart, createRowFilterPart)
//         + .../utils/common.tsx (escapeColumnName) + constants/shared-constants.ts (SQL_ESCAPE_CHARACTERS)
//   - BE (partial, sample-only): dq common-domain CommonJavaSqlUtils (getQueryWithSample/buildSampledQuery)
//
// The proper fix is server-side: have JobDefinitionMapperPublicV1 / getQueryWithSample compose
// filter+slice+sample from the structured fields, then this file goes away.
// ─────────────────────────────────────────────────────────────────────────────

// rd/rdEnd substitution tokens. Kept single-quoted in the SQL (per the public OAS guidance and
// verified persisted form); the scheduler/engine replaces them per run.
const (
	dqRunIDToken    = "'${rd}'"
	dqRunIDEndToken = "'${rdEnd}'"
)

// DqSourceQueryInput is everything needed to build the scan query. Filter is enabled when
// FilterColumn+FilterOperator are set; time-slice when TimeSliceColumn is set; sampling when
// SampleSize > 0. SelectedColumns empty => SELECT *.
type DqSourceQueryInput struct {
	DatabaseProduct     string // e.g. POSTGRES (connection.databaseProductName)
	SchemaName          string
	TableName           string
	SelectedColumns     []string
	FilterColumn        string
	FilterOperator      string
	FilterValue         string
	TimeSliceColumn     string
	TimeSliceColumnCast string // optional explicit cast expression
	TimeSliceColumnType string // optional source type (drives ATHENA/TRINO/ORACLE casts)
	RunDateFormat       string // "DATE" (default) | "TIMESTAMP"
	SampleSize          int
}

// BuildDqSourceQuery returns the dialect-correct scan SQL, composing column selection, row
// filter, time-slice (${rd}/${rdEnd}) and sampling — mirroring the wizard exactly.
func BuildDqSourceQuery(in DqSourceQueryInput) string {
	dialect := strings.ToUpper(strings.TrimSpace(in.DatabaseProduct))
	columns := dqColumnsPart(dialect, in.SelectedColumns)
	from := dqFromPart(dialect, in.SchemaName, in.TableName)
	where := dqTimeSliceAndRowFilterClause(in, dialect)

	whereClause := ""
	if where != "" {
		whereClause = "WHERE " + where
	}

	if in.SampleSize <= 0 {
		return dqJoin(fmt.Sprintf("SELECT %s", columns), fmt.Sprintf("FROM %s", from), whereClause)
	}

	size := strconv.Itoa(in.SampleSize)
	// For the RANDOM()/RAND() forms the predicate is folded into the sampled WHERE and the
	// total_rows count, so the sample is taken from the filtered+sliced subset (not the table).
	whereWithPrependedSpace := ""
	if where != "" {
		whereWithPrependedSpace = " WHERE " + where
	}
	predicateAnd := ""
	if where != "" {
		predicateAnd = where + " AND"
	}

	switch dialect {
	case "DENODO", "SYBASE":
		return dqJoin(
			"SELECT * FROM (",
			fmt.Sprintf("  SELECT %s, ROW_NUMBER() OVER (ORDER BY 1) AS rn_limit_alias", columns),
			fmt.Sprintf("  FROM %s", from),
			"  "+whereClause,
			") AS temp_table",
			fmt.Sprintf("WHERE rn_limit_alias <= %s", size),
		)
	case "SNOWFLAKE":
		return dqJoin(fmt.Sprintf("SELECT %s", columns), fmt.Sprintf("FROM %s", from),
			fmt.Sprintf("TABLESAMPLE (%s ROWS)", size), whereClause)
	case "SAP", "SQLSERVER", "TERADATA", "AZURESYNAPSE":
		return dqJoin(fmt.Sprintf("SELECT TOP %s %s", size, columns), fmt.Sprintf("FROM %s", from), whereClause)
	case "ATHENA", "DATABRICKS":
		return dqJoin(
			fmt.Sprintf("WITH total_rows AS (SELECT COUNT(*) AS total FROM %s%s)", from, whereWithPrependedSpace),
			fmt.Sprintf("SELECT %s", columns), fmt.Sprintf("FROM %s", from),
			fmt.Sprintf("WHERE %s RAND() < (CAST(%s AS DOUBLE) / CASE WHEN (SELECT total FROM total_rows) = 0 THEN 1 ELSE (SELECT total FROM total_rows) END) LIMIT %s", predicateAnd, size, size),
		)
	case "BIGQUERY":
		return dqJoin(
			fmt.Sprintf("WITH total_rows AS (SELECT COUNT(*) AS total FROM %s%s)", from, whereWithPrependedSpace),
			fmt.Sprintf("SELECT %s", columns), fmt.Sprintf("FROM %s", from),
			fmt.Sprintf("WHERE %s RAND() < (CAST(%s AS FLOAT64) / CASE WHEN (SELECT total FROM total_rows) = 0 THEN 1 ELSE (SELECT total FROM total_rows) END) LIMIT %s", predicateAnd, size, size),
		)
	case "REDSHIFT":
		return dqJoin(
			fmt.Sprintf("WITH total_rows AS (SELECT COUNT(*) AS total FROM %s%s)", from, whereWithPrependedSpace),
			fmt.Sprintf("SELECT %s", columns), fmt.Sprintf("FROM %s", from),
			fmt.Sprintf("WHERE %s RANDOM() < (CAST(%s AS FLOAT) / CASE WHEN (SELECT total FROM total_rows) = 0 THEN 1 ELSE (SELECT total FROM total_rows) END) LIMIT %s", predicateAnd, size, size),
		)
	case "TRINO":
		return dqJoin(
			fmt.Sprintf("WITH total_rows AS (SELECT COUNT(*) AS total FROM %s%s)", from, whereWithPrependedSpace),
			fmt.Sprintf("SELECT %s", columns), fmt.Sprintf("FROM %s", from),
			fmt.Sprintf("WHERE %s RANDOM() < (CAST(%s AS DOUBLE) / CASE WHEN (SELECT total FROM total_rows) = 0 THEN 1 ELSE (SELECT total FROM total_rows) END) limit %s", predicateAnd, size, size),
		)
	case "POSTGRES":
		return dqJoin(
			fmt.Sprintf("WITH total_rows AS (SELECT COUNT(*) AS total FROM %s%s)", from, whereWithPrependedSpace),
			fmt.Sprintf("SELECT %s", columns), fmt.Sprintf("FROM %s", from),
			fmt.Sprintf("WHERE %s RANDOM() < (%s::float / CASE WHEN (SELECT total FROM total_rows) = 0 THEN 1 ELSE (SELECT total FROM total_rows) END) LIMIT %s", predicateAnd, size, size),
		)
	case "DB2":
		return dqJoin(fmt.Sprintf("SELECT %s", columns), fmt.Sprintf("FROM %s", from), whereClause,
			fmt.Sprintf("FETCH FIRST %s ROWS ONLY", size))
	case "ORACLE":
		return dqJoin(fmt.Sprintf("SELECT %s", columns), fmt.Sprintf("FROM %s", from), whereClause,
			"ORDER BY DBMS_RANDOM.VALUE", fmt.Sprintf("FETCH FIRST %s ROWS ONLY", size))
	case "MYSQL":
		return dqJoin("SELECT *", fmt.Sprintf("FROM %s", from), whereClause, fmt.Sprintf("LIMIT %s", size))
	case "HIVE":
		return dqJoin(fmt.Sprintf("SELECT %s", columns), fmt.Sprintf("FROM %s", from), whereClause, fmt.Sprintf("LIMIT %s", size))
	default:
		return dqJoin(fmt.Sprintf("SELECT %s", columns), fmt.Sprintf("FROM %s", from), whereClause, fmt.Sprintf("LIMIT %s", size))
	}
}

// dqTimeSliceAndRowFilterClause builds the combined WHERE body (slice AND filter), each part dialect-aware.
func dqTimeSliceAndRowFilterClause(in DqSourceQueryInput, dialect string) string {
	var slice, filter string
	if strings.TrimSpace(in.TimeSliceColumn) != "" {
		slice = dqTimeSlicePart(in, dialect)
	}
	if strings.TrimSpace(in.FilterColumn) != "" && strings.TrimSpace(in.FilterOperator) != "" {
		filter = dqRowFilterPart(in, dialect)
	}
	switch {
	case slice != "" && filter != "":
		return slice + " AND " + filter
	case slice != "":
		return slice
	default:
		return filter
	}
}

// dqTimeSlicePart mirrors the wizard's createTimeSlicePart (per-dialect casts).
func dqTimeSlicePart(in DqSourceQueryInput, dialect string) string {
	col := strings.TrimSpace(in.TimeSliceColumnCast)
	if col == "" {
		col = dqEscapeIdentifier(dialect, in.TimeSliceColumn)
	}
	dflt := fmt.Sprintf("%s >= %s AND %s < %s", col, dqRunIDToken, col, dqRunIDEndToken)
	ts := strings.ToLower(strings.TrimSpace(in.TimeSliceColumnType))
	isTimestamp := strings.EqualFold(strings.TrimSpace(in.RunDateFormat), "TIMESTAMP")

	switch dialect {
	case "ATHENA":
		if !dqIsDateTimeType(ts) {
			return dflt
		}
		castType := "DATE"
		if isTimestamp {
			castType = "TIMESTAMP"
		}
		return fmt.Sprintf("%s >= cast(%s as %s) AND %s < cast(%s as %s)", col, dqRunIDToken, castType, col, dqRunIDEndToken, castType)
	case "TRINO":
		if ts == "date" {
			return fmt.Sprintf("%s >= CAST(%s AS DATE) AND %s < CAST(%s AS DATE)", col, dqRunIDToken, col, dqRunIDEndToken)
		}
		if ts == "timestamp" {
			return fmt.Sprintf("%s >= CAST(%s AS TIMESTAMP) AND %s < CAST(%s AS TIMESTAMP)", col, dqRunIDToken, col, dqRunIDEndToken)
		}
		return fmt.Sprintf("%s >= TIMESTAMP %s AND %s < TIMESTAMP %s", col, dqRunIDToken, col, dqRunIDEndToken)
	case "TERADATA":
		if isTimestamp {
			return fmt.Sprintf("%s >= TIMESTAMP %s AND %s < TIMESTAMP %s", col, dqRunIDToken, col, dqRunIDEndToken)
		}
		return dflt
	case "ORACLE":
		if !dqIsDateTimeType(ts) {
			return dflt
		}
		if isTimestamp {
			return fmt.Sprintf("%s >= TO_TIMESTAMP(%s, 'YYYY-MM-DD HH24:MI:SS') AND %s < TO_TIMESTAMP(%s, 'YYYY-MM-DD HH24:MI:SS')", col, dqRunIDToken, col, dqRunIDEndToken)
		}
		return fmt.Sprintf("%s >= TO_DATE(%s, 'YYYY-MM-DD HH24:MI:SS') AND %s < TO_DATE(%s, 'YYYY-MM-DD HH24:MI:SS')", col, dqRunIDToken, col, dqRunIDEndToken)
	default:
		return dflt
	}
}

// dqRowFilterPart mirrors the wizard's createRowFilterPart: `col op 'value'` (value single-quoted;
// BIGQUERY bare for numeric). For valueless operators (IS NULL / IS NOT NULL) the value is omitted.
func dqRowFilterPart(in DqSourceQueryInput, dialect string) string {
	col := dqEscapeIdentifier(dialect, in.FilterColumn)
	op := strings.TrimSpace(in.FilterOperator)
	val := strings.TrimSpace(in.FilterValue)
	// The value is meant to be BARE — the builder adds the surrounding single quotes (matching the
	// wizard). Be forgiving if a caller already wrapped it, so 'US' doesn't become ''US'' (a syntax
	// error). Only strips a matched outer pair; embedded quotes are left as-is.
	if len(val) >= 2 && strings.HasPrefix(val, "'") && strings.HasSuffix(val, "'") {
		val = val[1 : len(val)-1]
	}
	if val == "" {
		// e.g. IS NULL / IS NOT NULL — no right-hand value.
		return strings.TrimSpace(fmt.Sprintf("%s %s", col, op))
	}
	if dialect == "BIGQUERY" {
		if _, err := strconv.ParseFloat(val, 64); err == nil {
			return fmt.Sprintf("%s %s %s", col, op, val)
		}
	}
	return fmt.Sprintf("%s %s '%s'", col, op, val)
}

// dqColumnsPart mirrors createColumnsPart: '*' when no subset, else the escaped selected columns.
func dqColumnsPart(dialect string, selected []string) string {
	cols := make([]string, 0, len(selected))
	for _, c := range selected {
		if c = strings.TrimSpace(c); c != "" {
			cols = append(cols, dqEscapeIdentifier(dialect, c))
		}
	}
	if len(cols) == 0 {
		return "*"
	}
	return strings.Join(cols, ", ")
}

// dqFromPart mirrors createFromPart: escaped "schema"."table"; BIGQUERY wraps the whole ref in backticks.
func dqFromPart(dialect, schema, table string) string {
	parts := make([]string, 0, 2)
	if s := dqEscapeIdentifier(dialect, schema); strings.TrimSpace(schema) != "" {
		parts = append(parts, s)
	}
	parts = append(parts, dqEscapeIdentifier(dialect, table))
	from := strings.Join(parts, ".")
	if dialect == "BIGQUERY" {
		return "`" + from + "`"
	}
	return from
}

// dqEscapeIdentifier mirrors escapeColumnName / escapeTableOrSchemaName (enforced) per dialect.
func dqEscapeIdentifier(dialect, name string) string {
	if name == "" {
		return name
	}
	switch dialect {
	case "SNOWFLAKE", "ORACLE", "ATHENA": // '"' quoting; SNOWFLAKE/ORACLE double embedded quotes
		return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
	case "POSTGRES", "REDSHIFT", "DENODO", "SAP":
		return `"` + name + `"`
	case "HIVE":
		return "`" + strings.ReplaceAll(name, "`", "``") + "`"
	case "DATABRICKS":
		return "`" + name + "`"
	case "SQLSERVER", "AZURESYNAPSE", "SYBASE": // bracket quoting; escape only the closing bracket
		return "[" + strings.ReplaceAll(name, "]", "]]") + "]"
	default: // MYSQL, BIGQUERY, DB2, TRINO, TERADATA, … — unescaped (matches the wizard's default)
		return name
	}
}

func dqIsDateTimeType(t string) bool {
	t = strings.ToLower(t)
	return strings.Contains(t, "date") || strings.Contains(t, "time")
}

// dqJoin joins non-empty SQL fragments with a single space and trims (whitespace is SQL-insignificant).
func dqJoin(parts ...string) string {
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if s := strings.TrimSpace(p); s != "" {
			out = append(out, s)
		}
	}
	return strings.Join(out, " ")
}
