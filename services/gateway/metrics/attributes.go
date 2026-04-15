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
)

const (
	ProtocolWire      = "wire"
	ProtocolWebSocket = "websocket"
	ProtocolHTTP      = "http"
)
