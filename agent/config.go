package main

import (
	"encoding/json"
	"fmt"
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
		if !os.IsNotExist(err) {
			panic(fmt.Errorf("read saved pairing: %w", err))
		}
		return cfg
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		panic(fmt.Errorf("invalid saved pairing (left unchanged): %w", err))
	}
	if cfg.Repo == "" {
		cfg.Repo = defaultRepo
	}
	if cfg.Branch == "" {
		cfg.Branch = defaultBranch
	}
	return cfg
}

func saveConfig(cfg Config) error {
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(dataDir, "config-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err := f.Write(raw); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), configPath())
}
