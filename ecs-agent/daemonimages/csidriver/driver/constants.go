// this file has been modified from its original found in:
// https://github.com/kubernetes-sigs/aws-ebs-csi-driver

/*
Copyright 2019 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package driver

// constants of keys in VolumeContext
const (
	// VolumeAttributePartition represents key for partition config in VolumeContext
	// this represents the partition number on a device used to mount
	VolumeAttributePartition = "partition"
)

// constants for disk block size
const (
	//DefaultBlockSize represents the default block size (4KB)
	DefaultBlockSize = 4096
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

	// InodeSizeKey configures the inode size when formatting a volume
	InodeSizeKey = "inodesize"

	// BytesPerInodeKey configures the `bytes-per-inode` when formatting a volume
	BytesPerInodeKey = "bytesperinode"

	// NumberOfInodesKey configures the `number-of-inodes` when formatting a volume
	NumberOfInodesKey = "numberofinodes"

	// Ext4ClusterSizeKey enables the bigalloc option when formatting an ext4 volume
	Ext4BigAllocKey = "ext4bigalloc"

	// Ext4ClusterSizeKey configures the cluster size when formatting an ext4 volume with the bigalloc option enabled
	Ext4ClusterSizeKey = "ext4clustersize"
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
				BytesPerInodeKey:  {},
				NumberOfInodesKey: {},
			},
		},
		FSTypeNtfs: {
			NotSupportedParams: map[string]struct{}{
				BlockSizeKey:      {},
				InodeSizeKey:      {},
				BytesPerInodeKey:  {},
				NumberOfInodesKey: {},
			},
		},
	}
)

func (fsConfig fileSystemConfig) isParameterSupported(paramName string) bool {
	_, notSupported := fsConfig.NotSupportedParams[paramName]
	return !notSupported
}
