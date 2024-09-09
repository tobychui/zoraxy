package loadbalance

import (
	"fmt"
	"math"
	"math/rand"
	"testing"
	"time"
)

// func getRandomUpstreamByWeight(upstreams []*Upstream) (*Upstream, int, error) { ... }
func TestRandomUpstreamSelection(t *testing.T) {
	rand.Seed(time.Now().UnixNano()) // Seed for randomness

	// Define some test upstreams
	upstreams := []*Upstream{
		{
			OriginIpOrDomain:         "192.168.1.1:8080",
			RequireTLS:               false,
			SkipCertValidations:      false,
			SkipWebSocketOriginCheck: false,
			Weight:                   1,
			MaxConn:                  0, // No connection limit for now
		},
		{
			OriginIpOrDomain:         "192.168.1.2:8080",
			RequireTLS:               false,
			SkipCertValidations:      false,
			SkipWebSocketOriginCheck: false,
			Weight:                   1,
			MaxConn:                  0,
		},
		{
			OriginIpOrDomain:         "192.168.1.3:8080",
			RequireTLS:               true,
			SkipCertValidations:      true,
			SkipWebSocketOriginCheck: true,
			Weight:                   1,
			MaxConn:                  0,
		},
		{
			OriginIpOrDomain:         "192.168.1.4:8080",
			RequireTLS:               true,
			SkipCertValidations:      true,
			SkipWebSocketOriginCheck: true,
			Weight:                   1,
			MaxConn:                  0,
		},
	}

	// Track how many times each upstream is selected
	selectionCount := make(map[string]int)
	totalPicks := 10000 // Number of times to call getRandomUpstreamByWeight
	//expectedPickCount := totalPicks / len(upstreams) // Ideal count for each upstream

	// Pick upstreams and record their selection count
	for i := 0; i < totalPicks; i++ {
		upstream, _, err := getRandomUpstreamByWeight(upstreams)
		if err != nil {
			t.Fatalf("Error getting random upstream: %v", err)
		}
		selectionCount[upstream.OriginIpOrDomain]++
	}

	// Condition 1: Ensure every upstream has been picked at least once
	for _, upstream := range upstreams {
		if selectionCount[upstream.OriginIpOrDomain] == 0 {
			t.Errorf("Upstream %s was never selected", upstream.OriginIpOrDomain)
		}
	}

	// Condition 2: Check that the distribution is within 1-2 standard deviations
	counts := make([]float64, len(upstreams))
	for i, upstream := range upstreams {
		counts[i] = float64(selectionCount[upstream.OriginIpOrDomain])
	}

	mean := float64(totalPicks) / float64(len(upstreams))
	stddev := calculateStdDev(counts, mean)

	tolerance := 2 * stddev // Allowing up to 2 standard deviations
	for i, count := range counts {
		if math.Abs(count-mean) > tolerance {
			t.Errorf("Selection of upstream %s is outside acceptable range: %v picks (mean: %v, stddev: %v)", upstreams[i].OriginIpOrDomain, count, mean, stddev)
		}
	}

	fmt.Println("Selection count:", selectionCount)
	fmt.Printf("Mean: %.2f, StdDev: %.2f\n", mean, stddev)
}

// Helper function to calculate standard deviation
func calculateStdDev(data []float64, mean float64) float64 {
	var sumOfSquares float64
	for _, value := range data {
		sumOfSquares += (value - mean) * (value - mean)
	}
	variance := sumOfSquares / float64(len(data))
	return math.Sqrt(variance)
}
