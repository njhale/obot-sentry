package scan

import (
	"io/fs"
	"strings"

	"github.com/obot-platform/obot/apiclient/types"
)

// Zed's settings live under ~/.config/zed on macOS and Linux and
// %APPDATA%\Zed on Windows; the extensions data dir differs per
// platform (Windows uses %LOCALAPPDATA%):
// https://zed.dev/docs/configuring-zed
// https://zed.dev/docs/extensions/installing-extensions
func zedSettingsRel(platform string) string {
	if platform == "windows" {
		return "AppData/Roaming/Zed/settings.json"
	}
	return ".config/zed/settings.json"
}

func zedExtensionsRel(platform string) string {
	switch platform {
	case "darwin":
		return "Library/Application Support/Zed/extensions/installed"
	case "windows":
		return "AppData/Local/Zed/extensions/installed" // %LOCALAPPDATA%\Zed
	default:
		return ".local/share/zed/extensions/installed"
	}
}

const zedExtensionPrefix = "mcp-server-"

// zedSettings has only the field we care about. Zed's `context_servers`
// map keys use opaque server names; values follow Zed's own schema:
// either {url, env, headers} for SSE or {command, args, env} for stdio,
// with an optional explicit `enabled: false` skip.
type zedSettings struct {
	ContextServers map[string]zedContextServer `json:"context_servers"`
}

type zedContextServer struct {
	URL     string         `json:"url"`
	Command string         `json:"command"`
	Args    []string       `json:"args"`
	Env     map[string]any `json:"env"`
	Headers map[string]any `json:"headers"`
	Enabled *bool          `json:"enabled"`
}

// emitZedContextServers parses Zed's context_servers map. Returns the
// set of server names emitted (so the extensions merge can dedupe) and
// the observation slice.
func emitZedContextServers(servers map[string]zedContextServer, configPath, projectPath string) (map[string]bool, []types.DeviceScanMCPServer) {
	var (
		emitted = map[string]bool{}
		out     = make([]types.DeviceScanMCPServer, 0, len(servers))
	)
	for _, name := range sortedKeys(servers) {
		e := servers[name]
		if e.Enabled != nil && !*e.Enabled {
			continue
		}
		obs, ok := e.toServer(name, configPath, projectPath)
		if !ok {
			continue
		}
		out = append(out, obs)
		emitted[name] = true
	}
	return emitted, out
}

// toServer parses a Zed context-server entry. URL → sse; command →
// stdio; neither (settings-only extension placeholders) → drop.
func (e zedContextServer) toServer(name, configPath, projectPath string) (types.DeviceScanMCPServer, bool) {
	if e.URL != "" {
		return types.DeviceScanMCPServer{
			Client:      "zed",
			ProjectPath: projectPath,
			File:        configPath,
			Name:        name,
			Transport:   "sse",
			URL:         e.URL,
			EnvKeys:     sortedKeys(e.Env),
			HeaderKeys:  sortedKeys(e.Headers),
			ConfigHash:  mcpConfigHash(name, "sse", "", nil, e.URL),
		}, true
	}
	if e.Command != "" {
		return types.DeviceScanMCPServer{
			Client:      "zed",
			ProjectPath: projectPath,
			File:        configPath,
			Name:        name,
			Transport:   "stdio",
			Command:     e.Command,
			Args:        e.Args,
			EnvKeys:     sortedKeys(e.Env),
			HeaderKeys:  []string{},
			ConfigHash:  mcpConfigHash(name, "stdio", e.Command, e.Args, ""),
		}, true
	}
	return types.DeviceScanMCPServer{}, false
}

// mergeZedExtensions scans the extensions tree for folders prefixed
// with mcp-server- and emits a stdio observation for each name not
// already present in `existing`. The extension itself supplies command/
// args at runtime, so we leave those blank.
func mergeZedExtensions(s *state, configPath string, existing map[string]bool) []types.DeviceScanMCPServer {
	entries, err := fs.ReadDir(s.fsys, zedExtensionsRel(s.platform))
	if err != nil {
		return nil
	}
	var out []types.DeviceScanMCPServer
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasPrefix(name, zedExtensionPrefix) || existing[name] {
			continue
		}
		out = append(out, types.DeviceScanMCPServer{
			Client:     "zed",
			File:       configPath,
			Name:       name,
			Transport:  "stdio",
			EnvKeys:    []string{},
			HeaderKeys: []string{},
			ConfigHash: mcpConfigHash(name, "stdio", "", nil, ""),
		})
	}
	return out
}

// zedHomeServers reads Zed's settings.json and merges in the installed
// mcp-server-* extensions, which are servers Zed knows about without a
// context_servers entry.
func zedHomeServers(s *state, rel, _ string) observations {
	cfg, ok := readJSON[zedSettings](s.fsys, rel)
	var configPath string
	if ok {
		configPath = s.addFileOrAbs(rel)
	}
	emitted, servers := emitZedContextServers(cfg.ContextServers, configPath, "")
	return observations{servers: append(servers, mergeZedExtensions(s, configPath, emitted)...)}
}

// zedProjectServers reads a project .zed/settings.json. Extensions are
// installed per user, not per project, so they are not merged here.
func zedProjectServers(s *state, rel, projectPath string) observations {
	cfg, ok := readJSON[zedSettings](s.fsys, rel)
	if !ok {
		return observations{}
	}
	_, servers := emitZedContextServers(cfg.ContextServers, s.addFileOrAbs(rel), projectPath)
	return observations{servers: servers}
}
