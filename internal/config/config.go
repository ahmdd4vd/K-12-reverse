package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// GmailAccount holds the credentials and list file for a base Gmail
type GmailAccount struct {
	BaseEmail   string `json:"base_email"`
	AppPassword string `json:"app_password"`
	ListFile    string `json:"list_file"`
}

// Config holds the application configuration.
type Config struct {
	Proxy           string   `json:"proxy"`
	ProxyList       []string `json:"proxy_list,omitempty"`
	OutputFile      string   `json:"output_file"`
	TokenOutputFile string   `json:"token_output_file"`
	DefaultPassword string   `json:"default_password"`
	DefaultDomain   string   `json:"default_domain"`
	K12WorkspaceIDs []string `json:"k12_workspace_ids"`
	EnableK12Invite bool     `json:"enable_k12_invite"`
	WebhookURL      string   `json:"webhook_url,omitempty"`

	// Gmail mode settings
	GmailMode     bool           `json:"gmail_mode"`
	GmailAccounts []GmailAccount `json:"gmail_accounts"`
}

const (
	DefaultProxy           = ""
	DefaultOutputFile      = "results.txt"
	DefaultTokenOutputFile = "accounts.json"
	DefaultConfigFilename  = "config.json"
	DefaultPassword        = "" // Min 12 characters
	DefaultDomainValue     = ""
)

// DefaultConfigPath returns the default path to the config file.
func DefaultConfigPath() string {
	return DefaultConfigFilename
}

// Load reads the config from a JSON file and applies environment variable overrides.
func Load(path string) (*Config, error) {
	cfg := &Config{
		Proxy:           DefaultProxy,
		ProxyList:       []string{},
		OutputFile:      "results.txt",
		TokenOutputFile: DefaultTokenOutputFile,
		DefaultPassword: DefaultPassword,
		DefaultDomain:   DefaultDomainValue,
		K12WorkspaceIDs: []string{"ff598c4d-ccaf-40c1-bfaa-cb94565764b1"},
		EnableK12Invite: true,
		GmailMode:       false,
		GmailAccounts:   []GmailAccount{},
	}

	// Try to read the file
	data, err := os.ReadFile(path)
	if err == nil {
		if err := json.Unmarshal(data, cfg); err != nil {
			return nil, err
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	// Validate password length
	if cfg.DefaultPassword != "" && len(cfg.DefaultPassword) < 12 {
		return nil, fmt.Errorf("default_password must be at least 12 characters (got %d)", len(cfg.DefaultPassword))
	}

	// Environment variable overrides
	if proxy := os.Getenv("PROXY"); proxy != "" {
		cfg.Proxy = proxy
	}
	if proxyList := os.Getenv("PROXY_LIST"); proxyList != "" {
		cfg.ProxyList = strings.Split(proxyList, ",")
	}

	return cfg, nil
}

// Save writes the configuration back to a JSON file.
func (c *Config) Save(path string) error {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}
