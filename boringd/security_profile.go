package main

import (
	"fmt"
	"net"
	"strings"
)

const isolatedWorkerProfile = "isolated-worker"

type configError struct {
	message string
}

func (e *configError) Error() string { return e.message }

// Validate rejects invalid named security profiles before boringd reaps VMs,
// opens a listener, or creates host networking state.
func (c Config) Validate() error {
	if c.tokenLoadError != nil {
		return c.tokenLoadError
	}
	if c.SecurityProfile == "" {
		return nil
	}
	if c.SecurityProfile != isolatedWorkerProfile {
		return fmt.Errorf("unknown BORING_SECURITY_PROFILE %q", c.SecurityProfile)
	}

	var violations []string
	if !isLoopbackAddress(c.Addr) {
		violations = append(violations, "BORING_ADDR must bind to a loopback IP")
	}
	if strings.TrimSpace(c.Token) == "" {
		violations = append(violations, "BORING_TOKEN or BORING_TOKEN_FILE is required")
	}
	if c.AllowQueryToken {
		violations = append(violations, "BORING_ALLOW_QUERY_TOKEN must be 0")
	}
	if !c.JailerEnable {
		violations = append(violations, "BORING_JAILER must be 1")
	}
	if !c.CgroupEnable || c.CPUMaxPercent < 1 || c.PidsMax < 1 {
		violations = append(violations, "cgroup CPU and PID limits must be enabled and positive")
	}
	if !c.NetEnable {
		violations = append(violations, "BORING_NET must be 1 so the egress policy is applied")
	}
	if c.MaxMachines != 1 || c.PerIPMax != 1 {
		violations = append(violations, "BORING_MAX and BORING_PER_IP_MAX must both be 1")
	}
	if c.MaxTemplates != 0 {
		violations = append(violations, "BORING_MAX_TEMPLATES must be 0")
	}
	if c.MaxForks != 1 {
		violations = append(violations, "BORING_MAX_FORKS must be 1")
	}
	if c.AllowPersistent {
		violations = append(violations, "BORING_ALLOW_PERSISTENT must be 0")
	}
	if c.MinTTL < 1 || c.DefaultTTL < c.MinTTL || c.DefaultTTL > c.MaxTTL || c.MaxTTL > 900 {
		violations = append(violations, "TTL bounds must be ordered and BORING_MAX_TTL must not exceed 900")
	}
	if c.DesktopPool != 0 {
		violations = append(violations, "BORING_DESKTOP_POOL must be 0")
	}
	if !c.DisablePreview || c.PreviewBase != "" {
		violations = append(violations, "preview routes and preview hosts must be disabled")
	}
	if c.TrustProxy || c.CORSOrigin != "" {
		violations = append(violations, "proxy trust and CORS must be disabled for the loopback control plane")
	}
	if c.S3Endpoint != "" || c.S3Key != "" || c.S3Secret != "" {
		violations = append(violations, "S3 credentials and persistent storage must be disabled")
	}
	if c.AnthropicKey != "" || c.OpenRouterKey != "" {
		violations = append(violations, "cloud model credentials must not be configured")
	}
	if c.BuilderVCPUs < 1 || c.BuilderMemSizeMB < 1024 {
		violations = append(violations, "the builder requires at least one vCPU and 1024 MiB")
	}
	if len(violations) > 0 {
		return &configError{message: isolatedWorkerProfile + " profile rejected configuration: " + strings.Join(violations, "; ")}
	}
	return nil
}

func isLoopbackAddress(address string) bool {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return false
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
