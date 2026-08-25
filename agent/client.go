package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

type MappedDrive struct {
	Letter string `json:"letter"`
	Path   string `json:"path"`
}

type Heartbeat struct {
	Token          string        `json:"token"`
	MachineID      string        `json:"machineId,omitempty"`
	Hostname       string        `json:"hostname"`
	User           string        `json:"user"`
	OS             string        `json:"os"`
	CPU            *float64      `json:"cpu"`
	Memory         *float64      `json:"memory"`
	Frozen         bool          `json:"frozen"`
	MetricsLimited bool          `json:"metricsLimited"`
	LastInputAgeMs *int64        `json:"lastInputAgeMs"`
	MappedDrives   []MappedDrive `json:"mappedDrives"`
	Results        []JobResult   `json:"results,omitempty"`
}

type AssignedJob struct {
	ID      string         `json:"id"`
	Kind    string         `json:"kind"`
	Payload map[string]any `json:"payload"`
}

type JobResult struct {
	JobID   string `json:"jobId"`
	Status  string `json:"status"`
	Via     string `json:"via,omitempty"`
	Message string `json:"message"`
	Output  string `json:"output,omitempty"`
}

type PollResponse struct {
	OK        bool          `json:"ok"`
	Error     string        `json:"error"`
	MachineID string        `json:"machineId"`
	Jobs      []AssignedJob `json:"jobs"`
}

type Client struct {
	Server    string
	Token     string
	HTTPOnly  bool
	MachineID string
	OnID      func(string)
	pending   []JobResult
	wsFails   int
}

func (c *Client) RunOnce() error {
	if !c.HTTPOnly && c.wsFails < 2 {
		err := c.runWS()
		if err != nil {
			c.wsFails++
			return fmt.Errorf("websocket: %w (falling back to HTTP poll after failures)", err)
		}
		c.wsFails = 0
		return nil
	}
	return c.runPoll()
}

func (c *Client) collect() Heartbeat {
	snap := collectSnapshot()
	hb := Heartbeat{
		Token:          c.Token,
		MachineID:      c.MachineID,
		Hostname:       snap.Hostname,
		User:           snap.User,
		OS:             snap.OS,
		CPU:            snap.CPU,
		Memory:         snap.Memory,
		Frozen:         snap.Frozen,
		MetricsLimited: snap.MetricsLimited,
		LastInputAgeMs: snap.LastInputAgeMs,
		MappedDrives:   snap.MappedDrives,
	}
	if len(c.pending) > 0 {
		hb.Results = c.pending
		c.pending = nil
	}
	return hb
}

func (c *Client) apply(resp PollResponse) {
	if resp.MachineID != "" && resp.MachineID != c.MachineID {
		c.MachineID = resp.MachineID
		if c.OnID != nil {
			c.OnID(c.MachineID)
		}
	}
	for _, job := range resp.Jobs {
		c.pending = append(c.pending, runJob(job))
	}
}

func (c *Client) runPoll() error {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		if err := c.pollOnce(); err != nil {
			return err
		}
		<-ticker.C
	}
}

func (c *Client) pollOnce() error {
	body, _ := json.Marshal(c.collect())
	req, err := http.NewRequest(http.MethodPost, c.Server+"/api/agent/poll", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("content-type", "application/json")
	req.Header.Set("x-agent-transport", "poll")
	httpClient := &http.Client{Timeout: 20 * time.Second}
	res, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	var resp PollResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return fmt.Errorf("poll decode: %w", err)
	}
	if !resp.OK {
		return fmt.Errorf("poll: %s", resp.Error)
	}
	c.apply(resp)
	c.maybeRestart()
	return nil
}

func (c *Client) maybeRestart() {
	if !restartRequested {
		return
	}
	restartRequested = false
	_ = c.pollOnceNoRestart()
	spawnRestart(c)
	os.Exit(0)
}

func (c *Client) pollOnceNoRestart() error {
	body, _ := json.Marshal(c.collect())
	req, err := http.NewRequest(http.MethodPost, c.Server+"/api/agent/poll", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("content-type", "application/json")
	req.Header.Set("x-agent-transport", "poll")
	httpClient := &http.Client{Timeout: 20 * time.Second}
	res, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	io.Copy(io.Discard, res.Body)
	return nil
}

func (c *Client) runWS() error {
	u, err := url.Parse(c.Server)
	if err != nil {
		return err
	}
	if u.Scheme == "https" {
		u.Scheme = "wss"
	} else {
		u.Scheme = "ws"
	}
	u.Path = "/api/agent/ws"
	u.RawQuery = ""

	dialer := websocket.Dialer{HandshakeTimeout: 8 * time.Second}
	conn, _, err := dialer.Dial(u.String(), nil)
	if err != nil {
		return err
	}
	defer conn.Close()

	_ = conn.SetReadDeadline(time.Now().Add(30 * time.Second))
	conn.SetPongHandler(func(string) error {
		_ = conn.SetReadDeadline(time.Now().Add(30 * time.Second))
		return nil
	})

	errCh := make(chan error, 1)
	go func() {
		for {
			_, data, err := conn.ReadMessage()
			if err != nil {
				errCh <- err
				return
			}
			var resp PollResponse
			if err := json.Unmarshal(data, &resp); err != nil {
				continue
			}
			if !resp.OK {
				errCh <- fmt.Errorf("ws: %s", resp.Error)
				return
			}
			c.apply(resp)
			if restartRequested {
				restartRequested = false
				_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
				b, _ := json.Marshal(c.collect())
				_ = conn.WriteMessage(websocket.TextMessage, b)
				spawnRestart(c)
				os.Exit(0)
			}
		}
	}()

	send := func() error {
		b, _ := json.Marshal(c.collect())
		_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
		return conn.WriteMessage(websocket.TextMessage, b)
	}
	if err := send(); err != nil {
		return err
	}
	tick := time.NewTicker(5 * time.Second)
	defer tick.Stop()
	ping := time.NewTicker(20 * time.Second)
	defer ping.Stop()
	for {
		select {
		case err := <-errCh:
			return err
		case <-tick.C:
			if err := send(); err != nil {
				return err
			}
		case <-ping.C:
			_ = conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
			if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return err
			}
		}
	}
}

func payloadString(p map[string]any, key string) string {
	if p == nil {
		return ""
	}
	v, ok := p[key]
	if !ok || v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t)
	default:
		return strings.TrimSpace(fmt.Sprint(t))
	}
}
