package handlers

import (
	"encoding/json"
	"log"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"power-plant-scada/database"
	"power-plant-scada/telemetry"
	"power-plant-scada/websocket"
)

type API struct {
	db        *database.DB
	hub       *websocket.Hub
	monitor   *telemetry.Monitor
	startTime time.Time
}

func NewAPI(db *database.DB, hub *websocket.Hub, monitor *telemetry.Monitor) *API {
	return &API{
		db:        db,
		hub:       hub,
		monitor:   monitor,
		startTime: time.Now(),
	}
}

func (a *API) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/system/status", a.handleSystemStatus)
	mux.HandleFunc("/api/tags", a.handleGetTags)
	mux.HandleFunc("/api/control/override", a.handleControlOverride)
	mux.HandleFunc("/api/alarms/acknowledge", a.handleAcknowledgeAlarm)
	mux.HandleFunc("/api/alarms/history", a.handleAlarmHistory)
	mux.HandleFunc("/api/audit", a.handleAuditLogs)
	mux.HandleFunc("/api/simulation/toggle", a.handleToggleSimulation)
	mux.HandleFunc("/api/qwen", a.handleQwen)
}

func sendJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func sendError(w http.ResponseWriter, status int, message string) {
	sendJSON(w, status, map[string]string{"error": message})
}

func (a *API) handleSystemStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		sendError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	alarms, err := a.db.GetActiveAlarms()
	if err != nil {
		alarms = []database.Alarm{}
	}

	tags := a.monitor.GetCache().GetAll()
	var totalGenerators, activeGenerators int
	var estimatedMW float64 = 0.0

	for _, t := range tags {
		if t.Category == "GENERATOR" {
			totalGenerators++
			if t.State == 1 {
				activeGenerators++
				estimatedMW += 450.0 // 450MW per active generator unit
			}
		}
	}

	uptimeSeconds := int64(time.Since(a.startTime).Seconds())

	sendJSON(w, http.StatusOK, map[string]interface{}{
		"status":              "OPERATIONAL",
		"uptime_seconds":      uptimeSeconds,
		"uptime_formatted":    formatUptime(uptimeSeconds),
		"active_alarms":       len(alarms),
		"connected_clients":   a.hub.ClientCount(),
		"simulation_active":   a.monitor.IsSimActive(),
		"grid_frequency_hz":   50.00,
		"total_output_mw":     estimatedMW,
		"active_generators":   fmt.Sprintf("%d / %d", activeGenerators, totalGenerators),
		"system_clock":        time.Now().Format("2006-01-02 15:04:05 MST"),
	})
}

func (a *API) handleGetTags(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		sendError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	tags, err := a.db.GetAllTags()
	if err != nil {
		sendError(w, http.StatusInternalServerError, err.Error())
		return
	}

	sendJSON(w, http.StatusOK, map[string]interface{}{
		"tags": tags,
	})
}

type OverrideRequest struct {
	TagID       string `json:"tag_id"`
	TargetState int    `json:"target_state"`
	Operator    string `json:"operator"`
	Rationale   string `json:"rationale"`
}

func (a *API) handleControlOverride(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		sendError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	var req OverrideRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sendError(w, http.StatusBadRequest, "Invalid JSON payload")
		return
	}

	if req.TagID == "" {
		sendError(w, http.StatusBadRequest, "tag_id is required")
		return
	}

	if req.TargetState != 0 && req.TargetState != 1 {
		sendError(w, http.StatusBadRequest, "target_state must be 0 or 1")
		return
	}

	if req.Operator == "" {
		req.Operator = "OPERATOR_STATION_1"
	}

	// 1. Fetch Tag to check if high-risk
	existingTag, err := a.db.GetTagByID(req.TagID)
	if err != nil {
		sendError(w, http.StatusNotFound, "Tag not found")
		return
	}

	if existingTag.IsHighRisk && req.Rationale == "" {
		sendError(w, http.StatusBadRequest, "Rationale is mandatory for high-risk equipment override")
		return
	}

	// 2. Execute Transactional Database Override
	updatedTag, prevState, err := a.db.UpdateTagStateTx(req.TagID, req.TargetState, req.Operator, req.Rationale)
	if err != nil {
		sendError(w, http.StatusInternalServerError, fmt.Sprintf("Transaction failed: %v", err))
		return
	}

	// 3. Update In-Memory Cache
	a.monitor.GetCache().Set(*updatedTag)

	// 4. Dispatch WebSocket Event
	a.hub.Broadcast(websocket.EventPayload{
		Type:      "CONTROL_OVERRIDE",
		TagID:     updatedTag.TagID,
		TagName:   updatedTag.Name,
		PrevState: prevState,
		CurrState: updatedTag.State,
		Priority:  updatedTag.Priority,
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"tag":       updatedTag,
			"operator":  req.Operator,
			"rationale": req.Rationale,
		},
	})

	sendJSON(w, http.StatusOK, map[string]interface{}{
		"success":    true,
		"tag":        updatedTag,
		"prev_state": prevState,
		"message":    fmt.Sprintf("Successfully overridden tag %s to state %d", updatedTag.TagID, updatedTag.State),
	})
}

