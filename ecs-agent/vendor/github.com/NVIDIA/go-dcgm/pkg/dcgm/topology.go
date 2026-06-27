package dcgm

/*
#include "dcgm_agent.h"
#include "dcgm_structs.h"
*/
import "C"

import (
	"fmt"
	"unsafe"
)

// P2PLinkType represents the type of peer-to-peer connection between GPUs
type P2PLinkType uint

const (
	// P2PLinkUnknown represents an unknown link type
	P2PLinkUnknown P2PLinkType = iota
	// P2PLinkCrossCPU represents a connection across different CPUs
	P2PLinkCrossCPU
	// P2PLinkSameCPU represents a connection within the same CPU
	P2PLinkSameCPU
	// P2PLinkHostBridge represents a connection through the host bridge
	P2PLinkHostBridge
	// P2PLinkMultiSwitch represents a connection through multiple PCIe switches
	P2PLinkMultiSwitch
	// P2PLinkSingleSwitch represents a connection through a single PCIe switch
	P2PLinkSingleSwitch
	// P2PLinkSameBoard represents a connection on the same board
	P2PLinkSameBoard
	// SingleNVLINKLink represents a single NVLINK connection
	SingleNVLINKLink
	// TwoNVLINKLinks represents two NVLINK connections
	TwoNVLINKLinks
	// ThreeNVLINKLinks represents three NVLINK connections
	ThreeNVLINKLinks
	// FourNVLINKLinks represents four NVLINK connections
	FourNVLINKLinks
)

// PCIPaths returns a string representation of the P2P link type
func (l P2PLinkType) PCIPaths() string {
	switch l {
	case P2PLinkSameBoard:
		return "PSB"
	case P2PLinkSingleSwitch:
		return "PIX"
	case P2PLinkMultiSwitch:
		return "PXB"
	case P2PLinkHostBridge:
		return "PHB"
	case P2PLinkSameCPU:
		return "NODE"
	case P2PLinkCrossCPU:
		return "SYS"
	case SingleNVLINKLink:
		return "NV1"
	case TwoNVLINKLinks:
		return "NV2"
	case ThreeNVLINKLinks:
		return "NV3"
	case FourNVLINKLinks:
		return "NV4"
	case P2PLinkUnknown:
	}
	return "N/A"
}

// P2PLink contains information about a peer-to-peer connection
type P2PLink struct {
	// GPU is the ID of the GPU
	GPU uint
	// BusID is the PCIe bus ID of the GPU
	BusID string
	// Link is the type of P2P connection
	Link P2PLinkType
}

func getP2PLink(path uint) P2PLinkType {
	switch path {
	case C.DCGM_TOPOLOGY_BOARD:
		return P2PLinkSameBoard
	case C.DCGM_TOPOLOGY_SINGLE:
		return P2PLinkSingleSwitch
	case C.DCGM_TOPOLOGY_MULTIPLE:
		return P2PLinkMultiSwitch
	case C.DCGM_TOPOLOGY_HOSTBRIDGE:
		return P2PLinkHostBridge
	case C.DCGM_TOPOLOGY_CPU:
		return P2PLinkSameCPU
	case C.DCGM_TOPOLOGY_SYSTEM:
		return P2PLinkCrossCPU
	case C.DCGM_TOPOLOGY_NVLINK1:
		return SingleNVLINKLink
	case C.DCGM_TOPOLOGY_NVLINK2:
		return TwoNVLINKLinks
	case C.DCGM_TOPOLOGY_NVLINK3:
		return ThreeNVLINKLinks
	case C.DCGM_TOPOLOGY_NVLINK4:
		return FourNVLINKLinks
	}
	return P2PLinkUnknown
}

func getBusID(gpuID uint) (string, error) {
	var device C.dcgmDeviceAttributes_v3
	device.version = makeVersion3(unsafe.Sizeof(device))

	result := C.dcgmGetDeviceAttributes(handle.handle, C.uint(gpuID), &device)
	if err := errorString(result); err != nil {
		return "", fmt.Errorf("error getting device busid: %s", err)
	}
	return *stringPtr(&device.identifiers.pciBusId[0]), nil
}

func getDeviceTopology(gpuID uint) (links []P2PLink, err error) {
	var topology C.dcgmDeviceTopology_v1
	topology.version = makeVersion1(unsafe.Sizeof(topology))

	result := C.dcgmGetDeviceTopology(handle.handle, C.uint(gpuID), &topology)
	if result == C.DCGM_ST_NOT_SUPPORTED {
		return links, nil
	}
	if result != C.DCGM_ST_OK {
		return links, &Error{msg: C.GoString(C.errorString(result)), Code: result}
	}

	busid, err := getBusID(gpuID)
	if err != nil {
		return
	}
	links = make([]P2PLink, topology.numGpus)
	for i := uint(0); i < uint(topology.numGpus); i++ {
		links[i].GPU = uint(topology.gpuPaths[i].gpuId)
		links[i].BusID = busid
		links[i].Link = getP2PLink(uint(topology.gpuPaths[i].path))
	}
	return
}

// Link_State represents the state of an NVLINK connection
type Link_State uint

const (
	// LS_NOT_SUPPORTED indicates the link is unsupported (Default for GPUs)
	LS_NOT_SUPPORTED Link_State = iota
	// LS_DISABLED indicates the link is supported but disabled (Default for NvSwitches)
	LS_DISABLED
	// LS_DOWN indicates the link is down (inactive)
	LS_DOWN
	// LS_UP indicates the link is up (active)
	LS_UP
)

// NvLinkStatus contains information about an NVLINK connection status
type NvLinkStatus struct {
	// ParentId is the ID of the parent entity (GPU or NVSwitch)
	ParentId uint
	// ParentType is the type of the parent entity
	ParentType Field_Entity_Group
	// State is the current state of the NVLINK
	State Link_State
	// Index is the link index number
	Index uint
}

func getNvLinkLinkStatus() ([]NvLinkStatus, error) {
	var linkStatus C.dcgmNvLinkStatus_v4
	linkStatus.version = makeVersion4(unsafe.Sizeof(linkStatus))

	result := C.dcgmGetNvLinkLinkStatus(handle.handle, &linkStatus)
	if result == C.DCGM_ST_NOT_SUPPORTED {
		return nil, nil
	}

	if result != C.DCGM_ST_OK {
		return nil, &Error{msg: C.GoString(C.errorString(result)), Code: result}
	}

	links := make([]NvLinkStatus, linkStatus.numGpus*C.DCGM_NVLINK_MAX_LINKS_PER_GPU+linkStatus.numNvSwitches*C.DCGM_NVLINK_MAX_LINKS_PER_NVSWITCH)

	idx := 0
	for i := uint(0); i < uint(linkStatus.numGpus); i++ {
		for j := 0; j < int(C.DCGM_NVLINK_MAX_LINKS_PER_GPU); j++ {
			link := NvLinkStatus{
				uint(linkStatus.gpus[i].entityId),
				FE_GPU,
				Link_State(linkStatus.gpus[i].linkState[j]),
				uint(j),
			}

			links[idx] = link
			idx++
		}
	}

	for i := uint(0); i < uint(linkStatus.numNvSwitches); i++ {
		for j := 0; j < C.DCGM_NVLINK_MAX_LINKS_PER_NVSWITCH; j++ {
			link := NvLinkStatus{
				uint(linkStatus.nvSwitches[i].entityId),
				FE_SWITCH,
				Link_State(linkStatus.nvSwitches[i].linkState[j]),
				uint(j),
			}

			links[idx] = link
			idx++
		}
	}

	return links, nil
}
