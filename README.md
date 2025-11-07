# Collibra MCP Server

A Model Context Protocol (MCP) server that provides AI agents with access to Collibra Data Governance Center capabilities including data asset discovery, business glossary queries, and detailed asset information retrieval.

## Overview

This Go-based MCP server acts as a bridge between AI applications and Collibra, enabling intelligent data discovery and governance operations through three main tools:

- **Data Asset Discovery**: Query available data assets using natural language
- **Business Glossary**: Ask questions about terms and definitions in your business glossary  
- **Asset Details**: Retrieve comprehensive information about specific assets by UUID

## Quick Start

### Prerequisites

- Access to a Collibra Data Governance Center instance
- Valid Collibra credentials (can be provided via server config or client requests)

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
   cd ai-mcp-discovery
   go mod download
   go build cmd/chip
   ```

## Running and Configuration

### Authentication Options

The server supports two authentication approaches:

#### Option 1: Server-wide Authentication
When running over the stdio transport, configure credentials at the server level - all requests use the same credentials:
```bash
export COLLIBRA_MCP_API_URL="https://your-collibra-instance.com"
export COLLIBRA_MCP_API_USR="your-username"
export COLLIBRA_MCP_API_PWD="your-password"
./mcp-server
```

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

Here's how to integrate with Claude desktop or VSCode assuming you have exported your credentials:

### Claude Desktop
Add to your Claude Desktop configuration:
```json
{
  "mcpServers": {
    "collibra": {
      "type": "stdio",
      "command": "/path/to/mcp-server"
    }
  }
}
```

### VS Code
For VS Code with MCP extensions, add the server to `.vscode/mcp.json` in your workspace:

```json
{
    "servers": {
        "collibra": {
            "type": "stdio",
            "command": "./chip"
        }
    }
}
```


## Contributing

TODO

## License

TODO