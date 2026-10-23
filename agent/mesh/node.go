package mesh

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

type PeerState struct {
	Member      Member      `json:"member"`
	LastSeen    int64       `json:"lastSeen"`
	LastSync    int64       `json:"lastSync"`
	Error       string      `json:"error,omitempty"`
	ClockSkew   bool        `json:"clockSkew"`
	Observation Observation `json:"observation"`
	Kinds       []string    `json:"kinds"`
	Instance    string      `json:"instance"`
	busy        bool
	next        time.Time
	failures    int
}

type Node struct {
	Store          *Store
	Self           Member
	Execute        func(context.Context, Command) Result
	Observe        func() Observation
	UI             http.Handler
	mu             sync.RWMutex
	peers          map[string]*PeerState
	observation    Observation
	instance       string
	certificate    tls.Certificate
	clockHold      bool
	lastWall       time.Time
	lastMono       time.Time
	ctx            context.Context
	cancel         context.CancelFunc
	wg             sync.WaitGroup
	OnStagedUpdate func(Command) error
	PendingUpdate  string
	Releases       *ReleaseCache
	Version        string
	BinaryHash     string
	duplicate      bool
	clients        map[string]*http.Client
}

func NewNode(s *Store, self Member) (*Node, error) {
	cert, err := s.Identity.certificate()
	if err != nil {
		return nil, err
	}
	id := make([]byte, 16)
	if _, err = rand.Read(id); err != nil {
		return nil, err
	}
	n := &Node{Store: s, Self: self, certificate: cert, peers: map[string]*PeerState{}, clients: map[string]*http.Client{}, instance: hex.EncodeToString(id), lastWall: time.Now(), lastMono: time.Now()}
	n.ctx, n.cancel = context.WithCancel(context.Background())
	return n, nil
}
func (n *Node) Done() <-chan struct{} { return n.ctx.Done() }

func (n *Node) clockUncertain() bool {
	n.mu.RLock()
	defer n.mu.RUnlock()
	if n.clockHold {
		return true
	}
	for _, p := range n.peers {
		if nowMS()-p.LastSeen < 20000 && p.ClockSkew {
			return true
		}
	}
	return false
}
func (n *Node) observe() {
	if n.Observe == nil {
		return
	}
	o := n.Observe()
	n.mu.Lock()
	n.observation = o
	now := time.Now()
	wall := now.UnixMilli() - n.lastWall.UnixMilli()
	elapsed := now.Sub(n.lastMono).Milliseconds()
	if abs(wall-elapsed) > 30000 {
		n.clockHold = true
	}
	n.lastWall = now
	n.lastMono = now
	n.mu.Unlock()
}
func abs(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}

func (n *Node) Start(ctx context.Context, uiAddr, peerAddr string) error {
	if err := n.Store.RecoverExcept(n.PendingUpdate); err != nil {
		return err
	}
	peerListener, err := net.Listen("tcp", peerAddr)
	if err != nil {
		return err
	}
	uiListener, err := net.Listen("tcp", uiAddr)
	if err != nil {
		peerListener.Close()
		return err
	}
	peerTLS := &tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{n.certificate}, ClientAuth: tls.RequireAnyClientCert}
	// Use application-pinned self-signed node certificates. The TLS handshake
	// proves key possession; every request additionally checks current membership.
	peerTLS.VerifyPeerCertificate = nil
	peerServer := &http.Server{Handler: n.PeerHandler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16384}
	uiServer := &http.Server{Handler: n.BrowserHandler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16384}
	n.wg.Add(2)
	go func() {
		defer n.wg.Done()
		if err := peerServer.Serve(tls.NewListener(peerListener, peerTLS)); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("peer listener: %v", err)
			n.cancel()
		}
	}()
	go func() {
		defer n.wg.Done()
		if err := uiServer.Serve(uiListener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("browser listener: %v", err)
			n.cancel()
		}
	}()
	n.wg.Add(3)
	go n.monitor()
	go n.reconcile()
	go n.worker()
	select {
	case <-ctx.Done():
	case <-n.ctx.Done():
	}
	n.cancel()
	shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = peerServer.Shutdown(shutdown)
	_ = uiServer.Shutdown(shutdown)
	n.wg.Wait()
	n.mu.Lock()
	for _, client := range n.clients {
		client.CloseIdleConnections()
	}
	n.mu.Unlock()
	return nil
}

