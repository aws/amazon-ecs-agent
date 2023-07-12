package engine

import (
	"testing"

	"github.com/aws/amazon-ecs-agent/agent/utils"
	"github.com/aws/amazon-ecs-agent/ecs-agent/ecs_client/model/ecs"
	"github.com/stretchr/testify/assert"
)

func getTestHostResourceManager(cpu int64, mem int64, ports []*string, portsUdp []*string, numGPUs int64) *HostResourceManager {
	hostResources := make(map[string]*ecs.Resource)
	hostResources["CPU"] = &ecs.Resource{
		Name:         utils.Strptr("CPU"),
		Type:         utils.Strptr("INTEGER"),
		IntegerValue: &cpu,
	}

	hostResources["MEMORY"] = &ecs.Resource{
		Name:         utils.Strptr("MEMORY"),
		Type:         utils.Strptr("INTEGER"),
		IntegerValue: &mem,
	}

	hostResources["PORTS_TCP"] = &ecs.Resource{
		Name:           utils.Strptr("PORTS_TCP"),
		Type:           utils.Strptr("STRINGSET"),
		StringSetValue: ports,
	}

	hostResources["PORTS_UDP"] = &ecs.Resource{
		Name:           utils.Strptr("PORTS_UDP"),
		Type:           utils.Strptr("STRINGSET"),
		StringSetValue: portsUdp,
	}

	hostResources["GPU"] = &ecs.Resource{
		Name:         utils.Strptr("PORTS_UDP"),
		Type:         utils.Strptr("STRINGSET"),
		IntegerValue: &numGPUs,
	}

	hostResourceManager := NewHostResourceManager(hostResources)

	return &hostResourceManager
}

func getTestTaskResourceMap(cpu int64, mem int64, ports []*string, portsUdp []*string, numGPUs int64) map[string]*ecs.Resource {
	taskResources := make(map[string]*ecs.Resource)
	taskResources["CPU"] = &ecs.Resource{
		Name:         utils.Strptr("CPU"),
		Type:         utils.Strptr("INTEGER"),
		IntegerValue: &cpu,
	}

	taskResources["MEMORY"] = &ecs.Resource{
		Name:         utils.Strptr("MEMORY"),
		Type:         utils.Strptr("INTEGER"),
		IntegerValue: &mem,
	}

	taskResources["PORTS_TCP"] = &ecs.Resource{
		Name:           utils.Strptr("PORTS_TCP"),
		Type:           utils.Strptr("STRINGSET"),
		StringSetValue: ports,
	}

	taskResources["PORTS_UDP"] = &ecs.Resource{
		Name:           utils.Strptr("PORTS_UDP"),
		Type:           utils.Strptr("STRINGSET"),
		StringSetValue: portsUdp,
	}

	taskResources["GPU"] = &ecs.Resource{
		Name:         utils.Strptr("GPU"),
		Type:         utils.Strptr("INTEGER"),
		IntegerValue: &numGPUs,
	}

	return taskResources
}
func TestHostResourceConsumeSuccess(t *testing.T) {
	hostResourcePort1 := "22"
	hostResourcePort2 := "1000"
	numGPUs := 4
	h := getTestHostResourceManager(int64(2048), int64(2048), []*string{&hostResourcePort1}, []*string{&hostResourcePort2}, int64(numGPUs))

	testTaskArn := "arn:aws:ecs:us-east-1:<aws_account_id>:task/cluster-name/11111"
	taskPort1 := "23"
	taskPort2 := "1001"
	taskResources := getTestTaskResourceMap(int64(512), int64(768), []*string{&taskPort1}, []*string{&taskPort2}, 1)

	consumed, _ := h.consume(testTaskArn, taskResources)
	assert.Equal(t, consumed, true, "Incorrect consumed status")
	assert.Equal(t, *h.consumedResource["CPU"].IntegerValue, int64(512), "Incorrect cpu resource accounting during consume")
	assert.Equal(t, *h.consumedResource["MEMORY"].IntegerValue, int64(768), "Incorrect memory resource accounting during consume")
	assert.Equal(t, *h.consumedResource["PORTS_TCP"].StringSetValue[0], "22", "Incorrect port resource accounting during consume")
	assert.Equal(t, *h.consumedResource["PORTS_TCP"].StringSetValue[1], "23", "Incorrect port resource accounting during consume")
	assert.Equal(t, len(h.consumedResource["PORTS_TCP"].StringSetValue), 2, "Incorrect port resource accounting during consume")
	assert.Equal(t, *h.consumedResource["PORTS_UDP"].StringSetValue[0], "1000", "Incorrect udp port resource accounting during consume")
	assert.Equal(t, *h.consumedResource["PORTS_UDP"].StringSetValue[1], "1001", "Incorrect udp port resource accounting during consume")
	assert.Equal(t, len(h.consumedResource["PORTS_UDP"].StringSetValue), 2, "Incorrect port resource accounting during consume")
	assert.Equal(t, *h.consumedResource["GPU"].IntegerValue, int64(1), "Incorrect gpu resource accounting during consume")
}
func TestHostResourceConsumeFail(t *testing.T) {
	hostResourcePort1 := "22"
	hostResourcePort2 := "1000"
	numGPUs := 4
	h := getTestHostResourceManager(int64(2048), int64(2048), []*string{&hostResourcePort1}, []*string{&hostResourcePort2}, int64(numGPUs))

	testTaskArn := "arn:aws:ecs:us-east-1:<aws_account_id>:task/cluster-name/11111"
	taskPort1 := "22"
	taskPort2 := "1001"
	taskResources := getTestTaskResourceMap(int64(512), int64(768), []*string{&taskPort1}, []*string{&taskPort2}, 1)

	consumed, _ := h.consume(testTaskArn, taskResources)
	assert.Equal(t, consumed, false, "Incorrect consumed status")
	assert.Equal(t, *h.consumedResource["CPU"].IntegerValue, int64(0), "Incorrect cpu resource accounting during consume")
	assert.Equal(t, *h.consumedResource["MEMORY"].IntegerValue, int64(0), "Incorrect memory resource accounting during consume")
	assert.Equal(t, *h.consumedResource["PORTS_TCP"].StringSetValue[0], "22", "Incorrect port resource accounting during consume")
	assert.Equal(t, len(h.consumedResource["PORTS_TCP"].StringSetValue), 1, "Incorrect port resource accounting during consume")
	assert.Equal(t, *h.consumedResource["PORTS_UDP"].StringSetValue[0], "1000", "Incorrect udp port resource accounting during consume")
	assert.Equal(t, len(h.consumedResource["PORTS_UDP"].StringSetValue), 1, "Incorrect port resource accounting during consume")
	assert.Equal(t, *h.consumedResource["GPU"].IntegerValue, int64(0), "Incorrect gpu resource accounting during consume")
}

