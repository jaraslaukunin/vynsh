//go:build darwin

package clean

// platformRules returns the macOS cleanup rules. Order matters: rules for
// specific paths come before the broad "all caches" rule so that a specific
// target is attributed to the specific rule, not the catch-all one.
func platformRules() []Rule {
	return []Rule{
		// ---- Development caches (rebuilt or re-downloaded on demand) ----
		{
			ID: "go-build-cache", Name: "Go build cache", Category: "dev", Risk: RiskLow,
			Paths: []string{"~/Library/Caches/go-build", "~/.cache/go-build"},
			Note:  "Compiled package cache; `go build` recreates it (`go clean -cache`).",
		},
		{
			ID: "go-mod-cache", Name: "Go module downloads", Category: "dev", Risk: RiskMedium,
			Paths: []string{"~/go/pkg/mod/cache/download"},
			Note:  "Module archives are re-downloaded on the next build (`go clean -modcache` removes more).",
		},
		{
			ID: "npm-cache", Name: "npm cache", Category: "dev", Risk: RiskLow,
			Paths: []string{"~/.npm/_cacache"},
		},
		{
			ID: "pnpm-store", Name: "pnpm store", Category: "dev", Risk: RiskMedium,
			Paths: []string{"~/Library/pnpm/store", "~/.local/share/pnpm/store"},
			Note:  "Packages are re-fetched from the registry.",
		},
		{
			ID: "yarn-cache", Name: "Yarn cache", Category: "dev", Risk: RiskLow,
			Paths: []string{"~/Library/Caches/Yarn", "~/.cache/yarn"},
		},
		{
			ID: "pip-cache", Name: "pip cache", Category: "dev", Risk: RiskLow,
			Paths: []string{"~/Library/Caches/pip"},
		},
		{
			ID: "cargo-cache", Name: "Cargo registry cache", Category: "dev", Risk: RiskMedium,
			Paths: []string{"~/.cargo/registry/cache", "~/.cargo/registry/src"},
			Note:  "Crates are re-downloaded; compiled `target/` dirs are untouched.",
		},
		{
			ID: "gradle-cache", Name: "Gradle caches", Category: "dev", Risk: RiskMedium,
			Paths: []string{"~/.gradle/caches"},
		},
		{
			ID: "maven-cache", Name: "Maven repository", Category: "dev", Risk: RiskMedium,
			Paths: []string{"~/.m2/repository"},
			Note:  "Artifacts are re-downloaded from Maven Central.",
		},
		{
			ID: "xcode-derived-data", Name: "Xcode DerivedData", Category: "dev", Risk: RiskLow,
			Paths: []string{"~/Library/Developer/Xcode/DerivedData/*"},
			Note:  "Build products and indexes; the next Xcode build regenerates them.",
		},
		{
			ID: "simulator-caches", Name: "iOS Simulator caches", Category: "dev", Risk: RiskMedium,
			Paths: []string{"~/Library/Developer/CoreSimulator/Caches/*"},
		},
		{
			ID: "ios-device-support", Name: "iOS/watchOS device support", Category: "dev", Risk: RiskHigh,
			Paths: []string{
				"~/Library/Developer/Xcode/iOS DeviceSupport/*",
				"~/Library/Developer/Xcode/watchOS DeviceSupport/*",
				"~/Library/Developer/Xcode/tvOS DeviceSupport/*",
			},
			Note: "Symbols for older iOS versions; Xcode re-downloads them when a device needs them.",
		},
		{
			ID: "xcode-archives", Name: "Xcode archives (app builds)", Category: "dev", Risk: RiskHigh,
			Paths: []string{"~/Library/Developer/Xcode/Archives/*"},
			Note:  "Archives are your only copy of shipped builds unless uploaded. Check App Store Connect first.",
		},

		// ---- Browsers ----
		{
			ID: "safari-cache", Name: "Safari cache", Category: "browsers", Risk: RiskLow,
			Paths: []string{
				"~/Library/Caches/com.apple.Safari",
				"~/Library/Containers/com.apple.Safari/Data/Library/Caches/*",
			},
		},
		{
			ID: "chrome-cache", Name: "Chrome cache", Category: "browsers", Risk: RiskLow,
			Paths: []string{"~/Library/Caches/Google/Chrome", "~/Library/Caches/com.google.Chrome"},
			Note:  "Visited pages are re-fetched; cookies, logins and history are kept.",
		},
		{
			ID: "firefox-cache", Name: "Firefox cache", Category: "browsers", Risk: RiskLow,
			Paths: []string{"~/Library/Caches/Firefox"},
		},
		{
			ID: "arc-cache", Name: "Arc / Chromium cache", Category: "browsers", Risk: RiskLow,
			Paths: []string{"~/Library/Caches/Arc", "~/Library/Caches/com.thebrowser.Browser"},
		},

		// ---- Application caches and logs (catch-all, keep last) ----
		{
			ID: "user-caches", Name: "All user app caches", Category: "apps", Risk: RiskLow,
			Paths: []string{"~/Library/Caches/*"},
			Note:  "Everything in ~/Library/Caches not listed separately. Apps rebuild caches on launch.",
		},
		{
			ID: "user-logs", Name: "User logs", Category: "logs", Risk: RiskLow,
			Paths: []string{"~/Library/Logs/*"},
		},
		{
			ID: "crash-reports", Name: "Crash reports", Category: "logs", Risk: RiskLow,
			Paths: []string{"~/Library/Logs/DiagnosticReports/*"},
		},
		{
			ID: "saved-state", Name: "Saved application state", Category: "apps", Risk: RiskLow,
			Paths: []string{"~/Library/Saved Application State/*"},
			Note:  "Window restore data; apps open with a fresh window layout.",
		},
		{
			ID: "mail-downloads", Name: "Mail attachments cache", Category: "apps", Risk: RiskMedium,
			Paths: []string{"~/Library/Containers/com.apple.mail/Data/Library/Mail Downloads/*"},
		},
		{

			ID: "homebrew-cache", Name: "Homebrew downloads", Category: "dev", Risk: RiskLow,
			Paths: []string{"~/Library/Caches/Homebrew"},
			Note:  "Prefer `brew cleanup --prune=all` which also handles older versions.",
		},

		// ---- System-wide (needs admin rights) ----
		{
			ID: "system-caches", Name: "System caches", Category: "system", Risk: RiskMedium, Admin: true,
			Paths: []string{"/Library/Caches/*"},
		},
		{
			ID: "system-logs", Name: "System crash reports", Category: "system", Risk: RiskLow, Admin: true,
			Paths: []string{"/Library/Logs/DiagnosticReports/*"},
		},
		{
			ID: "tmp-files", Name: "Temporary files", Category: "system", Risk: RiskMedium, Admin: true,
			Paths: []string{"/private/tmp/*", "/private/var/tmp/*"},
			Note:  "Only remove files you own; the tool skips what it cannot access.",
		},
	}
}
