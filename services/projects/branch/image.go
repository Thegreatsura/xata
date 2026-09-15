package branch

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"xata/internal/flags"
	"xata/internal/postgresversions"
)

// MajorVersionEnabled reports whether a PostgreSQL major version hidden by
// default is available to the organization in ctx. A major marked hidden in
// versions.yaml without a flag in flags.PgMajorFlags stays hidden for everyone.
func (s *Service) MajorVersionEnabled(ctx context.Context, major string) bool {
	flag, ok := flags.PgMajorFlags[major]
	if !ok {
		return false
	}
	return s.feat.BoolValue(ctx, flag)
}

// ValidateImage resolves an image for a new branch, rejecting images that are
// not available to the organization.
func (s *Service) ValidateImage(ctx context.Context, organizationID, image string) (string, error) {
	// Reject experimental images if the feature flag is not enabled
	if strings.HasPrefix(image, "experimental:") && !s.feat.BoolValue(ctx, flags.ExperimentalImages) {
		return "", fmt.Errorf("image %s is not available", image)
	}

	// Reject analytics images if the feature flag is not enabled
	if strings.HasPrefix(image, "analytics:") && !s.feat.BoolValue(ctx, flags.AnalyticsImages) {
		return "", fmt.Errorf("image %s is not available", image)
	}

	// Reject major versions hidden by default if the flag for that major is not
	// enabled
	if major, hidden := postgresversions.HiddenMajorImages()[image]; hidden && !s.MajorVersionEnabled(ctx, major) {
		return "", fmt.Errorf("image %s is not available", image)
	}

	// Reject older minors hidden by default if the feature flag is not enabled
	if slices.Contains(postgresversions.HiddenImageNames(), image) && !s.feat.BoolValue(ctx, flags.LegacyPgVersions) {
		return "", fmt.Errorf("image %s is not available", image)
	}

	return s.resolveImage(ctx, organizationID, image)
}

// resolveImage checks that an image exists and returns its full registry URL,
// without applying the availability rules enforced by validateImage.
func (s *Service) resolveImage(ctx context.Context, organizationID, image string) (string, error) {
	allValidImages := s.imageProvider.GetAllImageNames()
	// TODO once the UI starts sending valid responses, remove the validImages var
	// this is only for backward compat
	if image == validImage {
		return "ghcr.io/xataio/postgres-images/cnpg-postgres-plus:17.5", nil
	}
	if slices.Contains(allValidImages, image) {
		imageURL := s.imageProvider.BuildImageURL(image)
		return imageURL, nil
	}
	return "", fmt.Errorf("image %s is not valid", image)
}

// ValidateImageUpgrade validates that newImage is a permissible in-place upgrade
// of currentImage (same offering and major, newer or equal minor), returning the
// resolved registry URL of newImage.
func (s *Service) ValidateImageUpgrade(ctx context.Context, organizationID, newImage, currentImage string) (string, error) {
	// Is the new image a valid one? This deliberately skips the availability
	// rules of validateImage: the checks below constrain the upgrade to a newer
	// minor of the offering and major the branch already runs, so a branch must
	// stay patchable even when that offering or version is hidden by default.
	newImageURL, err := s.resolveImage(ctx, organizationID, newImage)
	if err != nil {
		return "", err
	}

	// make sure the offering is the same and that the minor is bigger than the current one
	newImageInfo, err := s.imageProvider.ParseImageVersion(newImageURL)
	if err != nil {
		return "", err
	}
	currentImageInfo, err := s.imageProvider.ParseImageVersion(currentImage)
	if err != nil {
		return "", err
	}

	if newImageInfo.Offering != currentImageInfo.Offering {
		return "", fmt.Errorf("incompatible offering: %s is not compatible with %s", newImageInfo.Offering, currentImageInfo.Offering)
	}
	if newImageInfo.Major != currentImageInfo.Major {
		return "", fmt.Errorf("no major version upgrades supported: %d is different than current %d", newImageInfo.Major, currentImageInfo.Major)
	}

	if newImageInfo.Minor < currentImageInfo.Minor {
		return "", fmt.Errorf("new minor: %d is older than current  %d", newImageInfo.Minor, currentImageInfo.Minor)
	}

	return newImageURL, nil
}
