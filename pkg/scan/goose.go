package scan

import (
	"strings"

	"github.com/obot-platform/obot/apiclient/types"
)

// gooseGlobalConfigRel returns Goose's config.yaml location: ~/.config
// on macOS and Linux, %APPDATA%\Block\goose\config on Windows:
// https://github.com/block/goose/blob/main/documentation/docs/guides/config-files.md
func gooseGlobalConfigRel(platform string) string {
	if platform == "windows" {
		return "AppData/Roaming/Block/goose/config/config.yaml"
	}
	return ".config/goose/config.yaml"
}

func gooseConfigDir(platform string) string {
	if platform == "windows" {
		return "AppData/Roaming/Block/goose"
	}
	return ".config/goose"
}

// gooseConfig is Goose's config.yaml shape: a top-level `extensions`
// map. Goose uses non-standard field names (cmd/envs/uri instead of
// command/env/url) and gates every entry on a required `enabled: true`.
type gooseConfig struct {
	Extensions map[string]gooseExtension `yaml:"extensions"`
}

type gooseExtension struct {
	Type    string         `yaml:"type"`
	Name    string         `yaml:"name"`
	Cmd     string         `yaml:"cmd"`
	Args    []string       `yaml:"args"`
	URI     string         `yaml:"uri"`
	Envs    map[string]any `yaml:"envs"`
	Headers map[string]any `yaml:"headers"`
	Enabled bool           `yaml:"enabled"`
}

// toServer materializes a Goose extension. Only stdio/sse/streamable_http
// types are surfaced (other types are MCP-irrelevant).
func (e gooseExtension) toServer(key, configPath string) (types.DeviceScanMCPServer, bool) {
	switch e.Type {
	case "stdio", "sse", "streamable_http":
	default:
		return types.DeviceScanMCPServer{}, false
	}
	name := key
	if e.Name != "" {
		name = e.Name
	}

	if e.Type == "stdio" {
		return types.DeviceScanMCPServer{
			Client:     "goose",
			File:       configPath,
			Name:       name,
			Transport:  "stdio",
			Command:    e.Cmd,
			Args:       e.Args,
			EnvKeys:    sortedKeys(e.Envs),
			HeaderKeys: []string{},
			ConfigHash: mcpConfigHash(name, "stdio", e.Cmd, e.Args, ""),
		}, true
	}

	transport := strings.ReplaceAll(e.Type, "_", "-")
	return types.DeviceScanMCPServer{
		Client:     "goose",
		File:       configPath,
		Name:       name,
		Transport:  transport,
		URL:        e.URI,
		EnvKeys:    sortedKeys(e.Envs),
		HeaderKeys: sortedKeys(e.Headers),
		ConfigHash: mcpConfigHash(name, transport, "", nil, e.URI),
	}, true
}

// gooseServers reads Goose's config.yaml. Every extension is gated on an
// explicit `enabled: true`.
func gooseServers(s *state, rel, _ string) observations {
	cfg, ok := readYAML[gooseConfig](s.fsys, rel)
	if !ok {
		return observations{}
	}
	configPath := s.addFileOrAbs(rel)

	servers := make([]types.DeviceScanMCPServer, 0, len(cfg.Extensions))
	for _, key := range sortedKeys(cfg.Extensions) {
		ext := cfg.Extensions[key]
		if !ext.Enabled {
			continue
		}
		if server, ok := ext.toServer(key, configPath); ok {
			servers = append(servers, server)
		}
	}
	return observations{servers: servers}
}
