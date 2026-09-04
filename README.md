# Collibra MCP Server

A Model Context Protocol (MCP) server that provides AI agents with access to Collibra Data Governance Center capabilities including data asset discovery, business glossary queries, and detailed asset information retrieval.

## Overview

This Go-based MCP server acts as a bridge between AI applications and Collibra, enabling intelligent data discovery and governance operations through the following tools:

> **Permissions:** tools that require extra DGC scopes beyond standard catalog read access are flagged with **Requires:**. Tools without that marker work with the default scopes granted to any authenticated Collibra user. If a tool returns a permission error, the connecting user is missing the listed scope(s) — most commonly `dgc.ai-copilot` (for the natural-language discovery tools) and `dgc.classify` + `dgc.catalog` (for the classification tools).

### Read Tools

- [`discover_business_glossary`](pkg/tools/discover_business_glossary/) - Ask questions about terms and definitions. Note that this tool leverages Collibra AI and therefore consumes Collibra Units (CUs). **Requires:** `dgc.ai-copilot`
- [`discover_data_assets`](pkg/tools/discover_data_assets/) - Query available data assets using natural language. Note that this tool leverages Collibra AI and therefore consumes Collibra Units (CUs). **Requires:** `dgc.ai-copilot`
- [`get_assessment`](pkg/tools/get_assessment/) - Retrieve conducted assessment(s) from the Assessments application (these are not catalog assets). Direct lookup of a single assessment by name or UUID (or by its linked Assessment Review asset), or a filtered lookup combining name (partial), status, template, conducted asset, and a last-modified range (paginated)
- [`get_asset_details`](pkg/tools/get_asset_details/) - Retrieve detailed information about specific assets by UUID, including the asset's assignable attribute schema (every attribute it can hold, including empty ones)
- [`get_business_term_data`](pkg/tools/get_business_term_data/) - Trace a business term back to its connected physical data assets
- [`get_column_semantics`](pkg/tools/get_column_semantics/) - Retrieve data attributes, measures, and business assets connected to a column
- [`get_data_quality_rule`](pkg/tools/get_dq_rule/) - Read the definition of a single DQ rule (monitor) on a job — its type, SQL, filter, tolerance and active/suppressed state
- [`get_data_quality_rule_results`](pkg/tools/get_dq_rule_results/) - Read a rule's per-run results after a job run — score, breaking/passing record counts, pass/fail status and any exception. Paginated (`offset`/`limit`), newest first by default
- [`validate_data_quality_rule`](pkg/tools/validate_dq_rule/) - Validate a rule's SQL/definition against the source before saving or running it, so a malformed rule is caught up front. Returns whether the rule is valid plus the engine's message. Requires `edgeSiteId`/`connectionId`/`schemaName` (from `prepare_create_data_quality_job`)
- [`list_data_quality_rule_templates`](pkg/tools/list_dq_rule_templates/) - List the DQ rule templates (built-in + custom) available in the connected environment via the public DQ API. Each is a parameterized SQL pattern deployable via `deploy_data_quality_rule_template` (by its `ruleTemplateName`). Filters: `name`, `dimension`, `isSystem` (built-in vs custom); paginated
- [`get_data_quality_rule_template`](pkg/tools/get_dq_rule_template/) - Read a single DQ rule template by `ruleTemplateName` — its parameterized SQL, dialect, dimensions, default tolerance, built-in (`isSystem`) flag and deployed-rule count
- [`find_data_quality_rules`](pkg/tools/find_dq_rules/) - Search existing DQ rules (monitors) across jobs. Filter by exact `jobName` and/or `columnName` (combine both to detect rules already on a target column) or a rule-name substring. Returns each rule's job, column, type, status and SQL; paginated
- [`generate_data_quality_rule_sql`](pkg/tools/generate_dq_rule_sql/) - Turn a plain-language description of a check into rule SQL (Text2SQL / Collibra DQ AI), so a rule can be authored without writing SQL. Returns a single SQL string (validate before use). Requires `edgeSiteId`/`connectionId` (from `prepare_create_data_quality_job`)
- [`get_lineage_downstream`](pkg/tools/get_lineage_downstream/) - Get downstream technical lineage (consumers) for a data entity
- [`get_lineage_entity`](pkg/tools/get_lineage_entity/) - Get metadata about a specific entity in the technical lineage graph
- [`get_lineage_transformation`](pkg/tools/get_lineage_transformation/) - Get details and logic of a specific data transformation
- [`get_lineage_upstream`](pkg/tools/get_lineage_upstream/) - Get upstream technical lineage (sources) for a data entity
- [`get_measure_data`](pkg/tools/get_measure_data/) - Trace a measure back to its underlying physical columns and tables
- [`get_table_semantics`](pkg/tools/get_table_semantics/) - Retrieve the semantic layer for a table: columns, data attributes, and connected measures
- [`list_asset_types`](pkg/tools/list_asset_types/) - List available asset types
- [`list_data_contract`](pkg/tools/list_data_contracts/) - List data contracts with pagination
- [`prepare_create_asset`](pkg/tools/prepare_create_asset/) - Read-only companion to `create_asset`: enumerate available asset types and domains, resolve a UUID/publicId/displayName for either, and hydrate the scoped attribute and relation schema for a chosen pair
- [`pull_data_contract_manifest`](pkg/tools/pull_data_contract_manifest/) - Download manifest for a data contract
- [`search_asset_keyword`](pkg/tools/search_asset_keyword/) - Wildcard keyword search for assets; filters (status, community, domain, domain type, asset type, created-by) accept names or UUIDs
- [`search_catalog_columns`](pkg/tools/search_catalog_columns/) - Find catalog Column assets by metadata that keyword search can't filter on — Description/Data Type (attribute values), a Data Steward role, or relations to a Business Term/Business Rule/Data Element/Data Attribute (by name); AND-combined. Uses the DGC Knowledge Graph GraphQL API (must be enabled on the instance). Classification-tag filtering is not supported
- [`search_data_class`](pkg/tools/search_data_classes/) - Search for data classes with filters. **Requires:** `dgc.data-classes-read`
- [`search_data_classification_match`](pkg/tools/search_data_classification_matches/) - Search for associations between data classes and assets. **Requires:** `dgc.classify`, `dgc.catalog`
- [`search_lineage_entities`](pkg/tools/search_lineage_entities/) - Search for entities in the technical lineage graph
- [`search_lineage_transformations`](pkg/tools/search_lineage_transformations/) - Search for transformations in the technical lineage graph

