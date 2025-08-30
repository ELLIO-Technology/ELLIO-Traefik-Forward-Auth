package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/keygen-sh/machineid"
	"github.com/getsentry/sentry-go"
)

type BootstrapClient struct {
	httpClient *http.Client
	apiURL     string
}

type BootstrapRequest struct {
	BootstrapToken   string   `json:"bootstrap_token"`
	ComponentType    string   `json:"component_type"`
	ComponentVersion string   `json:"component_version"`
	MachineID        string   `json:"machine_id"`
	Scopes           []string `json:"scopes,omitempty"`
}

type BootstrapResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
	JWKSUrl     string `json:"jwks_url"`
	ConfigURL   string `json:"config_url"`
	LogsURL     string `json:"logs_url"`
}

type BootstrapClaims struct {
	jwt.RegisteredClaims
	WorkspaceID   string `json:"workspace_id"`
	DeploymentID  string `json:"deployment_id"`
	ComponentType string `json:"component_type"`
}

func NewBootstrapClient() *BootstrapClient {
	return &BootstrapClient{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (c *BootstrapClient) parseBootstrapToken(token string) (string, string, string, error) {
	parser := jwt.NewParser(jwt.WithoutClaimsValidation())

	var claims BootstrapClaims
	_, _, err := parser.ParseUnverified(token, &claims)
	if err != nil {
		return "", "", "", errors.New("failed to parse bootstrap token: " + err.Error())
	}

	if claims.Issuer == "" {
		return "", "", "", errors.New("bootstrap token missing issuer")
	}

	if claims.ComponentType == "" {
		return "", "", "", errors.New("bootstrap token missing component_type")
	}

	if claims.DeploymentID == "" {
		return "", "", "", errors.New("bootstrap token missing deployment_id")
	}

	return claims.Issuer, claims.ComponentType, claims.DeploymentID, nil
}

func (c *BootstrapClient) Bootstrap(ctx context.Context, bootstrapToken string) (*BootstrapResponse, error) {
	issuer, componentType, deploymentID, err := c.parseBootstrapToken(bootstrapToken)
	if err != nil {
		return nil, err
	}

	// Generate protected machine ID using deployment ID
	machineID, err := machineid.ProtectedID(deploymentID)
	if err != nil {
		return nil, errors.New("failed to get protected machine ID: " + err.Error())
	}

	// Construct bootstrap URL from issuer
	bootstrapURL := strings.TrimSuffix(issuer, "/") + "/api/v1/edl/bootstrap"

	req := BootstrapRequest{
		BootstrapToken:   bootstrapToken,
		ComponentType:    componentType,
		ComponentVersion: "1.0.0", // TODO: make this configurable
		MachineID:        machineID,
		Scopes:           []string{"edl_config", "edl_logs"}, // Request both config and logs access
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, errors.New("failed to marshal bootstrap request: " + err.Error())
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", bootstrapURL, bytes.NewReader(body))
	if err != nil {
		return nil, errors.New("failed to create bootstrap request: " + err.Error())
	}

	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, errors.New("bootstrap request failed: " + err.Error())
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))

		// 410 Gone means deployment is permanently deleted
		if resp.StatusCode == http.StatusGone {
			err := &PermanentError{
				StatusCode: resp.StatusCode,
				Message:    string(bodyBytes),
			}
			sentry.CaptureException(err)
			return nil, err
		}

		// Other errors (including 403) are temporary
		err := errors.New("bootstrap failed: " + string(bodyBytes))
		if resp.StatusCode >= 500 {
			sentry.CaptureException(err)
		}
		return nil, err
	}

	var bootstrapResp BootstrapResponse
	if err := json.NewDecoder(resp.Body).Decode(&bootstrapResp); err != nil {
		return nil, errors.New("failed to decode bootstrap response: " + err.Error())
	}

	// Store the API URL for future use
	c.apiURL = issuer

	return &bootstrapResp, nil
}
