// Copyright Amazon.com Inc. or its affiliates. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License"). You may
// not use this file except in compliance with the License. A copy of the
// License is located at
//
//	http://aws.amazon.com/apache2.0/
//
// or in the "license" file accompanying this file. This file is distributed
// on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either
// express or implied. See the License for the specific language governing
// permissions and limitations under the License.

package driver

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/klog/v2"

	"github.com/aws/amazon-ecs-agent/ecs-agent/daemonimages/csidriver/driver/internal"
	"github.com/aws/amazon-ecs-agent/ecs-agent/daemonimages/csidriver/util"
	"github.com/aws/amazon-ecs-agent/ecs-agent/daemonimages/csidriver/volume"
)

const (
	// default file system type to be used when it is not provided
	defaultFsType = FSTypeXfs

	VolumeOperationAlreadyExists = "An operation with the given volume=%q is already in progress"
)

var (
	ValidFSTypes = map[string]struct{}{
		FSTypeExt2: {},
		FSTypeExt3: {},
		FSTypeExt4: {},
		FSTypeXfs:  {},
		FSTypeNtfs: {},
	}
)

// nodeService represents the node service of CSI driver.
type nodeService struct {
	mounter Mounter
	// UnimplementedNodeServer implements all interfaces with empty implementation. As one mini version of csi driver,
	// we only need to override the necessary interfaces.
	csi.UnimplementedNodeServer
	inFlight         *internal.InFlight
	deviceIdentifier DeviceIdentifier
}

