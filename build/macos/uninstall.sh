#!/usr/bin/env bash
# Remove Obot Sentry from this Mac: both launchd jobs, the binary, the pkg
# receipt, any local configuration, and the device identity plus every
# user's scan state. Idempotent, and harmless on a Mac where Obot Sentry
# was never installed.
#
# It deliberately leaves the local-agent audit hooks alone: `obot-sentry
# hook-uninstall` owns that, and it targets the active console user, so it
# runs once per user rather than once per Mac. Run it first - hooks that
# outlive the binary leave each coding agent invoking a command that no
# longer exists.
#
# Usage: sudo bash uninstall.sh
set -euo pipefail

if [[ "$(id -u)" != 0 ]]; then
	echo "uninstall.sh must run as root: sudo bash uninstall.sh" >&2
	exit 1
fi

launchctl bootout system/ai.obot.obot-sentry.hook-install 2>/dev/null || true

# The scan agent runs in each signed-in user's GUI domain, so unload it per
# session. One loginwindow process per session gives the uids to visit.
while IFS= read -r uid; do
	launchctl bootout "gui/${uid}/ai.obot.obot-sentry.scan" 2>/dev/null || true
done < <(ps -axo uid= -o command= | awk '/[l]oginwindow.app/ { print $1 }' | sort -u)

rm -f /usr/local/bin/obot-sentry \
	/Library/LaunchAgents/ai.obot.obot-sentry.scan.plist \
	/Library/LaunchDaemons/ai.obot.obot-sentry.hook-install.plist
pkgutil --forget ai.obot.obot-sentry 2>/dev/null || true

# Local configuration only. MDM-delivered settings live under /Library/Managed
# Preferences and go away with the configuration profile.
defaults delete /Library/Preferences/ai.obot.obot-sentry 2>/dev/null || true

rm -rf "/Library/Application Support/obot/obot-sentry"
for home in /Users/*; do
	[[ -d "$home" ]] || continue
	rm -rf "$home/Library/Application Support/obot/obot-sentry" \
		"$home/Library/Caches/obot/obot-sentry"
done

echo "Obot Sentry removed. Full Disk Access entries for /usr/local/bin/obot-sentry," \
	"if any were granted by hand, remain in System Settings."
