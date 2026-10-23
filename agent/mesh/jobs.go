package mesh

import (
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

func (s *Store) CreateCommands(req SubmitRequest) ([]Event, error) {
	if len(req.SubmissionID) < 16 || len(req.SubmissionID) > 128 || !validKind(req.Kind) || len(req.MachineIDs) == 0 || len(req.MachineIDs) > 50 {
		return nil, errors.New("valid submission ID, kind, and 1–50 targets required")
	}
	if req.Delivery == "" {
		req.Delivery = "online"
	}
	if req.Delivery != "online" && req.Delivery != "queued" {
		return nil, errors.New("invalid delivery mode")
	}
	if req.Delivery == "queued" && (req.ExpiresAt <= nowMS() || req.ExpiresAt > nowMS()+int64(30*24*time.Hour/time.Millisecond)) {
		return nil, errors.New("queued work requires an expiry within 30 days")
	}
	if req.Payload == nil {
		req.Payload = map[string]string{}
	}
	if len(encode(req)) > 128*1024 {
		return nil, errors.New("command exceeds limit")
	}
	sort.Strings(req.MachineIDs)
	for j := 1; j < len(req.MachineIDs); j++ {
		if req.MachineIDs[j] == req.MachineIDs[j-1] {
			return nil, errors.New("duplicate target")
		}
	}
	mac := hmac.New(sha256.New, s.Identity.Signing)
	mac.Write(encode(req))
	requestDigest := hex.EncodeToString(mac.Sum(nil))
	events := []Event{}
	err := s.transaction(func(tx *sql.Tx) error {
		if _, err := memberTx(tx, s.Identity.ID(), true); err != nil {
			return errors.New("local identity is not active")
		}
		var previous string
		if tx.QueryRow("SELECT digest FROM submissions WHERE id=?", req.SubmissionID).Scan(&previous) == nil {
			if previous != requestDigest {
				return errors.New("submission ID reused with different contents")
			}
			for _, id := range req.MachineIDs {
				var body []byte
				if err := tx.QueryRow("SELECT e.body FROM commands c JOIN events e ON e.id=c.event_id WHERE c.id=?", commandID(s.FleetID, req.SubmissionID, id)).Scan(&body); err != nil {
					return err
				}
				e, err := decode[Event](body)
				if err != nil {
					return err
				}
				events = append(events, e)
			}
			return nil
		}
		var count int
		_ = tx.QueryRow("SELECT COUNT(*) FROM commands WHERE job_id=?", req.SubmissionID).Scan(&count)
		if count > 0 {
			return errors.New("submission originated on another peer; query its receipts instead of resubmitting")
		}
		for _, id := range req.MachineIDs {
			m, err := memberTx(tx, id, true)
			if err != nil {
				return fmt.Errorf("target %s is not an active member", id)
			}
			payload := map[string]string{}
			for k, v := range req.Payload {
				if k != "password" {
					payload[k] = v
				}
			}
			var secret []byte
			if req.Payload["password"] != "" {
				secret, err = seal(m.EncryptionKey, req.Payload["password"])
				if err != nil {
					return err
				}
			}
			if req.Kind == "check" && payload["match"] == "" {
				rows, err := tx.Query("SELECT body FROM catalog WHERE removed=0")
				if err != nil {
					return err
				}
				for rows.Next() {
					var b []byte
					if err = rows.Scan(&b); err != nil {
						rows.Close()
						return err
					}
					a, _ := decode[CatalogItem](b)
					if strings.EqualFold(a.WingetID, payload["package"]) || strings.EqualFold(a.Name, payload["package"]) || strings.EqualFold(a.Match, payload["package"]) {
						payload["match"] = a.Match
					}
				}
				rows.Close()
			}
			c := Command{ID: commandID(s.FleetID, req.SubmissionID, id), JobID: req.SubmissionID, Target: id, Kind: req.Kind, Payload: payload, Secret: secret, Delivery: req.Delivery, ExpiresAt: req.ExpiresAt, CreatedAt: nowMS(), RetryOf: req.RetryOf}
			var admission string
			if err = tx.QueryRow("SELECT event_id FROM members WHERE id=?", id).Scan(&admission); err != nil {
				return err
			}
			e, err := s.appendTx(tx, "command", c, []string{admission})
			if err != nil {
				return err
			}
			events = append(events, e)
		}
		if req.Kind == "map_drive" {
			// Preferences stay on the submitting node and never contain credentials.
			prefs := map[string]any{"letter": req.Payload["letter"], "unc": req.Payload["unc"], "username": req.Payload["username"], "password": "", "uncHistory": []string{req.Payload["unc"]}}
			if _, err := tx.Exec("INSERT OR REPLACE INTO meta VALUES ('mapPrefs',?)", encode(prefs)); err != nil {
				return err
			}
		}
		_, err := tx.Exec("INSERT INTO submissions VALUES (?,?)", req.SubmissionID, requestDigest)
		return err
	})
	return events, err
}

func commandTx(tx *sql.Tx, id string) (Command, Event, error) {
	var b, eb []byte
	err := tx.QueryRow("SELECT c.body,e.body FROM commands c JOIN events e ON c.event_id=e.id WHERE c.id=?", id).Scan(&b, &eb)
	if err != nil {
		return Command{}, Event{}, err
	}
	c, err := decode[Command](b)
	if err != nil {
		return c, Event{}, err
	}
	e, err := decode[Event](eb)
	return c, e, err
}

func (s *Store) receiptTx(tx *sql.Tx, c Command, status, message string, result *Result) (Receipt, error) {
	r := Receipt{CommandID: c.ID, Target: c.Target, Status: status, Message: message, UpdatedAt: nowMS()}
	if result != nil {
		r.Via = result.Via
		r.Installed = result.Installed
		r.Installations = result.Installations
		output := strings.ToValidUTF8(result.Output, "")
		if len(output) > MaxOutput {
			output = output[:MaxOutput]
			for !utf8.ValidString(output) {
				output = output[:len(output)-1]
			}
		}
		if output != "" {
			r.OutputHash = digest([]byte(output))
			if _, err := tx.Exec("INSERT OR IGNORE INTO outputs VALUES (?,?,?)", r.OutputHash, output, r.UpdatedAt+int64(90*24*time.Hour/time.Millisecond)); err != nil {
				return r, err
			}
		}
	}
	var commandEvent string
	if err := tx.QueryRow("SELECT event_id FROM commands WHERE id=?", c.ID).Scan(&commandEvent); err != nil {
		return r, err
	}
	_, err := s.appendTx(tx, "receipt", r, []string{commandEvent})
	return r, err
}

// Accept is the only entry point that creates executable inbox work. Ingest
// merely records history, even when that history contains a local receipt.
func (s *Store) Accept(id string, clockUncertain bool) (Receipt, error) {
	var receipt Receipt
	err := s.transaction(func(tx *sql.Tx) error {
		c, event, err := commandTx(tx, id)
		if err != nil {
			return err
		}
		if c.Target != s.Identity.ID() {
			return errors.New("command targets another machine")
		}
		if _, err = memberTx(tx, c.Target, true); err != nil {
			return err
		}
		if _, err = memberTx(tx, event.Origin, true); err != nil {
			return errors.New("command origin removed")
		}
		var body []byte
		if tx.QueryRow("SELECT body FROM receipts WHERE id=?", id).Scan(&body) == nil {
			receipt, err = decode[Receipt](body)
			return err
		}
		status, message := "queued", "Accepted durably by target"
		if !validKind(c.Kind) {
			status, message = "error", "This node version does not support the command"
		} else if c.Delivery == "queued" && clockUncertain {
			status, message = "held", "Clock uncertain; waiting for trustworthy time"
		} else if c.Delivery == "queued" && c.ExpiresAt <= nowMS() {
			status, message = "expired", "Expired before target acceptance"
		}
		receipt, err = s.receiptTx(tx, c, status, message, nil)
		if err != nil {
			return err
		}
		_, err = tx.Exec("INSERT INTO inbox VALUES (?,?)", id, status)
		return err
	})
	return receipt, err
}

// Recover never repeats a side effect whose start record was committed.
func (s *Store) Recover() error {
	return s.RecoverExcept("")
}
func (s *Store) RecoverExcept(pendingUpdate string) error {
	return s.transaction(func(tx *sql.Tx) error {
		rows, err := tx.Query("SELECT id FROM inbox WHERE state='running' AND id<>?", pendingUpdate)
		if err != nil {
			return err
		}
		ids := []string{}
		for rows.Next() {
			var id string
			if err = rows.Scan(&id); err != nil {
				rows.Close()
				return err
			}
			ids = append(ids, id)
		}
		rows.Close()
		for _, id := range ids {
			c, _, err := commandTx(tx, id)
			if err != nil {
				return err
			}
			status, message := "unknown", "Interrupted during execution; inspect the machine before an explicit retry"
			if c.Kind == "check" {
				status, message = "queued", "Retrying interrupted read-only check"
			}
			if _, err = s.receiptTx(tx, c, status, message, nil); err != nil {
				return err
			}
			if _, err = tx.Exec("UPDATE inbox SET state=? WHERE id=?", status, id); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Store) Next(clockUncertain bool) (*Command, error) {
	var command *Command
	err := s.transaction(func(tx *sql.Tx) error {
		rows, err := tx.Query("SELECT id FROM inbox WHERE state IN ('queued','held') ORDER BY rowid")
		if err != nil {
			return err
		}
		ids := []string{}
		for rows.Next() {
			var id string
			if err = rows.Scan(&id); err != nil {
				rows.Close()
				return err
			}
			ids = append(ids, id)
		}
		rows.Close()
		for _, id := range ids {
			c, e, err := commandTx(tx, id)
			if err != nil {
				return err
			}
			status, message := "running", "Running on target"
			_, localErr := memberTx(tx, c.Target, true)
			_, originErr := memberTx(tx, e.Origin, true)
			if localErr != nil || originErr != nil {
				status, message = "error", "Target or command origin was removed before execution"
			} else if c.Delivery == "queued" && clockUncertain {
				continue
			} else if c.Delivery == "queued" && c.ExpiresAt <= nowMS() {
				status, message = "expired", "Expired before execution"
			}
			if _, err = s.receiptTx(tx, c, status, message, nil); err != nil {
				return err
			}
			if _, err = tx.Exec("UPDATE inbox SET state=? WHERE id=?", status, id); err != nil {
				return err
			}
			if status == "running" {
				command = &c
				break
			}
		}
		return nil
	})
	return command, err
}

func (s *Store) Complete(c Command, result Result) error {
	return s.transaction(func(tx *sql.Tx) error {
		var state string
		if err := tx.QueryRow("SELECT state FROM inbox WHERE id=?", c.ID).Scan(&state); err != nil {
			return err
		}
		if state != "running" {
			return errors.New("execution is not running")
		}
		if result.Status != "ok" && result.Status != "error" && result.Status != "unknown" {
			return errors.New("invalid executor result")
		}
		if _, err := s.receiptTx(tx, c, result.Status, result.Message, &result); err != nil {
			return err
		}
		_, err := tx.Exec("UPDATE inbox SET state=? WHERE id=?", result.Status, c.ID)
		return err
	})
}

func (s *Store) queuedCommands() ([]Command, error) {
	rows, err := s.db.Query("SELECT c.body FROM commands c LEFT JOIN receipts r ON c.id=r.id WHERE json_extract(c.body,'$.delivery')='queued' AND r.id IS NULL")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Command{}
	for rows.Next() {
		var b []byte
		if err = rows.Scan(&b); err != nil {
			return nil, err
		}
		c, err := decode[Command](b)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Store) Catalog() ([]CatalogItem, error) {
	rows, err := s.db.Query("SELECT body FROM catalog WHERE removed=0 ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []CatalogItem{}
	for rows.Next() {
		var b []byte
		if err = rows.Scan(&b); err != nil {
			return nil, err
		}
		v, err := decode[CatalogItem](b)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (s *Store) EditCatalog(item CatalogItem, remove bool) (Event, error) {
	if item.ID == "" || len(item.ID) > 128 || (!remove && (strings.TrimSpace(item.Name) == "" || len(item.Name) > 255)) {
		return Event{}, errors.New("valid catalog ID and name required")
	}
	if item.Match == "" {
		item.Match = item.Name
	}
	kind := "catalog"
	if remove {
		kind = "catalog_remove"
	}
	var prior string
	_ = s.db.QueryRow("SELECT event_id FROM catalog WHERE id=?", item.ID).Scan(&prior)
	deps := []string{}
	if prior != "" {
		deps = append(deps, prior)
	}
	return s.Append(kind, item, deps...)
}

func (s *Store) RetryCommand(id, submission string) (Event, error) {
	if len(submission) < 16 || len(submission) > 128 {
		return Event{}, errors.New("new submission ID required")
	}
	var event Event
	err := s.transaction(func(tx *sql.Tx) error {
		c, _, err := commandTx(tx, id)
		if err != nil {
			return err
		}
		if _, err = memberTx(tx, s.Identity.ID(), true); err != nil {
			return err
		}
		if _, err = memberTx(tx, c.Target, true); err != nil {
			return err
		}
		var previous []byte
		if tx.QueryRow("SELECT body FROM receipts WHERE id=?", id).Scan(&previous) == nil {
			r, _ := decode[Receipt](previous)
			if r.Status == "running" || r.Status == "queued" || r.Status == "held" || r.Status == "ok" {
				return errors.New("execution must be failed, expired, or unknown before retry")
			}
		}
		newID := commandID(s.FleetID, submission, c.Target)
		if tx.QueryRow("SELECT e.body FROM commands c JOIN events e ON e.id=c.event_id WHERE c.id=?", newID).Scan(&previous) == nil {
			event, err = decode[Event](previous)
			if err != nil {
				return err
			}
			existing, _ := decode[Command](event.Body)
			if existing.RetryOf != id {
				return errors.New("retry submission identity conflict")
			}
			return nil
		}
		c.RetryOf = id
		c.ID = newID
		c.JobID = submission
		c.CreatedAt = nowMS()
		c.Delivery = "online"
		c.ExpiresAt = 0
		event, err = s.appendTx(tx, "command", c, nil)
		return err
	})
	return event, err
}