### Write Tools

- [`add_data_classification_match`](pkg/tools/add_data_classification_match/) - Associate a data class with an asset. **Requires:** `dgc.classify`, `dgc.catalog`
- [`create_assessment`](pkg/tools/create_assessment/) - Conduct a new assessment from a template (given by name or UUID) in the Assessments application. Returns the template's (unanswered) questions to fill in afterward with `edit_assessment` — no separate prepare step needed
- [`create_asset`](pkg/tools/create_asset/) - Create a new asset of any type. Resolves `assetType` (UUID, publicId, or display name), `domain` (UUID or name), `status` (UUID or name), and attributes (by name or typeId) server-side; converts Markdown to HTML for `RICH_TEXT` attributes; gates on duplicate-name (default `allowDuplicate: false`)
- [`create_data_quality_rule`](pkg/tools/create_dq_rule/) - Create a data quality rule (monitor) on an existing DQ job. `monitorType` is `FREEFORM_SQL` (full SQL query) or `SIMPLE_SQL` (single-column check); defaults to active and not suppressed. Confirm checkpoint: `confirm=false` (default) returns a preview of the rule + SQL without creating; `confirm=true` creates. Uses the DQ monitoring API and requires permission to create rules on the target job. **Experimental** (`data-quality` feature flag)
- [`deploy_data_quality_rule_template`](pkg/tools/deploy_dq_rule_template/) - Instantiate a rule template as concrete rules across one or more job/column targets (bulk). The DQ service resolves dialect-specific SQL and names each rule `{templateName}_{columnName}`. Confirm checkpoint: `confirm=false` (default) previews the template + targets without deploying; `confirm=true` deploys. Requires permission to deploy templates and create rules on the target jobs. **Experimental** (`data-quality` feature flag)
- [`create_data_quality_rule_template`](pkg/tools/create_dq_rule_template/) - Create a reusable rule template — a parameterized SQL pattern with a `{{column}}` placeholder — in the DQ template library. Requires `name`, `sql`, `dialect`, `dimensions` and `description` (the DQ API requires all five); `businessRuleLinks` accepts Business Rule asset names or UUIDs and resolves names before the write. Confirm checkpoint: `confirm=false` (default) previews the exact template without creating; `confirm=true` creates. Returns the created template with its assigned id. A duplicate name is reported rather than overwriting
- [`update_data_quality_rule_template`](pkg/tools/update_dq_rule_template/) - Partially update a rule template by `name` — supply only the fields to change and the rest keep their stored values (the tool reads the template and merges, since the DQ API's update is a full-replacement PUT). The change ALWAYS cascades to every rule deployed from the template; the API offers no way to update the definition alone, so there is no cascade switch. Confirm checkpoint: `confirm=false` (default) previews the merged template and the affected-rule count; `confirm=true` applies it. Returns per-deployment outcomes, reporting `partial` when some deployed rules were skipped
- [`delete_data_quality_rule_template`](pkg/tools/delete_dq_rule_template/) - Delete a rule template by `name`. `cascade=false` (default) refuses the delete while rules deployed from the template are live and reports how many; `cascade=true` deletes the template and those rules together. Confirm checkpoint: `confirm=false` (default) reports exactly what would be deleted; `confirm=true` deletes. Out-of-the-box (system) templates are read-only and rejected. A missing template is reported as an error even though the DQ API's delete is idempotent
- [`dq_cancel_job_run`](pkg/tools/cancel_dq_job_run/) - Cancel an IN-PROGRESS Collibra data-quality job run. Supply EITHER `jobRunId` OR `jobName` (not both). By `jobRunId`: looks up the run's state and refuses with a clear message if it is already in a terminal state (finished/failed/cancelled). By `jobName`: finds the job's cancellable (non-terminal) runs — if exactly one, cancels it; if several, returns them as candidates (`needs_input`) so you can pick one and re-call with its `jobRunId`. No confirm checkpoint — the terminal-state pre-check (by ID) and non-terminal search filter (by name) are the safety mechanism. Cancellation is irreversible and immediately queued on success. **Experimental** (`data-quality` feature flag)
- [`dq_delete_job`](pkg/tools/delete_dq_job/) - PERMANENTLY DELETE a Collibra data-quality job definition by `jobName`, along with ALL of its runs, rules, monitors and results. THIS CANNOT BE UNDONE. Safety checkpoint: `confirm=false` (default) is READ-ONLY — it looks the job up and returns a summary (job type, edge site, connection, schema/table, source query, schedule) so you can review it with the user; call again with the same `jobName` and `confirm=true` to actually delete. If a run is in progress the service may refuse the delete — cancel it first with `dq_cancel_job_run`. To delete a single run rather than the whole job, use `dq_delete_job_run`; to change a job's configuration instead of removing it, use `dq_update_job`. **Experimental** (`data-quality` feature flag)
- [`dq_delete_job_run`](pkg/tools/delete_dq_job_run/) - PERMANENTLY DELETE a COMPLETED Collibra data-quality job run and ALL of its per-run results (profile, scan, monitor, rule, and alert output). THIS CANNOT BE UNDONE. Supply EITHER `jobRunId` OR `jobName`. Safety checkpoint: `confirm=false` (default) is READ-ONLY — returns the run's details without deleting so you can review them with the user; call again with the same `jobRunId` and `confirm=true` to actually delete. A `jobName` NEVER deletes directly: it only resolves candidate runs for review. Only terminal runs can be deleted — use `dq_cancel_job_run` first to stop any in-progress run. **Experimental** (`data-quality` feature flag)
- [`dq_update_job`](pkg/tools/update_dq_job/) - PARTIALLY UPDATE an existing Collibra data-quality job by `jobName` — supply only the fields to change and everything else is left untouched. Covers the scan SQL (`sourceQuery`), the run-date window (`runDate`/`runDateEnd`/`dateFormat`), the recurring schedule (`scheduleRepeat`/`scheduleRunTime`/… — `scheduleRepeat=NEVER` switches an existing schedule OFF), the monitor set (`monitors`) and adaptive baseline (`dataLookback`/`learningPhase`), notifications (`notify` + thresholds + `notifyRecipients`), PUSHDOWN compute (`pushdownConnections`/`pushdownThreads`), PULLUP sizing (`sizing*`/`parallelJdbc*`/`sparkSqlProperties`), and the data location for a moved/renamed table. Safety checkpoint: `confirm=false` (default) is READ-ONLY — it looks the job up and returns a before/after diff (`changes`) plus the exact PATCH body, changing nothing; call again with the same inputs and `confirm=true` to apply. Most settings merge field by field, but three are REPLACED wholesale: `monitors` is authoritative (anything omitted is turned off), the schedule is rebuilt from the schedule inputs, and setting any `notify*` field replaces the entire notification configuration — re-supply what should be kept. Data-location fields are overlaid onto the current location, so changing just `tableName` works. A job's type (PUSHDOWN/PULLUP) is immutable and back-runs are not part of the update API; column selection, row filters, sampling and the time slice live inside `sourceQuery` and are not recomposed for you. Rules are managed by `create_data_quality_rule`/`deploy_data_quality_rule_template`; to remove a job entirely use `dq_delete_job`. **Experimental** (`data-quality` feature flag)
- [`dq_get_job`](pkg/tools/get_dq_job/) - Read the full definition of a single Collibra data-quality job by `name` — type (PUSHDOWN/PULLUP), edge site, connection, schema/table, source SQL, run-date window, configured monitors (adaptive + custom DQ rules), notifications, and schedule. An exact name match is tried first; if none is found, jobs whose name contains the given text are offered as candidates (`needs_input`) to disambiguate. Read-only. **Experimental** (`data-quality` feature flag)
- [`dq_get_job_run`](pkg/tools/get_dq_job_run/) - Read the full details of a single Collibra data-quality job run by `run_id` — lifecycle status/activity/timing, and once the run reaches a terminal state (FINISHED/CANCELLED/FAILED), its overall score, row count, execution time, and the per-monitor breakdown (adaptive + custom DQ rules) behind that score. Fields that are only meaningful once a run has finished are absent while it is still in progress. Read-only. **Experimental** (`data-quality` feature flag)
- [`edit_assessment`](pkg/tools/edit_assessment/) - Edit a conducted assessment (identified by name or UUID) via a list of typed operations, applied as a single atomic PATCH (all-or-nothing):
    - `set_answer` - set a question's answer by `questionId`: TEXT/HTML/EXPRESSION/NUMBER/BOOLEAN/DATE via `value`, or ITEMS (choice) via `items`; supply `answerType` for a not-yet-answered question (an already-answered question's type is inferred). ASSETS/USERORGROUPS/ATTACHMENTS answer types are not yet supported
    - `set_status` - move status (`DRAFT`, `SUBMITTED`, `OBSOLETE`)
    - `set_name` - rename the assessment
    - `set_owner` - set the owner by user UUID
    - `set_assignees` - replace the assignee list with the given users/groups
    - `set_visibility` - set whether the assessment is visible to everyone
