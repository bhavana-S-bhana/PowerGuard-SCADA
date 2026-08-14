package database

import (
	"database/sql"
	_ "embed"
	"fmt"
	"log"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schemaSQL string

type Tag struct {
	TagID       string    `json:"tag_id"`
	Name        string    `json:"name"`
	Category    string    `json:"category"`
	Unit        string    `json:"unit"`
	State       int       `json:"state"`
	TargetState int       `json:"target_state"`
	IsHighRisk  bool      `json:"is_high_risk"`
	Priority    string    `json:"priority"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type Alarm struct {
	ID        int64      `json:"id"`
	TagID     string     `json:"tag_id"`
	TagName   string     `json:"tag_name"`
	Severity  string     `json:"severity"`
	Message   string     `json:"message"`
	PrevState int        `json:"prev_state"`
	CurrState int        `json:"curr_state"`
	Status    string     `json:"status"`
	AckBy     *string    `json:"ack_by"`
	AckAt     *time.Time `json:"ack_at"`
	Timestamp time.Time  `json:"timestamp"`
}

type AuditLog struct {
	ID        int64     `json:"id"`
	Timestamp time.Time `json:"timestamp"`
	Operator  string    `json:"operator"`
	Action    string    `json:"action"`
	TagID     string    `json:"tag_id"`
	PrevValue int       `json:"prev_value"`
	NewValue  int       `json:"new_value"`
	Rationale string    `json:"rationale"`
}

type DB struct {
	*sql.DB
	mu sync.Mutex
}

func InitDB(dbPath string) (*DB, error) {
	log.Printf("[DB] Initializing embedded SQLite database at: %s", dbPath)
	db, err := sql.Open("sqlite", dbPath+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(ON)")
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	db.SetMaxOpenConns(1) // Embedded SQLite 1 connection max for safe concurrent file writes

	if _, err := db.Exec(schemaSQL); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to execute database schema: %w", err)
	}

	log.Println("[DB] Database schema and initial telemetry seed successfully loaded.")
	return &DB{DB: db}, nil
}

func (d *DB) GetAllTags() ([]Tag, error) {
	query := `SELECT tag_id, name, category, unit, state, target_state, is_high_risk, priority, updated_at FROM tags ORDER BY category, tag_id`
	rows, err := d.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tags []Tag
	for rows.Next() {
		var t Tag
		var highRisk int
		var updatedAtStr string
		if err := rows.Scan(&t.TagID, &t.Name, &t.Category, &t.Unit, &t.State, &t.TargetState, &highRisk, &t.Priority, &updatedAtStr); err != nil {
			return nil, err
		}
		t.IsHighRisk = highRisk == 1
		t.UpdatedAt, _ = time.Parse("2006-01-02 15:04:05", updatedAtStr)
		if t.UpdatedAt.IsZero() {
			t.UpdatedAt = time.Now()
		}
		tags = append(tags, t)
	}
	return tags, nil
}

func (d *DB) GetTagByID(tagID string) (*Tag, error) {
	query := `SELECT tag_id, name, category, unit, state, target_state, is_high_risk, priority, updated_at FROM tags WHERE tag_id = ?`
	row := d.QueryRow(query, tagID)
	var t Tag
	var highRisk int
	var updatedAtStr string
	if err := row.Scan(&t.TagID, &t.Name, &t.Category, &t.Unit, &t.State, &t.TargetState, &highRisk, &t.Priority, &updatedAtStr); err != nil {
		return nil, err
	}
	t.IsHighRisk = highRisk == 1
	t.UpdatedAt, _ = time.Parse("2006-01-02 15:04:05", updatedAtStr)
	return &t, nil
}

// UpdateTagStateTx executes a transactional control override update.
func (d *DB) UpdateTagStateTx(tagID string, newState int, operator string, rationale string) (*Tag, int, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	tx, err := d.Begin()
	if err != nil {
		return nil, 0, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// 1. Fetch current tag state
	var current Tag
	var highRisk int
	var updatedAtStr string
	err = tx.QueryRow(`SELECT tag_id, name, category, unit, state, target_state, is_high_risk, priority, updated_at FROM tags WHERE tag_id = ?`, tagID).
		Scan(&current.TagID, &current.Name, &current.Category, &current.Unit, &current.State, &current.TargetState, &highRisk, &current.Priority, &updatedAtStr)
	if err != nil {
		return nil, 0, fmt.Errorf("tag not found: %w", err)
	}
	current.IsHighRisk = highRisk == 1
	prevState := current.State

	if prevState == newState {
		return &current, prevState, nil // No change needed
	}

	now := time.Now().Format("2006-01-02 15:04:05")

	// 2. Update Tag state
	_, err = tx.Exec(`UPDATE tags SET state = ?, target_state = ?, updated_at = ? WHERE tag_id = ?`, newState, newState, now, tagID)
	if err != nil {
		return nil, prevState, fmt.Errorf("failed to update tag state: %w", err)
	}

	// 3. Create Audit Log Entry
	action := "MANUAL_OVERRIDE"
	if current.IsHighRisk {
		action = "HIGH_RISK_OVERRIDE"
	}
	_, err = tx.Exec(`INSERT INTO audit_logs (operator, action, tag_id, prev_value, new_value, rationale) VALUES (?, ?, ?, ?, ?, ?)`,
		operator, action, tagID, prevState, newState, rationale)
	if err != nil {
		return nil, prevState, fmt.Errorf("failed to create audit log: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, prevState, fmt.Errorf("failed to commit transaction: %w", err)
	}

	current.State = newState
	current.TargetState = newState
	current.UpdatedAt = time.Now()

	return &current, prevState, nil
}

// System-level telemetry update (e.g. background Goroutine or simulation)
func (d *DB) SystemSetTagState(tagID string, newState int) (int, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	var prevState int
	err := d.QueryRow(`SELECT state FROM tags WHERE tag_id = ?`, tagID).Scan(&prevState)
	if err != nil {
		return 0, err
	}

	if prevState == newState {
		return prevState, nil
	}

	now := time.Now().Format("2006-01-02 15:04:05")
	_, err = d.Exec(`UPDATE tags SET state = ?, target_state = ?, updated_at = ? WHERE tag_id = ?`, newState, newState, now, tagID)
	if err != nil {
		return prevState, err
	}

	return prevState, nil
}

func (d *DB) CreateAlarm(tagID string, tagName string, severity string, msg string, prevState int, currState int) (*Alarm, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	now := time.Now()
	res, err := d.Exec(`INSERT INTO alarms (tag_id, tag_name, severity, message, prev_state, curr_state, status, timestamp) VALUES (?, ?, ?, ?, ?, ?, 'ACTIVE', ?)`,
		tagID, tagName, severity, msg, prevState, currState, now.Format("2006-01-02 15:04:05"))
	if err != nil {
		return nil, err
	}

	id, _ := res.LastInsertId()
	return &Alarm{
		ID:        id,
		TagID:     tagID,
		TagName:   tagName,
		Severity:  severity,
		Message:   msg,
		PrevState: prevState,
		CurrState: currState,
		Status:    "ACTIVE",
		Timestamp: now,
	}, nil
}

func (d *DB) AcknowledgeAlarm(alarmID int64, operator string) (*Alarm, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	now := time.Now()
	nowStr := now.Format("2006-01-02 15:04:05")

	_, err := d.Exec(`UPDATE alarms SET status = 'ACKNOWLEDGED', ack_by = ?, ack_at = ? WHERE id = ?`, operator, nowStr, alarmID)
	if err != nil {
		return nil, err
	}

	var a Alarm
	var ackByStr, ackAtStr, tsStr string
	err = d.QueryRow(`SELECT id, tag_id, tag_name, severity, message, prev_state, curr_state, status, ack_by, ack_at, timestamp FROM alarms WHERE id = ?`, alarmID).
		Scan(&a.ID, &a.TagID, &a.TagName, &a.Severity, &a.Message, &a.PrevState, &a.CurrState, &a.Status, &ackByStr, &ackAtStr, &tsStr)
	if err != nil {
		return nil, err
	}

	a.AckBy = &ackByStr
	ackTime, _ := time.Parse("2006-01-02 15:04:05", ackAtStr)
	a.AckAt = &ackTime
	a.Timestamp, _ = time.Parse("2006-01-02 15:04:05", tsStr)

	// Add audit record for acknowledgment
	_, _ = d.Exec(`INSERT INTO audit_logs (operator, action, tag_id, prev_value, new_value, rationale) VALUES (?, 'ACKNOWLEDGE_ALARM', ?, 0, 0, ?)`,
		operator, a.TagID, fmt.Sprintf("Acknowledged Alarm #%d: %s", alarmID, a.Message))

	return &a, nil
}

func (d *DB) GetActiveAlarms() ([]Alarm, error) {
	query := `SELECT id, tag_id, tag_name, severity, message, prev_state, curr_state, status, timestamp FROM alarms WHERE status = 'ACTIVE' ORDER BY timestamp DESC`
	rows, err := d.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var alarms []Alarm
	for rows.Next() {
		var a Alarm
		var tsStr string
		if err := rows.Scan(&a.ID, &a.TagID, &a.TagName, &a.Severity, &a.Message, &a.PrevState, &a.CurrState, &a.Status, &tsStr); err != nil {
			return nil, err
		}
		a.Timestamp, _ = time.Parse("2006-01-02 15:04:05", tsStr)
		alarms = append(alarms, a)
	}
	return alarms, nil
}

func (d *DB) GetAlarmHistory(limit int) ([]Alarm, error) {
	query := fmt.Sprintf(`SELECT id, tag_id, tag_name, severity, message, prev_state, curr_state, status, ack_by, ack_at, timestamp FROM alarms ORDER BY timestamp DESC LIMIT %d`, limit)
	rows, err := d.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var alarms []Alarm
	for rows.Next() {
		var a Alarm
		var ackBy, ackAt, tsStr sql.NullString
		if err := rows.Scan(&a.ID, &a.TagID, &a.TagName, &a.Severity, &a.Message, &a.PrevState, &a.CurrState, &a.Status, &ackBy, &ackAt, &tsStr); err != nil {
			return nil, err
		}
		if tsStr.Valid {
			a.Timestamp, _ = time.Parse("2006-01-02 15:04:05", tsStr.String)
		}
		if ackBy.Valid {
			a.AckBy = &ackBy.String
		}
		if ackAt.Valid {
			t, _ := time.Parse("2006-01-02 15:04:05", ackAt.String)
			a.AckAt = &t
		}
		alarms = append(alarms, a)
	}
	return alarms, nil
}

func (d *DB) GetAuditLogs(limit int) ([]AuditLog, error) {
	query := fmt.Sprintf(`SELECT id, timestamp, operator, action, tag_id, prev_value, new_value, rationale FROM audit_logs ORDER BY timestamp DESC LIMIT %d`, limit)
	rows, err := d.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []AuditLog
	for rows.Next() {
		var l AuditLog
		var tsStr string
		if err := rows.Scan(&l.ID, &tsStr, &l.Operator, &l.Action, &l.TagID, &l.PrevValue, &l.NewValue, &l.Rationale); err != nil {
			return nil, err
		}
		l.Timestamp, _ = time.Parse("2006-01-02 15:04:05", tsStr)
		logs = append(logs, l)
	}
	return logs, nil
}
