package scan

// vscodeUserDir returns the home-relative VS Code user configuration
// directory holding the user-level mcp.json (default profile):
// https://code.visualstudio.com/docs/configure/settings
func vscodeUserDir(platform string) string {
	switch platform {
	case "darwin":
		return "Library/Application Support/Code/User"
	case "windows":
		return "AppData/Roaming/Code/User" // %APPDATA%\Code\User
	default:
		return ".config/Code/User"
	}
}

// VS Code uses "servers" rather than "mcpServers" for both global and
// project configs; entries follow the standard JSON shape:
// https://code.visualstudio.com/docs/copilot/customization/mcp-servers

// vscodeServers reads a VS Code mcp.json. VS Code uses "servers" rather
// than "mcpServers" for both scopes; entries follow the standard shape:
// https://code.visualstudio.com/docs/copilot/customization/mcp-servers
func vscodeServers(s *state, rel, projectPath string) observations {
	return observations{servers: emitJSONServers(s, rel, "servers", "vscode", projectPath)}
}