- [`edit_asset`](pkg/tools/edit_asset/) - Edit an existing asset via a list of typed operations:
    - `set_attribute`, `add_attribute`, `remove_attribute` - set an attribute value (creates if empty, updates if present), append an extra value to a multi-valued attribute, or clear one (e.g. `Definition`, `Note`)
    - `update_property` - rename the asset (`name`), change its `displayName`, or change its `statusId` (status name or UUID accepted)
    - `add_relation`, `remove_relation` - link or unlink the asset to another asset by relation role (e.g. `is synonym of`)
    - `add_tag` - append a free-text tag without replacing existing tags
    - `set_responsibility` - assign a user or group to a resource role (e.g. `Steward`, `Owner`) by username, email, or UUID
    - `remove_responsibility` - unassign a user or group from a resource role (only directly-assigned responsibilities, not inherited ones)
- [`init_data_contract`](pkg/tools/init_data_contract/) - Initialize a new data contract asset governing a Data Product Port, with an optional initial manifest. **Requires:** `dgc.data-contract`
- [`push_data_contract_manifest`](pkg/tools/push_data_contract_manifest/) - Upload manifest for a data contract. **Requires:** `dgc.data-contract`
- [`remove_data_classification_match`](pkg/tools/remove_data_classification_match/) - Remove a classification match. **Requires:** `dgc.classify`, `dgc.catalog`, `dgc.data-classes-edit`

