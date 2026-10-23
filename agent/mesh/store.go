package mesh

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

type Store struct {
	db       *sql.DB
	mu       sync.Mutex
	Identity *Identity
	FleetID  string
	Dir      string
}

const schema = `
CREATE TABLE IF NOT EXISTS meta (key TEXT PRIMARY KEY, value TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS events (id TEXT PRIMARY KEY, origin TEXT NOT NULL, seq INTEGER NOT NULL, lamport INTEGER NOT NULL, kind TEXT NOT NULL, body BLOB NOT NULL, UNIQUE(origin,seq));
CREATE TABLE IF NOT EXISTS members (id TEXT PRIMARY KEY, event_id TEXT NOT NULL, body BLOB NOT NULL, removed INTEGER NOT NULL DEFAULT 0);
CREATE TABLE IF NOT EXISTS removals (id TEXT PRIMARY KEY);
CREATE TABLE IF NOT EXISTS catalog (id TEXT PRIMARY KEY, event_id TEXT NOT NULL, rank TEXT NOT NULL, body BLOB NOT NULL, removed INTEGER NOT NULL DEFAULT 0);
CREATE TABLE IF NOT EXISTS commands (id TEXT PRIMARY KEY, job_id TEXT NOT NULL, event_id TEXT NOT NULL, target TEXT NOT NULL, body BLOB NOT NULL);
CREATE TABLE IF NOT EXISTS receipts (id TEXT PRIMARY KEY, event_id TEXT NOT NULL, seq INTEGER NOT NULL, body BLOB NOT NULL);
CREATE TABLE IF NOT EXISTS inbox (id TEXT PRIMARY KEY, state TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS outputs (hash TEXT PRIMARY KEY, body TEXT NOT NULL, expires INTEGER NOT NULL);
CREATE TABLE IF NOT EXISTS hidden (id TEXT PRIMARY KEY);
CREATE TABLE IF NOT EXISTS addresses (id TEXT PRIMARY KEY, peer_url TEXT NOT NULL, dashboard_url TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS replicas (event_id TEXT NOT NULL, peer TEXT NOT NULL, PRIMARY KEY(event_id,peer));
CREATE TABLE IF NOT EXISTS submissions (id TEXT PRIMARY KEY, digest TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS delivery (id TEXT PRIMARY KEY, status TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS inventory (id TEXT PRIMARY KEY, seq INTEGER NOT NULL, body BLOB NOT NULL);
CREATE TABLE IF NOT EXISTS imports (id TEXT PRIMARY KEY, body BLOB NOT NULL);
CREATE TABLE IF NOT EXISTS joins (id TEXT PRIMARY KEY, body BLOB NOT NULL, created INTEGER NOT NULL);
PRAGMA user_version=2;
`

