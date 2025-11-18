# Configuration Guide

The Collibra MCP Server supports configuration through multiple methods, with the following precedence order (highest to lowest):

1. **Environment Variables**
2. **Configuration File**
3. **Default Values**

## Environment Variables

The server can be configured using the following environment variables:

### Required Variables
- `COLLIBRA_MCP_API_URL` - Collibra API base URL (e.g., `https://your-instance.collibra.com`)

### Authentication Variables (Optional)
- `COLLIBRA_MCP_API_USR` - Collibra username (optional if using client-provided auth)
- `COLLIBRA_MCP_API_PWD` - Collibra password (optional if using client-provided auth)

### Optional Variables
- `COLLIBRA_MCP_MODE` - Server mode: `stdio` (default), `http`, `http-sse`, or `http-streamable`
- `COLLIBRA_MCP_HTTP_PORT` - HTTP server port (default: 8080, only used in HTTP modes)
- `COLLIBRA_MCP_API_SKIP_TLS_VERIFY` - Skip TLS certificate verification (default: false)
- `COLLIBRA_MCP_API_PROXY` | `HTTP_PROXY` | `HTTPS_PROXY`  - HTTP proxy URL for API requests (e.g., `http://proxy.example.com:8080`)
- `COLLIBRA_MCP_ENABLED_TOOLS` - Comma-separated list of tool names to enable instead of enabling all tools (cannot be used with `COLLIBRA_MCP_DISABLED_TOOLS`)
- `COLLIBRA_MCP_DISABLED_TOOLS` - Comma-separated list of tool names to disable while enabling the remaining tools (cannot be used with `COLLIBRA_MCP_ENABLED_TOOLS`)

## Configuration File

You can also use a YAML configuration file. The server looks for `mcp.yaml` in the following locations:

1. Current directory (`.`)
2. `$HOME/.config/collibra/`
3. `/etc/collibra/`

### Example mcp.yaml

```yaml
api:
  url: "https://your-collibra-instance.com"
  username: "your-username"      # optional - can be provided by client
  password: "your-password"      # optional - can be provided by client
  http-skip-tls-verify: false
  proxy: "http://proxy.example.com:8080"  # optional

mcp:
  mode: "stdio"  # or "http", "http-sse", "http-streamable"
  http:
    port: 8080

  # optionally enable OR disable specific tools using the tool names listed in the README.md file. 
  # enabled-tools: []  
  # disabled-tools: []
```

## Configuration Structure

The configuration is organized into two main sections:

### API Configuration (`api`)
- `url` - Collibra API base URL (required)
- `username` - Authentication username (optional - can be provided by client requests)
- `password` - Authentication password (optional - can be provided by client requests)
- `http-skip-tls-verify` - Whether to skip TLS certificate verification (boolean)
- `proxy` - HTTP proxy URL for API requests (optional)

### MCP Configuration (`mcp`)
- `mode` - Transport mode (`stdio`, `http`, `http-sse`, or `http-streamable`)
- `http` section:
  - `port` - HTTP server port number
- `stdio` section: (currently empty, reserved for future stdio-specific settings)
- `enabled-tools` - optional list of tool names to be enabled instead of enabling all tools.  Cannot be used with `disabled-tools`
- `disabled-tools` - optional list of tool names to be disabled while enabling remaining tools.  Cannot be used with `enabled-tools`

## Authentication Approaches

The server supports two authentication methods:

### Server-wide Authentication
Configure credentials at the server level. All API requests will use these credentials:
- Use this when running over the stdio transport
- Set `COLLIBRA_MCP_API_USR` and `COLLIBRA_MCP_API_PWD` environment variables
- Or configure `username` and `password` in the config file
- **Warning**: This approach attributes all actions to a single service account

### Client-provided Authentication (Recommended)
Let MCP clients provide Basic Auth headers with each request:
- Use this when running over the http transport
- Omit `username` and `password` from server configuration
- MCP clients include `Authorization: Basic <credentials>` headers in requests
- **Benefit**: Proper attribution of actions to individual users
- **Note**: Only works with HTTP transport modes, not stdio mode.

## Usage Examples

### Using Environment Variables

