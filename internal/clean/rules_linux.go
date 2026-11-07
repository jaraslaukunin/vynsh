//go:build linux

package clean

// platformRules returns the Linux cleanup rules. Anything that needs root is
// marked Admin and only selected with --admin.
func platformRules() []Rule {
	return []Rule{
		// ---- Development caches ----
		{
			ID: "go-build-cache", Name: "Go build cache", Category: "dev", Risk: RiskLow,
			Paths: []string{"~/.cache/go-build"},
			Note:  "Compiled package cache; `go build` recreates it (`go clean -cache`).",
		},
		{
			ID: "go-mod-cache", Name: "Go module downloads", Category: "dev", Risk: RiskMedium,
			Paths: []string{"~/go/pkg/mod/cache/download"},
		},
		{
			ID: "npm-cache", Name: "npm cache", Category: "dev", Risk: RiskLow,
			Paths: []string{"~/.npm/_cacache"},
		},
		{
			ID: "pnpm-store", Name: "pnpm store", Category: "dev", Risk: RiskMedium,
			Paths: []string{"~/.local/share/pnpm/store", "~/.pnpm-store"},
		},
		{
			ID: "yarn-cache", Name: "Yarn cache", Category: "dev", Risk: RiskLow,
			Paths: []string{"~/.cache/yarn"},
		},
		{
			ID: "pip-cache", Name: "pip cache", Category: "dev", Risk: RiskLow,
			Paths: []string{"~/.cache/pip"},
		},
		{
			ID: "cargo-cache", Name: "Cargo registry cache", Category: "dev", Risk: RiskMedium,
			Paths: []string{"~/.cargo/registry/cache", "~/.cargo/registry/src"},
		},
		{
			ID: "gradle-cache", Name: "Gradle caches", Category: "dev", Risk: RiskMedium,
			Paths: []string{"~/.gradle/caches"},
		},
		{
			ID: "maven-cache", Name: "Maven repository", Category: "dev", Risk: RiskMedium,
			Paths: []string{"~/.m2/repository"},
		},
		{
			ID: "ccache", Name: "ccache / compiler cache", Category: "dev", Risk: RiskLow,
			Paths: []string{"~/.ccache", "~/.cache/ccache"},
		},

		// ---- Browsers ----
		{
			ID: "chrome-cache", Name: "Chrome / Chromium cache", Category: "browsers", Risk: RiskLow,
			Paths: []string{"~/.cache/google-chrome", "~/.cache/chromium"},
		},
		{
			ID: "firefox-cache", Name: "Firefox cache", Category: "browsers", Risk: RiskLow,
			Paths: []string{"~/.cache/mozilla/firefox"},
		},
		{
			ID: "brave-cache", Name: "Brave cache", Category: "browsers", Risk: RiskLow,
			Paths: []string{"~/.cache/BraveSoftware"},
		},

		// ---- Applications (catch-all, keep last) ----
		{
			ID: "user-caches", Name: "All user cache directories", Category: "apps", Risk: RiskLow,
			Paths: []string{"~/.cache/*"},
			Note:  "Everything in ~/.cache not listed separately; apps rebuild it.",
		},
		{
			ID: "thumbnails", Name: "Thumbnail cache", Category: "apps", Risk: RiskLow,
			Paths: []string{"~/.cache/thumbnails", "~/.thumbnails"},
		},

		// ---- Temporary files ----
		{
			ID: "user-temp", Name: "User temporary files", Category: "temp", Risk: RiskMedium,
			Paths: []string{"/tmp/*", "/var/tmp/*"},
			Note:  "Only files you own are removed; the rest is skipped.",
		},

		// ---- System-wide (needs root) ----
		{
			ID: "apt-cache", Name: "APT package cache", Category: "system", Risk: RiskLow, Admin: true,
			Paths: []string{"/var/cache/apt/archives/*.deb"},
		},
		{
			ID: "pacman-cache", Name: "Pacman package cache", Category: "system", Risk: RiskMedium, Admin: true,
			Paths: []string{"/var/cache/pacman/pkg/*"},
			Note:  "Older versions are re-downloadable; `paccache -r` keeps the last few.",
		},
		{
			ID: "journal-logs", Name: "systemd journal", Category: "system", Risk: RiskMedium, Admin: true,
			Paths: []string{"/var/log/journal/*"},
			Note:  "Prefer `journalctl --vacuum-time=14d`, which keeps the journal consistent.",
		},
		{
			ID: "system-logs", Name: "Rotated system logs", Category: "logs", Risk: RiskLow, Admin: true,
			Paths: []string{"/var/log/*.gz", "/var/log/*.1", "/var/log/*.old"},
		},
		{
			ID: "snapd-cache", Name: "Snap package cache", Category: "system", Risk: RiskMedium, Admin: true,
			Paths: []string{"/var/lib/snapd/cache/*"},
		},
	}
}
