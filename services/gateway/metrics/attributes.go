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
