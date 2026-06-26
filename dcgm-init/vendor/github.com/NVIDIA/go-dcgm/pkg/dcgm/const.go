package dcgm

import "C"

// Short is an alias for the C.ushort type.
// It is primarily used for DCGM field identifiers and field collections
// in the DCGM API bindings. This type provides a direct mapping to the
// C unsigned short type used in the underlying DCGM C API.
type Short C.ushort

// FieldValue_v1 represents a field value in version 1
type FieldValue_v1 struct {
	Version   uint
	FieldID   Short
	FieldType uint
	Status    int
	TS        int64
	Value     [4096]byte
}

// FieldValue_v2 represents a field value in version 2
type FieldValue_v2 struct {
	Version       uint
	EntityGroupId Field_Entity_Group
	EntityID      uint
	FieldID       Short
	FieldType     uint
	Status        int
	TS            int64
	Value         [4096]byte
	StringValue   *string
}

// FieldType constants
const (
	// DCGM_FT_BINARY is the type for binary data
	DCGM_FT_BINARY = uint('b')
	// DCGM_FT_DOUBLE is the type for floating-point numbers
	DCGM_FT_DOUBLE = uint('d')
	// DCGM_FT_INT64 is the type for 64-bit integers
	DCGM_FT_INT64 = uint('i')
	// DCGM_FT_STRING is the type for strings
	DCGM_FT_STRING = uint('s')
	// DCGM_FT_TIMESTAMP is the type for timestamps
	DCGM_FT_TIMESTAMP = uint('t')
	// DCGM_FT_INT32_BLANK is the blank value for 32-bit integers
	DCGM_FT_INT32_BLANK = int64(2147483632)
	// DCGM_FT_INT32_NOT_FOUND is the value for not found in 32-bit integers
	DCGM_FT_INT32_NOT_FOUND = DCGM_FT_INT32_BLANK + 1
	// DCGM_FT_INT32_NOT_SUPPORTED is the value for not supported in 32-bit integers
	DCGM_FT_INT32_NOT_SUPPORTED = DCGM_FT_INT32_BLANK + 2
	// DCGM_FT_INT32_NOT_PERMISSIONED is the value for not permissioned in 32-bit integers
	DCGM_FT_INT32_NOT_PERMISSIONED = DCGM_FT_INT32_BLANK + 3
	// DCGM_FT_INT64_BLANK is the blank value for 64-bit integers
	DCGM_FT_INT64_BLANK = int64(9223372036854775792)
	// DCGM_FT_INT64_NOT_FOUND is the value for not found in 64-bit integers
	DCGM_FT_INT64_NOT_FOUND = DCGM_FT_INT64_BLANK + 1
	// DCGM_FT_INT64_NOT_SUPPORTED is the value for not supported in 64-bit integers
	DCGM_FT_INT64_NOT_SUPPORTED = DCGM_FT_INT64_BLANK + 2
	// DCGM_FT_INT64_NOT_PERMISSIONED is the value for not permissioned in 64-bit integers
	DCGM_FT_INT64_NOT_PERMISSIONED = DCGM_FT_INT64_BLANK + 3
	// DCGM_FT_FP64_BLANK is the blank value for floating-point numbers
	DCGM_FT_FP64_BLANK = 140737488355328.0
	// DCGM_FT_FP64_NOT_FOUND is the value for not found in floating-point numbers
	DCGM_FT_FP64_NOT_FOUND = float64(DCGM_FT_FP64_BLANK + 1.0)
	// DCGM_FT_FP64_NOT_SUPPORTED is the value for not supported in floating-point numbers
	DCGM_FT_FP64_NOT_SUPPORTED = float64(DCGM_FT_FP64_BLANK + 2.0)
	// DCGM_FT_FP64_NOT_PERMISSIONED is the value for not permissioned in floating-point numbers
	DCGM_FT_FP64_NOT_PERMISSIONED = float64(DCGM_FT_FP64_BLANK + 3.0)
	// DCGM_FT_STR_BLANK is the blank value for strings
	DCGM_FT_STR_BLANK = "<<<NULL>>>"
	// DCGM_FT_STR_NOT_FOUND is the value for not found in strings
	DCGM_FT_STR_NOT_FOUND = "<<<NOT_FOUND>>>"
	// DCGM_FT_STR_NOT_SUPPORTED is the value for not supported in strings
	DCGM_FT_STR_NOT_SUPPORTED = "<<<NOT_SUPPORTED>>>"
	// DCGM_FT_STR_NOT_PERMISSIONED is the value for not permissioned in strings
	DCGM_FT_STR_NOT_PERMISSIONED = "<<<NOT_PERMISSIONED>>>"

	// DCGM_ST_OK is the value for ECC OK
	DCGM_ST_OK = 0
	// DCGM_ST_BADPARAM is the value for ECC BAD PARAM
	DCGM_ST_BADPARAM = -1
	// DCGM_ST_GENERIC_ERROR is the value for ECC GENERIC ERROR
	DCGM_ST_GENERIC_ERROR = -3
	// DCGM_ST_MEMORY is the value for ECC MEMORY
	DCGM_ST_MEMORY = -4
	// DCGM_ST_NOT_CONFIGURED is the value for ECC NOT CONFIGURED
	DCGM_ST_NOT_CONFIGURED = -5
	// DCGM_ST_NOT_SUPPORTED is the value for ECC NOT SUPPORTED
	DCGM_ST_NOT_SUPPORTED = -6
	// DCGM_ST_INIT_ERROR is the value for ECC INIT ERROR
	DCGM_ST_INIT_ERROR = -7
	// DCGM_ST_NVML_ERROR is the value for ECC NVML ERROR
	DCGM_ST_NVML_ERROR = -8
	// DCGM_ST_PENDING is the value for ECC PENDING
	DCGM_ST_PENDING = -9
	// DCGM_ST_TIMEOUT is the value for ECC TIMEOUT
	DCGM_ST_TIMEOUT = -11
	// DCGM_ST_VER_MISMATCH is the value for ECC VER MISMATCH
	DCGM_ST_VER_MISMATCH = -12
	// DCGM_ST_UNKNOWN_FIELD is the value for ECC UNKNOWN FIELD
	DCGM_ST_UNKNOWN_FIELD = -13
	// DCGM_ST_NO_DATA is the value for ECC NO DATA
	DCGM_ST_NO_DATA = -14
	// DCGM_ST_STALE_DATA is the value for ECC STALE DATA
	DCGM_ST_STALE_DATA = -15
	// DCGM_ST_NOT_WATCHED is the value for ECC NOT WATCHED
	DCGM_ST_NOT_WATCHED = -16
	// DCGM_ST_NO_PERMISSION is the value for ECC NO PERMISSION
	DCGM_ST_NO_PERMISSION = -17
	// DCGM_ST_GPU_IS_LOST is the value for ECC GPU IS LOST
	DCGM_ST_GPU_IS_LOST = -18
	// DCGM_ST_RESET_REQUIRED is the value for ECC RESET REQUIRED
	DCGM_ST_RESET_REQUIRED = -19
	// DCGM_ST_FUNCTION_NOT_FOUND is the value for ECC FUNCTION NOT FOUND
	DCGM_ST_FUNCTION_NOT_FOUND = -20
	// DCGM_ST_CONNECTION_NOT_VALID is the value for ECC CONNECTION NOT VALID
	DCGM_ST_CONNECTION_NOT_VALID = -21
	// DCGM_ST_GPU_NOT_SUPPORTED is the value for ECC GPU NOT SUPPORTED
	DCGM_ST_GPU_NOT_SUPPORTED = -22
	// DCGM_ST_GROUP_INCOMPATIBLE is the value for ECC GROUP INCOMPATIBLE
	DCGM_ST_GROUP_INCOMPATIBLE = -23
	// DCGM_ST_MAX_LIMIT is the value for ECC MAX LIMIT
	DCGM_ST_MAX_LIMIT = -24
	// DCGM_ST_LIBRARY_NOT_FOUND is the value for ECC LIBRARY NOT FOUND
	DCGM_ST_LIBRARY_NOT_FOUND = -25
	// DCGM_ST_DUPLICATE_KEY is the value for ECC DUPLICATE KEY
	DCGM_ST_DUPLICATE_KEY = -26
	// DCGM_ST_GPU_IN_SYNC_BOOST_GROUP is the value for ECC GPU IN SYNC BOOST GROUP
	DCGM_ST_GPU_IN_SYNC_BOOST_GROUP = -27
	// DCGM_ST_GPU_NOT_IN_SYNC_BOOST_GROUP is the value for ECC GPU NOT IN SYNC BOOST GROUP
	DCGM_ST_GPU_NOT_IN_SYNC_BOOST_GROUP = -28
	// DCGM_ST_REQUIRES_ROOT is the value for ECC REQUIRES ROOT
	DCGM_ST_REQUIRES_ROOT = -29
	// DCGM_ST_NVVS_ERROR is the value for ECC NVVS ERROR
	DCGM_ST_NVVS_ERROR = -30
	// DCGM_ST_INSUFFICIENT_SIZE is the value for ECC INSUFFICIENT SIZE
	DCGM_ST_INSUFFICIENT_SIZE = -31
	// DCGM_ST_FIELD_UNSUPPORTED_BY_API is the value for ECC FIELD UNSUPPORTED BY API
	DCGM_ST_FIELD_UNSUPPORTED_BY_API = -32
	// DCGM_ST_MODULE_NOT_LOADED is the value for ECC MODULE NOT LOADED
	DCGM_ST_MODULE_NOT_LOADED = -33
	// DCGM_ST_IN_USE is the value for ECC IN USE
	DCGM_ST_IN_USE = -34
	// DCGM_ST_GROUP_IS_EMPTY is the value for ECC GROUP IS EMPTY
	DCGM_ST_GROUP_IS_EMPTY = -35
	// DCGM_ST_PROFILING_NOT_SUPPORTED is the value for ECC PROFILING NOT SUPPORTED
	DCGM_ST_PROFILING_NOT_SUPPORTED = -36
	// DCGM_ST_PROFILING_LIBRARY_ERROR is the value for ECC PROFILING LIBRARY ERROR
	DCGM_ST_PROFILING_LIBRARY_ERROR = -37
	// DCGM_ST_PROFILING_MULTI_PASS is the value for ECC PROFILING MULTI PASS
	DCGM_ST_PROFILING_MULTI_PASS = -38
	// DCGM_ST_DIAG_ALREADY_RUNNING is the value for ECC DIAG ALREADY RUNNING
	DCGM_ST_DIAG_ALREADY_RUNNING = -39
	// DCGM_ST_DIAG_BAD_JSON is the value for ECC DIAG BAD JSON
	DCGM_ST_DIAG_BAD_JSON = -40
	// DCGM_ST_DIAG_BAD_LAUNCH is the value for ECC DIAG BAD LAUNCH
	DCGM_ST_DIAG_BAD_LAUNCH = -41
	// DCGM_ST_DIAG_UNUSED is the value for ECC DIAG UNUSED
	DCGM_ST_DIAG_UNUSED = -42
	// DCGM_ST_DIAG_THRESHOLD_EXCEEDED is the value for ECC DIAG THRESHOLD EXCEEDED
	DCGM_ST_DIAG_THRESHOLD_EXCEEDED = -43
	// DCGM_ST_INSUFFICIENT_DRIVER_VERSION is the value for ECC INSUFFICIENT DRIVER VERSION
	DCGM_ST_INSUFFICIENT_DRIVER_VERSION = -44
	// DCGM_ST_INSTANCE_NOT_FOUND is the value for ECC INSTANCE NOT FOUND
	DCGM_ST_INSTANCE_NOT_FOUND = -45
	// DCGM_ST_COMPUTE_INSTANCE_NOT_FOUND is the value for ECC COMPUTE INSTANCE NOT FOUND
	DCGM_ST_COMPUTE_INSTANCE_NOT_FOUND = -46
	// DCGM_ST_CHILD_NOT_KILLED is the value for ECC CHILD NOT KILLED
	DCGM_ST_CHILD_NOT_KILLED = -47
	// DCGM_ST_3RD_PARTY_LIBRARY_ERROR is the value for ECC 3RD PARTY LIBRARY ERROR
	DCGM_ST_3RD_PARTY_LIBRARY_ERROR = -48
	// DCGM_ST_INSUFFICIENT_RESOURCES is the value for ECC INSUFFICIENT RESOURCES
	DCGM_ST_INSUFFICIENT_RESOURCES = -49
	// DCGM_ST_PLUGIN_EXCEPTION is the value for ECC PLUGIN EXCEPTION
	DCGM_ST_PLUGIN_EXCEPTION = -50
	// DCGM_ST_NVVS_ISOLATE_ERROR is the value for ECC NVVS ISOLATE ERROR
	DCGM_ST_NVVS_ISOLATE_ERROR = -51
	// DCGM_ST_NVVS_BINARY_NOT_FOUND is the value for ECC NVVS BINARY NOT FOUND
	DCGM_ST_NVVS_BINARY_NOT_FOUND = -52
	// DCGM_ST_NVVS_KILLED is the value for ECC NVVS KILLED
	DCGM_ST_NVVS_KILLED = -53
	// DCGM_ST_PAUSED is the value for ECC PAUSED
	DCGM_ST_PAUSED = -54
	// DCGM_ST_ALREADY_INITIALIZED is the value for ECC ALREADY INITIALIZED
	DCGM_ST_ALREADY_INITIALIZED = -55
	// DCGM_ST_NVML_NOT_LOADED is the value for ECC NVML NOT LOADED
	DCGM_ST_NVML_NOT_LOADED = -56
	// DCGM_ST_NVML_DRIVER_TIMEOUT is the value for ECC NVML DRIVER TIMEOUT
	DCGM_ST_NVML_DRIVER_TIMEOUT = -57
	// DCGM_ST_NVVS_NO_AVAILABLE_TEST is the value for ECC NVVS NO AVAILABLE TEST
	DCGM_ST_NVVS_NO_AVAILABLE_TEST = -58
)

