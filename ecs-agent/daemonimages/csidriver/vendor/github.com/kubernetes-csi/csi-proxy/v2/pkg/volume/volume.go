package volume

import (
	"context"
	"fmt"

	volumeapi "github.com/kubernetes-csi/csi-proxy/v2/pkg/volume/hostapi"
	"k8s.io/klog/v2"
)

// Volume wraps the host API and implements the interface
type Volume struct {
	hostAPI volumeapi.HostAPI
}

type Interface interface {
	// FormatVolume formats a volume with NTFS.
	FormatVolume(context.Context, *FormatVolumeRequest) (*FormatVolumeResponse, error)

	// GetClosestVolumeIDFromTargetPath gets the closest volume id for a given target path
	// by following symlinks and moving up in the filesystem, if after moving up in the filesystem
	// we get to a DriveLetter then the volume corresponding to this drive letter is returned instead.
	GetClosestVolumeIDFromTargetPath(context.Context, *GetClosestVolumeIDFromTargetPathRequest) (*GetClosestVolumeIDFromTargetPathResponse, error)

	// GetDiskNumberFromVolumeID gets the disk number of the disk where the volume is located.
	GetDiskNumberFromVolumeID(context.Context, *GetDiskNumberFromVolumeIDRequest) (*GetDiskNumberFromVolumeIDResponse, error)

	// GetVolumeIDFromTargetPath gets the volume id for a given target path.
	GetVolumeIDFromTargetPath(context.Context, *GetVolumeIDFromTargetPathRequest) (*GetVolumeIDFromTargetPathResponse, error)

	// GetVolumeStats gathers total bytes and used bytes for a volume.
	GetVolumeStats(context.Context, *GetVolumeStatsRequest) (*GetVolumeStatsResponse, error)

	// IsVolumeFormatted checks if a volume is formatted.
	IsVolumeFormatted(context.Context, *IsVolumeFormattedRequest) (*IsVolumeFormattedResponse, error)

	// ListVolumesOnDisk returns the volume IDs (in \\.\Volume{GUID} format) for all volumes from a
	// given disk number and partition number (optional)
	ListVolumesOnDisk(context.Context, *ListVolumesOnDiskRequest) (*ListVolumesOnDiskResponse, error)

	// MountVolume mounts the volume at the requested global staging path.
	MountVolume(context.Context, *MountVolumeRequest) (*MountVolumeResponse, error)

	// ResizeVolume performs resizing of the partition and file system for a block based volume.
	ResizeVolume(context.Context, *ResizeVolumeRequest) (*ResizeVolumeResponse, error)

	// UnmountVolume flushes data cache to disk and removes the global staging path.
	UnmountVolume(context.Context, *UnmountVolumeRequest) (*UnmountVolumeResponse, error)

	// WriteVolumeCache write volume cache to disk.
	WriteVolumeCache(context.Context, *WriteVolumeCacheRequest) (*WriteVolumeCacheResponse, error)
}

var _ Interface = &Volume{}

func New(hostAPI volumeapi.HostAPI) (*Volume, error) {
	return &Volume{
		hostAPI: hostAPI,
	}, nil
}

func (v *Volume) ListVolumesOnDisk(context context.Context, request *ListVolumesOnDiskRequest) (*ListVolumesOnDiskResponse, error) {
	klog.V(2).Infof("ListVolumesOnDisk: Request: %+v", request)
	response := &ListVolumesOnDiskResponse{}

	volumeIDs, err := v.hostAPI.ListVolumesOnDisk(request.DiskNumber, request.PartitionNumber)
	if err != nil {
		klog.Errorf("failed ListVolumeOnDisk %v", err)
		return response, err
	}

	response.VolumeIDs = volumeIDs
	return response, nil
}

func (v *Volume) MountVolume(context context.Context, request *MountVolumeRequest) (*MountVolumeResponse, error) {
	klog.V(2).Infof("MountVolume: Request: %+v", request)
	response := &MountVolumeResponse{}

	volumeID := request.VolumeID
	if volumeID == "" {
		klog.Errorf("volume id empty")
		return response, fmt.Errorf("MountVolumeRequest.VolumeID is empty")
	}
	targetPath := request.TargetPath
	if targetPath == "" {
		klog.Errorf("targetPath empty")
		return response, fmt.Errorf("MountVolumeRequest.TargetPath is empty")
	}

	err := v.hostAPI.MountVolume(volumeID, targetPath)
	if err != nil {
		klog.Errorf("failed MountVolume %v", err)
		return response, err
	}
	return response, nil
}

func (v *Volume) UnmountVolume(context context.Context, request *UnmountVolumeRequest) (*UnmountVolumeResponse, error) {
	klog.V(2).Infof("UnmountVolume: Request: %+v", request)
	response := &UnmountVolumeResponse{}

	volumeID := request.VolumeID
	if volumeID == "" {
		klog.Errorf("volume id empty")
		return response, fmt.Errorf("volume id empty")
	}
	targetPath := request.TargetPath
	if targetPath == "" {
		klog.Errorf("target path empty")
		return response, fmt.Errorf("target path empty")
	}
	err := v.hostAPI.UnmountVolume(volumeID, targetPath)
	if err != nil {
		klog.Errorf("failed UnmountVolume %v", err)
		return response, err
	}
	return response, nil
}

