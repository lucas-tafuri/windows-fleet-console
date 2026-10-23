package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/cursor/fleet-agent/mesh"
	"github.com/cursor/fleet-agent/ui"
)

var meshVersion = "2.0.0-dev"

func isMeshInvocation(args []string) bool {
	for _, arg := range args[1:] {
		if arg == "--session-job" {
			return false
		}
	}
	return strings.HasPrefix(strings.ToLower(filepath.Base(args[0])), "fleet-node") || len(args) > 1 && args[1] == "--mesh"
}

func meshDefaultDir() string {
	base := os.Getenv("ProgramData")
	if base == "" {
		base, _ = os.UserConfigDir()
	}
	return filepath.Join(base, "FleetConsole", "mesh")
}

func meshMain(args []string) error {
	if len(args) > 0 && args[0] == "--mesh" {
		args = args[1:]
	}
	if len(args) == 0 {
		return errors.New("choose create, join, run, tray, inspect, backup, restore, import, or cache-release")
	}
	command := args[0]
	fs := flag.NewFlagSet("fleet-node "+command, flag.ContinueOnError)
	dir := fs.String("data-dir", meshDefaultDir(), "Protected local mesh data directory")
	advertise := fs.String("advertise", "", "Advertised LAN IP (automatic if omitted)")
	listen := fs.String("listen", "0.0.0.0", "Listen IP; use 127.0.0.1 for isolated development")
	uiPort := fs.Int("ui-port", 43123, "Dashboard port")
	peerPort := fs.Int("peer-port", 43125, "Peer TLS port")
	discoverPort := fs.Int("discovery-port", 43124, "Discovery UDP port; zero disables discovery")
	peer := fs.String("peer", "", "Existing peer HTTPS URL")
	fingerprint := fs.String("fingerprint", "", "Existing peer public-key fingerprint")
	fleetID := fs.String("fleet", "", "Fleet fingerprint")
	input := fs.String("input", "", "Source import, backup, or release manifest")
	output := fs.String("output", "", "New backup destination")
	service := fs.Bool("service", false, "Run under Windows Service Control Manager")
	request := fs.String("request", "", "Protected update request file")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if net.ParseIP(*listen) == nil {
		return errors.New("listen must be an IP address")
	}
	if command == "version" {
		fmt.Println(meshVersion)
		return nil
	}
	if command == "tray" {
		return runMeshTray(mesh.Endpoint("127.0.0.1", *uiPort, false))
	}
	if command == "apply-update" {
		return applyMeshUpdate(*request)
	}
	if command == "run" && *service {
		return runMeshService(func(ctx context.Context) error {
			return meshRun(ctx, *dir, *advertise, *listen, *uiPort, *peerPort, *discoverPort)
		})
	}
	if command == "run" {
		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer cancel()
		return meshRun(ctx, *dir, *advertise, *listen, *uiPort, *peerPort, *discoverPort)
	}
	var store *mesh.Store
	var err error
	if command == "restore" {
		store, err = mesh.RestoreHistory(*input, *dir)
	} else {
		store, err = mesh.Open(*dir, command == "create" || command == "join")
	}
	if err != nil {
		return err
	}
	defer store.Close()
	host, _ := os.Hostname()
	ip := *advertise
	if ip == "" {
		ip = mesh.LANAddress()
	}
	self := store.Identity.Member(host, mesh.Endpoint(ip, *peerPort, true), mesh.Endpoint(ip, *uiPort, false))
	if b, err := os.ReadFile(filepath.Join(filepath.Dir(*dir), "config.json")); err == nil {
		var legacy Config
		if json.Unmarshal(b, &legacy) == nil {
			self.LegacyID = legacy.MachineID
		}
	}
	switch command {
	case "create":
		if store.FleetID != "" {
			return errors.New("fleet already initialized; use run")
		}
		if err = store.Bootstrap(self); err != nil {
			return err
		}
		for _, app := range []mesh.CatalogItem{{ID: "adobe-ae", Name: "Adobe After Effects", Match: "Adobe After Effects"}, {ID: "adobe-ps", Name: "Adobe Photoshop", Match: "Adobe Photoshop"}, {ID: "adobe-cc", Name: "Adobe Creative Cloud", Match: "Adobe Creative Cloud"}, {ID: "maxon", Name: "Maxon", Match: "Maxon"}, {ID: "c4d-2026", Name: "Cinema 4D 2026", Match: "Cinema 4D 2026"}} {
			if _, err = store.EditCatalog(app, false); err != nil {
				return err
			}
		}
	case "join":
		n, err := mesh.NewNode(store, self)
		if err != nil {
			return err
		}
		fmt.Println("Approve this fingerprint on the existing dashboard:", self.ID)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		if err = n.Join(ctx, *peer, *fingerprint, *fleetID); err != nil {
			return err
		}
	case "backup":
		return store.Backup(*output)
	case "import":
		counts, err := store.ImportLegacy(*input)
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(counts)
	case "cache-release":
		cache, err := mesh.OpenReleaseCache(*dir)
		if err != nil {
			return err
		}
		id, err := cache.Fetch(context.Background(), *input, nil)
		if err != nil {
			return err
		}
		fmt.Println(id)
		return nil
	case "inspect", "restore":
	default:
		return fmt.Errorf("unknown mesh command %q", command)
	}
	return json.NewEncoder(os.Stdout).Encode(map[string]any{"nodeId": self.ID, "fleetId": store.FleetID, "peerUrl": self.PeerURL, "dashboardUrl": self.DashboardURL, "active": store.Active(self.ID), "version": meshVersion})
}

