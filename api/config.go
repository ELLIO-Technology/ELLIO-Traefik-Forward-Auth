package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"
)

type ConfigClient struct {
	httpClient   *http.Client
	tokenManager *TokenManager
}

type EDLConfig struct {
	DeploymentID           string  `json:"deployment_id"`
	WorkspaceID            string  `json:"workspace_id"`
	Name                   string  `json:"name"`
	Purpose                string  `json:"purpose"`
	UpdateFrequencySeconds int     `json:"update_frequency_seconds"`
	URLs                   EDLURLs `json:"urls"`
	Enabled                bool    `json:"enabled"`
}

type EDLURLs struct {
	Combined      []string `json:"combined"`
	IPv4          []string `json:"ipv4"`
	IPv6          []string `json:"ipv6"`
	Checksums     []string `json:"checksums"`
	Unprocessable []string `json:"unprocessable"`
}

type ErrorResponse struct {
	Error string `json:"error"`
	Code  string `json:"code"`
}

func NewConfigClient(tokenManager *TokenManager) *ConfigClient {
	return &ConfigClient{
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		tokenManager: tokenManager,
	}
}

func (c *ConfigClient) GetEDLConfig(ctx context.Context) (*EDLConfig, error) {
	configURL := c.tokenManager.GetConfigURL()
	if configURL == "" {
		return nil, errors.New("config URL not available")
	}

	req, err := http.NewRequestWithContext(ctx, "GET", configURL, nil)
	if err != nil {
		return nil, errors.New("failed to create config request: " + err.Error())
	}

	token := c.tokenManager.GetToken()
	if token == "" {
		return nil, errors.New("no access token available")
	}

	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, errors.New("config request failed: " + err.Error())
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.New("failed to read response body: " + err.Error())
	}

	// Check for deployment disabled/deleted
	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusGone {
		var errResp ErrorResponse
		if err := json.Unmarshal(bodyBytes, &errResp); err == nil {
			if errResp.Code == "DEPLOYMENT_DISABLED" || errResp.Code == "DEPLOYMENT_DELETED" {
				// Return a special config that indicates "allow all"
				return &EDLConfig{
					Enabled: false,
					Purpose: "disabled",
				}, nil
			}
		}

		// For 410, return permanent error
		if resp.StatusCode == http.StatusGone {
			return nil, &PermanentError{
				StatusCode: resp.StatusCode,
				Message:    string(bodyBytes),
			}
		}

		return nil, errors.New("config request failed: " + string(bodyBytes))
	}

	if resp.StatusCode != http.StatusOK {
		return nil, errors.New("config request failed: " + string(bodyBytes))
	}

	var config EDLConfig
	if err := json.Unmarshal(bodyBytes, &config); err != nil {
		return nil, errors.New("failed to decode config response: " + err.Error())
	}

	// Set enabled to true for normal configs
	config.Enabled = true

	return &config, nil
}
