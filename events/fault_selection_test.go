package events

import (
	"testing"
)

func TestEvaluateFaultGroup_PrioritySelection(t *testing.T) {
	signals := []GroupSignal{
		{
			TagID:       "TEMP_STATOR_G1",
			TagName:     "Stator Winding Temp Sensor",
			Priority:    "WARNING",
			FaultCount:  10,
			Description: "Stator temp elevated at 85C",
		},
		{
			TagID:       "G1_CB",
			TagName:     "Generator Unit 1 Breaker",
			Priority:    "CRITICAL",
			FaultCount:  3,
			Description: "Generator breaker trip fault",
		},
	}

	result := EvaluateFaultGroup("GENERATOR SYSTEM", signals, "Last 10 minutes")

	if !result.DominantFound {
		t.Fatalf("Expected dominant signal to be found")
	}

	if result.PrimarySignal.TagID != "G1_CB" {
		t.Fatalf("Expected primary signal to be G1_CB (CRITICAL), got %s", result.PrimarySignal.TagID)
	}

	if !result.PriorityWinner {
		t.Fatalf("Expected PriorityWinner to be true")
	}
}

func TestEvaluateFaultGroup_FaultCountTieBreaker(t *testing.T) {
	signals := []GroupSignal{
		{
			TagID:       "TEMP_BEARING_G1",
			TagName:     "Generator 1 Bearing Temp",
			Priority:    "CRITICAL",
			FaultCount:  2,
			Description: "Bearing overtemp sensor",
		},
		{
			TagID:       "G1_CB",
			TagName:     "Generator Unit 1 Breaker",
			Priority:    "CRITICAL",
			FaultCount:  6,
			Description: "Generator breaker trip fault",
		},
	}

	result := EvaluateFaultGroup("GENERATOR SYSTEM", signals, "Last 10 minutes")

	if !result.DominantFound {
		t.Fatalf("Expected dominant signal to be found")
	}

	if result.PrimarySignal.TagID != "G1_CB" {
		t.Fatalf("Expected primary signal G1_CB (count 6 > 2), got %s", result.PrimarySignal.TagID)
	}

	if !result.FaultCountWinner {
		t.Fatalf("Expected FaultCountWinner to be true")
	}
}

func TestEvaluateFaultGroup_ExactTieNoDominant(t *testing.T) {
	signals := []GroupSignal{
		{
			TagID:       "PUMP_1",
			TagName:     "Cooling Pump 1",
			Priority:    "WARNING",
			FaultCount:  4,
			Description: "Pump trip",
		},
		{
			TagID:       "PUMP_2",
			TagName:     "Cooling Pump 2",
			Priority:    "WARNING",
			FaultCount:  4,
			Description: "Pump trip",
		},
	}

	result := EvaluateFaultGroup("COOLING SYSTEM", signals, "Last 10 minutes")

	if result.DominantFound {
		t.Fatalf("Expected no dominant signal on exact tie, but got dominant = true")
	}
}
