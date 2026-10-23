// Package mesh implements a local-first fleet. Replication never executes jobs;
// only a target's durable inbox grants permission to execute on that machine.
package mesh

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"
)

const Protocol = 2
const MaxOutput = 64 * 1024
const MaxBatch = 128
const MaxBody = 2 * 1024 * 1024

var Kinds = []string{"check", "install", "uninstall", "map_drive", "unmap_drive", "clean_downloads", "empty_recycle", "launch", "self_update", "diagnostics"}

func validDiagnostic(profile string) bool {
	switch profile {
	case "system", "disks", "network", "processes", "services", "events":
		return true
	}
	return false
}

type Member struct {
	ID            string `json:"id"`
	PublicKey     []byte `json:"publicKey"`
	EncryptionKey []byte `json:"encryptionKey"`
	Hostname      string `json:"hostname"`
	PeerURL       string `json:"peerUrl"`
	DashboardURL  string `json:"dashboardUrl"`
	LegacyID      string `json:"legacyId,omitempty"`
	Removed       bool   `json:"removed"`
}

type Event struct {
	ID           string          `json:"id"`
	FleetID      string          `json:"fleetId"`
	Origin       string          `json:"origin"`
	Sequence     int64           `json:"sequence"`
	Lamport      int64           `json:"lamport"`
	Version      int             `json:"version"`
	Kind         string          `json:"kind"`
	CreatedAt    int64           `json:"createdAt"`
	Dependencies []string        `json:"dependencies"`
	Body         json.RawMessage `json:"body"`
	PublicKey    []byte          `json:"publicKey"`
	Signature    []byte          `json:"signature"`
}

type Command struct {
	ID        string            `json:"id"`
	JobID     string            `json:"jobId"`
	Target    string            `json:"target"`
	Kind      string            `json:"kind"`
	Payload   map[string]string `json:"payload"`
	Secret    []byte            `json:"secret,omitempty"`
	Delivery  string            `json:"delivery"`
	ExpiresAt int64             `json:"expiresAt,omitempty"`
	CreatedAt int64             `json:"createdAt"`
	RetryOf   string            `json:"retryOf,omitempty"`
}

type SubmitRequest struct {
	SubmissionID string            `json:"submissionId"`
	Kind         string            `json:"kind"`
	MachineIDs   []string          `json:"machineIds"`
	Payload      map[string]string `json:"payload"`
	Delivery     string            `json:"delivery"`
	ExpiresAt    int64             `json:"expiresAt,omitempty"`
	RetryOf      string            `json:"retryOf,omitempty"`
}

type Receipt struct {
	CommandID     string         `json:"commandId"`
	Target        string         `json:"target"`
	Status        string         `json:"status"`
	Message       string         `json:"message"`
	Via           string         `json:"via,omitempty"`
	OutputHash    string         `json:"outputHash,omitempty"`
	UpdatedAt     int64          `json:"updatedAt"`
	Installed     *bool          `json:"installed,omitempty"`
	Installations []Installation `json:"installations,omitempty"`
}

type Installation struct {
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
}
type Result struct {
	Status        string         `json:"status"`
	Message       string         `json:"message"`
	Via           string         `json:"via,omitempty"`
	Output        string         `json:"output,omitempty"`
	Installed     *bool          `json:"installed,omitempty"`
	Installations []Installation `json:"installations,omitempty"`
}

type CatalogItem struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Match    string `json:"match"`
	WingetID string `json:"wingetId,omitempty"`
}

type Observation struct {
	Hostname       string              `json:"hostname"`
	User           string              `json:"user"`
	OS             string              `json:"os"`
	CPU            *float64            `json:"cpu"`
	GPU            *float64            `json:"gpu"`
	Memory         *float64            `json:"memory"`
	Frozen         bool                `json:"frozenHint"`
	Limited        bool                `json:"metricsLimited"`
	LastInputAgeMS *int64              `json:"lastInputAgeMs"`
	MappedDrives   []map[string]string `json:"mappedDrives"`
}

type Hello struct {
	Protocol    int              `json:"protocol"`
	FleetID     string           `json:"fleetId"`
	Member      Member           `json:"member"`
	Time        int64            `json:"time"`
	Kinds       []string         `json:"kinds"`
	Cursors     map[string]int64 `json:"cursors"`
	Observation Observation      `json:"observation"`
}

type Envelope struct {
	Events  []Event           `json:"events"`
	Outputs map[string]string `json:"outputs,omitempty"`
}

func digest(b []byte) string { s := sha256.Sum256(b); return hex.EncodeToString(s[:]) }
func commandID(fleet, job, target string) string {
	return digest([]byte(fleet + "/" + job + "/" + target))
}
func encode(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}
func nowMS() int64 { return time.Now().UTC().UnixMilli() }
func validKind(k string) bool {
	for _, v := range Kinds {
		if v == k {
			return true
		}
	}
	return false
}
func terminal(s string) bool            { return s == "ok" || s == "error" || s == "unknown" || s == "expired" }
func decode[T any](b []byte) (T, error) { var v T; err := json.Unmarshal(b, &v); return v, err }
func require(b bool, msg string) error {
	if !b {
		return fmt.Errorf("%s", msg)
	}
	return nil
}
