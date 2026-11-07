//go:build windows

package clean

// platformRules returns the Windows cleanup rules. Paths use environment
// variables so they follow the current user's profile and drive layout.
func platformRules() []Rule {
	return []Rule{
		// ---- Development caches ----
		{
			ID: "go-build-cache", Name: "Go build cache", Category: "dev", Risk: RiskLow,
			Paths: []string{"%LOCALAPPDATA%\\go-build", "%USERPROFILE%\\AppData\\Local\\go-build"},
			Note:  "Compiled package cache; `go build` recreates it (`go clean -cache`).",
		},
		{
			ID: "go-mod-cache", Name: "Go module downloads", Category: "dev", Risk: RiskMedium,
			Paths: []string{"%USERPROFILE%\\go\\pkg\\mod\\cache\\download"},
		},
		{
			ID: "npm-cache", Name: "npm cache", Category: "dev", Risk: RiskLow,
			Paths: []string{"%LOCALAPPDATA%\\npm-cache\\_cacache", "%APPDATA%\\npm-cache\\_cacache"},
		},
		{
			ID: "pnpm-store", Name: "pnpm store", Category: "dev", Risk: RiskMedium,
			Paths: []string{"%LOCALAPPDATA%\\pnpm\\store", "%LOCALAPPDATA%\\pnpm-store"},
		},
		{
			ID: "yarn-cache", Name: "Yarn cache", Category: "dev", Risk: RiskLow,
			Paths: []string{"%LOCALAPPDATA%\\Yarn\\Cache"},
		},
		{
			ID: "pip-cache", Name: "pip cache", Category: "dev", Risk: RiskLow,
			Paths: []string{"%LOCALAPPDATA%\\pip\\Cache"},
		},
		{
			ID: "nuget-cache", Name: "NuGet packages", Category: "dev", Risk: RiskMedium,
			Paths: []string{"%USERPROFILE%\\.nuget\\packages"},
			Note:  "Restored from nuget.org on the next build.",
		},
		{
			ID: "cargo-cache", Name: "Cargo registry cache", Category: "dev", Risk: RiskMedium,
			Paths: []string{"%USERPROFILE%\\.cargo\\registry\\cache", "%USERPROFILE%\\.cargo\\registry\\src"},
		},
		{
			ID: "gradle-cache", Name: "Gradle caches", Category: "dev", Risk: RiskMedium,
			Paths: []string{"%USERPROFILE%\\.gradle\\caches"},
		},
		{
			ID: "maven-cache", Name: "Maven repository", Category: "dev", Risk: RiskMedium,
			Paths: []string{"%USERPROFILE%\\.m2\\repository"},
		},
		{
			ID: "vs-temp", Name: "Visual Studio temp files", Category: "dev", Risk: RiskLow,
			Paths: []string{"%LOCALAPPDATA%\\Temp\\VSFeedbackVSRTCLogs", "%LOCALAPPDATA%\\Microsoft\\VSCommon\\*\\Logs"},
		},

		// ---- Browsers ----
		{
			ID: "chrome-cache", Name: "Chrome cache", Category: "browsers", Risk: RiskLow,
			Paths: []string{"%LOCALAPPDATA%\\Google\\Chrome\\User Data\\Default\\Cache", "%LOCALAPPDATA%\\Google\\Chrome\\User Data\\Default\\Code Cache"},
			Note:  "Cookies, logins and history are kept.",
		},
		{
			ID: "edge-cache", Name: "Edge cache", Category: "browsers", Risk: RiskLow,
			Paths: []string{"%LOCALAPPDATA%\\Microsoft\\Edge\\User Data\\Default\\Cache", "%LOCALAPPDATA%\\Microsoft\\Edge\\User Data\\Default\\Code Cache"},
		},
		{
			ID: "firefox-cache", Name: "Firefox cache", Category: "browsers", Risk: RiskLow,
			Paths: []string{"%LOCALAPPDATA%\\Mozilla\\Firefox\\Profiles\\*\\cache2"},
		},
		{
			ID: "brave-cache", Name: "Brave cache", Category: "browsers", Risk: RiskLow,
			Paths: []string{"%LOCALAPPDATA%\\BraveSoftware\\Brave-Browser\\User Data\\Default\\Cache"},
		},

		// ---- Temporary files, logs, dumps ----
		{
			ID: "user-temp", Name: "User temp files", Category: "temp", Risk: RiskLow,
			Paths: []string{"%TEMP%"},
			Note:  "Files in use by running programs are skipped.",
		},
		{
			ID: "crash-dumps", Name: "Crash dumps", Category: "logs", Risk: RiskLow,
			Paths: []string{"%LOCALAPPDATA%\\CrashDumps"},
		},
		{
			ID: "thumbnails", Name: "Explorer thumbnail cache", Category: "system", Risk: RiskLow,
			Paths: []string{"%LOCALAPPDATA%\\Microsoft\\Windows\\Explorer\\thumbcache_*.db"},
			Note:  "Recreated automatically; Explorer may show placeholders until then.",
		},

		// ---- System-wide (needs admin rights) ----
		{
			ID: "system-temp", Name: "Windows temp files", Category: "system", Risk: RiskMedium, Admin: true,
			Paths: []string{"%SystemRoot%\\Temp"},
		},
		{
			ID: "windows-update-cache", Name: "Windows Update download cache", Category: "system", Risk: RiskMedium, Admin: true,
			Paths: []string{"%SystemRoot%\\SoftwareDistribution\\Download"},
			Note:  "Already installed updates are re-downloaded only if rollback needs them.",
		},
		{
			ID: "delivery-optimization", Name: "Delivery Optimization cache", Category: "system", Risk: RiskMedium, Admin: true,
			Paths: []string{"%SystemRoot%\\SoftwareDistribution\\DeliveryOptimization"},
		},
		{
			ID: "error-reports", Name: "Windows Error Reporting queue", Category: "logs", Risk: RiskLow, Admin: true,
			Paths: []string{"%ProgramData%\\Microsoft\\Windows\\WER\\ReportQueue", "%ProgramData%\\Microsoft\\Windows\\WER\\ReportArchive"},
		},
	}
}
