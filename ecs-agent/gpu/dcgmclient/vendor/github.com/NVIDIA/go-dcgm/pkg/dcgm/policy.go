package dcgm

/*
#include "dcgm_agent.h"
#include "dcgm_structs.h"

// wrapper for go callback function
extern int violationNotify(void* p);
*/
import "C"

import (
	"context"
	"encoding/binary"
	"fmt"
	"log"
	"sync"
	"time"
	"unsafe"
)

// PolicyCondition represents a type of policy violation that can be monitored
type PolicyCondition string

// This alias is maintained for backward compatibility.
type policyCondition = PolicyCondition

// Policy condition types
const (
	// DbePolicy represents a Double-bit ECC error policy condition
	DbePolicy = PolicyCondition("Double-bit ECC error")

	// PCIePolicy represents a PCI error policy condition
	PCIePolicy = PolicyCondition("PCI error")

	// MaxRtPgPolicy represents a Maximum Retired Pages Limit policy condition
	MaxRtPgPolicy = PolicyCondition("Max Retired Pages Limit")

	// ThermalPolicy represents a Thermal Limit policy condition
	ThermalPolicy = PolicyCondition("Thermal Limit")

	// PowerPolicy represents a Power Limit policy condition
	PowerPolicy = PolicyCondition("Power Limit")

	// NvlinkPolicy represents an NVLink error policy condition
	NvlinkPolicy = PolicyCondition("Nvlink Error")

	// XidPolicy represents an XID error policy condition
	XidPolicy = PolicyCondition("XID Error")
)

// Default policy thresholds matching dcgmi defaults
const (
	// DefaultMaxRetiredPages is the default threshold for retired pages (matches dcgmi default)
	DefaultMaxRetiredPages = 10

	// DefaultMaxTemperature is the default threshold for temperature in Celsius (matches dcgmi default)
	DefaultMaxTemperature = 100

	// DefaultMaxPower is the default threshold for power in Watts (matches dcgmi default)
	DefaultMaxPower = 250
)

// PolicyAction specifies the action to take when a policy violation occurs
type PolicyAction uint32

const (
	// PolicyActionNone indicates no action should be taken on violation (default)
	PolicyActionNone PolicyAction = 0

	// PolicyActionGPUReset indicates the GPU should be reset on violation
	PolicyActionGPUReset PolicyAction = 1
)

// PolicyValidation specifies the validation to perform after a policy action
type PolicyValidation uint32

const (
	// PolicyValidationNone indicates no validation after action (default)
	PolicyValidationNone PolicyValidation = 0

	// PolicyValidationShort indicates a short system validation should be performed
	PolicyValidationShort PolicyValidation = 1

	// PolicyValidationMedium indicates a medium system validation should be performed
	PolicyValidationMedium PolicyValidation = 2

	// PolicyValidationLong indicates a long system validation should be performed
	PolicyValidationLong PolicyValidation = 3
)

// PolicyConfig configures a policy condition with optional custom thresholds and actions
type PolicyConfig struct {
	// Condition specifies the type of policy to monitor
	Condition PolicyCondition

	// Action specifies what action to take when this policy violation occurs (optional, defaults to PolicyActionNone)
	Action *PolicyAction

	// Validation specifies what validation to perform after the action (optional, defaults to PolicyValidationNone)
	Validation *PolicyValidation

	// MaxRetiredPages specifies the threshold for MaxRtPgPolicy (optional, defaults to DefaultMaxRetiredPages)
	MaxRetiredPages *uint32

	// MaxTemperature specifies the threshold for ThermalPolicy in Celsius (optional, defaults to DefaultMaxTemperature)
	MaxTemperature *uint32

	// MaxPower specifies the threshold for PowerPolicy in Watts (optional, defaults to DefaultMaxPower)
	MaxPower *uint32
}

// PolicyViolation represents a detected violation of a policy condition
type PolicyViolation struct {
	// Condition specifies the type of policy that was violated
	Condition PolicyCondition
	// Timestamp indicates when the violation occurred
	Timestamp time.Time
	// Data contains violation-specific details
	Data any
}