type AckRequest struct {
	AlarmID  int64  `json:"alarm_id"`
	Operator string `json:"operator"`
}

func (a *API) handleAcknowledgeAlarm(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		sendError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	var req AckRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sendError(w, http.StatusBadRequest, "Invalid JSON payload")
		return
	}

	if req.AlarmID <= 0 {
		sendError(w, http.StatusBadRequest, "alarm_id must be greater than 0")
		return
	}

	if req.Operator == "" {
		req.Operator = "OPERATOR_STATION_1"
	}

	alarm, err := a.db.AcknowledgeAlarm(req.AlarmID, req.Operator)
	if err != nil {
		sendError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to acknowledge alarm: %v", err))
		return
	}

	// Broadcast acknowledgment payload over WebSocket
	a.hub.Broadcast(websocket.EventPayload{
		Type:      "ALARM_ACKNOWLEDGED",
		AlarmID:   alarm.ID,
		TagID:     alarm.TagID,
		TagName:   alarm.TagName,
		Timestamp: time.Now(),
		Data:      alarm,
	})

	sendJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"alarm":   alarm,
	})
}

func (a *API) handleAlarmHistory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		sendError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	limitStr := r.URL.Query().Get("limit")
	limit := 100
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}

	alarms, err := a.db.GetAlarmHistory(limit)
	if err != nil {
		sendError(w, http.StatusInternalServerError, err.Error())
		return
	}

	sendJSON(w, http.StatusOK, map[string]interface{}{
		"alarms": alarms,
	})
}

func (a *API) handleAuditLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		sendError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	limitStr := r.URL.Query().Get("limit")
	limit := 100
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}

	logs, err := a.db.GetAuditLogs(limit)
	if err != nil {
		sendError(w, http.StatusInternalServerError, err.Error())
		return
	}

	sendJSON(w, http.StatusOK, map[string]interface{}{
		"audit_logs": logs,
	})
}

type SimToggleRequest struct {
	Active bool `json:"active"`
}

func (a *API) handleToggleSimulation(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		sendError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	var req SimToggleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sendError(w, http.StatusBadRequest, "Invalid payload")
		return
	}

	a.monitor.SetSimActive(req.Active)

	sendJSON(w, http.StatusOK, map[string]interface{}{
		"success":    true,
		"sim_active": req.Active,
	})
}

func formatUptime(seconds int64) string {
	d := seconds / 86400
	h := (seconds % 86400) / 3600
	m := (seconds % 3600) / 60
	s := seconds % 60
	if d > 0 {
		return fmt.Sprintf("%dd %02dh %02dm %02ds", d, h, m, s)
	}
	return fmt.Sprintf("%02dh %02dm %02ds", h, m, s)
}
func (a *API) handleQwen(w http.ResponseWriter, r *http.Request) {
	log.Println("[QWEN] Request received")
	if r.Method != http.MethodPost {
		sendError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	var req struct {
		Prompt string `json:"prompt"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sendError(w, http.StatusBadRequest, "Invalid JSON payload")
		return
	}

	if req.Prompt == "" {
		sendError(w, http.StatusBadRequest, "Prompt is required")
		return
	}
    log.Println("[QWEN] Sending request to Ollama")
	answer, err := askQwen(req.Prompt)
	log.Println("[QWEN] Ollama response received")
	if err != nil {
		sendError(w, http.StatusInternalServerError, err.Error())
		return
	}

	sendJSON(w, http.StatusOK, map[string]interface{}{
		"answer": answer,
	})
}
