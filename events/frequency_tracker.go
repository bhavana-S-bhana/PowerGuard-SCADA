package events

import (
	"container/list"
	"sync"
	"time"
)

// Event represents a SCADA telemetry or system alert event.
type Event struct {
	ID        string                 `json:"id"`
	TagID     string                 `json:"tag_id"`
	TagName   string                 `json:"tag_name"`
	Group     string                 `json:"group"`
	Priority  string                 `json:"priority"`
	Message   string                 `json:"message"`
	Timestamp time.Time              `json:"timestamp"`
	AlarmID   int64                  `json:"alarm_id"`
	Data      map[string]interface{} `json:"data,omitempty"`
}

// windowItem encapsulates an event entry in the sliding window queue.
type windowItem struct {
	eventID   string
	group     string
	timestamp time.Time
}

// PopupCallback is the function signature executed when a top frequency event triggers a pop-up.
type PopupCallback func(event Event, count int, group string)

// FrequencyTracker manages rolling 10-minute sliding windows for events across groups,
// identifies top occurrence frequency events, and prevents duplicate pop-ups.
type FrequencyTracker struct {
	mu                  sync.RWMutex
	windowDuration      time.Duration
	suppressionDuration time.Duration
	minOccurrenceCount  int

	// Sliding window queue for O(1) eviction of expired events
	queue *list.List

	// Frequency counts per group: counts[group][eventID] = occurrenceCount
	counts map[string]map[string]int

	// Cache of latest event details by eventID
	eventCache map[string]Event

	// Timestamp of last triggered pop-up: lastPopupTriggered[group][eventID] = time.Time
	lastPopupTriggered map[string]map[string]time.Time

	// Last pop-up count triggered per event: lastPopupCount[group][eventID] = int
	lastPopupCount map[string]map[string]int

	// Registered pop-up callback handler
	popupCallback PopupCallback
}

// TrackerOption defines configuration options for FrequencyTracker.
type TrackerOption func(*FrequencyTracker)

// WithWindowDuration sets custom sliding window duration (default: 10 minutes).
func WithWindowDuration(d time.Duration) TrackerOption {
	return func(ft *FrequencyTracker) {
		ft.windowDuration = d
	}
}

// WithSuppressionDuration sets custom duplicate suppression window duration (default: 10 minutes).
func WithSuppressionDuration(d time.Duration) TrackerOption {
	return func(ft *FrequencyTracker) {
		ft.suppressionDuration = d
	}
}

// WithMinOccurrenceCount sets minimum frequency needed to trigger a pop-up (default: 1).
func WithMinOccurrenceCount(min int) TrackerOption {
	return func(ft *FrequencyTracker) {
		ft.minOccurrenceCount = min
	}
}

// NewFrequencyTracker initializes an efficient 10-minute sliding window frequency tracker.
func NewFrequencyTracker(callback PopupCallback, opts ...TrackerOption) *FrequencyTracker {
	ft := &FrequencyTracker{
		windowDuration:      10 * time.Minute,
		suppressionDuration: 10 * time.Minute,
		minOccurrenceCount:  1,
		queue:               list.New(),
		counts:              make(map[string]map[string]int),
		eventCache:          make(map[string]Event),
		lastPopupTriggered:  make(map[string]map[string]time.Time),
		lastPopupCount:      make(map[string]map[string]int),
		popupCallback:       callback,
	}

	for _, opt := range opts {
		opt(ft)
	}

	return ft
}