type policyIndex int

const (
	dbePolicyIndex policyIndex = iota
	pciePolicyIndex
	maxRtPgPolicyIndex
	thermalPolicyIndex
	powerPolicyIndex
	nvlinkPolicyIndex
	xidPolicyIndex
)

type policyConditionParam struct {
	typ   uint32
	value uint32
}

// DbePolicyCondition contains details about a Double-bit ECC error
type DbePolicyCondition struct {
	// Location specifies where the ECC error occurred
	Location string
	// NumErrors indicates the number of errors detected
	NumErrors uint
}

// PciPolicyCondition contains details about a PCI error
type PciPolicyCondition struct {
	// ReplayCounter indicates the number of PCI replays
	ReplayCounter uint
}

// RetiredPagesPolicyCondition contains details about retired memory pages
type RetiredPagesPolicyCondition struct {
	// SbePages indicates the number of pages retired due to single-bit errors
	SbePages uint
	// DbePages indicates the number of pages retired due to double-bit errors
	DbePages uint
}

// ThermalPolicyCondition contains details about a thermal violation
type ThermalPolicyCondition struct {
	// ThermalViolation indicates the severity of the thermal violation
	ThermalViolation uint
}

// PowerPolicyCondition contains details about a power violation
type PowerPolicyCondition struct {
	// PowerViolation indicates the severity of the power violation
	PowerViolation uint
}

// NvlinkPolicyCondition contains details about an NVLink error
type NvlinkPolicyCondition struct {
	// FieldId identifies the specific NVLink field that had an error
	FieldId uint16
	// Counter indicates the number of errors detected
	Counter uint
}

// XidPolicyCondition contains details about an XID error
type XidPolicyCondition struct {
	// ErrNum is the XID error number
	ErrNum uint
}

var (
	policyChanOnce sync.Once
	policyMapOnce  sync.Once

	// callbacks maps PolicyViolation channels with policy
	// captures C callback() value for each violation condition
	callbacks map[string]chan PolicyViolation

	// paramMap maps C.dcgmPolicy_t.parms index and limits
	// to be used in setPolicy() for setting user selected policies
	paramMap map[policyIndex]policyConditionParam
)

func makePolicyChannels() {
	policyChanOnce.Do(func() {
		callbacks = make(map[string]chan PolicyViolation)
		callbacks["dbe"] = make(chan PolicyViolation, 1)
		callbacks["pcie"] = make(chan PolicyViolation, 1)
		callbacks["maxrtpg"] = make(chan PolicyViolation, 1)
		callbacks["thermal"] = make(chan PolicyViolation, 1)
		callbacks["power"] = make(chan PolicyViolation, 1)
		callbacks["nvlink"] = make(chan PolicyViolation, 1)
		callbacks["xid"] = make(chan PolicyViolation, 1)
	})
}

func makePolicyParmsMap() {
	const (
		policyFieldTypeBool    = 0
		policyFieldTypeLong    = 1
		policyBoolValue        = 1
		policyMaxRtPgThreshold = 10
		policyThermalThreshold = 100
		policyPowerThreshold   = 250
	)

	policyMapOnce.Do(func() {
		paramMap = make(map[policyIndex]policyConditionParam)
		paramMap[dbePolicyIndex] = policyConditionParam{
			typ:   policyFieldTypeBool,
			value: policyBoolValue,
		}

		paramMap[pciePolicyIndex] = policyConditionParam{
			typ:   policyFieldTypeBool,
			value: policyBoolValue,
		}

		paramMap[maxRtPgPolicyIndex] = policyConditionParam{
			typ:   policyFieldTypeLong,
			value: policyMaxRtPgThreshold,
		}

		paramMap[thermalPolicyIndex] = policyConditionParam{
			typ:   policyFieldTypeLong,
			value: policyThermalThreshold,
		}

		paramMap[powerPolicyIndex] = policyConditionParam{
			typ:   policyFieldTypeLong,
			value: policyPowerThreshold,
		}

		paramMap[nvlinkPolicyIndex] = policyConditionParam{
			typ:   policyFieldTypeBool,
			value: policyBoolValue,
		}

		paramMap[xidPolicyIndex] = policyConditionParam{
			typ:   policyFieldTypeBool,
			value: policyBoolValue,
		}
	})
}

