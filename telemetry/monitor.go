package telemetry

import (
	"fmt"
	"log"
	"math/rand"
	"sync"
	"time"

	"power-plant-scada/database"
	"power-plant-scada/websocket"
)

type Monitor struct {
	db               *database.DB
	hub              *websocket.Hub
	cache            *StateCache
	pollInterval     time.Duration
	stopChan         chan struct{}
	simActive        bool
	simMu            sync.Mutex
	lastStateToggles map[string]time.Time
}

func NewMonitor(db *database.DB, hub *websocket.Hub, pollInterval time.Duration) *Monitor {
	return &Monitor{
		db:               db,
		hub:              hub,
		cache:            NewStateCache(),
		pollInterval:     pollInterval,
		stopChan:         make(chan struct{}),
		simActive:        true, // Default telemetry simulation enabled
		lastStateToggles: make(map[string]time.Time),
	}
}

func (m *Monitor) Start() {
	log.Printf("[Telemetry Monitor] Launching dedicated background monitoring goroutine (poll interval: %v)...", m.pollInterval)

	// 1. Initial State Preload
	tags, err := m.db.GetAllTags()
	if err == nil {
		m.cache.LoadAll(tags)
		log.Printf("[Telemetry Monitor] Initialized in-memory state cache with %d tags.", len(tags))
	} else {
		log.Printf("[Telemetry Monitor] Warning preloading initial tags: %v", err)
	}

	// 2. Launch Polling Goroutine
	go m.pollLoop()

	// 3. Launch Simulation Goroutine
	go m.simLoop()
}

func (m *Monitor) Stop() {
	close(m.stopChan)
}

func (m *Monitor) IsSimActive() bool {
	m.simMu.Lock()
	defer m.simMu.Unlock()
	return m.simActive
}

func (m *Monitor) SetSimActive(active bool) {
	m.simMu.Lock()
	m.simActive = active
	m.simMu.Unlock()

	status := "DISABLED"
	if active {
		status = "ENABLED"
	}
	log.Printf("[Telemetry Simulator] Simulation mode set to: %s", status)

	m.hub.Broadcast(websocket.EventPayload{
		Type:      "SIMULATION_STATUS",
		Timestamp: time.Now(),
		Data:      map[string]interface{}{"active": active},
	})
}

func (m *Monitor) GetCache() *StateCache {
	return m.cache
}

func (m *Monitor) pollLoop() {
	ticker := time.NewTicker(m.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-m.stopChan:
			log.Println("[Telemetry Monitor] Monitoring goroutine shutdown.")
			return
		case <-ticker.C:
			m.checkStateChanges()
		}
	}
}

func (m *Monitor) checkStateChanges() {
	dbTags, err := m.db.GetAllTags()
	if err != nil {
		log.Printf("[Telemetry Monitor] Error querying database tags: %v", err)
		return
	}

	for _, dbTag := range dbTags {
		cachedTag, exists := m.cache.Get(dbTag.TagID)
		if !exists {
			m.cache.Set(dbTag)
			continue
		}

		// Detect Instant State Transition (0 -> 1 or 1 -> 0)
		if cachedTag.State != dbTag.State {
			prevState := cachedTag.State
			currState := dbTag.State
			now := time.Now()

			// Update in-memory state cache immediately
			m.cache.Set(dbTag)

			log.Printf("[TELEMETRY ALERT] Instant Binary State Transition Detected on Tag '%s' (%s): %d -> %d [Priority: %s]",
				dbTag.TagID, dbTag.Name, prevState, currState, dbTag.Priority)

			// Generate Alarm description
			stateDesc := "CLOSED / ENERGIZED / ACTIVE / ALARM"
			if currState == 0 {
				stateDesc = "OPEN / TRIPPED / DE-ENERGIZED / NORMAL"
			}
			alarmMsg := fmt.Sprintf("%s changed state from %d to %d (%s)", dbTag.Name, prevState, currState, stateDesc)

			// Create Alarm entry in SQLite
			alarm, err := m.db.CreateAlarm(dbTag.TagID, dbTag.Name, dbTag.Priority, alarmMsg, prevState, currState)
			if err != nil {
				log.Printf("[Telemetry Monitor] Error creating alarm record: %v", err)
			}

			var alarmID int64 = 0
			if alarm != nil {
				alarmID = alarm.ID
			}

			// Broadcast high-priority WebSocket event payload immediately
			m.hub.Broadcast(websocket.EventPayload{
				Type:      "BINARY_STATE_CHANGE",
				TagID:     dbTag.TagID,
				TagName:   dbTag.Name,
				PrevState: prevState,
				CurrState: currState,
				Priority:  dbTag.Priority,
				Timestamp: now,
				AlarmID:   alarmID,
				Data: map[string]interface{}{
					"tag":      dbTag,
					"message":  alarmMsg,
					"alarm_id": alarmID,
				},
			})
		}
	}
}

// Background simulation ticker simulating live power plant binary state toggles across 30 tags
func (m *Monitor) simLoop() {
	simTicker := time.NewTicker(10 * time.Second)
	defer simTicker.Stop()

	simTargets := []string{
		"V_AUX_COOLING",
		"TR2_CB",
		"GRID_FREQ_RELAY",
		"COOLING_PUMP_1",
		"COOLING_PUMP_2",
		"TEMP_BEARING_G1",
		"TEMP_BEARING_G2",
		"TEMP_TRANSFORMER_TR1",
		"PRESS_STEAM_DRUM_HI",
		"PRESS_LUBE_OIL_LOW",
		"V_REHEAT_STEAM",
		"BUS_BAR_2_CB",
		"GRID_FEED_CB1",
	}

	for {
		select {
		case <-m.stopChan:
			return
		case <-simTicker.C:
			if !m.IsSimActive() {
				continue
			}

			targetID := simTargets[rand.Intn(len(simTargets))]

			if last, ok := m.lastStateToggles[targetID]; ok && time.Since(last) < 18*time.Second {
				continue
			}

			tag, err := m.db.GetTagByID(targetID)
			if err != nil {
				continue
			}

			newState := 0
			if tag.State == 0 {
				newState = 1
			}

			m.lastStateToggles[targetID] = time.Now()

			log.Printf("[SIMULATOR] Automated telemetry simulation toggling tag %s from %d -> %d", targetID, tag.State, newState)
			_, _ = m.db.SystemSetTagState(targetID, newState)
		}
	}
}