## Quick Start

### Prerequisites

- Access to a Collibra Data Governance Center instance
- Valid Collibra credentials

### Installation

#### Option A: Download Prebuilt Binary (Recommended)

1. **Download the latest release:**
   - Go to the [GitHub Releases page](../../releases)
   - Download the appropriate binary for your platform:
     - `chip-linux-amd64` - Linux (Intel/AMD 64-bit)
     - `chip-linux-arm64` - Linux (ARM 64-bit)
     - `chip-mac-amd64` - macOS (Intel)
     - `chip-mac-arm64` - macOS (Apple Silicon)
     - `chip-windows-amd64.exe` - Windows (Intel/AMD 64-bit)
     - `chip-windows-arm64.exe` - Windows (ARM 64-bit)

2. **Make the binary executable (Linux/macOS):**
   ```bash
   chmod +x chip-*
   ```

3. **Optional: Move to your PATH:**
   ```bash
   # Linux/macOS
   sudo mv chip-* /usr/local/bin/mcp-server
   
   # Or add to your user bin directory
   mv chip-* ~/.local/bin/mcp-server
   ```

#### Option B: Build from Source
   ```bash
   git clone <repository-url>
   cd chip
   go mod download
   go build -o .build/chip ./cmd/chip

   # Run the build binary
   ./.build/chip
   ```