```bash
# Stdio mode (default)
export COLLIBRA_MCP_API_URL="https://your-instance.collibra.com"
export COLLIBRA_MCP_API_USR="your-username"
export COLLIBRA_MCP_API_PWD="your-password"
./mcp-server

# HTTP mode with custom port (server-wide auth)
export COLLIBRA_MCP_API_URL="https://your-instance.collibra.com"
export COLLIBRA_MCP_API_USR="your-username"
export COLLIBRA_MCP_API_PWD="your-password"
export COLLIBRA_MCP_MODE="http"
export COLLIBRA_MCP_HTTP_PORT="9000"
./mcp-server

# HTTP mode with client-provided auth (recommended)
export COLLIBRA_MCP_API_URL="https://your-instance.collibra.com"
export COLLIBRA_MCP_MODE="http"
export COLLIBRA_MCP_HTTP_PORT="9000"
./mcp-server

# Skip TLS verification (for development/testing)
export COLLIBRA_MCP_API_URL="https://your-instance.collibra.com"
export COLLIBRA_MCP_API_USR="your-username"
export COLLIBRA_MCP_API_PWD="your-password"
export COLLIBRA_MCP_API_SKIP_TLS_VERIFY="true"
./mcp-server

# Using a proxy
export COLLIBRA_MCP_API_URL="https://your-instance.collibra.com"
export COLLIBRA_MCP_API_USR="your-username"
export COLLIBRA_MCP_API_PWD="your-password"
export COLLIBRA_MCP_API_PROXY="http://proxy.example.com:8080" # Or use HTTP_PROXY or HTTPS_PROXY
./mcp-server

# Enabling specific tools only
export COLLIBRA_MCP_API_URL="https://your-instance.collibra.com"
export COLLIBRA_MCP_API_USR="your-username"
export COLLIBRA_MCP_API_PWD="your-password"
export COLLIBRA_MCP_ENABLED_TOOLS="asset_keyword_search,asset_details_get,asset_types_list"
./mcp-server
```

### Using Configuration File

1. Copy `config/mcp.yaml.example` to `mcp.yaml`
2. Update the values in `mcp.yaml`
3. Run: `./mcp-server`

### Mixed Configuration

You can mix configuration file and environment variables. Environment variables will override config file values:

```bash
# Use mcp.yaml for most settings, override port via env var
export COLLIBRA_MCP_HTTP_PORT="9000"
./mcp-server
```

## Server Modes

### Stdio Mode (`mode: "stdio"`)
- **Default mode**
- Uses standard input/output for communication
- Suitable for MCP client integration
- HTTP port setting is ignored

### HTTP Mode (`mode: "http"`)
- Provides HTTP transport using Server-Sent Events (SSE)
- Listens on `localhost` only for security
- Configurable port (default: 8080)
- Suitable for web-based integrations
- Default HTTP implementation when no specific sub-mode is specified

#### HTTP Sub-modes
The HTTP mode supports different transport implementations:

- `"http"` - Default HTTP mode using SSE (Server-Sent Events)
- `"http-sse"` - Explicitly uses SSE transport (same as default "http")
- `"http-streamable"` - Uses streamable HTTP transport for different client requirements

**Note:** HTTP sub-modes are internal implementation details. For most use cases, simply use `mode: "http"`.

## Security Notes

- The server binds to `localhost` only in HTTP mode for security
- Store sensitive configuration (passwords) in environment variables rather than config files when possible
- Ensure config files have appropriate permissions if they contain credentials
- Use `http-skip-tls-verify: true` only for development/testing environments with self-signed certificates

## File Locations

The configuration system follows the XDG Base Directory specification:

- **Current directory**: `./mcp.yaml`
- **User config**: `$HOME/.config/collibra/mcp.yaml`
- **System config**: `/etc/collibra/mcp.yaml`

## Environment Variable Prefix

All environment variables use the `COLLIBRA_MCP_` prefix. The configuration system automatically maps:

- `COLLIBRA_MCP_API_URL` → `api.url`
- `COLLIBRA_MCP_API_USR` → `api.username`
- `COLLIBRA_MCP_API_PWD` → `api.password`
- `COLLIBRA_MCP_API_SKIP_TLS_VERIFY` → `api.http-skip-tls-verify`
- `COLLIBRA_MCP_API_PROXY` → `api.proxy`
- `HTTP_PROXY` → `api.proxy`
- `HTTPS_PROXY` → `api.proxy` 
- `COLLIBRA_MCP_MODE` → `mcp.mode`
- `COLLIBRA_MCP_HTTP_PORT` → `mcp.http.port`
- `COLLIBRA_MCP_ENABLED_TOOLS` → `mcp.enabled-tools`
- `COLLIBRA_MCP_DISABLED_TOOLS` → `mcp.disabled-tools`
