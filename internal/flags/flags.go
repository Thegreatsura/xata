package flags

import "xata/internal/openfeature"

var (
	// XataUser flag to expose 'xata' user instead of 'superuser' for connections
	XataUser = openfeature.FeatureFlag{
		Name:           "xataUser",
		DefaultEnabled: false,
	}
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
	UseClusterPool = openfeature.FeatureFlag{
		Name:           "useClusterPool",
		DefaultEnabled: false,
	}
	// WARNING: Feature Flags should have positive names. Avoid disabled suffix in future
)
