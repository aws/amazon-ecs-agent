/*
 * Copyright (c) 2025, NVIDIA CORPORATION.  All rights reserved.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package dcgm

const (
	// DCGM_FI_UNKNOWN represents NULL field
	DCGM_FI_UNKNOWN Short = 0
	// DCGM_FI_DRIVER_VERSION represents Driver Version
	DCGM_FI_DRIVER_VERSION Short = 1
	// DCGM_FI_NVML_VERSION
	DCGM_FI_NVML_VERSION Short = 2
	// DCGM_FI_PROCESS_NAME represents Process Name
	DCGM_FI_PROCESS_NAME Short = 3
	// DCGM_FI_DEV_COUNT represents Number of Devices on the node
	DCGM_FI_DEV_COUNT Short = 4
	// DCGM_FI_CUDA_DRIVER_VERSION represents CUDA 11.1 = 11100
	DCGM_FI_CUDA_DRIVER_VERSION Short = 5
	// DCGM_FI_BIND_UNBIND_EVENT represents @note Recommended watch frequency: 1 second
	DCGM_FI_BIND_UNBIND_EVENT Short = 6
	// DCGM_FI_DEV_NAME represents Name of the GPU device
	DCGM_FI_DEV_NAME Short = 50
	// DCGM_FI_DEV_BRAND represents Device Brand
	DCGM_FI_DEV_BRAND Short = 51
	// DCGM_FI_DEV_NVML_INDEX represents NVML index of this GPU
	DCGM_FI_DEV_NVML_INDEX Short = 52
	// DCGM_FI_DEV_SERIAL represents Device Serial Number
	DCGM_FI_DEV_SERIAL Short = 53
	// DCGM_FI_DEV_UUID represents UUID corresponding to the device
	DCGM_FI_DEV_UUID Short = 54
	// DCGM_FI_DEV_MINOR_NUMBER represents Device node minor number /dev/nvidia#
	DCGM_FI_DEV_MINOR_NUMBER Short = 55
	// DCGM_FI_DEV_OEM_INFOROM_VER represents OEM inforom version
	DCGM_FI_DEV_OEM_INFOROM_VER Short = 56
	// DCGM_FI_DEV_PCI_BUSID represents PCI attributes for the device
	DCGM_FI_DEV_PCI_BUSID Short = 57
	// DCGM_FI_DEV_PCI_COMBINED_ID represents The combined 16-bit device id and 16-bit vendor id
	DCGM_FI_DEV_PCI_COMBINED_ID Short = 58
	// DCGM_FI_DEV_PCI_SUBSYS_ID represents The 32-bit Sub System Device ID
	DCGM_FI_DEV_PCI_SUBSYS_ID Short = 59
	// DCGM_FI_GPU_TOPOLOGY_PCI represents Topology of all GPUs on the system via PCI (static)
	DCGM_FI_GPU_TOPOLOGY_PCI Short = 60
	// DCGM_FI_GPU_TOPOLOGY_NVLINK represents Topology of all GPUs on the system via NVLINK (static)
	DCGM_FI_GPU_TOPOLOGY_NVLINK Short = 61
	// DCGM_FI_GPU_TOPOLOGY_AFFINITY represents Affinity of all GPUs on the system (static)
	DCGM_FI_GPU_TOPOLOGY_AFFINITY Short = 62
	// DCGM_FI_DEV_CUDA_COMPUTE_CAPABILITY represents the minor version is the lower 32 bits.
	DCGM_FI_DEV_CUDA_COMPUTE_CAPABILITY Short = 63
	// DCGM_FI_DEV_P2P_NVLINK_STATUS represents A bitmap of the P2P NVLINK status from this GPU to others on this host.
	DCGM_FI_DEV_P2P_NVLINK_STATUS Short = 64
	// DCGM_FI_DEV_COMPUTE_MODE represents Compute mode for the device
	DCGM_FI_DEV_COMPUTE_MODE Short = 65
	// DCGM_FI_DEV_PERSISTENCE_MODE represents Boolean: 0 is disabled, 1 is enabled
	DCGM_FI_DEV_PERSISTENCE_MODE Short = 66
	// DCGM_FI_DEV_MIG_MODE represents Boolean: 0 is disabled, 1 is enabled
	DCGM_FI_DEV_MIG_MODE Short = 67
	// DCGM_FI_DEV_CUDA_VISIBLE_DEVICES_STR represents be set to for this entity (including MIG)
	DCGM_FI_DEV_CUDA_VISIBLE_DEVICES_STR Short = 68
	// DCGM_FI_DEV_MIG_MAX_SLICES represents The maximum number of MIG slices supported by this GPU
	DCGM_FI_DEV_MIG_MAX_SLICES Short = 69
	// DCGM_FI_DEV_CPU_AFFINITY_0 represents Device CPU affinity. part 1/8 = cpus 0 - 63
	DCGM_FI_DEV_CPU_AFFINITY_0 Short = 70
	// DCGM_FI_DEV_CPU_AFFINITY_1 represents Device CPU affinity. part 1/8 = cpus 64 - 127
	DCGM_FI_DEV_CPU_AFFINITY_1 Short = 71
	// DCGM_FI_DEV_CPU_AFFINITY_2 represents Device CPU affinity. part 2/8 = cpus 128 - 191
	DCGM_FI_DEV_CPU_AFFINITY_2 Short = 72
	// DCGM_FI_DEV_CPU_AFFINITY_3 represents Device CPU affinity. part 3/8 = cpus 192 - 255
	DCGM_FI_DEV_CPU_AFFINITY_3 Short = 73
	// DCGM_FI_DEV_CC_MODE represents 1 = enabled
	DCGM_FI_DEV_CC_MODE Short = 74
	// DCGM_FI_DEV_MIG_ATTRIBUTES represents Attributes for the given MIG device handles
	DCGM_FI_DEV_MIG_ATTRIBUTES Short = 75
	// DCGM_FI_DEV_MIG_GI_INFO represents GPU instance profile information
	DCGM_FI_DEV_MIG_GI_INFO Short = 76
	// DCGM_FI_DEV_MIG_CI_INFO represents Compute instance profile information
	DCGM_FI_DEV_MIG_CI_INFO Short = 77
	// DCGM_FI_DEV_ECC_INFOROM_VER represents ECC inforom version
	DCGM_FI_DEV_ECC_INFOROM_VER Short = 80
	// DCGM_FI_DEV_POWER_INFOROM_VER represents Power management object inforom version
	DCGM_FI_DEV_POWER_INFOROM_VER Short = 81
	// DCGM_FI_DEV_INFOROM_IMAGE_VER represents Inforom image version
	DCGM_FI_DEV_INFOROM_IMAGE_VER Short = 82
	// DCGM_FI_DEV_INFOROM_CONFIG_CHECK represents Inforom configuration checksum
	DCGM_FI_DEV_INFOROM_CONFIG_CHECK Short = 83
	// DCGM_FI_DEV_INFOROM_CONFIG_VALID represents Reads the infoROM from the flash and verifies the checksums
	DCGM_FI_DEV_INFOROM_CONFIG_VALID Short = 84
	// DCGM_FI_DEV_VBIOS_VERSION represents VBIOS version of the device
	DCGM_FI_DEV_VBIOS_VERSION Short = 85
	// DCGM_FI_DEV_MEM_AFFINITY_0 represents Device Memory node affinity, 0-63
	DCGM_FI_DEV_MEM_AFFINITY_0 Short = 86
	// DCGM_FI_DEV_MEM_AFFINITY_1 represents Device Memory node affinity, 64-127
	DCGM_FI_DEV_MEM_AFFINITY_1 Short = 87
	// DCGM_FI_DEV_MEM_AFFINITY_2 represents Device Memory node affinity, 128-191
	DCGM_FI_DEV_MEM_AFFINITY_2 Short = 88
	// DCGM_FI_DEV_MEM_AFFINITY_3 represents Device Memory node affinity, 192-255
	DCGM_FI_DEV_MEM_AFFINITY_3 Short = 89
	// DCGM_FI_DEV_BAR1_TOTAL represents Total BAR1 of the GPU in MB
	DCGM_FI_DEV_BAR1_TOTAL Short = 90
	// DCGM_FI_SYNC_BOOST represents Deprecated - Sync boost settings on the node
	DCGM_FI_SYNC_BOOST Short = 91
	// DCGM_FI_DEV_BAR1_USED represents Used BAR1 of the GPU in MB
	DCGM_FI_DEV_BAR1_USED Short = 92
	// DCGM_FI_DEV_BAR1_FREE represents Free BAR1 of the GPU in MB
	DCGM_FI_DEV_BAR1_FREE Short = 93
	// DCGM_FI_DEV_GPM_SUPPORT represents * GPM support for the device
	DCGM_FI_DEV_GPM_SUPPORT Short = 94
	// DCGM_FI_DEV_SM_CLOCK represents SM clock for the device
	DCGM_FI_DEV_SM_CLOCK Short = 100
	// DCGM_FI_DEV_MEM_CLOCK represents Memory clock for the device
	DCGM_FI_DEV_MEM_CLOCK Short = 101
	// DCGM_FI_DEV_VIDEO_CLOCK represents Video encoder/decoder clock for the device
	DCGM_FI_DEV_VIDEO_CLOCK Short = 102
	// DCGM_FI_DEV_APP_SM_CLOCK represents SM Application clocks
	DCGM_FI_DEV_APP_SM_CLOCK Short = 110
	// DCGM_FI_DEV_APP_MEM_CLOCK represents Memory Application clocks
	DCGM_FI_DEV_APP_MEM_CLOCK Short = 111
	// DCGM_FI_DEV_CLOCKS_EVENT_REASONS represents Current clock event reasons (bitmask of DCGM_CLOCKS_EVENT_REASON_*)
	DCGM_FI_DEV_CLOCKS_EVENT_REASONS Short = 112
	// DCGM_FI_DEV_MAX_SM_CLOCK represents Maximum supported SM clock for the device
	DCGM_FI_DEV_MAX_SM_CLOCK Short = 113
	// DCGM_FI_DEV_MAX_MEM_CLOCK represents Maximum supported Memory clock for the device
	DCGM_FI_DEV_MAX_MEM_CLOCK Short = 114
	// DCGM_FI_DEV_MAX_VIDEO_CLOCK represents Maximum supported Video encoder/decoder clock for the device
	DCGM_FI_DEV_MAX_VIDEO_CLOCK Short = 115
	// DCGM_FI_DEV_AUTOBOOST represents Auto-boost for the device (1 = enabled. 0 = disabled)
	DCGM_FI_DEV_AUTOBOOST Short = 120
	// DCGM_FI_DEV_SUPPORTED_CLOCKS represents Supported clocks for the device
	DCGM_FI_DEV_SUPPORTED_CLOCKS Short = 130
	// DCGM_FI_DEV_MEMORY_TEMP represents Memory temperature for the device
	DCGM_FI_DEV_MEMORY_TEMP Short = 140
	// DCGM_FI_DEV_GPU_TEMP represents Current temperature readings for the device, in degrees C
	DCGM_FI_DEV_GPU_TEMP Short = 150
	// DCGM_FI_DEV_MEM_MAX_OP_TEMP represents Maximum operating temperature for the memory of this GPU. Above this temperature slowdown will occur.
	DCGM_FI_DEV_MEM_MAX_OP_TEMP Short = 151
	// DCGM_FI_DEV_GPU_MAX_OP_TEMP represents Maximum operating temperature for this GPU
	DCGM_FI_DEV_GPU_MAX_OP_TEMP Short = 152
	// DCGM_FI_DEV_GPU_TEMP_LIMIT represents Thermal margin temperature (distance to nearest slowdown threshold) for this GPU
	DCGM_FI_DEV_GPU_TEMP_LIMIT Short = 153
	// DCGM_FI_DEV_POWER_USAGE represents Power usage for the device in Watts
	DCGM_FI_DEV_POWER_USAGE Short = 155
	// DCGM_FI_DEV_TOTAL_ENERGY_CONSUMPTION represents Total energy consumption for the GPU in mJ since the driver was last reloaded
	DCGM_FI_DEV_TOTAL_ENERGY_CONSUMPTION Short = 156
	// DCGM_FI_DEV_POWER_USAGE_INSTANT represents Current instantaneous power usage of the device in Watts
	DCGM_FI_DEV_POWER_USAGE_INSTANT Short = 157
	// DCGM_FI_DEV_SLOWDOWN_TEMP represents Slowdown temperature for the device
	DCGM_FI_DEV_SLOWDOWN_TEMP Short = 158
	// DCGM_FI_DEV_SHUTDOWN_TEMP represents Shutdown temperature for the device
	DCGM_FI_DEV_SHUTDOWN_TEMP Short = 159
	// DCGM_FI_DEV_POWER_MGMT_LIMIT represents Current Power limit for the device
	DCGM_FI_DEV_POWER_MGMT_LIMIT Short = 160
	// DCGM_FI_DEV_POWER_MGMT_LIMIT_MIN represents Minimum power management limit for the device
	DCGM_FI_DEV_POWER_MGMT_LIMIT_MIN Short = 161
	// DCGM_FI_DEV_POWER_MGMT_LIMIT_MAX represents Maximum power management limit for the device
	DCGM_FI_DEV_POWER_MGMT_LIMIT_MAX Short = 162
	// DCGM_FI_DEV_POWER_MGMT_LIMIT_DEF represents Default power management limit for the device
	DCGM_FI_DEV_POWER_MGMT_LIMIT_DEF Short = 163
	// DCGM_FI_DEV_ENFORCED_POWER_LIMIT represents Effective power limit that the driver enforces after taking into account all limiters
	DCGM_FI_DEV_ENFORCED_POWER_LIMIT Short = 164
	// DCGM_FI_DEV_REQUESTED_POWER_PROFILE_MASK represents Requested workload power profile mask(Blackwell and newer)
	DCGM_FI_DEV_REQUESTED_POWER_PROFILE_MASK Short = 165
	// DCGM_FI_DEV_ENFORCED_POWER_PROFILE_MASK represents Enforced workload power profile mask(Blackwell and newer)
	DCGM_FI_DEV_ENFORCED_POWER_PROFILE_MASK Short = 166
	// DCGM_FI_DEV_VALID_POWER_PROFILE_MASK represents Requested workload power profile mask(Blackwell and newer)
	DCGM_FI_DEV_VALID_POWER_PROFILE_MASK Short = 167
	// DCGM_FI_DEV_FABRIC_MANAGER_STATUS represents The status of the fabric manager - a value from dcgmFabricManagerStatus_t.
	DCGM_FI_DEV_FABRIC_MANAGER_STATUS Short = 170
	// DCGM_FI_DEV_FABRIC_MANAGER_ERROR_CODE represents NOTE: this is not populated unless the fabric manager completed startup
	DCGM_FI_DEV_FABRIC_MANAGER_ERROR_CODE Short = 171
	// DCGM_FI_DEV_FABRIC_CLUSTER_UUID represents The uuid of the cluster to which this GPU belongs
	DCGM_FI_DEV_FABRIC_CLUSTER_UUID Short = 172
	// DCGM_FI_DEV_FABRIC_CLIQUE_ID represents The ID of the fabric clique to which this GPU belongs
	DCGM_FI_DEV_FABRIC_CLIQUE_ID Short = 173
	// DCGM_FI_DEV_FABRIC_HEALTH_MASK represents Use DCGM_GPU_FABRIC_HEALTH_GET macro to get the different health statuses.
	DCGM_FI_DEV_FABRIC_HEALTH_MASK Short = 174
	// DCGM_FI_DEV_FABRIC_HEALTH_SUMMARY represents - NVML_GPU_FABRIC_HEALTH_SUMMARY_LIMITED_CAPACITY (3)
	DCGM_FI_DEV_FABRIC_HEALTH_SUMMARY Short = 175
	// DCGM_FI_DEV_PSTATE represents Performance state (P-State) 0-15. 0=highest
	DCGM_FI_DEV_PSTATE Short = 190
	// DCGM_FI_DEV_FAN_SPEED represents Fan speed for the device in percent 0-100
	DCGM_FI_DEV_FAN_SPEED Short = 191
	// DCGM_FI_DEV_PCIE_TX_THROUGHPUT represents Deprecated: Use DCGM_FI_PROF_PCIE_TX_BYTES instead.
	DCGM_FI_DEV_PCIE_TX_THROUGHPUT Short = 200
	// DCGM_FI_DEV_PCIE_RX_THROUGHPUT represents Deprecated: Use DCGM_FI_PROF_PCIE_RX_BYTES instead.
	DCGM_FI_DEV_PCIE_RX_THROUGHPUT Short = 201
	// DCGM_FI_DEV_PCIE_REPLAY_COUNTER represents PCIe replay counter
	DCGM_FI_DEV_PCIE_REPLAY_COUNTER Short = 202
	// DCGM_FI_DEV_GPU_UTIL represents GPU Utilization
	DCGM_FI_DEV_GPU_UTIL Short = 203
	// DCGM_FI_DEV_MEM_COPY_UTIL represents Memory Utilization
	DCGM_FI_DEV_MEM_COPY_UTIL Short = 204
	// DCGM_FI_DEV_ACCOUNTING_DATA represents running "nvidia-smi -am 1" as root on the same node the host engine is running on.
	DCGM_FI_DEV_ACCOUNTING_DATA Short = 205
	// DCGM_FI_DEV_ENC_UTIL represents Encoder Utilization
	DCGM_FI_DEV_ENC_UTIL Short = 206
	// DCGM_FI_DEV_DEC_UTIL represents Decoder Utilization
	DCGM_FI_DEV_DEC_UTIL Short = 207
	// DCGM_FI_DEV_XID_ERRORS represents XID errors. The value is the specific XID error
	DCGM_FI_DEV_XID_ERRORS Short = 230
	// DCGM_FI_DEV_PCIE_MAX_LINK_GEN represents PCIe Max Link Generation
	DCGM_FI_DEV_PCIE_MAX_LINK_GEN Short = 235
	// DCGM_FI_DEV_PCIE_MAX_LINK_WIDTH represents PCIe Max Link Width
	DCGM_FI_DEV_PCIE_MAX_LINK_WIDTH Short = 236
	// DCGM_FI_DEV_PCIE_LINK_GEN represents PCIe Current Link Generation
	DCGM_FI_DEV_PCIE_LINK_GEN Short = 237
	// DCGM_FI_DEV_PCIE_LINK_WIDTH represents PCIe Current Link Width
	DCGM_FI_DEV_PCIE_LINK_WIDTH Short = 238
	// DCGM_FI_DEV_POWER_VIOLATION represents Power Violation time in ns
	DCGM_FI_DEV_POWER_VIOLATION Short = 240
	// DCGM_FI_DEV_THERMAL_VIOLATION represents Thermal Violation time in ns
	DCGM_FI_DEV_THERMAL_VIOLATION Short = 241
	// DCGM_FI_DEV_SYNC_BOOST_VIOLATION represents Sync Boost Violation time in ns
	DCGM_FI_DEV_SYNC_BOOST_VIOLATION Short = 242
	// DCGM_FI_DEV_BOARD_LIMIT_VIOLATION represents Board violation limit.
	DCGM_FI_DEV_BOARD_LIMIT_VIOLATION Short = 243
	// DCGM_FI_DEV_LOW_UTIL_VIOLATION represents Low utilisation violation limit.
	DCGM_FI_DEV_LOW_UTIL_VIOLATION Short = 244
	// DCGM_FI_DEV_RELIABILITY_VIOLATION represents Reliability violation limit.
	DCGM_FI_DEV_RELIABILITY_VIOLATION Short = 245
	// DCGM_FI_DEV_TOTAL_APP_CLOCKS_VIOLATION represents App clock violation limit.
	DCGM_FI_DEV_TOTAL_APP_CLOCKS_VIOLATION Short = 246
	// DCGM_FI_DEV_TOTAL_BASE_CLOCKS_VIOLATION represents Base clock violation limit.
	DCGM_FI_DEV_TOTAL_BASE_CLOCKS_VIOLATION Short = 247
	// DCGM_FI_DEV_FB_TOTAL represents Total Frame Buffer of the GPU in MB
	DCGM_FI_DEV_FB_TOTAL Short = 250
	// DCGM_FI_DEV_FB_FREE represents Free Frame Buffer in MB
	DCGM_FI_DEV_FB_FREE Short = 251
	// DCGM_FI_DEV_FB_USED represents Used Frame Buffer in MB
	DCGM_FI_DEV_FB_USED Short = 252
	// DCGM_FI_DEV_FB_RESERVED represents Reserved Frame Buffer in MB
	DCGM_FI_DEV_FB_RESERVED Short = 253
	// DCGM_FI_DEV_FB_USED_PERCENT represents Percentage used of Frame Buffer: 'Used/(Total - Reserved)'. Range 0.0-1.0
	DCGM_FI_DEV_FB_USED_PERCENT Short = 254
	// DCGM_FI_DEV_C2C_LINK_COUNT represents C2C Link Count
	DCGM_FI_DEV_C2C_LINK_COUNT Short = 285
	// DCGM_FI_DEV_C2C_LINK_STATUS represents The value of 1 the link is ACTIVE.
	DCGM_FI_DEV_C2C_LINK_STATUS Short = 286
	// DCGM_FI_DEV_C2C_MAX_BANDWIDTH represents The value indicates the link speed in MB/s.
	DCGM_FI_DEV_C2C_MAX_BANDWIDTH Short = 287
	// DCGM_FI_DEV_ECC_CURRENT represents Current ECC mode for the device
	DCGM_FI_DEV_ECC_CURRENT Short = 300
	// DCGM_FI_DEV_ECC_PENDING represents Pending ECC mode for the device
	DCGM_FI_DEV_ECC_PENDING Short = 301
	// DCGM_FI_DEV_ECC_SBE_VOL_TOTAL represents Total single bit volatile ECC errors
	DCGM_FI_DEV_ECC_SBE_VOL_TOTAL Short = 310
	// DCGM_FI_DEV_ECC_DBE_VOL_TOTAL represents Total double bit volatile ECC errors
	DCGM_FI_DEV_ECC_DBE_VOL_TOTAL Short = 311
	// DCGM_FI_DEV_ECC_SBE_AGG_TOTAL represents Note: monotonically increasing
	DCGM_FI_DEV_ECC_SBE_AGG_TOTAL Short = 312
	// DCGM_FI_DEV_ECC_DBE_AGG_TOTAL represents Note: monotonically increasing
	DCGM_FI_DEV_ECC_DBE_AGG_TOTAL Short = 313
	// DCGM_FI_DEV_ECC_SBE_VOL_L1 represents L1 cache single bit volatile ECC errors
	DCGM_FI_DEV_ECC_SBE_VOL_L1 Short = 314
	// DCGM_FI_DEV_ECC_DBE_VOL_L1 represents L1 cache double bit volatile ECC errors
	DCGM_FI_DEV_ECC_DBE_VOL_L1 Short = 315
	// DCGM_FI_DEV_ECC_SBE_VOL_L2 represents L2 cache single bit volatile ECC errors
	DCGM_FI_DEV_ECC_SBE_VOL_L2 Short = 316
	// DCGM_FI_DEV_ECC_DBE_VOL_L2 represents L2 cache double bit volatile ECC errors
	DCGM_FI_DEV_ECC_DBE_VOL_L2 Short = 317
	// DCGM_FI_DEV_ECC_SBE_VOL_DEV represents Device memory single bit volatile ECC errors
	DCGM_FI_DEV_ECC_SBE_VOL_DEV Short = 318
	// DCGM_FI_DEV_ECC_DBE_VOL_DEV represents Device memory double bit volatile ECC errors
	DCGM_FI_DEV_ECC_DBE_VOL_DEV Short = 319
	// DCGM_FI_DEV_ECC_SBE_VOL_REG represents Register file single bit volatile ECC errors
	DCGM_FI_DEV_ECC_SBE_VOL_REG Short = 320
	// DCGM_FI_DEV_ECC_DBE_VOL_REG represents Register file double bit volatile ECC errors
	DCGM_FI_DEV_ECC_DBE_VOL_REG Short = 321
	// DCGM_FI_DEV_ECC_SBE_VOL_TEX represents Texture memory single bit volatile ECC errors
	DCGM_FI_DEV_ECC_SBE_VOL_TEX Short = 322
	// DCGM_FI_DEV_ECC_DBE_VOL_TEX represents Texture memory double bit volatile ECC errors
	DCGM_FI_DEV_ECC_DBE_VOL_TEX Short = 323
	// DCGM_FI_DEV_ECC_SBE_AGG_L1 represents Note: monotonically increasing
	DCGM_FI_DEV_ECC_SBE_AGG_L1 Short = 324
	// DCGM_FI_DEV_ECC_DBE_AGG_L1 represents Note: monotonically increasing
	DCGM_FI_DEV_ECC_DBE_AGG_L1 Short = 325
	// DCGM_FI_DEV_ECC_SBE_AGG_L2 represents Note: monotonically increasing
	DCGM_FI_DEV_ECC_SBE_AGG_L2 Short = 326
	// DCGM_FI_DEV_ECC_DBE_AGG_L2 represents Note: monotonically increasing
	DCGM_FI_DEV_ECC_DBE_AGG_L2 Short = 327
	// DCGM_FI_DEV_ECC_SBE_AGG_DEV represents Note: monotonically increasing
	DCGM_FI_DEV_ECC_SBE_AGG_DEV Short = 328
	// DCGM_FI_DEV_ECC_DBE_AGG_DEV represents Note: monotonically increasing
	DCGM_FI_DEV_ECC_DBE_AGG_DEV Short = 329
	// DCGM_FI_DEV_ECC_SBE_AGG_REG represents Note: monotonically increasing
	DCGM_FI_DEV_ECC_SBE_AGG_REG Short = 330
	// DCGM_FI_DEV_ECC_DBE_AGG_REG represents Note: monotonically increasing
	DCGM_FI_DEV_ECC_DBE_AGG_REG Short = 331
	// DCGM_FI_DEV_ECC_SBE_AGG_TEX represents Note: monotonically increasing
	DCGM_FI_DEV_ECC_SBE_AGG_TEX Short = 332
	// DCGM_FI_DEV_ECC_DBE_AGG_TEX represents Note: monotonically increasing
	DCGM_FI_DEV_ECC_DBE_AGG_TEX Short = 333
	// DCGM_FI_DEV_ECC_SBE_VOL_SHM represents Texture SHM single bit volatile ECC errors
	DCGM_FI_DEV_ECC_SBE_VOL_SHM Short = 334
	// DCGM_FI_DEV_ECC_DBE_VOL_SHM represents Texture SHM double bit volatile ECC errors
	DCGM_FI_DEV_ECC_DBE_VOL_SHM Short = 335
	// DCGM_FI_DEV_ECC_SBE_VOL_CBU represents CBU single bit ECC volatile errors
	DCGM_FI_DEV_ECC_SBE_VOL_CBU Short = 336
	// DCGM_FI_DEV_ECC_DBE_VOL_CBU represents CBU double bit ECC volatile errors
	DCGM_FI_DEV_ECC_DBE_VOL_CBU Short = 337
	// DCGM_FI_DEV_ECC_SBE_AGG_SHM represents Texture SHM single bit aggregate ECC errors
	DCGM_FI_DEV_ECC_SBE_AGG_SHM Short = 338
	// DCGM_FI_DEV_ECC_DBE_AGG_SHM represents Texture SHM double bit aggregate ECC errors
	DCGM_FI_DEV_ECC_DBE_AGG_SHM Short = 339
	// DCGM_FI_DEV_ECC_SBE_AGG_CBU represents CBU single bit ECC aggregate errors
	DCGM_FI_DEV_ECC_SBE_AGG_CBU Short = 340
	// DCGM_FI_DEV_ECC_DBE_AGG_CBU represents CBU double bit ECC aggregate errors
	DCGM_FI_DEV_ECC_DBE_AGG_CBU Short = 341
	// DCGM_FI_DEV_ECC_SBE_VOL_SRM represents SRAM single bit ECC volatile errors
	DCGM_FI_DEV_ECC_SBE_VOL_SRM Short = 342
	// DCGM_FI_DEV_ECC_DBE_VOL_SRM represents SRAM double bit ECC volatile errors
	DCGM_FI_DEV_ECC_DBE_VOL_SRM Short = 343
	// DCGM_FI_DEV_ECC_SBE_AGG_SRM represents SRAM single bit ECC aggregate errors
	DCGM_FI_DEV_ECC_SBE_AGG_SRM Short = 344
	// DCGM_FI_DEV_ECC_DBE_AGG_SRM represents SRAM double bit ECC aggregate errors
	DCGM_FI_DEV_ECC_DBE_AGG_SRM Short = 345
	// DCGM_FI_DEV_THRESHOLD_SRM represents SRAM Threashhold Exceeded boolean (1=true)
	DCGM_FI_DEV_THRESHOLD_SRM Short = 346
	// DCGM_FI_DEV_DIAG_MEMORY_RESULT represents Refers to a `int64_t` storing a value drawn from `dcgmError_t` enumeration
	DCGM_FI_DEV_DIAG_MEMORY_RESULT Short = 350
	// DCGM_FI_DEV_DIAG_DIAGNOSTIC_RESULT represents Refers to a `int64_t` storing a value drawn from `dcgmError_t` enumeration
	DCGM_FI_DEV_DIAG_DIAGNOSTIC_RESULT Short = 351
	// DCGM_FI_DEV_DIAG_PCIE_RESULT represents Refers to a `int64_t` storing a value drawn from `dcgmError_t` enumeration
	DCGM_FI_DEV_DIAG_PCIE_RESULT Short = 352
	// DCGM_FI_DEV_DIAG_TARGETED_STRESS_RESULT represents Refers to a `int64_t` storing a value drawn from `dcgmError_t` enumeration
	DCGM_FI_DEV_DIAG_TARGETED_STRESS_RESULT Short = 353
	// DCGM_FI_DEV_DIAG_TARGETED_POWER_RESULT represents Refers to a `int64_t` storing a value drawn from `dcgmError_t` enumeration
	DCGM_FI_DEV_DIAG_TARGETED_POWER_RESULT Short = 354
	// DCGM_FI_DEV_DIAG_MEMORY_BANDWIDTH_RESULT represents Refers to a `int64_t` storing a value drawn from `dcgmError_t` enumeration
	DCGM_FI_DEV_DIAG_MEMORY_BANDWIDTH_RESULT Short = 355
	// DCGM_FI_DEV_DIAG_MEMTEST_RESULT represents Refers to a `int64_t` storing a value drawn from `dcgmError_t` enumeration
	DCGM_FI_DEV_DIAG_MEMTEST_RESULT Short = 356
	// DCGM_FI_DEV_DIAG_PULSE_TEST_RESULT represents Refers to a `int64_t` storing a value drawn from `dcgmError_t` enumeration
	DCGM_FI_DEV_DIAG_PULSE_TEST_RESULT Short = 357
	// DCGM_FI_DEV_DIAG_EUD_RESULT represents Refers to a `int64_t` storing a value drawn from `dcgmError_t` enumeration
	DCGM_FI_DEV_DIAG_EUD_RESULT Short = 358
	// DCGM_FI_DEV_DIAG_CPU_EUD_RESULT represents Refers to a `int64_t` storing a value drawn from `dcgmError_t` enumeration
	DCGM_FI_DEV_DIAG_CPU_EUD_RESULT Short = 359
	// DCGM_FI_DEV_DIAG_SOFTWARE_RESULT represents Refers to a `int64_t` storing a value drawn from `dcgmError_t` enumeration
	DCGM_FI_DEV_DIAG_SOFTWARE_RESULT Short = 360
	// DCGM_FI_DEV_DIAG_NVBANDWIDTH_RESULT represents Refers to a `int64_t` storing a value drawn from `dcgmError_t` enumeration
	DCGM_FI_DEV_DIAG_NVBANDWIDTH_RESULT Short = 361
	// DCGM_FI_DEV_DIAG_STATUS represents Refers to a binary blob of a `dcgmDiagStatus_t` struct
	DCGM_FI_DEV_DIAG_STATUS Short = 362
	// DCGM_FI_DEV_DIAG_NCCL_TESTS_RESULT represents Refers to a `int64_t` storing a value drawn from `dcgmError_t` enumeration
	DCGM_FI_DEV_DIAG_NCCL_TESTS_RESULT Short = 363
	// DCGM_FI_DEV_BANKS_REMAP_ROWS_AVAIL_MAX represents Historical max available spare memory rows per memory bank
	DCGM_FI_DEV_BANKS_REMAP_ROWS_AVAIL_MAX Short = 385
	// DCGM_FI_DEV_BANKS_REMAP_ROWS_AVAIL_HIGH represents Historical high mark of available spare memory rows per memory bank
	DCGM_FI_DEV_BANKS_REMAP_ROWS_AVAIL_HIGH Short = 386
	// DCGM_FI_DEV_BANKS_REMAP_ROWS_AVAIL_PARTIAL represents Historical mark of partial available spare memory rows per memory bank
	DCGM_FI_DEV_BANKS_REMAP_ROWS_AVAIL_PARTIAL Short = 387
	// DCGM_FI_DEV_BANKS_REMAP_ROWS_AVAIL_LOW represents Historical low mark of available spare memory rows per memory bank
	DCGM_FI_DEV_BANKS_REMAP_ROWS_AVAIL_LOW Short = 388
	// DCGM_FI_DEV_BANKS_REMAP_ROWS_AVAIL_NONE represents Historical marker of memory banks with no available spare memory rows
	DCGM_FI_DEV_BANKS_REMAP_ROWS_AVAIL_NONE Short = 389
	// DCGM_FI_DEV_RETIRED_SBE represents Note: monotonically increasing
	DCGM_FI_DEV_RETIRED_SBE Short = 390
	// DCGM_FI_DEV_RETIRED_DBE represents Note: monotonically increasing
	DCGM_FI_DEV_RETIRED_DBE Short = 391
	// DCGM_FI_DEV_RETIRED_PENDING represents Number of pages pending retirement
	DCGM_FI_DEV_RETIRED_PENDING Short = 392
	// DCGM_FI_DEV_UNCORRECTABLE_REMAPPED_ROWS represents Number of remapped rows for uncorrectable errors
	DCGM_FI_DEV_UNCORRECTABLE_REMAPPED_ROWS Short = 393
	// DCGM_FI_DEV_CORRECTABLE_REMAPPED_ROWS represents Number of remapped rows for correctable errors
	DCGM_FI_DEV_CORRECTABLE_REMAPPED_ROWS Short = 394
	// DCGM_FI_DEV_ROW_REMAP_FAILURE represents Whether remapping of rows has failed
	DCGM_FI_DEV_ROW_REMAP_FAILURE Short = 395
	// DCGM_FI_DEV_ROW_REMAP_PENDING represents Whether remapping of rows is pending
	DCGM_FI_DEV_ROW_REMAP_PENDING Short = 396
	// DCGM_FI_DEV_NVLINK_CRC_FLIT_ERROR_COUNT_L0 represents NV Link flow control CRC  Error Counter for Lane 0
	DCGM_FI_DEV_NVLINK_CRC_FLIT_ERROR_COUNT_L0 Short = 400
	// DCGM_FI_DEV_NVLINK_CRC_FLIT_ERROR_COUNT_L1 represents NV Link flow control CRC  Error Counter for Lane 1
	DCGM_FI_DEV_NVLINK_CRC_FLIT_ERROR_COUNT_L1 Short = 401
	// DCGM_FI_DEV_NVLINK_CRC_FLIT_ERROR_COUNT_L2 represents NV Link flow control CRC  Error Counter for Lane 2
	DCGM_FI_DEV_NVLINK_CRC_FLIT_ERROR_COUNT_L2 Short = 402
	// DCGM_FI_DEV_NVLINK_CRC_FLIT_ERROR_COUNT_L3 represents NV Link flow control CRC  Error Counter for Lane 3
	DCGM_FI_DEV_NVLINK_CRC_FLIT_ERROR_COUNT_L3 Short = 403
	// DCGM_FI_DEV_NVLINK_CRC_FLIT_ERROR_COUNT_L4 represents NV Link flow control CRC  Error Counter for Lane 4
	DCGM_FI_DEV_NVLINK_CRC_FLIT_ERROR_COUNT_L4 Short = 404
	// DCGM_FI_DEV_NVLINK_CRC_FLIT_ERROR_COUNT_L5 represents NV Link flow control CRC  Error Counter for Lane 5
	DCGM_FI_DEV_NVLINK_CRC_FLIT_ERROR_COUNT_L5 Short = 405
	// DCGM_FI_DEV_NVLINK_CRC_FLIT_ERROR_COUNT_L12
	DCGM_FI_DEV_NVLINK_CRC_FLIT_ERROR_COUNT_L12 Short = 406
	// DCGM_FI_DEV_NVLINK_CRC_FLIT_ERROR_COUNT_L13
	DCGM_FI_DEV_NVLINK_CRC_FLIT_ERROR_COUNT_L13 Short = 407
	// DCGM_FI_DEV_NVLINK_CRC_FLIT_ERROR_COUNT_L14
	DCGM_FI_DEV_NVLINK_CRC_FLIT_ERROR_COUNT_L14 Short = 408
	// DCGM_FI_DEV_NVLINK_CRC_FLIT_ERROR_COUNT_TOTAL represents NV Link flow control CRC  Error Counter total for all Lanes
	DCGM_FI_DEV_NVLINK_CRC_FLIT_ERROR_COUNT_TOTAL Short = 409
	// DCGM_FI_DEV_NVLINK_CRC_DATA_ERROR_COUNT_L0 represents NV Link data CRC Error Counter for Lane 0
	DCGM_FI_DEV_NVLINK_CRC_DATA_ERROR_COUNT_L0 Short = 410
	// DCGM_FI_DEV_NVLINK_CRC_DATA_ERROR_COUNT_L1 represents NV Link data CRC Error Counter for Lane 1
	DCGM_FI_DEV_NVLINK_CRC_DATA_ERROR_COUNT_L1 Short = 411
	// DCGM_FI_DEV_NVLINK_CRC_DATA_ERROR_COUNT_L2 represents NV Link data CRC Error Counter for Lane 2
	DCGM_FI_DEV_NVLINK_CRC_DATA_ERROR_COUNT_L2 Short = 412
	// DCGM_FI_DEV_NVLINK_CRC_DATA_ERROR_COUNT_L3 represents NV Link data CRC Error Counter for Lane 3
	DCGM_FI_DEV_NVLINK_CRC_DATA_ERROR_COUNT_L3 Short = 413
	// DCGM_FI_DEV_NVLINK_CRC_DATA_ERROR_COUNT_L4 represents NV Link data CRC Error Counter for Lane 4
	DCGM_FI_DEV_NVLINK_CRC_DATA_ERROR_COUNT_L4 Short = 414
	// DCGM_FI_DEV_NVLINK_CRC_DATA_ERROR_COUNT_L5 represents NV Link data CRC Error Counter for Lane 5
	DCGM_FI_DEV_NVLINK_CRC_DATA_ERROR_COUNT_L5 Short = 415
	// DCGM_FI_DEV_NVLINK_CRC_DATA_ERROR_COUNT_L12
	DCGM_FI_DEV_NVLINK_CRC_DATA_ERROR_COUNT_L12 Short = 416
	// DCGM_FI_DEV_NVLINK_CRC_DATA_ERROR_COUNT_L13
	DCGM_FI_DEV_NVLINK_CRC_DATA_ERROR_COUNT_L13 Short = 417
	// DCGM_FI_DEV_NVLINK_CRC_DATA_ERROR_COUNT_L14
	DCGM_FI_DEV_NVLINK_CRC_DATA_ERROR_COUNT_L14 Short = 418
	// DCGM_FI_DEV_NVLINK_CRC_DATA_ERROR_COUNT_TOTAL represents NV Link data CRC Error Counter total for all Lanes
	DCGM_FI_DEV_NVLINK_CRC_DATA_ERROR_COUNT_TOTAL Short = 419
	// DCGM_FI_DEV_NVLINK_REPLAY_ERROR_COUNT_L0 represents NV Link Replay Error Counter for Lane 0
	DCGM_FI_DEV_NVLINK_REPLAY_ERROR_COUNT_L0 Short = 420
	// DCGM_FI_DEV_NVLINK_REPLAY_ERROR_COUNT_L1 represents NV Link Replay Error Counter for Lane 1
	DCGM_FI_DEV_NVLINK_REPLAY_ERROR_COUNT_L1 Short = 421
	// DCGM_FI_DEV_NVLINK_REPLAY_ERROR_COUNT_L2 represents NV Link Replay Error Counter for Lane 2
	DCGM_FI_DEV_NVLINK_REPLAY_ERROR_COUNT_L2 Short = 422
	// DCGM_FI_DEV_NVLINK_REPLAY_ERROR_COUNT_L3 represents NV Link Replay Error Counter for Lane 3
	DCGM_FI_DEV_NVLINK_REPLAY_ERROR_COUNT_L3 Short = 423
	// DCGM_FI_DEV_NVLINK_REPLAY_ERROR_COUNT_L4 represents NV Link Replay Error Counter for Lane 4
	DCGM_FI_DEV_NVLINK_REPLAY_ERROR_COUNT_L4 Short = 424
	// DCGM_FI_DEV_NVLINK_REPLAY_ERROR_COUNT_L5 represents NV Link Replay Error Counter for Lane 5
	DCGM_FI_DEV_NVLINK_REPLAY_ERROR_COUNT_L5 Short = 425
	// DCGM_FI_DEV_NVLINK_REPLAY_ERROR_COUNT_L12
	DCGM_FI_DEV_NVLINK_REPLAY_ERROR_COUNT_L12 Short = 426
	// DCGM_FI_DEV_NVLINK_REPLAY_ERROR_COUNT_L13
	DCGM_FI_DEV_NVLINK_REPLAY_ERROR_COUNT_L13 Short = 427
	// DCGM_FI_DEV_NVLINK_REPLAY_ERROR_COUNT_L14
	DCGM_FI_DEV_NVLINK_REPLAY_ERROR_COUNT_L14 Short = 428
	// DCGM_FI_DEV_NVLINK_REPLAY_ERROR_COUNT_TOTAL represents NV Link Replay Error Counter total for all Lanes
	DCGM_FI_DEV_NVLINK_REPLAY_ERROR_COUNT_TOTAL Short = 429
	// DCGM_FI_DEV_NVLINK_RECOVERY_ERROR_COUNT_L0 represents NV Link Recovery Error Counter for Lane 0
	DCGM_FI_DEV_NVLINK_RECOVERY_ERROR_COUNT_L0 Short = 430
	// DCGM_FI_DEV_NVLINK_RECOVERY_ERROR_COUNT_L1 represents NV Link Recovery Error Counter for Lane 1
	DCGM_FI_DEV_NVLINK_RECOVERY_ERROR_COUNT_L1 Short = 431
	// DCGM_FI_DEV_NVLINK_RECOVERY_ERROR_COUNT_L2 represents NV Link Recovery Error Counter for Lane 2
	DCGM_FI_DEV_NVLINK_RECOVERY_ERROR_COUNT_L2 Short = 432
	// DCGM_FI_DEV_NVLINK_RECOVERY_ERROR_COUNT_L3 represents NV Link Recovery Error Counter for Lane 3
	DCGM_FI_DEV_NVLINK_RECOVERY_ERROR_COUNT_L3 Short = 433
	// DCGM_FI_DEV_NVLINK_RECOVERY_ERROR_COUNT_L4 represents NV Link Recovery Error Counter for Lane 4
	DCGM_FI_DEV_NVLINK_RECOVERY_ERROR_COUNT_L4 Short = 434
	// DCGM_FI_DEV_NVLINK_RECOVERY_ERROR_COUNT_L5 represents NV Link Recovery Error Counter for Lane 5
	DCGM_FI_DEV_NVLINK_RECOVERY_ERROR_COUNT_L5 Short = 435
	// DCGM_FI_DEV_NVLINK_RECOVERY_ERROR_COUNT_L12
	DCGM_FI_DEV_NVLINK_RECOVERY_ERROR_COUNT_L12 Short = 436
	// DCGM_FI_DEV_NVLINK_RECOVERY_ERROR_COUNT_L13
	DCGM_FI_DEV_NVLINK_RECOVERY_ERROR_COUNT_L13 Short = 437
	// DCGM_FI_DEV_NVLINK_RECOVERY_ERROR_COUNT_L14
	DCGM_FI_DEV_NVLINK_RECOVERY_ERROR_COUNT_L14 Short = 438
	// DCGM_FI_DEV_NVLINK_RECOVERY_ERROR_COUNT_TOTAL represents NV Link Recovery Error Counter total for all Lanes
	DCGM_FI_DEV_NVLINK_RECOVERY_ERROR_COUNT_TOTAL Short = 439
	// DCGM_FI_DEV_NVLINK_THROUGHPUT_L0 represents NV Link Throughput for Lane 0
	DCGM_FI_DEV_NVLINK_THROUGHPUT_L0 Short = 440
	// DCGM_FI_DEV_NVLINK_THROUGHPUT_L1 represents NV Link Throughput for Lane 1
	DCGM_FI_DEV_NVLINK_THROUGHPUT_L1 Short = 441
	// DCGM_FI_DEV_NVLINK_THROUGHPUT_L2 represents NV Link Throughput for Lane 2
	DCGM_FI_DEV_NVLINK_THROUGHPUT_L2 Short = 442
	// DCGM_FI_DEV_NVLINK_THROUGHPUT_L3 represents NV Link Throughput for Lane 3
	DCGM_FI_DEV_NVLINK_THROUGHPUT_L3 Short = 443
	// DCGM_FI_DEV_NVLINK_THROUGHPUT_L4 represents NV Link Throughput for Lane 4
	DCGM_FI_DEV_NVLINK_THROUGHPUT_L4 Short = 444
	// DCGM_FI_DEV_NVLINK_THROUGHPUT_L5 represents NV Link Throughput for Lane 5
	DCGM_FI_DEV_NVLINK_THROUGHPUT_L5 Short = 445
	// DCGM_FI_DEV_NVLINK_THROUGHPUT_L12
	DCGM_FI_DEV_NVLINK_THROUGHPUT_L12 Short = 446
	// DCGM_FI_DEV_NVLINK_THROUGHPUT_L13
	DCGM_FI_DEV_NVLINK_THROUGHPUT_L13 Short = 447
	// DCGM_FI_DEV_NVLINK_THROUGHPUT_L14
	DCGM_FI_DEV_NVLINK_THROUGHPUT_L14 Short = 448
	// DCGM_FI_DEV_NVLINK_THROUGHPUT_TOTAL represents NV Link Throughput total for all Lanes
	DCGM_FI_DEV_NVLINK_THROUGHPUT_TOTAL Short = 449
	// DCGM_FI_DEV_GPU_NVLINK_ERRORS represents GPU NVLink error information
	DCGM_FI_DEV_GPU_NVLINK_ERRORS Short = 450
	// DCGM_FI_DEV_NVLINK_CRC_FLIT_ERROR_COUNT_L6 represents NV Link flow control CRC  Error Counter for Lane 6
	DCGM_FI_DEV_NVLINK_CRC_FLIT_ERROR_COUNT_L6 Short = 451
	// DCGM_FI_DEV_NVLINK_CRC_FLIT_ERROR_COUNT_L7 represents NV Link flow control CRC  Error Counter for Lane 7
	DCGM_FI_DEV_NVLINK_CRC_FLIT_ERROR_COUNT_L7 Short = 452
	// DCGM_FI_DEV_NVLINK_CRC_FLIT_ERROR_COUNT_L8 represents NV Link flow control CRC  Error Counter for Lane 8
	DCGM_FI_DEV_NVLINK_CRC_FLIT_ERROR_COUNT_L8 Short = 453
	// DCGM_FI_DEV_NVLINK_CRC_FLIT_ERROR_COUNT_L9 represents NV Link flow control CRC  Error Counter for Lane 9
	DCGM_FI_DEV_NVLINK_CRC_FLIT_ERROR_COUNT_L9 Short = 454
	// DCGM_FI_DEV_NVLINK_CRC_FLIT_ERROR_COUNT_L10 represents NV Link flow control CRC  Error Counter for Lane 10
	DCGM_FI_DEV_NVLINK_CRC_FLIT_ERROR_COUNT_L10 Short = 455
	// DCGM_FI_DEV_NVLINK_CRC_FLIT_ERROR_COUNT_L11 represents NV Link flow control CRC  Error Counter for Lane 11
	DCGM_FI_DEV_NVLINK_CRC_FLIT_ERROR_COUNT_L11 Short = 456
	// DCGM_FI_DEV_NVLINK_CRC_DATA_ERROR_COUNT_L6 represents NV Link data CRC Error Counter for Lane 6
	DCGM_FI_DEV_NVLINK_CRC_DATA_ERROR_COUNT_L6 Short = 457
	// DCGM_FI_DEV_NVLINK_CRC_DATA_ERROR_COUNT_L7 represents NV Link data CRC Error Counter for Lane 7
	DCGM_FI_DEV_NVLINK_CRC_DATA_ERROR_COUNT_L7 Short = 458
	// DCGM_FI_DEV_NVLINK_CRC_DATA_ERROR_COUNT_L8 represents NV Link data CRC Error Counter for Lane 8
	DCGM_FI_DEV_NVLINK_CRC_DATA_ERROR_COUNT_L8 Short = 459
	// DCGM_FI_DEV_NVLINK_CRC_DATA_ERROR_COUNT_L9 represents NV Link data CRC Error Counter for Lane 9
	DCGM_FI_DEV_NVLINK_CRC_DATA_ERROR_COUNT_L9 Short = 460
	// DCGM_FI_DEV_NVLINK_CRC_DATA_ERROR_COUNT_L10 represents NV Link data CRC Error Counter for Lane 10
	DCGM_FI_DEV_NVLINK_CRC_DATA_ERROR_COUNT_L10 Short = 461
	// DCGM_FI_DEV_NVLINK_CRC_DATA_ERROR_COUNT_L11 represents NV Link data CRC Error Counter for Lane 11
	DCGM_FI_DEV_NVLINK_CRC_DATA_ERROR_COUNT_L11 Short = 462
	// DCGM_FI_DEV_NVLINK_REPLAY_ERROR_COUNT_L6 represents NV Link Replay Error Counter for Lane 6
	DCGM_FI_DEV_NVLINK_REPLAY_ERROR_COUNT_L6 Short = 463
	// DCGM_FI_DEV_NVLINK_REPLAY_ERROR_COUNT_L7 represents NV Link Replay Error Counter for Lane 7
	DCGM_FI_DEV_NVLINK_REPLAY_ERROR_COUNT_L7 Short = 464
	// DCGM_FI_DEV_NVLINK_REPLAY_ERROR_COUNT_L8 represents NV Link Replay Error Counter for Lane 8
	DCGM_FI_DEV_NVLINK_REPLAY_ERROR_COUNT_L8 Short = 465
	// DCGM_FI_DEV_NVLINK_REPLAY_ERROR_COUNT_L9 represents NV Link Replay Error Counter for Lane 9
	DCGM_FI_DEV_NVLINK_REPLAY_ERROR_COUNT_L9 Short = 466
	// DCGM_FI_DEV_NVLINK_REPLAY_ERROR_COUNT_L10 represents NV Link Replay Error Counter for Lane 10
	DCGM_FI_DEV_NVLINK_REPLAY_ERROR_COUNT_L10 Short = 467
	// DCGM_FI_DEV_NVLINK_REPLAY_ERROR_COUNT_L11 represents NV Link Replay Error Counter for Lane 11
	DCGM_FI_DEV_NVLINK_REPLAY_ERROR_COUNT_L11 Short = 468
	// DCGM_FI_DEV_NVLINK_RECOVERY_ERROR_COUNT_L6 represents NV Link Recovery Error Counter for Lane 6
	DCGM_FI_DEV_NVLINK_RECOVERY_ERROR_COUNT_L6 Short = 469
	// DCGM_FI_DEV_NVLINK_RECOVERY_ERROR_COUNT_L7 represents NV Link Recovery Error Counter for Lane 7
	DCGM_FI_DEV_NVLINK_RECOVERY_ERROR_COUNT_L7 Short = 470
	// DCGM_FI_DEV_NVLINK_RECOVERY_ERROR_COUNT_L8 represents NV Link Recovery Error Counter for Lane 8
	DCGM_FI_DEV_NVLINK_RECOVERY_ERROR_COUNT_L8 Short = 471
	// DCGM_FI_DEV_NVLINK_RECOVERY_ERROR_COUNT_L9 represents NV Link Recovery Error Counter for Lane 9
	DCGM_FI_DEV_NVLINK_RECOVERY_ERROR_COUNT_L9 Short = 472
	// DCGM_FI_DEV_NVLINK_RECOVERY_ERROR_COUNT_L10 represents NV Link Recovery Error Counter for Lane 10
	DCGM_FI_DEV_NVLINK_RECOVERY_ERROR_COUNT_L10 Short = 473
	// DCGM_FI_DEV_NVLINK_RECOVERY_ERROR_COUNT_L11 represents NV Link Recovery Error Counter for Lane 11
	DCGM_FI_DEV_NVLINK_RECOVERY_ERROR_COUNT_L11 Short = 474
	// DCGM_FI_DEV_NVLINK_THROUGHPUT_L6 represents NV Link Throughput for Lane 6
	DCGM_FI_DEV_NVLINK_THROUGHPUT_L6 Short = 475
	// DCGM_FI_DEV_NVLINK_THROUGHPUT_L7 represents NV Link Throughput for Lane 7
	DCGM_FI_DEV_NVLINK_THROUGHPUT_L7 Short = 476
	// DCGM_FI_DEV_NVLINK_THROUGHPUT_L8 represents NV Link Throughput for Lane 8
	DCGM_FI_DEV_NVLINK_THROUGHPUT_L8 Short = 477
	// DCGM_FI_DEV_NVLINK_THROUGHPUT_L9 represents NV Link Throughput for Lane 9
	DCGM_FI_DEV_NVLINK_THROUGHPUT_L9 Short = 478
	// DCGM_FI_DEV_NVLINK_THROUGHPUT_L10 represents NV Link Throughput for Lane 10
	DCGM_FI_DEV_NVLINK_THROUGHPUT_L10 Short = 479
	// DCGM_FI_DEV_NVLINK_THROUGHPUT_L11 represents NV Link Throughput for Lane 11
	DCGM_FI_DEV_NVLINK_THROUGHPUT_L11 Short = 480
	// DCGM_FI_DEV_NVLINK_CRC_FLIT_ERROR_COUNT_L15
	DCGM_FI_DEV_NVLINK_CRC_FLIT_ERROR_COUNT_L15 Short = 481
	// DCGM_FI_DEV_NVLINK_CRC_FLIT_ERROR_COUNT_L16
	DCGM_FI_DEV_NVLINK_CRC_FLIT_ERROR_COUNT_L16 Short = 482
	// DCGM_FI_DEV_NVLINK_CRC_FLIT_ERROR_COUNT_L17
	DCGM_FI_DEV_NVLINK_CRC_FLIT_ERROR_COUNT_L17 Short = 483
	// DCGM_FI_DEV_NVLINK_CRC_DATA_ERROR_COUNT_L15
	DCGM_FI_DEV_NVLINK_CRC_DATA_ERROR_COUNT_L15 Short = 484
	// DCGM_FI_DEV_NVLINK_CRC_DATA_ERROR_COUNT_L16
	DCGM_FI_DEV_NVLINK_CRC_DATA_ERROR_COUNT_L16 Short = 485
	// DCGM_FI_DEV_NVLINK_CRC_DATA_ERROR_COUNT_L17
	DCGM_FI_DEV_NVLINK_CRC_DATA_ERROR_COUNT_L17 Short = 486
	// DCGM_FI_DEV_NVLINK_REPLAY_ERROR_COUNT_L15
	DCGM_FI_DEV_NVLINK_REPLAY_ERROR_COUNT_L15 Short = 487
	// DCGM_FI_DEV_NVLINK_REPLAY_ERROR_COUNT_L16
	DCGM_FI_DEV_NVLINK_REPLAY_ERROR_COUNT_L16 Short = 488
	// DCGM_FI_DEV_NVLINK_REPLAY_ERROR_COUNT_L17
	DCGM_FI_DEV_NVLINK_REPLAY_ERROR_COUNT_L17 Short = 489
	// DCGM_FI_DEV_NVLINK_RECOVERY_ERROR_COUNT_L15
	DCGM_FI_DEV_NVLINK_RECOVERY_ERROR_COUNT_L15 Short = 491
	// DCGM_FI_DEV_NVLINK_RECOVERY_ERROR_COUNT_L16
	DCGM_FI_DEV_NVLINK_RECOVERY_ERROR_COUNT_L16 Short = 492
	// DCGM_FI_DEV_NVLINK_RECOVERY_ERROR_COUNT_L17
	DCGM_FI_DEV_NVLINK_RECOVERY_ERROR_COUNT_L17 Short = 493
	// DCGM_FI_DEV_NVLINK_THROUGHPUT_L15
	DCGM_FI_DEV_NVLINK_THROUGHPUT_L15 Short = 494
	// DCGM_FI_DEV_NVLINK_THROUGHPUT_L16
	DCGM_FI_DEV_NVLINK_THROUGHPUT_L16 Short = 495
	// DCGM_FI_DEV_NVLINK_THROUGHPUT_L17
	DCGM_FI_DEV_NVLINK_THROUGHPUT_L17 Short = 496
	// DCGM_FI_DEV_NVLINK_ERROR_DL_CRC represents NVLink CRC Error Counter
	DCGM_FI_DEV_NVLINK_ERROR_DL_CRC Short = 497
	// DCGM_FI_DEV_NVLINK_ERROR_DL_RECOVERY represents NVLink Recovery Error Counter
	DCGM_FI_DEV_NVLINK_ERROR_DL_RECOVERY Short = 498
	// DCGM_FI_DEV_NVLINK_ERROR_DL_REPLAY represents NVLink Replay Error Counter
	DCGM_FI_DEV_NVLINK_ERROR_DL_REPLAY Short = 499
	// DCGM_FI_DEV_VIRTUAL_MODE represents One of DCGM_GPU_VIRTUALIZATION_MODE_* constants.
	DCGM_FI_DEV_VIRTUAL_MODE Short = 500
	// DCGM_FI_DEV_SUPPORTED_TYPE_INFO represents Includes Count and Static info of vGPU types supported on a device
	DCGM_FI_DEV_SUPPORTED_TYPE_INFO Short = 501
	// DCGM_FI_DEV_CREATABLE_VGPU_TYPE_IDS represents Includes Count and currently Creatable vGPU types on a device
	DCGM_FI_DEV_CREATABLE_VGPU_TYPE_IDS Short = 502
	// DCGM_FI_DEV_VGPU_INSTANCE_IDS represents Includes Count and currently Active vGPU Instances on a device
	DCGM_FI_DEV_VGPU_INSTANCE_IDS Short = 503
	// DCGM_FI_DEV_VGPU_UTILIZATIONS represents Utilization values for vGPUs running on the device
	DCGM_FI_DEV_VGPU_UTILIZATIONS Short = 504
	// DCGM_FI_DEV_VGPU_PER_PROCESS_UTILIZATION represents Utilization values for processes running within vGPU VMs using the device
	DCGM_FI_DEV_VGPU_PER_PROCESS_UTILIZATION Short = 505
	// DCGM_FI_DEV_ENC_STATS represents Current encoder statistics for a given device
	DCGM_FI_DEV_ENC_STATS Short = 506
	// DCGM_FI_DEV_FBC_STATS represents Statistics of current active frame buffer capture sessions on a given device
	DCGM_FI_DEV_FBC_STATS Short = 507
	// DCGM_FI_DEV_FBC_SESSIONS_INFO represents Information about active frame buffer capture sessions on a target device
	DCGM_FI_DEV_FBC_SESSIONS_INFO Short = 508
	// DCGM_FI_DEV_SUPPORTED_VGPU_TYPE_IDS represents Includes Count and currently Supported vGPU types on a device
	DCGM_FI_DEV_SUPPORTED_VGPU_TYPE_IDS Short = 509
	// DCGM_FI_DEV_VGPU_TYPE_INFO represents Includes Static info of vGPU types supported on a device
	DCGM_FI_DEV_VGPU_TYPE_INFO Short = 510
	// DCGM_FI_DEV_VGPU_TYPE_NAME represents Includes the name of a vGPU type supported on a device
	DCGM_FI_DEV_VGPU_TYPE_NAME Short = 511
	// DCGM_FI_DEV_VGPU_TYPE_CLASS represents Includes the class of a vGPU type supported on a device
	DCGM_FI_DEV_VGPU_TYPE_CLASS Short = 512
	// DCGM_FI_DEV_VGPU_TYPE_LICENSE represents Includes the license info for a vGPU type supported on a device
	DCGM_FI_DEV_VGPU_TYPE_LICENSE Short = 513
	// DCGM_FI_FIRST_VGPU_FIELD_ID represents Starting field ID of the vGPU instance
	DCGM_FI_FIRST_VGPU_FIELD_ID Short = 520
	// DCGM_FI_DEV_VGPU_VM_ID represents VM ID of the vGPU instance
	DCGM_FI_DEV_VGPU_VM_ID Short = 520
	// DCGM_FI_DEV_VGPU_VM_NAME represents VM name of the vGPU instance
	DCGM_FI_DEV_VGPU_VM_NAME Short = 521
	// DCGM_FI_DEV_VGPU_TYPE represents vGPU type of the vGPU instance
	DCGM_FI_DEV_VGPU_TYPE Short = 522
	// DCGM_FI_DEV_VGPU_UUID represents UUID of the vGPU instance
	DCGM_FI_DEV_VGPU_UUID Short = 523
	// DCGM_FI_DEV_VGPU_DRIVER_VERSION represents Driver version of the vGPU instance
	DCGM_FI_DEV_VGPU_DRIVER_VERSION Short = 524
	// DCGM_FI_DEV_VGPU_MEMORY_USAGE represents Memory usage of the vGPU instance
	DCGM_FI_DEV_VGPU_MEMORY_USAGE Short = 525
	// DCGM_FI_DEV_VGPU_LICENSE_STATUS represents 1 = vgpu is licensed
	DCGM_FI_DEV_VGPU_LICENSE_STATUS Short = 526
	// DCGM_FI_DEV_VGPU_FRAME_RATE_LIMIT represents Frame rate limit of the vGPU instance
	DCGM_FI_DEV_VGPU_FRAME_RATE_LIMIT Short = 527
	// DCGM_FI_DEV_VGPU_ENC_STATS represents Current encoder statistics of the vGPU instance
	DCGM_FI_DEV_VGPU_ENC_STATS Short = 528
	// DCGM_FI_DEV_VGPU_ENC_SESSIONS_INFO represents Information about all active encoder sessions on the vGPU instance
	DCGM_FI_DEV_VGPU_ENC_SESSIONS_INFO Short = 529
	// DCGM_FI_DEV_VGPU_FBC_STATS represents Statistics of current active frame buffer capture sessions on the vGPU instance
	DCGM_FI_DEV_VGPU_FBC_STATS Short = 530
	// DCGM_FI_DEV_VGPU_FBC_SESSIONS_INFO represents Information about active frame buffer capture sessions on the vGPU instance
	DCGM_FI_DEV_VGPU_FBC_SESSIONS_INFO Short = 531
	// DCGM_FI_DEV_VGPU_INSTANCE_LICENSE_STATE represents License state information of the vGPU instance
	DCGM_FI_DEV_VGPU_INSTANCE_LICENSE_STATE Short = 532
	// DCGM_FI_DEV_VGPU_PCI_ID represents PCI Id of the vGPU instance
	DCGM_FI_DEV_VGPU_PCI_ID Short = 533
	// DCGM_FI_DEV_VGPU_VM_GPU_INSTANCE_ID represents GPU Instance ID for the given vGPU Instance
	DCGM_FI_DEV_VGPU_VM_GPU_INSTANCE_ID Short = 534
	// DCGM_FI_LAST_VGPU_FIELD_ID represents Last field ID of the vGPU instance
	DCGM_FI_LAST_VGPU_FIELD_ID Short = 570
	// DCGM_FI_DEV_PLATFORM_INFINIBAND_GUID represents Infiniband GUID string with format 0xXXXXXXXXXXXXXXXX for the specified GPU.
	DCGM_FI_DEV_PLATFORM_INFINIBAND_GUID Short = 571
	// DCGM_FI_DEV_PLATFORM_CHASSIS_SERIAL_NUMBER represents Serial number of the chassis containing this GPU
	DCGM_FI_DEV_PLATFORM_CHASSIS_SERIAL_NUMBER Short = 572
	// DCGM_FI_DEV_PLATFORM_CHASSIS_SLOT_NUMBER represents Slot number in the rack containing the GPU (includes switches)
	DCGM_FI_DEV_PLATFORM_CHASSIS_SLOT_NUMBER Short = 573
	// DCGM_FI_DEV_PLATFORM_TRAY_INDEX represents Tray index within the compute slots in the chassis containing this GPU (does not include switches)
	DCGM_FI_DEV_PLATFORM_TRAY_INDEX Short = 574
	// DCGM_FI_DEV_PLATFORM_HOST_ID represents Index of the node within the slot containing the GPU
	DCGM_FI_DEV_PLATFORM_HOST_ID Short = 575
	// DCGM_FI_DEV_PLATFORM_PEER_TYPE represents Platform indicated NVLink-peer type (e.g. switch present or not)
	DCGM_FI_DEV_PLATFORM_PEER_TYPE Short = 576
	// DCGM_FI_DEV_PLATFORM_MODULE_ID represents ID of the GPU within the node
	DCGM_FI_DEV_PLATFORM_MODULE_ID Short = 577
	// DCGM_FI_DEV_NVLINK_PPRM_OPER_RECOVERY represents PPRM recovery operation status
	DCGM_FI_DEV_NVLINK_PPRM_OPER_RECOVERY Short = 580
	// DCGM_FI_DEV_NVLINK_PPCNT_RECOVERY_TIME_SINCE_LAST represents Time in seconds since last PRM recovery
	DCGM_FI_DEV_NVLINK_PPCNT_RECOVERY_TIME_SINCE_LAST Short = 581
	// DCGM_FI_DEV_NVLINK_PPCNT_RECOVERY_TIME_BETWEEN_LAST_TWO represents Time in milliseconds between last two recoveries
	DCGM_FI_DEV_NVLINK_PPCNT_RECOVERY_TIME_BETWEEN_LAST_TWO Short = 582
	// DCGM_FI_DEV_NVLINK_PPCNT_RECOVERY_TOTAL_SUCCESSFUL_EVENTS represents Total successful recovery events counter
	DCGM_FI_DEV_NVLINK_PPCNT_RECOVERY_TOTAL_SUCCESSFUL_EVENTS Short = 583
	// DCGM_FI_DEV_NVLINK_PPCNT_PHYSICAL_SUCCESSFUL_RECOVERY_EVENTS represents Physical layer successful recovery events
	DCGM_FI_DEV_NVLINK_PPCNT_PHYSICAL_SUCCESSFUL_RECOVERY_EVENTS Short = 584
	// DCGM_FI_DEV_NVLINK_PPCNT_PHYSICAL_LINK_DOWN_COUNTER represents Physical layer link down counter
	DCGM_FI_DEV_NVLINK_PPCNT_PHYSICAL_LINK_DOWN_COUNTER Short = 585
	// DCGM_FI_DEV_NVLINK_PPCNT_PLR_RCV_CODES represents PLR received codewords counter
	DCGM_FI_DEV_NVLINK_PPCNT_PLR_RCV_CODES Short = 586
	// DCGM_FI_DEV_NVLINK_PPCNT_PLR_RCV_CODE_ERR represents PLR received code error counter
	DCGM_FI_DEV_NVLINK_PPCNT_PLR_RCV_CODE_ERR Short = 587
	// DCGM_FI_DEV_NVLINK_PPCNT_PLR_RCV_UNCORRECTABLE_CODE represents PLR received uncorrectable codes counter
	DCGM_FI_DEV_NVLINK_PPCNT_PLR_RCV_UNCORRECTABLE_CODE Short = 588
	// DCGM_FI_DEV_NVLINK_PPCNT_PLR_XMIT_CODES represents PLR transmitted codewords counter
	DCGM_FI_DEV_NVLINK_PPCNT_PLR_XMIT_CODES Short = 589
	// DCGM_FI_DEV_NVLINK_PPCNT_PLR_XMIT_RETRY_CODES represents PLR transmitted retry codes counter
	DCGM_FI_DEV_NVLINK_PPCNT_PLR_XMIT_RETRY_CODES Short = 590
	// DCGM_FI_DEV_NVLINK_PPCNT_PLR_XMIT_RETRY_EVENTS represents PLR transmitted retry events counter
	DCGM_FI_DEV_NVLINK_PPCNT_PLR_XMIT_RETRY_EVENTS Short = 591
	// DCGM_FI_DEV_NVLINK_PPCNT_PLR_SYNC_EVENTS represents PLR sync events counter
	DCGM_FI_DEV_NVLINK_PPCNT_PLR_SYNC_EVENTS Short = 592
	// DCGM_FI_INTERNAL_FIELDS_0_START represents Starting ID for all the internal fields
	DCGM_FI_INTERNAL_FIELDS_0_START Short = 600
	// DCGM_FI_INTERNAL_FIELDS_0_END represents <p>NVSwitch latency bins for port 0</p>
	DCGM_FI_INTERNAL_FIELDS_0_END Short = 699
	// DCGM_FI_FIRST_NVSWITCH_FIELD_ID represents Starting field ID of the NVSwitch instance
	DCGM_FI_FIRST_NVSWITCH_FIELD_ID Short = 700
	// DCGM_FI_DEV_NVSWITCH_VOLTAGE_MVOLT represents NvSwitch voltage
	DCGM_FI_DEV_NVSWITCH_VOLTAGE_MVOLT Short = 701
	// DCGM_FI_DEV_NVSWITCH_CURRENT_IDDQ represents NvSwitch Current IDDQ
	DCGM_FI_DEV_NVSWITCH_CURRENT_IDDQ Short = 702
	// DCGM_FI_DEV_NVSWITCH_CURRENT_IDDQ_REV represents NvSwitch Current IDDQ Rev
	DCGM_FI_DEV_NVSWITCH_CURRENT_IDDQ_REV Short = 703
	// DCGM_FI_DEV_NVSWITCH_CURRENT_IDDQ_DVDD represents NvSwitch Current IDDQ Rev DVDD
	DCGM_FI_DEV_NVSWITCH_CURRENT_IDDQ_DVDD Short = 704
	// DCGM_FI_DEV_NVSWITCH_POWER_VDD represents NvSwitch Power VDD in watts
	DCGM_FI_DEV_NVSWITCH_POWER_VDD Short = 705
	// DCGM_FI_DEV_NVSWITCH_POWER_DVDD represents NvSwitch Power DVDD in watts
	DCGM_FI_DEV_NVSWITCH_POWER_DVDD Short = 706
	// DCGM_FI_DEV_NVSWITCH_POWER_HVDD represents NvSwitch Power HVDD in watts
	DCGM_FI_DEV_NVSWITCH_POWER_HVDD Short = 707
	// DCGM_FI_DEV_NVSWITCH_LINK_THROUGHPUT_TX represents <p>NVSwitch Tx Throughput Counter for ports 0-17</p>
	DCGM_FI_DEV_NVSWITCH_LINK_THROUGHPUT_TX Short = 780
	// DCGM_FI_DEV_NVSWITCH_LINK_THROUGHPUT_RX represents NVSwitch Rx Throughput Counter for ports 0-17
	DCGM_FI_DEV_NVSWITCH_LINK_THROUGHPUT_RX Short = 781
	// DCGM_FI_DEV_NVSWITCH_LINK_FATAL_ERRORS represents NvSwitch fatal_errors for ports 0-17
	DCGM_FI_DEV_NVSWITCH_LINK_FATAL_ERRORS Short = 782
	// DCGM_FI_DEV_NVSWITCH_LINK_NON_FATAL_ERRORS represents NvSwitch non_fatal_errors for ports 0-17
	DCGM_FI_DEV_NVSWITCH_LINK_NON_FATAL_ERRORS Short = 783
	// DCGM_FI_DEV_NVSWITCH_LINK_REPLAY_ERRORS represents NvSwitch replay_count_errors for ports  0-17
	DCGM_FI_DEV_NVSWITCH_LINK_REPLAY_ERRORS Short = 784
	// DCGM_FI_DEV_NVSWITCH_LINK_RECOVERY_ERRORS represents NvSwitch recovery_count_errors for ports 0-17
	DCGM_FI_DEV_NVSWITCH_LINK_RECOVERY_ERRORS Short = 785
	// DCGM_FI_DEV_NVSWITCH_LINK_FLIT_ERRORS represents NvSwitch filt_err_count_errors for ports 0-17
	DCGM_FI_DEV_NVSWITCH_LINK_FLIT_ERRORS Short = 786
	// DCGM_FI_DEV_NVSWITCH_LINK_CRC_ERRORS represents NvLink lane_crs_err_count_aggregate_errors for ports 0-17
	DCGM_FI_DEV_NVSWITCH_LINK_CRC_ERRORS Short = 787
	// DCGM_FI_DEV_NVSWITCH_LINK_ECC_ERRORS represents NvLink lane ecc_err_count_aggregate_errors for ports 0-17
	DCGM_FI_DEV_NVSWITCH_LINK_ECC_ERRORS Short = 788
	// DCGM_FI_DEV_NVSWITCH_LINK_LATENCY_LOW_VC0 represents Nvlink lane latency low lane0 counter.
	DCGM_FI_DEV_NVSWITCH_LINK_LATENCY_LOW_VC0 Short = 789
	// DCGM_FI_DEV_NVSWITCH_LINK_LATENCY_LOW_VC1 represents Nvlink lane latency low lane1 counter.
	DCGM_FI_DEV_NVSWITCH_LINK_LATENCY_LOW_VC1 Short = 790
	// DCGM_FI_DEV_NVSWITCH_LINK_LATENCY_LOW_VC2 represents Nvlink lane latency low lane2 counter.
	DCGM_FI_DEV_NVSWITCH_LINK_LATENCY_LOW_VC2 Short = 791
	// DCGM_FI_DEV_NVSWITCH_LINK_LATENCY_LOW_VC3 represents Nvlink lane latency low lane3 counter.
	DCGM_FI_DEV_NVSWITCH_LINK_LATENCY_LOW_VC3 Short = 792
	// DCGM_FI_DEV_NVSWITCH_LINK_LATENCY_MEDIUM_VC0 represents Nvlink lane latency medium lane0 counter.
	DCGM_FI_DEV_NVSWITCH_LINK_LATENCY_MEDIUM_VC0 Short = 793
	// DCGM_FI_DEV_NVSWITCH_LINK_LATENCY_MEDIUM_VC1 represents Nvlink lane latency medium lane1 counter.
	DCGM_FI_DEV_NVSWITCH_LINK_LATENCY_MEDIUM_VC1 Short = 794
	// DCGM_FI_DEV_NVSWITCH_LINK_LATENCY_MEDIUM_VC2 represents Nvlink lane latency medium lane2 counter.
	DCGM_FI_DEV_NVSWITCH_LINK_LATENCY_MEDIUM_VC2 Short = 795
	// DCGM_FI_DEV_NVSWITCH_LINK_LATENCY_MEDIUM_VC3 represents Nvlink lane latency medium lane3 counter.
	DCGM_FI_DEV_NVSWITCH_LINK_LATENCY_MEDIUM_VC3 Short = 796
	// DCGM_FI_DEV_NVSWITCH_LINK_LATENCY_HIGH_VC0 represents Nvlink lane latency high lane0 counter.
	DCGM_FI_DEV_NVSWITCH_LINK_LATENCY_HIGH_VC0 Short = 797
	// DCGM_FI_DEV_NVSWITCH_LINK_LATENCY_HIGH_VC1 represents Nvlink lane latency high lane1 counter.
	DCGM_FI_DEV_NVSWITCH_LINK_LATENCY_HIGH_VC1 Short = 798
	// DCGM_FI_DEV_NVSWITCH_LINK_LATENCY_HIGH_VC2 represents Nvlink lane latency high lane2 counter.
	DCGM_FI_DEV_NVSWITCH_LINK_LATENCY_HIGH_VC2 Short = 799
	// DCGM_FI_DEV_NVSWITCH_LINK_LATENCY_HIGH_VC3 represents Nvlink lane latency high lane3 counter.
	DCGM_FI_DEV_NVSWITCH_LINK_LATENCY_HIGH_VC3 Short = 800
	// DCGM_FI_DEV_NVSWITCH_LINK_LATENCY_PANIC_VC0 represents Nvlink lane latency panic lane0 counter.
	DCGM_FI_DEV_NVSWITCH_LINK_LATENCY_PANIC_VC0 Short = 801
	// DCGM_FI_DEV_NVSWITCH_LINK_LATENCY_PANIC_VC1 represents Nvlink lane latency panic lane1 counter.
	DCGM_FI_DEV_NVSWITCH_LINK_LATENCY_PANIC_VC1 Short = 802
	// DCGM_FI_DEV_NVSWITCH_LINK_LATENCY_PANIC_VC2 represents Nvlink lane latency panic lane2 counter.
	DCGM_FI_DEV_NVSWITCH_LINK_LATENCY_PANIC_VC2 Short = 803
	// DCGM_FI_DEV_NVSWITCH_LINK_LATENCY_PANIC_VC3 represents Nvlink lane latency panic lane2 counter.
	DCGM_FI_DEV_NVSWITCH_LINK_LATENCY_PANIC_VC3 Short = 804
	// DCGM_FI_DEV_NVSWITCH_LINK_LATENCY_COUNT_VC0 represents Nvlink lane latency count lane0 counter.
	DCGM_FI_DEV_NVSWITCH_LINK_LATENCY_COUNT_VC0 Short = 805
	// DCGM_FI_DEV_NVSWITCH_LINK_LATENCY_COUNT_VC1 represents Nvlink lane latency count lane1 counter.
	DCGM_FI_DEV_NVSWITCH_LINK_LATENCY_COUNT_VC1 Short = 806
	// DCGM_FI_DEV_NVSWITCH_LINK_LATENCY_COUNT_VC2 represents Nvlink lane latency count lane2 counter.
	DCGM_FI_DEV_NVSWITCH_LINK_LATENCY_COUNT_VC2 Short = 807
	// DCGM_FI_DEV_NVSWITCH_LINK_LATENCY_COUNT_VC3 represents Nvlink lane latency count lane3 counter.
	DCGM_FI_DEV_NVSWITCH_LINK_LATENCY_COUNT_VC3 Short = 808
	// DCGM_FI_DEV_NVSWITCH_LINK_CRC_ERRORS_LANE0 represents NvLink lane crc_err_count for lane 0 on ports 0-17
	DCGM_FI_DEV_NVSWITCH_LINK_CRC_ERRORS_LANE0 Short = 809
	// DCGM_FI_DEV_NVSWITCH_LINK_CRC_ERRORS_LANE1 represents NvLink lane crc_err_count for lane 1 on ports 0-17
	DCGM_FI_DEV_NVSWITCH_LINK_CRC_ERRORS_LANE1 Short = 810
	// DCGM_FI_DEV_NVSWITCH_LINK_CRC_ERRORS_LANE2 represents NvLink lane crc_err_count for lane 2 on ports 0-17
	DCGM_FI_DEV_NVSWITCH_LINK_CRC_ERRORS_LANE2 Short = 811
	// DCGM_FI_DEV_NVSWITCH_LINK_CRC_ERRORS_LANE3 represents NvLink lane crc_err_count for lane 3 on ports 0-17
	DCGM_FI_DEV_NVSWITCH_LINK_CRC_ERRORS_LANE3 Short = 812
	// DCGM_FI_DEV_NVSWITCH_LINK_ECC_ERRORS_LANE0 represents NvLink lane ecc_err_count for lane 0 on ports 0-17
	DCGM_FI_DEV_NVSWITCH_LINK_ECC_ERRORS_LANE0 Short = 813
	// DCGM_FI_DEV_NVSWITCH_LINK_ECC_ERRORS_LANE1 represents NvLink lane ecc_err_count for lane 1 on ports 0-17
	DCGM_FI_DEV_NVSWITCH_LINK_ECC_ERRORS_LANE1 Short = 814
	// DCGM_FI_DEV_NVSWITCH_LINK_ECC_ERRORS_LANE2 represents NvLink lane ecc_err_count for lane 2 on ports 0-17
	DCGM_FI_DEV_NVSWITCH_LINK_ECC_ERRORS_LANE2 Short = 815
	// DCGM_FI_DEV_NVSWITCH_LINK_ECC_ERRORS_LANE3 represents NvLink lane ecc_err_count for lane 3 on ports 0-17
	DCGM_FI_DEV_NVSWITCH_LINK_ECC_ERRORS_LANE3 Short = 816
	// DCGM_FI_DEV_NVSWITCH_LINK_CRC_ERRORS_LANE4 represents NvLink lane crc_err_count for lane 4 on ports 0-17
	DCGM_FI_DEV_NVSWITCH_LINK_CRC_ERRORS_LANE4 Short = 817
	// DCGM_FI_DEV_NVSWITCH_LINK_CRC_ERRORS_LANE5 represents NvLink lane crc_err_count for lane 5 on ports 0-17
	DCGM_FI_DEV_NVSWITCH_LINK_CRC_ERRORS_LANE5 Short = 818
	// DCGM_FI_DEV_NVSWITCH_LINK_CRC_ERRORS_LANE6 represents NvLink lane crc_err_count for lane 6 on ports 0-17
	DCGM_FI_DEV_NVSWITCH_LINK_CRC_ERRORS_LANE6 Short = 819
	// DCGM_FI_DEV_NVSWITCH_LINK_CRC_ERRORS_LANE7 represents NvLink lane crc_err_count for lane 7 on ports 0-17
	DCGM_FI_DEV_NVSWITCH_LINK_CRC_ERRORS_LANE7 Short = 820
	// DCGM_FI_DEV_NVSWITCH_LINK_ECC_ERRORS_LANE4 represents NvLink lane ecc_err_count for lane 4 on ports 0-17
	DCGM_FI_DEV_NVSWITCH_LINK_ECC_ERRORS_LANE4 Short = 821
	// DCGM_FI_DEV_NVSWITCH_LINK_ECC_ERRORS_LANE5 represents NvLink lane ecc_err_count for lane 5 on ports 0-17
	DCGM_FI_DEV_NVSWITCH_LINK_ECC_ERRORS_LANE5 Short = 822
	// DCGM_FI_DEV_NVSWITCH_LINK_ECC_ERRORS_LANE6 represents NvLink lane ecc_err_count for lane 6 on ports 0-17
	DCGM_FI_DEV_NVSWITCH_LINK_ECC_ERRORS_LANE6 Short = 823
	// DCGM_FI_DEV_NVSWITCH_LINK_ECC_ERRORS_LANE7 represents NvLink lane ecc_err_count for lane 7 on ports 0-17
	DCGM_FI_DEV_NVSWITCH_LINK_ECC_ERRORS_LANE7 Short = 824
	// DCGM_FI_DEV_NVLINK_TX_THROUGHPUT_L0 represents NV Link TX Throughput for Lane 0
	DCGM_FI_DEV_NVLINK_TX_THROUGHPUT_L0 Short = 825
	// DCGM_FI_DEV_NVLINK_TX_THROUGHPUT_L1 represents NV Link TX Throughput for Lane 1
	DCGM_FI_DEV_NVLINK_TX_THROUGHPUT_L1 Short = 826
	// DCGM_FI_DEV_NVLINK_TX_THROUGHPUT_L2 represents NV Link TX Throughput for Lane 2
	DCGM_FI_DEV_NVLINK_TX_THROUGHPUT_L2 Short = 827
	// DCGM_FI_DEV_NVLINK_TX_THROUGHPUT_L3 represents NV Link TX Throughput for Lane 3
	DCGM_FI_DEV_NVLINK_TX_THROUGHPUT_L3 Short = 828
	// DCGM_FI_DEV_NVLINK_TX_THROUGHPUT_L4 represents NV Link TX Throughput for Lane 4
	DCGM_FI_DEV_NVLINK_TX_THROUGHPUT_L4 Short = 829
	// DCGM_FI_DEV_NVLINK_TX_THROUGHPUT_L5 represents NV Link TX Throughput for Lane 5
	DCGM_FI_DEV_NVLINK_TX_THROUGHPUT_L5 Short = 830
	// DCGM_FI_DEV_NVLINK_TX_THROUGHPUT_L6 represents NV Link TX Throughput for Lane 6
	DCGM_FI_DEV_NVLINK_TX_THROUGHPUT_L6 Short = 831
	// DCGM_FI_DEV_NVLINK_TX_THROUGHPUT_L7 represents NV Link TX Throughput for Lane 7
	DCGM_FI_DEV_NVLINK_TX_THROUGHPUT_L7 Short = 832
	// DCGM_FI_DEV_NVLINK_TX_THROUGHPUT_L8 represents NV Link TX Throughput for Lane 8
	DCGM_FI_DEV_NVLINK_TX_THROUGHPUT_L8 Short = 833
	// DCGM_FI_DEV_NVLINK_TX_THROUGHPUT_L9 represents NV Link TX Throughput for Lane 9
	DCGM_FI_DEV_NVLINK_TX_THROUGHPUT_L9 Short = 834
	// DCGM_FI_DEV_NVLINK_TX_THROUGHPUT_L10 represents NV Link TX Throughput for Lane 10
	DCGM_FI_DEV_NVLINK_TX_THROUGHPUT_L10 Short = 835
	// DCGM_FI_DEV_NVLINK_TX_THROUGHPUT_L11 represents NV Link TX Throughput for Lane 11
	DCGM_FI_DEV_NVLINK_TX_THROUGHPUT_L11 Short = 836
	// DCGM_FI_DEV_NVLINK_TX_THROUGHPUT_L12 represents NV Link TX Throughput for Lane 12
	DCGM_FI_DEV_NVLINK_TX_THROUGHPUT_L12 Short = 837
	// DCGM_FI_DEV_NVLINK_TX_THROUGHPUT_L13 represents NV Link TX Throughput for Lane 13
	DCGM_FI_DEV_NVLINK_TX_THROUGHPUT_L13 Short = 838
	// DCGM_FI_DEV_NVLINK_TX_THROUGHPUT_L14 represents NV Link TX Throughput for Lane 14
	DCGM_FI_DEV_NVLINK_TX_THROUGHPUT_L14 Short = 839
	// DCGM_FI_DEV_NVLINK_TX_THROUGHPUT_L15 represents NV Link TX Throughput for Lane 15
	DCGM_FI_DEV_NVLINK_TX_THROUGHPUT_L15 Short = 840
	// DCGM_FI_DEV_NVLINK_TX_THROUGHPUT_L16 represents NV Link TX Throughput for Lane 16
	DCGM_FI_DEV_NVLINK_TX_THROUGHPUT_L16 Short = 841
	// DCGM_FI_DEV_NVLINK_TX_THROUGHPUT_L17 represents NV Link TX Throughput for Lane 17
	DCGM_FI_DEV_NVLINK_TX_THROUGHPUT_L17 Short = 842
	// DCGM_FI_DEV_NVLINK_TX_THROUGHPUT_TOTAL represents NV Link Throughput total for all TX Lanes
	DCGM_FI_DEV_NVLINK_TX_THROUGHPUT_TOTAL Short = 843
	// DCGM_FI_DEV_NVSWITCH_FATAL_ERRORS represents Note: value field indicates the specific SXid reported
	DCGM_FI_DEV_NVSWITCH_FATAL_ERRORS Short = 856
	// DCGM_FI_DEV_NVSWITCH_NON_FATAL_ERRORS represents Note: value field indicates the specific SXid reported
	DCGM_FI_DEV_NVSWITCH_NON_FATAL_ERRORS Short = 857
	// DCGM_FI_DEV_NVSWITCH_TEMPERATURE_CURRENT represents NVSwitch current temperature.
	DCGM_FI_DEV_NVSWITCH_TEMPERATURE_CURRENT Short = 858
	// DCGM_FI_DEV_NVSWITCH_TEMPERATURE_LIMIT_SLOWDOWN represents NVSwitch limit slowdown temperature.
	DCGM_FI_DEV_NVSWITCH_TEMPERATURE_LIMIT_SLOWDOWN Short = 859
	// DCGM_FI_DEV_NVSWITCH_TEMPERATURE_LIMIT_SHUTDOWN represents NVSwitch limit shutdown temperature.
	DCGM_FI_DEV_NVSWITCH_TEMPERATURE_LIMIT_SHUTDOWN Short = 860
	// DCGM_FI_DEV_NVSWITCH_THROUGHPUT_TX represents NVSwitch throughput Tx.
	DCGM_FI_DEV_NVSWITCH_THROUGHPUT_TX Short = 861
	// DCGM_FI_DEV_NVSWITCH_THROUGHPUT_RX represents NVSwitch throughput Rx.
	DCGM_FI_DEV_NVSWITCH_THROUGHPUT_RX Short = 862
	// DCGM_FI_DEV_NVSWITCH_PHYS_ID represents NVSwitch Physical ID.
	DCGM_FI_DEV_NVSWITCH_PHYS_ID Short = 863
	// DCGM_FI_DEV_NVSWITCH_RESET_REQUIRED represents NVSwitch reset required.
	DCGM_FI_DEV_NVSWITCH_RESET_REQUIRED Short = 864
	// DCGM_FI_DEV_NVSWITCH_LINK_ID represents NvSwitch NvLink ID
	DCGM_FI_DEV_NVSWITCH_LINK_ID Short = 865
	// DCGM_FI_DEV_NVSWITCH_PCIE_DOMAIN represents NvSwitch PCIE domain
	DCGM_FI_DEV_NVSWITCH_PCIE_DOMAIN Short = 866
	// DCGM_FI_DEV_NVSWITCH_PCIE_BUS represents NvSwitch PCIE bus
	DCGM_FI_DEV_NVSWITCH_PCIE_BUS Short = 867
	// DCGM_FI_DEV_NVSWITCH_PCIE_DEVICE represents NvSwitch PCIE device
	DCGM_FI_DEV_NVSWITCH_PCIE_DEVICE Short = 868
	// DCGM_FI_DEV_NVSWITCH_PCIE_FUNCTION represents NvSwitch PCIE function
	DCGM_FI_DEV_NVSWITCH_PCIE_FUNCTION Short = 869
	// DCGM_FI_DEV_NVSWITCH_LINK_STATUS represents NvLink status.  UNKNOWN:-1 OFF:0 SAFE:1 ACTIVE:2 ERROR:3
	DCGM_FI_DEV_NVSWITCH_LINK_STATUS Short = 870
	// DCGM_FI_DEV_NVSWITCH_LINK_TYPE represents NvLink device type (NSCQ: GPU=1, Switch=2; NVSDM: CA=1, Switch=2, GPU=5)
	DCGM_FI_DEV_NVSWITCH_LINK_TYPE Short = 871
	// DCGM_FI_DEV_NVSWITCH_LINK_REMOTE_PCIE_DOMAIN represents NvLink device pcie domain.
	DCGM_FI_DEV_NVSWITCH_LINK_REMOTE_PCIE_DOMAIN Short = 872
	// DCGM_FI_DEV_NVSWITCH_LINK_REMOTE_PCIE_BUS represents NvLink device pcie bus.
	DCGM_FI_DEV_NVSWITCH_LINK_REMOTE_PCIE_BUS Short = 873
	// DCGM_FI_DEV_NVSWITCH_LINK_REMOTE_PCIE_DEVICE represents NvLink device pcie device.
	DCGM_FI_DEV_NVSWITCH_LINK_REMOTE_PCIE_DEVICE Short = 874
	// DCGM_FI_DEV_NVSWITCH_LINK_REMOTE_PCIE_FUNCTION represents NvLink device pcie function.
	DCGM_FI_DEV_NVSWITCH_LINK_REMOTE_PCIE_FUNCTION Short = 875
	// DCGM_FI_DEV_NVSWITCH_LINK_DEVICE_LINK_ID represents NvLink device link ID
	DCGM_FI_DEV_NVSWITCH_LINK_DEVICE_LINK_ID Short = 876
	// DCGM_FI_DEV_NVSWITCH_LINK_DEVICE_LINK_SID represents NvLink device SID.
	DCGM_FI_DEV_NVSWITCH_LINK_DEVICE_LINK_SID Short = 877
	// DCGM_FI_DEV_NVSWITCH_DEVICE_UUID represents NvLink device switch/link uid.
	DCGM_FI_DEV_NVSWITCH_DEVICE_UUID Short = 878
	// DCGM_FI_DEV_NVLINK_RX_THROUGHPUT_L0 represents NV Link RX Throughput for Lane 0
	DCGM_FI_DEV_NVLINK_RX_THROUGHPUT_L0 Short = 879
	// DCGM_FI_DEV_NVLINK_RX_THROUGHPUT_L1 represents NV Link RX Throughput for Lane 1
	DCGM_FI_DEV_NVLINK_RX_THROUGHPUT_L1 Short = 880
	// DCGM_FI_DEV_NVLINK_RX_THROUGHPUT_L2 represents NV Link RX Throughput for Lane 2
	DCGM_FI_DEV_NVLINK_RX_THROUGHPUT_L2 Short = 881
	// DCGM_FI_DEV_NVLINK_RX_THROUGHPUT_L3 represents NV Link RX Throughput for Lane 3
	DCGM_FI_DEV_NVLINK_RX_THROUGHPUT_L3 Short = 882
	// DCGM_FI_DEV_NVLINK_RX_THROUGHPUT_L4 represents NV Link RX Throughput for Lane 4
	DCGM_FI_DEV_NVLINK_RX_THROUGHPUT_L4 Short = 883
	// DCGM_FI_DEV_NVLINK_RX_THROUGHPUT_L5 represents NV Link RX Throughput for Lane 5
	DCGM_FI_DEV_NVLINK_RX_THROUGHPUT_L5 Short = 884
	// DCGM_FI_DEV_NVLINK_RX_THROUGHPUT_L6 represents NV Link RX Throughput for Lane 6
	DCGM_FI_DEV_NVLINK_RX_THROUGHPUT_L6 Short = 885
	// DCGM_FI_DEV_NVLINK_RX_THROUGHPUT_L7 represents NV Link RX Throughput for Lane 7
	DCGM_FI_DEV_NVLINK_RX_THROUGHPUT_L7 Short = 886
	// DCGM_FI_DEV_NVLINK_RX_THROUGHPUT_L8 represents NV Link RX Throughput for Lane 8
	DCGM_FI_DEV_NVLINK_RX_THROUGHPUT_L8 Short = 887
	// DCGM_FI_DEV_NVLINK_RX_THROUGHPUT_L9 represents NV Link RX Throughput for Lane 9
	DCGM_FI_DEV_NVLINK_RX_THROUGHPUT_L9 Short = 888
	// DCGM_FI_DEV_NVLINK_RX_THROUGHPUT_L10 represents NV Link RX Throughput for Lane 10
	DCGM_FI_DEV_NVLINK_RX_THROUGHPUT_L10 Short = 889
	// DCGM_FI_DEV_NVLINK_RX_THROUGHPUT_L11 represents NV Link RX Throughput for Lane 11
	DCGM_FI_DEV_NVLINK_RX_THROUGHPUT_L11 Short = 890
	// DCGM_FI_DEV_NVLINK_RX_THROUGHPUT_L12 represents NV Link RX Throughput for Lane 12
	DCGM_FI_DEV_NVLINK_RX_THROUGHPUT_L12 Short = 891
	// DCGM_FI_DEV_NVLINK_RX_THROUGHPUT_L13 represents NV Link RX Throughput for Lane 13
	DCGM_FI_DEV_NVLINK_RX_THROUGHPUT_L13 Short = 892
	// DCGM_FI_DEV_NVLINK_RX_THROUGHPUT_L14 represents NV Link RX Throughput for Lane 14
	DCGM_FI_DEV_NVLINK_RX_THROUGHPUT_L14 Short = 893
	// DCGM_FI_DEV_NVLINK_RX_THROUGHPUT_L15 represents NV Link RX Throughput for Lane 15
	DCGM_FI_DEV_NVLINK_RX_THROUGHPUT_L15 Short = 894
	// DCGM_FI_DEV_NVLINK_RX_THROUGHPUT_L16 represents NV Link RX Throughput for Lane 16
	DCGM_FI_DEV_NVLINK_RX_THROUGHPUT_L16 Short = 895
	// DCGM_FI_DEV_NVLINK_RX_THROUGHPUT_L17 represents NV Link RX Throughput for Lane 17
	DCGM_FI_DEV_NVLINK_RX_THROUGHPUT_L17 Short = 896
	// DCGM_FI_DEV_NVLINK_RX_THROUGHPUT_TOTAL represents NV Link Throughput total for all RX Lanes
	DCGM_FI_DEV_NVLINK_RX_THROUGHPUT_TOTAL Short = 897
	// DCGM_FI_LAST_NVSWITCH_FIELD_ID represents Last field ID of the NVSwitch instance
	DCGM_FI_LAST_NVSWITCH_FIELD_ID Short = 899
	// DCGM_FI_PROF_GR_ENGINE_ACTIVE represents compute pipe is busy.
	DCGM_FI_PROF_GR_ENGINE_ACTIVE Short = 1001
	// DCGM_FI_PROF_SM_ACTIVE represents (computed from the number of cycles and elapsed cycles)
	DCGM_FI_PROF_SM_ACTIVE Short = 1002
	// DCGM_FI_PROF_SM_OCCUPANCY represents maximum number of warps per elapsed cycle)
	DCGM_FI_PROF_SM_OCCUPANCY Short = 1003
	// DCGM_FI_PROF_PIPE_TENSOR_ACTIVE represents (off the peak sustained elapsed cycles)
	DCGM_FI_PROF_PIPE_TENSOR_ACTIVE Short = 1004
	// DCGM_FI_PROF_DRAM_ACTIVE represents active sending or receiving data.
	DCGM_FI_PROF_DRAM_ACTIVE Short = 1005
	// DCGM_FI_PROF_PIPE_FP64_ACTIVE represents Ratio of cycles the fp64 pipe is active.
	DCGM_FI_PROF_PIPE_FP64_ACTIVE Short = 1006
	// DCGM_FI_PROF_PIPE_FP32_ACTIVE represents Ratio of cycles the fp32 pipe is active.
	DCGM_FI_PROF_PIPE_FP32_ACTIVE Short = 1007
	// DCGM_FI_PROF_PIPE_FP16_ACTIVE represents Ratio of cycles the fp16 pipe is active. This does not include HMMA.
	DCGM_FI_PROF_PIPE_FP16_ACTIVE Short = 1008
	// DCGM_FI_PROF_PCIE_TX_BYTES represents would be reflected in this metric.
	DCGM_FI_PROF_PCIE_TX_BYTES Short = 1009
	// DCGM_FI_PROF_PCIE_RX_BYTES represents would be reflected in this metric.
	DCGM_FI_PROF_PCIE_RX_BYTES Short = 1010
	// DCGM_FI_PROF_NVLINK_TX_BYTES represents Per-link fields are available below
	DCGM_FI_PROF_NVLINK_TX_BYTES Short = 1011
	// DCGM_FI_PROF_NVLINK_RX_BYTES represents Per-link fields are available below
	DCGM_FI_PROF_NVLINK_RX_BYTES Short = 1012
	// DCGM_FI_PROF_PIPE_TENSOR_IMMA_ACTIVE represents The ratio of cycles the tensor (IMMA) pipe is active (off the peak sustained elapsed cycles)
	DCGM_FI_PROF_PIPE_TENSOR_IMMA_ACTIVE Short = 1013
	// DCGM_FI_PROF_PIPE_TENSOR_HMMA_ACTIVE represents The ratio of cycles the tensor (HMMA) pipe is active (off the peak sustained elapsed cycles)
	DCGM_FI_PROF_PIPE_TENSOR_HMMA_ACTIVE Short = 1014
	// DCGM_FI_PROF_PIPE_TENSOR_DFMA_ACTIVE represents The ratio of cycles the tensor (DFMA) pipe is active (off the peak sustained elapsed cycles)
	DCGM_FI_PROF_PIPE_TENSOR_DFMA_ACTIVE Short = 1015
	// DCGM_FI_PROF_PIPE_INT_ACTIVE represents Ratio of cycles the integer pipe is active.
	DCGM_FI_PROF_PIPE_INT_ACTIVE Short = 1016
	// DCGM_FI_PROF_NVDEC0_ACTIVE represents Ratio of cycles each of the NVDEC engines are active.
	DCGM_FI_PROF_NVDEC0_ACTIVE Short = 1017
	// DCGM_FI_PROF_NVDEC1_ACTIVE
	DCGM_FI_PROF_NVDEC1_ACTIVE Short = 1018
	// DCGM_FI_PROF_NVDEC2_ACTIVE
	DCGM_FI_PROF_NVDEC2_ACTIVE Short = 1019
	// DCGM_FI_PROF_NVDEC3_ACTIVE
	DCGM_FI_PROF_NVDEC3_ACTIVE Short = 1020
	// DCGM_FI_PROF_NVDEC4_ACTIVE
	DCGM_FI_PROF_NVDEC4_ACTIVE Short = 1021
	// DCGM_FI_PROF_NVDEC5_ACTIVE
	DCGM_FI_PROF_NVDEC5_ACTIVE Short = 1022
	// DCGM_FI_PROF_NVDEC6_ACTIVE
	DCGM_FI_PROF_NVDEC6_ACTIVE Short = 1023
	// DCGM_FI_PROF_NVDEC7_ACTIVE
	DCGM_FI_PROF_NVDEC7_ACTIVE Short = 1024
	// DCGM_FI_PROF_NVJPG0_ACTIVE represents Ratio of cycles each of the NVJPG engines are active.
	DCGM_FI_PROF_NVJPG0_ACTIVE Short = 1025
	// DCGM_FI_PROF_NVJPG1_ACTIVE
	DCGM_FI_PROF_NVJPG1_ACTIVE Short = 1026
	// DCGM_FI_PROF_NVJPG2_ACTIVE
	DCGM_FI_PROF_NVJPG2_ACTIVE Short = 1027
	// DCGM_FI_PROF_NVJPG3_ACTIVE
	DCGM_FI_PROF_NVJPG3_ACTIVE Short = 1028
	// DCGM_FI_PROF_NVJPG4_ACTIVE
	DCGM_FI_PROF_NVJPG4_ACTIVE Short = 1029
	// DCGM_FI_PROF_NVJPG5_ACTIVE
	DCGM_FI_PROF_NVJPG5_ACTIVE Short = 1030
	// DCGM_FI_PROF_NVJPG6_ACTIVE
	DCGM_FI_PROF_NVJPG6_ACTIVE Short = 1031
	// DCGM_FI_PROF_NVJPG7_ACTIVE
	DCGM_FI_PROF_NVJPG7_ACTIVE Short = 1032
	// DCGM_FI_PROF_NVOFA0_ACTIVE represents Ratio of cycles each of the NVOFA engines are active.
	DCGM_FI_PROF_NVOFA0_ACTIVE Short = 1033
	// DCGM_FI_PROF_NVOFA1_ACTIVE
	DCGM_FI_PROF_NVOFA1_ACTIVE Short = 1034
	// DCGM_FI_PROF_NVLINK_L0_TX_BYTES represents total = DCGM_FI_PROF_NVLINK_L0_TX_BYTES + DCGM_FI_PROF_NVLINK_L0_RX_BYTES
	DCGM_FI_PROF_NVLINK_L0_TX_BYTES Short = 1040
	// DCGM_FI_PROF_NVLINK_L0_RX_BYTES
	DCGM_FI_PROF_NVLINK_L0_RX_BYTES Short = 1041
	// DCGM_FI_PROF_NVLINK_L1_TX_BYTES
	DCGM_FI_PROF_NVLINK_L1_TX_BYTES Short = 1042
	// DCGM_FI_PROF_NVLINK_L1_RX_BYTES
	DCGM_FI_PROF_NVLINK_L1_RX_BYTES Short = 1043
	// DCGM_FI_PROF_NVLINK_L2_TX_BYTES
	DCGM_FI_PROF_NVLINK_L2_TX_BYTES Short = 1044
	// DCGM_FI_PROF_NVLINK_L2_RX_BYTES
	DCGM_FI_PROF_NVLINK_L2_RX_BYTES Short = 1045
	// DCGM_FI_PROF_NVLINK_L3_TX_BYTES
	DCGM_FI_PROF_NVLINK_L3_TX_BYTES Short = 1046
	// DCGM_FI_PROF_NVLINK_L3_RX_BYTES
	DCGM_FI_PROF_NVLINK_L3_RX_BYTES Short = 1047
	// DCGM_FI_PROF_NVLINK_L4_TX_BYTES
	DCGM_FI_PROF_NVLINK_L4_TX_BYTES Short = 1048
	// DCGM_FI_PROF_NVLINK_L4_RX_BYTES
	DCGM_FI_PROF_NVLINK_L4_RX_BYTES Short = 1049
	// DCGM_FI_PROF_NVLINK_L5_TX_BYTES
	DCGM_FI_PROF_NVLINK_L5_TX_BYTES Short = 1050
	// DCGM_FI_PROF_NVLINK_L5_RX_BYTES
	DCGM_FI_PROF_NVLINK_L5_RX_BYTES Short = 1051
	// DCGM_FI_PROF_NVLINK_L6_TX_BYTES
	DCGM_FI_PROF_NVLINK_L6_TX_BYTES Short = 1052
	// DCGM_FI_PROF_NVLINK_L6_RX_BYTES
	DCGM_FI_PROF_NVLINK_L6_RX_BYTES Short = 1053
	// DCGM_FI_PROF_NVLINK_L7_TX_BYTES
	DCGM_FI_PROF_NVLINK_L7_TX_BYTES Short = 1054
	// DCGM_FI_PROF_NVLINK_L7_RX_BYTES
	DCGM_FI_PROF_NVLINK_L7_RX_BYTES Short = 1055
	// DCGM_FI_PROF_NVLINK_L8_TX_BYTES
	DCGM_FI_PROF_NVLINK_L8_TX_BYTES Short = 1056
	// DCGM_FI_PROF_NVLINK_L8_RX_BYTES
	DCGM_FI_PROF_NVLINK_L8_RX_BYTES Short = 1057
	// DCGM_FI_PROF_NVLINK_L9_TX_BYTES
	DCGM_FI_PROF_NVLINK_L9_TX_BYTES Short = 1058
	// DCGM_FI_PROF_NVLINK_L9_RX_BYTES
	DCGM_FI_PROF_NVLINK_L9_RX_BYTES Short = 1059
	// DCGM_FI_PROF_NVLINK_L10_TX_BYTES
	DCGM_FI_PROF_NVLINK_L10_TX_BYTES Short = 1060
	// DCGM_FI_PROF_NVLINK_L10_RX_BYTES
	DCGM_FI_PROF_NVLINK_L10_RX_BYTES Short = 1061
	// DCGM_FI_PROF_NVLINK_L11_TX_BYTES
	DCGM_FI_PROF_NVLINK_L11_TX_BYTES Short = 1062
	// DCGM_FI_PROF_NVLINK_L11_RX_BYTES
	DCGM_FI_PROF_NVLINK_L11_RX_BYTES Short = 1063
	// DCGM_FI_PROF_NVLINK_L12_TX_BYTES
	DCGM_FI_PROF_NVLINK_L12_TX_BYTES Short = 1064
	// DCGM_FI_PROF_NVLINK_L12_RX_BYTES
	DCGM_FI_PROF_NVLINK_L12_RX_BYTES Short = 1065
	// DCGM_FI_PROF_NVLINK_L13_TX_BYTES
	DCGM_FI_PROF_NVLINK_L13_TX_BYTES Short = 1066
	// DCGM_FI_PROF_NVLINK_L13_RX_BYTES
	DCGM_FI_PROF_NVLINK_L13_RX_BYTES Short = 1067
	// DCGM_FI_PROF_NVLINK_L14_TX_BYTES
	DCGM_FI_PROF_NVLINK_L14_TX_BYTES Short = 1068
	// DCGM_FI_PROF_NVLINK_L14_RX_BYTES
	DCGM_FI_PROF_NVLINK_L14_RX_BYTES Short = 1069
	// DCGM_FI_PROF_NVLINK_L15_TX_BYTES
	DCGM_FI_PROF_NVLINK_L15_TX_BYTES Short = 1070
	// DCGM_FI_PROF_NVLINK_L15_RX_BYTES
	DCGM_FI_PROF_NVLINK_L15_RX_BYTES Short = 1071
	// DCGM_FI_PROF_NVLINK_L16_TX_BYTES
	DCGM_FI_PROF_NVLINK_L16_TX_BYTES Short = 1072
	// DCGM_FI_PROF_NVLINK_L16_RX_BYTES
	DCGM_FI_PROF_NVLINK_L16_RX_BYTES Short = 1073
	// DCGM_FI_PROF_NVLINK_L17_TX_BYTES
	DCGM_FI_PROF_NVLINK_L17_TX_BYTES Short = 1074
	// DCGM_FI_PROF_NVLINK_L17_RX_BYTES
	DCGM_FI_PROF_NVLINK_L17_RX_BYTES Short = 1075
	// DCGM_FI_PROF_C2C_TX_ALL_BYTES represents The total number of bytes transmitted over the C2C (Chip-to-Chip) interface, including both header and payload data
	DCGM_FI_PROF_C2C_TX_ALL_BYTES Short = 1076
	// DCGM_FI_PROF_C2C_TX_DATA_BYTES represents The number of data-only bytes transmitted over the C2C (Chip-to-Chip) interface
	DCGM_FI_PROF_C2C_TX_DATA_BYTES Short = 1077
	// DCGM_FI_PROF_C2C_RX_ALL_BYTES represents The total number of bytes received over the C2C (Chip-to-Chip) interface, including both header and payload data
	DCGM_FI_PROF_C2C_RX_ALL_BYTES Short = 1078
	// DCGM_FI_PROF_C2C_RX_DATA_BYTES represents The number of data-only bytes received over the C2C (Chip-to-Chip) interface
	DCGM_FI_PROF_C2C_RX_DATA_BYTES Short = 1079
	// DCGM_FI_PROF_HOSTMEM_CACHE_HIT represents Percentage of requests to Host Memory that were served from cache
	DCGM_FI_PROF_HOSTMEM_CACHE_HIT Short = 1080
	// DCGM_FI_PROF_HOSTMEM_CACHE_MISS represents Percentage of requests to Host Memory that were cache misses
	DCGM_FI_PROF_HOSTMEM_CACHE_MISS Short = 1081
	// DCGM_FI_PROF_PEERMEM_CACHE_HIT represents Percentage of requests to Peer Memory that were served from cache
	DCGM_FI_PROF_PEERMEM_CACHE_HIT Short = 1082
	// DCGM_FI_PROF_PEERMEM_CACHE_MISS represents Percentage of requests to Peer Memory that were cache misses
	DCGM_FI_PROF_PEERMEM_CACHE_MISS Short = 1083
	// DCGM_FI_DEV_CPU_UTIL_TOTAL represents CPU Utilization, total
	DCGM_FI_DEV_CPU_UTIL_TOTAL Short = 1100
	// DCGM_FI_DEV_CPU_UTIL_USER represents CPU Utilization, user
	DCGM_FI_DEV_CPU_UTIL_USER Short = 1101
	// DCGM_FI_DEV_CPU_UTIL_NICE represents CPU Utilization, nice
	DCGM_FI_DEV_CPU_UTIL_NICE Short = 1102
	// DCGM_FI_DEV_CPU_UTIL_SYS represents CPU Utilization, system time
	DCGM_FI_DEV_CPU_UTIL_SYS Short = 1103
	// DCGM_FI_DEV_CPU_UTIL_IRQ represents CPU Utilization, interrupt servicing
	DCGM_FI_DEV_CPU_UTIL_IRQ Short = 1104
	// DCGM_FI_DEV_CPU_TEMP_CURRENT represents CPU temperature
	DCGM_FI_DEV_CPU_TEMP_CURRENT Short = 1110
	// DCGM_FI_DEV_CPU_TEMP_WARNING represents CPU Warning Temperature
	DCGM_FI_DEV_CPU_TEMP_WARNING Short = 1111
	// DCGM_FI_DEV_CPU_TEMP_CRITICAL represents CPU Critical Temperature
	DCGM_FI_DEV_CPU_TEMP_CRITICAL Short = 1112
	// DCGM_FI_DEV_CPU_CLOCK_CURRENT represents CPU instantaneous clock speed
	DCGM_FI_DEV_CPU_CLOCK_CURRENT Short = 1120
	// DCGM_FI_DEV_CPU_POWER_UTIL_CURRENT represents CPU power utilization
	DCGM_FI_DEV_CPU_POWER_UTIL_CURRENT Short = 1130
	// DCGM_FI_DEV_CPU_POWER_LIMIT represents CPU power limit
	DCGM_FI_DEV_CPU_POWER_LIMIT Short = 1131
	// DCGM_FI_DEV_SYSIO_POWER_UTIL_CURRENT represents SoC power utilization
	DCGM_FI_DEV_SYSIO_POWER_UTIL_CURRENT Short = 1132
	// DCGM_FI_DEV_MODULE_POWER_UTIL_CURRENT represents Module power utilization
	DCGM_FI_DEV_MODULE_POWER_UTIL_CURRENT Short = 1133
	// DCGM_FI_DEV_CPU_VENDOR represents CPU vendor name
	DCGM_FI_DEV_CPU_VENDOR Short = 1140
	// DCGM_FI_DEV_CPU_MODEL represents CPU model name
	DCGM_FI_DEV_CPU_MODEL Short = 1141
	// DCGM_FI_DEV_NVLINK_COUNT_TX_PACKETS represents Note: NVLink5+ only. Returns aggregate value across all links. Not supported on NVLink4 and earlier.
	DCGM_FI_DEV_NVLINK_COUNT_TX_PACKETS Short = 1200
	// DCGM_FI_DEV_NVLINK_COUNT_TX_BYTES represents Note: NVLink5+ only. Returns aggregate value across all links. Not supported on NVLink4 and earlier.
	DCGM_FI_DEV_NVLINK_COUNT_TX_BYTES Short = 1201
	// DCGM_FI_DEV_NVLINK_COUNT_RX_PACKETS represents Note: NVLink5+ only. Returns aggregate value across all links. Not supported on NVLink4 and earlier.
	DCGM_FI_DEV_NVLINK_COUNT_RX_PACKETS Short = 1202
	// DCGM_FI_DEV_NVLINK_COUNT_RX_BYTES represents Note: NVLink5+ only. Returns aggregate value across all links. Not supported on NVLink4 and earlier.
	DCGM_FI_DEV_NVLINK_COUNT_RX_BYTES Short = 1203
	// DCGM_FI_DEV_NVLINK_COUNT_RX_MALFORMED_PACKET_ERRORS represents Note: NVLink5+ only. Returns aggregate value across all links. Not supported on NVLink4 and earlier.
	DCGM_FI_DEV_NVLINK_COUNT_RX_MALFORMED_PACKET_ERRORS Short = 1204
	// DCGM_FI_DEV_NVLINK_COUNT_RX_BUFFER_OVERRUN_ERRORS represents Note: NVLink5+ only. Returns aggregate value across all links. Not supported on NVLink4 and earlier.
	DCGM_FI_DEV_NVLINK_COUNT_RX_BUFFER_OVERRUN_ERRORS Short = 1205
	// DCGM_FI_DEV_NVLINK_COUNT_RX_ERRORS represents Note: NVLink5+ only. Returns aggregate value across all links. Not supported on NVLink4 and earlier.
	DCGM_FI_DEV_NVLINK_COUNT_RX_ERRORS Short = 1206
	// DCGM_FI_DEV_NVLINK_COUNT_RX_REMOTE_ERRORS represents Note: NVLink5+ only. Returns aggregate value across all links. Not supported on NVLink4 and earlier.
	DCGM_FI_DEV_NVLINK_COUNT_RX_REMOTE_ERRORS Short = 1207
	// DCGM_FI_DEV_NVLINK_COUNT_RX_GENERAL_ERRORS represents Note: NVLink5+ only. Returns aggregate value across all links. Not supported on NVLink4 and earlier.
	DCGM_FI_DEV_NVLINK_COUNT_RX_GENERAL_ERRORS Short = 1208
	// DCGM_FI_DEV_NVLINK_COUNT_LOCAL_LINK_INTEGRITY_ERRORS represents Note: NVLink5+ only. Returns aggregate value across all links. Not supported on NVLink4 and earlier.
	DCGM_FI_DEV_NVLINK_COUNT_LOCAL_LINK_INTEGRITY_ERRORS Short = 1209
	// DCGM_FI_DEV_NVLINK_COUNT_TX_DISCARDS represents Note: NVLink5+ only. Returns aggregate value across all links. Not supported on NVLink4 and earlier.
	DCGM_FI_DEV_NVLINK_COUNT_TX_DISCARDS Short = 1210
	// DCGM_FI_DEV_NVLINK_COUNT_LINK_RECOVERY_SUCCESSFUL_EVENTS represents Note: NVLink5+ only. Returns aggregate value across all links. Not supported on NVLink4 and earlier.
	DCGM_FI_DEV_NVLINK_COUNT_LINK_RECOVERY_SUCCESSFUL_EVENTS Short = 1211
	// DCGM_FI_DEV_NVLINK_COUNT_LINK_RECOVERY_FAILED_EVENTS represents Note: NVLink5+ only. Returns aggregate value across all links. Not supported on NVLink4 and earlier.
	DCGM_FI_DEV_NVLINK_COUNT_LINK_RECOVERY_FAILED_EVENTS Short = 1212
	// DCGM_FI_DEV_NVLINK_COUNT_LINK_RECOVERY_EVENTS represents Note: NVLink5+ only. Returns aggregate value across all links. Not supported on NVLink4 and earlier.
	DCGM_FI_DEV_NVLINK_COUNT_LINK_RECOVERY_EVENTS Short = 1213
	// DCGM_FI_DEV_NVLINK_COUNT_RX_SYMBOL_ERRORS represents Note: NVLink5+ only. Returns aggregate value across all links. Not supported on NVLink4 and earlier.
	DCGM_FI_DEV_NVLINK_COUNT_RX_SYMBOL_ERRORS Short = 1214
	// DCGM_FI_DEV_NVLINK_COUNT_SYMBOL_BER represents Note: NVLink5+ only. Returns aggregate value across all links. Not supported on NVLink4 and earlier.
	DCGM_FI_DEV_NVLINK_COUNT_SYMBOL_BER Short = 1215
	// DCGM_FI_DEV_NVLINK_COUNT_SYMBOL_BER_FLOAT represents Note: NVLink5+ only. Returns aggregate value across all links. Not supported on NVLink4 and earlier.
	DCGM_FI_DEV_NVLINK_COUNT_SYMBOL_BER_FLOAT Short = 1216
	// DCGM_FI_DEV_NVLINK_COUNT_EFFECTIVE_BER represents Note: NVLink5+ only. Returns aggregate value across all links. Not supported on NVLink4 and earlier.
	DCGM_FI_DEV_NVLINK_COUNT_EFFECTIVE_BER Short = 1217
	// DCGM_FI_DEV_NVLINK_COUNT_EFFECTIVE_BER_FLOAT represents Note: NVLink5+ only. Returns aggregate value across all links. Not supported on NVLink4 and earlier.
	DCGM_FI_DEV_NVLINK_COUNT_EFFECTIVE_BER_FLOAT Short = 1218
	// DCGM_FI_DEV_NVLINK_COUNT_EFFECTIVE_ERRORS represents Note: NVLink5+ only. Returns aggregate value across all links. Not supported on NVLink4 and earlier.
	DCGM_FI_DEV_NVLINK_COUNT_EFFECTIVE_ERRORS Short = 1219
	// DCGM_FI_DEV_NVLINK_ECC_DATA_ERROR_COUNT_TOTAL represents NVLink ECC Data Error Counter total for all Links
	DCGM_FI_DEV_NVLINK_ECC_DATA_ERROR_COUNT_TOTAL Short = 1220
	// DCGM_FI_DEV_FIRST_CONNECTX_FIELD_ID represents First field id of ConnectX
	DCGM_FI_DEV_FIRST_CONNECTX_FIELD_ID Short = 1300
	// DCGM_FI_DEV_CONNECTX_HEALTH represents Health state of ConnectX
	DCGM_FI_DEV_CONNECTX_HEALTH Short = 1300
	// DCGM_FI_DEV_CONNECTX_ACTIVE_PCIE_LINK_WIDTH represents Active PCIe link width
	DCGM_FI_DEV_CONNECTX_ACTIVE_PCIE_LINK_WIDTH Short = 1301
	// DCGM_FI_DEV_CONNECTX_ACTIVE_PCIE_LINK_SPEED represents Active PCIe link speed
	DCGM_FI_DEV_CONNECTX_ACTIVE_PCIE_LINK_SPEED Short = 1302
	// DCGM_FI_DEV_CONNECTX_EXPECT_PCIE_LINK_WIDTH represents Expect PCIe link width
	DCGM_FI_DEV_CONNECTX_EXPECT_PCIE_LINK_WIDTH Short = 1303
	// DCGM_FI_DEV_CONNECTX_EXPECT_PCIE_LINK_SPEED represents Expect PCIe link speed
	DCGM_FI_DEV_CONNECTX_EXPECT_PCIE_LINK_SPEED Short = 1304
	// DCGM_FI_DEV_CONNECTX_CORRECTABLE_ERR_STATUS represents Correctable error status
	DCGM_FI_DEV_CONNECTX_CORRECTABLE_ERR_STATUS Short = 1305
	// DCGM_FI_DEV_CONNECTX_CORRECTABLE_ERR_MASK represents Correctable error mask
	DCGM_FI_DEV_CONNECTX_CORRECTABLE_ERR_MASK Short = 1306
	// DCGM_FI_DEV_CONNECTX_UNCORRECTABLE_ERR_STATUS represents Uncorrectable error status
	DCGM_FI_DEV_CONNECTX_UNCORRECTABLE_ERR_STATUS Short = 1307
	// DCGM_FI_DEV_CONNECTX_UNCORRECTABLE_ERR_MASK represents Uncorrectable error mask
	DCGM_FI_DEV_CONNECTX_UNCORRECTABLE_ERR_MASK Short = 1308
	// DCGM_FI_DEV_CONNECTX_UNCORRECTABLE_ERR_SEVERITY represents Uncorrectable error severity
	DCGM_FI_DEV_CONNECTX_UNCORRECTABLE_ERR_SEVERITY Short = 1309
	// DCGM_FI_DEV_CONNECTX_DEVICE_TEMPERATURE represents Device temperature
	DCGM_FI_DEV_CONNECTX_DEVICE_TEMPERATURE Short = 1310
	// DCGM_FI_DEV_LAST_CONNECTX_FIELD_ID represents The last field id of ConnectX
	DCGM_FI_DEV_LAST_CONNECTX_FIELD_ID Short = 1399
	// DCGM_FI_DEV_C2C_LINK_ERROR_INTR represents C2C Link CRC Error Counter
	DCGM_FI_DEV_C2C_LINK_ERROR_INTR Short = 1400
	// DCGM_FI_DEV_C2C_LINK_ERROR_REPLAY represents C2C Link Replay Error Counter
	DCGM_FI_DEV_C2C_LINK_ERROR_REPLAY Short = 1401
	// DCGM_FI_DEV_C2C_LINK_ERROR_REPLAY_B2B represents C2C Link Back to Back Replay Error Counter
	DCGM_FI_DEV_C2C_LINK_ERROR_REPLAY_B2B Short = 1402
	// DCGM_FI_DEV_C2C_LINK_POWER_STATE represents C2C Link Power state. See NVML_C2C_POWER_STATE_*
	DCGM_FI_DEV_C2C_LINK_POWER_STATE Short = 1403
	// DCGM_FI_DEV_NVLINK_COUNT_FEC_HISTORY_0 represents Note: NVLink5+ only. Returns aggregate value across all links. Not supported on NVLink4 and earlier.
	DCGM_FI_DEV_NVLINK_COUNT_FEC_HISTORY_0 Short = 1404
	// DCGM_FI_DEV_NVLINK_COUNT_FEC_HISTORY_1 represents Note: NVLink5+ only. Returns aggregate value across all links. Not supported on NVLink4 and earlier.
	DCGM_FI_DEV_NVLINK_COUNT_FEC_HISTORY_1 Short = 1405
	// DCGM_FI_DEV_NVLINK_COUNT_FEC_HISTORY_2 represents Note: NVLink5+ only. Returns aggregate value across all links. Not supported on NVLink4 and earlier.
	DCGM_FI_DEV_NVLINK_COUNT_FEC_HISTORY_2 Short = 1406
	// DCGM_FI_DEV_NVLINK_COUNT_FEC_HISTORY_3 represents Note: NVLink5+ only. Returns aggregate value across all links. Not supported on NVLink4 and earlier.
	DCGM_FI_DEV_NVLINK_COUNT_FEC_HISTORY_3 Short = 1407
	// DCGM_FI_DEV_NVLINK_COUNT_FEC_HISTORY_4 represents Note: NVLink5+ only. Returns aggregate value across all links. Not supported on NVLink4 and earlier.
	DCGM_FI_DEV_NVLINK_COUNT_FEC_HISTORY_4 Short = 1408
	// DCGM_FI_DEV_NVLINK_COUNT_FEC_HISTORY_5 represents Note: NVLink5+ only. Returns aggregate value across all links. Not supported on NVLink4 and earlier.
	DCGM_FI_DEV_NVLINK_COUNT_FEC_HISTORY_5 Short = 1409
	// DCGM_FI_DEV_NVLINK_COUNT_FEC_HISTORY_6 represents Note: NVLink5+ only. Returns aggregate value across all links. Not supported on NVLink4 and earlier.
	DCGM_FI_DEV_NVLINK_COUNT_FEC_HISTORY_6 Short = 1410
	// DCGM_FI_DEV_NVLINK_COUNT_FEC_HISTORY_7 represents Note: NVLink5+ only. Returns aggregate value across all links. Not supported on NVLink4 and earlier.
	DCGM_FI_DEV_NVLINK_COUNT_FEC_HISTORY_7 Short = 1411
	// DCGM_FI_DEV_NVLINK_COUNT_FEC_HISTORY_8 represents Note: NVLink5+ only. Returns aggregate value across all links. Not supported on NVLink4 and earlier.
	DCGM_FI_DEV_NVLINK_COUNT_FEC_HISTORY_8 Short = 1412
	// DCGM_FI_DEV_NVLINK_COUNT_FEC_HISTORY_9 represents Note: NVLink5+ only. Returns aggregate value across all links. Not supported on NVLink4 and earlier.
	DCGM_FI_DEV_NVLINK_COUNT_FEC_HISTORY_9 Short = 1413
	// DCGM_FI_DEV_NVLINK_COUNT_FEC_HISTORY_10 represents Note: NVLink5+ only. Returns aggregate value across all links. Not supported on NVLink4 and earlier.
	DCGM_FI_DEV_NVLINK_COUNT_FEC_HISTORY_10 Short = 1414
	// DCGM_FI_DEV_NVLINK_COUNT_FEC_HISTORY_11 represents Note: NVLink5+ only. Returns aggregate value across all links. Not supported on NVLink4 and earlier.
	DCGM_FI_DEV_NVLINK_COUNT_FEC_HISTORY_11 Short = 1415
	// DCGM_FI_DEV_NVLINK_COUNT_FEC_HISTORY_12 represents Note: NVLink5+ only. Returns aggregate value across all links. Not supported on NVLink4 and earlier.
	DCGM_FI_DEV_NVLINK_COUNT_FEC_HISTORY_12 Short = 1416
	// DCGM_FI_DEV_NVLINK_COUNT_FEC_HISTORY_13 represents Note: NVLink5+ only. Returns aggregate value across all links. Not supported on NVLink4 and earlier.
	DCGM_FI_DEV_NVLINK_COUNT_FEC_HISTORY_13 Short = 1417
	// DCGM_FI_DEV_NVLINK_COUNT_FEC_HISTORY_14 represents Note: NVLink5+ only. Returns aggregate value across all links. Not supported on NVLink4 and earlier.
	DCGM_FI_DEV_NVLINK_COUNT_FEC_HISTORY_14 Short = 1418
	// DCGM_FI_DEV_NVLINK_COUNT_FEC_HISTORY_15 represents Note: NVLink5+ only. Returns aggregate value across all links. Not supported on NVLink4 and earlier.
	DCGM_FI_DEV_NVLINK_COUNT_FEC_HISTORY_15 Short = 1419
	// DCGM_FI_DEV_CLOCKS_EVENT_REASON_SW_POWER_CAP_NS represents Throttling to not exceed currently set power limits in ns
	DCGM_FI_DEV_CLOCKS_EVENT_REASON_SW_POWER_CAP_NS Short = 1420
	// DCGM_FI_DEV_CLOCKS_EVENT_REASON_SYNC_BOOST_NS represents Boost Group in ns
	DCGM_FI_DEV_CLOCKS_EVENT_REASON_SYNC_BOOST_NS Short = 1421
	// DCGM_FI_DEV_CLOCKS_EVENT_REASON_SW_THERM_SLOWDOWN_NS represents (Memory Temp < Memory Max Operating Temp)) in ns
	DCGM_FI_DEV_CLOCKS_EVENT_REASON_SW_THERM_SLOWDOWN_NS Short = 1422
	// DCGM_FI_DEV_CLOCKS_EVENT_REASON_HW_THERM_SLOWDOWN_NS represents clocks by a factor of 2 or more) in ns
	DCGM_FI_DEV_CLOCKS_EVENT_REASON_HW_THERM_SLOWDOWN_NS Short = 1423
	// DCGM_FI_DEV_CLOCKS_EVENT_REASON_HW_POWER_BRAKE_SLOWDOWN_NS represents (reducing core clocks by a factor of 2 or more) in ns
	DCGM_FI_DEV_CLOCKS_EVENT_REASON_HW_POWER_BRAKE_SLOWDOWN_NS Short = 1424
	// DCGM_FI_DEV_PWR_SMOOTHING_ENABLED represents either level 1 or level 2 (e.g. via Redfish API)
	DCGM_FI_DEV_PWR_SMOOTHING_ENABLED Short = 1425
	// DCGM_FI_DEV_PWR_SMOOTHING_PRIV_LVL represents either level 1 or level 2 (e.g. via Redfish API)
	DCGM_FI_DEV_PWR_SMOOTHING_PRIV_LVL Short = 1426
	// DCGM_FI_DEV_PWR_SMOOTHING_IMM_RAMP_DOWN_ENABLED represents either level 1 or level 2 (e.g. via Redfish API)
	DCGM_FI_DEV_PWR_SMOOTHING_IMM_RAMP_DOWN_ENABLED Short = 1427
	// DCGM_FI_DEV_PWR_SMOOTHING_APPLIED_TMP_CEIL represents either level 1 or level 2 (e.g. via Redfish API)
	DCGM_FI_DEV_PWR_SMOOTHING_APPLIED_TMP_CEIL Short = 1428
	// DCGM_FI_DEV_PWR_SMOOTHING_APPLIED_TMP_FLOOR represents either level 1 or level 2 (e.g. via Redfish API)
	DCGM_FI_DEV_PWR_SMOOTHING_APPLIED_TMP_FLOOR Short = 1429
	// DCGM_FI_DEV_PWR_SMOOTHING_MAX_PERCENT_TMP_FLOOR_SETTING represents either level 1 or level 2 (e.g. via Redfish API)
	DCGM_FI_DEV_PWR_SMOOTHING_MAX_PERCENT_TMP_FLOOR_SETTING Short = 1430
	// DCGM_FI_DEV_PWR_SMOOTHING_MIN_PERCENT_TMP_FLOOR_SETTING represents either level 1 or level 2 (e.g. via Redfish API)
	DCGM_FI_DEV_PWR_SMOOTHING_MIN_PERCENT_TMP_FLOOR_SETTING Short = 1431
	// DCGM_FI_DEV_PWR_SMOOTHING_HW_CIRCUITRY_PERCENT_LIFETIME_REMAINING represents either level 1 or level 2 (e.g. via Redfish API)
	DCGM_FI_DEV_PWR_SMOOTHING_HW_CIRCUITRY_PERCENT_LIFETIME_REMAINING Short = 1432
	// DCGM_FI_DEV_PWR_SMOOTHING_MAX_NUM_PRESET_PROFILES represents either level 1 or level 2 (e.g. via Redfish API)
	DCGM_FI_DEV_PWR_SMOOTHING_MAX_NUM_PRESET_PROFILES Short = 1433
	// DCGM_FI_DEV_PWR_SMOOTHING_PROFILE_PERCENT_TMP_FLOOR represents either level 1 or level 2 (e.g. via Redfish API)
	DCGM_FI_DEV_PWR_SMOOTHING_PROFILE_PERCENT_TMP_FLOOR Short = 1434
	// DCGM_FI_DEV_PWR_SMOOTHING_PROFILE_RAMP_UP_RATE represents either level 1 or level 2 (e.g. via Redfish API)
	DCGM_FI_DEV_PWR_SMOOTHING_PROFILE_RAMP_UP_RATE Short = 1435
	// DCGM_FI_DEV_PWR_SMOOTHING_PROFILE_RAMP_DOWN_RATE represents either level 1 or level 2 (e.g. via Redfish API)
	DCGM_FI_DEV_PWR_SMOOTHING_PROFILE_RAMP_DOWN_RATE Short = 1436
	// DCGM_FI_DEV_PWR_SMOOTHING_PROFILE_RAMP_DOWN_HYST_VAL represents either level 1 or level 2 (e.g. via Redfish API)
	DCGM_FI_DEV_PWR_SMOOTHING_PROFILE_RAMP_DOWN_HYST_VAL Short = 1437
	// DCGM_FI_DEV_PWR_SMOOTHING_ACTIVE_PRESET_PROFILE represents either level 1 or level 2 (e.g. via Redfish API)
	DCGM_FI_DEV_PWR_SMOOTHING_ACTIVE_PRESET_PROFILE Short = 1438
	// DCGM_FI_DEV_PWR_SMOOTHING_ADMIN_OVERRIDE_PERCENT_TMP_FLOOR represents either level 1 or level 2 (e.g. via Redfish API)
	DCGM_FI_DEV_PWR_SMOOTHING_ADMIN_OVERRIDE_PERCENT_TMP_FLOOR Short = 1439
	// DCGM_FI_DEV_PWR_SMOOTHING_ADMIN_OVERRIDE_RAMP_UP_RATE represents either level 1 or level 2 (e.g. via Redfish API)
	DCGM_FI_DEV_PWR_SMOOTHING_ADMIN_OVERRIDE_RAMP_UP_RATE Short = 1440
	// DCGM_FI_DEV_PWR_SMOOTHING_ADMIN_OVERRIDE_RAMP_DOWN_RATE represents either level 1 or level 2 (e.g. via Redfish API)
	DCGM_FI_DEV_PWR_SMOOTHING_ADMIN_OVERRIDE_RAMP_DOWN_RATE Short = 1441
	// DCGM_FI_DEV_PWR_SMOOTHING_ADMIN_OVERRIDE_RAMP_DOWN_HYST_VAL represents either level 1 or level 2 (e.g. via Redfish API)
	DCGM_FI_DEV_PWR_SMOOTHING_ADMIN_OVERRIDE_RAMP_DOWN_HYST_VAL Short = 1442
	// DCGM_FI_DEV_PCIE_COUNT_CORRECTABLE_ERRORS represents PCIe Correctable Errors Counter
	DCGM_FI_DEV_PCIE_COUNT_CORRECTABLE_ERRORS Short = 1501
	// DCGM_FI_IMEX_DOMAIN_STATUS represents Retrieved from nvidia-imex-ctl -N -j command
	DCGM_FI_IMEX_DOMAIN_STATUS Short = 1502
	// DCGM_FI_IMEX_DAEMON_STATUS represents WAITING_FOR_RECOVERY=3, INIT_GPU=4, READY=5, SHUTTING_DOWN=6, UNAVAILABLE=7
	DCGM_FI_IMEX_DAEMON_STATUS Short = 1503
	// DCGM_FI_DEV_MEMORY_UNREPAIRABLE_FLAG represents 1=yes, 0=no
	DCGM_FI_DEV_MEMORY_UNREPAIRABLE_FLAG Short = 1507
	// DCGM_FI_DEV_NVLINK_GET_STATE represents Use DCGM_FE_LINK entity group when accessing this field.
	DCGM_FI_DEV_NVLINK_GET_STATE Short = 1508
	// DCGM_FI_DEV_NVLINK_PPCNT_IBPC_PORT_XMIT_WAIT represents Use DCGM_FE_LINK entity group when accessing this field.
	DCGM_FI_DEV_NVLINK_PPCNT_IBPC_PORT_XMIT_WAIT Short = 1509
	// DCGM_FI_DEV_GET_GPU_RECOVERY_ACTION represents GPU Recovery Action (see nvmlDeviceGpuRecoveryAction_t for return values)
	DCGM_FI_DEV_GET_GPU_RECOVERY_ACTION Short = 1523
	// DCGM_FI_DEV_NVSWITCH_FIRMWARE_VERSION represents NVSwitch firmware version string
	DCGM_FI_DEV_NVSWITCH_FIRMWARE_VERSION Short = 1524
)

// dcgmFields maps field names to their IDs
var dcgmFields = map[string]Short{
	"DCGM_FI_UNKNOWN":                                                   0,
	"DCGM_FI_DRIVER_VERSION":                                            1,
	"DCGM_FI_NVML_VERSION":                                              2,
	"DCGM_FI_PROCESS_NAME":                                              3,
	"DCGM_FI_DEV_COUNT":                                                 4,
	"DCGM_FI_CUDA_DRIVER_VERSION":                                       5,
	"DCGM_FI_BIND_UNBIND_EVENT":                                         6,
	"DCGM_FI_DEV_NAME":                                                  50,
	"DCGM_FI_DEV_BRAND":                                                 51,
	"DCGM_FI_DEV_NVML_INDEX":                                            52,
	"DCGM_FI_DEV_SERIAL":                                                53,
	"DCGM_FI_DEV_UUID":                                                  54,
	"DCGM_FI_DEV_MINOR_NUMBER":                                          55,
	"DCGM_FI_DEV_OEM_INFOROM_VER":                                       56,
	"DCGM_FI_DEV_PCI_BUSID":                                             57,
	"DCGM_FI_DEV_PCI_COMBINED_ID":                                       58,
	"DCGM_FI_DEV_PCI_SUBSYS_ID":                                         59,
	"DCGM_FI_GPU_TOPOLOGY_PCI":                                          60,
	"DCGM_FI_GPU_TOPOLOGY_NVLINK":                                       61,
	"DCGM_FI_GPU_TOPOLOGY_AFFINITY":                                     62,
	"DCGM_FI_DEV_CUDA_COMPUTE_CAPABILITY":                               63,
	"DCGM_FI_DEV_P2P_NVLINK_STATUS":                                     64,
	"DCGM_FI_DEV_COMPUTE_MODE":                                          65,
	"DCGM_FI_DEV_PERSISTENCE_MODE":                                      66,
	"DCGM_FI_DEV_MIG_MODE":                                              67,
	"DCGM_FI_DEV_CUDA_VISIBLE_DEVICES_STR":                              68,
	"DCGM_FI_DEV_MIG_MAX_SLICES":                                        69,
	"DCGM_FI_DEV_CPU_AFFINITY_0":                                        70,
	"DCGM_FI_DEV_CPU_AFFINITY_1":                                        71,
	"DCGM_FI_DEV_CPU_AFFINITY_2":                                        72,
	"DCGM_FI_DEV_CPU_AFFINITY_3":                                        73,
	"DCGM_FI_DEV_CC_MODE":                                               74,
	"DCGM_FI_DEV_MIG_ATTRIBUTES":                                        75,
	"DCGM_FI_DEV_MIG_GI_INFO":                                           76,
	"DCGM_FI_DEV_MIG_CI_INFO":                                           77,
	"DCGM_FI_DEV_ECC_INFOROM_VER":                                       80,
	"DCGM_FI_DEV_POWER_INFOROM_VER":                                     81,
	"DCGM_FI_DEV_INFOROM_IMAGE_VER":                                     82,
	"DCGM_FI_DEV_INFOROM_CONFIG_CHECK":                                  83,
	"DCGM_FI_DEV_INFOROM_CONFIG_VALID":                                  84,
	"DCGM_FI_DEV_VBIOS_VERSION":                                         85,
	"DCGM_FI_DEV_MEM_AFFINITY_0":                                        86,
	"DCGM_FI_DEV_MEM_AFFINITY_1":                                        87,
	"DCGM_FI_DEV_MEM_AFFINITY_2":                                        88,
	"DCGM_FI_DEV_MEM_AFFINITY_3":                                        89,
	"DCGM_FI_DEV_BAR1_TOTAL":                                            90,
	"DCGM_FI_SYNC_BOOST":                                                91,
	"DCGM_FI_DEV_BAR1_USED":                                             92,
	"DCGM_FI_DEV_BAR1_FREE":                                             93,
	"DCGM_FI_DEV_GPM_SUPPORT":                                           94,
	"DCGM_FI_DEV_SM_CLOCK":                                              100,
	"DCGM_FI_DEV_MEM_CLOCK":                                             101,
	"DCGM_FI_DEV_VIDEO_CLOCK":                                           102,
	"DCGM_FI_DEV_APP_SM_CLOCK":                                          110,
	"DCGM_FI_DEV_APP_MEM_CLOCK":                                         111,
	"DCGM_FI_DEV_CLOCKS_EVENT_REASONS":                                  112,
	"DCGM_FI_DEV_MAX_SM_CLOCK":                                          113,
	"DCGM_FI_DEV_MAX_MEM_CLOCK":                                         114,
	"DCGM_FI_DEV_MAX_VIDEO_CLOCK":                                       115,
	"DCGM_FI_DEV_AUTOBOOST":                                             120,
	"DCGM_FI_DEV_SUPPORTED_CLOCKS":                                      130,
	"DCGM_FI_DEV_MEMORY_TEMP":                                           140,
	"DCGM_FI_DEV_GPU_TEMP":                                              150,
	"DCGM_FI_DEV_MEM_MAX_OP_TEMP":                                       151,
	"DCGM_FI_DEV_GPU_MAX_OP_TEMP":                                       152,
	"DCGM_FI_DEV_GPU_TEMP_LIMIT":                                        153,
	"DCGM_FI_DEV_POWER_USAGE":                                           155,
	"DCGM_FI_DEV_TOTAL_ENERGY_CONSUMPTION":                              156,
	"DCGM_FI_DEV_POWER_USAGE_INSTANT":                                   157,
	"DCGM_FI_DEV_SLOWDOWN_TEMP":                                         158,
	"DCGM_FI_DEV_SHUTDOWN_TEMP":                                         159,
	"DCGM_FI_DEV_POWER_MGMT_LIMIT":                                      160,
	"DCGM_FI_DEV_POWER_MGMT_LIMIT_MIN":                                  161,
	"DCGM_FI_DEV_POWER_MGMT_LIMIT_MAX":                                  162,
	"DCGM_FI_DEV_POWER_MGMT_LIMIT_DEF":                                  163,
	"DCGM_FI_DEV_ENFORCED_POWER_LIMIT":                                  164,
	"DCGM_FI_DEV_REQUESTED_POWER_PROFILE_MASK":                          165,
	"DCGM_FI_DEV_ENFORCED_POWER_PROFILE_MASK":                           166,
	"DCGM_FI_DEV_VALID_POWER_PROFILE_MASK":                              167,
	"DCGM_FI_DEV_FABRIC_MANAGER_STATUS":                                 170,
	"DCGM_FI_DEV_FABRIC_MANAGER_ERROR_CODE":                             171,
	"DCGM_FI_DEV_FABRIC_CLUSTER_UUID":                                   172,
	"DCGM_FI_DEV_FABRIC_CLIQUE_ID":                                      173,
	"DCGM_FI_DEV_FABRIC_HEALTH_MASK":                                    174,
	"DCGM_FI_DEV_FABRIC_HEALTH_SUMMARY":                                 175,
	"DCGM_FI_DEV_PSTATE":                                                190,
	"DCGM_FI_DEV_FAN_SPEED":                                             191,
	"DCGM_FI_DEV_PCIE_TX_THROUGHPUT":                                    200,
	"DCGM_FI_DEV_PCIE_RX_THROUGHPUT":                                    201,
	"DCGM_FI_DEV_PCIE_REPLAY_COUNTER":                                   202,
	"DCGM_FI_DEV_GPU_UTIL":                                              203,
	"DCGM_FI_DEV_MEM_COPY_UTIL":                                         204,
	"DCGM_FI_DEV_ACCOUNTING_DATA":                                       205,
	"DCGM_FI_DEV_ENC_UTIL":                                              206,
	"DCGM_FI_DEV_DEC_UTIL":                                              207,
	"DCGM_FI_DEV_XID_ERRORS":                                            230,
	"DCGM_FI_DEV_PCIE_MAX_LINK_GEN":                                     235,
	"DCGM_FI_DEV_PCIE_MAX_LINK_WIDTH":                                   236,
	"DCGM_FI_DEV_PCIE_LINK_GEN":                                         237,
	"DCGM_FI_DEV_PCIE_LINK_WIDTH":                                       238,
	"DCGM_FI_DEV_POWER_VIOLATION":                                       240,
	"DCGM_FI_DEV_THERMAL_VIOLATION":                                     241,
	"DCGM_FI_DEV_SYNC_BOOST_VIOLATION":                                  242,
	"DCGM_FI_DEV_BOARD_LIMIT_VIOLATION":                                 243,
	"DCGM_FI_DEV_LOW_UTIL_VIOLATION":                                    244,
	"DCGM_FI_DEV_RELIABILITY_VIOLATION":                                 245,
	"DCGM_FI_DEV_TOTAL_APP_CLOCKS_VIOLATION":                            246,
	"DCGM_FI_DEV_TOTAL_BASE_CLOCKS_VIOLATION":                           247,
	"DCGM_FI_DEV_FB_TOTAL":                                              250,
	"DCGM_FI_DEV_FB_FREE":                                               251,
	"DCGM_FI_DEV_FB_USED":                                               252,
	"DCGM_FI_DEV_FB_RESERVED":                                           253,
	"DCGM_FI_DEV_FB_USED_PERCENT":                                       254,
	"DCGM_FI_DEV_C2C_LINK_COUNT":                                        285,
	"DCGM_FI_DEV_C2C_LINK_STATUS":                                       286,
	"DCGM_FI_DEV_C2C_MAX_BANDWIDTH":                                     287,
	"DCGM_FI_DEV_ECC_CURRENT":                                           300,
	"DCGM_FI_DEV_ECC_PENDING":                                           301,
	"DCGM_FI_DEV_ECC_SBE_VOL_TOTAL":                                     310,
	"DCGM_FI_DEV_ECC_DBE_VOL_TOTAL":                                     311,
	"DCGM_FI_DEV_ECC_SBE_AGG_TOTAL":                                     312,
	"DCGM_FI_DEV_ECC_DBE_AGG_TOTAL":                                     313,
	"DCGM_FI_DEV_ECC_SBE_VOL_L1":                                        314,
	"DCGM_FI_DEV_ECC_DBE_VOL_L1":                                        315,
	"DCGM_FI_DEV_ECC_SBE_VOL_L2":                                        316,
	"DCGM_FI_DEV_ECC_DBE_VOL_L2":                                        317,
	"DCGM_FI_DEV_ECC_SBE_VOL_DEV":                                       318,
	"DCGM_FI_DEV_ECC_DBE_VOL_DEV":                                       319,
	"DCGM_FI_DEV_ECC_SBE_VOL_REG":                                       320,
	"DCGM_FI_DEV_ECC_DBE_VOL_REG":                                       321,
	"DCGM_FI_DEV_ECC_SBE_VOL_TEX":                                       322,
	"DCGM_FI_DEV_ECC_DBE_VOL_TEX":                                       323,
	"DCGM_FI_DEV_ECC_SBE_AGG_L1":                                        324,
	"DCGM_FI_DEV_ECC_DBE_AGG_L1":                                        325,
	"DCGM_FI_DEV_ECC_SBE_AGG_L2":                                        326,
	"DCGM_FI_DEV_ECC_DBE_AGG_L2":                                        327,
	"DCGM_FI_DEV_ECC_SBE_AGG_DEV":                                       328,
	"DCGM_FI_DEV_ECC_DBE_AGG_DEV":                                       329,
	"DCGM_FI_DEV_ECC_SBE_AGG_REG":                                       330,
	"DCGM_FI_DEV_ECC_DBE_AGG_REG":                                       331,
	"DCGM_FI_DEV_ECC_SBE_AGG_TEX":                                       332,
	"DCGM_FI_DEV_ECC_DBE_AGG_TEX":                                       333,
	"DCGM_FI_DEV_ECC_SBE_VOL_SHM":                                       334,
	"DCGM_FI_DEV_ECC_DBE_VOL_SHM":                                       335,
	"DCGM_FI_DEV_ECC_SBE_VOL_CBU":                                       336,
	"DCGM_FI_DEV_ECC_DBE_VOL_CBU":                                       337,
	"DCGM_FI_DEV_ECC_SBE_AGG_SHM":                                       338,
	"DCGM_FI_DEV_ECC_DBE_AGG_SHM":                                       339,
	"DCGM_FI_DEV_ECC_SBE_AGG_CBU":                                       340,
	"DCGM_FI_DEV_ECC_DBE_AGG_CBU":                                       341,
	"DCGM_FI_DEV_ECC_SBE_VOL_SRM":                                       342,
	"DCGM_FI_DEV_ECC_DBE_VOL_SRM":                                       343,
	"DCGM_FI_DEV_ECC_SBE_AGG_SRM":                                       344,
	"DCGM_FI_DEV_ECC_DBE_AGG_SRM":                                       345,
	"DCGM_FI_DEV_THRESHOLD_SRM":                                         346,
	"DCGM_FI_DEV_DIAG_MEMORY_RESULT":                                    350,
	"DCGM_FI_DEV_DIAG_DIAGNOSTIC_RESULT":                                351,
	"DCGM_FI_DEV_DIAG_PCIE_RESULT":                                      352,
	"DCGM_FI_DEV_DIAG_TARGETED_STRESS_RESULT":                           353,
	"DCGM_FI_DEV_DIAG_TARGETED_POWER_RESULT":                            354,
	"DCGM_FI_DEV_DIAG_MEMORY_BANDWIDTH_RESULT":                          355,
	"DCGM_FI_DEV_DIAG_MEMTEST_RESULT":                                   356,
	"DCGM_FI_DEV_DIAG_PULSE_TEST_RESULT":                                357,
	"DCGM_FI_DEV_DIAG_EUD_RESULT":                                       358,
	"DCGM_FI_DEV_DIAG_CPU_EUD_RESULT":                                   359,
	"DCGM_FI_DEV_DIAG_SOFTWARE_RESULT":                                  360,
	"DCGM_FI_DEV_DIAG_NVBANDWIDTH_RESULT":                               361,
	"DCGM_FI_DEV_DIAG_STATUS":                                           362,
	"DCGM_FI_DEV_DIAG_NCCL_TESTS_RESULT":                                363,
	"DCGM_FI_DEV_BANKS_REMAP_ROWS_AVAIL_MAX":                            385,
	"DCGM_FI_DEV_BANKS_REMAP_ROWS_AVAIL_HIGH":                           386,
	"DCGM_FI_DEV_BANKS_REMAP_ROWS_AVAIL_PARTIAL":                        387,
	"DCGM_FI_DEV_BANKS_REMAP_ROWS_AVAIL_LOW":                            388,
	"DCGM_FI_DEV_BANKS_REMAP_ROWS_AVAIL_NONE":                           389,
	"DCGM_FI_DEV_RETIRED_SBE":                                           390,
	"DCGM_FI_DEV_RETIRED_DBE":                                           391,
	"DCGM_FI_DEV_RETIRED_PENDING":                                       392,
	"DCGM_FI_DEV_UNCORRECTABLE_REMAPPED_ROWS":                           393,
	"DCGM_FI_DEV_CORRECTABLE_REMAPPED_ROWS":                             394,
	"DCGM_FI_DEV_ROW_REMAP_FAILURE":                                     395,
	"DCGM_FI_DEV_ROW_REMAP_PENDING":                                     396,
	"DCGM_FI_DEV_NVLINK_CRC_FLIT_ERROR_COUNT_L0":                        400,
	"DCGM_FI_DEV_NVLINK_CRC_FLIT_ERROR_COUNT_L1":                        401,
	"DCGM_FI_DEV_NVLINK_CRC_FLIT_ERROR_COUNT_L2":                        402,
	"DCGM_FI_DEV_NVLINK_CRC_FLIT_ERROR_COUNT_L3":                        403,
	"DCGM_FI_DEV_NVLINK_CRC_FLIT_ERROR_COUNT_L4":                        404,
	"DCGM_FI_DEV_NVLINK_CRC_FLIT_ERROR_COUNT_L5":                        405,
	"DCGM_FI_DEV_NVLINK_CRC_FLIT_ERROR_COUNT_L12":                       406,
	"DCGM_FI_DEV_NVLINK_CRC_FLIT_ERROR_COUNT_L13":                       407,
	"DCGM_FI_DEV_NVLINK_CRC_FLIT_ERROR_COUNT_L14":                       408,
	"DCGM_FI_DEV_NVLINK_CRC_FLIT_ERROR_COUNT_TOTAL":                     409,
	"DCGM_FI_DEV_NVLINK_CRC_DATA_ERROR_COUNT_L0":                        410,
	"DCGM_FI_DEV_NVLINK_CRC_DATA_ERROR_COUNT_L1":                        411,
	"DCGM_FI_DEV_NVLINK_CRC_DATA_ERROR_COUNT_L2":                        412,
	"DCGM_FI_DEV_NVLINK_CRC_DATA_ERROR_COUNT_L3":                        413,
	"DCGM_FI_DEV_NVLINK_CRC_DATA_ERROR_COUNT_L4":                        414,
	"DCGM_FI_DEV_NVLINK_CRC_DATA_ERROR_COUNT_L5":                        415,
	"DCGM_FI_DEV_NVLINK_CRC_DATA_ERROR_COUNT_L12":                       416,
	"DCGM_FI_DEV_NVLINK_CRC_DATA_ERROR_COUNT_L13":                       417,
	"DCGM_FI_DEV_NVLINK_CRC_DATA_ERROR_COUNT_L14":                       418,
	"DCGM_FI_DEV_NVLINK_CRC_DATA_ERROR_COUNT_TOTAL":                     419,
	"DCGM_FI_DEV_NVLINK_REPLAY_ERROR_COUNT_L0":                          420,
	"DCGM_FI_DEV_NVLINK_REPLAY_ERROR_COUNT_L1":                          421,
	"DCGM_FI_DEV_NVLINK_REPLAY_ERROR_COUNT_L2":                          422,
	"DCGM_FI_DEV_NVLINK_REPLAY_ERROR_COUNT_L3":                          423,
	"DCGM_FI_DEV_NVLINK_REPLAY_ERROR_COUNT_L4":                          424,
	"DCGM_FI_DEV_NVLINK_REPLAY_ERROR_COUNT_L5":                          425,
	"DCGM_FI_DEV_NVLINK_REPLAY_ERROR_COUNT_L12":                         426,
	"DCGM_FI_DEV_NVLINK_REPLAY_ERROR_COUNT_L13":                         427,
	"DCGM_FI_DEV_NVLINK_REPLAY_ERROR_COUNT_L14":                         428,
	"DCGM_FI_DEV_NVLINK_REPLAY_ERROR_COUNT_TOTAL":                       429,
	"DCGM_FI_DEV_NVLINK_RECOVERY_ERROR_COUNT_L0":                        430,
	"DCGM_FI_DEV_NVLINK_RECOVERY_ERROR_COUNT_L1":                        431,
	"DCGM_FI_DEV_NVLINK_RECOVERY_ERROR_COUNT_L2":                        432,
	"DCGM_FI_DEV_NVLINK_RECOVERY_ERROR_COUNT_L3":                        433,
	"DCGM_FI_DEV_NVLINK_RECOVERY_ERROR_COUNT_L4":                        434,
	"DCGM_FI_DEV_NVLINK_RECOVERY_ERROR_COUNT_L5":                        435,
	"DCGM_FI_DEV_NVLINK_RECOVERY_ERROR_COUNT_L12":                       436,
	"DCGM_FI_DEV_NVLINK_RECOVERY_ERROR_COUNT_L13":                       437,
	"DCGM_FI_DEV_NVLINK_RECOVERY_ERROR_COUNT_L14":                       438,
	"DCGM_FI_DEV_NVLINK_RECOVERY_ERROR_COUNT_TOTAL":                     439,
	"DCGM_FI_DEV_NVLINK_THROUGHPUT_L0":                                  440,
	"DCGM_FI_DEV_NVLINK_THROUGHPUT_L1":                                  441,
	"DCGM_FI_DEV_NVLINK_THROUGHPUT_L2":                                  442,
	"DCGM_FI_DEV_NVLINK_THROUGHPUT_L3":                                  443,
	"DCGM_FI_DEV_NVLINK_THROUGHPUT_L4":                                  444,
	"DCGM_FI_DEV_NVLINK_THROUGHPUT_L5":                                  445,
	"DCGM_FI_DEV_NVLINK_THROUGHPUT_L12":                                 446,
	"DCGM_FI_DEV_NVLINK_THROUGHPUT_L13":                                 447,
	"DCGM_FI_DEV_NVLINK_THROUGHPUT_L14":                                 448,
	"DCGM_FI_DEV_NVLINK_THROUGHPUT_TOTAL":                               449,
	"DCGM_FI_DEV_GPU_NVLINK_ERRORS":                                     450,
	"DCGM_FI_DEV_NVLINK_CRC_FLIT_ERROR_COUNT_L6":                        451,
	"DCGM_FI_DEV_NVLINK_CRC_FLIT_ERROR_COUNT_L7":                        452,
	"DCGM_FI_DEV_NVLINK_CRC_FLIT_ERROR_COUNT_L8":                        453,
	"DCGM_FI_DEV_NVLINK_CRC_FLIT_ERROR_COUNT_L9":                        454,
	"DCGM_FI_DEV_NVLINK_CRC_FLIT_ERROR_COUNT_L10":                       455,
	"DCGM_FI_DEV_NVLINK_CRC_FLIT_ERROR_COUNT_L11":                       456,
	"DCGM_FI_DEV_NVLINK_CRC_DATA_ERROR_COUNT_L6":                        457,
	"DCGM_FI_DEV_NVLINK_CRC_DATA_ERROR_COUNT_L7":                        458,
	"DCGM_FI_DEV_NVLINK_CRC_DATA_ERROR_COUNT_L8":                        459,
	"DCGM_FI_DEV_NVLINK_CRC_DATA_ERROR_COUNT_L9":                        460,
	"DCGM_FI_DEV_NVLINK_CRC_DATA_ERROR_COUNT_L10":                       461,
	"DCGM_FI_DEV_NVLINK_CRC_DATA_ERROR_COUNT_L11":                       462,
	"DCGM_FI_DEV_NVLINK_REPLAY_ERROR_COUNT_L6":                          463,
	"DCGM_FI_DEV_NVLINK_REPLAY_ERROR_COUNT_L7":                          464,
	"DCGM_FI_DEV_NVLINK_REPLAY_ERROR_COUNT_L8":                          465,
	"DCGM_FI_DEV_NVLINK_REPLAY_ERROR_COUNT_L9":                          466,
	"DCGM_FI_DEV_NVLINK_REPLAY_ERROR_COUNT_L10":                         467,
	"DCGM_FI_DEV_NVLINK_REPLAY_ERROR_COUNT_L11":                         468,
	"DCGM_FI_DEV_NVLINK_RECOVERY_ERROR_COUNT_L6":                        469,
	"DCGM_FI_DEV_NVLINK_RECOVERY_ERROR_COUNT_L7":                        470,
	"DCGM_FI_DEV_NVLINK_RECOVERY_ERROR_COUNT_L8":                        471,
	"DCGM_FI_DEV_NVLINK_RECOVERY_ERROR_COUNT_L9":                        472,
	"DCGM_FI_DEV_NVLINK_RECOVERY_ERROR_COUNT_L10":                       473,
	"DCGM_FI_DEV_NVLINK_RECOVERY_ERROR_COUNT_L11":                       474,
	"DCGM_FI_DEV_NVLINK_THROUGHPUT_L6":                                  475,
	"DCGM_FI_DEV_NVLINK_THROUGHPUT_L7":                                  476,
	"DCGM_FI_DEV_NVLINK_THROUGHPUT_L8":                                  477,
	"DCGM_FI_DEV_NVLINK_THROUGHPUT_L9":                                  478,
	"DCGM_FI_DEV_NVLINK_THROUGHPUT_L10":                                 479,
	"DCGM_FI_DEV_NVLINK_THROUGHPUT_L11":                                 480,
	"DCGM_FI_DEV_NVLINK_CRC_FLIT_ERROR_COUNT_L15":                       481,
	"DCGM_FI_DEV_NVLINK_CRC_FLIT_ERROR_COUNT_L16":                       482,
	"DCGM_FI_DEV_NVLINK_CRC_FLIT_ERROR_COUNT_L17":                       483,
	"DCGM_FI_DEV_NVLINK_CRC_DATA_ERROR_COUNT_L15":                       484,
	"DCGM_FI_DEV_NVLINK_CRC_DATA_ERROR_COUNT_L16":                       485,
	"DCGM_FI_DEV_NVLINK_CRC_DATA_ERROR_COUNT_L17":                       486,
	"DCGM_FI_DEV_NVLINK_REPLAY_ERROR_COUNT_L15":                         487,
	"DCGM_FI_DEV_NVLINK_REPLAY_ERROR_COUNT_L16":                         488,
	"DCGM_FI_DEV_NVLINK_REPLAY_ERROR_COUNT_L17":                         489,
	"DCGM_FI_DEV_NVLINK_RECOVERY_ERROR_COUNT_L15":                       491,
	"DCGM_FI_DEV_NVLINK_RECOVERY_ERROR_COUNT_L16":                       492,
	"DCGM_FI_DEV_NVLINK_RECOVERY_ERROR_COUNT_L17":                       493,
	"DCGM_FI_DEV_NVLINK_THROUGHPUT_L15":                                 494,
	"DCGM_FI_DEV_NVLINK_THROUGHPUT_L16":                                 495,
	"DCGM_FI_DEV_NVLINK_THROUGHPUT_L17":                                 496,
	"DCGM_FI_DEV_NVLINK_ERROR_DL_CRC":                                   497,
	"DCGM_FI_DEV_NVLINK_ERROR_DL_RECOVERY":                              498,
	"DCGM_FI_DEV_NVLINK_ERROR_DL_REPLAY":                                499,
	"DCGM_FI_DEV_VIRTUAL_MODE":                                          500,
	"DCGM_FI_DEV_SUPPORTED_TYPE_INFO":                                   501,
	"DCGM_FI_DEV_CREATABLE_VGPU_TYPE_IDS":                               502,
	"DCGM_FI_DEV_VGPU_INSTANCE_IDS":                                     503,
	"DCGM_FI_DEV_VGPU_UTILIZATIONS":                                     504,
	"DCGM_FI_DEV_VGPU_PER_PROCESS_UTILIZATION":                          505,
	"DCGM_FI_DEV_ENC_STATS":                                             506,
	"DCGM_FI_DEV_FBC_STATS":                                             507,
	"DCGM_FI_DEV_FBC_SESSIONS_INFO":                                     508,
	"DCGM_FI_DEV_SUPPORTED_VGPU_TYPE_IDS":                               509,
	"DCGM_FI_DEV_VGPU_TYPE_INFO":                                        510,
	"DCGM_FI_DEV_VGPU_TYPE_NAME":                                        511,
	"DCGM_FI_DEV_VGPU_TYPE_CLASS":                                       512,
	"DCGM_FI_DEV_VGPU_TYPE_LICENSE":                                     513,
	"DCGM_FI_FIRST_VGPU_FIELD_ID":                                       520,
	"DCGM_FI_DEV_VGPU_VM_ID":                                            520,
	"DCGM_FI_DEV_VGPU_VM_NAME":                                          521,
	"DCGM_FI_DEV_VGPU_TYPE":                                             522,
	"DCGM_FI_DEV_VGPU_UUID":                                             523,
	"DCGM_FI_DEV_VGPU_DRIVER_VERSION":                                   524,
	"DCGM_FI_DEV_VGPU_MEMORY_USAGE":                                     525,
	"DCGM_FI_DEV_VGPU_LICENSE_STATUS":                                   526,
	"DCGM_FI_DEV_VGPU_FRAME_RATE_LIMIT":                                 527,
	"DCGM_FI_DEV_VGPU_ENC_STATS":                                        528,
	"DCGM_FI_DEV_VGPU_ENC_SESSIONS_INFO":                                529,
	"DCGM_FI_DEV_VGPU_FBC_STATS":                                        530,
	"DCGM_FI_DEV_VGPU_FBC_SESSIONS_INFO":                                531,
	"DCGM_FI_DEV_VGPU_INSTANCE_LICENSE_STATE":                           532,
	"DCGM_FI_DEV_VGPU_PCI_ID":                                           533,
	"DCGM_FI_DEV_VGPU_VM_GPU_INSTANCE_ID":                               534,
	"DCGM_FI_LAST_VGPU_FIELD_ID":                                        570,
	"DCGM_FI_DEV_PLATFORM_INFINIBAND_GUID":                              571,
	"DCGM_FI_DEV_PLATFORM_CHASSIS_SERIAL_NUMBER":                        572,
	"DCGM_FI_DEV_PLATFORM_CHASSIS_SLOT_NUMBER":                          573,
	"DCGM_FI_DEV_PLATFORM_TRAY_INDEX":                                   574,
	"DCGM_FI_DEV_PLATFORM_HOST_ID":                                      575,
	"DCGM_FI_DEV_PLATFORM_PEER_TYPE":                                    576,
	"DCGM_FI_DEV_PLATFORM_MODULE_ID":                                    577,
	"DCGM_FI_DEV_NVLINK_PPRM_OPER_RECOVERY":                             580,
	"DCGM_FI_DEV_NVLINK_PPCNT_RECOVERY_TIME_SINCE_LAST":                 581,
	"DCGM_FI_DEV_NVLINK_PPCNT_RECOVERY_TIME_BETWEEN_LAST_TWO":           582,
	"DCGM_FI_DEV_NVLINK_PPCNT_RECOVERY_TOTAL_SUCCESSFUL_EVENTS":         583,
	"DCGM_FI_DEV_NVLINK_PPCNT_PHYSICAL_SUCCESSFUL_RECOVERY_EVENTS":      584,
	"DCGM_FI_DEV_NVLINK_PPCNT_PHYSICAL_LINK_DOWN_COUNTER":               585,
	"DCGM_FI_DEV_NVLINK_PPCNT_PLR_RCV_CODES":                            586,
	"DCGM_FI_DEV_NVLINK_PPCNT_PLR_RCV_CODE_ERR":                         587,
	"DCGM_FI_DEV_NVLINK_PPCNT_PLR_RCV_UNCORRECTABLE_CODE":               588,
	"DCGM_FI_DEV_NVLINK_PPCNT_PLR_XMIT_CODES":                           589,
	"DCGM_FI_DEV_NVLINK_PPCNT_PLR_XMIT_RETRY_CODES":                     590,
	"DCGM_FI_DEV_NVLINK_PPCNT_PLR_XMIT_RETRY_EVENTS":                    591,
	"DCGM_FI_DEV_NVLINK_PPCNT_PLR_SYNC_EVENTS":                          592,
	"DCGM_FI_INTERNAL_FIELDS_0_START":                                   600,
	"DCGM_FI_INTERNAL_FIELDS_0_END":                                     699,
	"DCGM_FI_FIRST_NVSWITCH_FIELD_ID":                                   700,
	"DCGM_FI_DEV_NVSWITCH_VOLTAGE_MVOLT":                                701,
	"DCGM_FI_DEV_NVSWITCH_CURRENT_IDDQ":                                 702,
	"DCGM_FI_DEV_NVSWITCH_CURRENT_IDDQ_REV":                             703,
	"DCGM_FI_DEV_NVSWITCH_CURRENT_IDDQ_DVDD":                            704,
	"DCGM_FI_DEV_NVSWITCH_POWER_VDD":                                    705,
	"DCGM_FI_DEV_NVSWITCH_POWER_DVDD":                                   706,
	"DCGM_FI_DEV_NVSWITCH_POWER_HVDD":                                   707,
	"DCGM_FI_DEV_NVSWITCH_LINK_THROUGHPUT_TX":                           780,
	"DCGM_FI_DEV_NVSWITCH_LINK_THROUGHPUT_RX":                           781,
	"DCGM_FI_DEV_NVSWITCH_LINK_FATAL_ERRORS":                            782,
	"DCGM_FI_DEV_NVSWITCH_LINK_NON_FATAL_ERRORS":                        783,
	"DCGM_FI_DEV_NVSWITCH_LINK_REPLAY_ERRORS":                           784,
	"DCGM_FI_DEV_NVSWITCH_LINK_RECOVERY_ERRORS":                         785,
	"DCGM_FI_DEV_NVSWITCH_LINK_FLIT_ERRORS":                             786,
	"DCGM_FI_DEV_NVSWITCH_LINK_CRC_ERRORS":                              787,
	"DCGM_FI_DEV_NVSWITCH_LINK_ECC_ERRORS":                              788,
	"DCGM_FI_DEV_NVSWITCH_LINK_LATENCY_LOW_VC0":                         789,
	"DCGM_FI_DEV_NVSWITCH_LINK_LATENCY_LOW_VC1":                         790,
	"DCGM_FI_DEV_NVSWITCH_LINK_LATENCY_LOW_VC2":                         791,
	"DCGM_FI_DEV_NVSWITCH_LINK_LATENCY_LOW_VC3":                         792,
	"DCGM_FI_DEV_NVSWITCH_LINK_LATENCY_MEDIUM_VC0":                      793,
	"DCGM_FI_DEV_NVSWITCH_LINK_LATENCY_MEDIUM_VC1":                      794,
	"DCGM_FI_DEV_NVSWITCH_LINK_LATENCY_MEDIUM_VC2":                      795,
	"DCGM_FI_DEV_NVSWITCH_LINK_LATENCY_MEDIUM_VC3":                      796,
	"DCGM_FI_DEV_NVSWITCH_LINK_LATENCY_HIGH_VC0":                        797,
	"DCGM_FI_DEV_NVSWITCH_LINK_LATENCY_HIGH_VC1":                        798,
	"DCGM_FI_DEV_NVSWITCH_LINK_LATENCY_HIGH_VC2":                        799,
	"DCGM_FI_DEV_NVSWITCH_LINK_LATENCY_HIGH_VC3":                        800,
	"DCGM_FI_DEV_NVSWITCH_LINK_LATENCY_PANIC_VC0":                       801,
	"DCGM_FI_DEV_NVSWITCH_LINK_LATENCY_PANIC_VC1":                       802,
	"DCGM_FI_DEV_NVSWITCH_LINK_LATENCY_PANIC_VC2":                       803,
	"DCGM_FI_DEV_NVSWITCH_LINK_LATENCY_PANIC_VC3":                       804,
	"DCGM_FI_DEV_NVSWITCH_LINK_LATENCY_COUNT_VC0":                       805,
	"DCGM_FI_DEV_NVSWITCH_LINK_LATENCY_COUNT_VC1":                       806,
	"DCGM_FI_DEV_NVSWITCH_LINK_LATENCY_COUNT_VC2":                       807,
	"DCGM_FI_DEV_NVSWITCH_LINK_LATENCY_COUNT_VC3":                       808,
	"DCGM_FI_DEV_NVSWITCH_LINK_CRC_ERRORS_LANE0":                        809,
	"DCGM_FI_DEV_NVSWITCH_LINK_CRC_ERRORS_LANE1":                        810,
	"DCGM_FI_DEV_NVSWITCH_LINK_CRC_ERRORS_LANE2":                        811,
	"DCGM_FI_DEV_NVSWITCH_LINK_CRC_ERRORS_LANE3":                        812,
	"DCGM_FI_DEV_NVSWITCH_LINK_ECC_ERRORS_LANE0":                        813,
	"DCGM_FI_DEV_NVSWITCH_LINK_ECC_ERRORS_LANE1":                        814,
	"DCGM_FI_DEV_NVSWITCH_LINK_ECC_ERRORS_LANE2":                        815,
	"DCGM_FI_DEV_NVSWITCH_LINK_ECC_ERRORS_LANE3":                        816,
	"DCGM_FI_DEV_NVSWITCH_LINK_CRC_ERRORS_LANE4":                        817,
	"DCGM_FI_DEV_NVSWITCH_LINK_CRC_ERRORS_LANE5":                        818,
	"DCGM_FI_DEV_NVSWITCH_LINK_CRC_ERRORS_LANE6":                        819,
	"DCGM_FI_DEV_NVSWITCH_LINK_CRC_ERRORS_LANE7":                        820,
	"DCGM_FI_DEV_NVSWITCH_LINK_ECC_ERRORS_LANE4":                        821,
	"DCGM_FI_DEV_NVSWITCH_LINK_ECC_ERRORS_LANE5":                        822,
	"DCGM_FI_DEV_NVSWITCH_LINK_ECC_ERRORS_LANE6":                        823,
	"DCGM_FI_DEV_NVSWITCH_LINK_ECC_ERRORS_LANE7":                        824,
	"DCGM_FI_DEV_NVLINK_TX_THROUGHPUT_L0":                               825,
	"DCGM_FI_DEV_NVLINK_TX_THROUGHPUT_L1":                               826,
	"DCGM_FI_DEV_NVLINK_TX_THROUGHPUT_L2":                               827,
	"DCGM_FI_DEV_NVLINK_TX_THROUGHPUT_L3":                               828,
	"DCGM_FI_DEV_NVLINK_TX_THROUGHPUT_L4":                               829,
	"DCGM_FI_DEV_NVLINK_TX_THROUGHPUT_L5":                               830,
	"DCGM_FI_DEV_NVLINK_TX_THROUGHPUT_L6":                               831,
	"DCGM_FI_DEV_NVLINK_TX_THROUGHPUT_L7":                               832,
	"DCGM_FI_DEV_NVLINK_TX_THROUGHPUT_L8":                               833,
	"DCGM_FI_DEV_NVLINK_TX_THROUGHPUT_L9":                               834,
	"DCGM_FI_DEV_NVLINK_TX_THROUGHPUT_L10":                              835,
	"DCGM_FI_DEV_NVLINK_TX_THROUGHPUT_L11":                              836,
	"DCGM_FI_DEV_NVLINK_TX_THROUGHPUT_L12":                              837,
	"DCGM_FI_DEV_NVLINK_TX_THROUGHPUT_L13":                              838,
	"DCGM_FI_DEV_NVLINK_TX_THROUGHPUT_L14":                              839,
	"DCGM_FI_DEV_NVLINK_TX_THROUGHPUT_L15":                              840,
	"DCGM_FI_DEV_NVLINK_TX_THROUGHPUT_L16":                              841,
	"DCGM_FI_DEV_NVLINK_TX_THROUGHPUT_L17":                              842,
	"DCGM_FI_DEV_NVLINK_TX_THROUGHPUT_TOTAL":                            843,
	"DCGM_FI_DEV_NVSWITCH_FATAL_ERRORS":                                 856,
	"DCGM_FI_DEV_NVSWITCH_NON_FATAL_ERRORS":                             857,
	"DCGM_FI_DEV_NVSWITCH_TEMPERATURE_CURRENT":                          858,
	"DCGM_FI_DEV_NVSWITCH_TEMPERATURE_LIMIT_SLOWDOWN":                   859,
	"DCGM_FI_DEV_NVSWITCH_TEMPERATURE_LIMIT_SHUTDOWN":                   860,
	"DCGM_FI_DEV_NVSWITCH_THROUGHPUT_TX":                                861,
	"DCGM_FI_DEV_NVSWITCH_THROUGHPUT_RX":                                862,
	"DCGM_FI_DEV_NVSWITCH_PHYS_ID":                                      863,
	"DCGM_FI_DEV_NVSWITCH_RESET_REQUIRED":                               864,
	"DCGM_FI_DEV_NVSWITCH_LINK_ID":                                      865,
	"DCGM_FI_DEV_NVSWITCH_PCIE_DOMAIN":                                  866,
	"DCGM_FI_DEV_NVSWITCH_PCIE_BUS":                                     867,
	"DCGM_FI_DEV_NVSWITCH_PCIE_DEVICE":                                  868,
	"DCGM_FI_DEV_NVSWITCH_PCIE_FUNCTION":                                869,
	"DCGM_FI_DEV_NVSWITCH_LINK_STATUS":                                  870,
	"DCGM_FI_DEV_NVSWITCH_LINK_TYPE":                                    871,
	"DCGM_FI_DEV_NVSWITCH_LINK_REMOTE_PCIE_DOMAIN":                      872,
	"DCGM_FI_DEV_NVSWITCH_LINK_REMOTE_PCIE_BUS":                         873,
	"DCGM_FI_DEV_NVSWITCH_LINK_REMOTE_PCIE_DEVICE":                      874,
	"DCGM_FI_DEV_NVSWITCH_LINK_REMOTE_PCIE_FUNCTION":                    875,
	"DCGM_FI_DEV_NVSWITCH_LINK_DEVICE_LINK_ID":                          876,
	"DCGM_FI_DEV_NVSWITCH_LINK_DEVICE_LINK_SID":                         877,
	"DCGM_FI_DEV_NVSWITCH_DEVICE_UUID":                                  878,
	"DCGM_FI_DEV_NVLINK_RX_THROUGHPUT_L0":                               879,
	"DCGM_FI_DEV_NVLINK_RX_THROUGHPUT_L1":                               880,
	"DCGM_FI_DEV_NVLINK_RX_THROUGHPUT_L2":                               881,
	"DCGM_FI_DEV_NVLINK_RX_THROUGHPUT_L3":                               882,
	"DCGM_FI_DEV_NVLINK_RX_THROUGHPUT_L4":                               883,
	"DCGM_FI_DEV_NVLINK_RX_THROUGHPUT_L5":                               884,
	"DCGM_FI_DEV_NVLINK_RX_THROUGHPUT_L6":                               885,
	"DCGM_FI_DEV_NVLINK_RX_THROUGHPUT_L7":                               886,
	"DCGM_FI_DEV_NVLINK_RX_THROUGHPUT_L8":                               887,
	"DCGM_FI_DEV_NVLINK_RX_THROUGHPUT_L9":                               888,
	"DCGM_FI_DEV_NVLINK_RX_THROUGHPUT_L10":                              889,
	"DCGM_FI_DEV_NVLINK_RX_THROUGHPUT_L11":                              890,
	"DCGM_FI_DEV_NVLINK_RX_THROUGHPUT_L12":                              891,
	"DCGM_FI_DEV_NVLINK_RX_THROUGHPUT_L13":                              892,
	"DCGM_FI_DEV_NVLINK_RX_THROUGHPUT_L14":                              893,
	"DCGM_FI_DEV_NVLINK_RX_THROUGHPUT_L15":                              894,
	"DCGM_FI_DEV_NVLINK_RX_THROUGHPUT_L16":                              895,
	"DCGM_FI_DEV_NVLINK_RX_THROUGHPUT_L17":                              896,
	"DCGM_FI_DEV_NVLINK_RX_THROUGHPUT_TOTAL":                            897,
	"DCGM_FI_LAST_NVSWITCH_FIELD_ID":                                    899,
	"DCGM_FI_PROF_GR_ENGINE_ACTIVE":                                     1001,
	"DCGM_FI_PROF_SM_ACTIVE":                                            1002,
	"DCGM_FI_PROF_SM_OCCUPANCY":                                         1003,
	"DCGM_FI_PROF_PIPE_TENSOR_ACTIVE":                                   1004,
	"DCGM_FI_PROF_DRAM_ACTIVE":                                          1005,
	"DCGM_FI_PROF_PIPE_FP64_ACTIVE":                                     1006,
	"DCGM_FI_PROF_PIPE_FP32_ACTIVE":                                     1007,
	"DCGM_FI_PROF_PIPE_FP16_ACTIVE":                                     1008,
	"DCGM_FI_PROF_PCIE_TX_BYTES":                                        1009,
	"DCGM_FI_PROF_PCIE_RX_BYTES":                                        1010,
	"DCGM_FI_PROF_NVLINK_TX_BYTES":                                      1011,
	"DCGM_FI_PROF_NVLINK_RX_BYTES":                                      1012,
	"DCGM_FI_PROF_PIPE_TENSOR_IMMA_ACTIVE":                              1013,
	"DCGM_FI_PROF_PIPE_TENSOR_HMMA_ACTIVE":                              1014,
	"DCGM_FI_PROF_PIPE_TENSOR_DFMA_ACTIVE":                              1015,
	"DCGM_FI_PROF_PIPE_INT_ACTIVE":                                      1016,
	"DCGM_FI_PROF_NVDEC0_ACTIVE":                                        1017,
	"DCGM_FI_PROF_NVDEC1_ACTIVE":                                        1018,
	"DCGM_FI_PROF_NVDEC2_ACTIVE":                                        1019,
	"DCGM_FI_PROF_NVDEC3_ACTIVE":                                        1020,
	"DCGM_FI_PROF_NVDEC4_ACTIVE":                                        1021,
	"DCGM_FI_PROF_NVDEC5_ACTIVE":                                        1022,
	"DCGM_FI_PROF_NVDEC6_ACTIVE":                                        1023,
	"DCGM_FI_PROF_NVDEC7_ACTIVE":                                        1024,
	"DCGM_FI_PROF_NVJPG0_ACTIVE":                                        1025,
	"DCGM_FI_PROF_NVJPG1_ACTIVE":                                        1026,
	"DCGM_FI_PROF_NVJPG2_ACTIVE":                                        1027,
	"DCGM_FI_PROF_NVJPG3_ACTIVE":                                        1028,
	"DCGM_FI_PROF_NVJPG4_ACTIVE":                                        1029,
	"DCGM_FI_PROF_NVJPG5_ACTIVE":                                        1030,
	"DCGM_FI_PROF_NVJPG6_ACTIVE":                                        1031,
	"DCGM_FI_PROF_NVJPG7_ACTIVE":                                        1032,
	"DCGM_FI_PROF_NVOFA0_ACTIVE":                                        1033,
	"DCGM_FI_PROF_NVOFA1_ACTIVE":                                        1034,
	"DCGM_FI_PROF_NVLINK_L0_TX_BYTES":                                   1040,
	"DCGM_FI_PROF_NVLINK_L0_RX_BYTES":                                   1041,
	"DCGM_FI_PROF_NVLINK_L1_TX_BYTES":                                   1042,
	"DCGM_FI_PROF_NVLINK_L1_RX_BYTES":                                   1043,
	"DCGM_FI_PROF_NVLINK_L2_TX_BYTES":                                   1044,
	"DCGM_FI_PROF_NVLINK_L2_RX_BYTES":                                   1045,
	"DCGM_FI_PROF_NVLINK_L3_TX_BYTES":                                   1046,
	"DCGM_FI_PROF_NVLINK_L3_RX_BYTES":                                   1047,
	"DCGM_FI_PROF_NVLINK_L4_TX_BYTES":                                   1048,
	"DCGM_FI_PROF_NVLINK_L4_RX_BYTES":                                   1049,
	"DCGM_FI_PROF_NVLINK_L5_TX_BYTES":                                   1050,
	"DCGM_FI_PROF_NVLINK_L5_RX_BYTES":                                   1051,
	"DCGM_FI_PROF_NVLINK_L6_TX_BYTES":                                   1052,
	"DCGM_FI_PROF_NVLINK_L6_RX_BYTES":                                   1053,
	"DCGM_FI_PROF_NVLINK_L7_TX_BYTES":                                   1054,
	"DCGM_FI_PROF_NVLINK_L7_RX_BYTES":                                   1055,
	"DCGM_FI_PROF_NVLINK_L8_TX_BYTES":                                   1056,
	"DCGM_FI_PROF_NVLINK_L8_RX_BYTES":                                   1057,
	"DCGM_FI_PROF_NVLINK_L9_TX_BYTES":                                   1058,
	"DCGM_FI_PROF_NVLINK_L9_RX_BYTES":                                   1059,
	"DCGM_FI_PROF_NVLINK_L10_TX_BYTES":                                  1060,
	"DCGM_FI_PROF_NVLINK_L10_RX_BYTES":                                  1061,
	"DCGM_FI_PROF_NVLINK_L11_TX_BYTES":                                  1062,
	"DCGM_FI_PROF_NVLINK_L11_RX_BYTES":                                  1063,
	"DCGM_FI_PROF_NVLINK_L12_TX_BYTES":                                  1064,
	"DCGM_FI_PROF_NVLINK_L12_RX_BYTES":                                  1065,
	"DCGM_FI_PROF_NVLINK_L13_TX_BYTES":                                  1066,
	"DCGM_FI_PROF_NVLINK_L13_RX_BYTES":                                  1067,
	"DCGM_FI_PROF_NVLINK_L14_TX_BYTES":                                  1068,
	"DCGM_FI_PROF_NVLINK_L14_RX_BYTES":                                  1069,
	"DCGM_FI_PROF_NVLINK_L15_TX_BYTES":                                  1070,
	"DCGM_FI_PROF_NVLINK_L15_RX_BYTES":                                  1071,
	"DCGM_FI_PROF_NVLINK_L16_TX_BYTES":                                  1072,
	"DCGM_FI_PROF_NVLINK_L16_RX_BYTES":                                  1073,
	"DCGM_FI_PROF_NVLINK_L17_TX_BYTES":                                  1074,
	"DCGM_FI_PROF_NVLINK_L17_RX_BYTES":                                  1075,
	"DCGM_FI_PROF_C2C_TX_ALL_BYTES":                                     1076,
	"DCGM_FI_PROF_C2C_TX_DATA_BYTES":                                    1077,
	"DCGM_FI_PROF_C2C_RX_ALL_BYTES":                                     1078,
	"DCGM_FI_PROF_C2C_RX_DATA_BYTES":                                    1079,
	"DCGM_FI_PROF_HOSTMEM_CACHE_HIT":                                    1080,
	"DCGM_FI_PROF_HOSTMEM_CACHE_MISS":                                   1081,
	"DCGM_FI_PROF_PEERMEM_CACHE_HIT":                                    1082,
	"DCGM_FI_PROF_PEERMEM_CACHE_MISS":                                   1083,
	"DCGM_FI_DEV_CPU_UTIL_TOTAL":                                        1100,
	"DCGM_FI_DEV_CPU_UTIL_USER":                                         1101,
	"DCGM_FI_DEV_CPU_UTIL_NICE":                                         1102,
	"DCGM_FI_DEV_CPU_UTIL_SYS":                                          1103,
	"DCGM_FI_DEV_CPU_UTIL_IRQ":                                          1104,
	"DCGM_FI_DEV_CPU_TEMP_CURRENT":                                      1110,
	"DCGM_FI_DEV_CPU_TEMP_WARNING":                                      1111,
	"DCGM_FI_DEV_CPU_TEMP_CRITICAL":                                     1112,
	"DCGM_FI_DEV_CPU_CLOCK_CURRENT":                                     1120,
	"DCGM_FI_DEV_CPU_POWER_UTIL_CURRENT":                                1130,
	"DCGM_FI_DEV_CPU_POWER_LIMIT":                                       1131,
	"DCGM_FI_DEV_SYSIO_POWER_UTIL_CURRENT":                              1132,
	"DCGM_FI_DEV_MODULE_POWER_UTIL_CURRENT":                             1133,
	"DCGM_FI_DEV_CPU_VENDOR":                                            1140,
	"DCGM_FI_DEV_CPU_MODEL":                                             1141,
	"DCGM_FI_DEV_NVLINK_COUNT_TX_PACKETS":                               1200,
	"DCGM_FI_DEV_NVLINK_COUNT_TX_BYTES":                                 1201,
	"DCGM_FI_DEV_NVLINK_COUNT_RX_PACKETS":                               1202,
	"DCGM_FI_DEV_NVLINK_COUNT_RX_BYTES":                                 1203,
	"DCGM_FI_DEV_NVLINK_COUNT_RX_MALFORMED_PACKET_ERRORS":               1204,
	"DCGM_FI_DEV_NVLINK_COUNT_RX_BUFFER_OVERRUN_ERRORS":                 1205,
	"DCGM_FI_DEV_NVLINK_COUNT_RX_ERRORS":                                1206,
	"DCGM_FI_DEV_NVLINK_COUNT_RX_REMOTE_ERRORS":                         1207,
	"DCGM_FI_DEV_NVLINK_COUNT_RX_GENERAL_ERRORS":                        1208,
	"DCGM_FI_DEV_NVLINK_COUNT_LOCAL_LINK_INTEGRITY_ERRORS":              1209,
	"DCGM_FI_DEV_NVLINK_COUNT_TX_DISCARDS":                              1210,
	"DCGM_FI_DEV_NVLINK_COUNT_LINK_RECOVERY_SUCCESSFUL_EVENTS":          1211,
	"DCGM_FI_DEV_NVLINK_COUNT_LINK_RECOVERY_FAILED_EVENTS":              1212,
	"DCGM_FI_DEV_NVLINK_COUNT_LINK_RECOVERY_EVENTS":                     1213,
	"DCGM_FI_DEV_NVLINK_COUNT_RX_SYMBOL_ERRORS":                         1214,
	"DCGM_FI_DEV_NVLINK_COUNT_SYMBOL_BER":                               1215,
	"DCGM_FI_DEV_NVLINK_COUNT_SYMBOL_BER_FLOAT":                         1216,
	"DCGM_FI_DEV_NVLINK_COUNT_EFFECTIVE_BER":                            1217,
	"DCGM_FI_DEV_NVLINK_COUNT_EFFECTIVE_BER_FLOAT":                      1218,
	"DCGM_FI_DEV_NVLINK_COUNT_EFFECTIVE_ERRORS":                         1219,
	"DCGM_FI_DEV_NVLINK_ECC_DATA_ERROR_COUNT_TOTAL":                     1220,
	"DCGM_FI_DEV_FIRST_CONNECTX_FIELD_ID":                               1300,
	"DCGM_FI_DEV_CONNECTX_HEALTH":                                       1300,
	"DCGM_FI_DEV_CONNECTX_ACTIVE_PCIE_LINK_WIDTH":                       1301,
	"DCGM_FI_DEV_CONNECTX_ACTIVE_PCIE_LINK_SPEED":                       1302,
	"DCGM_FI_DEV_CONNECTX_EXPECT_PCIE_LINK_WIDTH":                       1303,
	"DCGM_FI_DEV_CONNECTX_EXPECT_PCIE_LINK_SPEED":                       1304,
	"DCGM_FI_DEV_CONNECTX_CORRECTABLE_ERR_STATUS":                       1305,
	"DCGM_FI_DEV_CONNECTX_CORRECTABLE_ERR_MASK":                         1306,
	"DCGM_FI_DEV_CONNECTX_UNCORRECTABLE_ERR_STATUS":                     1307,
	"DCGM_FI_DEV_CONNECTX_UNCORRECTABLE_ERR_MASK":                       1308,
	"DCGM_FI_DEV_CONNECTX_UNCORRECTABLE_ERR_SEVERITY":                   1309,
	"DCGM_FI_DEV_CONNECTX_DEVICE_TEMPERATURE":                           1310,
	"DCGM_FI_DEV_LAST_CONNECTX_FIELD_ID":                                1399,
	"DCGM_FI_DEV_C2C_LINK_ERROR_INTR":                                   1400,
	"DCGM_FI_DEV_C2C_LINK_ERROR_REPLAY":                                 1401,
	"DCGM_FI_DEV_C2C_LINK_ERROR_REPLAY_B2B":                             1402,
	"DCGM_FI_DEV_C2C_LINK_POWER_STATE":                                  1403,
	"DCGM_FI_DEV_NVLINK_COUNT_FEC_HISTORY_0":                            1404,
	"DCGM_FI_DEV_NVLINK_COUNT_FEC_HISTORY_1":                            1405,
	"DCGM_FI_DEV_NVLINK_COUNT_FEC_HISTORY_2":                            1406,
	"DCGM_FI_DEV_NVLINK_COUNT_FEC_HISTORY_3":                            1407,
	"DCGM_FI_DEV_NVLINK_COUNT_FEC_HISTORY_4":                            1408,
	"DCGM_FI_DEV_NVLINK_COUNT_FEC_HISTORY_5":                            1409,
	"DCGM_FI_DEV_NVLINK_COUNT_FEC_HISTORY_6":                            1410,
	"DCGM_FI_DEV_NVLINK_COUNT_FEC_HISTORY_7":                            1411,
	"DCGM_FI_DEV_NVLINK_COUNT_FEC_HISTORY_8":                            1412,
	"DCGM_FI_DEV_NVLINK_COUNT_FEC_HISTORY_9":                            1413,
	"DCGM_FI_DEV_NVLINK_COUNT_FEC_HISTORY_10":                           1414,
	"DCGM_FI_DEV_NVLINK_COUNT_FEC_HISTORY_11":                           1415,
	"DCGM_FI_DEV_NVLINK_COUNT_FEC_HISTORY_12":                           1416,
	"DCGM_FI_DEV_NVLINK_COUNT_FEC_HISTORY_13":                           1417,
	"DCGM_FI_DEV_NVLINK_COUNT_FEC_HISTORY_14":                           1418,
	"DCGM_FI_DEV_NVLINK_COUNT_FEC_HISTORY_15":                           1419,
	"DCGM_FI_DEV_CLOCKS_EVENT_REASON_SW_POWER_CAP_NS":                   1420,
	"DCGM_FI_DEV_CLOCKS_EVENT_REASON_SYNC_BOOST_NS":                     1421,
	"DCGM_FI_DEV_CLOCKS_EVENT_REASON_SW_THERM_SLOWDOWN_NS":              1422,
	"DCGM_FI_DEV_CLOCKS_EVENT_REASON_HW_THERM_SLOWDOWN_NS":              1423,
	"DCGM_FI_DEV_CLOCKS_EVENT_REASON_HW_POWER_BRAKE_SLOWDOWN_NS":        1424,
	"DCGM_FI_DEV_PWR_SMOOTHING_ENABLED":                                 1425,
	"DCGM_FI_DEV_PWR_SMOOTHING_PRIV_LVL":                                1426,
	"DCGM_FI_DEV_PWR_SMOOTHING_IMM_RAMP_DOWN_ENABLED":                   1427,
	"DCGM_FI_DEV_PWR_SMOOTHING_APPLIED_TMP_CEIL":                        1428,
	"DCGM_FI_DEV_PWR_SMOOTHING_APPLIED_TMP_FLOOR":                       1429,
	"DCGM_FI_DEV_PWR_SMOOTHING_MAX_PERCENT_TMP_FLOOR_SETTING":           1430,
	"DCGM_FI_DEV_PWR_SMOOTHING_MIN_PERCENT_TMP_FLOOR_SETTING":           1431,
	"DCGM_FI_DEV_PWR_SMOOTHING_HW_CIRCUITRY_PERCENT_LIFETIME_REMAINING": 1432,
	"DCGM_FI_DEV_PWR_SMOOTHING_MAX_NUM_PRESET_PROFILES":                 1433,
	"DCGM_FI_DEV_PWR_SMOOTHING_PROFILE_PERCENT_TMP_FLOOR":               1434,
	"DCGM_FI_DEV_PWR_SMOOTHING_PROFILE_RAMP_UP_RATE":                    1435,
	"DCGM_FI_DEV_PWR_SMOOTHING_PROFILE_RAMP_DOWN_RATE":                  1436,
	"DCGM_FI_DEV_PWR_SMOOTHING_PROFILE_RAMP_DOWN_HYST_VAL":              1437,
	"DCGM_FI_DEV_PWR_SMOOTHING_ACTIVE_PRESET_PROFILE":                   1438,
	"DCGM_FI_DEV_PWR_SMOOTHING_ADMIN_OVERRIDE_PERCENT_TMP_FLOOR":        1439,
	"DCGM_FI_DEV_PWR_SMOOTHING_ADMIN_OVERRIDE_RAMP_UP_RATE":             1440,
	"DCGM_FI_DEV_PWR_SMOOTHING_ADMIN_OVERRIDE_RAMP_DOWN_RATE":           1441,
	"DCGM_FI_DEV_PWR_SMOOTHING_ADMIN_OVERRIDE_RAMP_DOWN_HYST_VAL":       1442,
	"DCGM_FI_DEV_PCIE_COUNT_CORRECTABLE_ERRORS":                         1501,
	"DCGM_FI_IMEX_DOMAIN_STATUS":                                        1502,
	"DCGM_FI_IMEX_DAEMON_STATUS":                                        1503,
	"DCGM_FI_DEV_MEMORY_UNREPAIRABLE_FLAG":                              1507,
	"DCGM_FI_DEV_NVLINK_GET_STATE":                                      1508,
	"DCGM_FI_DEV_NVLINK_PPCNT_IBPC_PORT_XMIT_WAIT":                      1509,
	"DCGM_FI_DEV_GET_GPU_RECOVERY_ACTION":                               1523,
	"DCGM_FI_DEV_NVSWITCH_FIRMWARE_VERSION":                             1524,
}

// legacyDCGMFields maps legacy field names to their IDs
var legacyDCGMFields = map[string]Short{
	"DCGM_FI_DEV_CLOCK_THROTTLE_REASONS":     112,
	"DCGM_FI_DEV_NVLINK_BANDWIDTH_L0":        440,
	"DCGM_FI_DEV_NVLINK_BANDWIDTH_L1":        441,
	"DCGM_FI_DEV_NVLINK_BANDWIDTH_L10":       479,
	"DCGM_FI_DEV_NVLINK_BANDWIDTH_L11":       480,
	"DCGM_FI_DEV_NVLINK_BANDWIDTH_L12":       446,
	"DCGM_FI_DEV_NVLINK_BANDWIDTH_L13":       447,
	"DCGM_FI_DEV_NVLINK_BANDWIDTH_L14":       448,
	"DCGM_FI_DEV_NVLINK_BANDWIDTH_L15":       494,
	"DCGM_FI_DEV_NVLINK_BANDWIDTH_L16":       495,
	"DCGM_FI_DEV_NVLINK_BANDWIDTH_L17":       496,
	"DCGM_FI_DEV_NVLINK_BANDWIDTH_L2":        442,
	"DCGM_FI_DEV_NVLINK_BANDWIDTH_L3":        443,
	"DCGM_FI_DEV_NVLINK_BANDWIDTH_L4":        444,
	"DCGM_FI_DEV_NVLINK_BANDWIDTH_L5":        445,
	"DCGM_FI_DEV_NVLINK_BANDWIDTH_L6":        475,
	"DCGM_FI_DEV_NVLINK_BANDWIDTH_L7":        476,
	"DCGM_FI_DEV_NVLINK_BANDWIDTH_L8":        477,
	"DCGM_FI_DEV_NVLINK_BANDWIDTH_L9":        478,
	"DCGM_FI_DEV_NVLINK_BANDWIDTH_TOTAL":     449,
	"DCGM_FI_DEV_NVLINK_RX_BANDWIDTH_L0":     879,
	"DCGM_FI_DEV_NVLINK_RX_BANDWIDTH_L1":     880,
	"DCGM_FI_DEV_NVLINK_RX_BANDWIDTH_L10":    889,
	"DCGM_FI_DEV_NVLINK_RX_BANDWIDTH_L11":    890,
	"DCGM_FI_DEV_NVLINK_RX_BANDWIDTH_L12":    891,
	"DCGM_FI_DEV_NVLINK_RX_BANDWIDTH_L13":    892,
	"DCGM_FI_DEV_NVLINK_RX_BANDWIDTH_L14":    893,
	"DCGM_FI_DEV_NVLINK_RX_BANDWIDTH_L15":    894,
	"DCGM_FI_DEV_NVLINK_RX_BANDWIDTH_L16":    895,
	"DCGM_FI_DEV_NVLINK_RX_BANDWIDTH_L17":    896,
	"DCGM_FI_DEV_NVLINK_RX_BANDWIDTH_L2":     881,
	"DCGM_FI_DEV_NVLINK_RX_BANDWIDTH_L3":     882,
	"DCGM_FI_DEV_NVLINK_RX_BANDWIDTH_L4":     883,
	"DCGM_FI_DEV_NVLINK_RX_BANDWIDTH_L5":     884,
	"DCGM_FI_DEV_NVLINK_RX_BANDWIDTH_L6":     885,
	"DCGM_FI_DEV_NVLINK_RX_BANDWIDTH_L7":     886,
	"DCGM_FI_DEV_NVLINK_RX_BANDWIDTH_L8":     887,
	"DCGM_FI_DEV_NVLINK_RX_BANDWIDTH_L9":     888,
	"DCGM_FI_DEV_NVLINK_RX_BANDWIDTH_TOTAL":  897,
	"DCGM_FI_DEV_NVLINK_TX_BANDWIDTH_L0":     825,
	"DCGM_FI_DEV_NVLINK_TX_BANDWIDTH_L1":     826,
	"DCGM_FI_DEV_NVLINK_TX_BANDWIDTH_L10":    835,
	"DCGM_FI_DEV_NVLINK_TX_BANDWIDTH_L11":    836,
	"DCGM_FI_DEV_NVLINK_TX_BANDWIDTH_L12":    837,
	"DCGM_FI_DEV_NVLINK_TX_BANDWIDTH_L13":    838,
	"DCGM_FI_DEV_NVLINK_TX_BANDWIDTH_L14":    839,
	"DCGM_FI_DEV_NVLINK_TX_BANDWIDTH_L15":    840,
	"DCGM_FI_DEV_NVLINK_TX_BANDWIDTH_L16":    841,
	"DCGM_FI_DEV_NVLINK_TX_BANDWIDTH_L17":    842,
	"DCGM_FI_DEV_NVLINK_TX_BANDWIDTH_L2":     827,
	"DCGM_FI_DEV_NVLINK_TX_BANDWIDTH_L3":     828,
	"DCGM_FI_DEV_NVLINK_TX_BANDWIDTH_L4":     829,
	"DCGM_FI_DEV_NVLINK_TX_BANDWIDTH_L5":     830,
	"DCGM_FI_DEV_NVLINK_TX_BANDWIDTH_L6":     831,
	"DCGM_FI_DEV_NVLINK_TX_BANDWIDTH_L7":     832,
	"DCGM_FI_DEV_NVLINK_TX_BANDWIDTH_L8":     833,
	"DCGM_FI_DEV_NVLINK_TX_BANDWIDTH_L9":     834,
	"DCGM_FI_DEV_NVLINK_TX_BANDWIDTH_TOTAL":  843,
	"dcgm_board_limit_violation":             243,
	"dcgm_dec_utilization":                   207,
	"dcgm_ecc_dbe_aggregate_total":           313,
	"dcgm_ecc_dbe_volatile_total":            311,
	"dcgm_ecc_sbe_aggregate_total":           312,
	"dcgm_ecc_sbe_volatile_total":            310,
	"dcgm_enc_utilization":                   206,
	"dcgm_fb_free":                           251,
	"dcgm_fb_used":                           252,
	"dcgm_fi_prof_dram_active":               1005,
	"dcgm_fi_prof_gr_engine_active":          1001,
	"dcgm_fi_prof_pcie_rx_bytes":             1010,
	"dcgm_fi_prof_pcie_tx_bytes":             1009,
	"dcgm_fi_prof_pipe_tensor_active":        1004,
	"dcgm_fi_prof_sm_active":                 1002,
	"dcgm_fi_prof_sm_occupancy":              1003,
	"dcgm_gpu_temp":                          150,
	"dcgm_gpu_utilization":                   203,
	"dcgm_low_util_violation":                244,
	"dcgm_mem_copy_utilization":              204,
	"dcgm_memory_clock":                      101,
	"dcgm_memory_temp":                       140,
	"dcgm_nvlink_bandwidth_total":            449,
	"dcgm_nvlink_data_crc_error_count_total": 419,
	"dcgm_nvlink_flit_crc_error_count_total": 409,
	"dcgm_nvlink_recovery_error_count_total": 439,
	"dcgm_nvlink_replay_error_count_total":   429,
	"dcgm_pcie_replay_counter":               202,
	"dcgm_pcie_rx_throughput":                201,
	"dcgm_pcie_tx_throughput":                200,
	"dcgm_power_usage":                       155,
	"dcgm_power_violation":                   240,
	"dcgm_reliability_violation":             245,
	"dcgm_retired_pages_dbe":                 391,
	"dcgm_retired_pages_pending":             392,
	"dcgm_retired_pages_sbe":                 390,
	"dcgm_sm_clock":                          100,
	"dcgm_sync_boost_violation":              242,
	"dcgm_thermal_violation":                 241,
	"dcgm_total_energy_consumption":          156,
	"dcgm_xid_errors":                        230,
}

// GetFieldID returns the DCGM field ID for a given field name and whether it was found
// It first checks the current field IDs, then falls back to legacy field IDs if not found
func GetFieldID(fieldName string) (Short, bool) {
	// First check current field IDs
	if fieldID, ok := dcgmFields[fieldName]; ok {
		return fieldID, true
	}

	// Then check legacy field IDs
	if fieldID, ok := legacyDCGMFields[fieldName]; ok {
		return fieldID, true
	}

	return 0, false
}

// GetFieldIDOrPanic returns the DCGM field ID for a given field name
// It panics if the field name is not found in either current or legacy maps
func GetFieldIDOrPanic(fieldName string) Short {
	fieldID, ok := GetFieldID(fieldName)
	if !ok {
		panic("field name not found: " + fieldName)
	}
	return fieldID
}

// IsLegacyField returns true if the given field name is a legacy field
func IsLegacyField(fieldName string) bool {
	_, ok := legacyDCGMFields[fieldName]
	return ok
}

// IsCurrentField returns true if the given field name is a current field
func IsCurrentField(fieldName string) bool {
	_, ok := dcgmFields[fieldName]
	return ok
}
