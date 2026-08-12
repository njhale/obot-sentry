package scan

import (
	"io/fs"
	"log/slog"
	"path"
)

// Cursor keeps its config under ~/.cursor on every platform:
// https://cursor.com/docs/mcp
const (
	cursorGlobalConfigRel   = ".cursor/mcp.json"
	cursorSettingsRel       = ".cursor/settings.json"
	cursorPluginCacheRel    = ".cursor/plugins/cache/cursor-public"
	cursorPluginManifestSub = ".cursor-plugin/plugin.json"
	cursorMarketplace       = "cursor-public"
)

// scanPlugins walks ~/.cursor/plugins/cache/cursor-public/<name>/<hash>/
// for .cursor-plugin/plugin.json manifests. Dedupes by plugin name
// (first hash dir wins) and resolves enabled state from
// .cursor/settings.json (two key forms: "<name>@cursor-public" and
// "<name>").
func cursorPlugins(s *state, _, _ string) observations {
	plugins, err := fs.ReadDir(s.fsys, cursorPluginCacheRel)
	if err != nil {
		return observations{}
	}
	enabledByKey := readEnabledPluginsMap(s.fsys, cursorSettingsRel)
	seen := map[string]bool{}

	var obs observations
	for _, p := range plugins {
		if !p.IsDir() || seen[p.Name()] {
			continue
		}
		pluginRel := path.Join(cursorPluginCacheRel, p.Name())
		// Claim every hash dir, not just the one emitted, so stale
		// copies don't leak observations through the walk.
		s.claim(pluginRel)
		hashes, err := fs.ReadDir(s.fsys, pluginRel)
		if err != nil {
			slog.Debug("cursor: skipping plugin", "path", pluginRel, "err", err)
			continue
		}
		for _, h := range hashes {
			if !h.IsDir() {
				continue
			}
			installRel := path.Join(pluginRel, h.Name())
			manifestRel := path.Join(installRel, cursorPluginManifestSub)
			if !fileExists(s.fsys, manifestRel) {
				continue
			}

			var enabled bool
			for _, key := range []string{p.Name() + "@" + cursorMarketplace, p.Name()} {
				if v, ok := enabledByKey[key]; ok {
					enabled = v
					break
				}
			}

			obs.add(emitPlugin(s, emitPluginOpts{
				installRel:   installRel,
				manifestRel:  manifestRel,
				pluginType:   "cursor_plugin",
				client:       "cursor",
				marketplace:  cursorMarketplace,
				enabled:      enabled,
				nameFallback: p.Name(),
				nestedMCPRel: []string{"mcp.json", ".mcp.json"},
			}))
			seen[p.Name()] = true
			break
		}
	}
	return obs
}

// cursorServers reads a Cursor mcp.json. Home and project scope share
// one path and one shape.
func cursorServers(s *state, rel, projectPath string) observations {
	return observations{servers: emitJSONServers(s, rel, "mcpServers", "cursor", projectPath)}
}
