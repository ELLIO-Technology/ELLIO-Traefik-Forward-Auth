package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"
)

type Client struct {
	httpClient *http.Client
	apiURL     string
	apiKey     string
}

type DeploymentResponse struct {
	Deployment DeploymentInfo `json:"deployment"`
	URLs       URLs           `json:"urls"`
}

type DeploymentInfo struct {
	ID                     string    `json:"id"`
	Name                   string    `json:"name"`
	Purpose                string    `json:"purpose"`
	FirewallFormat         string    `json:"firewall_format"`
	UpdateFrequency        string    `json:"update_frequency"`
	UpdateFrequencySeconds int       `json:"update_frequency_seconds"`
	IPv4EntriesCount       int       `json:"ipv4_entries_count"`
	IPv6EntriesCount       int       `json:"ipv6_entries_count"`
	LastGenerated          time.Time `json:"last_generated"`
	LastAccessed           time.Time `json:"last_accessed"`
}

type URLs struct {
	Combined      []string `json:"combined"`
	IPv4          []string `json:"ipv4"`
	IPv6          []string `json:"ipv6"`
	Checksums     []string `json:"checksums"`
	Unprocessable []string `json:"unprocessable"`
}

func NewClient(apiURL, apiKey string) *Client {
	return &Client{
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		apiURL: apiURL,
		apiKey: apiKey,
	}
}

func (c *Client) GetDeploymentInfo(ctx context.Context, edlID string) (*DeploymentResponse, error) {
	url := c.apiURL + "/edl/deployments/" + edlID + "/urls/"

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, errors.New("creating request: " + err.Error())
	}

	req.Header.Set("X-API-KEY", c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, errors.New("making API request: " + err.Error())
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, errors.New("API error: " + string(body))
	}

	var deploymentResp DeploymentResponse
	if err := json.NewDecoder(resp.Body).Decode(&deploymentResp); err != nil {
		return nil, errors.New("decoding response: " + err.Error())
	}

	return &deploymentResp, nil
}
