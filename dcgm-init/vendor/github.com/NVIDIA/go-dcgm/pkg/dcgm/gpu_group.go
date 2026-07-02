package dcgm

/*
#include "dcgm_agent.h"
#include "dcgm_structs.h"
*/
import "C"

import (
	"context"
	"encoding/binary"
	"fmt"
)

// DCGM_GROUP_MAX_ENTITIES represents the maximum number of entities allowed in a group
const (
	DCGM_GROUP_MAX_ENTITIES int = C.DCGM_GROUP_MAX_ENTITIES_V2
)

// GroupHandle represents a handle to a DCGM GPU group
type GroupHandle struct{ handle C.dcgmGpuGrp_t }

// SetHandle sets the internal group handle value
func (g *GroupHandle) SetHandle(val uintptr) {
	g.handle = C.dcgmGpuGrp_t(val)
}

// GetHandle returns the internal group handle value
func (g *GroupHandle) GetHandle() uintptr {
	return uintptr(g.handle)
}

// GroupAllGPUs returns a GroupHandle representing all GPUs in the system
func GroupAllGPUs() GroupHandle {
	return GroupHandle{C.DCGM_GROUP_ALL_GPUS}
}

// CreateGroup creates a new empty GPU group with the specified name.
//
// Important: Groups must be destroyed using DestroyGroup when no longer needed
// to prevent resource leaks in the DCGM library.
//
// Example:
//
//	group, err := dcgm.CreateGroup("myGroup")
//	if err != nil {
//	    return err
//	}
//	defer dcgm.DestroyGroup(group)
//
//	// Use the group...
func CreateGroup(groupName string) (goGroupId GroupHandle, err error) {
	var cGroupID C.dcgmGpuGrp_t
	cname := C.CString(groupName)
	defer freeCString(cname)

	result := C.dcgmGroupCreate(handle.handle, C.DCGM_GROUP_EMPTY, cname, &cGroupID)
	if err = errorString(result); err != nil {
		return goGroupId, fmt.Errorf("error creating group: %s", err)
	}

	goGroupId = GroupHandle{cGroupID}
	return
}

// NewDefaultGroup creates a new group with default GPUs and the specified name
func NewDefaultGroup(groupName string) (GroupHandle, error) {
	var cGroupID C.dcgmGpuGrp_t

	cname := C.CString(groupName)
	defer freeCString(cname)

	result := C.dcgmGroupCreate(handle.handle, C.DCGM_GROUP_DEFAULT, cname, &cGroupID)
	if err := errorString(result); err != nil {
		return GroupHandle{}, fmt.Errorf("error creating group: %s", err)
	}

	return GroupHandle{cGroupID}, nil
}

// AddToGroup adds a GPU to an existing group
func AddToGroup(groupID GroupHandle, gpuID uint) (err error) {
	result := C.dcgmGroupAddDevice(handle.handle, groupID.handle, C.uint(gpuID))
	if err = errorString(result); err != nil {
		return fmt.Errorf("error adding GPU %v to group: %s", gpuID, err)
	}

	return
}

// AddLinkEntityToGroup adds a link entity to the group
func AddLinkEntityToGroup(groupID GroupHandle, index uint, entityGroupID Field_Entity_Group, parentID uint) (err error) {
	/* Only supported on little-endian systems currently */
	slice := make([]byte, 4)
	slice[0] = uint8(entityGroupID)
	binary.LittleEndian.PutUint16(slice[1:3], uint16(index))
	slice[3] = uint8(parentID)

	entityId := binary.LittleEndian.Uint32(slice)

	return AddEntityToGroup(groupID, FE_LINK, uint(entityId))
}

// AddEntityToGroup adds an entity to an existing group
func AddEntityToGroup(groupID GroupHandle, entityGroupID Field_Entity_Group, entityID uint) (err error) {
	result := C.dcgmGroupAddEntity(handle.handle, groupID.handle, C.dcgm_field_entity_group_t(entityGroupID),
		C.uint(entityID))
	if err = errorString(result); err != nil {
		return fmt.Errorf("error adding entity group type %v, entity %v to group: %s", entityGroupID, entityID, err)
	}

	return
}

// DestroyGroup destroys an existing GPU group
func DestroyGroup(groupID GroupHandle) (err error) {
	result := C.dcgmGroupDestroy(handle.handle, groupID.handle)
	if err = errorString(result); err != nil {
		return fmt.Errorf("error destroying group: %s", err)
	}

	return
}

// GroupInfo contains information about a DCGM group
type GroupInfo struct {
	Version    uint32
	GroupName  string
	EntityList []GroupEntityPair
}

// GetGroupInfo retrieves information about a DCGM group
func GetGroupInfo(groupID GroupHandle) (*GroupInfo, error) {
	response := C.dcgmGroupInfo_v3{
		version: C.dcgmGroupInfo_version3,
	}

	result := C.dcgmGroupGetInfo(handle.handle, groupID.handle, &response)
	if err := errorString(result); err != nil {
		return nil, err
	}

	ret := GroupInfo{
		Version:    uint32(response.version),
		GroupName:  C.GoString(&response.groupName[0]),
		EntityList: make([]GroupEntityPair, response.count),
	}

	for i := 0; i < int(response.count); i++ {
		ret.EntityList[i].EntityId = uint(response.entityList[i].entityId)
		ret.EntityList[i].EntityGroupId = Field_Entity_Group(response.entityList[i].entityGroupId)
	}

	return &ret, nil
}

// CreateGroupWithContext creates a new group with a context
func CreateGroupWithContext(ctx context.Context, groupName string) (GroupHandle, error) {
	select {
	case <-ctx.Done():
		return GroupHandle{}, ctx.Err()
	default:
		return CreateGroup(groupName)
	}
}
