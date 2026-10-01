package wakeup

import (
	"context"
	"fmt"

	v1 "k8s.io/api/core/v1"

	slotv1 "xata/gen/proto/slots/v1"
	internalgrpc "xata/internal/grpc"
	"xata/internal/o11y"
)

// wakeUp dials the SlotController gRPC service on the given CSI node pod and
// calls the WakeUp RPC to connect the NVMe volume and mount it on the slot
// path.
func (r *WakeupReconciler) wakeUp(ctx context.Context, csiNodePod *v1.Pod, xvolName, pvName string) error {
	// Build the address of the SlotController service on the CSI node pod
	addr := fmt.Sprintf("%s:%d", csiNodePod.Status.PodIP, r.CSINodePort)

	// Connect to the SlotController service on the CSI node pod
	conn, err := internalgrpc.NewClient(o11y.Ctx(ctx), addr)
	if err != nil {
		return fmt.Errorf("dial CSI node pod %q at %s: %w", csiNodePod.Name, addr, err)
	}
	defer conn.Close()

	// Set a timeout for the WakeUp RPC
	ctx, cancel := context.WithTimeout(ctx, r.WakeupRPCTimeout)
	defer cancel()

	// Call the WakeUp RPC
	client := slotv1.NewSlotControllerClient(conn)
	_, err = client.WakeUp(ctx, &slotv1.WakeUpRequest{
		XvolId: xvolName,
		PvId:   pvName,
	})
	if err != nil {
		return fmt.Errorf("failed WakeUp RPC on pod %q: %w", csiNodePod.Name, err)
	}

	return nil
}
