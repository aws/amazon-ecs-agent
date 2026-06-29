package dcgm

/*
#include <stdint.h>
#include "dcgm_agent.h"
#include "dcgm_structs.h"

// wrapper for go callback function
extern int violationNotify(dcgmPolicyCallbackResponse_t *response, uint64_t userData);
*/
import "C"

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
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
	policyCallbacks = newPolicyDispatcher()
)

type translatedPolicyConditions struct {
	condition C.dcgmPolicyCondition_t
}

type policySubscription struct {
	id         uint64
	group      GroupHandle
	groupKey   uintptr
	conditions C.dcgmPolicyCondition_t
	ch         chan PolicyViolation
}

type policyRegistration struct {
	id         uint64
	group      GroupHandle
	groupKey   uintptr
	conditions C.dcgmPolicyCondition_t
}

type policyUnregister struct {
	group     GroupHandle
	condition C.dcgmPolicyCondition_t
}

type policyDispatcher struct {
	registerMu sync.Mutex
	mu         sync.Mutex
	nextID     uint64

	subscriptions     map[uint64]*policySubscription
	registrations     map[uint64]policyRegistration
	registeredByGroup map[uintptr]C.dcgmPolicyCondition_t

	drops atomic.Uint64
}

// newPolicyDispatcher initializes the Go-side policy callback registry.
func newPolicyDispatcher() *policyDispatcher {
	return &policyDispatcher{
		subscriptions:     make(map[uint64]*policySubscription),
		registrations:     make(map[uint64]policyRegistration),
		registeredByGroup: make(map[uintptr]C.dcgmPolicyCondition_t),
	}
}

// nextLocked returns the next non-zero dispatcher ID; d.mu must be held.
func (d *policyDispatcher) nextLocked() uint64 {
	d.nextID++
	if d.nextID == 0 {
		d.nextID++
	}
	return d.nextID
}

// addSubscription records a listener and returns any missing DCGM registration.
func (d *policyDispatcher) addSubscription(
	group GroupHandle,
	conditions C.dcgmPolicyCondition_t,
	buffer int,
) (uint64, chan PolicyViolation, *policyRegistration) {
	if buffer < 1 {
		buffer = 1
	}

	groupKey := group.GetHandle()

	d.mu.Lock()
	defer d.mu.Unlock()

	subID := d.nextLocked()
	ch := make(chan PolicyViolation, buffer)
	d.subscriptions[subID] = &policySubscription{
		id:         subID,
		group:      group,
		groupKey:   groupKey,
		conditions: conditions,
		ch:         ch,
	}

	missing := conditions &^ d.registeredByGroup[groupKey]
	if missing == 0 {
		return subID, ch, nil
	}

	regID := d.nextLocked()
	registration := policyRegistration{
		id:         regID,
		group:      group,
		groupKey:   groupKey,
		conditions: missing,
	}
	d.registrations[regID] = registration
	d.registeredByGroup[groupKey] |= missing

	return subID, ch, &registration
}

// removeSubscription removes one listener and returns newly unused DCGM conditions.
func (d *policyDispatcher) removeSubscription(subID uint64) (chan PolicyViolation, []policyUnregister) {
	d.mu.Lock()
	defer d.mu.Unlock()

	sub, exists := d.subscriptions[subID]
	if !exists {
		return nil, nil
	}
	delete(d.subscriptions, subID)

	registered := d.registeredByGroup[sub.groupKey]
	if registered == 0 {
		return sub.ch, nil
	}

	var stillNeeded C.dcgmPolicyCondition_t
	for _, subscription := range d.subscriptions {
		if subscription.groupKey == sub.groupKey {
			stillNeeded |= subscription.conditions
		}
	}

	unused := registered &^ stillNeeded
	if unused == 0 {
		return sub.ch, nil
	}

	return sub.ch, []policyUnregister{{
		group:     sub.group,
		condition: unused,
	}}
}

