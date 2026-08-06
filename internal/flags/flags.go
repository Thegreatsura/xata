package flags

import "xata/internal/openfeature"

var (
	OrgAutoWindDown = openfeature.FeatureFlag{
		Name:           "orgAutoWindDown",
		DefaultEnabled: true,
	}
	OrganizationCreation = openfeature.FeatureFlag{
		Name:           "organizationCreation",
		DefaultEnabled: true,
	}
	// WARNING: Feature Flags should have positive names. Avoid disabled suffix in future
	BranchCreationDisabled = openfeature.FeatureFlag{
		Name:           "branchCreationDisabled",
		DefaultEnabled: false,
	}
	ChildBranchCreationDisabled = openfeature.FeatureFlag{
		Name:           "childBranchCreationDisabled",
		DefaultEnabled: false,
	}
	// ExperimentalImages flag to enable experimental PostgreSQL images (for internal users)
	ExperimentalImages = openfeature.FeatureFlag{
		Name:           "experimentalImages",
		DefaultEnabled: false,
	}
	// AnalyticsImages flag to enable analytics PostgreSQL images
	AnalyticsImages = openfeature.FeatureFlag{
		Name:           "analyticsImages",
		DefaultEnabled: false,
	}
	// LegacyPgVersions flag to enable older PostgreSQL minor versions that
	// are hidden by default (see show_only_latest in versions.yaml)
	LegacyPgVersions = openfeature.FeatureFlag{
		Name:           "legacyPgVersions",
		DefaultEnabled: false,
	}
	// PgMajor14 and PgMajor15 flag to enable PostgreSQL major versions that are
	// hidden by default (see hidden in versions.yaml)
	PgMajor14 = openfeature.FeatureFlag{
		Name:           "pgMajor14",
		DefaultEnabled: false,
	}
	PgMajor15 = openfeature.FeatureFlag{
		Name:           "pgMajor15",
		DefaultEnabled: false,
	}
	UseClusterPool = openfeature.FeatureFlag{
		Name:           "useClusterPool",
		DefaultEnabled: false,
	}
	UseXatastor = openfeature.FeatureFlag{
		Name:           "useXatastor",
		DefaultEnabled: false,
	}
	UsePgBackRest = openfeature.FeatureFlag{
		Name:           "usePgBackRest",
		DefaultEnabled: false,
	}
	// WARNING: Feature Flags should have positive names. Avoid disabled suffix in future
)

// PgMajorFlags maps a PostgreSQL major version hidden by default to the feature
// flag that makes it available to an organization. Every major marked hidden in
// versions.yaml must have an entry here.
var PgMajorFlags = map[string]openfeature.FeatureFlag{
	"14": PgMajor14,
	"15": PgMajor15,
}