func (d *nodeService) NodeStageVolume(ctx context.Context, req *csi.NodeStageVolumeRequest) (*csi.NodeStageVolumeResponse, error) {
	klog.V(4).InfoS("NodeStageVolume: called", "args", *req)

	volumeID := req.GetVolumeId()
	if len(volumeID) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Volume ID not provided")
	}

	target := req.GetStagingTargetPath()
	if len(target) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Staging target not provided")
	}

	volCap := req.GetVolumeCapability()
	if volCap == nil {
		return nil, status.Error(codes.InvalidArgument, "Volume capability not provided")
	}

	if !isValidVolumeCapabilities([]*csi.VolumeCapability{volCap}) {
		return nil, status.Error(codes.InvalidArgument, "Volume capability not supported")
	}
	volumeContext := req.GetVolumeContext()
	if isValidVolumeContext := isValidVolumeContext(volumeContext); !isValidVolumeContext {
		return nil, status.Error(codes.InvalidArgument, "Volume Attribute is not valid")
	}

	// If the access type is block, do nothing for stage
	switch volCap.GetAccessType().(type) {
	case *csi.VolumeCapability_Block:
		return &csi.NodeStageVolumeResponse{}, nil
	}

	mountVolume := volCap.GetMount()
	if mountVolume == nil {
		return nil, status.Error(codes.InvalidArgument, "NodeStageVolume: mount is nil within volume capability")
	}

	fsType := mountVolume.GetFsType()
	if len(fsType) == 0 {
		fsType = defaultFsType
	}

	_, ok := ValidFSTypes[strings.ToLower(fsType)]
	if !ok {
		return nil, status.Errorf(codes.InvalidArgument, "NodeStageVolume: invalid fstype %s", fsType)
	}

	context := req.GetVolumeContext()

	blockSize, err := recheckParameter(context, BlockSizeKey, FileSystemConfigs, fsType)
	if err != nil {
		return nil, err
	}
	inodeSize, err := recheckParameter(context, INodeSizeKey, FileSystemConfigs, fsType)
	if err != nil {
		return nil, err
	}
	bytesPerINode, err := recheckParameter(context, BytesPerINodeKey, FileSystemConfigs, fsType)
	if err != nil {
		return nil, err
	}
	numINodes, err := recheckParameter(context, NumberOfINodesKey, FileSystemConfigs, fsType)
	if err != nil {
		return nil, err
	}

	mountOptions := collectMountOptions(fsType, mountVolume.MountFlags)

	if ok = d.inFlight.Insert(volumeID); !ok {
		return nil, status.Errorf(codes.Aborted, VolumeOperationAlreadyExists, volumeID)
	}
	defer func() {
		klog.V(4).InfoS("NodeStageVolume: volume operation finished", "volumeID", volumeID)
		d.inFlight.Delete(volumeID)
	}()

	devicePath, ok := req.PublishContext[DevicePathKey]
	if !ok {
		return nil, status.Error(codes.InvalidArgument, "Device path not provided")
	}

	partition := ""
	if part, ok := volumeContext[VolumeAttributePartition]; ok {
		if part != "0" {
			partition = part
		} else {
			klog.InfoS("NodeStageVolume: invalid partition config, will ignore.", "partition", part)
		}
	}

	source, err := d.findDevicePath(devicePath, volumeID, partition)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to find device path %s. %v", devicePath, err)
	}

	exists, err := d.mounter.PathExists(target)
	klog.InfoS("NodeStageVolume: path exists:", "exists", exists)
	if err != nil {
		klog.InfoS("NodeStageVolume: path exists:", "err", err)
		msg := fmt.Sprintf("failed to check if target %q exists: %v", target, err)
		return nil, status.Error(codes.Internal, msg)
	}
	// When exists is true it means target path was created but device isn't mounted.
	// We don't want to do anything in that case and let the operation proceed.
	// Otherwise we need to create the target directory.
	if !exists {
		// If target path does not exist we need to create the directory where volume will be staged
		klog.InfoS("NodeStageVolume: creating target dir", "target", target)
		if err = d.mounter.MakeDir(target); err != nil {
			msg := fmt.Sprintf("could not create target dir %q: %v", target, err)
			return nil, status.Error(codes.Internal, msg)
		}
	}

	// Check if a device is mounted in target directory
	device, _, err := d.mounter.GetDeviceNameFromMount(target)
	klog.InfoS("NodeStageVolume: find device path", "device", device)
	if err != nil {
		msg := fmt.Sprintf("failed to check if volume is already mounted: %v", err)
		return nil, status.Error(codes.Internal, msg)
	}

	// This operation (NodeStageVolume) MUST be idempotent.
	// If the volume corresponding to the volume_id is already staged to the staging_target_path,
	// and is identical to the specified volume_capability the Plugin MUST reply 0 OK.
	klog.InfoS("NodeStageVolume: checking if volume is already staged", "device", device, "source", source, "target", target)
	if device == source {
		klog.InfoS("NodeStageVolume: volume already staged", "volumeID", volumeID)
		return &csi.NodeStageVolumeResponse{}, nil
	}

	// FormatAndMount will format only if needed
	klog.InfoS("NodeStageVolume: staging volume", "source", source, "volumeID", volumeID, "target", target, "fstype", fsType)
	formatOptions := []string{}
	if len(blockSize) > 0 {
		if fsType == FSTypeXfs {
			blockSize = "size=" + blockSize
		}
		formatOptions = append(formatOptions, "-b", blockSize)
	}
	if len(inodeSize) > 0 {
		option := "-I"
		if fsType == FSTypeXfs {
			option, inodeSize = "-i", "size="+inodeSize
		}
		formatOptions = append(formatOptions, option, inodeSize)
	}
	if len(bytesPerINode) > 0 {
		formatOptions = append(formatOptions, "-i", bytesPerINode)
	}
	if len(numINodes) > 0 {
		formatOptions = append(formatOptions, "-N", numINodes)
	}
	err = d.mounter.FormatAndMountSensitiveWithFormatOptions(source, target, fsType, mountOptions, nil, formatOptions)
	if err != nil {
		klog.InfoS("NodeStageVolume: format mount fail", "error", err)
		msg := fmt.Sprintf("could not format %q and mount it at %q: %v", source, target, err)
		return nil, status.Error(codes.Internal, msg)
	}

	needResize, err := d.mounter.NeedResize(source, target)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Could not determine if volume %q (%q) need to be resized:  %v", req.GetVolumeId(), source, err)
	}

	if needResize {
		r, err := d.mounter.NewResizeFs()
		if err != nil {
			return nil, status.Errorf(codes.Internal, "Error attempting to create new ResizeFs:  %v", err)
		}
		klog.V(2).InfoS("Volume needs resizing", "source", source)
		if _, err := r.Resize(source, target); err != nil {
			return nil, status.Errorf(codes.Internal, "Could not resize volume %q (%q):  %v", volumeID, source, err)
		}
	}
	klog.InfoS("NodeStageVolume: successfully staged volume", "source", source, "volumeID", volumeID, "target", target, "fstype", fsType)
	return &csi.NodeStageVolumeResponse{}, nil
}

