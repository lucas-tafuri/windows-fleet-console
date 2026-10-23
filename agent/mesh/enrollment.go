package mesh

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

func (n *Node) handleJoin(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != "POST" {
		w.WriteHeader(405)
		return
	}
	if !n.Store.Active(n.Self.ID) {
		failHTTP(w, 403, errors.New("node is not an active fleet member"))
		return
	}
	var m Member
	if err := readJSON(w, r, &m); err != nil {
		failHTTP(w, 400, err)
		return
	}
	if m.ID != id || m.ID != digest(m.PublicKey) || len(m.PublicKey) != 32 || len(m.EncryptionKey) != 32 || m.Hostname == "" || len(m.Hostname) > 255 {
		failHTTP(w, 400, errors.New("join identity does not match TLS identity"))
		return
	}
	if err := validEndpoint(m.PeerURL, "https"); err != nil {
		failHTTP(w, 400, err)
		return
	}
	if err := validEndpoint(m.DashboardURL, "http"); err != nil {
		failHTTP(w, 400, err)
		return
	}
	if existing, err := n.Store.Member(id); err == nil {
		if existing.Removed {
			failHTTP(w, 403, errors.New("identity permanently removed; re-enroll with a new identity"))
			return
		}
		cursors, err := n.Store.Cursors()
		if err != nil {
			failHTTP(w, 500, err)
			return
		}
		writeJSON(w, 200, map[string]any{"status": "approved", "fleetId": n.Store.FleetID, "cursors": cursors})
		return
	}
	_, _ = n.Store.db.Exec("DELETE FROM joins WHERE created<?", nowMS()-int64(10*time.Minute/time.Millisecond))
	var count int
	_ = n.Store.db.QueryRow("SELECT COUNT(*) FROM joins").Scan(&count)
	if count >= 50 {
		failHTTP(w, 429, errors.New("too many pending joins"))
		return
	}
	if _, err := n.Store.db.Exec("INSERT INTO joins VALUES (?,?,?) ON CONFLICT(id) DO NOTHING", m.ID, encode(m), nowMS()); err != nil {
		failHTTP(w, 500, err)
		return
	}
	writeJSON(w, 200, map[string]string{"status": "pending", "fleetId": n.Store.FleetID})
}

