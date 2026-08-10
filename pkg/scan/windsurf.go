package scan

// Windsurf keeps its MCP config under ~/.codeium on every platform:
// https://docs.windsurf.com/windsurf/cascade/mcp
const windsurfGlobalConfigRel = ".codeium/windsurf/mcp_config.json"

// windsurfServers reads a Windsurf mcp_config.json.
func windsurfServers(s *state, rel, projectPath string) observations {
	return observations{servers: emitJSONServers(s, rel, "mcpServers", "windsurf", projectPath)}
}