// ViolationRegistration is a go callback function for dcgmPolicyRegister() wrapped in C.violationNotify()
//
//export ViolationRegistration
func ViolationRegistration(data unsafe.Pointer) int {
	var con PolicyCondition
	var timestamp time.Time
	var val any

	response := *(*C.dcgmPolicyCallbackResponse_t)(data)

	switch response.condition {
	case C.DCGM_POLICY_COND_DBE:
		dbe := (*C.dcgmPolicyConditionDbe_t)(unsafe.Pointer(&response.val))
		con = DbePolicy
		timestamp = createTimeStamp(dbe.timestamp)
		val = DbePolicyCondition{
			Location:  dbeLocation(int(dbe.location)),
			NumErrors: *uintPtr(dbe.numerrors),
		}
	case C.DCGM_POLICY_COND_PCI:
		pci := (*C.dcgmPolicyConditionPci_t)(unsafe.Pointer(&response.val))
		con = PCIePolicy
		timestamp = createTimeStamp(pci.timestamp)
		val = PciPolicyCondition{
			ReplayCounter: *uintPtr(pci.counter),
		}
	case C.DCGM_POLICY_COND_MAX_PAGES_RETIRED:
		mpr := (*C.dcgmPolicyConditionMpr_t)(unsafe.Pointer(&response.val))
		con = MaxRtPgPolicy
		timestamp = createTimeStamp(mpr.timestamp)
		val = RetiredPagesPolicyCondition{
			SbePages: *uintPtr(mpr.sbepages),
			DbePages: *uintPtr(mpr.dbepages),
		}
	case C.DCGM_POLICY_COND_THERMAL:
		thermal := (*C.dcgmPolicyConditionThermal_t)(unsafe.Pointer(&response.val))
		con = ThermalPolicy
		timestamp = createTimeStamp(thermal.timestamp)
		val = ThermalPolicyCondition{
			ThermalViolation: *uintPtr(thermal.thermalViolation),
		}
	case C.DCGM_POLICY_COND_POWER:
		pwr := (*C.dcgmPolicyConditionPower_t)(unsafe.Pointer(&response.val))
		con = PowerPolicy
		timestamp = createTimeStamp(pwr.timestamp)
		val = PowerPolicyCondition{
			PowerViolation: *uintPtr(pwr.powerViolation),
		}
	case C.DCGM_POLICY_COND_NVLINK:
		nvlink := (*C.dcgmPolicyConditionNvlink_t)(unsafe.Pointer(&response.val))
		con = NvlinkPolicy
		timestamp = createTimeStamp(nvlink.timestamp)
		val = NvlinkPolicyCondition{
			FieldId: uint16(nvlink.fieldId),
			Counter: *uintPtr(nvlink.counter),
		}
	case C.DCGM_POLICY_COND_XID:
		xid := (*C.dcgmPolicyConditionXID_t)(unsafe.Pointer(&response.val))
		con = XidPolicy
		timestamp = createTimeStamp(xid.timestamp)
		val = XidPolicyCondition{
			ErrNum: *uintPtr(xid.errnum),
		}
	}

	err := PolicyViolation{
		Condition: con,
		Timestamp: timestamp,
		Data:      val,
	}

	switch con {
	case DbePolicy:
		callbacks["dbe"] <- err
	case PCIePolicy:
		callbacks["pcie"] <- err
	case MaxRtPgPolicy:
		callbacks["maxrtpg"] <- err
	case ThermalPolicy:
		callbacks["thermal"] <- err
	case PowerPolicy:
		callbacks["power"] <- err
	case NvlinkPolicy:
		callbacks["nvlink"] <- err
	case XidPolicy:
		callbacks["xid"] <- err
	}
	return 0
}