func (n *Node) monitor() {
	defer n.wg.Done()
	tick := time.NewTicker(5 * time.Second)
	defer tick.Stop()
	lastInventory := time.Time{}
	for {
		n.observe()
		if time.Since(lastInventory) > time.Minute && n.Store.Active(n.Self.ID) {
			n.mu.RLock()
			o := n.observation
			n.mu.RUnlock()
			if err := n.persistInventory(o); err != nil {
				log.Printf("inventory: %v", err)
			}
			lastInventory = time.Now()
		}
		select {
		case <-n.ctx.Done():
			return
		case <-tick.C:
		}
	}
}

func (n *Node) persistInventory(o Observation) error {
	// Resource samples travel in live hellos, not the permanent event history.
	o.CPU, o.GPU, o.Memory, o.LastInputAgeMS = nil, nil, nil, nil
	o.Frozen, o.Limited = false, false
	var previous []byte
	err := n.Store.db.QueryRow("SELECT body FROM inventory WHERE id=?", n.Self.ID).Scan(&previous)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if bytes.Equal(previous, encode(o)) {
		return nil
	}
	_, err = n.Store.Append("inventory", o)
	return err
}

func (n *Node) reconcile() {
	defer n.wg.Done()
	tick := time.NewTicker(2 * time.Second)
	defer tick.Stop()
	for {
		members, err := n.Store.Members()
		if err == nil {
			for _, m := range members {
				if m.ID == n.Self.ID || m.Removed || m.PeerURL == "" {
					continue
				}
				n.mu.Lock()
				p := n.peers[m.ID]
				if p == nil {
					p = &PeerState{Member: m}
					n.peers[m.ID] = p
				}
				if p.busy || time.Now().Before(p.next) {
					n.mu.Unlock()
					continue
				}
				p.busy = true
				n.mu.Unlock()
				n.wg.Add(1)
				go func(m Member) {
					defer n.wg.Done()
					err := n.Sync(n.ctx, m)
					n.mu.Lock()
					defer n.mu.Unlock()
					p := n.peers[m.ID]
					p.busy = false
					if err != nil {
						p.Error = err.Error()
						p.failures++
						shift := min(p.failures, 4)
						jitter := time.Duration(time.Now().UnixNano()%1000) * time.Millisecond
						p.next = time.Now().Add(time.Duration(1<<shift)*time.Second + jitter)
					} else {
						p.Error = ""
						p.failures = 0
						p.next = time.Time{}
					}
				}(m)
			}
		}
		if n.Store.Active(n.Self.ID) {
			commands, err := n.Store.queuedCommands()
			if err == nil {
				for _, c := range commands {
					if c.Target == n.Self.ID {
						if _, err = n.Store.Accept(c.ID, n.clockUncertain()); err != nil {
							log.Printf("deferred acceptance: %v", err)
						}
					}
				}
			}
		}
		_, _ = n.Store.db.Exec("DELETE FROM outputs WHERE expires<?", nowMS())
		select {
		case <-n.ctx.Done():
			return
		case <-tick.C:
		}
	}
}

func (n *Node) worker() {
	defer n.wg.Done()
	tick := time.NewTicker(500 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-n.ctx.Done():
			return
		default:
		}
		n.mu.RLock()
		duplicate := n.duplicate
		n.mu.RUnlock()
		if duplicate {
			select {
			case <-n.ctx.Done():
				return
			case <-tick.C:
			}
			continue
		}
		c, err := n.Store.Next(n.clockUncertain())
		if err != nil {
			log.Printf("inbox: %v", err)
		}
		if c != nil {
			result := Result{Status: "error", Message: "Executor unavailable"}
			secret, openErr := n.Store.Identity.open(c.Secret)
			if openErr != nil {
				result.Message = openErr.Error()
			} else if n.Execute != nil {
				execution := *c
				execution.Payload = map[string]string{}
				for k, v := range c.Payload {
					execution.Payload[k] = v
				}
				if secret != "" {
					execution.Payload["password"] = secret
				}
				result = n.Execute(n.ctx, execution)
			}
			if secret != "" {
				result.Output = strings.ReplaceAll(result.Output, secret, "[redacted]")
				result.Message = strings.ReplaceAll(result.Message, secret, "[redacted]")
			}
			if result.Status == "staged" && n.OnStagedUpdate != nil {
				if err = n.OnStagedUpdate(*c); err == nil {
					// Keep the executor occupied until the helper stops us or a
					// durable receipt records success, rollback, or timeout.
					for {
						select {
						case <-n.ctx.Done():
							return
						case <-tick.C:
						}
						r, e := n.Store.Receipt(c.ID)
						if e == nil && terminal(r.Status) {
							break
						}
					}
					continue
				}
				result = Result{Status: "error", Message: err.Error()}
			}
			if err = n.Store.Complete(*c, result); err != nil {
				log.Printf("result persistence failed; execution will not be repeated: %v", err)
			}
			continue
		}
		select {
		case <-n.ctx.Done():
			return
		case <-tick.C:
		}
	}
}