func meshRun(ctx context.Context, dir, advertise, listen string, uiPort, peerPort, discoverPort int) error {
	release, err := acquireInstance(false)
	if err != nil {
		return err
	}
	defer release()
	store, err := mesh.Open(dir, false)
	if err != nil {
		return err
	}
	defer store.Close()
	dataDir = dir
	background = true
	setupLog()
	log.SetOutput(os.Stderr)
	host, _ := os.Hostname()
	if advertise == "" {
		advertise = mesh.LANAddress()
	}
	self := store.Identity.Member(host, mesh.Endpoint(advertise, peerPort, true), mesh.Endpoint(advertise, uiPort, false))
	n, err := mesh.NewNode(store, self)
	if err != nil {
		return err
	}
	n.UI = ui.Handler()
	n.Version = meshVersion
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	n.BinaryHash, err = mesh.FileHash(exe)
	if err != nil {
		return err
	}
	n.Observe = func() mesh.Observation {
		s := collectSnapshot()
		drives := []map[string]string{}
		for _, d := range s.MappedDrives {
			drives = append(drives, map[string]string{"letter": d.Letter, "path": d.Path})
		}
		return mesh.Observation{Hostname: s.Hostname, User: s.User, OS: s.OS, CPU: s.CPU, GPU: s.GPU, Memory: s.Memory, Frozen: s.Frozen, Limited: s.MetricsLimited, LastInputAgeMS: s.LastInputAgeMs, MappedDrives: drives}
	}
	cache, err := mesh.OpenReleaseCache(dir)
	if err != nil {
		return err
	}
	n.Releases = cache
	configureMeshUpdates(n, dir, exe)
	n.Execute = func(jobContext context.Context, c mesh.Command) mesh.Result {
		if c.Kind == "self_update" {
			return stageMeshUpdate(jobContext, n, c, dir, exe)
		}
		payload := map[string]any{}
		for k, v := range c.Payload {
			payload[k] = v
		}
		r := runJob(AssignedJob{ID: c.ID, Kind: c.Kind, Payload: payload})
		result := mesh.Result{Status: r.Status, Message: r.Message, Output: r.Output, Via: r.Via, Installed: r.Installed}
		for _, i := range r.Installations {
			result.Installations = append(result.Installations, mesh.Installation{Name: i.Name, Version: i.Version})
		}
		return result
	}
	go monitorDrives()
	if discoverPort > 0 {
		go func() {
			if err := n.Discover(ctx, discoverPort); err != nil {
				log.Printf("discovery unavailable (saved peers remain usable): %v", err)
			}
		}()
	}
	log.Printf("mesh %s node=%s dashboard=%s", meshVersion, self.ID, self.DashboardURL)
	return n.Start(ctx, net.JoinHostPort(listen, fmt.Sprint(uiPort)), net.JoinHostPort(listen, fmt.Sprint(peerPort)))
}