func newNodeService() nodeService {
	klog.V(4).InfoS("New node service")
	nodeMounter, err := newNodeMounter()
	if err != nil {
		panic(err)
	}

	return nodeService{
		mounter:          nodeMounter,
		deviceIdentifier: newNodeDeviceIdentifier(),
		inFlight:         internal.NewInFlight(),
	}
}

func (d *nodeService) NodeGetVolumeStats(ctx context.Context, req *csi.NodeGetVolumeStatsRequest) (*csi.NodeGetVolumeStatsResponse, error) {
	klog.InfoS("NodeGetVolumeStats: called")
	//klog.V(4).InfoS("NodeGetVolumeStats: called", "args", *req)
	if len(req.VolumeId) == 0 {
		return nil, status.Error(codes.InvalidArgument, "NodeGetVolumeStats volume ID was empty")
	}

	if len(req.VolumePath) == 0 {
		return nil, status.Error(codes.InvalidArgument, "NodeGetVolumeStats volume path was empty")
	}

	exists, err := d.mounter.PathExists(req.VolumePath)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "unknown error when stat on %s: %v", req.VolumePath, err)
	}
	if !exists {
		return nil, status.Errorf(codes.NotFound, "path %s does not exist", req.VolumePath)
	}

	isBlock, err := util.IsBlockDevice(req.VolumePath)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to determine whether %s is block device: %v", req.VolumePath, err)
	}

	if isBlock {
		bcap, blockErr := d.getBlockSizeBytes(req.VolumePath)
		if blockErr != nil {
			return nil, status.Errorf(codes.Internal, "failed to get block capacity on path %s: %v", req.VolumePath, err)
		}
		return &csi.NodeGetVolumeStatsResponse{
			Usage: []*csi.VolumeUsage{
				{
					Unit:  csi.VolumeUsage_BYTES,
					Total: bcap,
				},
			},
		}, nil
	}

	metricsProvider := volume.NewMetricsStatFS(req.VolumePath)
	metrics, err := metricsProvider.GetMetrics()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get fs info on path %s: %v", req.VolumePath, err)
	}

	return &csi.NodeGetVolumeStatsResponse{
		Usage: []*csi.VolumeUsage{
			{
				Unit:      csi.VolumeUsage_BYTES,
				Available: metrics.Available.AsDec().UnscaledBig().Int64(),
				Total:     metrics.Capacity.AsDec().UnscaledBig().Int64(),
				Used:      metrics.Used.AsDec().UnscaledBig().Int64(),
			},
			{
				Unit:      csi.VolumeUsage_INODES,
				Available: metrics.InodesFree.AsDec().UnscaledBig().Int64(),
				Total:     metrics.Inodes.AsDec().UnscaledBig().Int64(),
				Used:      metrics.InodesUsed.AsDec().UnscaledBig().Int64(),
			},
		},
	}, nil
}

func recheckParameter(context map[string]string, key string, fsConfigs map[string]fileSystemConfig, fsType string) (value string, err error) {
	v, ok := context[key]
	if ok {
		// This check is already performed on the controller side
		// However, because it is potentially security-sensitive, we redo it here to be safe
		_, err := strconv.Atoi(v)
		if err != nil {
			return "", status.Errorf(codes.InvalidArgument, "Invalid %s (aborting!): %v", key, err)
		}

		// In the case that the default fstype does not support custom sizes we could
		// be using an invalid fstype, so recheck that here
		if supported := fsConfigs[strings.ToLower(fsType)].isParameterSupported(key); !supported {
			return "", status.Errorf(codes.InvalidArgument, "Cannot use %s with fstype %s", key, fsType)
		}
	}
	return v, nil
}

// collectMountOptions returns array of mount options from
// VolumeCapability_MountVolume and special mount options for
// given filesystem.
func collectMountOptions(fsType string, mntFlags []string) []string {
	var options []string
	for _, opt := range mntFlags {
		if !hasMountOption(options, opt) {
			options = append(options, opt)
		}
	}

	// By default, xfs does not allow mounting of two volumes with the same filesystem uuid.
	// Force ignore this uuid to be able to mount volume + its clone / restored snapshot on the same node.
	if fsType == FSTypeXfs {
		if !hasMountOption(options, "nouuid") {
			options = append(options, "nouuid")
		}
	}
	return options
}

// hasMountOption returns a boolean indicating whether the given
// slice already contains a mount option. This is used to prevent
// passing duplicate option to the mount command.
func hasMountOption(options []string, opt string) bool {
	for _, o := range options {
		if o == opt {
			return true
		}
	}
	return false
}
