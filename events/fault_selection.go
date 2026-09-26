package events

import (
	"fmt"
	"sort"
	"strings"
)

// Priority ranks for comparison: CRITICAL > WARNING > INFO
var priorityRank = map[string]int{
	"CRITICAL": 3,
	"WARNING":  2,
	"INFO":     1,
}

// GroupSignal represents a signal being evaluated within a group.
type GroupSignal struct {
	TagID       string
	TagName     string
	Priority    string
	FaultCount  int
	Description string
}

// FaultEvaluationResult contains the output of the two-step validation check.
type FaultEvaluationResult struct {
	DominantFound     bool
	Group             string
	PrimarySignal     GroupSignal
	OtherSignals      []GroupSignal
	PriorityWinner    bool
	FaultCountWinner  bool
	SelectionSummary  string
	UIPopupBox        string
	AnalysisPanelData string
}

// EvaluateFaultGroup executes Step 1 (Priority check & Fault Count tie-breaker)
// and Step 2 (Conditional UI payload generation).
func EvaluateFaultGroup(groupName string, signals []GroupSignal, windowDesc string) FaultEvaluationResult {
	if len(signals) == 0 {
		return FaultEvaluationResult{
			DominantFound:    false,
			Group:            groupName,
			SelectionSummary: "No active alarms exist in group. Updates suppressed.",
		}
	}

	if windowDesc == "" {
		windowDesc = "Last 10 minutes"
	}

	// Make a copy of signals for sorting
	sorted := make([]GroupSignal, len(signals))
	copy(sorted, signals)

	// Sort by Priority Rank DESC, then FaultCount DESC
	sort.Slice(sorted, func(i, j int) bool {
		pI := priorityRank[strings.ToUpper(sorted[i].Priority)]
		pJ := priorityRank[strings.ToUpper(sorted[j].Priority)]

		if pI != pJ {
			return pI > pJ
		}
		return sorted[i].FaultCount > sorted[j].FaultCount
	})

	topSignal := sorted[0]

	// Step 1: Verification of dominance
	// Check if top signal uniquely strictly dominates candidate #2
	dominant := true
	priorityWinner := false
	faultCountWinner := false

	if len(sorted) > 1 {
		secondSignal := sorted[1]

		pTop := priorityRank[strings.ToUpper(topSignal.Priority)]
		pSec := priorityRank[strings.ToUpper(secondSignal.Priority)]

		if pTop > pSec {
			dominant = true
			priorityWinner = true
		} else if pTop == pSec {
			if topSignal.FaultCount > secondSignal.FaultCount {
				dominant = true
				faultCountWinner = true
			} else {
				// Exact tie in both priority and fault count -> no single dominant signal
				dominant = false
			}
		} else {
			dominant = false
		}
	} else {
		// Only 1 signal present in group -> dominant by default
		dominant = true
		priorityWinner = true
	}

	// STEP 2: Conditional Action Execution
	if !dominant {
		return FaultEvaluationResult{
			DominantFound:    false,
			Group:            groupName,
			SelectionSummary: "Evaluation failed to identify a single dominant signal due to an exact priority and fault count tie. UI updates suppressed.",
		}
	}

	others := sorted[1:]

	// Build Selection Summary
	var reasonSummary string
	if priorityWinner {
		reasonSummary = fmt.Sprintf("Selected '%s' because its priority (%s) strictly exceeded all other signals in %s.", topSignal.TagName, topSignal.Priority, groupName)
	} else if faultCountWinner {
		reasonSummary = fmt.Sprintf("Selected '%s' after resolving priority tie (%s) via highest fault count (%d occurrences in %s).", topSignal.TagName, topSignal.Priority, topSignal.FaultCount, windowDesc)
	}

	// Format Output 1: UI Alert Popup
	popup := fmt.Sprintf(
		"┌─────────────────────────────┐\n"+
			"│ %-22s ×   │\n"+
			"│                             │\n"+
			"│ %-27s │\n"+
			"│ %-27s │\n"+
			"│                             │\n"+
			"│ %-27s │\n"+
			"│                             │\n"+
			"│ WHY SELECTED?               │\n"+
			"│ 🔴 %-25s │\n"+
			"│ 🔥 %-25s │\n"+
			"└─────────────────────────────┘",
		truncate(groupName, 22),
		truncate(topSignal.TagName, 27),
		truncate(fmt.Sprintf("[%s]", strings.ToUpper(topSignal.Priority)), 27),
		truncate(topSignal.Description, 27),
		truncate(fmt.Sprintf("Priority: %s", strings.ToUpper(topSignal.Priority)), 25),
		truncate(fmt.Sprintf("%d Faults in Window", topSignal.FaultCount), 25),
	)

	// Format Output 2: Fault Analysis Panel Data
	var otherList []string
	if len(others) == 0 {
		otherList = append(otherList, "None (Single active signal in group)")
	} else {
		for _, o := range others {
			otherList = append(otherList, fmt.Sprintf("- %s [%s] — %d occurrences", o.TagName, strings.ToUpper(o.Priority), o.FaultCount))
		}
	}

	panelData := fmt.Sprintf(
		"- System / Group Name: %s\n"+
			"- Primary Suspected Signal: %s [%s]\n"+
			"- Selection Parameters:\n"+
			"  - Priority: %s\n"+
			"  - Fault Count: %d occurrences\n"+
			"  - Time Window: %s\n"+
			"- Other Signals in Group:\n%s\n"+
			"- Selection Logic Summary: %s",
		groupName,
		topSignal.TagName,
		strings.ToUpper(topSignal.Priority),
		strings.ToUpper(topSignal.Priority),
		topSignal.FaultCount,
		windowDesc,
		strings.Join(otherList, "\n"),
		reasonSummary,
	)

	return FaultEvaluationResult{
		DominantFound:     true,
		Group:             groupName,
		PrimarySignal:     topSignal,
		OtherSignals:      others,
		PriorityWinner:    priorityWinner,
		FaultCountWinner:  faultCountWinner,
		SelectionSummary:  reasonSummary,
		UIPopupBox:        popup,
		AnalysisPanelData: panelData,
	}
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}
