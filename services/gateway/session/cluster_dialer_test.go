package session

import (
	"context"
	"errors"
	"net"
	"syscall"
	"testing"
	"time"

	clustersv1 "xata/gen/proto/clusters/v1"
	"xata/gen/protomocks"

	"github.com/stretchr/testify/require"
	apiv1 "github.com/xataio/xata-cnpg/api/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/resolver"
	"google.golang.org/grpc/resolver/manual"
	"google.golang.org/grpc/status"
)

func TestClusterDialer_Dial(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	errTest := errors.New("oh noes")
	errDNS := &net.DNSError{Err: "server misbehaving", Name: "branch-rw.svc", IsTemporary: true}

	tests := map[string]struct {
		dialer          *mockDialer
		clustersService clustersServiceClientFn
		setupMocks      func(*protomocks.ClustersServiceClient)

		wantDialCalls    uint // exact expected dial count; ignored if wantMinDialCalls is set
		wantMinDialCalls uint // for timeout-driven tests where the exact count depends on tick timing
		wantErr          error
	}{
		"ok - no dial error": {
			dialer: &mockDialer{
				dialFn: func(ctx context.Context, _ uint, network, address string) (net.Conn, error) {
					return &net.TCPConn{}, nil
				},
			},
			setupMocks: func(mockClusters *protomocks.ClustersServiceClient) {},

			wantDialCalls: 1,
			wantErr:       nil,
		},
		"ok - hibernated cluster with scale to zero reactivates on dial": {
			dialer: &mockDialer{
				dialFn: func(ctx context.Context, i uint, network, address string) (net.Conn, error) {
					switch i {
					case 1:
						return nil, syscall.ECONNREFUSED
					case 2:
						return &net.TCPConn{}, nil
					default:
						return nil, errors.New("unexpected dial call")
					}
				},
			},
			setupMocks: func(mockClusters *protomocks.ClustersServiceClient) {
				mockClusters.EXPECT().DescribePostgresCluster(ctx, &clustersv1.DescribePostgresClusterRequest{
					Id: "test-branch",
				}).Return(&clustersv1.DescribePostgresClusterResponse{
					Status: &clustersv1.ClusterStatus{
						StatusType: clustersv1.ClusterStatus_STATUS_TYPE_HIBERNATED,
					},
					Configuration: &clustersv1.ClusterConfiguration{
						ScaleToZero: &clustersv1.ScaleToZero{Enabled: true},
					},
				}, nil).Once()

				mockClusters.EXPECT().UpdatePostgresCluster(ctx, &clustersv1.UpdatePostgresClusterRequest{
					Id: "test-branch",
					UpdateConfiguration: &clustersv1.UpdateClusterConfiguration{
						Hibernate: new(false),
					},
				}).Return(&clustersv1.UpdatePostgresClusterResponse{}, nil).Once()

				mockClusters.EXPECT().DescribePostgresCluster(ctx, &clustersv1.DescribePostgresClusterRequest{
					Id: "test-branch",
				}).Return(&clustersv1.DescribePostgresClusterResponse{
					Status: &clustersv1.ClusterStatus{
						StatusType:         clustersv1.ClusterStatus_STATUS_TYPE_HEALTHY,
						InstanceCount:      1,
						InstanceReadyCount: 1,
					},
				}, nil).Once()
			},

			wantDialCalls: 2,
			wantErr:       nil,
		},
		"ok - DNS not found triggers reactivation for hibernated cluster": {
			dialer: &mockDialer{
				dialFn: func(ctx context.Context, i uint, network, address string) (net.Conn, error) {
					switch i {
					case 1:
						return nil, &net.DNSError{Err: "no such host", Name: "branch-rw.xata-clusters.svc", IsNotFound: true}
					case 2:
						return &net.TCPConn{}, nil
					default:
						return nil, errors.New("unexpected dial call")
					}
				},
			},
			setupMocks: func(mockClusters *protomocks.ClustersServiceClient) {
				mockClusters.EXPECT().DescribePostgresCluster(ctx, &clustersv1.DescribePostgresClusterRequest{
					Id: "test-branch",
				}).Return(&clustersv1.DescribePostgresClusterResponse{
					Status: &clustersv1.ClusterStatus{
						StatusType: clustersv1.ClusterStatus_STATUS_TYPE_HIBERNATED,
					},
					Configuration: &clustersv1.ClusterConfiguration{
						ScaleToZero: &clustersv1.ScaleToZero{Enabled: true},
					},
				}, nil).Once()

				mockClusters.EXPECT().UpdatePostgresCluster(ctx, &clustersv1.UpdatePostgresClusterRequest{
					Id: "test-branch",
					UpdateConfiguration: &clustersv1.UpdateClusterConfiguration{
						Hibernate: new(false),
					},
				}).Return(&clustersv1.UpdatePostgresClusterResponse{}, nil).Once()

				mockClusters.EXPECT().DescribePostgresCluster(ctx, &clustersv1.DescribePostgresClusterRequest{
					Id: "test-branch",
				}).Return(&clustersv1.DescribePostgresClusterResponse{
					Status: &clustersv1.ClusterStatus{
						StatusType:         clustersv1.ClusterStatus_STATUS_TYPE_HEALTHY,
						InstanceCount:      1,
						InstanceReadyCount: 1,
					},
				}, nil).Once()
			},

			wantDialCalls: 2,
			wantErr:       nil,
		},
		"ok - connection refused scale to zero enabled and hibernated cluster, reactivation ongoing": {
			dialer: &mockDialer{
				dialFn: func(ctx context.Context, i uint, network, address string) (net.Conn, error) {
					switch i {
					case 1:
						return nil, syscall.ECONNREFUSED
					case 2:
						return &net.TCPConn{}, nil
					default:
						return nil, errors.New("unexpected dial call")
					}
				},
			},
			setupMocks: func(mockClusters *protomocks.ClustersServiceClient) {
				mockClusters.EXPECT().DescribePostgresCluster(ctx, &clustersv1.DescribePostgresClusterRequest{
					Id: "test-branch",
				}).Return(&clustersv1.DescribePostgresClusterResponse{
					Status: &clustersv1.ClusterStatus{
						StatusType: clustersv1.ClusterStatus_STATUS_TYPE_TRANSIENT,
						Status:     apiv1.PhaseWaitingForInstancesToBeActive,
					},
					Configuration: &clustersv1.ClusterConfiguration{
						ScaleToZero: &clustersv1.ScaleToZero{Enabled: true},
					},
				}, nil).Once()

				mockClusters.EXPECT().DescribePostgresCluster(ctx, &clustersv1.DescribePostgresClusterRequest{
					Id: "test-branch",
				}).Return(&clustersv1.DescribePostgresClusterResponse{
					Status: &clustersv1.ClusterStatus{
						StatusType:         clustersv1.ClusterStatus_STATUS_TYPE_TRANSIENT,
						InstanceCount:      2,
						InstanceReadyCount: 1,
						Instances: map[string]*clustersv1.InstanceStatus{
							"instance-1": {
								Primary: true,
								Status:  apiv1.PodHealthy,
							},
							"instance-2": {
								Primary: false,
								Status:  apiv1.PodFailed,
							},
						},
					},
				}, nil).Once()
			},

			wantDialCalls: 2,
			wantErr:       nil,
		},
		"ok - connection refused scale to zero enabled and hibernated cluster, reactivates cluster with only primary": {
			dialer: &mockDialer{
				dialFn: func(ctx context.Context, i uint, network, address string) (net.Conn, error) {
					switch i {
					case 1:
						return nil, syscall.ECONNREFUSED
					case 2:
						return &net.TCPConn{}, nil
					default:
						return nil, errors.New("unexpected dial call")
					}
				},
			},
			setupMocks: func(mockClusters *protomocks.ClustersServiceClient) {
				mockClusters.EXPECT().DescribePostgresCluster(ctx, &clustersv1.DescribePostgresClusterRequest{
					Id: "test-branch",
				}).Return(&clustersv1.DescribePostgresClusterResponse{
					Status: &clustersv1.ClusterStatus{
						StatusType: clustersv1.ClusterStatus_STATUS_TYPE_HIBERNATED,
					},
					Configuration: &clustersv1.ClusterConfiguration{
						ScaleToZero: &clustersv1.ScaleToZero{Enabled: true},
					},
				}, nil).Once()

				mockClusters.EXPECT().UpdatePostgresCluster(ctx, &clustersv1.UpdatePostgresClusterRequest{
					Id: "test-branch",
					UpdateConfiguration: &clustersv1.UpdateClusterConfiguration{
						Hibernate: new(false),
					},
				}).Return(&clustersv1.UpdatePostgresClusterResponse{}, nil).Once()

				mockClusters.EXPECT().DescribePostgresCluster(ctx, &clustersv1.DescribePostgresClusterRequest{
					Id: "test-branch",
				}).Return(&clustersv1.DescribePostgresClusterResponse{
					Status: &clustersv1.ClusterStatus{
						StatusType:         clustersv1.ClusterStatus_STATUS_TYPE_TRANSIENT,
						InstanceCount:      2,
						InstanceReadyCount: 1,
						Instances: map[string]*clustersv1.InstanceStatus{
							"instance-1": {
								Primary: true,
								Status:  apiv1.PodHealthy,
							},
							"instance-2": {
								Primary: false,
								Status:  apiv1.PodFailed,
							},
						},
					},
				}, nil).Once()
			},

			wantDialCalls: 2,
			wantErr:       nil,
		},
		"ok - connection refused scale to zero enabled and hibernated cluster, reactivates cluster waiting for instances to be ready": {
			dialer: &mockDialer{
				dialFn: func(ctx context.Context, i uint, network, address string) (net.Conn, error) {
					switch i {
					case 1:
						return nil, syscall.ECONNREFUSED
					case 2:
						return &net.TCPConn{}, nil
					default:
						return nil, errors.New("unexpected dial call")
					}
				},
			},
			setupMocks: func(mockClusters *protomocks.ClustersServiceClient) {
				mockClusters.EXPECT().DescribePostgresCluster(ctx, &clustersv1.DescribePostgresClusterRequest{
					Id: "test-branch",
				}).Return(&clustersv1.DescribePostgresClusterResponse{
					Status: &clustersv1.ClusterStatus{
						StatusType: clustersv1.ClusterStatus_STATUS_TYPE_HIBERNATED,
					},
					Configuration: &clustersv1.ClusterConfiguration{
						ScaleToZero: &clustersv1.ScaleToZero{Enabled: true},
					},
				}, nil).Once()

				mockClusters.EXPECT().UpdatePostgresCluster(ctx, &clustersv1.UpdatePostgresClusterRequest{
					Id: "test-branch",
					UpdateConfiguration: &clustersv1.UpdateClusterConfiguration{
						Hibernate: new(false),
					},
				}).Return(&clustersv1.UpdatePostgresClusterResponse{}, nil).Once()

				mockClusters.EXPECT().DescribePostgresCluster(ctx, &clustersv1.DescribePostgresClusterRequest{
					Id: "test-branch",
				}).Return(&clustersv1.DescribePostgresClusterResponse{
					Status: &clustersv1.ClusterStatus{
						StatusType:         clustersv1.ClusterStatus_STATUS_TYPE_HEALTHY,
						InstanceCount:      1,
						InstanceReadyCount: 0,
					},
				}, nil).Once()
				mockClusters.EXPECT().DescribePostgresCluster(ctx, &clustersv1.DescribePostgresClusterRequest{
					Id: "test-branch",
				}).Return(&clustersv1.DescribePostgresClusterResponse{
					Status: &clustersv1.ClusterStatus{
						StatusType:         clustersv1.ClusterStatus_STATUS_TYPE_HEALTHY,
						InstanceCount:      1,
						InstanceReadyCount: 1,
					},
				}, nil).Once()
			},

			wantDialCalls: 2,
			wantErr:       nil,
		},
		"error - unable to connect to clusters service, returns dial error": {
			clustersService: clustersServiceClientFn(func(ctx context.Context, branchID string) (clustersServiceClient, error) {
				return nil, errors.New("some error")
			}),
			dialer: &mockDialer{
				dialFn: func(ctx context.Context, i uint, network, address string) (net.Conn, error) {
					switch i {
					case 1:
						return nil, syscall.ECONNREFUSED
					default:
						return nil, errors.New("unexpected dial call")
					}
				},
			},
			setupMocks: func(mockClusters *protomocks.ClustersServiceClient) {},

			wantDialCalls: 1,
			wantErr:       syscall.ECONNREFUSED,
		},
		"ok - connection refused with scale to zero disabled, cluster healthy, waits then connects": {
			dialer: &mockDialer{
				dialFn: func(ctx context.Context, i uint, network, address string) (net.Conn, error) {
					switch i {
					case 1:
						return nil, syscall.ECONNREFUSED
					default:
						return &net.TCPConn{}, nil
					}
				},
			},
			setupMocks: func(mockClusters *protomocks.ClustersServiceClient) {
				mockClusters.EXPECT().DescribePostgresCluster(ctx, &clustersv1.DescribePostgresClusterRequest{
					Id: "test-branch",
				}).Return(&clustersv1.DescribePostgresClusterResponse{
					Status: &clustersv1.ClusterStatus{
						StatusType:         clustersv1.ClusterStatus_STATUS_TYPE_HEALTHY,
						InstanceCount:      1,
						InstanceReadyCount: 1,
					},
					Configuration: &clustersv1.ClusterConfiguration{
						ScaleToZero: &clustersv1.ScaleToZero{Enabled: false},
					},
				}, nil)
			},

			wantDialCalls: 2,
			wantErr:       nil,
		},
		"error - manually hibernated cluster returns ErrBranchHibernated": {
			dialer: &mockDialer{
				dialFn: func(ctx context.Context, i uint, network, address string) (net.Conn, error) {
					switch i {
					case 1:
						return nil, syscall.ECONNREFUSED
					default:
						return nil, errors.New("unexpected dial call")
					}
				},
			},
			setupMocks: func(mockClusters *protomocks.ClustersServiceClient) {
				mockClusters.EXPECT().DescribePostgresCluster(ctx, &clustersv1.DescribePostgresClusterRequest{
					Id: "test-branch",
				}).Return(&clustersv1.DescribePostgresClusterResponse{
					Status: &clustersv1.ClusterStatus{
						StatusType: clustersv1.ClusterStatus_STATUS_TYPE_HIBERNATED,
					},
					Configuration: &clustersv1.ClusterConfiguration{
						ScaleToZero: &clustersv1.ScaleToZero{Enabled: false},
					},
				}, nil).Once()
			},

			wantDialCalls: 1,
			wantErr:       ErrBranchHibernated,
		},
		// A concurrent request already reactivated the cluster (so it reports
		// HEALTHY, not HIBERNATED) but the dial target — typically the pooler
		// Service — isn't routable yet. The connection must be held and the
		// target re-probed until it succeeds, rather than failing fast.
		"ok - scale to zero enabled, cluster healthy but target briefly unreachable, waits then connects": {
			dialer: &mockDialer{
				dialFn: func(ctx context.Context, i uint, network, address string) (net.Conn, error) {
					switch i {
					case 1:
						return nil, syscall.ECONNREFUSED
					default:
						return &net.TCPConn{}, nil
					}
				},
			},
			setupMocks: func(mockClusters *protomocks.ClustersServiceClient) {
				mockClusters.EXPECT().DescribePostgresCluster(ctx, &clustersv1.DescribePostgresClusterRequest{
					Id: "test-branch",
				}).Return(&clustersv1.DescribePostgresClusterResponse{
					Status: &clustersv1.ClusterStatus{
						StatusType:         clustersv1.ClusterStatus_STATUS_TYPE_HEALTHY,
						InstanceCount:      1,
						InstanceReadyCount: 1,
					},
					Configuration: &clustersv1.ClusterConfiguration{
						ScaleToZero: &clustersv1.ScaleToZero{Enabled: true},
					},
				}, nil)
			},

			wantDialCalls: 2,
			wantErr:       nil,
		},
		// Same race as above, but the target never recovers: the dialer keeps
		// re-probing until reactivateTimeout, then surfaces the dial error.
		"error - scale to zero enabled, cluster healthy but target stays unreachable, returns dial error": {
			dialer: &mockDialer{
				dialFn: func(ctx context.Context, _ uint, network, address string) (net.Conn, error) {
					return nil, syscall.ECONNREFUSED
				},
			},
			setupMocks: func(mockClusters *protomocks.ClustersServiceClient) {
				mockClusters.EXPECT().DescribePostgresCluster(ctx, &clustersv1.DescribePostgresClusterRequest{
					Id: "test-branch",
				}).Return(&clustersv1.DescribePostgresClusterResponse{
					Status: &clustersv1.ClusterStatus{
						StatusType:         clustersv1.ClusterStatus_STATUS_TYPE_HEALTHY,
						InstanceCount:      1,
						InstanceReadyCount: 1,
					},
					Configuration: &clustersv1.ClusterConfiguration{
						ScaleToZero: &clustersv1.ScaleToZero{Enabled: true},
					},
				}, nil)
			},

			wantMinDialCalls: 2,
			wantErr:          syscall.ECONNREFUSED,
		},
		// A branch on a wakeup pool that has not been assigned a cluster yet.
		// The clusters service has no Cluster resource to derive a status from,
		// so it synthesizes one: the healthy phase with a Transient status type
		// and no instances. A freshly created branch connects in exactly this
		// window, so the connection has to be held until the assigned cluster
		// comes up. The phase is not one of startingFromZeroPhases, so the
		// no-instance-reported check is what keeps this waiting.
		"ok - branch awaiting a wakeup pool assignment, waits then connects": {
			dialer: &mockDialer{
				dialFn: func(ctx context.Context, i uint, network, address string) (net.Conn, error) {
					switch i {
					case 1:
						return nil, syscall.ECONNREFUSED
					default:
						return &net.TCPConn{}, nil
					}
				},
			},
			setupMocks: func(mockClusters *protomocks.ClustersServiceClient) {
				mockClusters.EXPECT().DescribePostgresCluster(ctx, &clustersv1.DescribePostgresClusterRequest{
					Id: "test-branch",
				}).Return(&clustersv1.DescribePostgresClusterResponse{
					Status: &clustersv1.ClusterStatus{
						Status:     apiv1.PhaseHealthy,
						StatusType: clustersv1.ClusterStatus_STATUS_TYPE_TRANSIENT,
					},
					Configuration: &clustersv1.ClusterConfiguration{
						ScaleToZero: &clustersv1.ScaleToZero{Enabled: true},
					},
				}, nil).Once()

				mockClusters.EXPECT().DescribePostgresCluster(ctx, &clustersv1.DescribePostgresClusterRequest{
					Id: "test-branch",
				}).Return(&clustersv1.DescribePostgresClusterResponse{
					Status: &clustersv1.ClusterStatus{
						Status:             apiv1.PhaseHealthy,
						StatusType:         clustersv1.ClusterStatus_STATUS_TYPE_HEALTHY,
						InstanceCount:      1,
						InstanceReadyCount: 1,
					},
				}, nil).Once()
			},

			wantDialCalls: 2,
			wantErr:       nil,
		},
		// A Cluster that exists but whose status the operator has not populated
		// yet: the clusters service reports the unknown phase with a Transient
		// status type. Nothing is running and nothing can be crash-looping, so
		// the connection is held.
		"ok - cluster status not populated yet, waits then connects": {
			dialer: &mockDialer{
				dialFn: func(ctx context.Context, i uint, network, address string) (net.Conn, error) {
					switch i {
					case 1:
						return nil, syscall.ECONNREFUSED
					default:
						return &net.TCPConn{}, nil
					}
				},
			},
			setupMocks: func(mockClusters *protomocks.ClustersServiceClient) {
				mockClusters.EXPECT().DescribePostgresCluster(ctx, &clustersv1.DescribePostgresClusterRequest{
					Id: "test-branch",
				}).Return(&clustersv1.DescribePostgresClusterResponse{
					Status: &clustersv1.ClusterStatus{
						Status:     "unknown",
						StatusType: clustersv1.ClusterStatus_STATUS_TYPE_TRANSIENT,
					},
					Configuration: &clustersv1.ClusterConfiguration{},
				}, nil).Once()

				mockClusters.EXPECT().DescribePostgresCluster(ctx, &clustersv1.DescribePostgresClusterRequest{
					Id: "test-branch",
				}).Return(&clustersv1.DescribePostgresClusterResponse{
					Status: &clustersv1.ClusterStatus{
						Status:             apiv1.PhaseHealthy,
						StatusType:         clustersv1.ClusterStatus_STATUS_TYPE_HEALTHY,
						InstanceCount:      1,
						InstanceReadyCount: 1,
					},
				}, nil).Once()
			},

			wantDialCalls: 2,
			wantErr:       nil,
		},
		// StatusType is derived from the Cluster resource and lags the instances,
		// so it can still read Healthy while the primary is crashed or in
		// recovery and nothing can accept connections. There is nothing to wait
		// for, so the dial error is surfaced immediately rather than holding the
		// client connection for the full reactivate timeout. The single dial
		// call is the assertion that matters: it proves waitUntilReachable was
		// never entered, so the clusters service is not polled either.
		"error - cluster reports healthy but no instance is ready, fails fast without holding": {
			dialer: &mockDialer{
				dialFn: func(ctx context.Context, _ uint, network, address string) (net.Conn, error) {
					return nil, syscall.ECONNREFUSED
				},
			},
			setupMocks: func(mockClusters *protomocks.ClustersServiceClient) {
				mockClusters.EXPECT().DescribePostgresCluster(ctx, &clustersv1.DescribePostgresClusterRequest{
					Id: "test-branch",
				}).Return(&clustersv1.DescribePostgresClusterResponse{
					Status: &clustersv1.ClusterStatus{
						StatusType:         clustersv1.ClusterStatus_STATUS_TYPE_HEALTHY,
						InstanceCount:      1,
						InstanceReadyCount: 0,
					},
					Configuration: &clustersv1.ClusterConfiguration{},
				}, nil)
			},

			wantDialCalls: 1,
			wantErr:       syscall.ECONNREFUSED,
		},
		// Same fail-fast path, reached with a Transient cluster whose primary is
		// restarting in place and reported unhealthy. This is the shape seen
		// when a primary is being OOM-killed repeatedly. A restart phase is not
		// distinguishable from a crash loop, so it is not held for: an in-place
		// restart of a running cluster is excluded from startingFromZeroPhases.
		"error - cluster restarting primary in place, fails fast without holding": {
			dialer: &mockDialer{
				dialFn: func(ctx context.Context, _ uint, network, address string) (net.Conn, error) {
					return nil, syscall.ECONNREFUSED
				},
			},
			setupMocks: func(mockClusters *protomocks.ClustersServiceClient) {
				mockClusters.EXPECT().DescribePostgresCluster(ctx, &clustersv1.DescribePostgresClusterRequest{
					Id: "test-branch",
				}).Return(&clustersv1.DescribePostgresClusterResponse{
					Status: &clustersv1.ClusterStatus{
						Status:             apiv1.PhaseInplacePrimaryRestart,
						StatusType:         clustersv1.ClusterStatus_STATUS_TYPE_TRANSIENT,
						InstanceCount:      1,
						InstanceReadyCount: 0,
						Instances: map[string]*clustersv1.InstanceStatus{
							"test-branch-1": {
								Primary: true,
								Status:  apiv1.PodFailed,
							},
						},
					},
					Configuration: &clustersv1.ClusterConfiguration{},
				}, nil)
			},

			wantDialCalls: 1,
			wantErr:       syscall.ECONNREFUSED,
		},
		// Simulates a hibernated cluster that reactivates successfully (Postgres
		// instances come back) but the dial target (e.g. the pooler Service)
		// stays unreachable. waitUntilReachable retries the probe-dial until
		// reactivateTimeout, then surfaces the original dial error.
		"error - hibernated cluster reactivates but target stays unreachable, returns dial error": {
			dialer: &mockDialer{
				dialFn: func(ctx context.Context, _ uint, network, address string) (net.Conn, error) {
					return nil, syscall.ECONNREFUSED
				},
			},
			setupMocks: func(mockClusters *protomocks.ClustersServiceClient) {
				mockClusters.EXPECT().DescribePostgresCluster(ctx, &clustersv1.DescribePostgresClusterRequest{
					Id: "test-branch",
				}).Return(&clustersv1.DescribePostgresClusterResponse{
					Status: &clustersv1.ClusterStatus{
						StatusType: clustersv1.ClusterStatus_STATUS_TYPE_HIBERNATED,
					},
					Configuration: &clustersv1.ClusterConfiguration{
						ScaleToZero: &clustersv1.ScaleToZero{Enabled: true},
					},
				}, nil).Once()

				mockClusters.EXPECT().UpdatePostgresCluster(ctx, &clustersv1.UpdatePostgresClusterRequest{
					Id: "test-branch",
					UpdateConfiguration: &clustersv1.UpdateClusterConfiguration{
						Hibernate: new(false),
					},
				}).Return(&clustersv1.UpdatePostgresClusterResponse{}, nil).Once()

				mockClusters.EXPECT().DescribePostgresCluster(ctx, &clustersv1.DescribePostgresClusterRequest{
					Id: "test-branch",
				}).Return(&clustersv1.DescribePostgresClusterResponse{
					Status: &clustersv1.ClusterStatus{
						StatusType:         clustersv1.ClusterStatus_STATUS_TYPE_HEALTHY,
						InstanceCount:      1,
						InstanceReadyCount: 1,
					},
				}, nil).Once()
			},

			wantMinDialCalls: 2,
			wantErr:          syscall.ECONNREFUSED,
		},
		"error - connection refused, error describing cluster, returns dial error": {
			dialer: &mockDialer{
				dialFn: func(ctx context.Context, i uint, network, address string) (net.Conn, error) {
					switch i {
					case 1:
						return nil, syscall.ECONNREFUSED
					default:
						return nil, errors.New("unexpected dial call")
					}
				},
			},
			setupMocks: func(mockClusters *protomocks.ClustersServiceClient) {
				mockClusters.EXPECT().DescribePostgresCluster(ctx, &clustersv1.DescribePostgresClusterRequest{
					Id: "test-branch",
				}).Return(nil, errTest).Once()
			},

			wantDialCalls: 1,
			wantErr:       syscall.ECONNREFUSED,
		},
		"error - connection refused, error updating cluster, returns dial error": {
			dialer: &mockDialer{
				dialFn: func(ctx context.Context, i uint, network, address string) (net.Conn, error) {
					switch i {
					case 1:
						return nil, syscall.ECONNREFUSED
					default:
						return nil, errors.New("unexpected dial call")
					}
				},
			},
			setupMocks: func(mockClusters *protomocks.ClustersServiceClient) {
				mockClusters.EXPECT().DescribePostgresCluster(ctx, &clustersv1.DescribePostgresClusterRequest{
					Id: "test-branch",
				}).Return(&clustersv1.DescribePostgresClusterResponse{
					Status: &clustersv1.ClusterStatus{
						StatusType: clustersv1.ClusterStatus_STATUS_TYPE_HIBERNATED,
					},
					Configuration: &clustersv1.ClusterConfiguration{
						ScaleToZero: &clustersv1.ScaleToZero{Enabled: true},
					},
				}, nil).Once()

				mockClusters.EXPECT().UpdatePostgresCluster(ctx, &clustersv1.UpdatePostgresClusterRequest{
					Id: "test-branch",
					UpdateConfiguration: &clustersv1.UpdateClusterConfiguration{
						Hibernate: new(false),
					},
				}).Return(nil, errTest).Once()
			},

			wantDialCalls: 1,
			wantErr:       syscall.ECONNREFUSED,
		},
		"error - connection refused, error describing cluster after update, returns dial error": {
			dialer: &mockDialer{
				dialFn: func(ctx context.Context, i uint, network, address string) (net.Conn, error) {
					switch i {
					case 1:
						return nil, syscall.ECONNREFUSED
					default:
						return nil, errors.New("unexpected dial call")
					}
				},
			},
			setupMocks: func(mockClusters *protomocks.ClustersServiceClient) {
				mockClusters.EXPECT().DescribePostgresCluster(ctx, &clustersv1.DescribePostgresClusterRequest{
					Id: "test-branch",
				}).Return(&clustersv1.DescribePostgresClusterResponse{
					Status: &clustersv1.ClusterStatus{
						StatusType: clustersv1.ClusterStatus_STATUS_TYPE_HIBERNATED,
					},
					Configuration: &clustersv1.ClusterConfiguration{
						ScaleToZero: &clustersv1.ScaleToZero{Enabled: true},
					},
				}, nil).Once()

				mockClusters.EXPECT().UpdatePostgresCluster(ctx, &clustersv1.UpdatePostgresClusterRequest{
					Id: "test-branch",
					UpdateConfiguration: &clustersv1.UpdateClusterConfiguration{
						Hibernate: new(false),
					},
				}).Return(&clustersv1.UpdatePostgresClusterResponse{}, nil).Once()

				mockClusters.EXPECT().DescribePostgresCluster(ctx, &clustersv1.DescribePostgresClusterRequest{
					Id: "test-branch",
				}).Return(nil, errTest).Once()
			},

			wantDialCalls: 1,
			wantErr:       syscall.ECONNREFUSED,
		},
		"error - connection refused with hibernated cluster and scale to zero enabled, timeout on reactivation returns dial error": {
			dialer: &mockDialer{
				dialFn: func(ctx context.Context, i uint, network, address string) (net.Conn, error) {
					switch i {
					case 1:
						return nil, syscall.ECONNREFUSED
					default:
						return nil, errors.New("unexpected dial call")
					}
				},
			},
			setupMocks: func(mockClusters *protomocks.ClustersServiceClient) {
				mockClusters.EXPECT().DescribePostgresCluster(ctx, &clustersv1.DescribePostgresClusterRequest{
					Id: "test-branch",
				}).Return(&clustersv1.DescribePostgresClusterResponse{
					Status: &clustersv1.ClusterStatus{
						StatusType: clustersv1.ClusterStatus_STATUS_TYPE_HIBERNATED,
					},
					Configuration: &clustersv1.ClusterConfiguration{
						ScaleToZero: &clustersv1.ScaleToZero{Enabled: true},
					},
				}, nil)

				mockClusters.EXPECT().UpdatePostgresCluster(ctx, &clustersv1.UpdatePostgresClusterRequest{
					Id: "test-branch",
					UpdateConfiguration: &clustersv1.UpdateClusterConfiguration{
						Hibernate: new(false),
					},
				}).Return(&clustersv1.UpdatePostgresClusterResponse{}, nil).Once()
			},

			wantDialCalls: 1,
			wantErr:       syscall.ECONNREFUSED,
		},
		"error - other dial error": {
			dialer: &mockDialer{
				dialFn: func(ctx context.Context, _ uint, network, address string) (net.Conn, error) {
					return nil, errTest
				},
			},
			setupMocks: func(mockClusters *protomocks.ClustersServiceClient) {
			},

			wantDialCalls: 1,
			wantErr:       errTest,
		},
		"error - temporary DNS error does not trigger reactivation": {
			dialer: &mockDialer{
				dialFn: func(ctx context.Context, _ uint, network, address string) (net.Conn, error) {
					return nil, errDNS
				},
			},
			setupMocks: func(mockClusters *protomocks.ClustersServiceClient) {},

			wantDialCalls: 1,
			wantErr:       errDNS,
		},
		"error - transient clusters service unavailable returns dial error": {
			dialer: &mockDialer{
				dialFn: func(ctx context.Context, _ uint, network, address string) (net.Conn, error) {
					return nil, syscall.ECONNREFUSED
				},
			},
			setupMocks: func(mockClusters *protomocks.ClustersServiceClient) {
				mockClusters.EXPECT().DescribePostgresCluster(ctx, &clustersv1.DescribePostgresClusterRequest{
					Id: "test-branch",
				}).Return(nil, status.Error(codes.Unavailable, "connection refused")).Once()
			},

			wantDialCalls: 1,
			wantErr:       syscall.ECONNREFUSED,
		},
		// Real grpc client so upstream changes to "produced zero addresses" break the match in Dial.
		"error - real grpc client resolves to zero addresses returns branch-not-found": {
			dialer: &mockDialer{
				dialFn: func(_ context.Context, _ uint, _, address string) (net.Conn, error) {
					return nil, &net.DNSError{Err: "no such host", Name: address, IsNotFound: true}
				},
			},
			setupMocks:      func(mockClusters *protomocks.ClustersServiceClient) {},
			clustersService: zeroAddressClustersService(),
			wantDialCalls:   1,
			wantErr:         ErrBranchNotFound,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			mockClusters := protomocks.NewClustersServiceClient(t)
			tc.setupMocks(mockClusters)

			if tc.clustersService == nil {
				tc.clustersService = clustersServiceClientFn(func(ctx context.Context, branchID string) (clustersServiceClient, error) {
					return &mockClustersServiceClient{mockClusters}, nil
				})
			}

			d := NewClusterDialer(ClusterDialerConfiguration{
				ReactivateTimeout:   time.Second,
				StatusCheckInterval: time.Millisecond * 100,
			}, WithClustersService(tc.clustersService), WithDialer(tc.dialer.Dial))

			_, err := d.Dial(ctx, "tcp", &Branch{
				ID:      "test-branch",
				Address: "test-branch-address",
			})
			require.ErrorIs(t, err, tc.wantErr)
			if tc.wantMinDialCalls > 0 {
				require.GreaterOrEqual(t, tc.dialer.DialCalls(), tc.wantMinDialCalls, "unexpected number of dial calls")
			} else {
				require.Equal(t, tc.wantDialCalls, tc.dialer.DialCalls(), "unexpected number of dial calls")
			}

			mockClusters.AssertExpectations(t)
		})
	}
}

