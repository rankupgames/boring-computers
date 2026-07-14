package main

import (
	"os"
	"strconv"
	"strings"
)

// Config holds all runtime configuration for boringd. Values come from env with
// the fixed defaults described in the boring-computers contract.
type Config struct {
	// SecurityProfile enables a named, fail-closed deployment policy. Empty keeps
	// the general-purpose upstream behavior; "isolated-worker" is the hardened
	// single-worker policy used by the Rank Up build host.
	SecurityProfile string

	// Listen address for the HTTP/WS server.
	Addr string

	// Token, if non-empty, requires "Authorization: Bearer <token>" on /v1/*
	// routes (and ?token= on the WebSocket route). /healthz is always open.
	Token          string
	tokenLoadError error

	// AllowQueryToken permits bearer tokens in URL query strings for WebSocket
	// clients. Hardened workers disable this to keep credentials out of URLs.
	AllowQueryToken bool

	// CORSOrigin is sent as Access-Control-Allow-Origin so a browser on another
	// origin (the deployed site) can call the public endpoint. "" disables CORS.
	CORSOrigin string

	// MaxMachines caps the number of live machines; creation returns 429 when full.
	MaxMachines int

	// MaxTemplates caps user-published templates (POST /v1/machines/{id}/publish);
	// 0 disables publishing entirely. Built-in templates don't count.
	MaxTemplates int

	// MaxForks caps ?count on POST /v1/machines/{id}/branch (fleet fork).
	MaxForks int

	// MemReserveMB is host RAM kept free — boot is refused if a new VM would eat
	// into it, so the box gracefully hits capacity instead of OOMing. 0 disables.
	MemReserveMB int

	// Fixed host paths (created by bootstrap).
	FirecrackerBin string // /opt/boring/bin/firecracker
	KernelPath     string // /opt/boring/kernel/vmlinux
	BaseRootfs     string // /opt/boring/rootfs/rootfs.ext4
	DesktopRootfs  string // /opt/boring/rootfs/desktop.ext4
	BuilderRootfs  string // /opt/boring/rootfs/unterm-builder.ext4
	TemplatesDir   string // /opt/boring/templates
	RunDir         string // /opt/boring/run

	// TTL clamp bounds (seconds) and default.
	DefaultTTL int
	MinTTL     int
	MaxTTL     int

	// AllowPersistent lets a request opt out of the TTL entirely (a machine that
	// runs until explicitly stopped). Off by default so a public instance can't be
	// drained by never-expiring machines; self-hosters set BORING_ALLOW_PERSISTENT=1.
	AllowPersistent bool

	// Guest machine sizing.
	VCPUs            int
	MemSizeMB        int
	BuilderVCPUs     int
	BuilderMemSizeMB int

	// Public-facing abuse controls.
	PerIPMax         int  // max concurrent machines per client IP
	CreateRatePerMin int  // max creations per minute per client IP
	TrustProxy       bool // read client IP from X-Forwarded-For (behind Caddy)

	// Per-VM cgroup v2 caps (0 disables that limit).
	CgroupEnable  bool
	CPUMaxPercent int // host CPU % cap per VM (e.g. 100 = 1 core)
	PidsMax       int // max host-visible pids for the firecracker child

	// Guest internet: attach a NIC per cold-booted VM, NAT out via the host. The
	// host side (bridge, dnsmasq, egress firewall) is set up by net-setup.sh.
	NetEnable bool   // BORING_NET=="1"
	NetBridge string // bridge to attach taps to (default boring0)
	NetSubnet string // guest /24 prefix, e.g. 10.200.0 (gateway .1)

	// Preview: expose a guest port at <id>--<port>.<PreviewBase>.
	PreviewBase    string // BORING_PREVIEW_BASE, e.g. previews.example.com ("" disables previews)
	LeasesPath     string // dnsmasq lease file, for guest IP lookup
	DisablePreview bool

	// Storage: persistent volumes on an S3-compatible store (MinIO / Latitude).
	// Enabled when S3Endpoint is set.
	S3Endpoint       string // BORING_S3_ENDPOINT host:port (no scheme)
	S3Key            string // BORING_S3_KEY
	S3Secret         string // BORING_S3_SECRET
	S3Bucket         string // BORING_S3_BUCKET (default boring-volumes)
	S3Region         string // BORING_S3_REGION (SigV4 signing region)
	S3UseSSL         bool   // BORING_S3_SSL=="1"
	VolumeQuotaMB    int    // per-volume size cap
	VolumeTTLDefault int    // default volume lifetime (seconds)
	VolumeTTLMax     int    // max volume lifetime (seconds)
	VolumeRatePerMin int    // per-IP volume creations/min

	// Warm pool: keep this many desktops pre-booted so a request is instant.
	DesktopPool int

	// Inference gateway: an OpenAI-compatible /v1/chat/completions that routes
	// Claude models to Anthropic natively and everything else to OpenRouter.
	// Enabled when either key is set. Both may be set at once.
	OpenRouterKey       string // BORING_OPENROUTER_KEY
	InferenceMaxTokens  int    // hard cap on max_tokens per request (cost guard)
	InferenceRatePerMin int    // per-IP requests/min (cost guard)

	// Global daily circuit breakers on the shared Anthropic key (0 disables).
	DailyAgentMax int // agent runs (computer-use + terminal) per UTC day
	DailyInferMax int // inference requests per UTC day

	// Computer-use agent: an AI driving the GUI desktop, streamed to the browser.
	// AnthropicKey also backs the gateway's Claude path.
	AnthropicKey       string // BORING_ANTHROPIC_KEY; empty disables the agent
	AgentModel         string // model id (default claude-opus-4-8)
	AgentMaxSteps      int    // hard cap on model turns per run (cost guard)
	AgentMaxConcurrent int    // hard cap on simultaneous agent runs (cost guard)

	// Jailer: run firecracker chrooted + unprivileged (defense-in-depth).
	JailerEnable bool
	JailerBin    string // /opt/boring/bin/jailer
	JailerUID    int
	JailerGID    int
	ChrootBase   string // /srv/jailer
}

