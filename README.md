# Collibra MCP Server

A Model Context Protocol (MCP) server that provides AI agents with access to Collibra Data Governance Center capabilities including data asset discovery, business glossary queries, and detailed asset information retrieval.

## Overview

This Go-based MCP server acts as a bridge between AI applications and Collibra, enabling intelligent data discovery and governance operations through the following tools:

> **Permissions:** tools that require extra DGC scopes beyond standard catalog read access are flagged with **Requires:**. Tools without that marker work with the default scopes granted to any authenticated Collibra user. If a tool returns a permission error, the connecting user is missing the listed scope(s) — most commonly `dgc.ai-copilot` (for the natural-language discovery tools) and `dgc.classify` + `dgc.catalog` (for the classification tools).

### Read Tools

- [`discover_business_glossary`](pkg/tools/discover_business_glossary/) - Ask questions about terms and definitions. **Requires:** `dgc.ai-copilot`
- [`discover_data_assets`](pkg/tools/discover_data_assets/) - Query available data assets using natural language. **Requires:** `dgc.ai-copilot`
- [`get_asset_details`](pkg/tools/get_asset_details/) - Retrieve detailed information about specific assets by UUID
- [`get_business_term_data`](pkg/tools/get_business_term_data/) - Trace a business term back to its connected physical data assets
- [`get_column_semantics`](pkg/tools/get_column_semantics/) - Retrieve data attributes, measures, and business assets connected to a column
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
- [`search_data_class`](pkg/tools/search_data_classes/) - Search for data classes with filters. **Requires:** `dgc.data-classes-read`
- [`search_data_classification_match`](pkg/tools/search_data_classification_matches/) - Search for associations between data classes and assets. **Requires:** `dgc.classify`, `dgc.catalog`
- [`search_lineage_entities`](pkg/tools/search_lineage_entities/) - Search for entities in the technical lineage graph
- [`search_lineage_transformations`](pkg/tools/search_lineage_transformations/) - Search for transformations in the technical lineage graph

### Write Tools

- [`add_data_classification_match`](pkg/tools/add_data_classification_match/) - Associate a data class with an asset. **Requires:** `dgc.classify`, `dgc.catalog`
- [`create_asset`](pkg/tools/create_asset/) - Create a new asset of any type. Resolves `assetType` (UUID, publicId, or display name), `domain` (UUID or name), `status` (UUID or name), and attributes (by name or typeId) server-side; converts Markdown to HTML for `RICH_TEXT` attributes; gates on duplicate-name (default `allowDuplicate: false`)
- [`edit_asset`](pkg/tools/edit_asset/) - Edit an existing asset via a list of typed operations:
    - `update_attribute`, `add_attribute`, `remove_attribute` - change, append, or clear an attribute value (e.g. `Definition`, `Note`)
    - `update_property` - rename the asset (`name`), change its `displayName`, or change its `statusId` (status name or UUID accepted)
    - `add_relation`, `remove_relation` - link or unlink the asset to another asset by relation role (e.g. `is synonym of`)
    - `add_tag` - append a free-text tag without replacing existing tags
    - `set_responsibility` - assign a user or group to a resource role (e.g. `Steward`, `Owner`) by username, email, or UUID
    - `remove_responsibility` - unassign a user or group from a resource role (only directly-assigned responsibilities, not inherited ones)
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

- `skills` — Embedded skill catalog served via two additional tools, `list_collibra_skills` and `load_collibra_skill`. Skills are short Markdown guides that document multi-step Collibra workflows (discovery, lineage, asset create/edit, …) for the connecting LLM. See [SKILLS.md](SKILLS.md) for the catalog.

  Point chip at an **external skills directory** with `--skills-dir=<path>` (or `COLLIBRA_MCP_SKILLS_DIR`, or `mcp.skills-dir` in YAML) to add your own skills on top of the embedded ones. The expected layout is `<dir>/<namespace>/<name>/SKILL.md` (with optional `references/*.md` and `_shared/*.md` siblings) — same as the bundled catalog. External skills whose name matches an embedded skill (e.g. `collibra/lineage`) **fully replace** the embedded entry, including its resources, so you can override the shipped guides without rebuilding chip. `~` and `~user` in the path are expanded.