func Open(dir string, create bool) (*Store, error) {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	file := filepath.Join(dir, "mesh.db")
	_, statErr := os.Stat(file)
	fresh := errors.Is(statErr, os.ErrNotExist)
	if statErr != nil && !fresh {
		return nil, statErr
	}
	if fresh && !create {
		return nil, errors.New("mesh database missing; use create/join explicitly, or restore a backup")
	}
	if fresh {
		if _, err := os.Stat(filepath.Join(dir, "identity.dat")); err == nil {
			return nil, errors.New("database missing beside an identity; recovery required")
		}
	}
	if err = os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	if err = restrictDirectory(dir); err != nil {
		return nil, fmt.Errorf("protect mesh directory: %w", err)
	}
	i, err := loadIdentity(dir, fresh)
	if err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", filepath.ToSlash(file))
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	s := &Store{db: db, Identity: i, Dir: dir}
	fail := func(err error) (*Store, error) { db.Close(); return nil, err }
	if _, err = db.Exec("PRAGMA busy_timeout=5000; PRAGMA journal_mode=WAL; PRAGMA synchronous=FULL; PRAGMA foreign_keys=ON;"); err != nil {
		return fail(err)
	}
	var check string
	if err = db.QueryRow("PRAGMA quick_check").Scan(&check); err != nil || check != "ok" {
		return fail(errors.New("mesh database failed integrity check; left unchanged"))
	}
	var version int
	if err = db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		return fail(err)
	}
	if !fresh && version != Protocol {
		return fail(fmt.Errorf("unsupported database version %d", version))
	}
	if fresh {
		if _, err = db.Exec(schema); err != nil {
			return fail(err)
		}
	}
	var savedID string
	err = db.QueryRow("SELECT value FROM meta WHERE key='identity'").Scan(&savedID)
	if fresh {
		_, err = db.Exec("INSERT INTO meta VALUES ('identity',?)", i.ID())
	} else if err == nil && savedID != i.ID() {
		err = errors.New("database does not belong to this identity")
	}
	if err != nil {
		return fail(err)
	}
	err = db.QueryRow("SELECT value FROM meta WHERE key='fleet'").Scan(&s.FleetID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fail(err)
	}
	if !fresh && s.FleetID == "" && !create {
		return fail(errors.New("incomplete enrollment; resume using join, do not initialize a new fleet"))
	}
	return s, nil
}
func (s *Store) Close() error { return s.db.Close() }
func (s *Store) transaction(fn func(*sql.Tx) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.BeginTx(context.Background(), nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = fn(tx); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) Bootstrap(member Member) error {
	if s.FleetID != "" {
		return errors.New("fleet already initialized")
	}
	fleet := digest([]byte("PrettyDamnFleet/v2/" + s.Identity.ID()))
	s.FleetID = fleet
	err := s.transaction(func(tx *sql.Tx) error {
		if _, err := tx.Exec("INSERT INTO meta VALUES ('fleet',?)", fleet); err != nil {
			return err
		}
		_, err := s.appendTx(tx, "member", member, nil)
		return err
	})
	if err != nil {
		s.FleetID = ""
	}
	return err
}

func (s *Store) InitializeJoin(fleet string, env Envelope) error {
	if s.FleetID != "" && s.FleetID != fleet {
		return errors.New("different fleet")
	}
	s.FleetID = fleet
	err := s.transaction(func(tx *sql.Tx) error {
		if _, err := tx.Exec("INSERT OR REPLACE INTO meta VALUES ('fleet',?)", fleet); err != nil {
			return err
		}
		for _, e := range env.Events {
			if err := s.insertTx(tx, e); err != nil {
				return err
			}
		}
		_, err := memberTx(tx, s.Identity.ID(), true)
		return err
	})
	if err != nil {
		s.FleetID = ""
	}
	return err
}

func memberTx(tx *sql.Tx, id string, active bool) (Member, error) {
	var body []byte
	var removed bool
	err := tx.QueryRow("SELECT body,removed FROM members WHERE id=?", id).Scan(&body, &removed)
	if err != nil {
		return Member{}, err
	}
	m, err := decode[Member](body)
	m.Removed = removed
	if err == nil && active && removed {
		err = errors.New("node identity removed")
	}
	return m, err
}

func (s *Store) appendTx(tx *sql.Tx, kind string, body any, deps []string) (Event, error) {
	e := Event{FleetID: s.FleetID, Origin: s.Identity.ID(), Version: Protocol, Kind: kind, Body: encode(body), CreatedAt: nowMS(), Dependencies: append([]string{}, deps...)}
	var prev string
	err := tx.QueryRow("SELECT id,seq FROM events WHERE origin=? ORDER BY seq DESC LIMIT 1", e.Origin).Scan(&prev, &e.Sequence)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return e, err
	}
	e.Sequence++
	if prev != "" {
		e.Dependencies = append(e.Dependencies, prev)
	}
	var admission string
	if tx.QueryRow("SELECT event_id FROM members WHERE id=?", e.Origin).Scan(&admission) == nil {
		e.Dependencies = append(e.Dependencies, admission)
	}
	if err = tx.QueryRow("SELECT COALESCE(MAX(lamport),0)+1 FROM events").Scan(&e.Lamport); err != nil {
		return e, err
	}
	sort.Strings(e.Dependencies)
	unique := []string{}
	for _, d := range e.Dependencies {
		if len(unique) == 0 || unique[len(unique)-1] != d {
			unique = append(unique, d)
		}
	}
	e.Dependencies = unique
	s.Identity.sign(&e)
	return e, s.insertTx(tx, e)
}

func (s *Store) Append(kind string, body any, deps ...string) (Event, error) {
	if !s.Active(s.Identity.ID()) {
		return Event{}, errors.New("local node is not an active member")
	}
	var e Event
	err := s.transaction(func(tx *sql.Tx) error { var err error; e, err = s.appendTx(tx, kind, body, deps); return err })
	return e, err
}

