package database

import (
	"os"
	"testing"
)

func TestDatabaseAnd30Tags(t *testing.T) {
	tmpDB := "test_scada_30.db"
	defer os.Remove(tmpDB)

	db, err := InitDB(tmpDB)
	if err != nil {
		t.Fatalf("Failed to initialize test DB: %v", err)
	}
	defer db.Close()

	// 1. Verify Exactly 30 Seed Tags
	tags, err := db.GetAllTags()
	if err != nil {
		t.Fatalf("Failed to fetch tags: %v", err)
	}
	if len(tags) != 30 {
		t.Fatalf("Expected exactly 30 seed tags in database, got %d", len(tags))
	}

	// 2. Test Transactional Tag State Override
	testTagID := "TEMP_BEARING_G1"
	operator := "TEST_OPERATOR"
	rationale := "Bearing Temperature Sensor Self-Test Override"

	updatedTag, prevState, err := db.UpdateTagStateTx(testTagID, 1, operator, rationale)
	if err != nil {
		t.Fatalf("Failed to execute tag override transaction: %v", err)
	}

	if prevState != 0 || updatedTag.State != 1 {
		t.Errorf("Expected transition 0 -> 1, got prev=%d, curr=%d", prevState, updatedTag.State)
	}

	// 3. Verify Audit Log Entry
	logs, err := db.GetAuditLogs(10)
	if err != nil {
		t.Fatalf("Failed to fetch audit logs: %v", err)
	}
	if len(logs) == 0 {
		t.Fatalf("Expected audit log entry, got 0")
	}
	if logs[0].TagID != testTagID || logs[0].Operator != operator {
		t.Errorf("Audit log record mismatch: got tag=%s op=%s", logs[0].TagID, logs[0].Operator)
	}

	// 4. Test Alarm Creation and Acknowledgment
	alarm, err := db.CreateAlarm(testTagID, "Generator 1 Bearing Over-Temp Sensor", "CRITICAL", "High Temperature Trip Alert", 0, 1)
	if err != nil {
		t.Fatalf("Failed to create alarm: %v", err)
	}
	if alarm.Status != "ACTIVE" {
		t.Errorf("Expected ACTIVE alarm, got %s", alarm.Status)
	}

	ackAlarm, err := db.AcknowledgeAlarm(alarm.ID, operator)
	if err != nil {
		t.Fatalf("Failed to acknowledge alarm: %v", err)
	}
	if ackAlarm.Status != "ACKNOWLEDGED" {
		t.Errorf("Expected ACKNOWLEDGED alarm, got %s", ackAlarm.Status)
	}
}