// LoadConfig builds a Config from the environment, applying the fixed defaults.
func LoadConfig() Config {
	token, tokenLoadError := loadSecret("BORING_TOKEN", "BORING_TOKEN_FILE")
	c := Config{
		SecurityProfile:     os.Getenv("BORING_SECURITY_PROFILE"),
		Addr:                envStr("BORING_ADDR", "0.0.0.0:8080"),
		Token:               token,
		tokenLoadError:      tokenLoadError,
		AllowQueryToken:     envBool("BORING_ALLOW_QUERY_TOKEN", true),
		CORSOrigin:          os.Getenv("BORING_CORS_ORIGIN"),
		MaxMachines:         envInt("BORING_MAX", 20),
		MaxTemplates:        envInt("BORING_MAX_TEMPLATES", 10),
		MaxForks:            envInt("BORING_MAX_FORKS", 8),
		AllowPersistent:     os.Getenv("BORING_ALLOW_PERSISTENT") == "1",
		MemReserveMB:        envInt("BORING_MEM_RESERVE_MB", 3072),
		FirecrackerBin:      envStr("BORING_FIRECRACKER_BIN", "/opt/boring/bin/firecracker"),
		KernelPath:          envStr("BORING_KERNEL", "/opt/boring/kernel/vmlinux"),
		BaseRootfs:          envStr("BORING_ROOTFS", "/opt/boring/rootfs/rootfs.ext4"),
		DesktopRootfs:       envStr("BORING_DESKTOP_ROOTFS", "/opt/boring/rootfs/desktop.ext4"),
		BuilderRootfs:       envStr("BORING_BUILDER_ROOTFS", "/opt/boring/rootfs/unterm-builder.ext4"),
		TemplatesDir:        envStr("BORING_TEMPLATES", "/opt/boring/templates"),
		RunDir:              envStr("BORING_RUN", "/opt/boring/run"),
		DefaultTTL:          envInt("BORING_DEFAULT_TTL", 120),
		MinTTL:              envInt("BORING_MIN_TTL", 15),
		MaxTTL:              envInt("BORING_MAX_TTL", 900),
		VCPUs:               envInt("BORING_VCPUS", 1),
		MemSizeMB:           envInt("BORING_MEM_MB", 256),
		BuilderVCPUs:        envInt("BORING_BUILDER_VCPUS", 2),
		BuilderMemSizeMB:    envInt("BORING_BUILDER_MEM_MB", 4096),
		PerIPMax:            envInt("BORING_PER_IP_MAX", 2),
		CreateRatePerMin:    envInt("BORING_CREATE_RATE", 8),
		TrustProxy:          os.Getenv("BORING_TRUST_PROXY") == "1",
		CgroupEnable:        os.Getenv("BORING_CGROUP") != "0",
		CPUMaxPercent:       envInt("BORING_CPU_MAX_PCT", 150),
		PidsMax:             envInt("BORING_PIDS_MAX", 512),
		NetEnable:           os.Getenv("BORING_NET") == "1",
		NetBridge:           envStr("BORING_NET_BRIDGE", "boring0"),
		NetSubnet:           envStr("BORING_NET_SUBNET", "10.200.0"),
		PreviewBase:         os.Getenv("BORING_PREVIEW_BASE"), // deployment-specific; unset disables previews
		LeasesPath:          envStr("BORING_LEASES", "/var/lib/misc/dnsmasq.leases"),
		DisablePreview:      os.Getenv("BORING_DISABLE_PREVIEW") == "1",
		S3Endpoint:          os.Getenv("BORING_S3_ENDPOINT"),
		S3Key:               os.Getenv("BORING_S3_KEY"),
		S3Secret:            os.Getenv("BORING_S3_SECRET"),
		S3Bucket:            envStr("BORING_S3_BUCKET", "boring-volumes"),
		S3Region:            os.Getenv("BORING_S3_REGION"),
		S3UseSSL:            os.Getenv("BORING_S3_SSL") == "1",
		VolumeQuotaMB:       envInt("BORING_VOLUME_QUOTA_MB", 256),
		VolumeTTLDefault:    envInt("BORING_VOLUME_TTL", 86400),
		VolumeTTLMax:        envInt("BORING_VOLUME_TTL_MAX", 604800),
		VolumeRatePerMin:    envInt("BORING_VOLUME_RATE", 10),
		DesktopPool:         envInt("BORING_DESKTOP_POOL", 1),
		OpenRouterKey:       os.Getenv("BORING_OPENROUTER_KEY"),
		InferenceMaxTokens:  envInt("BORING_INFER_MAX_TOKENS", 1024),
		InferenceRatePerMin: envInt("BORING_INFER_RATE", 20),
		DailyAgentMax:       envInt("BORING_DAILY_AGENT_MAX", 200),
		DailyInferMax:       envInt("BORING_DAILY_INFER_MAX", 3000),
		AnthropicKey:        os.Getenv("BORING_ANTHROPIC_KEY"),
		AgentModel:          envStr("BORING_AGENT_MODEL", "claude-opus-4-8"),
		AgentMaxSteps:       envInt("BORING_AGENT_MAX_STEPS", 30),
		AgentMaxConcurrent:  envInt("BORING_AGENT_MAX_CONCURRENT", 2),
		JailerEnable:        os.Getenv("BORING_JAILER") == "1",
		JailerBin:           envStr("BORING_JAILER_BIN", "/opt/boring/bin/jailer"),
		JailerUID:           envInt("BORING_JAILER_UID", 30000),
		JailerGID:           envInt("BORING_JAILER_GID", 30000),
		ChrootBase:          envStr("BORING_CHROOT_BASE", "/srv/jailer"),
	}
	if c.MaxMachines < 1 {
		c.MaxMachines = 1
	}
	return c
}