func setPolicy(groupID GroupHandle, condition C.dcgmPolicyCondition_t, paramList []policyIndex) (err error) {
	var policy C.dcgmPolicy_t
	policy.version = makeVersion1(unsafe.Sizeof(policy))
	policy.mode = C.dcgmPolicyMode_t(C.DCGM_OPERATION_MODE_AUTO)
	policy.action = C.DCGM_POLICY_ACTION_NONE
	policy.isolation = C.DCGM_POLICY_ISOLATION_NONE
	policy.validation = C.DCGM_POLICY_VALID_NONE
	policy.condition = condition

	// iterate on paramMap for given policy conditions
	for _, key := range paramList {
		conditionParam, exists := paramMap[key]
		if !exists {
			return fmt.Errorf("invalid policy condition: %v does not exist", key)
		}
		// set policy condition parameters
		// set condition type (bool or longlong)
		policy.parms[key].tag = conditionParam.typ

		// set condition val (violation threshold)
		// policy.parms.val is a C union type
		// cgo docs: Go doesn't have support for C's union type
		// C union types are represented as a Go byte array
		binary.LittleEndian.PutUint32(policy.parms[key].val[:], conditionParam.value)
	}

	var statusHandle C.dcgmStatus_t

	result := C.dcgmPolicySet(handle.handle, groupID.handle, &policy, statusHandle)
	if err = errorString(result); err != nil {
		return fmt.Errorf("error setting policies: %s", err)
	}

	log.Println("Policy successfully set.")

	return
}

func setPolicyInternal(groupID GroupHandle, condition C.dcgmPolicyCondition_t, configs []policyConfigInternal, action PolicyAction, validation PolicyValidation) (err error) {
	var policy C.dcgmPolicy_t
	policy.version = makeVersion1(unsafe.Sizeof(policy))
	policy.mode = C.dcgmPolicyMode_t(C.DCGM_OPERATION_MODE_AUTO)
	policy.action = C.dcgmPolicyAction_t(action)
	policy.isolation = C.DCGM_POLICY_ISOLATION_NONE
	policy.validation = C.dcgmPolicyValidation_t(validation)
	policy.condition = condition

	// iterate on configs for given policy conditions
	for _, cfg := range configs {
		// set policy condition parameters
		// set condition type (bool or longlong)
		policy.parms[cfg.index].tag = cfg.param.typ

		// set condition val (violation threshold)
		// policy.parms.val is a C union type
		// cgo docs: Go doesn't have support for C's union type
		// C union types are represented as a Go byte array
		binary.LittleEndian.PutUint32(policy.parms[cfg.index].val[:], cfg.param.value)
	}

	var statusHandle C.dcgmStatus_t

	result := C.dcgmPolicySet(handle.handle, groupID.handle, &policy, statusHandle)
	if err = errorString(result); err != nil {
		return fmt.Errorf("error setting policies: %s", err)
	}

	return
}

type policyConfigInternal struct {
	index policyIndex
	param policyConditionParam
}

// PolicyStatus represents the current policy configuration for a group
type PolicyStatus struct {
	// Mode indicates the operation mode (automatic or manual)
	Mode uint32

	// Action specifies what action is taken on violation
	Action PolicyAction

	// Validation specifies what validation is performed after action
	Validation PolicyValidation

	// Conditions is a map of enabled policy conditions with their thresholds
	// Key is the PolicyCondition, value is the threshold (if applicable)
	Conditions map[PolicyCondition]interface{}
}

