package switcher

import "testing"

func TestHealthyIPsRequiresAPIAndManifest(t *testing.T) {
	healthy := HealthyIPs(
		[]HealthSample{
			{IP: "10.0.0.1", Value: 1},
			{IP: "10.0.0.2", Value: 1},
		},
		[]HealthSample{
			{IP: "10.0.0.1", Value: 1},
			{IP: "10.0.0.2", Value: 0},
		},
	)

	if _, exists := healthy["10.0.0.1"]; !exists {
		t.Fatal("10.0.0.1 should be healthy")
	}

	if _, exists := healthy["10.0.0.2"]; exists {
		t.Fatal("10.0.0.2 should not be healthy")
	}
}

func TestSelectTargetUsesHighestPriority(t *testing.T) {
	selected, err := SelectTarget(
		[]Target{
			{IP: "10.0.0.1", Priority: 10, Enabled: true},
			{IP: "10.0.0.2", Priority: 20, Enabled: true},
			{IP: "10.0.0.3", Priority: 30, Enabled: false},
		},
		map[string]struct{}{
			"10.0.0.1": {},
			"10.0.0.2": {},
			"10.0.0.3": {},
		},
	)
	if err != nil {
		t.Fatalf("SelectTarget returned error: %v", err)
	}

	if selected.IP != "10.0.0.2" {
		t.Fatalf("selected IP = %q, want 10.0.0.2", selected.IP)
	}
}

func TestSelectTargetUsesLowestLatencyForPriorityTie(t *testing.T) {
	selected, err := SelectTargetWithPolicy(
		[]Target{
			{IP: "10.0.0.1", Priority: 10, Enabled: true},
			{IP: "10.0.0.2", Priority: 10, Enabled: true},
		},
		map[string]struct{}{
			"10.0.0.1": {},
			"10.0.0.2": {},
		},
		SelectionPolicy{
			TieBreaker: "latency",
			Latencies: map[string]float64{
				"10.0.0.1": 0.3,
				"10.0.0.2": 0.1,
			},
		},
	)
	if err != nil {
		t.Fatalf("SelectTargetWithPolicy returned error: %v", err)
	}

	if selected.IP != "10.0.0.2" {
		t.Fatalf("selected IP = %q, want 10.0.0.2", selected.IP)
	}
}

func TestSelectTargetKeepsOrderWhenPriorityAndLatencyTie(t *testing.T) {
	selected, err := SelectTargetWithPolicy(
		[]Target{
			{IP: "10.0.0.1", Priority: 10, Enabled: true},
			{IP: "10.0.0.2", Priority: 10, Enabled: true},
		},
		map[string]struct{}{
			"10.0.0.1": {},
			"10.0.0.2": {},
		},
		SelectionPolicy{
			TieBreaker: "latency",
			Latencies: map[string]float64{
				"10.0.0.1": 0.1,
				"10.0.0.2": 0.1,
			},
		},
	)
	if err != nil {
		t.Fatalf("SelectTargetWithPolicy returned error: %v", err)
	}

	if selected.IP != "10.0.0.1" {
		t.Fatalf("selected IP = %q, want 10.0.0.1", selected.IP)
	}
}
