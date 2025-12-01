# Collibra MCP Server

A Model Context Protocol (MCP) server that provides AI agents with access to Collibra Data Governance Center capabilities including data asset discovery, business glossary queries, and detailed asset information retrieval.

## Overview

This Go-based MCP server acts as a bridge between AI applications and Collibra, enabling intelligent data discovery and governance operations through the following tools:

- [`asset_details_get`](pkg/tools/get_asset_details.go) - Retrieve detailed information about specific assets by UUID
- [`asset_keyword_search`](pkg/tools/keyword_search.go) - Wildcard keyword search for assets
- [`asset_types_list`](pkg/tools/list_asset_types.go) - List available asset types
- [`business_glossary_discover`](pkg/tools/ask_glossary.go) - Ask questions about terms and definitions
- [`data_classification_match_add`](pkg/tools/add_data_classification_match.go) - Associate a data class with an asset
- [`data_classification_match_remove`](pkg/tools/remove_data_classification_match.go) - Remove a classification match
- [`data_classification_match_search`](pkg/tools/find_data_classification_matches.go) - Find associations between data classes and assets
- [`data_assets_discover`](pkg/tools/ask_dad.go) - Query available data assets using natural language
- [`data_class_search`](pkg/tools/search_data_classes.go) - Search for data classes with filters
- [`data_contract_list`](pkg/tools/list_data_contracts.go) - List data contracts with pagination
- [`data_contract_manifest_pull`](pkg/tools/pull_data_contract_manifest.go) - Download manifest for a data contract
- [`data_contract_manifest_push`](pkg/tools/push_data_contract_manifest.go) - Upload manifest for a data contract

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

## Enabling or disabling specific tools

You can enable or disable specific tools by passing command line parameters, setting environment variables, or customizing the `mcp.yaml` configuration file.
You can specify tools to enable or disable by using the tool names listed above (e.g. `asset_details_get`).  For more information, see the [CONFIG.md](docs/CONFIG.md) documentation.

By default, all tools are enabled. Specifying tools to be enabled will enable *only* those tools.  Disabling tools will disable *only* those tools and leave all others enabled.
At present, enabling and disabling at the same time is not supported. 