func getPolicyForGroup(groupID GroupHandle) (*PolicyStatus, error) {
	var policy C.dcgmPolicy_t
	policy.version = makeVersion1(unsafe.Sizeof(policy))

	var statusHandle C.dcgmStatus_t

	result := C.dcgmPolicyGet(handle.handle, groupID.handle, 1, &policy, statusHandle)
	if err := errorString(result); err != nil {
		return nil, fmt.Errorf("error getting policy: %s", err)
	}

	status := &PolicyStatus{
		Mode:       uint32(policy.mode),
		Action:     PolicyAction(policy.action),
		Validation: PolicyValidation(policy.validation),
		Conditions: make(map[PolicyCondition]interface{}),
	}

	condition := policy.condition

	// Check each condition bit and extract its parameters
	if condition&C.DCGM_POLICY_COND_DBE != 0 {
		status.Conditions[DbePolicy] = true
	}

	if condition&C.DCGM_POLICY_COND_PCI != 0 {
		status.Conditions[PCIePolicy] = true
	}

	if condition&C.DCGM_POLICY_COND_MAX_PAGES_RETIRED != 0 {
		param := policy.parms[maxRtPgPolicyIndex]
		if param.tag == 1 { // LLONG type
			threshold := binary.LittleEndian.Uint32(param.val[:])
			status.Conditions[MaxRtPgPolicy] = threshold
		}
	}

	if condition&C.DCGM_POLICY_COND_THERMAL != 0 {
		param := policy.parms[thermalPolicyIndex]
		if param.tag == 1 { // LLONG type
			threshold := binary.LittleEndian.Uint32(param.val[:])
			status.Conditions[ThermalPolicy] = threshold
		}
	}

	if condition&C.DCGM_POLICY_COND_POWER != 0 {
		param := policy.parms[powerPolicyIndex]
		if param.tag == 1 { // LLONG type
			threshold := binary.LittleEndian.Uint32(param.val[:])
			status.Conditions[PowerPolicy] = threshold
		}
	}

	if condition&C.DCGM_POLICY_COND_NVLINK != 0 {
		status.Conditions[NvlinkPolicy] = true
	}

	if condition&C.DCGM_POLICY_COND_XID != 0 {
		status.Conditions[XidPolicy] = true
	}

	return status, nil
}

func clearPolicyForGroup(groupID GroupHandle) error {
	// Clear all policies by setting condition to 0 (no conditions enabled)
	var policy C.dcgmPolicy_t
	policy.version = makeVersion1(unsafe.Sizeof(policy))
	policy.mode = C.dcgmPolicyMode_t(C.DCGM_OPERATION_MODE_AUTO)
	policy.action = C.DCGM_POLICY_ACTION_NONE
	policy.isolation = C.DCGM_POLICY_ISOLATION_NONE
	policy.validation = C.DCGM_POLICY_VALID_NONE
	policy.condition = 0 // No conditions - clears all policies

	var statusHandle C.dcgmStatus_t

	result := C.dcgmPolicySet(handle.handle, groupID.handle, &policy, statusHandle)
	if err := errorString(result); err != nil {
		return fmt.Errorf("error clearing policies: %s", err)
	}

	return nil
}