func (v *Volume) IsVolumeFormatted(context context.Context, request *IsVolumeFormattedRequest) (*IsVolumeFormattedResponse, error) {
	klog.V(2).Infof("IsVolumeFormatted: Request: %+v", request)
	response := &IsVolumeFormattedResponse{}

	volumeID := request.VolumeID
	if volumeID == "" {
		klog.Errorf("volume id empty")
		return response, fmt.Errorf("volume id empty")
	}
	isFormatted, err := v.hostAPI.IsVolumeFormatted(volumeID)
	if err != nil {
		klog.Errorf("failed IsVolumeFormatted %v", err)
		return response, err
	}
	klog.V(5).Infof("IsVolumeFormatted: return: %v", isFormatted)
	response.Formatted = isFormatted
	return response, nil
}

func (v *Volume) FormatVolume(context context.Context, request *FormatVolumeRequest) (*FormatVolumeResponse, error) {
	klog.V(2).Infof("FormatVolume: Request: %+v", request)
	response := &FormatVolumeResponse{}

	volumeID := request.VolumeID
	if volumeID == "" {
		klog.Errorf("volume id empty")
		return response, fmt.Errorf("volume id empty")
	}

	err := v.hostAPI.FormatVolume(volumeID)
	if err != nil {
		klog.Errorf("failed FormatVolume %v", err)
		return response, err
	}
	return response, nil
}

func (v *Volume) WriteVolumeCache(context context.Context, request *WriteVolumeCacheRequest) (*WriteVolumeCacheResponse, error) {
	klog.V(2).Infof("WriteVolumeCache: Request: %+v", request)
	response := &WriteVolumeCacheResponse{}

	volumeID := request.VolumeID
	if volumeID == "" {
		klog.Errorf("volume id empty")
		return response, fmt.Errorf("volume id empty")
	}

	err := v.hostAPI.WriteVolumeCache(volumeID)
	if err != nil {
		klog.Errorf("failed WriteVolumeCache %v", err)
		return response, err
	}
	return response, nil
}

func (v *Volume) ResizeVolume(context context.Context, request *ResizeVolumeRequest) (*ResizeVolumeResponse, error) {
	klog.V(2).Infof("ResizeVolume: Request: %+v", request)
	response := &ResizeVolumeResponse{}

	volumeID := request.VolumeID
	if volumeID == "" {
		klog.Errorf("volume id empty")
		return response, fmt.Errorf("volume id empty")
	}
	sizeBytes := request.SizeBytes
	// TODO : Validate size param

	err := v.hostAPI.ResizeVolume(volumeID, sizeBytes)
	if err != nil {
		klog.Errorf("failed ResizeVolume %v", err)
		return response, err
	}
	return response, nil
}

func (v *Volume) GetVolumeStats(context context.Context, request *GetVolumeStatsRequest) (*GetVolumeStatsResponse, error) {
	klog.V(2).Infof("GetVolumeStats: Request: %+v", request)
	volumeID := request.VolumeID
	if volumeID == "" {
		return nil, fmt.Errorf("volume id empty")
	}

	totalBytes, usedBytes, err := v.hostAPI.GetVolumeStats(volumeID)
	if err != nil {
		klog.Errorf("failed GetVolumeStats %v", err)
		return nil, err
	}

	klog.V(2).Infof("VolumeStats: returned: Capacity %v Used %v", totalBytes, usedBytes)

	response := &GetVolumeStatsResponse{
		TotalBytes: totalBytes,
		UsedBytes:  usedBytes,
	}

	return response, nil
}

func (v *Volume) GetDiskNumberFromVolumeID(context context.Context, request *GetDiskNumberFromVolumeIDRequest) (*GetDiskNumberFromVolumeIDResponse, error) {
	klog.V(2).Infof("GetDiskNumberFromVolumeID: Request: %+v", request)

	volumeID := request.VolumeID
	if volumeID == "" {
		return nil, fmt.Errorf("volume id empty")
	}

	diskNumber, err := v.hostAPI.GetDiskNumberFromVolumeID(volumeID)
	if err != nil {
		klog.Errorf("failed GetDiskNumberFromVolumeID %v", err)
		return nil, err
	}

	response := &GetDiskNumberFromVolumeIDResponse{
		DiskNumber: diskNumber,
	}

	return response, nil
}

func (v *Volume) GetVolumeIDFromTargetPath(context context.Context, request *GetVolumeIDFromTargetPathRequest) (*GetVolumeIDFromTargetPathResponse, error) {
	klog.V(2).Infof("GetVolumeIDFromTargetPath: Request: %+v", request)

	targetPath := request.TargetPath
	if targetPath == "" {
		return nil, fmt.Errorf("target path is empty")
	}

	volume, err := v.hostAPI.GetVolumeIDFromTargetPath(targetPath)
	if err != nil {
		klog.Errorf("failed GetVolumeIDFromTargetPath: %v", err)
		return nil, err
	}

	response := &GetVolumeIDFromTargetPathResponse{
		VolumeID: volume,
	}

	return response, nil
}

func (v *Volume) GetClosestVolumeIDFromTargetPath(context context.Context, request *GetClosestVolumeIDFromTargetPathRequest) (*GetClosestVolumeIDFromTargetPathResponse, error) {
	klog.V(2).Infof("GetClosestVolumeIDFromTargetPath: Request: %+v", request)

	targetPath := request.TargetPath
	if targetPath == "" {
		return nil, fmt.Errorf("target path is empty")
	}

	volume, err := v.hostAPI.GetClosestVolumeIDFromTargetPath(targetPath)
	if err != nil {
		klog.Errorf("failed GetClosestVolumeIDFromTargetPath: %v", err)
		return nil, err
	}

	response := &GetClosestVolumeIDFromTargetPathResponse{
		VolumeID: volume,
	}

	return response, nil
}
