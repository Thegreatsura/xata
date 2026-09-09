package session

import (
	"context"
	"net"
	"syscall"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	clustersv1 "xata/gen/proto/clusters/v1"
)

func TestStatusPollInterval(t *testing.T) {
	for _, tc := range []struct{ elapsed, initial, want time.Duration }{
		{0, 100 * time.Millisecond, 100 * time.Millisecond},
		{5*time.Second - time.Nanosecond, 100 * time.Millisecond, 100 * time.Millisecond},
		{5 * time.Second, 100 * time.Millisecond, 200 * time.Millisecond},
		{10 * time.Second, 100 * time.Millisecond, 400 * time.Millisecond},
		{15 * time.Second, 100 * time.Millisecond, 800 * time.Millisecond},
		{20 * time.Second, 100 * time.Millisecond, time.Second},
		{time.Hour, 100 * time.Millisecond, time.Second},
		{time.Hour, 2 * time.Second, 2 * time.Second},
	} {
		require.Equal(t, tc.want, statusPollInterval(tc.initial, tc.elapsed))
	}
}

type pollingStatusService struct {
	clustersService
	describe func() (*clustersv1.DescribePostgresClusterResponse, error)
}

func (s pollingStatusService) DescribePostgresCluster(context.Context, *clustersv1.DescribePostgresClusterRequest, ...grpc.CallOption) (*clustersv1.DescribePostgresClusterResponse, error) {
	return s.describe()
}

func TestStatusPollingBackoffAndTCPReadiness(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		start := time.Now()
		var polls, dials []time.Duration
		svc := pollingStatusService{describe: func() (*clustersv1.DescribePostgresClusterResponse, error) {
			polls = append(polls, time.Since(start))
			state := clustersv1.ClusterStatus_STATUS_TYPE_TRANSIENT
			if time.Since(start) >= 6*time.Second {
				state = clustersv1.ClusterStatus_STATUS_TYPE_HEALTHY
			}
			return &clustersv1.DescribePostgresClusterResponse{Status: &clustersv1.ClusterStatus{StatusType: state}}, nil
		}}
		d := &ClusterDialer{statusCheckInterval: 100 * time.Millisecond, reactivateTimeout: 50 * time.Second, dialer: func(context.Context, string, string) (net.Conn, error) {
			dials = append(dials, time.Since(start))
			if len(dials) == 1 {
				time.Sleep(80 * time.Millisecond)
				return nil, syscall.ECONNREFUSED
			}
			return &net.TCPConn{}, nil
		}}
		conn, err := d.waitUntilReachable(context.Background(), svc, "a", "tcp", "example:5432")
		require.NoError(t, err)
		require.NotNil(t, conn)
		require.Len(t, polls, 55)
		for i, got := range polls[:50] {
			require.Equal(t, time.Duration(i+1)*100*time.Millisecond, got)
		}
		require.Equal(t, []time.Duration{5200 * time.Millisecond, 5400 * time.Millisecond, 5600 * time.Millisecond, 5800 * time.Millisecond, 6 * time.Second}, polls[50:])
		require.Equal(t, []time.Duration{6 * time.Second, 6100 * time.Millisecond}, dials, "TCP retries retain their ticker cadence after readiness")
	})
}

func TestStatusPollingBackoffCancellation(t *testing.T) {
	for _, cancelWait := range []bool{false, true} {
		synctest.Test(t, func(t *testing.T) {
			start := time.Now()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			calls := 0
			svc := pollingStatusService{describe: func() (*clustersv1.DescribePostgresClusterResponse, error) {
				calls++
				return &clustersv1.DescribePostgresClusterResponse{Status: &clustersv1.ClusterStatus{StatusType: clustersv1.ClusterStatus_STATUS_TYPE_TRANSIENT}}, nil
			}}
			d := &ClusterDialer{statusCheckInterval: 100 * time.Millisecond, reactivateTimeout: 6250 * time.Millisecond}
			if cancelWait {
				d.reactivateTimeout = 50 * time.Second
				go func() { time.Sleep(6250 * time.Millisecond); cancel() }()
			}
			_, err := d.waitUntilReachable(ctx, svc, "a", "tcp", "example:5432")
			if cancelWait {
				require.ErrorIs(t, err, context.Canceled)
			} else {
				require.ErrorContains(t, err, "timed out waiting for cluster")
			}
			require.Equal(t, 6250*time.Millisecond, time.Since(start))
			require.Equal(t, 56, calls)
		})
	}
}
