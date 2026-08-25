package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"
)

func main() {
	server := flag.String("server", "", "PrettyDamnFleet URL, e.g. http://192.168.1.10:43123")
	token := flag.String("token", "", "Fleet token from the Enroll page")
	httpOnly := flag.Bool("http-only", false, "Skip WebSocket and use HTTP poll only")
	dataDirFlag := flag.String("data-dir", "", "Directory for machine-id and install (default: %LOCALAPPDATA%\\FleetConsole)")
	flag.Parse()

	dir := *dataDirFlag
	if dir == "" {
		base, err := os.UserCacheDir()
		if err != nil {
			base, _ = os.UserHomeDir()
		}
		dir = filepath.Join(base, "FleetConsole")
	}
	dataDir = dir
	_ = os.MkdirAll(dataDir, 0o755)
	setupLog()

	agentCfg = loadConfig()
	if *server != "" {
		agentCfg.Server = trimSlash(*server)
	}
	if *token != "" {
		agentCfg.Token = *token
	}
	if *httpOnly {
		agentCfg.HTTPOnly = true
	}
	if agentCfg.Server == "" || agentCfg.Token == "" {
		fmt.Fprintln(os.Stderr, "usage: fleet-agent.exe --server http://host:43123 --token <token>")
		os.Exit(2)
	}
	saveConfig(agentCfg)

	relocated, err := ensureInstalled()
	if err != nil {
		fmt.Fprintf(os.Stderr, "startup install: %v\n", err)
	}
	if relocated {
		fmt.Println("Installed to", filepath.Join(dataDir, "fleet-agent.exe"))
		if err != nil {
			fmt.Println("Logon registration note:", err)
		} else {
			fmt.Println("Registered to start at Windows logon.")
		}
		fmt.Println("Switching to that copy.")
		os.Exit(0)
	}

	idPath := filepath.Join(dataDir, "machine-id")
	id, _ := os.ReadFile(idPath)
	machineID := string(bytesTrim(id))
	if machineID == "" {
		machineID = agentCfg.MachineID
	}

	client := &Client{
		Server:    agentCfg.Server,
		Token:     agentCfg.Token,
		HTTPOnly:  agentCfg.HTTPOnly,
		MachineID: machineID,
		OnID: func(id string) {
			_ = os.WriteFile(idPath, []byte(id), 0o600)
			agentCfg.MachineID = id
			saveConfig(agentCfg)
		},
	}

	fmt.Printf("PrettyDamnFleet agent → %s (ws=%v) install=%s\n", client.Server, !client.HTTPOnly, dataDir)

	go runLoop(client)
	serveTray(client)
}

func runLoop(client *Client) {
	backoff := time.Second
	for {
		err := client.RunOnce()
		if err != nil {
			fmt.Fprintf(os.Stderr, "agent: %v\n", err)
		}
		time.Sleep(backoff)
		if backoff < 15*time.Second {
			backoff *= 2
		}
		if backoff > 15*time.Second {
			backoff = 15 * time.Second
		}
	}
}

func waitSignal() {
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
}

func trimSlash(s string) string {
	for len(s) > 0 && s[len(s)-1] == '/' {
		s = s[:len(s)-1]
	}
	return s
}

func bytesTrim(b []byte) []byte {
	i, j := 0, len(b)
	for i < j && (b[i] == ' ' || b[i] == '\n' || b[i] == '\r' || b[i] == '\t') {
		i++
	}
	for j > i && (b[j-1] == ' ' || b[j-1] == '\n' || b[j-1] == '\r' || b[j-1] == '\t') {
		j--
	}
	return b[i:j]
}