## Running and Configuration

### Authentication Options

The server supports two authentication approaches, either configured through environment variables or a configuration file

#### Option 1: Server-wide Authentication
When running over the stdio transport, configure credentials at the server level - all requests use the same credentials:

```bash
mkdir -p ~/.config/collibra/
```

Powershell:
```powershell
New-Item -ItemType File -Path $HOME\.config\collibra\mcp.yaml
```

bash/zsh:
```bash
touch ~/.config/collibra/mcp.yaml
```


```yaml
# ~/.config/collibra/mcp.yaml
api:
  url: "https://your-collibra-instance.com"
  username: "your-username"
  password: "your-password"
```

The same options can be configured through the respective environment variables COLLIBRA_MCP_API_URL, COLLIBRA_MCP_API_USR and COLLIBRA_MCP_API_PWD.

#### Option 2: Client-provided Authentication
When running over the http transport, it is recommended that MCP clients provide their own Basic Auth headers for each request:
```bash
export COLLIBRA_MCP_API_URL="https://your-collibra-instance.com"
./mcp-server
```

**For detailed configuration instructions, see [CONFIG.md](docs/CONFIG.md).**

## Security Considerations

- 🔐 **Credentials**: Store sensitive information in environment variables rather than config files
- 🌐 **Network**: HTTP mode binds to localhost only for security
- 🔒 **TLS**: Only use `skip-tls-verify: true` for development with self-signed certificates
- 📁 **File Permissions**: Ensure config files have appropriate permissions when containing credentials