// DCGM_FV_FLAG_LIVE_DATA is a flag for the DCGM fields.
const (
	DCGM_FV_FLAG_LIVE_DATA = uint(0x00000001)
)

// HealthSystem is the system to watch for health checks.
type HealthSystem uint

const (
	// DCGM_HEALTH_WATCH_PCIE PCIe health check
	DCGM_HEALTH_WATCH_PCIE HealthSystem = 0x1
	// DCGM_HEALTH_WATCH_NVLINK NVLink health check
	DCGM_HEALTH_WATCH_NVLINK HealthSystem = 0x2
	// DCGM_HEALTH_WATCH_PMU PMU health check
	DCGM_HEALTH_WATCH_PMU HealthSystem = 0x4
	// DCGM_HEALTH_WATCH_MCU MCU health check
	DCGM_HEALTH_WATCH_MCU HealthSystem = 0x8
	// DCGM_HEALTH_WATCH_MEM Memory health check
	DCGM_HEALTH_WATCH_MEM HealthSystem = 0x10
	// DCGM_HEALTH_WATCH_SM SM health check
	DCGM_HEALTH_WATCH_SM HealthSystem = 0x20
	// DCGM_HEALTH_WATCH_INFOROM Inforom health check
	DCGM_HEALTH_WATCH_INFOROM HealthSystem = 0x40
	// DCGM_HEALTH_WATCH_THERMAL Thermal health check
	DCGM_HEALTH_WATCH_THERMAL HealthSystem = 0x80
	// DCGM_HEALTH_WATCH_POWER Power health check
	DCGM_HEALTH_WATCH_POWER HealthSystem = 0x100
	// DCGM_HEALTH_WATCH_DRIVER Driver health check
	DCGM_HEALTH_WATCH_DRIVER HealthSystem = 0x200
	// DCGM_HEALTH_WATCH_NVSWITCH_NONFATAL NVSwitch non-fatal health check
	DCGM_HEALTH_WATCH_NVSWITCH_NONFATAL HealthSystem = 0x400
	// DCGM_HEALTH_WATCH_NVSWITCH_FATAL NVSwitch fatal health check
	DCGM_HEALTH_WATCH_NVSWITCH_FATAL HealthSystem = 0x800
	// DCGM_HEALTH_WATCH_ALL All health checks
	DCGM_HEALTH_WATCH_ALL HealthSystem = 0xFFFFFFFF
)

