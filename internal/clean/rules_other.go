//go:build !darwin && !windows && !linux

package clean

// platformRules returns a small, generic rule set for platforms vynsh is not
// officially supporting yet. It keeps the CLI usable (and testable) on Linux.
func platformRules() []Rule {
	return []Rule{
		{
			ID: "go-build-cache", Name: "Go build cache", Category: "dev", Risk: RiskLow,
			Paths: []string{"~/.cache/go-build"},
		},
		{
			ID: "npm-cache", Name: "npm cache", Category: "dev", Risk: RiskLow,
			Paths: []string{"~/.npm/_cacache"},
		},
		{
			ID: "pip-cache", Name: "pip cache", Category: "dev", Risk: RiskLow,
			Paths: []string{"~/.cache/pip"},
		},
		{
			ID: "user-caches", Name: "User cache directories", Category: "apps", Risk: RiskLow,
			Paths: []string{"~/.cache/*"},
		},
		{
			ID: "tmp-files", Name: "Temporary files", Category: "system", Risk: RiskMedium,
			Paths: []string{"/tmp/*", "/var/tmp/*"},
		},
	}
}
