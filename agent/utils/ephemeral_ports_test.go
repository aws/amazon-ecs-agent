package utils

import (
	"math/rand"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestGenerateEphemeralPortNumbers(t *testing.T) {
	// This number is just to "stress" this test by increasing the changes of collision, which should not be a problem
	// in prod, only around a dozen ports will be needed in the worst case.
	expectedPortsGenerated := 1000
	var reservedPorts []uint16
	rand.Seed(time.Now().UnixNano())
	// Since absolute max containers for a task is 20, let's exclude 40 ports at random, 2 per container
	// in order to make a somewhat realistic test
	for i := 0; i < 40; i++ {
		port := uint16(rand.Intn(EphemeralPortMax-EphemeralPortMin+1) + EphemeralPortMin)
		reservedPorts = append(reservedPorts, port)
	}
	toExcludeSet := map[uint16]struct{}{}
	for _, e := range reservedPorts {
		toExcludeSet[e] = struct{}{}
	}
	ports, err := GenerateEphemeralPortNumbers(expectedPortsGenerated, reservedPorts)
	assert.NoError(t, err)
	assert.Len(t, ports, expectedPortsGenerated, "Not enough ports generated")
	for _, port := range ports {
		_, ok := toExcludeSet[port]
		assert.False(t, ok, "Port collision detected")
		assert.Conditionf(t, func() (success bool) {
			return port >= EphemeralPortMin && port <= EphemeralPortMax
		}, "Port was generated outside the ephemeral range [%d-%d]", EphemeralPortMin, EphemeralPortMax)
	}
}

func TestGenerateEphemeralPortNumbers_CollisionError(t *testing.T) {
	randIntFuncTmp := randIntFunc
	defer func() {
		randIntFunc = randIntFuncTmp
	}()
	// Inject mock rand.Int that always returns the same number in order to test max attempts
	randIntFunc = func(n int) int {
		return EphemeralPortMin
	}
	ports, err := GenerateEphemeralPortNumbers(100, []uint16{})
	assert.Nil(t, ports)
	assert.Error(t, err)
	assert.Equal(t, "maximum number of attempts to generate unique ports reached", err.Error())
}