func setPolicyForGroupWithConfig(groupID GroupHandle, configs ...PolicyConfig) error {
	const (
		policyFieldTypeBool = 0
		policyFieldTypeLong = 1
		policyBoolValue     = 1
	)

	if len(configs) == 0 {
		return fmt.Errorf("at least one policy config must be provided")
	}

	// Extract action and validation from first config (applies to all conditions)
	// This matches dcgmi behavior where --set actn,val applies to the entire policy set
	action := PolicyActionNone
	validation := PolicyValidationNone

	if configs[0].Action != nil {
		action = *configs[0].Action
	}
	if configs[0].Validation != nil {
		validation = *configs[0].Validation
	}

	// Build internal configs with custom or default thresholds
	internalConfigs := make([]policyConfigInternal, len(configs))
	var condition C.dcgmPolicyCondition_t = 0

	for i, cfg := range configs {
		var idx policyIndex
		var param policyConditionParam

		switch cfg.Condition {
		case DbePolicy:
			idx = dbePolicyIndex
			condition |= C.DCGM_POLICY_COND_DBE
			param = policyConditionParam{
				typ:   policyFieldTypeBool,
				value: policyBoolValue,
			}

		case PCIePolicy:
			idx = pciePolicyIndex
			condition |= C.DCGM_POLICY_COND_PCI
			param = policyConditionParam{
				typ:   policyFieldTypeBool,
				value: policyBoolValue,
			}

		case MaxRtPgPolicy:
			idx = maxRtPgPolicyIndex
			condition |= C.DCGM_POLICY_COND_MAX_PAGES_RETIRED
			threshold := uint32(DefaultMaxRetiredPages)
			if cfg.MaxRetiredPages != nil {
				threshold = *cfg.MaxRetiredPages
			}
			param = policyConditionParam{
				typ:   policyFieldTypeLong,
				value: threshold,
			}

		case ThermalPolicy:
			idx = thermalPolicyIndex
			condition |= C.DCGM_POLICY_COND_THERMAL
			threshold := uint32(DefaultMaxTemperature)
			if cfg.MaxTemperature != nil {
				threshold = *cfg.MaxTemperature
			}
			param = policyConditionParam{
				typ:   policyFieldTypeLong,
				value: threshold,
			}

		case PowerPolicy:
			idx = powerPolicyIndex
			condition |= C.DCGM_POLICY_COND_POWER
			threshold := uint32(DefaultMaxPower)
			if cfg.MaxPower != nil {
				threshold = *cfg.MaxPower
			}
			param = policyConditionParam{
				typ:   policyFieldTypeLong,
				value: threshold,
			}

		case NvlinkPolicy:
			idx = nvlinkPolicyIndex
			condition |= C.DCGM_POLICY_COND_NVLINK
			param = policyConditionParam{
				typ:   policyFieldTypeBool,
				value: policyBoolValue,
			}

		case XidPolicy:
			idx = xidPolicyIndex
			condition |= C.DCGM_POLICY_COND_XID
			param = policyConditionParam{
				typ:   policyFieldTypeBool,
				value: policyBoolValue,
			}

		default:
			return fmt.Errorf("unknown policy condition: %s", cfg.Condition)
		}

		internalConfigs[i] = policyConfigInternal{
			index: idx,
			param: param,
		}
	}

	return setPolicyInternal(groupID, condition, internalConfigs, action, validation)
}

func registerPolicy(ctx context.Context, groupID GroupHandle, typ ...PolicyCondition) (<-chan PolicyViolation, error) {
	var err error
	// init policy globals for internal API
	makePolicyChannels()
	makePolicyParmsMap()

	// make a list of policy conditions for setting their parameters
	paramKeys := make([]policyIndex, len(typ))
	// get all conditions to be set in setPolicy()
	var condition C.dcgmPolicyCondition_t = 0

	for i, t := range typ {
		switch t {
		case DbePolicy:
			paramKeys[i] = dbePolicyIndex
			condition |= C.DCGM_POLICY_COND_DBE
		case PCIePolicy:
			paramKeys[i] = pciePolicyIndex
			condition |= C.DCGM_POLICY_COND_PCI
		case MaxRtPgPolicy:
			paramKeys[i] = maxRtPgPolicyIndex
			condition |= C.DCGM_POLICY_COND_MAX_PAGES_RETIRED
		case ThermalPolicy:
			paramKeys[i] = thermalPolicyIndex
			condition |= C.DCGM_POLICY_COND_THERMAL
		case PowerPolicy:
			paramKeys[i] = powerPolicyIndex
			condition |= C.DCGM_POLICY_COND_POWER
		case NvlinkPolicy:
			paramKeys[i] = nvlinkPolicyIndex
			condition |= C.DCGM_POLICY_COND_NVLINK
		case XidPolicy:
			paramKeys[i] = xidPolicyIndex
			condition |= C.DCGM_POLICY_COND_XID
		}
	}

	err = setPolicy(groupID, condition, paramKeys)
	if err != nil {
		return nil, err
	}

	result := C.dcgmPolicyRegister_v2(handle.handle, groupID.handle, condition, C.fpRecvUpdates(C.violationNotify), C.ulong(0))

	if err = errorString(result); err != nil {
		return nil, &Error{msg: C.GoString(C.errorString(result)), Code: result}
	}

	log.Println("Listening for violations...")

	violation := make(chan PolicyViolation, len(typ))

	go func() {
		defer func() {
			log.Println("unregister policy violation...")
			close(violation)
			unregisterPolicy(groupID, condition)
		}()

		for {
			select {
			case dbe := <-callbacks["dbe"]:
				violation <- dbe
			case pcie := <-callbacks["pcie"]:
				violation <- pcie
			case maxrtpg := <-callbacks["maxrtpg"]:
				violation <- maxrtpg
			case thermal := <-callbacks["thermal"]:
				violation <- thermal
			case power := <-callbacks["power"]:
				violation <- power
			case nvlink := <-callbacks["nvlink"]:
				violation <- nvlink
			case xid := <-callbacks["xid"]:
				violation <- xid
			case <-ctx.Done():
				return
			}
		}
	}()

	return violation, err
}