func (n *Node) Approve(id string) (Event, error) {
	var b []byte
	if err := n.Store.db.QueryRow("SELECT body FROM joins WHERE id=? AND created>?", id, nowMS()-int64(10*time.Minute/time.Millisecond)).Scan(&b); err != nil {
		return Event{}, err
	}
	m, err := decode[Member](b)
	if err != nil {
		return Event{}, err
	}
	if old, err := n.Store.Member(id); err == nil && old.Removed {
		return Event{}, errors.New("removed identity cannot be reapproved")
	}
	e, err := n.Store.Append("member", m)
	if err != nil {
		return e, err
	}
	_, err = n.Store.db.Exec("DELETE FROM joins WHERE id=?", id)
	return e, err
}
func (n *Node) decideJoinHTTP(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != "POST" {
		w.WriteHeader(405)
		return
	}
	var req struct {
		Action string `json:"action"`
	}
	if err := readJSON(w, r, &req); err != nil {
		failHTTP(w, 400, err)
		return
	}
	if req.Action == "approve" {
		if _, err := n.Approve(id); err != nil {
			failHTTP(w, 409, err)
			return
		}
	} else if req.Action == "deny" {
		if _, err := n.Store.db.Exec("DELETE FROM joins WHERE id=?", id); err != nil {
			failHTTP(w, 500, err)
			return
		}
	} else {
		failHTTP(w, 400, errors.New("approve or deny required"))
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// Join pins the operator-selected peer fingerprint before sending any request.
// Approval admits only this public key, never a reusable shared fleet token.
func (n *Node) Join(ctx context.Context, peerURL, fingerprint, fleet string) error {
	if len(fingerprint) != 64 || len(fleet) != 64 {
		return errors.New("peer fingerprint and fleet ID required")
	}
	if n.Store.FleetID != "" && n.Store.FleetID != fleet {
		return errors.New("saved fleet differs from requested fleet")
	}
	m := Member{ID: fingerprint, PeerURL: peerURL}
	for {
		var reply struct {
			Status  string           `json:"status"`
			FleetID string           `json:"fleetId"`
			Cursors map[string]int64 `json:"cursors"`
		}
		if err := n.request(ctx, m, "/peer/v2/join", n.Self, &reply); err != nil {
			return err
		}
		if reply.FleetID != fleet {
			return errors.New("fleet fingerprint mismatch")
		}
		if reply.Status == "approved" {
			break
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
	if n.Store.FleetID == "" {
		n.Store.FleetID = fleet
		if _, err := n.Store.db.Exec("INSERT INTO meta VALUES ('fleet',?)", fleet); err != nil {
			return err
		}
	}
	for {
		cursors, err := n.Store.Cursors()
		if err != nil {
			return err
		}
		var env Envelope
		if err = n.request(ctx, m, "/peer/v2/pull", cursors, &env); err != nil {
			return err
		}
		if len(env.Events) == 0 {
			break
		}
		if _, err = n.Store.Ingest(env); err != nil {
			return err
		}
	}
	if !n.Store.Active(n.Self.ID) {
		return errors.New("approval not present after synchronization")
	}
	return n.Store.Address(m)
}

type Discovery struct {
	Protocol int    `json:"protocol"`
	Query    bool   `json:"query"`
	FleetID  string `json:"fleetId"`
	Member   Member `json:"member"`
}

func broadcasts() []net.IP {
	out := []net.IP{}
	interfaces, _ := net.Interfaces()
	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addresses, _ := iface.Addrs()
		for _, a := range addresses {
			ip, network, err := net.ParseCIDR(a.String())
			if err != nil || ip.To4() == nil || !ip.IsPrivate() {
				continue
			}
			v4 := ip.To4()
			mask := network.Mask
			if len(mask) != 4 {
				continue
			}
			broadcast := make(net.IP, 4)
			for i := range broadcast {
				broadcast[i] = v4[i] | ^mask[i]
			}
			out = append(out, broadcast)
		}
	}
	return out
}

func (n *Node) Discover(ctx context.Context, port int) error {
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{Port: port})
	if err != nil {
		return err
	}
	go func() { <-ctx.Done(); conn.Close() }()
	go func() {
		tick := time.NewTicker(10 * time.Second)
		defer tick.Stop()
		for {
			query := encode(Discovery{Protocol: Protocol, Query: true, FleetID: n.Store.FleetID, Member: n.Self})
			for _, ip := range broadcasts() {
				_, _ = conn.WriteToUDP(query, &net.UDPAddr{IP: ip, Port: port})
			}
			select {
			case <-ctx.Done():
				return
			case <-tick.C:
			}
		}
	}()
	buffer := make([]byte, 8192)
	for {
		size, remote, err := conn.ReadFromUDP(buffer)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		if !remote.IP.IsPrivate() && !remote.IP.IsLoopback() {
			continue
		}
		d, err := decode[Discovery](buffer[:size])
		if err != nil || d.Protocol != Protocol || d.FleetID != n.Store.FleetID {
			continue
		}
		if d.Member.ID == n.Self.ID {
			local := false
			addresses, _ := net.InterfaceAddrs()
			for _, a := range addresses {
				ip, _, _ := net.ParseCIDR(a.String())
				if ip != nil && ip.Equal(remote.IP) {
					local = true
				}
			}
			if local {
				continue
			}
			u, err := url.Parse(d.Member.PeerURL)
			if err != nil || u.Port() == "" {
				continue
			}
			candidate := n.Self
			candidate.PeerURL = "https://" + net.JoinHostPort(remote.IP.String(), u.Port())
			probe, cancel := context.WithTimeout(ctx, 3*time.Second)
			var identity struct {
				Instance string `json:"instance"`
			}
			err = n.request(probe, candidate, "/peer/v2/identity", nil, &identity)
			cancel()
			if err == nil && identity.Instance != n.instance {
				n.mu.Lock()
				n.duplicate = true
				n.mu.Unlock()
			}
			continue
		}
		if d.Query {
			_, _ = conn.WriteToUDP(encode(Discovery{Protocol: Protocol, FleetID: n.Store.FleetID, Member: n.Self}), remote)
		}
		if !n.Store.Active(d.Member.ID) {
			continue
		}
		// A broadcast cannot overwrite a trusted address. Probe the sender's LAN
		// address using the already admitted public-key fingerprint first.
		peerURL, err := url.Parse(d.Member.PeerURL)
		if err != nil {
			continue
		}
		peerPort, err := strconv.Atoi(peerURL.Port())
		if err != nil || peerPort < 1 || peerPort > 65535 {
			continue
		}
		candidate, err := n.Store.Member(d.Member.ID)
		if err != nil {
			continue
		}
		candidate.PeerURL = "https://" + net.JoinHostPort(remote.IP.String(), strconv.Itoa(peerPort))
		probeCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		var hello struct {
			Hello
			Instance string `json:"instance"`
		}
		err = n.request(probeCtx, candidate, "/peer/v2/hello", nil, &hello)
		cancel()
		if err == nil && hello.FleetID == n.Store.FleetID && hello.Member.ID == candidate.ID {
			u, e := url.Parse(hello.Member.DashboardURL)
			if e == nil && u.Port() != "" {
				candidate.DashboardURL = "http://" + net.JoinHostPort(remote.IP.String(), u.Port())
			}
			_ = n.Store.Address(candidate)
		}
	}
}

func LANAddress() string {
	addresses, _ := net.InterfaceAddrs()
	for _, a := range addresses {
		ip, _, err := net.ParseCIDR(a.String())
		if err == nil && ip.To4() != nil && ip.IsPrivate() {
			return ip.String()
		}
	}
	return "127.0.0.1"
}
func Endpoint(host string, port int, tls bool) string {
	scheme := "http"
	if tls {
		scheme = "https"
	}
	return fmt.Sprintf("%s://%s", scheme, net.JoinHostPort(host, strconv.Itoa(port)))
}
