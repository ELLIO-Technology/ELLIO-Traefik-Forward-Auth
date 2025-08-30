package auth

import (
	"errors"
	"io"
	"net"
	"net/http"
	"net/netip"
	"os"
	"strings"
	"time"

	"github.com/ELLIO-Technology/ellio_traefik_forward_auth/logs"
	"github.com/ELLIO-Technology/ellio_traefik_forward_auth/metrics"
	"github.com/ELLIO-Technology/ellio_traefik_forward_auth/ipmatcher"
	"github.com/ELLIO-Technology/ellio_traefik_forward_auth/logger"
	"github.com/getsentry/sentry-go"
)

// HTML content for the 403 page will be loaded from file system

type Handler struct {
	matcher           *ipmatcher.Matcher
	isBlocklist       bool
	deploymentEnabled bool
	logShipper        *logs.LogShipper
	deviceID          string
	ipHeaderOverride  string
}

func NewHandler(matcher *ipmatcher.Matcher, edlMode string, deploymentEnabled bool) *Handler {
	return &Handler{
		matcher:           matcher,
		isBlocklist:       edlMode == "blocklist",
		deploymentEnabled: deploymentEnabled,
	}
}

func (h *Handler) SetLogShipper(shipper *logs.LogShipper) {
	h.logShipper = shipper
}

func (h *Handler) SetDeviceID(deviceID string) {
	h.deviceID = deviceID
}

func (h *Handler) SetIPHeaderOverride(headerName string) {
	h.ipHeaderOverride = headerName
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	clientIP := h.extractClientIP(r)
	if clientIP == "" {
		metrics.RequestsTotal.WithLabelValues("invalid").Inc()
		metrics.RequestDuration.WithLabelValues("invalid").Observe(time.Since(start).Seconds())
		logger.Warn("Unable to determine client IP",
			"path", r.URL.Path,
			"headers", r.Header)
		sentry.CaptureMessage("Unable to determine client IP")
		http.Error(w, "Unable to determine client IP", http.StatusBadRequest)
		return
	}

	allowed, err := h.evaluateAccess(clientIP)
	if err != nil {
		// Invalid IP address error
		metrics.RequestsTotal.WithLabelValues("invalid").Inc()
		metrics.RequestDuration.WithLabelValues("invalid").Observe(time.Since(start).Seconds())
		logger.Error("Invalid IP address",
			"ip", clientIP,
			"error", err)
		sentry.CaptureException(errors.New("invalid IP address: " + clientIP))
		http.Error(w, "Invalid IP address", http.StatusBadRequest)
		return
	}

	if allowed {
		metrics.RequestsTotal.WithLabelValues("allowed").Inc()
		metrics.RequestDuration.WithLabelValues("allowed").Observe(time.Since(start).Seconds())
		w.WriteHeader(http.StatusOK)
	} else {
		metrics.RequestsTotal.WithLabelValues("denied").Inc()
		metrics.RequestDuration.WithLabelValues("denied").Observe(time.Since(start).Seconds())

		// Send block event to log shipper
		if h.logShipper != nil {
			h.sendAccessEvent(clientIP, r, false)
		}

		h.serveForbidden(w, r)
	}
}

func (h *Handler) serveForbidden(w http.ResponseWriter, r *http.Request) {
	accept := r.Header.Get("Accept")
	if strings.Contains(accept, "text/html") {
		file, err := os.Open("/static/403.html")
		if err != nil {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		defer file.Close()

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusForbidden)
		if _, err := io.Copy(w, file); err != nil {
			logger.Error("Failed to serve 403 page", "error", err)
		}
	} else {
		http.Error(w, "Forbidden", http.StatusForbidden)
	}
}

// evaluateAccess determines if the client IP should be allowed
func (h *Handler) evaluateAccess(clientIP string) (bool, error) {
	// If deployment is disabled, allow all traffic
	if !h.deploymentEnabled {
		return true, nil
	}

	addr, err := netip.ParseAddr(clientIP)
	if err != nil {
		return false, err
	}

	inList := h.matcher.Contains(addr)

	// XOR operation: allowed if (blocklist AND NOT in list) OR (allowlist AND in list)
	allowed := h.isBlocklist != inList
	return allowed, nil
}

func (h *Handler) extractClientIP(r *http.Request) string {
	// Check custom header override first if configured
	if h.ipHeaderOverride != "" {
		if customIP := r.Header.Get(h.ipHeaderOverride); customIP != "" {
			// Handle comma-separated values (like X-Forwarded-For)
			parts := strings.Split(customIP, ",")
			if len(parts) > 0 {
				return strings.TrimSpace(parts[0])
			}
		}
	}

	// Default behavior: check standard headers
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		if len(parts) > 0 {
			return strings.TrimSpace(parts[0])
		}
	}

	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return strings.TrimSpace(xri)
	}

	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}

	return r.RemoteAddr
}

func (h *Handler) sendAccessEvent(clientIP string, r *http.Request, allowed bool) {
	edlMode := "blocklist"
	if !h.isBlocklist {
		edlMode = "allowlist"
	}

	headers := make(map[string]string)
	for key, values := range r.Header {
		if len(values) > 0 {
			headers[key] = values[0]
		}
	}

	responseCode := http.StatusOK
	if !allowed {
		responseCode = http.StatusForbidden
	}

	event := logs.NewAccessEvent(
		clientIP,
		headers,
		h.deviceID,
		edlMode,
		allowed,
		responseCode,
	)

	h.logShipper.SendEvent(event)
}
