// Package connection has the connection profiles of the provider, and gives each resource and data source the client of its profile.
package connection

import (
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/netapp/terraform-provider-netapp-storagegrid/internal/restclient"
	"github.com/netapp/terraform-provider-netapp-storagegrid/internal/utils"
)

// Profile describes how to reach and sign in to a grid, or to a tenant of a grid.
type Profile struct {
	Hostname  string
	Username  string
	Password  string
	AccountID string
	// ValidateCerts is true by default
	ValidateCerts         bool
	CACertFile            string
	APIVersion            string
	MaxConcurrentRequests int
}

// Config is created by the provider configure method.
// It is shared by all the resources and data sources, and must be used through a pointer.
type Config struct {
	ConnectionProfiles map[string]Profile
	Version            string

	mu      sync.Mutex
	clients map[string]*restclient.RestClient
}

// GetConnectionProfile retrieves a connection profile based on name
// If name is empty and only one profile is defined, it is returned
func (c *Config) GetConnectionProfile(name string) (string, *Profile, error) {
	if c == nil {
		return "", nil, fmt.Errorf("internal error, config is not initialized")
	}
	if len(c.ConnectionProfiles) == 0 {
		return "", nil, fmt.Errorf("error, at least one connection profile is required to connect to StorageGRID")
	}
	if name == "" && len(c.ConnectionProfiles) == 1 {
		for onlyName := range c.ConnectionProfiles {
			name = onlyName
		}
	}
	if name == "" {
		names := make([]string, 0, len(c.ConnectionProfiles))
		for profileName := range c.ConnectionProfiles {
			names = append(names, profileName)
		}
		sort.Strings(names)
		return "", nil, fmt.Errorf("error, connection profile name is required if more than one profile is defined: %s", strings.Join(names, ", "))
	}
	if profile, ok := c.ConnectionProfiles[name]; ok {
		return name, &profile, nil
	}
	return "", nil, fmt.Errorf("connection profile with name %s is not defined", name)
}

// NewClient returns the REST client of the connection profile identified by cxProfileName.
//
// There is a single client per profile.  It signs in once, and its token is used by every resource and data source
// that refers to the profile, instead of signing in for each call.  The returned client logs with the context of
// the error handler.
func (c *Config) NewClient(errorHandler *utils.ErrorHandler, cxProfileName string) (*restclient.RestClient, error) {
	name, profile, err := c.GetConnectionProfile(cxProfileName)
	if err != nil {
		return nil, errorHandler.MakeAndReportError("failed to set connection profile", err.Error())
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if client, ok := c.clients[name]; ok {
		return client.WithContext(errorHandler.Ctx), nil
	}

	// the tag is sent as the User-Agent, and used for telemetry
	tag := strings.Join([]string{"TerraformStorageGRID", c.Version}, "/")
	client, err := restclient.NewClient(errorHandler.Ctx, restclient.ConnectionProfile{
		Hostname:              profile.Hostname,
		Username:              profile.Username,
		Password:              profile.Password,
		AccountID:             profile.AccountID,
		ValidateCerts:         profile.ValidateCerts,
		CACertFile:            profile.CACertFile,
		APIVersion:            profile.APIVersion,
		MaxConcurrentRequests: profile.MaxConcurrentRequests,
	}, tag)
	if err != nil {
		return nil, errorHandler.MakeAndReportError("unable to create REST client",
			fmt.Sprintf("error creating REST client for connection profile %s: %s", name, err))
	}
	if c.clients == nil {
		c.clients = map[string]*restclient.RestClient{}
	}
	c.clients[name] = client
	return client, nil
}