// RecordEvent inserts an incoming event into the rolling window, evicts expired events,
// calculates the top event for the group, and triggers a pop-up if duplicate suppression permits.
func (ft *FrequencyTracker) RecordEvent(event Event) (topEvent Event, topCount int, triggered bool) {
	ft.mu.Lock()

	now := event.Timestamp
	if now.IsZero() {
		now = time.Now()
		event.Timestamp = now
	}

	// Use TagID or ID as event identifier
	if event.ID == "" {
		event.ID = event.TagID
	}
	if event.Group == "" {
		event.Group = "DEFAULT_GROUP"
	}

	group := event.Group

	// 1. Evict expired entries older than 10-minute sliding window
	ft.evictExpiredLocked(now)

	// 2. Insert new event into queue and increment count
	ft.queue.PushBack(windowItem{
		eventID:   event.ID,
		group:     group,
		timestamp: now,
	})

	if ft.counts[group] == nil {
		ft.counts[group] = make(map[string]int)
	}
	ft.counts[group][event.ID]++
	ft.eventCache[event.ID] = event

	// 3. Identify single event with highest occurrence count in the group window
	topID, topCount := ft.calculateTopEventLocked(group)
	if topID == "" || topCount < ft.minOccurrenceCount {
		ft.mu.Unlock()
		return Event{}, 0, false
	}

	topEv := ft.eventCache[topID]

	// 4. Duplicate Pop-up Prevention Guard
	if ft.shouldTriggerPopupLocked(group, topID, topCount, now) {
		if ft.lastPopupTriggered[group] == nil {
			ft.lastPopupTriggered[group] = make(map[string]time.Time)
		}
		if ft.lastPopupCount[group] == nil {
			ft.lastPopupCount[group] = make(map[string]int)
		}

		ft.lastPopupTriggered[group][topID] = now
		ft.lastPopupCount[group][topID] = topCount
		triggered = true
	}

	cb := ft.popupCallback
	ft.mu.Unlock()

	if triggered && cb != nil {
		cb(topEv, topCount, group)
	}

	return topEv, topCount, triggered
}

// evictExpiredLocked purges elements outside the rolling window duration. Must be called with lock held.
func (ft *FrequencyTracker) evictExpiredLocked(now time.Time) {
	cutoff := now.Add(-ft.windowDuration)

	for ft.queue.Len() > 0 {
		front := ft.queue.Front()
		item, ok := front.Value.(windowItem)
		if !ok || item.timestamp.After(cutoff) {
			break // Front element is within active window
		}

		// Evict element from queue
		ft.queue.Remove(front)

		// Decrement occurrence count
		if grpCounts, exists := ft.counts[item.group]; exists {
			grpCounts[item.eventID]--
			if grpCounts[item.eventID] <= 0 {
				delete(grpCounts, item.eventID)
			}
			if len(grpCounts) == 0 {
				delete(ft.counts, item.group)
			}
		}
	}
}

// calculateTopEventLocked finds the event with the highest occurrence count in a group window.
func (ft *FrequencyTracker) calculateTopEventLocked(group string) (string, int) {
	grpCounts, exists := ft.counts[group]
	if !exists || len(grpCounts) == 0 {
		return "", 0
	}

	var topID string
	maxCount := 0

	for eventID, count := range grpCounts {
		if count > maxCount {
			maxCount = count
			topID = eventID
		}
	}

	return topID, maxCount
}

// shouldTriggerPopupLocked checks duplicate pop-up suppression conditions.
func (ft *FrequencyTracker) shouldTriggerPopupLocked(group, eventID string, count int, now time.Time) bool {
	lastTime, exists := ft.lastPopupTriggered[group][eventID]
	if !exists {
		// Event has never triggered a pop-up in this window -> ALLOW
		return true
	}

	// If suppression window (10 mins) has elapsed since last popup -> ALLOW
	if now.Sub(lastTime) >= ft.suppressionDuration {
		return true
	}

	// Duplicate detected within suppression window -> PREVENT POP-UP
	return false
}

// GetTopEvent queries the current single top event and occurrence count for a group without modifying state.
func (ft *FrequencyTracker) GetTopEvent(group string) (Event, int, bool) {
	ft.mu.Lock()
	defer ft.mu.Unlock()

	ft.evictExpiredLocked(time.Now())
	topID, count := ft.calculateTopEventLocked(group)
	if topID == "" {
		return Event{}, 0, false
	}
	return ft.eventCache[topID], count, true
}

// GetEventCount returns the current occurrence count for a specific event in the sliding window.
func (ft *FrequencyTracker) GetEventCount(group, eventID string) int {
	ft.mu.Lock()
	defer ft.mu.Unlock()

	ft.evictExpiredLocked(time.Now())

	if grpCounts, exists := ft.counts[group]; exists {
		return grpCounts[eventID]
	}
	return 0
}

// Clear resets the tracking state.
func (ft *FrequencyTracker) Clear() {
	ft.mu.Lock()
	defer ft.mu.Unlock()

	ft.queue.Init()
	ft.counts = make(map[string]map[string]int)
	ft.eventCache = make(map[string]Event)
	ft.lastPopupTriggered = make(map[string]map[string]time.Time)
	ft.lastPopupCount = make(map[string]map[string]int)
}
