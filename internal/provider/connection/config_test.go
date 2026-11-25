package connection

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/netapp/terraform-provider-netapp-storagegrid/internal/utils"
)

func profile(hostname string) Profile {
	return Profile{Hostname: hostname, Username: "admin", Password: "secret", ValidateCerts: true}
}

func TestGetConnectionProfile(t *testing.T) {
	one := &Config{ConnectionProfiles: map[string]Profile{"grid1": profile("a.example.com")}}
	two := &Config{ConnectionProfiles: map[string]Profile{"grid1": profile("a.example.com"), "grid2": profile("b.example.com")}}

	tests := []struct {
		name     string
		config   *Config
		request  string
		expected string
		errorMsg string
	}{
		{"single profile, no name", one, "", "grid1", ""},
		{"single profile, by name", one, "grid1", "grid1", ""},
		{"two profiles, by name", two, "grid2", "grid2", ""},
		{"two profiles, no name", two, "", "", "grid1, grid2"},
		{"unknown name", two, "nope", "", "nope is not defined"},
		{"no profile", &Config{}, "", "", "at least one connection profile"},
		{"nil config", nil, "", "", "not initialized"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			name, got, err := test.config.GetConnectionProfile(test.request)
			if test.errorMsg != "" {
				if err == nil || !strings.Contains(err.Error(), test.errorMsg) {
					t.Errorf("expected an error containing %q, got %v", test.errorMsg, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %s", err)
			}
			if name != test.expected || got == nil || got.Hostname != test.config.ConnectionProfiles[test.expected].Hostname {
				t.Errorf("expected profile %s, got %s: %#v", test.expected, name, got)
			}
		})
	}
}

func TestNewClientIsCachedPerProfile(t *testing.T) {
	config := &Config{
		Version: "test",
		ConnectionProfiles: map[string]Profile{
			"grid1": profile("a.example.com"),
			"grid2": profile("b.example.com"),
		},
	}
	var diags diag.Diagnostics
	errorHandler := utils.NewErrorHandler(context.Background(), &diags)

	first, err := config.NewClient(errorHandler, "grid1")
	if err != nil {
		t.Fatalf("NewClient: %s", err)
	}
	again, err := config.NewClient(errorHandler, "grid1")
	if err != nil {
		t.Fatalf("NewClient: %s", err)
	}
	other, err := config.NewClient(errorHandler, "grid2")
	if err != nil {
		t.Fatalf("NewClient: %s", err)
	}
	if len(config.clients) != 2 {
		t.Errorf("expected one client per profile, got %d", len(config.clients))
	}
	if first.IsTenant() || again.IsTenant() || other.IsTenant() {
		t.Error("no account id, so expected grid clients")
	}
}

func TestNewClientReportsErrors(t *testing.T) {
	config := &Config{ConnectionProfiles: map[string]Profile{
		"bad": {Hostname: "http://insecure.example.com", Username: "u", Password: "p"},
	}}
	var diags diag.Diagnostics
	errorHandler := utils.NewErrorHandler(context.Background(), &diags)

	if _, err := config.NewClient(errorHandler, "bad"); err == nil {
		t.Fatal("expected an error")
	}
	if !diags.HasError() {
		t.Error("the error must be reported in the diagnostics")
	}
	if _, err := config.NewClient(errorHandler, "missing"); err == nil {
		t.Fatal("expected an error for an unknown profile")
	}
}
