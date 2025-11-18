package main

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/collibra/chip/pkg/chip"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

func Init() *Config {
	viper.SetConfigName("mcp")
	viper.SetConfigType("yaml")
	viper.AddConfigPath(".")
	viper.AddConfigPath("$HOME/.config/collibra")
	viper.AddConfigPath("/etc/collibra")
	viper.SetEnvPrefix("COLLIBRA_MCP")
	viper.AutomaticEnv()

	initConfigOptions()

	pflag.Usage = func() {
		printUsage(chip.Version)
	}

	showHelp := pflag.BoolP("help", "h", false, "Show help message")
	showVersion := pflag.BoolP("version", "v", false, "Show version information")
	pflag.Parse()

	if *showHelp {
		pflag.Usage()
		os.Exit(0)
	}

	if *showVersion {
		fmt.Println(chip.Version)
		os.Exit(0)
	}

	config := readConfigFile()
	validateConfigFile(config)
	return &config
}

func initConfigOptions() {
	pflag.String("api-url", "", "Collibra API URL (env: COLLIBRA_MCP_API_URL)")
	viper.BindEnv("api.url", "COLLIBRA_MCP_API_URL")
	viper.BindPFlag("api.url", pflag.Lookup("api-url"))

	pflag.String("api-username", "", "Collibra API username (env: COLLIBRA_MCP_API_USR)")
	viper.BindEnv("api.username", "COLLIBRA_MCP_API_USR")
	viper.BindPFlag("api.username", pflag.Lookup("api-username"))

	pflag.String("api-password", "", "Collibra API password (env: COLLIBRA_MCP_API_PWD)")
	viper.BindEnv("api.password", "COLLIBRA_MCP_API_PWD")
	viper.BindPFlag("api.password", pflag.Lookup("api-password"))

	pflag.Bool("skip-tls-verify", false, "Skip TLS certificate verification (env: COLLIBRA_MCP_API_SKIP_TLS_VERIFY)")
	viper.BindEnv("api.skip-tls-verify", "COLLIBRA_MCP_API_SKIP_TLS_VERIFY")
	viper.BindPFlag("api.skip-tls-verify", pflag.Lookup("skip-tls-verify"))
	viper.SetDefault("api.skip-tls-verify", false)

	pflag.String("api-proxy", "", "HTTP proxy URL for API requests (env: COLLIBRA_MCP_API_PROXY, HTTP_PROXY, HTTPS_PROXY)")
	viper.BindEnv("api.proxy", "COLLIBRA_MCP_API_PROXY")
	viper.BindEnv("api.proxy", "HTTP_PROXY")  // For compatibility with DefaultTransport
	viper.BindEnv("api.proxy", "HTTPS_PROXY") // For compatibility with DefaultTransport
	viper.BindPFlag("api.proxy", pflag.Lookup("api-proxy"))

	pflag.String("mode", "stdio", "MCP server mode: 'stdio', 'http', 'http-sse', or 'http-streamable' (env: COLLIBRA_MCP_MODE)")
	viper.BindEnv("mcp.mode", "COLLIBRA_MCP_MODE")
	viper.BindPFlag("mcp.mode", pflag.Lookup("mode"))
	viper.SetDefault("mcp.mode", "stdio")

	pflag.Int("port", 8080, "HTTP server port (only used in http mode) (env: COLLIBRA_MCP_HTTP_PORT)")
	viper.BindEnv("mcp.http.port", "COLLIBRA_MCP_HTTP_PORT")
	viper.BindPFlag("mcp.http.port", pflag.Lookup("port"))
	viper.SetDefault("mcp.http.port", 8080)

	pflag.StringSlice("enabled-tools", []string{}, "Optional comma-separated list of tool names to enable instead of enabling all tools (cannot be used with disabled-tools) (env: COLLIBRA_MCP_ENABLED_TOOLS)")
	viper.BindEnv("mcp.enabled-tools", "COLLIBRA_MCP_ENABLED_TOOLS")
	viper.BindPFlag("mcp.enabled-tools", pflag.Lookup("enabled-tools"))

	pflag.StringSlice("disabled-tools", []string{}, "Optional comma-separated list of tool names to disable while enabling the remaining tools (cannot be used with enabled-tools) (env: COLLIBRA_MCP_DISABLED_TOOLS)")
	viper.BindEnv("mcp.disabled-tools", "COLLIBRA_MCP_DISABLED_TOOLS")
	viper.BindPFlag("mcp.disabled-tools", pflag.Lookup("disabled-tools"))
}

