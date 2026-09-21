package metrics

import "go.opentelemetry.io/otel/attribute"

const (
	AttrProtocol  = attribute.Key("protocol")
	AttrBranchID  = attribute.Key("branch_id")
	AttrHost      = attribute.Key("host")
	AttrAddress   = attribute.Key("address")
	AttrDatabase  = attribute.Key("database")
	AttrBatch     = attribute.Key("batch")
	AttrSuccess   = attribute.Key("success")
	AttrErrorType = attribute.Key("error_type")
	AttrDirection = attribute.Key("direction")

	AttrPool = attribute.Key("pool")
	// AttrInstanceSize is the vCPU request and memory of the cluster being
	// reactivated, formatted by the caller as "<vcpu>/<memory>GB" (e.g.
	// "500m/1GB", "2/8GB"). Instance sizes come from a small catalog, so the
	// cardinality stays bounded.
	AttrInstanceSize = attribute.Key("instance_size")
)

// Copy directions for xata.gateway.bytes_forwarded. These are distinct from
// the human-readable "direction" value on session logs, which is unchanged.
const (
	DirectionClientToBackend = "client_to_backend"
	DirectionBackendToClient = "backend_to_client"
)

const (
	ProtocolWire      = "wire"
	ProtocolWebSocket = "websocket"
	ProtocolHTTP      = "http"
)

// Error types for a failed cluster wait, recorded as AttrErrorType.
const (
	WaitErrorTimeout  = "timeout"
	WaitErrorCanceled = "canceled"
	WaitErrorRPC      = "rpc"
	WaitErrorDial     = "dial"
)
