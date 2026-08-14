package telemetry

import (
	"sync"
	"time"

	"power-plant-scada/database"
)

type StateCache struct {
	mu    sync.RWMutex
	cache map[string]database.Tag
}

func NewStateCache() *StateCache {
	return &StateCache{
		cache: make(map[string]database.Tag),
	}
}

func (sc *StateCache) LoadAll(tags []database.Tag) {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	for _, t := range tags {
		sc.cache[t.TagID] = t
	}
}

func (sc *StateCache) Get(tagID string) (database.Tag, bool) {
	sc.mu.RLock()
	defer sc.mu.RUnlock()
	t, ok := sc.cache[tagID]
	return t, ok
}

func (sc *StateCache) Set(t database.Tag) {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	sc.cache[t.TagID] = t
}

func (sc *StateCache) GetAll() []database.Tag {
	sc.mu.RLock()
	defer sc.mu.RUnlock()
	var tags []database.Tag
	for _, t := range sc.cache {
		tags = append(tags, t)
	}
	return tags
}

type TelemetryEvent struct {
	TagID     string
	TagName   string
	PrevState int
	CurrState int
	Priority  string
	IsManual  bool
	Timestamp time.Time
}