func registerPolicyOnly(ctx context.Context, groupID GroupHandle, typ ...PolicyCondition) (<-chan PolicyViolation, error) {
	var err error
	// init policy globals for internal API
	makePolicyChannels()

	// get all conditions to listen for
	var condition C.dcgmPolicyCondition_t = 0

	for _, t := range typ {
		switch t {
		case DbePolicy:
			condition |= C.DCGM_POLICY_COND_DBE
		case PCIePolicy:
			condition |= C.DCGM_POLICY_COND_PCI
		case MaxRtPgPolicy:
			condition |= C.DCGM_POLICY_COND_MAX_PAGES_RETIRED
		case ThermalPolicy:
			condition |= C.DCGM_POLICY_COND_THERMAL
		case PowerPolicy:
			condition |= C.DCGM_POLICY_COND_POWER
		case NvlinkPolicy:
			condition |= C.DCGM_POLICY_COND_NVLINK
		case XidPolicy:
			condition |= C.DCGM_POLICY_COND_XID
		}
	}

	// Register for violations without setting policies
	result := C.dcgmPolicyRegister_v2(handle.handle, groupID.handle, condition, C.fpRecvUpdates(C.violationNotify), C.ulong(0))

	if err = errorString(result); err != nil {
		return nil, &Error{msg: C.GoString(C.errorString(result)), Code: result}
	}

	violation := make(chan PolicyViolation, len(typ))

	go func() {
		defer func() {
			close(violation)
			unregisterPolicy(groupID, condition)
		}()

		for {
			select {
			case dbe := <-callbacks["dbe"]:
				violation <- dbe
			case pcie := <-callbacks["pcie"]:
				violation <- pcie
			case maxrtpg := <-callbacks["maxrtpg"]:
				violation <- maxrtpg
			case thermal := <-callbacks["thermal"]:
				violation <- thermal
			case power := <-callbacks["power"]:
				violation <- power
			case nvlink := <-callbacks["nvlink"]:
				violation <- nvlink
			case xid := <-callbacks["xid"]:
				violation <- xid
			case <-ctx.Done():
				return
			}
		}
	}()

	return violation, err
}

func unregisterPolicy(groupID GroupHandle, condition C.dcgmPolicyCondition_t) {
	result := C.dcgmPolicyUnregister(handle.handle, groupID.handle, condition)

	if err := errorString(result); err != nil {
		log.Println(fmt.Errorf("error unregistering policy: %s", err))
	}
}

func createTimeStamp(t C.longlong) time.Time {
	tm := int64(t) / 1000000
	ts := time.Unix(tm, 0)
	return ts
}

func dbeLocation(location int) string {
	switch location {
	case 0:
		return "L1"
	case 1:
		return "L2"
	case 2:
		return "Device"
	case 3:
		return "Register"
	case 4:
		return "Texture"
	}
	return "N/A"
}
