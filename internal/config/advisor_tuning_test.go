package config

import "testing"

func TestAdvisorTurnIntervalDefaultsToOne(t *testing.T) {
	var a Advisor
	if got := a.TurnInterval(); got != 1 {
		t.Fatalf("TurnInterval() = %d, want 1", got)
	}
}

func TestAdvisorTurnIntervalHonorsPositiveValue(t *testing.T) {
	a := Advisor{EveryNTurns: 5}
	if got := a.TurnInterval(); got != 5 {
		t.Fatalf("TurnInterval() = %d, want 5", got)
	}
}

func TestAdvisorTurnIntervalRejectsNonPositiveValue(t *testing.T) {
	a := Advisor{EveryNTurns: -3}
	if got := a.TurnInterval(); got != 1 {
		t.Fatalf("TurnInterval() = %d, want 1 for a negative configured value", got)
	}
}

func TestAdvisorNotifyThresholdDefaultsToConcern(t *testing.T) {
	var a Advisor
	if got := a.NotifyThreshold(); got != "CONCERN" {
		t.Fatalf("NotifyThreshold() = %q, want CONCERN", got)
	}
}

func TestAdvisorNotifyThresholdIsCaseInsensitive(t *testing.T) {
	a := Advisor{MinSeverity: "blocker"}
	if got := a.NotifyThreshold(); got != "BLOCKER" {
		t.Fatalf("NotifyThreshold() = %q, want BLOCKER", got)
	}
}

func TestAdvisorNotifyThresholdRejectsUnknownValue(t *testing.T) {
	a := Advisor{MinSeverity: "urgent"}
	if got := a.NotifyThreshold(); got != "CONCERN" {
		t.Fatalf("NotifyThreshold() = %q, want CONCERN for an unrecognized value", got)
	}
}

func TestAdvisorEscalateSeverityThresholdDefaultsToBlocker(t *testing.T) {
	var a Advisor
	if got := a.EscalateSeverityThreshold(); got != "BLOCKER" {
		t.Fatalf("EscalateSeverityThreshold() = %q, want BLOCKER", got)
	}
}

func TestAdvisorEscalateSeverityThresholdIsCaseInsensitive(t *testing.T) {
	a := Advisor{EscalateThreshold: "concern"}
	if got := a.EscalateSeverityThreshold(); got != "CONCERN" {
		t.Fatalf("EscalateSeverityThreshold() = %q, want CONCERN", got)
	}
}

func TestAdvisorEscalateSeverityThresholdRejectsUnknownValue(t *testing.T) {
	a := Advisor{EscalateThreshold: "urgent"}
	if got := a.EscalateSeverityThreshold(); got != "BLOCKER" {
		t.Fatalf("EscalateSeverityThreshold() = %q, want BLOCKER for an unrecognized value", got)
	}
}
