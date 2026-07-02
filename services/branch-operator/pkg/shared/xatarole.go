package shared

import (
	apiv1 "github.com/xataio/xata-cnpg/api/v1"
	apiv1ac "github.com/xataio/xata-cnpg/pkg/client/applyconfiguration/api/v1"
	corev1ac "k8s.io/client-go/applyconfigurations/core/v1"
)

// XataRoleName is the name of the `xata` application role.
const XataRoleName = "xata"

// XataRoleConfiguration returns the managed role configuration for the `xata`
// application role. CNPG reconciles the role's password from the referenced
// `{branchName}-app` secret, which is the only mechanism that syncs the
// password onto Clusters taken from a pool (where `bootstrap.initdb.secret`
// never runs). The attributes match those set by the initdb post-init SQL.
func XataRoleConfiguration(branchName string) *apiv1ac.RoleConfigurationApplyConfiguration {
	return apiv1ac.RoleConfiguration().
		WithName(XataRoleName).
		WithEnsure(apiv1.EnsurePresent).
		WithPasswordSecret(corev1ac.LocalObjectReference().
			WithName(branchName + "-app")).
		WithLogin(true).
		WithInherit(true).
		WithCreateDB(true).
		WithCreateRole(true).
		WithBypassRLS(true).
		WithReplication(true).
		WithInRoles("xata_superuser")
}
