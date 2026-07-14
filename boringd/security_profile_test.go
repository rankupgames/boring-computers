package main

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIsolatedWorkerProfileAcceptsHardenedConfiguration(t *testing.T) {
	cfg := validIsolatedWorkerConfig()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestIsolatedWorkerProfileRejectsUnsafeConfiguration(t *testing.T) {
	tests := []struct {
		name      string
		configure func(*Config)
		want      string
	}{
		{name: "public bind", configure: func(cfg *Config) { cfg.Addr = "0.0.0.0:8080" }, want: "loopback"},
		{name: "missing token", configure: func(cfg *Config) { cfg.Token = "" }, want: "required"},
		{name: "query token", configure: func(cfg *Config) { cfg.AllowQueryToken = true }, want: "ALLOW_QUERY_TOKEN"},
		{name: "no jailer", configure: func(cfg *Config) { cfg.JailerEnable = false }, want: "BORING_JAILER"},
		{name: "root jailer", configure: func(cfg *Config) { cfg.JailerUID = 0 }, want: "unprivileged identity"},
		{name: "unexpected jailer group", configure: func(cfg *Config) { cfg.JailerGID = 991 }, want: "unprivileged identity"},
		{name: "no cgroups", configure: func(cfg *Config) { cfg.CgroupEnable = false }, want: "cgroup"},
		{name: "no egress policy", configure: func(cfg *Config) { cfg.NetEnable = false }, want: "BORING_NET"},
		{name: "multiple machines", configure: func(cfg *Config) { cfg.MaxMachines = 2 }, want: "must both be 1"},
		{name: "template publishing", configure: func(cfg *Config) { cfg.MaxTemplates = 1 }, want: "MAX_TEMPLATES"},
		{name: "persistent machine", configure: func(cfg *Config) { cfg.AllowPersistent = true }, want: "ALLOW_PERSISTENT"},
		{name: "long ttl", configure: func(cfg *Config) { cfg.MaxTTL = 901 }, want: "must not exceed 900"},
		{name: "preview route", configure: func(cfg *Config) { cfg.DisablePreview = false }, want: "preview"},
		{name: "cloud key", configure: func(cfg *Config) { cfg.AnthropicKey = "configured" }, want: "cloud model"},
		{name: "persistent storage", configure: func(cfg *Config) { cfg.S3Endpoint = "configured" }, want: "persistent storage"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg := validIsolatedWorkerConfig()
			test.configure(&cfg)
			err := cfg.Validate()
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Validate() error = %v, want substring %q", err, test.want)
			}
		})
	}
}

func TestLoadConfigReadsTokenFromProtectedFile(t *testing.T) {
	tokenPath := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(tokenPath, []byte("control-token\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BORING_TOKEN", "")
	t.Setenv("BORING_TOKEN_FILE", tokenPath)

	cfg := LoadConfig()
	if cfg.Token != "control-token" {
		t.Fatalf("Token = %q, want protected file value", cfg.Token)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestLoadConfigRejectsAmbiguousTokenSources(t *testing.T) {
	t.Setenv("BORING_TOKEN", "direct")
	t.Setenv("BORING_TOKEN_FILE", filepath.Join(t.TempDir(), "token"))

	cfg := LoadConfig()
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "mutually exclusive") {
		t.Fatalf("Validate() error = %v, want mutually exclusive", err)
	}
}

func TestLoadConfigRejectsUnprotectedTokenFile(t *testing.T) {
	tokenPath := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(tokenPath, []byte("control-token\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BORING_TOKEN", "")
	t.Setenv("BORING_TOKEN_FILE", tokenPath)

	cfg := LoadConfig()
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "inaccessible to group and other users") {
		t.Fatalf("Validate() error = %v, want protected-file error", err)
	}
}

func TestHardenedServerRejectsQueryTokenAndOmitsPreviewRoute(t *testing.T) {
	cfg := validIsolatedWorkerConfig()
	server := NewServer(cfg, NewManager(cfg))

	queryRequest := httptest.NewRequest("GET", "/v1/machines?token=control-token", nil)
	if server.authorized(queryRequest) {
		t.Fatal("query token unexpectedly authorized")
	}

	previewRequest := httptest.NewRequest("GET", "/v1/machines/machine/web/8080/", nil)
	previewResponse := httptest.NewRecorder()
	server.ServeHTTP(previewResponse, previewRequest)
	if previewResponse.Code != 404 {
		t.Fatalf("preview status = %d, want 404", previewResponse.Code)
	}
}

func TestUntermBuilderTemplateUsesHeadlessVsock(t *testing.T) {
	cfg := validIsolatedWorkerConfig()
	cfg.BuilderRootfs = "/builder.ext4"
	cfg.BuilderVCPUs = 4
	cfg.BuilderMemSizeMB = 6144

	template := cfg.Template("unterm-builder")
	if template.Rootfs != cfg.BuilderRootfs || template.VCPUs != 4 || template.MemSizeMB != 6144 {
		t.Fatalf("Template() = %+v", template)
	}
	if !template.Vsock || template.Display || template.Snapshot {
		t.Fatalf("Template() vsock/display/snapshot = %v/%v/%v", template.Vsock, template.Display, template.Snapshot)
	}
}

func validIsolatedWorkerConfig() Config {
	return Config{
		SecurityProfile:  isolatedWorkerProfile,
		Addr:             "127.0.0.1:8080",
		Token:            "control-token",
		AllowQueryToken:  false,
		MaxMachines:      1,
		MaxTemplates:     0,
		MaxForks:         1,
		MemReserveMB:     3072,
		DefaultTTL:       900,
		MinTTL:           60,
		MaxTTL:           900,
		BuilderVCPUs:     2,
		BuilderMemSizeMB: 4096,
		PerIPMax:         1,
		CgroupEnable:     true,
		CPUMaxPercent:    200,
		PidsMax:          512,
		NetEnable:        true,
		DisablePreview:   true,
		DesktopPool:      0,
		JailerEnable:     true,
		JailerUID:        30000,
		JailerGID:        30000,
	}
}
