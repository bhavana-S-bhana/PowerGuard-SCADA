package events

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestSlidingWindowEviction(t *testing.T) {
	var popupCounter int32
	tracker := NewFrequencyTracker(func(event Event, count int, group string) {
		atomic.AddInt32(&popupCounter, 1)
	}, WithWindowDuration(100*time.Millisecond), WithSuppressionDuration(50*time.Millisecond))

	now := time.Now()
	ev := Event{
		ID:        "TURBINE_TRIP",
		TagID:     "TURBINE_TRIP",
		Group:     "TURBINE",
		Priority:  "CRITICAL",
		Message:   "Turbine overspeed protection trip",
		Timestamp: now,
	}

	// Insert event
	topEv, count, triggered := tracker.RecordEvent(ev)
	if !triggered {
		t.Fatalf("Expected pop-up to trigger on first event")
	}
	if topEv.ID != "TURBINE_TRIP" || count != 1 {
		t.Fatalf("Expected top event TURBINE_TRIP with count 1, got %s with count %d", topEv.ID, count)
	}

	// Wait past 100ms window duration
	time.Sleep(120 * time.Millisecond)

	// Check count after eviction
	currentCount := tracker.GetEventCount("TURBINE", "TURBINE_TRIP")
	if currentCount != 0 {
		t.Fatalf("Expected count to be 0 after sliding window eviction, got %d", currentCount)
	}
}

func TestTopEventCalculation(t *testing.T) {
	var lastPopupEvent string
	var lastPopupCount int
	var mu sync.Mutex

	tracker := NewFrequencyTracker(func(event Event, count int, group string) {
		mu.Lock()
		defer mu.Unlock()
		lastPopupEvent = event.ID
		lastPopupCount = count
	})

	now := time.Now()

	// Insert 2 Pump Events
	for i := 0; i < 2; i++ {
		tracker.RecordEvent(Event{
			ID:        "PUMP_LOW_PRESS",
			TagID:     "PUMP_LOW_PRESS",
			Group:     "AUX_SYSTEM",
			Timestamp: now.Add(time.Duration(i) * time.Second),
		})
	}

	// Insert 5 Generator Events (higher frequency)
	for i := 0; i < 5; i++ {
		tracker.RecordEvent(Event{
			ID:        "GEN_OVERTEMP",
			TagID:     "GEN_OVERTEMP",
			Group:     "AUX_SYSTEM",
			Timestamp: now.Add(time.Duration(i+2) * time.Second),
		})
	}

	topEv, topCount, exists := tracker.GetTopEvent("AUX_SYSTEM")
	if !exists {
		t.Fatalf("Expected top event to exist")
	}

	if topEv.ID != "GEN_OVERTEMP" || topCount != 5 {
		t.Fatalf("Expected top event GEN_OVERTEMP with count 5, got %s with count %d", topEv.ID, topCount)
	}

	// Give callback goroutine time to run
	time.Sleep(20 * time.Millisecond)

	mu.Lock()
	if lastPopupEvent != "GEN_OVERTEMP" || lastPopupCount < 3 {
		t.Fatalf("Expected pop-up callback for GEN_OVERTEMP (count >= 3), got %s (count %d)", lastPopupEvent, lastPopupCount)
	}
	mu.Unlock()
}

func TestDuplicatePopupSuppression(t *testing.T) {
	var popupCount int32

	tracker := NewFrequencyTracker(func(event Event, count int, group string) {
		atomic.AddInt32(&popupCount, 1)
	}, WithWindowDuration(10*time.Minute), WithSuppressionDuration(10*time.Minute))

	now := time.Now()
	ev := Event{
		ID:        "GRID_FREQ_WARN",
		TagID:     "GRID_FREQ_WARN",
		Group:     "GRID",
		Priority:  "HIGH",
		Timestamp: now,
	}

	// First occurrence: triggers pop-up
	_, _, trig1 := tracker.RecordEvent(ev)
	if !trig1 {
		t.Fatalf("First event should trigger pop-up")
	}

	// Rapid repeated occurrences within suppression window: pop-up MUST BE PREVENTED
	for i := 1; i <= 5; i++ {
		ev.Timestamp = now.Add(time.Duration(i) * time.Second)
		_, _, trig := tracker.RecordEvent(ev)
		if trig {
			t.Fatalf("Duplicate pop-up triggered on occurrence %d within 10-minute suppression window!", i+1)
		}
	}

	time.Sleep(20 * time.Millisecond)

	if atomic.LoadInt32(&popupCount) != 1 {
		t.Fatalf("Expected exactly 1 pop-up trigger due to duplicate suppression, got %d", popupCount)
	}
}

func TestConcurrencySafety(t *testing.T) {
	tracker := NewFrequencyTracker(func(event Event, count int, group string) {})

	var wg sync.WaitGroup
	workers := 10
	eventsPerWorker := 100

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for j := 0; j < eventsPerWorker; j++ {
				tracker.RecordEvent(Event{
					ID:        "HIGH_CONCURRENCY_EVENT",
					Group:     "LOAD_TEST",
					Timestamp: time.Now(),
				})
			}
		}(i)
	}

	wg.Wait()

	totalCount := tracker.GetEventCount("LOAD_TEST", "HIGH_CONCURRENCY_EVENT")
	if totalCount != workers*eventsPerWorker {
		t.Fatalf("Expected total count %d, got %d", workers*eventsPerWorker, totalCount)
	}
}
