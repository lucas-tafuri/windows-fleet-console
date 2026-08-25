package main

import (
	"encoding/json"
	"os"
	"path/filepath"
)

const (
	defaultRepo   = "https://github.com/lucas-tafuri/windows-fleet-console.git"
	defaultBranch = "main"
	defaultExeURL = "https://github.com/lucas-tafuri/windows-fleet-console/raw/main/dist/fleet-agent.exe"
	taskName      = "Fleet Console Agent"
)

type Config struct {
	Server    string `json:"server"`
	Token     string `json:"token"`
	HTTPOnly  bool   `json:"httpOnly"`
	MachineID string `json:"machineId,omitempty"`
	Repo      string `json:"repo,omitempty"`
	Branch    string `json:"branch,omitempty"`
}

var (
	dataDir          string
	agentCfg         Config
	exePath          string
	restartRequested bool
)

func configPath() string {
	return filepath.Join(dataDir, "config.json")
}

func loadConfig() Config {
	cfg := Config{Repo: defaultRepo, Branch: defaultBranch}
	raw, err := os.ReadFile(configPath())
	if err != nil {
		return cfg
	}
	_ = json.Unmarshal(raw, &cfg)
	if cfg.Repo == "" {
		cfg.Repo = defaultRepo
	}
	if cfg.Branch == "" {
		cfg.Branch = defaultBranch
	}
	return cfg
}

func saveConfig(cfg Config) {
	_ = os.MkdirAll(dataDir, 0o755)
	raw, _ := json.MarshalIndent(cfg, "", "  ")
	_ = os.WriteFile(configPath(), raw, 0o600)
}