// ClampTTL applies the default when ttl <= 0 and clamps into [MinTTL, MaxTTL].
func (c Config) ClampTTL(ttl int) int {
	if ttl <= 0 {
		ttl = c.DefaultTTL
	}
	if ttl < c.MinTTL {
		ttl = c.MinTTL
	}
	if ttl > c.MaxTTL {
		ttl = c.MaxTTL
	}
	return ttl
}

func envStr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

// envBool reads the common 1/0 boolean form while preserving an explicit
// default for backward-compatible settings.
func envBool(key string, def bool) bool {
	value := os.Getenv(key)
	if value == "" {
		return def
	}
	return value == "1"
}

// loadSecret accepts either a direct environment value or a protected file,
// never both. systemd credentials use the file path so the token is absent from
// the daemon's inherited environment and command line.
func loadSecret(valueKey, fileKey string) (string, error) {
	value := os.Getenv(valueKey)
	path := os.Getenv(fileKey)
	if value != "" && path != "" {
		return "", &configError{message: valueKey + " and " + fileKey + " are mutually exclusive"}
	}
	if path == "" {
		return value, nil
	}
	info, err := os.Lstat(path)
	if err != nil {
		return "", &configError{message: "inspect " + fileKey + ": " + err.Error()}
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 {
		return "", &configError{message: fileKey + " must be a regular file inaccessible to group and other users"}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", &configError{message: "read " + fileKey + ": " + err.Error()}
	}
	return strings.TrimSpace(string(data)), nil
}