// HealthResult is the result of a health check.
type HealthResult uint

const (
	// DCGM_HEALTH_RESULT_PASS All results within this system are reporting normal
	DCGM_HEALTH_RESULT_PASS HealthResult = 0
	// DCGM_HEALTH_RESULT_WARN A warning has been issued, refer to the response for more information
	DCGM_HEALTH_RESULT_WARN HealthResult = 10
	// DCGM_HEALTH_RESULT_FAIL A failure has been issued, refer to the response for more information
	DCGM_HEALTH_RESULT_FAIL HealthResult = 20
)

// HealthCheckErrorCode error codes for passive and active health checks.
type HealthCheckErrorCode uint

const (
	// DCGM_FR_OK No error
	DCGM_FR_OK HealthCheckErrorCode = 0
	// DCGM_FR_UNKNOWN Unknown error code
	DCGM_FR_UNKNOWN HealthCheckErrorCode = 1
	// DCGM_FR_UNRECOGNIZED Unrecognized error code
	DCGM_FR_UNRECOGNIZED HealthCheckErrorCode = 2
	// DCGM_FR_PCI_REPLAY_RATE Unacceptable rate of PCI errors
	DCGM_FR_PCI_REPLAY_RATE HealthCheckErrorCode = 3
	// DCGM_FR_VOLATILE_DBE_DETECTED Unacceptable rate of volatile double bit errors
	DCGM_FR_VOLATILE_DBE_DETECTED HealthCheckErrorCode = 4
	// DCGM_FR_VOLATILE_SBE_DETECTED Unacceptable rate of volatile single bit errors
	DCGM_FR_VOLATILE_SBE_DETECTED HealthCheckErrorCode = 5
	// DCGM_FR_VOLATILE_SBE_DETECTED_TS Unacceptable rate of volatile single bit errors with a timestamp
	DCGM_FR_VOLATILE_SBE_DETECTED_TS HealthCheckErrorCode = 6
	// DCGM_FR_PENDING_PAGE_RETIREMENTS Pending page retirements detected
	DCGM_FR_PENDING_PAGE_RETIREMENTS HealthCheckErrorCode = 6
	// DCGM_FR_RETIRED_PAGES_LIMIT Unacceptable total page retirements detected
	DCGM_FR_RETIRED_PAGES_LIMIT HealthCheckErrorCode = 7
	// DCGM_FR_RETIRED_PAGES_DBE_LIMIT Unacceptable total page retirements due to uncorrectable errors
	DCGM_FR_RETIRED_PAGES_DBE_LIMIT HealthCheckErrorCode = 8
	// DCGM_FR_CORRUPT_INFOROM Corrupt inforom found
	DCGM_FR_CORRUPT_INFOROM HealthCheckErrorCode = 9
	// DCGM_FR_CLOCK_THROTTLE_THERMAL Clocks being throttled due to overheating
	DCGM_FR_CLOCK_THROTTLE_THERMAL HealthCheckErrorCode = 10
	// DCGM_FR_POWER_UNREADABLE Cannot get a reading for power from NVML
	DCGM_FR_POWER_UNREADABLE HealthCheckErrorCode = 11
	// DCGM_FR_CLOCK_THROTTLE_POWER Clock being throttled due to power restrictions
	DCGM_FR_CLOCK_THROTTLE_POWER HealthCheckErrorCode = 12
	// DCGM_FR_NVLINK_ERROR_THRESHOLD Unacceptable rate of NVLink errors
	DCGM_FR_NVLINK_ERROR_THRESHOLD HealthCheckErrorCode = 13
	// DCGM_FR_NVLINK_DOWN NVLink is down
	DCGM_FR_NVLINK_DOWN HealthCheckErrorCode = 14
	// DCGM_FR_NVSWITCH_FATAL_ERROR Fatal errors on the NVSwitch
	DCGM_FR_NVSWITCH_FATAL_ERROR HealthCheckErrorCode = 15
	// DCGM_FR_NVSWITCH_NON_FATAL_ERROR Non-fatal errors on the NVSwitch
	DCGM_FR_NVSWITCH_NON_FATAL_ERROR HealthCheckErrorCode = 16
	// DCGM_FR_NVSWITCH_DOWN NVSwitch is down
	DCGM_FR_NVSWITCH_DOWN HealthCheckErrorCode = 17
	// DCGM_FR_NO_ACCESS_TO_FILE Cannot access a file
	DCGM_FR_NO_ACCESS_TO_FILE HealthCheckErrorCode = 18
	// DCGM_FR_NVML_API Error occurred on an NVML API - NOT USED: DEPRECATED
	DCGM_FR_NVML_API HealthCheckErrorCode = 19
	// DCGM_FR_DEVICE_COUNT_MISMATCH Device count mismatch
	DCGM_FR_DEVICE_COUNT_MISMATCH HealthCheckErrorCode = 20
	// DCGM_FR_BAD_PARAMETER Bad parameter passed to API
	DCGM_FR_BAD_PARAMETER HealthCheckErrorCode = 21
	// DCGM_FR_CANNOT_OPEN_LIB Cannot open a library that must be accessed
	DCGM_FR_CANNOT_OPEN_LIB HealthCheckErrorCode = 22
	// DCGM_FR_DENYLISTED_DRIVER A driver on the denylist (nouveau) is active
	DCGM_FR_DENYLISTED_DRIVER HealthCheckErrorCode = 23
	// DCGM_FR_NVML_LIB_BAD NVML library is missing expected functions - NOT USED: DEPRECATED
	DCGM_FR_NVML_LIB_BAD HealthCheckErrorCode = 24
	// DCGM_FR_GRAPHICS_PROCESSES HealthCheckErrorCode = 25
	DCGM_FR_GRAPHICS_PROCESSES HealthCheckErrorCode = 25
	// DCGM_FR_HOSTENGINE_CONN Bad connection to nv-hostengine - NOT USED: DEPRECATED
	DCGM_FR_HOSTENGINE_CONN HealthCheckErrorCode = 26
	// DCGM_FR_FIELD_QUERY Field query failed
	DCGM_FR_FIELD_QUERY HealthCheckErrorCode = 27
	// DCGM_FR_BAD_CUDA_ENV The environment has variables that hurt CUDA
	DCGM_FR_BAD_CUDA_ENV HealthCheckErrorCode = 28
	// DCGM_FR_PERSISTENCE_MODE Persistence mode is disabled
	DCGM_FR_PERSISTENCE_MODE HealthCheckErrorCode = 29
	// DCGM_FR_BAD_NVLINK_ENV The environment has variables that hurt NVLink
	DCGM_FR_BAD_NVLINK_ENV HealthCheckErrorCode = 29
	// DCGM_FR_LOW_BANDWIDTH The bandwidth is unacceptably low
	DCGM_FR_LOW_BANDWIDTH HealthCheckErrorCode = 30
	// DCGM_FR_HIGH_LATENCY Latency is too high
	DCGM_FR_HIGH_LATENCY HealthCheckErrorCode = 31
	// DCGM_FR_CANNOT_GET_FIELD_TAG Cannot find a tag for a field
	DCGM_FR_CANNOT_GET_FIELD_TAG HealthCheckErrorCode = 32
	// DCGM_FR_FIELD_VIOLATION The value for the specified error field is above 0
	DCGM_FR_FIELD_VIOLATION HealthCheckErrorCode = 33
	// DCGM_FR_FIELD_THRESHOLD The value for the specified field is above the threshold
	DCGM_FR_FIELD_THRESHOLD HealthCheckErrorCode = 34
	// DCGM_FR_FIELD_VIOLATION_DBL The value for the specified error field is above 0
	DCGM_FR_FIELD_VIOLATION_DBL HealthCheckErrorCode = 35
	// DCGM_FR_FIELD_THRESHOLD_DBL The value for the specified field is above the threshold
	DCGM_FR_FIELD_THRESHOLD_DBL HealthCheckErrorCode = 36
	// DCGM_FR_UNSUPPORTED_FIELD_TYPE Field type cannot be supported
	DCGM_FR_UNSUPPORTED_FIELD_TYPE HealthCheckErrorCode = 37
	// DCGM_FR_FIELD_THRESHOLD_TS The value for the specified field is above the threshold
	DCGM_FR_FIELD_THRESHOLD_TS HealthCheckErrorCode = 38
	// DCGM_FR_FIELD_THRESHOLD_TS_DBL The value for the specified field is above the threshold
	DCGM_FR_FIELD_THRESHOLD_TS_DBL HealthCheckErrorCode = 39
	// DCGM_FR_THERMAL_VIOLATIONS Thermal violations detected
	DCGM_FR_THERMAL_VIOLATIONS HealthCheckErrorCode = 40
	// DCGM_FR_THERMAL_VIOLATIONS_TS Thermal violations detected with a timestamp
	DCGM_FR_THERMAL_VIOLATIONS_TS HealthCheckErrorCode = 41
	// DCGM_FR_TEMP_VIOLATION Non-benign clock throttling is occurring
	DCGM_FR_TEMP_VIOLATION HealthCheckErrorCode = 42
	// DCGM_FR_THROTTLING_VIOLATION Non-benign clock throttling is occurring
	DCGM_FR_THROTTLING_VIOLATION HealthCheckErrorCode = 43
	// DCGM_FR_INTERNAL An internal error was detected
	DCGM_FR_INTERNAL HealthCheckErrorCode = 44
	// DCGM_FR_PCIE_GENERATION PCIe generation is too low
	DCGM_FR_PCIE_GENERATION HealthCheckErrorCode = 45
	// DCGM_FR_PCIE_WIDTH PCIe width is too low
	DCGM_FR_PCIE_WIDTH HealthCheckErrorCode = 46
	// DCGM_FR_ABORTED Test was aborted by a user signal
	DCGM_FR_ABORTED HealthCheckErrorCode = 47
	// DCGM_FR_TEST_DISABLED Test was disabled by a user signal
	DCGM_FR_TEST_DISABLED HealthCheckErrorCode = 48
	// DCGM_FR_CANNOT_GET_STAT Cannot get telemetry for a needed value
	DCGM_FR_CANNOT_GET_STAT HealthCheckErrorCode = 49
	// DCGM_FR_STRESS_LEVEL Stress level is too low (bad performance)
	DCGM_FR_STRESS_LEVEL HealthCheckErrorCode = 50
	// DCGM_FR_CUDA_API HealthCheckErrorCode = 51
	DCGM_FR_CUDA_API HealthCheckErrorCode = 51
	// DCGM_FR_FAULTY_MEMORY Faulty memory detected on this GPU
	DCGM_FR_FAULTY_MEMORY HealthCheckErrorCode = 52
	// DCGM_FR_CANNOT_SET_WATCHES Unable to set field watches in DCGM - NOT USED: DEPRECATED
	DCGM_FR_CANNOT_SET_WATCHES HealthCheckErrorCode = 53
	// DCGM_FR_CUDA_UNBOUND CUDA context is no longer bound
	DCGM_FR_CUDA_UNBOUND HealthCheckErrorCode = 54
	// DCGM_FR_ECC_DISABLED ECC memory is disabled right now
	DCGM_FR_ECC_DISABLED HealthCheckErrorCode = 55
	// DCGM_FR_MEMORY_ALLOC Cannot allocate memory on the GPU
	DCGM_FR_MEMORY_ALLOC HealthCheckErrorCode = 56
	// DCGM_FR_CUDA_DBE CUDA detected unrecovable double-bit error
	DCGM_FR_CUDA_DBE HealthCheckErrorCode = 57
	// DCGM_FR_MEMORY_MISMATCH Memory error detected
	DCGM_FR_MEMORY_MISMATCH HealthCheckErrorCode = 58
	// DCGM_FR_CUDA_DEVICE No CUDA device discoverable for existing GPU
	DCGM_FR_CUDA_DEVICE HealthCheckErrorCode = 59
	// DCGM_FR_ECC_UNSUPPORTED ECC memory is unsupported by this SKU
	DCGM_FR_ECC_UNSUPPORTED HealthCheckErrorCode = 60
	// DCGM_FR_ECC_PENDING ECC memory is in a pending state - NOT USED: DEPRECATED
	DCGM_FR_ECC_PENDING HealthCheckErrorCode = 61
	// DCGM_FR_MEMORY_BANDWIDTH Memory bandwidth is too low
	DCGM_FR_MEMORY_BANDWIDTH HealthCheckErrorCode = 62
	// DCGM_FR_TARGET_POWER The target power is too low
	DCGM_FR_TARGET_POWER HealthCheckErrorCode = 63
	// DCGM_FR_API_FAIL The specified API call failed
	DCGM_FR_API_FAIL HealthCheckErrorCode = 64
	// DCGM_FR_API_FAIL_GPU The specified API call failed for the specified GPU
	DCGM_FR_API_FAIL_GPU HealthCheckErrorCode = 65
	// DCGM_FR_CUDA_CONTEXT Cannot create a CUDA context on this GPU
	DCGM_FR_CUDA_CONTEXT HealthCheckErrorCode = 66
	// DCGM_FR_DCGM_API DCGM API failure
	DCGM_FR_DCGM_API HealthCheckErrorCode = 67
	// DCGM_FR_CONCURRENT_GPUS Need multiple GPUs to run this test
	DCGM_FR_CONCURRENT_GPUS HealthCheckErrorCode = 68
	// DCGM_FR_TOO_MANY_ERRORS More errors than fit in the return struct - NOT USED: DEPRECATED
	DCGM_FR_TOO_MANY_ERRORS HealthCheckErrorCode = 69
	// DCGM_FR_NVLINK_CRC_ERROR_THRESHOLD NVLink CRC error threshold violation
	DCGM_FR_NVLINK_CRC_ERROR_THRESHOLD HealthCheckErrorCode = 70
	// DCGM_FR_NVLINK_ERROR_CRITICAL NVLink error for a field that should always be 0
	DCGM_FR_NVLINK_ERROR_CRITICAL HealthCheckErrorCode = 71
	// DCGM_FR_ENFORCED_POWER_LIMIT The enforced power limit is too low to hit the target
	DCGM_FR_ENFORCED_POWER_LIMIT HealthCheckErrorCode = 72
	// DCGM_FR_MEMORY_ALLOC_HOST Cannot allocate memory on the host
	DCGM_FR_MEMORY_ALLOC_HOST HealthCheckErrorCode = 73
	// DCGM_FR_GPU_OP_MODE Bad GPU operating mode for running plugin - NOT USED: DEPRECATED
	DCGM_FR_GPU_OP_MODE HealthCheckErrorCode = 74
	// DCGM_FR_NO_MEMORY_CLOCKS No memory clocks with the needed MHz found - NOT USED: DEPRECATED
	DCGM_FR_NO_MEMORY_CLOCKS HealthCheckErrorCode = 75
	// DCGM_FR_NO_GRAPHICS_CLOCKS No graphics clocks with the needed MHz found - NOT USED: DEPRECATED
	DCGM_FR_NO_GRAPHICS_CLOCKS HealthCheckErrorCode = 76
	// DCGM_FR_HAD_TO_RESTORE_STATE Note that we had to restore a GPU's state
	DCGM_FR_HAD_TO_RESTORE_STATE HealthCheckErrorCode = 77
	// DCGM_FR_L1TAG_UNSUPPORTED L1TAG test is unsupported by this SKU
	DCGM_FR_L1TAG_UNSUPPORTED HealthCheckErrorCode = 78
	// DCGM_FR_L1TAG_MISCOMPARE L1TAG test failed on a miscompare
	DCGM_FR_L1TAG_MISCOMPARE HealthCheckErrorCode = 79
	// DCGM_FR_ROW_REMAP_FAILURE Row remapping failed (Ampere or newer GPUs)
	DCGM_FR_ROW_REMAP_FAILURE HealthCheckErrorCode = 80
	// DCGM_FR_UNCONTAINED_ERROR Uncontained error - XID 95
	DCGM_FR_UNCONTAINED_ERROR HealthCheckErrorCode = 81
	// DCGM_FR_EMPTY_GPU_LIST No GPU information given to plugin
	DCGM_FR_EMPTY_GPU_LIST HealthCheckErrorCode = 82
	// DCGM_FR_DBE_PENDING_PAGE_RETIREMENTS Pending page retirements due to a DBE
	DCGM_FR_DBE_PENDING_PAGE_RETIREMENTS HealthCheckErrorCode = 83
	// DCGM_FR_UNCORRECTABLE_ROW_REMAP Uncorrectable row remapping
	DCGM_FR_UNCORRECTABLE_ROW_REMAP HealthCheckErrorCode = 84
	// DCGM_FR_PENDING_ROW_REMAP Row remapping is pending
	DCGM_FR_PENDING_ROW_REMAP HealthCheckErrorCode = 85
	// DCGM_FR_BROKEN_P2P_MEMORY_DEVICE P2P copy test detected an error writing to this GPU
	DCGM_FR_BROKEN_P2P_MEMORY_DEVICE HealthCheckErrorCode = 86
	// DCGM_FR_BROKEN_P2P_WRITER_DEVICE P2P copy test detected an error writing from this GPU
	DCGM_FR_BROKEN_P2P_WRITER_DEVICE HealthCheckErrorCode = 87
	// DCGM_FR_NVSWITCH_NVLINK_DOWN An NvLink is down for the specified NVSwitch
	DCGM_FR_NVSWITCH_NVLINK_DOWN HealthCheckErrorCode = 88
	// DCGM_FR_EUD_BINARY_PERMISSIONS EUD binary permissions are incorrect
	DCGM_FR_EUD_BINARY_PERMISSIONS HealthCheckErrorCode = 89
	// DCGM_FR_EUD_NON_ROOT_USER EUD plugin is not running as root
	DCGM_FR_EUD_NON_ROOT_USER HealthCheckErrorCode = 90
	// DCGM_FR_EUD_SPAWN_FAILURE EUD plugin failed to spawn the EUD binary
	DCGM_FR_EUD_SPAWN_FAILURE HealthCheckErrorCode = 91
	// DCGM_FR_EUD_TIMEOUT EUD plugin timed out
	DCGM_FR_EUD_TIMEOUT HealthCheckErrorCode = 92
	// DCGM_FR_EUD_ZOMBIE EUD process remains running after the plugin considers it finished
	DCGM_FR_EUD_ZOMBIE HealthCheckErrorCode = 93
	// DCGM_FR_EUD_NON_ZERO_EXIT_CODE EUD process exited with a non-zero exit code
	DCGM_FR_EUD_NON_ZERO_EXIT_CODE HealthCheckErrorCode = 94
	// DCGM_FR_EUD_TEST_FAILED EUD test failed
	DCGM_FR_EUD_TEST_FAILED HealthCheckErrorCode = 95
	// DCGM_FR_FILE_CREATE_PERMISSIONS We cannot create a file in this directory.
	DCGM_FR_FILE_CREATE_PERMISSIONS HealthCheckErrorCode = 96
	// DCGM_FR_PAUSE_RESUME_FAILED Pause/Resume failed
	DCGM_FR_PAUSE_RESUME_FAILED HealthCheckErrorCode = 97
	// DCGM_FR_PCIE_H_REPLAY_VIOLATION PCIe H replay violation
	DCGM_FR_PCIE_H_REPLAY_VIOLATION HealthCheckErrorCode = 98
	// DCGM_FR_GPU_EXPECTED_NVLINKS_UP Expected nvlinks up per gpu
	DCGM_FR_GPU_EXPECTED_NVLINKS_UP HealthCheckErrorCode = 99
	// DCGM_FR_NVSWITCH_EXPECTED_NVLINKS_UP Expected nvlinks up per nvswitch
	DCGM_FR_NVSWITCH_EXPECTED_NVLINKS_UP HealthCheckErrorCode = 100
	// DCGM_FR_XID_ERROR XID error detected
	DCGM_FR_XID_ERROR HealthCheckErrorCode = 101
	// DCGM_FR_SBE_VIOLATION Single bit error detected
	DCGM_FR_SBE_VIOLATION HealthCheckErrorCode = 102
	// DCGM_FR_DBE_VIOLATION Double bit error detected
	DCGM_FR_DBE_VIOLATION HealthCheckErrorCode = 103
	// DCGM_FR_PCIE_REPLAY_VIOLATION PCIe replay errors detected
	DCGM_FR_PCIE_REPLAY_VIOLATION HealthCheckErrorCode = 104
	// DCGM_FR_SBE_THRESHOLD_VIOLATION SBE threshold violated
	DCGM_FR_SBE_THRESHOLD_VIOLATION HealthCheckErrorCode = 105
	// DCGM_FR_DBE_THRESHOLD_VIOLATION DBE threshold violated
	DCGM_FR_DBE_THRESHOLD_VIOLATION HealthCheckErrorCode = 106
	// DCGM_FR_PCIE_REPLAY_THRESHOLD_VIOLATION PCIe replay count violated
	DCGM_FR_PCIE_REPLAY_THRESHOLD_VIOLATION HealthCheckErrorCode = 107
	// DCGM_FR_CUDA_FM_NOT_INITIALIZED The fabricmanager is not initialized
	DCGM_FR_CUDA_FM_NOT_INITIALIZED HealthCheckErrorCode = 108
	// DCGM_FR_SXID_ERROR NvSwitch fatal error detected
	DCGM_FR_SXID_ERROR HealthCheckErrorCode = 109
	// DCGM_FR_GFLOPS_THRESHOLD_VIOLATION GPU GFLOPs threshold violated
	DCGM_FR_GFLOPS_THRESHOLD_VIOLATION HealthCheckErrorCode = 110
	// DCGM_FR_NAN_VALUE NaN value detected on this GPU
	DCGM_FR_NAN_VALUE HealthCheckErrorCode = 111
	// DCGM_FR_FABRIC_MANAGER_TRAINING_ERROR Fabric Manager did not finish training
	DCGM_FR_FABRIC_MANAGER_TRAINING_ERROR HealthCheckErrorCode = 112
	// DCGM_FR_BROKEN_P2P_PCIE_MEMORY_DEVICE P2P copy test detected an error writing to this GPU over PCIE
	DCGM_FR_BROKEN_P2P_PCIE_MEMORY_DEVICE HealthCheckErrorCode = 113
	// DCGM_FR_BROKEN_P2P_PCIE_WRITER_DEVICE P2P copy test detected an error writing from this GPU over PCIE
	DCGM_FR_BROKEN_P2P_PCIE_WRITER_DEVICE HealthCheckErrorCode = 114
	// DCGM_FR_BROKEN_P2P_NVLINK_MEMORY_DEVICE P2P copy test detected an error writing to this GPU over NVLink
	DCGM_FR_BROKEN_P2P_NVLINK_MEMORY_DEVICE HealthCheckErrorCode = 115
	// DCGM_FR_BROKEN_P2P_NVLINK_WRITER_DEVICE P2P copy test detected an error writing from this GPU over NVLink
	DCGM_FR_BROKEN_P2P_NVLINK_WRITER_DEVICE HealthCheckErrorCode = 116
	// DCGM_FR_ERROR_SENTINEL MUST BE THE LAST ERROR CODE
	DCGM_FR_ERROR_SENTINEL HealthCheckErrorCode = 117
)
