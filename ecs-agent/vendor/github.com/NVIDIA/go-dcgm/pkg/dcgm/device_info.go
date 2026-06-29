package dcgm

/*
#include "dcgm_agent.h"
#include "dcgm_structs.h"
*/
import "C"

import (
	"fmt"
	"log"
	"math/rand"
	"unsafe"

	"github.com/bits-and-blooms/bitset"
)

// PCIInfo contains PCI bus related information for a GPU device
type PCIInfo struct {
	BusID     string
	BAR1      uint  // MB
	FBTotal   uint  // MB
	Bandwidth int64 // MB/s
}

// DeviceIdentifiers contains various identification information for a GPU device
type DeviceIdentifiers struct {
	Brand               string
	Model               string
	Serial              string
	Vbios               string
	InforomImageVersion string
	DriverVersion       string
}

// Device represents a GPU device and its properties
type Device struct {
	GPU           uint
	DCGMSupported string
	UUID          string
	Power         uint // W
	PCI           PCIInfo
	Identifiers   DeviceIdentifiers
	Topology      []P2PLink
	CPUAffinity   string
}

// NvLinkP2PStatus represents the state of NvLinks between the GPU pairs
type NvLinkP2PStatus struct {
	numGpus uint
	Gpus    [][]Link_State
}

// getAllDeviceCount counts all GPUs on the system
func getAllDeviceCount() (gpuCount uint, err error) {
	var (
		gpuIDList [C.DCGM_MAX_NUM_DEVICES]C.uint
		count     C.int
	)

	result := C.dcgmGetAllDevices(handle.handle, &gpuIDList[0], &count)
	if err = errorString(result); err != nil {
		return gpuCount, fmt.Errorf("error getting devices count: %s", err)
	}
	gpuCount = uint(count)
	return
}

// getAllDeviceCount counts all GPUs on the system
func getEntityGroupEntities(entityGroup Field_Entity_Group) ([]uint, error) {
	var err error
	var pEntities [C.DCGM_GROUP_MAX_ENTITIES_V2]C.uint
	var count C.int = C.DCGM_GROUP_MAX_ENTITIES_V2

	result := C.dcgmGetEntityGroupEntities(handle.handle, C.dcgm_field_entity_group_t(entityGroup), &pEntities[0], &count, 0)
	if err = errorString(result); err != nil {
		return nil, fmt.Errorf("error getting entity count: %s", err)
	}

	entities := make([]uint, count)

	for i := 0; i < int(count); i++ {
		entities[i] = uint(pEntities[i])
	}
	return entities, nil
}

// getSupportedDevices returns DCGM supported GPUs
func getSupportedDevices() (gpus []uint, err error) {
	var gpuIDList [C.DCGM_MAX_NUM_DEVICES]C.uint
	var count C.int

	result := C.dcgmGetAllSupportedDevices(handle.handle, &gpuIDList[0], &count)
	if err = errorString(result); err != nil {
		return gpus, &Error{msg: C.GoString(C.errorString(result)), Code: result}
	}

	numGpus := int(count)
	gpus = make([]uint, numGpus)
	for i := 0; i < numGpus; i++ {
		gpus[i] = uint(gpuIDList[i])
	}
	return
}

func getPciBandwidth(gpuID uint) (int64, error) {
	const (
		maxLinkGen int = iota
		maxLinkWidth
		fieldsCount
	)

	pciFields := make([]Short, fieldsCount)
	pciFields[maxLinkGen] = C.DCGM_FI_DEV_PCIE_MAX_LINK_GEN
	pciFields[maxLinkWidth] = C.DCGM_FI_DEV_PCIE_MAX_LINK_WIDTH

	fieldsName := fmt.Sprintf("pciBandwidthFields%d", rand.Uint64())

	fieldsID, err := FieldGroupCreate(fieldsName, pciFields)
	if err != nil {
		return 0, err
	}

	groupName := fmt.Sprintf("pciBandwidth%d", rand.Uint64())
	groupID, err := WatchFields(gpuID, fieldsID, groupName)
	if err != nil {
		_ = FieldGroupDestroy(fieldsID)
		return 0, err
	}

	values, err := GetLatestValuesForFields(gpuID, pciFields)
	if err != nil {
		_ = FieldGroupDestroy(fieldsID)
		_ = DestroyGroup(groupID)
		return 0, fmt.Errorf("error getting Pcie bandwidth: %s", err)
	}

	gen := values[maxLinkGen].Int64()
	width := values[maxLinkWidth].Int64()

	_ = FieldGroupDestroy(fieldsID)
	_ = DestroyGroup(groupID)

	genMap := map[int64]int64{
		1: 250, // MB/s
		2: 500,
		3: 985,
		4: 1969,
	}

	bandwidth := genMap[gen] * width
	return bandwidth, nil
}