func privateDial(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	for _, a := range ips {
		if !a.IP.IsPrivate() && !a.IP.IsLoopback() && !a.IP.IsLinkLocalUnicast() {
			continue
		}
		d := net.Dialer{Timeout: 5 * time.Second}
		c, err := d.DialContext(ctx, network, net.JoinHostPort(a.IP.String(), port))
		if err == nil {
			return c, nil
		}
	}
	return nil, errors.New("peer must be reachable on a private LAN address")
}
func validEndpoint(raw, scheme string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != scheme || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return errors.New("invalid LAN endpoint")
	}
	p, err := strconv.Atoi(u.Port())
	if err != nil || p < 1 || p > 65535 {
		return errors.New("endpoint requires valid port")
	}
	return nil
}
func (n *Node) client(id string) *http.Client {
	n.mu.Lock()
	defer n.mu.Unlock()
	if client := n.clients[id]; client != nil {
		return client
	}
	client := &http.Client{Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }, Transport: &http.Transport{DialContext: privateDial, TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{n.certificate}, InsecureSkipVerify: true, VerifyConnection: func(cs tls.ConnectionState) error {
		raw := [][]byte{}
		for _, c := range cs.PeerCertificates {
			raw = append(raw, c.Raw)
		}
		got, err := peerIdentity(raw)
		if err != nil {
			return err
		}
		if got != id {
			return errors.New("peer fingerprint mismatch")
		}
		return nil
	}}, MaxIdleConnsPerHost: 2, IdleConnTimeout: 10 * time.Second}}
	n.clients[id] = client
	return client
}
func (n *Node) request(ctx context.Context, m Member, path string, input, output any) error {
	if err := validEndpoint(m.PeerURL, "https"); err != nil {
		return err
	}
	method := http.MethodGet
	var body io.Reader
	if input != nil {
		method = http.MethodPost
		body = bytes.NewReader(encode(input))
	}
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(m.PeerURL, "/")+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Fleet-Instance", n.instance)
	client := n.client(m.ID)
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	data, err := io.ReadAll(io.LimitReader(res.Body, MaxBody+1))
	if err != nil {
		return err
	}
	if len(data) > MaxBody {
		return errors.New("peer response too large")
	}
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("peer HTTP %d: %s", res.StatusCode, string(data))
	}
	if output != nil {
		return json.Unmarshal(data, output)
	}
	return nil
}

