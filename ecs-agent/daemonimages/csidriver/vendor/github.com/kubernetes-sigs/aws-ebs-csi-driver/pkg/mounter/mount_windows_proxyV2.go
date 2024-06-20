//go:build windows
// +build windows

/*
Copyright 2024 The Kubernetes Authors.

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

package mounter

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	diskv2 "github.com/kubernetes-csi/csi-proxy/v2/pkg/disk"
	diskapiv2 "github.com/kubernetes-csi/csi-proxy/v2/pkg/disk/hostapi"
	fsv2 "github.com/kubernetes-csi/csi-proxy/v2/pkg/filesystem"
	fsapiv2 "github.com/kubernetes-csi/csi-proxy/v2/pkg/filesystem/hostapi"
	volumev2 "github.com/kubernetes-csi/csi-proxy/v2/pkg/volume"
	volumeapiv2 "github.com/kubernetes-csi/csi-proxy/v2/pkg/volume/hostapi"
	"github.com/kubernetes-sigs/aws-ebs-csi-driver/pkg/util"
	"k8s.io/klog/v2"
	mountutils "k8s.io/mount-utils"
	utilexec "k8s.io/utils/exec"
)

type CSIProxyMounterV2 struct {
	FsClient     fsv2.Interface
	DiskClient   diskv2.Interface
	VolumeClient volumev2.Interface
}

// NewSafeMounterV2 returns a new instance of SafeFormatAndMount.
func NewSafeMounterV2() (*mountutils.SafeFormatAndMount, error) {
	fs, err := fsv2.New(fsapiv2.New())
	if err != nil {
		return nil, err
	}
	disk, err := diskv2.New(diskapiv2.New())
	if err != nil {
		return nil, err
	}
	volume, err := volumev2.New(volumeapiv2.New())
	if err != nil {
		return nil, err
	}
	return &mountutils.SafeFormatAndMount{
		Interface: &CSIProxyMounterV2{
			FsClient:     fs,
			DiskClient:   disk,
			VolumeClient: volume,
		},
		Exec: utilexec.New(),
	}, nil
}

// Mount just creates a soft link at target pointing to source.
func (mounter *CSIProxyMounterV2) Mount(source string, target string, fstype string, options []string) error {
	// Mount is called after the format is done.
	// TODO: Confirm that fstype is empty.
	linkRequest := &fsv2.CreateSymlinkRequest{
		SourcePath: util.NormalizeWindowsPath(source),
		TargetPath: util.NormalizeWindowsPath(target),
	}
	_, err := mounter.FsClient.CreateSymlink(context.Background(), linkRequest)
	if err != nil {
		return err
	}
	return nil
}

func (mounter *CSIProxyMounterV2) Unmount(target string) error {
	// Find the volume id
	getVolumeIdRequest := &volumev2.GetVolumeIDFromTargetPathRequest{
		TargetPath: util.NormalizeWindowsPath(target),
	}
	volumeIdResponse, err := mounter.VolumeClient.GetVolumeIDFromTargetPath(context.Background(), getVolumeIdRequest)
	if err != nil {
		return err
	}

	// Call UnmountVolume CSI proxy function which flushes data cache to disk and removes the global staging path
	unmountVolumeRequest := &volumev2.UnmountVolumeRequest{
		VolumeID:   volumeIdResponse.VolumeID,
		TargetPath: util.NormalizeWindowsPath(target),
	}
	_, err = mounter.VolumeClient.UnmountVolume(context.Background(), unmountVolumeRequest)
	if err != nil {
		return err
	}

	// Cleanup stage path
	err = mounter.Rmdir(target)
	if err != nil {
		return err
	}

	// Get disk number
	getDiskNumberRequest := &volumev2.GetDiskNumberFromVolumeIDRequest{
		VolumeID: volumeIdResponse.VolumeID,
	}
	getDiskNumberResponse, err := mounter.VolumeClient.GetDiskNumberFromVolumeID(context.Background(), getDiskNumberRequest)
	if err != nil {
		return err
	}

	// Offline the disk
	setDiskStateRequest := &diskv2.SetDiskStateRequest{
		DiskNumber: getDiskNumberResponse.DiskNumber,
		IsOnline:   false,
	}
	_, err = mounter.DiskClient.SetDiskState(context.Background(), setDiskStateRequest)
	if err != nil {
		return err
	}
	klog.V(4).InfoS("Successfully unmounted volume", "diskNumber", getDiskNumberResponse.DiskNumber, "volumeId", volumeIdResponse.VolumeID, "target", target)
	return nil
}

// Rmdir - delete the given directory
func (mounter *CSIProxyMounterV2) Rmdir(path string) error {
	rmdirRequest := &fsv2.RmdirRequest{
		Path:  util.NormalizeWindowsPath(path),
		Force: true,
	}
	_, err := mounter.FsClient.Rmdir(context.Background(), rmdirRequest)
	if err != nil {
		return err
	}
	return nil
}

func (mounter *CSIProxyMounterV2) WriteVolumeCache(target string) {
	request := &volumev2.GetVolumeIDFromTargetPathRequest{TargetPath: util.NormalizeWindowsPath(target)}
	response, err := mounter.VolumeClient.GetVolumeIDFromTargetPath(context.Background(), request)
	if err != nil || response == nil {
		klog.InfoS("GetVolumeIDFromTargetPath failed", "target", target, "err", err, "response", response)
	} else {
		request := &volumev2.WriteVolumeCacheRequest{
			VolumeID: response.VolumeID,
		}
		if res, err := mounter.VolumeClient.WriteVolumeCache(context.Background(), request); err != nil {
			klog.InfoS("WriteVolumeCache failed", "volumeID", response.VolumeID, "err", err, "res", res)
		}
	}
}

func (mounter *CSIProxyMounterV2) List() ([]mountutils.MountPoint, error) {
	return []mountutils.MountPoint{}, fmt.Errorf("List not implemented for CSIProxyMounter")
}

func (mounter *CSIProxyMounterV2) IsMountPointMatch(mp mountutils.MountPoint, dir string) bool {
	return mp.Path == dir
}

// IsMountPoint: determines if a directory is a mountpoint.
func (mounter *CSIProxyMounterV2) IsMountPoint(file string) (bool, error) {
	isNotMnt, err := mounter.IsLikelyNotMountPoint(file)
	if err != nil {
		return false, err
	}
	return !isNotMnt, nil
}

// IsLikelyMountPoint - If the directory does not exists, the function will return os.ErrNotExist error.
//
//	If the path exists, call to CSI proxy will check if its a link, if its a link then existence of target
//	path is checked.
func (mounter *CSIProxyMounterV2) IsLikelyNotMountPoint(path string) (bool, error) {
	isExists, err := mounter.ExistsPath(path)
	if err != nil {
		return false, err
	}

	if !isExists {
		return true, os.ErrNotExist
	}

	response, err := mounter.FsClient.IsSymlink(context.Background(),
		&fsv2.IsSymlinkRequest{
			Path: util.NormalizeWindowsPath(path),
		})
	if err != nil {
		return false, err
	}
	return !response.IsSymlink, nil
}

func (mounter *CSIProxyMounterV2) PathIsDevice(pathname string) (bool, error) {
	return false, fmt.Errorf("PathIsDevice not implemented for CSIProxyMounter")
}

func (mounter *CSIProxyMounterV2) DeviceOpened(pathname string) (bool, error) {
	return false, fmt.Errorf("DeviceOpened not implemented for CSIProxyMounter")
}

// GetDeviceNameFromMount returns the disk number for a mount path.
func (mounter *CSIProxyMounterV2) GetDeviceNameFromMount(mountPath, _ string) (string, error) {
	req := &volumev2.GetVolumeIDFromTargetPathRequest{TargetPath: util.NormalizeWindowsPath(mountPath)}
	resp, err := mounter.VolumeClient.GetVolumeIDFromTargetPath(context.Background(), req)
	if err != nil {
		return "", err
	}
	// Get disk number
	getDiskNumberRequest := &volumev2.GetDiskNumberFromVolumeIDRequest{
		VolumeID: resp.VolumeID,
	}
	getDiskNumberResponse, err := mounter.VolumeClient.GetDiskNumberFromVolumeID(context.Background(), getDiskNumberRequest)
	if err != nil {
		return "", err
	}
	klog.V(4).InfoS("GetDeviceNameFromMount called", "diskNumber", getDiskNumberResponse.DiskNumber, "volumeID", resp.VolumeID, "mountPath", mountPath)
	return fmt.Sprint(getDiskNumberResponse.DiskNumber), nil
}

func (mounter *CSIProxyMounterV2) MakeRShared(path string) error {
	return fmt.Errorf("MakeRShared not implemented for CSIProxyMounter")
}

func (mounter *CSIProxyMounterV2) MakeFile(pathname string) error {
	return fmt.Errorf("MakeFile not implemented for CSIProxyMounter")
}

// MakeDir - Creates a directory. The CSI proxy takes in context information.
// Currently the make dir is only used from the staging code path, hence we call it
// with Plugin context..
func (mounter *CSIProxyMounterV2) MakeDir(pathname string) error {
	mkdirReq := &fsv2.MkdirRequest{
		Path: util.NormalizeWindowsPath(pathname),
	}
	_, err := mounter.FsClient.Mkdir(context.Background(), mkdirReq)
	if err != nil {
		klog.V(4).InfoS("Error", err)
		return err
	}

	return nil
}

// ExistsPath - Checks if a path exists. Unlike util ExistsPath, this call does not perform follow link.
func (mounter *CSIProxyMounterV2) ExistsPath(path string) (bool, error) {
	isExistsResponse, err := mounter.FsClient.PathExists(context.Background(),
		&fsv2.PathExistsRequest{
			Path: util.NormalizeWindowsPath(path),
		})
	if err != nil {
		return false, err
	}
	return isExistsResponse.Exists, err
}

func (mounter *CSIProxyMounterV2) EvalHostSymlinks(pathname string) (string, error) {
	return "", fmt.Errorf("EvalHostSymlinks is not implemented for CSIProxyMounter")
}

func (mounter *CSIProxyMounterV2) GetMountRefs(pathname string) ([]string, error) {
	return []string{}, fmt.Errorf("GetMountRefs is not implemented for CSIProxyMounter")
}

func (mounter *CSIProxyMounterV2) GetFSGroup(pathname string) (int64, error) {
	return -1, fmt.Errorf("GetFSGroup is not implemented for CSIProxyMounter")
}

func (mounter *CSIProxyMounterV2) GetSELinuxSupport(pathname string) (bool, error) {
	return false, fmt.Errorf("GetSELinuxSupport is not implemented for CSIProxyMounter")
}

func (mounter *CSIProxyMounterV2) GetMode(pathname string) (os.FileMode, error) {
	return 0, fmt.Errorf("GetMode is not implemented for CSIProxyMounter")
}

func (mounter *CSIProxyMounterV2) MountSensitive(source string, target string, fstype string, options []string, sensitiveOptions []string) error {
	return fmt.Errorf("MountSensitive is not implemented for CSIProxyMounter")
}

func (mounter *CSIProxyMounterV2) MountSensitiveWithoutSystemd(source string, target string, fstype string, options []string, sensitiveOptions []string) error {
	return fmt.Errorf("MountSensitiveWithoutSystemd is not implemented for CSIProxyMounter")
}

func (mounter *CSIProxyMounterV2) MountSensitiveWithoutSystemdWithMountFlags(source string, target string, fstype string, options []string, sensitiveOptions []string, mountFlags []string) error {
	return fmt.Errorf("MountSensitiveWithoutSystemdWithMountFlags is not implemented for CSIProxyMounter")
}

// Rescan would trigger an update storage cache via the CSI proxy.
func (mounter *CSIProxyMounterV2) Rescan() error {
	// Call Rescan from disk APIs of CSI Proxy.
	if _, err := mounter.DiskClient.Rescan(context.Background(), &diskv2.RescanRequest{}); err != nil {
		return err
	}
	return nil
}

// FindDiskByLun - given a lun number, find out the corresponding disk
func (mounter *CSIProxyMounterV2) FindDiskByLun(lun string) (diskNum string, err error) {
	findDiskByLunResponse, err := mounter.DiskClient.ListDiskLocations(context.Background(), &diskv2.ListDiskLocationsRequest{})
	if err != nil {
		return "", err
	}

	// List all disk locations and match the lun id being requested for.
	// If match is found then return back the disk number.
	for diskID, location := range findDiskByLunResponse.DiskLocations {
		if strings.EqualFold(location.LUNID, lun) {
			return strconv.Itoa(int(diskID)), nil
		}
	}
	return "", fmt.Errorf("could not find disk id for lun: %s", lun)
}

// FormatAndMount - accepts the source disk number, target path to mount, the fstype to format with and options to be used.
func (mounter *CSIProxyMounterV2) FormatAndMountSensitiveWithFormatOptions(source string, target string, fstype string, options []string, sensitiveOptions []string, formatOptions []string) error {
	// sensitiveOptions is not supported on Windows because we have no reasonable way to control what the csi-proxy does
	if len(sensitiveOptions) > 0 {
		return errors.New("sensitiveOptions not supported on Windows!")
	}
	// formatOptions is not supported on Windows because the csi-proxy does not allow supplying format arguments
	// This limitation will be addressed in the future with privileged Windows containers
	if len(formatOptions) > 0 {
		return errors.New("formatOptions not supported on Windows!")
	}

	diskNumber, err := strconv.Atoi(source)
	if err != nil {
		return err
	}

	// Call PartitionDisk CSI proxy call to partition the disk and return the volume id
	partionDiskRequest := &diskv2.PartitionDiskRequest{
		DiskNumber: uint32(diskNumber),
	}
	_, err = mounter.DiskClient.PartitionDisk(context.Background(), partionDiskRequest)
	if err != nil {
		return err
	}

	// Ensure the disk is online before mounting.
	setDiskStateRequest := &diskv2.SetDiskStateRequest{
		DiskNumber: uint32(diskNumber),
		IsOnline:   true,
	}
	_, err = mounter.DiskClient.SetDiskState(context.Background(), setDiskStateRequest)
	if err != nil {
		return err
	}

	// List the volumes on the given disk.
	volumeIDsRequest := &volumev2.ListVolumesOnDiskRequest{
		DiskNumber: uint32(diskNumber),
	}
	volumeIdResponse, err := mounter.VolumeClient.ListVolumesOnDisk(context.Background(), volumeIDsRequest)
	if err != nil {
		return err
	}

	// TODO: consider partitions and choose the right partition.
	// For now just choose the first volume.
	volumeID := volumeIdResponse.VolumeIDs[0]

	// Check if the volume is formatted.
	isVolumeFormattedRequest := &volumev2.IsVolumeFormattedRequest{
		VolumeID: volumeID,
	}
	isVolumeFormattedResponse, err := mounter.VolumeClient.IsVolumeFormatted(context.Background(), isVolumeFormattedRequest)
	if err != nil {
		return err
	}

	// If the volume is not formatted, then format it, else proceed to mount.
	if !isVolumeFormattedResponse.Formatted {
		formatVolumeRequest := &volumev2.FormatVolumeRequest{
			VolumeID: volumeID,
			// TODO: Accept the filesystem and other options
		}
		_, err = mounter.VolumeClient.FormatVolume(context.Background(), formatVolumeRequest)
		if err != nil {
			return err
		}
	}

	// Mount the volume by calling the CSI proxy call.
	mountVolumeRequest := &volumev2.MountVolumeRequest{
		VolumeID:   volumeID,
		TargetPath: util.NormalizeWindowsPath(target),
	}
	_, err = mounter.VolumeClient.MountVolume(context.Background(), mountVolumeRequest)
	if err != nil {
		return err
	}
	return nil
}

// ResizeVolume resizes the volume at given mount path
func (mounter *CSIProxyMounterV2) ResizeVolume(deviceMountPath string) (bool, error) {
	// Find the volume id
	getVolumeIdRequest := &volumev2.GetVolumeIDFromTargetPathRequest{
		TargetPath: util.NormalizeWindowsPath(deviceMountPath),
	}
	volumeIdResponse, err := mounter.VolumeClient.GetVolumeIDFromTargetPath(context.Background(), getVolumeIdRequest)
	if err != nil {
		return false, err
	}
	volumeId := volumeIdResponse.VolumeID

	// Resize volume
	resizeVolumeRequest := &volumev2.ResizeVolumeRequest{
		VolumeID: volumeId,
	}
	_, err = mounter.VolumeClient.ResizeVolume(context.Background(), resizeVolumeRequest)
	if err != nil {
		return false, err
	}

	return true, nil
}

// GetVolumeSizeInBytes returns the size of the volume in bytes
func (mounter *CSIProxyMounterV2) GetVolumeSizeInBytes(deviceMountPath string) (int64, error) {
	// Find the volume id
	getVolumeIdRequest := &volumev2.GetVolumeIDFromTargetPathRequest{
		TargetPath: util.NormalizeWindowsPath(deviceMountPath),
	}
	volumeIdResponse, err := mounter.VolumeClient.GetVolumeIDFromTargetPath(context.Background(), getVolumeIdRequest)
	if err != nil {
		return -1, err
	}
	volumeId := volumeIdResponse.VolumeID

	// Get size of the volume
	getVolumeStatsRequest := &volumev2.GetVolumeStatsRequest{
		VolumeID: volumeId,
	}
	resp, err := mounter.VolumeClient.GetVolumeStats(context.Background(), getVolumeStatsRequest)
	if err != nil {
		return -1, err
	}

	return resp.TotalBytes, nil
}

// GetDeviceSize returns the size of the disk in bytes
func (mounter *CSIProxyMounterV2) GetDeviceSize(devicePath string) (int64, error) {
	diskNumber, err := strconv.Atoi(devicePath)
	if err != nil {
		return -1, err
	}

	//Get size of the disk
	getDiskStatsRequest := &diskv2.GetDiskStatsRequest{
		DiskNumber: uint32(diskNumber),
	}
	resp, err := mounter.DiskClient.GetDiskStats(context.Background(), getDiskStatsRequest)
	if err != nil {
		return -1, err
	}

	return resp.TotalBytes, nil
}

func (mounter *CSIProxyMounterV2) CanSafelySkipMountPointCheck() bool {
	return false
}

func (mounter *CSIProxyMounterV2) FindDevicePath(devicePath, volumeID, _, _ string) (string, error) {
	response, err := mounter.DiskClient.ListDiskIDs(context.TODO(), &diskv2.ListDiskIDsRequest{})
	if err != nil {
		return "", fmt.Errorf("error listing disk ids: %q", err)
	}

	diskIDs := response.DiskIDs

	foundDiskNumber := ""
	for diskNumber, diskID := range diskIDs {
		serialNumber := diskID.SerialNumber
		cleanVolumeID := strings.ReplaceAll(volumeID, "-", "")
		if strings.Contains(serialNumber, cleanVolumeID) {
			foundDiskNumber = strconv.Itoa(int(diskNumber))
			break
		}
	}

	if foundDiskNumber == "" {
		return "", fmt.Errorf("disk number for device path %q volume id %q not found", devicePath, volumeID)
	}

	return foundDiskNumber, nil
}
