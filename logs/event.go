package logs

import (
	"time"
)

// AccessEvent - clean, simple event structure
type AccessEvent struct {
	// Core event info
	Timestamp  time.Time `json:"ts"`
	EventType  string    `json:"event_type"`
	Outcome    string    `json:"outcome"`
	Reason     string    `json:"reason"`
	StatusCode int       `json:"status_code"`

	// Device identifier
	DeviceID string `json:"device_id"`

	// Request info
	Request RequestDetails `json:"request"`
	Client  ClientInfo     `json:"client"`

	// Policy info
	Policy PolicyInfo `json:"policy"`

	// Internal debug info (hidden in UI)
	Internal *InternalInfo `json:"internal,omitempty"`
}

type RequestDetails struct {
	Method string `json:"method"` // From X-Forwarded-Method
	Host   string `json:"host"`   // From X-Forwarded-Host
	Path   string `json:"path"`   // From X-Forwarded-Uri
	Scheme string `json:"scheme"` // From X-Forwarded-Proto
}

type ClientInfo struct {
	IP        string `json:"ip"`
	UserAgent string `json:"user_agent,omitempty"`
}

type PolicyInfo struct {
	Mode string `json:"mode"` // "allowlist" or "blocklist"
}

type InternalInfo struct {
	ProxyPath   string            `json:"proxy_path"` // The /auth path
	IngressHost string            `json:"ingress_host,omitempty"`
	Headers     map[string]string `json:"headers,omitempty"`
}

func NewAccessEvent(
	sourceIP string,
	headers map[string]string,
	deviceID string,
	edlMode string,
	allowed bool,
	responseCode int,
) *AccessEvent {
	// Determine outcome and reason
	outcome := "allowed"
	reason := "in_allowlist"

	if !allowed {
		outcome = "blocked"
		if edlMode == "allowlist" {
			reason = "not_in_allowlist"
		} else if edlMode == "blocklist" {
			reason = "in_blocklist"
		}
	} else {
		if edlMode == "allowlist" {
			reason = "in_allowlist"
		} else if edlMode == "blocklist" {
			reason = "not_in_blocklist"
		}
	}

	// Extract real request details from X-Forwarded headers ONLY
	method := headers["X-Forwarded-Method"]
	actualHost := headers["X-Forwarded-Host"]
	actualPath := headers["X-Forwarded-Uri"]
	scheme := headers["X-Forwarded-Proto"]

	event := &AccessEvent{
		Timestamp:  time.Now().UTC(),
		EventType:  "access_decision",
		Outcome:    outcome,
		Reason:     reason,
		StatusCode: responseCode,
		DeviceID:   deviceID,
		Request: RequestDetails{
			Method: method,
			Host:   actualHost,
			Path:   actualPath,
			Scheme: scheme,
		},
		Client: ClientInfo{
			IP:        sourceIP,
			UserAgent: headers["User-Agent"],
		},
		Policy: PolicyInfo{
			Mode: edlMode,
		},
	}

	// Add internal debug info if needed
	if headers["X-Forwarded-Server"] != "" || headers["X-Real-Ip"] != "" {
		debugHeaders := make(map[string]string)

		// Only include useful debug headers
		debugKeys := []string{
			"X-Forwarded-Server",
			"X-Forwarded-Port",
			"X-Real-Ip",
			"X-Forwarded-Method",
		}

		for _, key := range debugKeys {
			if val, ok := headers[key]; ok && val != "" {
				debugHeaders[key] = val
			}
		}

		if len(debugHeaders) > 0 {
			event.Internal = &InternalInfo{
				ProxyPath:   "/auth", // The actual path hit on forwardauth
				IngressHost: headers["X-Forwarded-Host"],
				Headers:     debugHeaders,
			}
		}
	}

	return event
}
