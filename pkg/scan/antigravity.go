package scan

// Antigravity is Google's agentic IDE (https://antigravity.google).
// Presence-only: skills under ~/.gemini/config/skills and project
// .agents/skills attribute to it via skillDirRules, but it has no MCP
// or plugin config scanned yet. Antigravity 2.0 renamed its
// dot-directory and Windows install dir from "Antigravity" to
// "Antigravity IDE"; both generations are checked.
type antigravityScanner struct{}

func (antigravityScanner) Name() string { return "antigravity" }

func (antigravityScanner) Presence(string) presenceDef {
	return presenceDef{
		binaries:   []string{"antigravity"},
		appBundles: []string{"Antigravity.app", "Antigravity IDE.app"},
		installDirs: []string{
			"AppData/Local/Programs/Antigravity IDE",
			"AppData/Local/Programs/Antigravity",
		},
		configPaths: []string{".antigravity-ide", ".antigravity"},
	}
}

func (antigravityScanner) GlobalConfigs(string) []string { return nil }

func (antigravityScanner) ProjectConfigs() []string { return nil }

func (antigravityScanner) ScanHome(*state) observations { return observations{} }

func (antigravityScanner) ScanProject(*state, string) observations { return observations{} }
