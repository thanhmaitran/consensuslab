package tests

import (
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/thanhmaitran/consensuslab/internal/sim"
)

// TestRandomScenarios runs seeded random fault schedules. A failing seed is
// printed so the exact run can be replayed.
func TestRandomScenarios(t *testing.T) {
	seeds := 30
	if testing.Short() {
		seeds = 5
	}
	for seed := int64(1); seed <= int64(seeds); seed++ {
		seed := seed
		t.Run(fmt.Sprintf("seed-%d", seed), func(t *testing.T) {
			rng := rand.New(rand.NewSource(seed))
			nodes := 3 + 2*rng.Intn(3)
			e := newEngine(t, nodes, seed, func(c *sim.Config) {
				c.Network = netMillis(float64(5+rng.Intn(40)), float64(rng.Intn(20)), rng.Float64()*0.2)
			})
			for at := 500; at < 25000; at += 200 {
				_, _ = e.ScheduleCommand(time.Duration(at)*time.Millisecond, "", fmt.Sprintf("SET k%d v%d", at, seed))
			}
			for i := 0; i < 10; i++ {
				at := time.Duration(1000+rng.Intn(23000)) * time.Millisecond
				switch rng.Intn(3) {
				case 0:
					id := sim.NodeName(1 + rng.Intn(nodes))
					_ = e.ScheduleCrash(at, id, time.Duration(200+rng.Intn(3000))*time.Millisecond)
				case 1:
					_ = e.ScheduleCrash(at, "", time.Duration(200+rng.Intn(3000))*time.Millisecond)
				default:
					_ = e.SchedulePartitionSizes(at, randomSizes(rng, nodes), rng.Intn(2) == 0)
					e.ScheduleHeal(at + time.Duration(500+rng.Intn(3500))*time.Millisecond)
				}
			}
			e.RunUntil(30 * time.Second)
			mustPass(t, e)
		})
	}
}
