package zoraxy_plugin

// RequestInspectProtocolVersion is the version of RequestInspectPayload sent by Zoraxy.
const RequestInspectProtocolVersion = 1

// RequestInspectPayload extends the existing sniff metadata. JSON encodes Body as base64.
type RequestInspectPayload struct {
	DynamicSniffForwardRequest
	ProtocolVersion int    `json:"protocol_version"`
	RouteID         string `json:"route_id"` //RootOrMatchingDomain of the matched proxy rule
	Body            []byte `json:"body"`
	BodyTruncated   bool   `json:"body_truncated"`
}

// RequestInspectResult is a decision, not a capture or forwarding instruction.
type RequestInspectResult struct {
	Action  string `json:"action"`
	Status  int    `json:"status,omitempty"`
	Reason  string `json:"reason,omitempty"`   //Public reason shown to the denied client
	EventID string `json:"event_id,omitempty"` //Plugin event reference, logged by Zoraxy
}

// RequestInspectPolicy is the control-plane snapshot served by the optional
// IntroSpect.RequestInspectPolicy endpoint. Missing or zero values use the
// defaults: mode report, fail open, 64 KiB body prefix, 5000 ms body timeout.
type RequestInspectPolicy struct {
	Mode               string                              `json:"mode"`                 //off, report or block
	FailClosed         bool                                `json:"fail_closed"`          //Block mode returns 503 on inspection errors
	InvertFailureHosts []string                            `json:"invert_failure_hosts"` //Exact hosts or route IDs using the opposite of FailClosed
	MaxBodyKB          int                                 `json:"max_body_kb"`          //1-1024
	BodyTimeoutMS      int                                 `json:"body_timeout_ms"`      //100-30000
	Hosts              map[string]RequestInspectHostPolicy `json:"hosts"`                //Keyed by lowercase host or route ID; route ID wins
}

// RequestInspectHostPolicy overrides the policy for one host or route ID.
type RequestInspectHostPolicy struct {
	Mode          string `json:"mode"` //inherit, off, report or block
	MaxBodyKB     int    `json:"max_body_kb,omitempty"`
	BodyTimeoutMS int    `json:"body_timeout_ms,omitempty"`
}
