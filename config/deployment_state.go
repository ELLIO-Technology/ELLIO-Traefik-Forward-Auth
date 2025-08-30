package config

// DeploymentState represents the current state of the deployment
type DeploymentState int

const (
	// DeploymentActive means the deployment is enabled and functional
	DeploymentActive DeploymentState = iota
	// DeploymentDisabled means the deployment is explicitly disabled
	DeploymentDisabled
	// DeploymentDeleted means the deployment has been permanently deleted (410 error)
	DeploymentDeleted
)

// IsActive returns true if the deployment should process traffic according to EDL rules
func (s DeploymentState) IsActive() bool {
	return s == DeploymentActive
}

// AllowsAllTraffic returns true if the deployment should allow all traffic
func (s DeploymentState) AllowsAllTraffic() bool {
	return s != DeploymentActive
}

// String returns a human-readable representation of the state
func (s DeploymentState) String() string {
	switch s {
	case DeploymentActive:
		return "active"
	case DeploymentDisabled:
		return "disabled"
	case DeploymentDeleted:
		return "deleted"
	default:
		return "unknown"
	}
}

// GetDeploymentState determines the deployment state based on configuration
func (cfg *Config) GetDeploymentState() DeploymentState {
	if !cfg.DeploymentEnabled {
		if cfg.TokenManager != nil && cfg.TokenManager.IsDeploymentDeleted() {
			return DeploymentDeleted
		}
		return DeploymentDisabled
	}
	return DeploymentActive
}