func getCPUAffinity(gpuID uint) (string, error) {
	const (
		affinity0 int = iota
		affinity1
		affinity2
		affinity3
		fieldsCount
	)

	affFields := make([]Short, fieldsCount)
	affFields[affinity0] = C.DCGM_FI_DEV_CPU_AFFINITY_0
	affFields[affinity1] = C.DCGM_FI_DEV_CPU_AFFINITY_1
	affFields[affinity2] = C.DCGM_FI_DEV_CPU_AFFINITY_2
	affFields[affinity3] = C.DCGM_FI_DEV_CPU_AFFINITY_3

	fieldsName := fmt.Sprintf("cpuAffFields%d", rand.Uint64())

	fieldsId, err := FieldGroupCreate(fieldsName, affFields)
	if err != nil {
		return "N/A", err
	}
	defer func() {
		ret := FieldGroupDestroy(fieldsId)

		if ret != nil {
			log.Printf("error destroying field group: %v", ret)
		}
	}()

	groupName := fmt.Sprintf("cpuAff%d", rand.Uint64())
	groupID, err := WatchFields(gpuID, fieldsId, groupName)
	if err != nil {
		return "N/A", err
	}
	defer func() {
		ret := DestroyGroup(groupID)

		if ret != nil {
			log.Printf("error destroying group: %v", ret)
		}
	}()

	values, err := GetLatestValuesForFields(gpuID, affFields)
	if err != nil {
		return "N/A", fmt.Errorf("error getting cpu affinity: %s", err)
	}

	bits := make([]uint64, 4)
	bits[0] = uint64(values[affinity0].Int64())
	bits[1] = uint64(values[affinity1].Int64())
	bits[2] = uint64(values[affinity2].Int64())
	bits[3] = uint64(values[affinity3].Int64())

	b := bitset.From(bits)

	return b.String(), nil
}

func getDeviceInfo(gpuID uint) (deviceInfo Device, err error) {
	var device C.dcgmDeviceAttributes_t
	device.version = makeVersion3(unsafe.Sizeof(device))

	result := C.dcgmGetDeviceAttributes(handle.handle, C.uint(gpuID), &device)
	if err = errorString(result); err != nil {
		return deviceInfo, &Error{msg: C.GoString(C.errorString(result)), Code: result}
	}

	// check if the given GPU is DCGM supported
	gpus, err := getSupportedDevices()
	if err != nil {
		return
	}

	supported := "No"

	for _, gpu := range gpus {
		if gpuID == gpu {
			supported = "Yes"
			break
		}
	}
	status := getGPUStatus(gpuID)
	if status != EntityStatusOk {
		supported = "No"
	}

	busid := *stringPtr(&device.identifiers.pciBusId[0])

	var (
		topology  []P2PLink
		bandwidth int64
		cpuAffinity string
	)

	// get device topology and bandwidth only if its a DCGM supported device
	if supported == "Yes" {
		cpuAffinity, err = getCPUAffinity(gpuID)
		if err != nil {
			return
		}

		topology, err = getDeviceTopology(gpuID)
		if err != nil {
			return
		}
		bandwidth, err = getPciBandwidth(gpuID)
		if err != nil {
			return
		}
	}

	uuid := *stringPtr(&device.identifiers.uuid[0])
	power := *uintPtr(device.powerLimits.defaultPowerLimit)

	pci := PCIInfo{
		BusID:     busid,
		BAR1:      *uintPtr(device.memoryUsage.bar1Total),
		FBTotal:   *uintPtr(device.memoryUsage.fbTotal),
		Bandwidth: bandwidth,
	}

	identifiers := DeviceIdentifiers{
		Brand:               *stringPtr(&device.identifiers.brandName[0]),
		Model:               *stringPtr(&device.identifiers.deviceName[0]),
		Serial:              *stringPtr(&device.identifiers.serial[0]),
		Vbios:               *stringPtr(&device.identifiers.vbios[0]),
		InforomImageVersion: *stringPtr(&device.identifiers.inforomImageVersion[0]),
		DriverVersion:       *stringPtr(&device.identifiers.driverVersion[0]),
	}

	deviceInfo = Device{
		GPU:           gpuID,
		DCGMSupported: supported,
		UUID:          uuid,
		Power:         power,
		PCI:           pci,
		Identifiers:   identifiers,
		Topology:      topology,
		CPUAffinity:   cpuAffinity,
	}
	return
}

func getNvLinkP2PStatus() (NvLinkP2PStatus, error) {
	var linkStatus C.dcgmNvLinkP2PStatus_v1
	linkStatus.version = makeVersion1(unsafe.Sizeof(linkStatus))

	result := C.dcgmGetNvLinkP2PStatus(handle.handle, &linkStatus)
	if result == C.DCGM_ST_NOT_SUPPORTED {
		return NvLinkP2PStatus{}, nil
	}

	if result != C.DCGM_ST_OK {
		return NvLinkP2PStatus{}, &Error{msg: C.GoString(C.errorString(result)), Code: result}
	}

	links := make([][]Link_State, linkStatus.numGpus)
	for i := range links {
		links[i] = make([]Link_State, linkStatus.numGpus)
	}

	for i := uint(0); i < uint(linkStatus.numGpus); i++ {
		for j := 0; j < int(linkStatus.numGpus); j++ {
			links[i][j] = Link_State(linkStatus.gpus[i].linkStatus[j])
		}
	}

	return NvLinkP2PStatus{
		numGpus: uint(linkStatus.numGpus),
		Gpus:    links,
	}, nil
}