type mockDialer struct {
	dialCalls uint
	dialFn    func(ctx context.Context, i uint, network, address string) (net.Conn, error)
}

func (m *mockDialer) Dial(ctx context.Context, network, address string) (net.Conn, error) {
	m.dialCalls++
	return m.dialFn(ctx, m.dialCalls, network, address)
}

func (m *mockDialer) DialCalls() uint {
	return m.dialCalls
}

type mockClustersServiceClient struct {
	*protomocks.ClustersServiceClient
}

func (m *mockClustersServiceClient) Close() error {
	return nil
}

func zeroAddressClustersService() clustersServiceClientFn {
	mr := manual.NewBuilderWithScheme("test-zero-addr")
	resolver.Register(mr)
	mr.InitialState(resolver.State{})
	return func(ctx context.Context, branchID string) (clustersServiceClient, error) {
		conn, err := grpc.NewClient(mr.Scheme()+":///"+branchID,
			grpc.WithTransportCredentials(insecure.NewCredentials()),
			grpc.WithDefaultServiceConfig(`{}`),
		)
		if err != nil {
			return nil, err
		}
		return &realClustersClient{ClustersServiceClient: clustersv1.NewClustersServiceClient(conn), conn: conn}, nil
	}
}

type realClustersClient struct {
	clustersv1.ClustersServiceClient
	conn *grpc.ClientConn
}

func (c *realClustersClient) Close() error { return c.conn.Close() }

func TestNewNetDialer(t *testing.T) {
	tests := map[string]struct {
		userTimeout time.Duration
	}{
		"system default": {
			userTimeout: 0,
		},
		"with user timeout": {
			userTimeout: 30 * time.Second,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			require.NoError(t, err)
			defer listener.Close()

			accepted := make(chan struct{})
			go func() {
				defer close(accepted)
				conn, err := listener.Accept()
				if err == nil {
					conn.Close()
				}
			}()

			// The Control hook runs during dial, so a successful connection
			// means the socket option was accepted by the kernel. On non-Linux
			// platforms setTCPUserTimeout is a no-op and this just checks the
			// dialer is still usable.
			conn, err := newNetDialer(test.userTimeout)(t.Context(), "tcp", listener.Addr().String())
			require.NoError(t, err)
			require.NoError(t, conn.Close())
			<-accepted
		})
	}
}