## Integration with MCP Clients

This server is compatible with any MCP client. Refer to your MCP client's documentation for server configuration. 

Here's how to integrate with some popular clients assuming you have a configuration file setup:

* Claude desktop
```json
// ~/Library/Application Support/Claude/claude_desktop_config.json
{
  "mcpServers": {
    "collibra": {
      "type": "stdio",
      "command": "/path/to/chip-..."
    }
  }
}
```

* VS Code
```json
// .vscode/mcp.json
{
    "servers": {
        "collibra": {
            "type": "stdio",
            "command": "./chip"
        }
    }
}
```

* Gemini-cli
```json
// ~/.gemini/settings.json
{
  "mcpServers": {
    "collibra": {
      "command": "/path/to/chip-..."
    }
  }
}
```

## Structured Tool Output

Every tool declares an `outputSchema` and returns results in two forms on the same response:

- `content` — a human-readable `TextContent` block containing the output serialized as JSON. Kept for backward compatibility with clients that only render text.
- `structuredContent` — the typed, parseable object. New clients should prefer this for programmatic consumption.

Schemas are auto-generated from each tool's Go `Output` struct via [`github.com/google/jsonschema-go`](https://pkg.go.dev/github.com/google/jsonschema-go), which emits **JSON Schema draft 2020-12**. The MCP SDK validates every response against the declared schema before sending, so clients can rely on the shape. Field-level descriptions live as `jsonschema:"..."` tags on the `Output` struct in each tool's `pkg/tools/<name>/tool.go`.

To discover the live schema for any tool, inspect the `outputSchema` field returned by a `tools/list` MCP request against a running server.

## Enabling or disabling specific tools

You can enable or disable specific tools by passing command line parameters, setting environment variables, or customizing the `mcp.yaml` configuration file.
You can specify tools to enable or disable by using the tool names listed above (e.g. `get_asset_details`).  For more information, see the [CONFIG.md](docs/CONFIG.md) documentation.

By default, all tools are enabled. Specifying tools to be enabled will enable *only* those tools.  Disabling tools will disable *only* those tools and leave all others enabled.
At present, enabling and disabling at the same time is not supported. 

## Experimental features

Some functionality ships behind an opt-in `experimental` flag. These features are off by default and may change or be removed without a deprecation cycle. Enable them via `--experimental=<name>`, the `COLLIBRA_MCP_EXPERIMENTAL` environment variable, or the `mcp.experimental` field in `mcp.yaml`. Unknown names log a warning but do not fail startup, so stale configs survive a feature being retired or renamed.

### Known experimental features

- `context-specifications` — Context specification tools: `list_context_specifications`, `get_context_specification`, and the `contextSpecificationId` parameter on `get_asset_details`. These tools generate structured YAML context for assets using the Semantic Blueprint API.

- `skills` — Embedded skill catalog served via two additional tools, `list_collibra_skills` and `load_collibra_skill`. Skills are short Markdown guides that document multi-step Collibra workflows (discovery, lineage, asset create/edit, …) for the connecting LLM. See [SKILLS.md](SKILLS.md) for the catalog.

  Point chip at an **external skills directory** with `--skills-dir=<path>` (or `COLLIBRA_MCP_SKILLS_DIR`, or `mcp.skills-dir` in YAML) to add your own skills on top of the embedded ones. The expected layout is `<dir>/<namespace>/<name>/SKILL.md` (with optional `references/*.md` and `_shared/*.md` siblings) — same as the bundled catalog. External skills whose name matches an embedded skill (e.g. `collibra/lineage`) **fully replace** the embedded entry, including its resources, so you can override the shipped guides without rebuilding chip. `~` and `~user` in the path are expanded.