func printUsage(version string) {
	fmt.Fprintf(os.Stderr, `Collibra MCP Server %s

A Model Context Protocol (MCP) server that provides tools for interacting with Collibra.

USAGE:
  %s [flags]

FLAGS:
`, version, os.Args[0])
	pflag.PrintDefaults()
	fmt.Fprintf(os.Stderr, `
ENVIRONMENT VARIABLES:
  COLLIBRA_MCP_API_URL          Collibra API URL
  COLLIBRA_MCP_API_USR          Collibra API username
  COLLIBRA_MCP_API_PWD          Collibra API password
  COLLIBRA_MCP_API_SKIP_TLS_VERIFY  Skip TLS certificate verification (default: false)
  COLLIBRA_MCP_API_PROXY        HTTP proxy URL for API requests
  HTTP_PROXY                    HTTP proxy URL (alternative to COLLIBRA_MCP_API_PROXY)
  HTTPS_PROXY                   HTTPS proxy URL (alternative to COLLIBRA_MCP_API_PROXY)
  COLLIBRA_MCP_MODE             Server mode: 'stdio', 'http', 'http-sse', or 'http-streamable' (default: stdio)
  COLLIBRA_MCP_HTTP_PORT        HTTP server port (default: 8080)
  COLLIBRA_MCP_ENABLED_TOOLS    Optional comma-separated list of tool names to enable instead of enabling all tools, cannot be used with disabled-tools
  COLLIBRA_MCP_DISABLED_TOOLS   Optional comma-separated list of tool names to disable while enabling the remaining tools, cannot be used with enabled-tools

CONFIGURATION:
  Configuration can be provided in the following order of precedence: command-line flags (highest), environment variables, or a YAML configuration file (lowest).
  File locations searched in order:
  - ./mcp.yaml
  - $HOME/.config/collibra/mcp.yaml
  - /etc/collibra/mcp.yaml

CONFIGURATION FILE EXAMPLE:
  api:
    url: "https://your-collibra-instance.com"
    username: "your-username"
    password: "your-password"
    skip-tls-verify: false
    proxy: "http://proxy.example.com:8080"
  mcp:
    mode: "http"  # or "stdio", "http-sse", "http-streamable"
    http:
      port: 8080
    enabled-tools:  # Optional: list of tools to enable (cannot be used with disabled-tools)
      - "tool1"
      - "tool2"
    # disabled-tools:  # Optional: list of tools to disable (cannot be used with enabled-tools)
    #   - "tool3"
    #   - "tool4"
`)
}

func validateConfigFile(config Config) {
	if config.Mcp.Mode != "stdio" && config.Mcp.Mode != "http" && config.Mcp.Mode != "http-sse" && config.Mcp.Mode != "http-streamable" {
		slog.Error(fmt.Sprintf("Invalid server mode: %s (must be 'stdio', 'http', 'http-sse' or 'http-streamable')", config.Mcp.Mode))
		os.Exit(1)
	}

	if len(config.Mcp.EnabledTools) > 0 && len(config.Mcp.DisabledTools) > 0 {
		slog.Error("Cannot specify both enabled-tools and disabled-tools, only one can be specified")
		os.Exit(1)
	}
}

func readConfigFile() Config {
	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			slog.Info("No config file found, using environment variables, command-line flags, and defaults")
		} else {
			slog.Error(fmt.Sprintf("Error reading config file: %v", err))
			os.Exit(1)
		}
	} else {
		slog.Info(fmt.Sprintf("Using config file: %s", viper.ConfigFileUsed()))
	}

	var config Config
	if err := viper.Unmarshal(&config); err != nil {
		slog.Error(fmt.Sprintf("Unable to decode config: %v", err))
		os.Exit(1)
	}
	return config
}

type Config struct {
	Api CollibraApiConfig `mapstructure:"api"`
	Mcp McpConfig         `mapstructure:"mcp"`
}

// CollibraConfig holds Collibra-specific configuration
type CollibraApiConfig struct {
	Url           string `mapstructure:"url"`
	Username      string `mapstructure:"username"`
	Password      string `mapstructure:"password"`
	SkipTLSVerify bool   `mapstructure:"skip-tls-verify"`
	Proxy         string `mapstructure:"proxy"`
}

// ServerConfig holds server configuration
type McpConfig struct {
	Mode          string      `mapstructure:"mode"` // "stdio", "http", "http-sse", or "http-streamable"
	Http          HttpConfig  `mapstructure:"http"`
	Stdio         StdioConfig `mapstructure:"stdio"`
	EnabledTools  []string    `mapstructure:"enabled-tools"`
	DisabledTools []string    `mapstructure:"disabled-tools"`
}

type HttpConfig struct {
	Port int `mapstructure:"port"`
}

type StdioConfig struct {
}
