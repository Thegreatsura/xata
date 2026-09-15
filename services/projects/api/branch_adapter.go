package api

import (
	"errors"
	"fmt"

	"xata/services/projects/api/spec"
	"xata/services/projects/branch"
)

// apiErrorFromBranch translates a branch domain error into the corresponding api
// error type, so HTTP responses (status code and message) stay identical to
// before the branch logic was extracted. Non-branch errors pass through.
func apiErrorFromBranch(err error) error {
	if err == nil {
		return nil
	}
	if invalidParam, ok := errors.AsType[branch.ErrInvalidParam](err); ok {
		return ErrorInvalidParam{ProjectID: invalidParam.ProjectID, BranchName: invalidParam.BranchName, Param: invalidParam.Param, Message: invalidParam.Message}
	}
	if branchNotFound, ok := errors.AsType[branch.ErrBranchNotFound](err); ok {
		return ErrorBranchNotFound{BranchID: branchNotFound.BranchID}
	}
	if invalidRegion, ok := errors.AsType[branch.ErrInvalidRegion](err); ok {
		return ErrorInvalidRegion{Message: invalidRegion.Message}
	}
	if _, ok := errors.AsType[branch.ErrChildBranchCreationDisabled](err); ok {
		return ErrorChildBranchCreationDisabled{}
	}
	if parentUnhealthy, ok := errors.AsType[branch.ErrParentBranchUnhealthy](err); ok {
		return ErrorParentBranchUnhealthy{ParentID: parentUnhealthy.ParentID}
	}
	return err
}

// toBranchMode maps the discriminated branch-creation value to a branch.Mode.
func toBranchMode(value any) (branch.Mode, error) {
	switch v := value.(type) {
	case spec.BranchFromParent:
		return branch.FromParent{ParentID: v.ParentID}, nil
	case spec.BranchFromConfiguration:
		return branch.FromConfiguration{Config: toBranchConfiguration(v.Configuration)}, nil
	default:
		return nil, fmt.Errorf("unsupported branch creation mode: %T", value)
	}
}

// toBranchConfiguration maps the spec cluster configuration to the neutral
// branch configuration.
func toBranchConfiguration(c spec.ClusterConfiguration) branch.Configuration {
	return branch.Configuration{
		Image:                           c.Image,
		Region:                          c.Region,
		InstanceType:                    c.InstanceType,
		Replicas:                        c.Replicas,
		PreloadLibraries:                c.PreloadLibraries,
		PostgresConfigurationParameters: c.PostgresConfigurationParameters,
		Storage:                         c.Storage,
	}
}

// toBranchBackup maps the spec backup configuration to the neutral one.
func toBranchBackup(c *spec.BackupConfiguration) *branch.BackupConfiguration {
	if c == nil {
		return nil
	}
	return &branch.BackupConfiguration{
		BackupTime:      c.BackupTime,
		RetentionPeriod: c.RetentionPeriod,
	}
}

// toBranchScaleToZero maps the spec scale-to-zero configuration to the neutral one.
func toBranchScaleToZero(s *spec.ScaleToZeroConfiguration) *branch.ScaleToZero {
	if s == nil {
		return nil
	}
	return &branch.ScaleToZero{
		Enabled:                 s.Enabled,
		InactivityPeriodMinutes: s.InactivityPeriodMinutes,
	}
}
