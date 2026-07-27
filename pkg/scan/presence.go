package scan

import (
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/obot-platform/obot/apiclient/types"
)

// presenceDef describes how to detect that a given AI client is
// installed. Each field is a list because most clients have one or two
// canonical names; the first match wins per category.
//
// binaries, appBundles, and installDirs are host-level checks (the
// scanning process's $PATH, absolute host paths) and only run for the
// primary root. installPaths, configPaths, and configFiles are
// root-relative and run for every root, including WSL homes.
type presenceDef struct {
	// binaries are command names resolved against the host $PATH.
	binaries []string
	// appBundles are .app bundle names checked under the macOS
	// application directories. Ignored on other platforms.
	appBundles []string
	// installDirs are install locations checked on Windows —
	// home-relative (e.g. AppData/Local/Programs/...) or absolute
	// (Program Files). Ignored on other platforms, where binaries and
	// appBundles cover installs.
	installDirs []string
	// installPaths are root-relative install artifacts (launcher
	// binaries, version stores) the client's own installer creates,
	// e.g. .local/bin/claude. They catch installs whose binary is not
	// on the scanning process's $PATH (daemon contexts, WSL roots).
	installPaths []string
	// configPaths are root-relative directories whose existence
	// indicates an install.
	configPaths []string
	// configFiles are root-relative files only the client itself
	// writes. Stronger than configPaths for directories that other
	// tools also create (e.g. ~/.claude).
	configFiles []string
}

// appBundleDirs is overridable in tests so detection doesn't depend on
// the real /Applications tree. nil → platform defaults (/Applications
// and ~/Applications on darwin).
var appBundleDirs []string

// detectPresence runs presence detection for every registered scanner
// against one root, adds a clients[] row whenever any signal fires,
// and returns the names detected on this root (the shared client
// table spans roots, so it can't answer per-root questions like skill
// attribution). Root-relative signals are checked for every root;
// host-level signals only for the primary root.
func detectPresence(s *state) map[string]bool {
	present := map[string]bool{}
	for _, c := range scanners {
		def := c.Presence(s.platform)
		binary, install, configPath := detectClientPresence(def, s)
		if binary == "" && install == "" && configPath == "" {
			continue
		}
		present[c.Name()] = true
		s.addClient(types.DeviceScanClient{
			Name:        c.Name(),
			BinaryPath:  binary,
			InstallPath: install,
			ConfigPath:  configPath,
		})
	}
	return present
}

// detectClientPresence returns the first-matching binary, install path,
// and config path for def. Empty strings mean no signal in that
// category.
func detectClientPresence(def presenceDef, s *state) (binary, install, configPath string) {
	if s.primary {
		for _, b := range def.binaries {
			if p, err := exec.LookPath(b); err == nil && p != "" {
				binary = p
				break
			}
		}

		switch s.platform {
		case "darwin":
			bundles := appBundleDirs
			if bundles == nil {
				bundles = []string{"/Applications", filepath.Join(s.base, "Applications")}
			}
		bundleLoop:
			for _, name := range def.appBundles {
				for _, dir := range bundles {
					candidate := filepath.Join(dir, name)
					if fi, err := os.Stat(candidate); err == nil && fi.IsDir() {
						install = candidate
						break bundleLoop
					}
				}
			}
		case "windows":
			for _, dir := range def.installDirs {
				candidate := dir
				if !filepath.IsAbs(dir) {
					candidate = filepath.Join(s.base, filepath.FromSlash(dir))
				}
				if fi, err := os.Stat(candidate); err == nil && fi.IsDir() {
					install = candidate
					break
				}
			}
		}
	}

	if install == "" {
		for _, rel := range def.installPaths {
			if _, err := fs.Stat(s.fsys, rel); err == nil {
				install = s.abs(rel)
				break
			}
		}
	}

	for _, rel := range def.configPaths {
		if dirExists(s.fsys, rel) {
			configPath = s.abs(rel)
			break
		}
	}
	if configPath == "" {
		for _, rel := range def.configFiles {
			if fileExists(s.fsys, rel) {
				configPath = s.abs(rel)
				break
			}
		}
	}
	return
}
