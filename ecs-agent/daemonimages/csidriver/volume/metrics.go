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

package volume

import (
	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/aws/amazon-ecs-agent/ecs-agent/daemonimages/csidriver/util/fs"
)

// MetricsProvider exposes metrics (e.g. used,available space) related to a
// Volume.
type MetricsProvider interface {
	// GetMetrics returns the Metrics for the Volume. Maybe expensive for
	// some implementations.
	GetMetrics() (*Metrics, error)
}

// metricsStatFS represents a MetricsProvider that calculates the used and available
// Volume space by stat'ing and gathering filesystem info for the Volume path.
type metricsStatFS struct {
	// the directory path the volume is mounted to.
	path string
}

// NewMetricsStatfs creates a new metricsStatFS with the Volume path.
func NewMetricsStatFS(path string) MetricsProvider {
	return &metricsStatFS{path}
}

// See MetricsProvider.GetMetrics
// GetMetrics calculates the volume usage and device free space by executing "du"
// and gathering filesystem info for the Volume path.
func (md *metricsStatFS) GetMetrics() (*Metrics, error) {
	metrics := &Metrics{}
	if md.path == "" {
		return metrics, NewNoPathDefinedError()
	}

	err := md.getFsInfo(metrics)
	if err != nil {
		return metrics, err
	}

	return metrics, nil
}

// getFsInfo writes metrics.Capacity, metrics.Used and metrics.Available from the filesystem info
func (md *metricsStatFS) getFsInfo(metrics *Metrics) error {
	available, capacity, usage, inodes, inodesFree, inodesUsed, err := fs.Info(md.path)
	if err != nil {
		return NewFsInfoFailedError(err)
	}

	metrics.Available = resource.NewQuantity(available, resource.BinarySI)
	metrics.Capacity = resource.NewQuantity(capacity, resource.BinarySI)
	metrics.Used = resource.NewQuantity(usage, resource.BinarySI)
	metrics.Inodes = resource.NewQuantity(inodes, resource.BinarySI)
	metrics.InodesFree = resource.NewQuantity(inodesFree, resource.BinarySI)
	metrics.InodesUsed = resource.NewQuantity(inodesUsed, resource.BinarySI)
	return nil
}

// Metrics represents the used and available bytes of the Volume.
type Metrics struct {
	// Used represents the total bytes used by the Volume.
	// Note: For block devices this maybe more than the total size of the files.
	Used *resource.Quantity

	// Capacity represents the total capacity (bytes) of the volume's
	// underlying storage. For Volumes that share a filesystem with the host
	// (e.g. emptydir, hostpath) this is the size of the underlying storage,
	// and will not equal Used + Available as the fs is shared.
	Capacity *resource.Quantity

	// Available represents the storage space available (bytes) for the
	// Volume. For Volumes that share a filesystem with the host (e.g.
	// emptydir, hostpath), this is the available space on the underlying
	// storage, and is shared with host processes and other Volumes.
	Available *resource.Quantity

	// InodesUsed represents the total inodes used by the Volume.
	InodesUsed *resource.Quantity

	// Inodes represents the total number of inodes available in the volume.
	// For volumes that share a filesystem with the host (e.g. emptydir, hostpath),
	// this is the inodes available in the underlying storage,
	// and will not equal InodesUsed + InodesFree as the fs is shared.
	Inodes *resource.Quantity

	// InodesFree represent the inodes available for the volume.  For Volumes that share
	// a filesystem with the host (e.g. emptydir, hostpath), this is the free inodes
	// on the underlying storage, and is shared with host processes and other volumes
	InodesFree *resource.Quantity

	// Normal volumes are available for use and operating optimally.
	// An abnormal volume does not meet these criteria.
	// This field is OPTIONAL. Only some csi drivers which support NodeServiceCapability_RPC_VOLUME_CONDITION
	// need to fill it.
	Abnormal *bool

	// The message describing the condition of the volume.
	// This field is OPTIONAL. Only some csi drivers which support capability_RPC_VOLUME_CONDITION
	// need to fill it.
	Message *string
}