func (n *Node) Sync(ctx context.Context, m Member) error {
	if !n.Store.Active(m.ID) || !n.Store.Active(n.Self.ID) {
		return errors.New("inactive identity")
	}
	var hello struct {
		Hello
		Instance string `json:"instance"`
	}
	if err := n.request(ctx, m, "/peer/v2/hello", nil, &hello); err != nil {
		return err
	}
	if hello.FleetID != n.Store.FleetID || hello.Member.ID != m.ID || hello.Protocol != Protocol {
		return errors.New("peer protocol, identity, or fleet mismatch")
	}
	n.mu.Lock()
	p := n.peers[m.ID]
	if p == nil {
		p = &PeerState{}
		n.peers[m.ID] = p
	}
	p.Member = m
	p.LastSeen = nowMS()
	p.Observation = hello.Observation
	p.Kinds = hello.Kinds
	p.ClockSkew = abs(hello.Time-nowMS()) > 30000
	p.Instance = hello.Instance
	n.mu.Unlock()
	for round := 0; round < 32; round++ {
		cursors, err := n.Store.Cursors()
		if err != nil {
			return err
		}
		var incoming Envelope
		if err = n.request(ctx, m, "/peer/v2/pull", cursors, &incoming); err != nil {
			return err
		}
		if len(incoming.Events) > 0 {
			if _, err = n.Store.Ingest(incoming); err != nil {
				return err
			}
		}
		outgoing, err := n.Store.Export(hello.Cursors, MaxBatch)
		if err != nil {
			return err
		}
		if len(outgoing.Events) > 0 {
			var ack struct {
				IDs     []string         `json:"ids"`
				Cursors map[string]int64 `json:"cursors"`
			}
			if err = n.request(ctx, m, "/peer/v2/push", outgoing, &ack); err != nil {
				return err
			}
			hello.Cursors = ack.Cursors
			if err = n.Store.RememberReplicas(m.ID, ack.IDs); err != nil {
				return err
			}
		}
		if len(incoming.Events) == 0 && len(outgoing.Events) == 0 {
			break
		}
	}
	// Existing records on a peer also count as copies after reconnect, not only
	// newly pushed events. Cursors are contiguous because gaps cannot commit.
	if err := n.Store.transaction(func(tx *sql.Tx) error {
		for origin, seq := range hello.Cursors {
			if _, err := tx.Exec("INSERT OR IGNORE INTO replicas SELECT id,? FROM events WHERE origin=? AND seq<=?", m.ID, origin, seq); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return err
	}
	if err := n.syncOutputs(ctx, m); err != nil {
		return err
	}
	n.mu.Lock()
	n.peers[m.ID].LastSync = nowMS()
	n.mu.Unlock()
	return nil
}

func (n *Node) Deliver(ctx context.Context, e Event) error {
	c, err := decode[Command](e.Body)
	if err != nil {
		return err
	}
	if c.Target == n.Self.ID {
		_, err = n.Store.Accept(c.ID, n.clockUncertain())
		return err
	}
	m, err := n.Store.Member(c.Target)
	if err != nil || m.Removed {
		return errors.New("target unavailable")
	}
	// Sync establishes causal dependencies but cannot execute an online command.
	if err = n.Sync(ctx, m); err != nil {
		_, _ = n.Store.db.Exec("INSERT OR REPLACE INTO delivery VALUES (?,?)", c.ID, "unavailable")
		return err
	}
	_, _ = n.Store.db.Exec("INSERT OR REPLACE INTO delivery VALUES (?,?)", c.ID, "acceptance_unknown")
	var r Receipt
	if err = n.request(ctx, m, "/peer/v2/accept", map[string]string{"id": c.ID}, &r); err != nil {
		return err
	}
	// The signed receipt arrives through replication; HTTP acknowledgment alone
	// is never stored as another machine's authoritative receipt.
	return n.Sync(ctx, m)
}

func (n *Node) syncOutputs(ctx context.Context, m Member) error {
	// Retry missing blobs independently of event cursors: the receipt may have
	// committed before a previous output request failed.
	rows, err := n.Store.db.Query(`SELECT r.body FROM receipts r LEFT JOIN outputs o
		ON o.hash=json_extract(r.body,'$.outputHash')
		WHERE o.hash IS NULL AND json_extract(r.body,'$.outputHash') <> ''
		AND json_extract(r.body,'$.updatedAt') > ? LIMIT ?`, nowMS()-int64(90*24*time.Hour/time.Millisecond), MaxBatch)
	if err != nil {
		return err
	}
	var pending []Receipt
	for rows.Next() {
		var body []byte
		if err = rows.Scan(&body); err != nil {
			rows.Close()
			return err
		}
		r, err := decode[Receipt](body)
		if err != nil {
			rows.Close()
			return err
		}
		pending = append(pending, r)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, r := range pending {
		if r.OutputHash == "" || r.UpdatedAt+int64(90*24*time.Hour/time.Millisecond) <= nowMS() {
			continue
		}
		var exists int
		if err = n.Store.db.QueryRow("SELECT COUNT(*) FROM outputs WHERE hash=?", r.OutputHash).Scan(&exists); err != nil {
			return err
		}
		if exists > 0 {
			continue
		}
		var blob struct {
			Output string `json:"output"`
		}
		if err = n.request(ctx, m, "/peer/v2/output/"+r.OutputHash, nil, &blob); err != nil {
			continue
		}
		if _, err = n.Store.Ingest(Envelope{Outputs: map[string]string{r.OutputHash: blob.Output}}); err != nil {
			return err
		}
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func failHTTP(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}
func readJSON(w http.ResponseWriter, r *http.Request, v any) error {
	r.Body = http.MaxBytesReader(w, r.Body, MaxBody)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		return err
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return errors.New("one JSON value required")
	}
	return nil
}

func (n *Node) PeerHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.TLS == nil || len(r.TLS.PeerCertificates) != 1 {
			failHTTP(w, 401, errors.New("node certificate required"))
			return
		}
		id, err := peerIdentity([][]byte{r.TLS.PeerCertificates[0].Raw})
		if err != nil {
			failHTTP(w, 403, errors.New("invalid node identity"))
			return
		}
		if id == n.Self.ID {
			if r.Header.Get("X-Fleet-Instance") != n.instance {
				n.mu.Lock()
				n.duplicate = true
				n.mu.Unlock()
			}
			if r.URL.Path == "/peer/v2/identity" && r.Method == "GET" {
				writeJSON(w, 200, map[string]string{"instance": n.instance})
				return
			}
			failHTTP(w, 403, errors.New("duplicate node identity; execution quarantined"))
			return
		}
		if r.URL.Path == "/peer/v2/join" {
			n.handleJoin(w, r, id)
			return
		}
		if !n.Store.Active(id) || !n.Store.Active(n.Self.ID) {
			failHTTP(w, 403, errors.New("identity not enrolled or removed"))
			return
		}
		switch r.URL.Path {
		case "/peer/v2/hello":
			if r.Method != "GET" {
				w.WriteHeader(405)
				return
			}
			cursors, err := n.Store.Cursors()
			if err != nil {
				failHTTP(w, 500, err)
				return
			}
			n.mu.RLock()
			o := n.observation
			n.mu.RUnlock()
			writeJSON(w, 200, struct {
				Hello
				Instance string `json:"instance"`
			}{Hello{Protocol, n.Store.FleetID, n.Self, nowMS(), Kinds, cursors, o}, n.instance})
		case "/peer/v2/pull":
			if r.Method != "POST" {
				w.WriteHeader(405)
				return
			}
			var cursors map[string]int64
			if err = readJSON(w, r, &cursors); err != nil {
				failHTTP(w, 400, err)
				return
			}
			env, err := n.Store.Export(cursors, MaxBatch)
			if err != nil {
				failHTTP(w, 500, err)
				return
			}
			writeJSON(w, 200, env)
		case "/peer/v2/push":
			if r.Method != "POST" {
				w.WriteHeader(405)
				return
			}
			var env Envelope
			if err = readJSON(w, r, &env); err != nil {
				failHTTP(w, 400, err)
				return
			}
			ids, err := n.Store.Ingest(env)
			if err != nil {
				failHTTP(w, 409, err)
				return
			}
			cursors, err := n.Store.Cursors()
			if err != nil {
				failHTTP(w, 500, err)
				return
			}
			writeJSON(w, 200, map[string]any{"ids": ids, "cursors": cursors})
		case "/peer/v2/accept":
			n.mu.RLock()
			duplicate := n.duplicate
			n.mu.RUnlock()
			if duplicate {
				failHTTP(w, 409, errors.New("duplicate identity quarantined"))
				return
			}
			if r.Method != "POST" {
				w.WriteHeader(405)
				return
			}
			var input struct {
				ID string `json:"id"`
			}
			if err = readJSON(w, r, &input); err != nil {
				failHTTP(w, 400, err)
				return
			}
			receipt, err := n.Store.Accept(input.ID, n.clockUncertain())
			if err != nil {
				failHTTP(w, 409, err)
				return
			}
			writeJSON(w, 200, receipt)
		default:
			if strings.HasPrefix(r.URL.Path, "/peer/v2/releases/") {
				n.serveRelease(w, r)
				return
			}
			if strings.HasPrefix(r.URL.Path, "/peer/v2/output/") && r.Method == "GET" {
				hash := strings.TrimPrefix(r.URL.Path, "/peer/v2/output/")
				if !hashPattern.MatchString(hash) {
					http.NotFound(w, r)
					return
				}
				var output string
				if err = n.Store.db.QueryRow("SELECT body FROM outputs WHERE hash=? AND expires>?", hash, nowMS()).Scan(&output); err != nil {
					http.NotFound(w, r)
					return
				}
				writeJSON(w, 200, map[string]string{"output": output})
				return
			}
			http.NotFound(w, r)
		}
	})
}
