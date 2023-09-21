// Copyright Amazon.com Inc. or its affiliates. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License"). You may
// not use this file except in compliance with the License. A copy of the
// License is located at
//
//      http://aws.amazon.com/apache2.0/
//
// or in the "license" file accompanying this file. This file is distributed
// on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either
// express or implied. See the License for the specific language governing
// permissions and limitations under the License.

package driver

// constants of keys in VolumeContext
const (
	// VolumeAttributePartition represents key for partition config in VolumeContext
	// this represents the partition number on a device used to mount
	VolumeAttributePartition = "partition"
)

// constants for fstypes
const (
	// FSTypeExt2 represents the ext2 filesystem type
	FSTypeExt2 = "ext2"
	// FSTypeExt3 represents the ext3 filesystem type
	FSTypeExt3 = "ext3"
	// FSTypeExt4 represents the ext4 filesystem type
	FSTypeExt4 = "ext4"
	// FSTypeXfs represents the xfs filesystem type
	FSTypeXfs = "xfs"
	// FSTypeNtfs represents the ntfs filesystem type
	FSTypeNtfs = "ntfs"
)

// constants of disk partition suffix
const (
	diskPartitionSuffix     = ""
	nvmeDiskPartitionSuffix = "p"
)

// constants of keys in volume parameters
const (
	// BlockSizeKey configures the block size when formatting a volume
	BlockSizeKey = "blocksize"

	// INodeSizeKey configures the inode size when formatting a volume
	INodeSizeKey = "inodesize"

	// BytesPerINodeKey configures the `bytes-per-inode` when formatting a volume
	BytesPerINodeKey = "bytesperinode"

	// NumberOfINodesKey configures the `number-of-inodes` when formatting a volume
	NumberOfINodesKey = "numberofinodes"
)

type fileSystemConfig struct {
	NotSupportedParams map[string]struct{}
}

// constants of keys in PublishContext
const (
	// devicePathKey represents key for device path in PublishContext
	// devicePath is the device path where the volume is attached to
	DevicePathKey = "devicePath"
)

var (
	FileSystemConfigs = map[string]fileSystemConfig{
		FSTypeExt2: {
			NotSupportedParams: map[string]struct{}{},
		},
		FSTypeExt3: {
			NotSupportedParams: map[string]struct{}{},
		},
		FSTypeExt4: {
			NotSupportedParams: map[string]struct{}{},
		},
		FSTypeXfs: {
			NotSupportedParams: map[string]struct{}{
				BytesPerINodeKey:  {},
				NumberOfINodesKey: {},
			},
		},
		FSTypeNtfs: {
			NotSupportedParams: map[string]struct{}{
				BlockSizeKey:      {},
				INodeSizeKey:      {},
				BytesPerINodeKey:  {},
				NumberOfINodesKey: {},
			},
		},
	}
)

func (fsConfig fileSystemConfig) isParameterSupported(paramName string) bool {
	_, notSupported := fsConfig.NotSupportedParams[paramName]
	return !notSupported
}
