package api

import (
	clustersv1 "xata/gen/proto/clusters/v1"
	"xata/services/projects/store"
)

func createProjectConfig(name string, scaleToZero *store.ProjectScaleToZero) *store.CreateProjectConfiguration {
	cfg := &store.CreateProjectConfiguration{
		Name:        name,
		ScaleToZero: defaultProjectScaleToZeroConfig(),
		IPFiltering: store.IPFiltering{
			Enabled: false,
			CIDRs:   []store.CIDREntry{},
		},
	}
	if scaleToZero != nil {
		cfg.ScaleToZero = *scaleToZero
	}
	return cfg
}

func updateProjectConfig(name *string, scaleToZero *store.ProjectScaleToZero, ipFiltering *store.IPFiltering) *store.UpdateProjectConfiguration {
	return &store.UpdateProjectConfiguration{
		Name:        name,
		ScaleToZero: scaleToZero,
		IPFiltering: ipFiltering,
	}
}

func createBranchConfig(name string, parentID, description *string) *store.CreateBranchConfiguration {
	return &store.CreateBranchConfiguration{
		Name:                  name,
		ParentID:              parentID,
		Description:           description,
		BackupRetentionPeriod: DefaultBackupRetentionPeriod,
		BackupsEnabled:        true,
	}
}

func updateBranchConfig(name, description *string) *store.UpdateBranchConfiguration {
	return &store.UpdateBranchConfiguration{
		Name:        name,
		Description: description,
	}
}

func defaultClustersScaleToZero() *clustersv1.ScaleToZero {
	return &clustersv1.ScaleToZero{
		Enabled:                 false,
		InactivityPeriodMinutes: int64(defaultInactivityDuration.Minutes()),
	}
}
