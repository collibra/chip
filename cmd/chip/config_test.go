package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

// TestDataQualityConfigPrecedence exercises the data quality flag through all
// three configuration sources and asserts the documented precedence:
// command-line flag over environment variable over YAML file, with the
// default being off.
//
// The phases run in one test function on purpose: initConfigOptions registers
// its flags on the process-global pflag.CommandLine, so it can only be called
// once per test binary.
func TestDataQualityConfigPrecedence(t *testing.T) {
	dir := t.TempDir()

	viper.Reset()
	t.Cleanup(viper.Reset)
	viper.SetConfigName("mcp")
	viper.SetConfigType("yaml")
	viper.AddConfigPath(dir)
	viper.SetEnvPrefix("COLLIBRA_MCP")
	viper.AutomaticEnv()
	initConfigOptions()

	// No file, no env var, no flag.
	readConfig(t, dir, false)
	if got := dataQualityFromConfig(t); got {
		t.Error("mcp.data-quality should default to false")
	}

	// YAML file only.
	writeConfigFile(t, dir, "mcp:\n  data-quality: true\n")
	readConfig(t, dir, true)
	if got := dataQualityFromConfig(t); !got {
		t.Error("mcp.data-quality: true in mcp.yaml should enable the capability")
	}

	// Environment variable beats the file.
	t.Setenv("COLLIBRA_MCP_DATA_QUALITY", "false")
	if got := dataQualityFromConfig(t); got {
		t.Error("COLLIBRA_MCP_DATA_QUALITY=false should override mcp.yaml")
	}

	// Command-line flag beats the environment variable.
	if err := pflag.CommandLine.Set("data-quality", "true"); err != nil {
		t.Fatalf("set --data-quality: %v", err)
	}
	if got := dataQualityFromConfig(t); !got {
		t.Error("--data-quality=true should override COLLIBRA_MCP_DATA_QUALITY=false")
	}
}

func dataQualityFromConfig(t *testing.T) bool {
	t.Helper()
	var config Config
	if err := viper.Unmarshal(&config); err != nil {
		t.Fatalf("unmarshal config: %v", err)
	}
	return config.Mcp.DataQuality
}

func readConfig(t *testing.T, dir string, wantFile bool) {
	t.Helper()
	err := viper.ReadInConfig()
	if wantFile {
		if err != nil {
			t.Fatalf("read %s/mcp.yaml: %v", dir, err)
		}
		return
	}
	if _, notFound := err.(viper.ConfigFileNotFoundError); err != nil && !notFound {
		t.Fatalf("unexpected config read error: %v", err)
	}
}

func writeConfigFile(t *testing.T, dir, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "mcp.yaml"), []byte(content), 0o644); err != nil {
		t.Fatalf("write mcp.yaml: %v", err)
	}
}