// clearRegistrations removes bookkeeping for DCGM registrations that are gone.
func (d *policyDispatcher) clearRegistrations(unregisters []policyUnregister) {
	if len(unregisters) == 0 {
		return
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	for _, unregister := range unregisters {
		groupKey := unregister.group.GetHandle()
		for regID, registration := range d.registrations {
			if registration.groupKey != groupKey {
				continue
			}

			remaining := registration.conditions &^ unregister.condition
			if remaining == 0 {
				delete(d.registrations, regID)
				continue
			}

			registration.conditions = remaining
			d.registrations[regID] = registration
		}

		remaining := d.registeredByGroup[groupKey] &^ unregister.condition
		if remaining == 0 {
			delete(d.registeredByGroup, groupKey)
		} else {
			d.registeredByGroup[groupKey] = remaining
		}
	}
}

// rollbackSubscription undoes local state after DCGM registration fails.
func (d *policyDispatcher) rollbackSubscription(subID uint64, registration *policyRegistration) {
	d.mu.Lock()
	sub, exists := d.subscriptions[subID]
	if exists {
		delete(d.subscriptions, subID)
	}
	if registration != nil {
		delete(d.registrations, registration.id)
		remaining := d.registeredByGroup[registration.groupKey] &^ registration.conditions
		if remaining == 0 {
			delete(d.registeredByGroup, registration.groupKey)
		} else {
			d.registeredByGroup[registration.groupKey] = remaining
		}
	}
	d.mu.Unlock()

	if exists {
		close(sub.ch)
	}
}

// unsubscribe closes one listener and unregisters conditions no remaining listener needs.
func (d *policyDispatcher) unsubscribe(subID uint64) {
	d.registerMu.Lock()
	defer d.registerMu.Unlock()

	ch, unregisters := d.removeSubscription(subID)
	if ch == nil {
		return
	}
	close(ch)

	succeeded := make([]policyUnregister, 0, len(unregisters))
	for _, unregister := range unregisters {
		if err := unregisterPolicy(unregister.group, unregister.condition); err != nil {
			if unregisterErrorClearsLocalState(err) {
				log.Printf("policy unregister found no live DCGM registration for group %d condition %d: %v",
					unregister.group.GetHandle(), unregister.condition, err)
				succeeded = append(succeeded, unregister)
				continue
			}

			log.Printf("error unregistering policy for group %d condition %d: %v; retrying once",
				unregister.group.GetHandle(), unregister.condition, err)
			if retryErr := unregisterPolicy(unregister.group, unregister.condition); retryErr != nil {
				if unregisterErrorClearsLocalState(retryErr) {
					log.Printf("policy unregister retry found no live DCGM registration for group %d condition %d: %v",
						unregister.group.GetHandle(), unregister.condition, retryErr)
					succeeded = append(succeeded, unregister)
					continue
				}

				log.Printf("error unregistering policy for group %d condition %d after retry: %v",
					unregister.group.GetHandle(), unregister.condition, retryErr)
				continue
			}
		}
		succeeded = append(succeeded, unregister)
	}
	d.clearRegistrations(succeeded)
}

// deliver fans out a violation to interested subscribers without blocking.
func (d *policyDispatcher) deliver(registrationID uint64, violation PolicyViolation) {
	condition, ok := policyConditionMask(violation.Condition)
	if !ok {
		return
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	registration, exists := d.registrations[registrationID]
	if !exists {
		return
	}

	for _, subscription := range d.subscriptions {
		if subscription.groupKey != registration.groupKey || subscription.conditions&condition == 0 {
			continue
		}

		select {
		case subscription.ch <- violation:
		default:
			d.drops.Add(1)
		}
	}
}

// dropped returns the process-local count of best-effort delivery drops.
func (d *policyDispatcher) dropped() uint64 {
	return d.drops.Load()
}

// policyConditionMask returns the DCGM mask for a public policy condition.
func policyConditionMask(condition PolicyCondition) (C.dcgmPolicyCondition_t, bool) {
	return policyConditionInfo(condition)
}

// policyConditionInfo maps public policy conditions to DCGM condition masks.
func policyConditionInfo(condition PolicyCondition) (C.dcgmPolicyCondition_t, bool) {
	switch condition {
	case DbePolicy:
		return C.DCGM_POLICY_COND_DBE, true
	case PCIePolicy:
		return C.DCGM_POLICY_COND_PCI, true
	case MaxRtPgPolicy:
		return C.DCGM_POLICY_COND_MAX_PAGES_RETIRED, true
	case ThermalPolicy:
		return C.DCGM_POLICY_COND_THERMAL, true
	case PowerPolicy:
		return C.DCGM_POLICY_COND_POWER, true
	case NvlinkPolicy:
		return C.DCGM_POLICY_COND_NVLINK, true
	case XidPolicy:
		return C.DCGM_POLICY_COND_XID, true
	default:
		return 0, false
	}
}

// translateConditions validates requested conditions and builds their DCGM mask.
func translateConditions(conditions []PolicyCondition) (translatedPolicyConditions, error) {
	if len(conditions) == 0 {
		return translatedPolicyConditions{}, fmt.Errorf("at least one policy condition must be provided")
	}

	var translated translatedPolicyConditions
	for _, condition := range conditions {
		mask, ok := policyConditionInfo(condition)
		if !ok {
			return translatedPolicyConditions{}, fmt.Errorf("unknown policy condition: %s", condition)
		}
		translated.condition |= mask
	}

	return translated, nil
}

var policyConditionOrder = []PolicyCondition{
	DbePolicy,
	PCIePolicy,
	MaxRtPgPolicy,
	ThermalPolicy,
	PowerPolicy,
	NvlinkPolicy,
	XidPolicy,
}

// ensurePolicyForListen preserves existing policy config before a Listen subscription.
func ensurePolicyForListen(groupID GroupHandle, requested []PolicyCondition) error {
	status, err := getPolicyForGroup(groupID)
	if err != nil {
		if !policyReadNeedsDefaultSetup(err) {
			return fmt.Errorf("error getting policy before registering listener: %w", err)
		}

		configs, _ := policyConfigsForListen(nil, requested)
		return setPolicyForGroupWithConfig(groupID, configs...)
	}

	configs, needsUpdate := policyConfigsForListen(status, requested)
	if !needsUpdate {
		return nil
	}

	return setPolicyForGroupWithConfig(groupID, configs...)
}

// policyReadNeedsDefaultSetup reports whether no usable policy exists yet.
func policyReadNeedsDefaultSetup(err error) bool {
	var dcgmErr *Error
	if !errors.As(err, &dcgmErr) {
		return false
	}

	return dcgmErr.Code == C.DCGM_ST_INSUFFICIENT_SIZE ||
		dcgmErr.Code == C.DCGM_ST_NOT_CONFIGURED
}

// policyConfigsForListen merges missing Listen conditions into existing policy config.
func policyConfigsForListen(status *PolicyStatus, requested []PolicyCondition) ([]PolicyConfig, bool) {
	if status == nil {
		status = &PolicyStatus{
			Conditions: make(map[PolicyCondition]interface{}),
		}
	}
	if status.Conditions == nil {
		status.Conditions = make(map[PolicyCondition]interface{})
	}

	missing := make(map[PolicyCondition]struct{})
	for _, condition := range requested {
		if _, exists := status.Conditions[condition]; !exists {
			missing[condition] = struct{}{}
		}
	}
	if len(missing) == 0 {
		return nil, false
	}

	configs := make([]PolicyConfig, 0, len(status.Conditions)+len(missing))
	for _, condition := range policyConditionOrder {
		if value, exists := status.Conditions[condition]; exists {
			configs = append(configs, policyConfigFromStatus(condition, value))
			continue
		}
		if _, needsDefault := missing[condition]; needsDefault {
			configs = append(configs, defaultPolicyConfig(condition))
		}
	}

	if len(configs) > 0 {
		configs[0].Action = policyActionPtr(status.Action)
		configs[0].Validation = policyValidationPtr(status.Validation)
	}

	return configs, true
}

// policyConfigFromStatus converts a current policy condition into a set config.
func policyConfigFromStatus(condition PolicyCondition, value interface{}) PolicyConfig {
	config := PolicyConfig{Condition: condition}

	switch condition {
	case MaxRtPgPolicy:
		config.MaxRetiredPages = uint32PolicyPtr(policyThresholdUint32(value, DefaultMaxRetiredPages))
	case ThermalPolicy:
		config.MaxTemperature = uint32PolicyPtr(policyThresholdUint32(value, DefaultMaxTemperature))
	case PowerPolicy:
		config.MaxPower = uint32PolicyPtr(policyThresholdUint32(value, DefaultMaxPower))
	}

	return config
}

// defaultPolicyConfig returns default thresholds for a missing condition.
func defaultPolicyConfig(condition PolicyCondition) PolicyConfig {
	switch condition {
	case MaxRtPgPolicy:
		return PolicyConfig{
			Condition:       condition,
			MaxRetiredPages: uint32PolicyPtr(DefaultMaxRetiredPages),
		}
	case ThermalPolicy:
		return PolicyConfig{
			Condition:      condition,
			MaxTemperature: uint32PolicyPtr(DefaultMaxTemperature),
		}
	case PowerPolicy:
		return PolicyConfig{
			Condition: condition,
			MaxPower:  uint32PolicyPtr(DefaultMaxPower),
		}
	default:
		return PolicyConfig{Condition: condition}
	}
}

// policyThresholdUint32 normalizes numeric thresholds read from DCGM policy status.
func policyThresholdUint32(value interface{}, defaultValue uint32) uint32 {
	switch v := value.(type) {
	case uint32:
		return v
	case uint:
		return uint32(v)
	case int:
		if v >= 0 {
			return uint32(v)
		}
	case int32:
		if v >= 0 {
			return uint32(v)
		}
	case int64:
		if v >= 0 {
			return uint32(v)
		}
	}

	return defaultValue
}

// uint32PolicyPtr returns a stable pointer for PolicyConfig threshold fields.
func uint32PolicyPtr(value uint32) *uint32 {
	return &value
}

// policyActionPtr returns a stable pointer for PolicyConfig action.
func policyActionPtr(value PolicyAction) *PolicyAction {
	return &value
}

// policyValidationPtr returns a stable pointer for PolicyConfig validation.
func policyValidationPtr(value PolicyValidation) *PolicyValidation {
	return &value
}

// ViolationRegistration decodes a DCGM callback and dispatches by registration ID.
//
//export ViolationRegistration
func ViolationRegistration(data unsafe.Pointer, userData C.uint64_t) C.int {
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

	policyCallbacks.deliver(uint64(userData), err)

	return 0
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

// getPolicyForGroup returns current policy config while preserving DCGM error codes.
func getPolicyForGroup(groupID GroupHandle) (*PolicyStatus, error) {
	var policy C.dcgmPolicy_t
	policy.version = makeVersion1(unsafe.Sizeof(policy))

	var statusHandle C.dcgmStatus_t

	result := C.dcgmPolicyGet(handle.handle, groupID.handle, 1, &policy, statusHandle)
	if err := errorString(result); err != nil {
		return nil, &Error{msg: fmt.Sprintf("error getting policy: %s", err), Code: result}
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

// registerPolicy configures requested policy conditions before subscribing.
func registerPolicy(ctx context.Context, groupID GroupHandle, typ ...PolicyCondition) (<-chan PolicyViolation, error) {
	if ctx == nil {
		return nil, fmt.Errorf("context must not be nil")
	}

	translated, err := translateConditions(typ)
	if err != nil {
		return nil, err
	}

	return subscribePolicy(ctx, groupID, translated.condition, len(typ), func() error {
		return ensurePolicyForListen(groupID, typ)
	})
}

// registerPolicyOnly subscribes to existing policy conditions without changing thresholds.
func registerPolicyOnly(ctx context.Context, groupID GroupHandle, typ ...PolicyCondition) (<-chan PolicyViolation, error) {
	if ctx == nil {
		return nil, fmt.Errorf("context must not be nil")
	}

	translated, err := translateConditions(typ)
	if err != nil {
		return nil, err
	}

	return subscribePolicy(ctx, groupID, translated.condition, len(typ), nil)
}

// subscribePolicy serializes local subscription setup and DCGM registration.
func subscribePolicy(
	ctx context.Context,
	groupID GroupHandle,
	condition C.dcgmPolicyCondition_t,
	buffer int,
	setup func() error,
) (<-chan PolicyViolation, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	policyCallbacks.registerMu.Lock()
	defer policyCallbacks.registerMu.Unlock()

	if setup != nil {
		if err := setup(); err != nil {
			return nil, err
		}
	}

	subID, violation, registration := policyCallbacks.addSubscription(groupID, condition, buffer)
	if registration != nil {
		result := C.dcgmPolicyRegister_v2(
			handle.handle,
			groupID.handle,
			registration.conditions,
			C.fpRecvUpdates(C.violationNotify),
			C.uint64_t(registration.id),
		)
		if err := errorString(result); err != nil {
			policyCallbacks.rollbackSubscription(subID, registration)
			return nil, &Error{msg: C.GoString(C.errorString(result)), Code: result}
		}
	}

	context.AfterFunc(ctx, func() {
		policyCallbacks.unsubscribe(subID)
	})

	log.Println("Listening for violations...")

	return violation, nil
}

// unregisterPolicy unregisters DCGM callbacks for a group condition mask.
func unregisterPolicy(groupID GroupHandle, condition C.dcgmPolicyCondition_t) error {
	result := C.dcgmPolicyUnregister(handle.handle, groupID.handle, condition)

	if err := errorString(result); err != nil {
		return &Error{msg: fmt.Sprintf("error unregistering policy: %s", err), Code: result}
	}

	return nil
}

// unregisterErrorClearsLocalState reports whether DCGM state is already gone.
func unregisterErrorClearsLocalState(err error) bool {
	var dcgmErr *Error
	if !errors.As(err, &dcgmErr) {
		return false
	}

	return dcgmErr.Code == C.DCGM_ST_UNINITIALIZED ||
		dcgmErr.Code == C.DCGM_ST_CONNECTION_NOT_VALID
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
