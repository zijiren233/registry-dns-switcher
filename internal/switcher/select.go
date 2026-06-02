package switcher

import (
	"fmt"
	"net"
)

type Target struct {
	IP       string
	Priority int
	Enabled  bool
}

type HealthSample struct {
	IP    string
	Value float64
}

func HealthyIPs(apiSamples, manifestSamples []HealthSample) map[string]struct{} {
	apiOK := okIPs(apiSamples)
	manifestOK := okIPs(manifestSamples)

	healthy := make(map[string]struct{})
	for ip := range apiOK {
		if _, exists := manifestOK[ip]; exists {
			healthy[ip] = struct{}{}
		}
	}
	return healthy
}

func SelectTarget(targets []Target, healthy map[string]struct{}) (Target, error) {
	var selected Target
	found := false

	for _, target := range targets {
		if !target.Enabled {
			continue
		}
		if net.ParseIP(target.IP) == nil {
			return Target{}, fmt.Errorf("invalid target IP %q", target.IP)
		}
		if _, exists := healthy[target.IP]; !exists {
			continue
		}
		if !found || target.Priority > selected.Priority {
			selected = target
			found = true
		}
	}

	if !found {
		return Target{}, fmt.Errorf("no healthy target found")
	}
	return selected, nil
}

func okIPs(samples []HealthSample) map[string]struct{} {
	result := make(map[string]struct{})
	for _, sample := range samples {
		if sample.Value == 1 {
			result[sample.IP] = struct{}{}
		}
	}
	return result
}