func (s *Store) insertTx(tx *sql.Tx, e Event) error {
	if err := e.verify(s.FleetID); err != nil {
		return err
	}
	var exists string
	if tx.QueryRow("SELECT id FROM events WHERE id=?", e.ID).Scan(&exists) == nil {
		return nil
	}
	var last int64
	if err := tx.QueryRow("SELECT COALESCE(MAX(seq),0) FROM events WHERE origin=?", e.Origin).Scan(&last); err != nil {
		return err
	}
	if e.Sequence != last+1 {
		return errors.New("event sequence gap or identity fork")
	}
	if last > 0 {
		var prev string
		if err := tx.QueryRow("SELECT id FROM events WHERE origin=? AND seq=?", e.Origin, last).Scan(&prev); err != nil {
			return err
		}
		found := false
		for _, d := range e.Dependencies {
			if d == prev {
				found = true
			}
		}
		if !found {
			return errors.New("missing origin predecessor")
		}
	}
	for _, id := range e.Dependencies {
		var l int64
		if tx.QueryRow("SELECT lamport FROM events WHERE id=?", id).Scan(&l) != nil || l >= e.Lamport {
			return errors.New("missing or invalid causal dependency")
		}
	}
	_, authorErr := memberTx(tx, e.Origin, false)
	genesis := e.Kind == "member" && e.Sequence == 1 && e.FleetID == digest([]byte("PrettyDamnFleet/v2/"+e.Origin)) && len(e.Dependencies) == 0
	if authorErr != nil && !genesis {
		return errors.New("event author has no admission")
	}
	if _, err := tx.Exec("INSERT INTO events VALUES (?,?,?,?,?,?)", e.ID, e.Origin, e.Sequence, e.Lamport, e.Kind, encode(e)); err != nil {
		return err
	}
	switch e.Kind {
	case "member":
		m, err := decode[Member](e.Body)
		if err != nil {
			return err
		}
		if len(m.PublicKey) != 32 || len(m.EncryptionKey) != 32 || m.ID != digest(m.PublicKey) || len(m.Hostname) > 255 {
			return errors.New("invalid member")
		}
		if genesis && m.ID != e.Origin {
			return errors.New("invalid genesis")
		}
		var removed bool
		_ = tx.QueryRow("SELECT EXISTS(SELECT 1 FROM removals WHERE id=?)", m.ID).Scan(&removed)
		_, err = tx.Exec("INSERT INTO members VALUES (?,?,?,?) ON CONFLICT(id) DO NOTHING", m.ID, e.ID, encode(m), removed)
		return err
	case "remove":
		v, err := decode[struct {
			ID string `json:"id"`
		}](e.Body)
		if err != nil {
			return err
		}
		if _, err = tx.Exec("INSERT OR IGNORE INTO removals VALUES (?)", v.ID); err != nil {
			return err
		}
		_, err = tx.Exec("UPDATE members SET removed=1 WHERE id=?", v.ID)
		return err
	case "catalog", "catalog_remove":
		v, err := decode[CatalogItem](e.Body)
		if err != nil || v.ID == "" {
			return errors.New("invalid catalog item")
		}
		rank := fmt.Sprintf("%020d:%s:%020d", e.Lamport, e.Origin, e.Sequence)
		_, err = tx.Exec(`INSERT INTO catalog VALUES (?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET event_id=CASE WHEN excluded.rank>catalog.rank THEN excluded.event_id ELSE catalog.event_id END,body=CASE WHEN excluded.rank>catalog.rank THEN excluded.body ELSE catalog.body END,rank=MAX(catalog.rank,excluded.rank),removed=MAX(catalog.removed,excluded.removed)`, v.ID, e.ID, rank, e.Body, e.Kind == "catalog_remove")
		return err
	case "command":
		c, err := decode[Command](e.Body)
		if err != nil {
			return err
		}
		if c.ID != commandID(s.FleetID, c.JobID, c.Target) || c.Kind == "" || len(c.Kind) > 64 || (c.Delivery != "online" && c.Delivery != "queued") || c.Payload["password"] != "" || (c.Delivery == "queued" && c.ExpiresAt <= c.CreatedAt) {
			return errors.New("invalid command")
		}
		if _, err = memberTx(tx, c.Target, false); err != nil {
			return errors.New("unknown command target")
		}
		var prior []byte
		if tx.QueryRow("SELECT body FROM commands WHERE id=?", c.ID).Scan(&prior) == nil {
			if !bytes.Equal(prior, e.Body) {
				return errors.New("command identity conflict")
			}
			return nil
		}
		_, err = tx.Exec("INSERT INTO commands VALUES (?,?,?,?,?)", c.ID, c.JobID, e.ID, c.Target, e.Body)
		return err
	case "receipt":
		r, err := decode[Receipt](e.Body)
		if err != nil {
			return err
		}
		var target string
		if tx.QueryRow("SELECT target FROM commands WHERE id=?", r.CommandID).Scan(&target) != nil || target != e.Origin || target != r.Target {
			return errors.New("only target may publish receipt")
		}
		if r.Status != "queued" && r.Status != "running" && r.Status != "held" && !terminal(r.Status) {
			return errors.New("invalid execution status")
		}
		var old []byte
		if tx.QueryRow("SELECT body FROM receipts WHERE id=?", r.CommandID).Scan(&old) == nil {
			p, _ := decode[Receipt](old)
			if terminal(p.Status) {
				return errors.New("terminal receipt is immutable")
			}
		}
		_, err = tx.Exec("INSERT INTO receipts VALUES (?,?,?,?) ON CONFLICT(id) DO UPDATE SET event_id=excluded.event_id,seq=excluded.seq,body=excluded.body", r.CommandID, e.ID, e.Sequence, e.Body)
		return err
	case "inventory":
		v, err := decode[Observation](e.Body)
		if err != nil {
			return err
		}
		_ = v
		_, err = tx.Exec("INSERT INTO inventory VALUES (?,?,?) ON CONFLICT(id) DO UPDATE SET seq=excluded.seq,body=excluded.body WHERE excluded.seq>inventory.seq", e.Origin, e.Sequence, e.Body)
		return err
	case "import":
		_, err := tx.Exec("INSERT OR IGNORE INTO imports VALUES (?,?)", e.ID, e.Body)
		return err
	case "alias":
		v, err := decode[struct {
			ID       string `json:"id"`
			LegacyID string `json:"legacyId"`
		}](e.Body)
		if err != nil {
			return err
		}
		m, err := memberTx(tx, v.ID, false)
		if err != nil {
			return err
		}
		if m.LegacyID != "" && m.LegacyID != v.LegacyID {
			return errors.New("alias already bound")
		}
		m.LegacyID = v.LegacyID
		_, err = tx.Exec("UPDATE members SET body=? WHERE id=?", encode(m), m.ID)
		return err
	default:
		return errors.New("unsupported event kind")
	}
}

