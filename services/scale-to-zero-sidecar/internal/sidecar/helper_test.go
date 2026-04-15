package sidecar

import (
	"context"

	cnpgv1 "github.com/xataio/xata-cnpg/api/v1"

	branchv1alpha1 "xata/services/branch-operator/api/v1alpha1"
	"xata/services/scale-to-zero-sidecar/internal/postgres"
)

type mockClusterClient struct {
	getClusterFunc                   func(ctx context.Context, forceUpdate bool) (*cnpgv1.Cluster, error)
	updateClusterFunc                func(ctx context.Context, cluster *cnpgv1.Cluster) error
	getClusterCredentialsFunc        func(ctx context.Context) (*postgreSQLCredentials, error)
	getClusterScheduledBackupFunc    func(ctx context.Context) (*cnpgv1.ScheduledBackup, error)
	updateClusterScheduledBackupFunc func(ctx context.Context, scheduledBackup *cnpgv1.ScheduledBackup) error
	getBranchFunc                    func(ctx context.Context) (*branchv1alpha1.Branch, error)
	patchBranchHibernationFunc       func(ctx context.Context, branch *branchv1alpha1.Branch) error
	patchBranchClusterNameFunc       func(ctx context.Context, branch *branchv1alpha1.Branch) error
}

func (m *mockClusterClient) getCluster(ctx context.Context, forceUpdate bool) (*cnpgv1.Cluster, error) {
	if m.getClusterFunc != nil {
		return m.getClusterFunc(ctx, forceUpdate)
	}
	return nil, nil
}

func (m *mockClusterClient) updateCluster(ctx context.Context, cluster *cnpgv1.Cluster) error {
	if m.updateClusterFunc != nil {
		return m.updateClusterFunc(ctx, cluster)
	}
	return nil
}

func (m *mockClusterClient) getClusterCredentials(ctx context.Context) (*postgreSQLCredentials, error) {
	if m.getClusterCredentialsFunc != nil {
		return m.getClusterCredentialsFunc(ctx)
	}
	return nil, nil
}

func (m *mockClusterClient) getClusterScheduledBackup(ctx context.Context) (*cnpgv1.ScheduledBackup, error) {
	if m.getClusterScheduledBackupFunc != nil {
		return m.getClusterScheduledBackupFunc(ctx)
	}
	return nil, nil
}

func (m *mockClusterClient) updateClusterScheduledBackup(ctx context.Context, scheduledBackup *cnpgv1.ScheduledBackup) error {
	if m.updateClusterScheduledBackupFunc != nil {
		return m.updateClusterScheduledBackupFunc(ctx, scheduledBackup)
	}
	return nil
}

func (m *mockClusterClient) getBranch(ctx context.Context) (*branchv1alpha1.Branch, error) {
	if m.getBranchFunc != nil {
		return m.getBranchFunc(ctx)
	}
	return nil, nil
}

func (m *mockClusterClient) patchBranchHibernation(ctx context.Context, branch *branchv1alpha1.Branch) error {
	if m.patchBranchHibernationFunc != nil {
		return m.patchBranchHibernationFunc(ctx, branch)
	}
	return nil
}

func (m *mockClusterClient) patchBranchClusterName(ctx context.Context, branch *branchv1alpha1.Branch) error {
	if m.patchBranchClusterNameFunc != nil {
		return m.patchBranchClusterNameFunc(ctx, branch)
	}
	return nil
}

type mockQuerier struct {
	queryFunc func(ctx context.Context, query string, args ...any) (postgres.Row, error)
}

func (m *mockQuerier) QueryRow(ctx context.Context, query string, args ...any) postgres.Row {
	if m.queryFunc != nil {
		row, err := m.queryFunc(ctx, query, args...)
		if err != nil {
			return &mockRow{
				scanFn: func(dest ...any) error {
					return err
				},
			}
		}
		return row
	}
	return nil
}

func (m *mockQuerier) Close(ctx context.Context) error {
	// Implement close logic if needed
	return nil
}

type mockRow struct {
	scanFn func(dest ...any) error
}

func (m *mockRow) Scan(dest ...any) error {
	if m.scanFn != nil {
		return m.scanFn(dest...)
	}
	return nil
}