func TestHostResourceRelease(t *testing.T) {
	hostResourcePort1 := "22"
	hostResourcePort2 := "1000"
	numGPUs := 4
	h := getTestHostResourceManager(int64(2048), int64(2048), []*string{&hostResourcePort1}, []*string{&hostResourcePort2}, int64(numGPUs))

	testTaskArn := "arn:aws:ecs:us-east-1:<aws_account_id>:task/cluster-name/11111"
	taskPort1 := "23"
	taskPort2 := "1001"
	taskResources := getTestTaskResourceMap(int64(512), int64(768), []*string{&taskPort1}, []*string{&taskPort2}, 1)

	h.consume(testTaskArn, taskResources)
	h.release(testTaskArn, taskResources)

	assert.Equal(t, *h.consumedResource["CPU"].IntegerValue, int64(0), "Incorrect cpu resource accounting during release")
	assert.Equal(t, *h.consumedResource["MEMORY"].IntegerValue, int64(0), "Incorrect memory resource accounting during release")
	assert.Equal(t, *h.consumedResource["PORTS_TCP"].StringSetValue[0], "22", "Incorrect port resource accounting during release")
	assert.Equal(t, len(h.consumedResource["PORTS_TCP"].StringSetValue), 1, "Incorrect port resource accounting during release")
	assert.Equal(t, *h.consumedResource["PORTS_UDP"].StringSetValue[0], "1000", "Incorrect udp port resource accounting during release")
	assert.Equal(t, len(h.consumedResource["PORTS_UDP"].StringSetValue), 1, "Incorrect udp port resource accounting during release")
	assert.Equal(t, *h.consumedResource["GPU"].IntegerValue, int64(0), "Incorrect gpu resource accounting during release")
}

func TestConsumable(t *testing.T) {
	hostResourcePort1 := "22"
	hostResourcePort2 := "1000"
	numGPUs := 4
	h := getTestHostResourceManager(int64(2048), int64(2048), []*string{&hostResourcePort1}, []*string{&hostResourcePort2}, int64(numGPUs))

	testCases := []struct {
		cpu           int64
		mem           int64
		ports         []uint16
		portsUdp      []uint16
		gpus          int64
		canBeConsumed bool
	}{
		{
			cpu:           int64(1024),
			mem:           int64(1024),
			ports:         []uint16{25},
			portsUdp:      []uint16{1003},
			gpus:          int64(2),
			canBeConsumed: true,
		},
		{
			cpu:           int64(2500),
			mem:           int64(1024),
			ports:         []uint16{},
			portsUdp:      []uint16{},
			gpus:          int64(0),
			canBeConsumed: false,
		},
		{
			cpu:           int64(1024),
			mem:           int64(2500),
			ports:         []uint16{},
			portsUdp:      []uint16{},
			gpus:          int64(0),
			canBeConsumed: false,
		},
		{
			cpu:           int64(1024),
			mem:           int64(1024),
			ports:         []uint16{22},
			portsUdp:      []uint16{},
			gpus:          int64(0),
			canBeConsumed: false,
		},
		{
			cpu:           int64(1024),
			mem:           int64(1024),
			ports:         []uint16{},
			portsUdp:      []uint16{1000},
			gpus:          int64(0),
			canBeConsumed: false,
		},
		{
			cpu:           int64(1024),
			mem:           int64(1024),
			ports:         []uint16{},
			portsUdp:      []uint16{1000},
			gpus:          int64(5),
			canBeConsumed: false,
		},
	}

	for _, tc := range testCases {
		resources := getTestTaskResourceMap(tc.cpu, tc.mem, utils.Uint16SliceToStringSlice(tc.ports), utils.Uint16SliceToStringSlice(tc.portsUdp), tc.gpus)
		canBeConsumed, err := h.consumable(resources)
		assert.Equal(t, canBeConsumed, tc.canBeConsumed, "Error in checking if resources can be successfully consumed")
		assert.Equal(t, err, nil, "Error in checking if resources can be successfully consumed, error returned from consumable")
	}
}