func (s *Store) Ingest(env Envelope) ([]string, error) {
	if len(env.Events) > MaxBatch {
		return nil, errors.New("batch limit exceeded")
	}
	ids := []string{}
	err := s.transaction(func(tx *sql.Tx) error {
		for _, e := range env.Events {
			if err := s.insertTx(tx, e); err != nil {
				return err
			}
			ids = append(ids, e.ID)
		}
		for hash, out := range env.Outputs {
			if len(out) > MaxOutput || digest([]byte(out)) != hash {
				return errors.New("invalid output blob")
			}
			var references int
			if err := tx.QueryRow("SELECT COUNT(*) FROM receipts WHERE json_extract(body,'$.outputHash')=?", hash).Scan(&references); err != nil {
				return err
			}
			if references == 0 {
				return errors.New("unreferenced output")
			}
			var updated int64
			_ = tx.QueryRow("SELECT MAX(json_extract(body,'$.updatedAt')) FROM receipts WHERE json_extract(body,'$.outputHash')=?", hash).Scan(&updated)
			expires := updated + int64(90*24*time.Hour/time.Millisecond)
			if expires > nowMS() {
				if _, err := tx.Exec("INSERT OR IGNORE INTO outputs VALUES (?,?,?)", hash, out, expires); err != nil {
					return err
				}
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return ids, nil
}

func (s *Store) Cursors() (map[string]int64, error) {
	rows, err := s.db.Query("SELECT origin,MAX(seq) FROM events GROUP BY origin")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int64{}
	for rows.Next() {
		var id string
		var seq int64
		if err = rows.Scan(&id, &seq); err != nil {
			return nil, err
		}
		out[id] = seq
	}
	return out, rows.Err()
}
func (s *Store) Export(cursors map[string]int64, limit int) (Envelope, error) {
	if limit < 1 || limit > MaxBatch {
		limit = MaxBatch
	}
	rows, err := s.db.Query("SELECT body FROM events ORDER BY lamport,origin,seq")
	if err != nil {
		return Envelope{}, err
	}
	env := Envelope{Events: []Event{}, Outputs: map[string]string{}}
	size := 0
	for rows.Next() {
		var b []byte
		if err = rows.Scan(&b); err != nil {
			rows.Close()
			return env, err
		}
		e, err := decode[Event](b)
		if err != nil {
			rows.Close()
			return env, err
		}
		if e.Sequence <= cursors[e.Origin] {
			continue
		}
		if len(env.Events) > 0 && size+len(b) > MaxBody/2 {
			break
		}
		env.Events = append(env.Events, e)
		size += len(b)
		if len(env.Events) >= limit {
			break
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return env, err
	}
	for _, e := range env.Events {
		if e.Kind == "receipt" {
			r, _ := decode[Receipt](e.Body)
			if r.OutputHash != "" {
				var out string
				if s.db.QueryRow("SELECT body FROM outputs WHERE hash=? AND expires>?", r.OutputHash, nowMS()).Scan(&out) == nil && size+len(out) < MaxBody-65536 {
					env.Outputs[r.OutputHash] = out
					size += len(out)
				}
			}
		}
	}
	return env, nil
}
func (s *Store) Event(id string) (Event, error) {
	var b []byte
	err := s.db.QueryRow("SELECT body FROM events WHERE id=?", id).Scan(&b)
	if err != nil {
		return Event{}, err
	}
	return decode[Event](b)
}
func (s *Store) Members() ([]Member, error) {
	rows, err := s.db.Query("SELECT m.body,m.removed,COALESCE(a.peer_url,''),COALESCE(a.dashboard_url,'') FROM members m LEFT JOIN addresses a ON m.id=a.id ORDER BY m.id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Member{}
	for rows.Next() {
		var b []byte
		var r bool
		var p, d string
		if err = rows.Scan(&b, &r, &p, &d); err != nil {
			return nil, err
		}
		m, err := decode[Member](b)
		if err != nil {
			return nil, err
		}
		m.Removed = r
		if p != "" {
			m.PeerURL = p
		}
		if d != "" {
			m.DashboardURL = d
		}
		out = append(out, m)
	}
	return out, rows.Err()
}
func (s *Store) Member(id string) (Member, error) {
	ms, err := s.Members()
	if err != nil {
		return Member{}, err
	}
	for _, m := range ms {
		if m.ID == id {
			return m, nil
		}
	}
	return Member{}, sql.ErrNoRows
}
func (s *Store) Active(id string) bool { m, err := s.Member(id); return err == nil && !m.Removed }
func (s *Store) Address(m Member) error {
	_, err := s.db.Exec("INSERT INTO addresses VALUES (?,?,?) ON CONFLICT(id) DO UPDATE SET peer_url=CASE WHEN excluded.peer_url<>'' THEN excluded.peer_url ELSE addresses.peer_url END,dashboard_url=CASE WHEN excluded.dashboard_url<>'' THEN excluded.dashboard_url ELSE addresses.dashboard_url END", m.ID, m.PeerURL, m.DashboardURL)
	return err
}
func (s *Store) Command(id string) (Command, string, error) {
	var b []byte
	var e string
	err := s.db.QueryRow("SELECT body,event_id FROM commands WHERE id=?", id).Scan(&b, &e)
	if err != nil {
		return Command{}, e, err
	}
	c, err := decode[Command](b)
	return c, e, err
}
func (s *Store) Receipt(id string) (Receipt, error) {
	var b []byte
	err := s.db.QueryRow("SELECT body FROM receipts WHERE id=?", id).Scan(&b)
	if err != nil {
		return Receipt{}, err
	}
	return decode[Receipt](b)
}
func (s *Store) RememberReplicas(peer string, ids []string) error {
	return s.transaction(func(tx *sql.Tx) error {
		for _, id := range ids {
			if _, err := tx.Exec("INSERT OR IGNORE INTO replicas VALUES (?,?)", id, peer); err != nil {
				return err
			}
		}
		return nil
	})
}
func (s *Store) Backup(file string) error {
	abs, err := filepath.Abs(file)
	if err != nil {
		return err
	}
	if _, err = os.Stat(abs); !errors.Is(err, os.ErrNotExist) {
		return errors.New("backup destination must not exist")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err = s.db.Exec("VACUUM INTO '" + strings.ReplaceAll(filepath.ToSlash(abs), "'", "''") + "'")
	return err
}
