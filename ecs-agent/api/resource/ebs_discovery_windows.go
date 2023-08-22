//go:build windows
// +build windows

package resource

func (api *EBSDiscoveryClient) ConfirmEBSVolumeIsAttached(deviceName, volumeID string) error {
	return nil
}
