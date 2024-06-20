package api

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"unsafe"

	"github.com/kubernetes-csi/csi-proxy/v2/pkg/utils"
	"k8s.io/klog/v2"
)

var (
	kernel32DLL = syscall.NewLazyDLL("kernel32.dll")
)

const (
	IOCTL_STORAGE_GET_DEVICE_NUMBER = 0x2D1080
	IOCTL_STORAGE_QUERY_PROPERTY    = 0x002d1400
)

// HostAPI declares the interface exposed by the internal API
type HostAPI interface {
	// ListDiskLocations - constructs a map with the disk number as the key and the DiskLocation structure
	// as the value. The DiskLocation struct has various fields like the Adapter, Bus, Target and LUNID.
	ListDiskLocations() (map[uint32]DiskLocation, error)
	// IsDiskInitialized returns true if the disk identified by `diskNumber` is initialized.
	IsDiskInitialized(diskNumber uint32) (bool, error)
	// InitializeDisk initializes the disk `diskNumber`
	InitializeDisk(diskNumber uint32) error
	// BasicPartitionsExist checks if the disk `diskNumber` has any basic partitions.
	BasicPartitionsExist(diskNumber uint32) (bool, error)
	// CreateBasicPartition creates a partition in disk `diskNumber`
	CreateBasicPartition(diskNumber uint32) error
	// Rescan updates the host storage cache (re-enumerates disk, partition and volume objects)
	Rescan() error
	// ListDiskIDs list all disks by disk number.
	ListDiskIDs() (map[uint32]DiskIDs, error)
	// GetDiskStats gets the disk stats of the disk `diskNumber`.
	GetDiskStats(diskNumber uint32) (int64, error)
	// SetDiskState sets the offline/online state of the disk `diskNumber`.
	SetDiskState(diskNumber uint32, isOnline bool) error
	// GetDiskState gets the offline/online state of the disk `diskNumber`.
	GetDiskState(diskNumber uint32) (bool, error)
}

// DiskAPI implements the OS API calls related to Disk Devices. All code here should be very simple
// pass-through to the OS APIs or cmdlets. Any logic around the APIs/cmdlet invocation
// should go in pkg/disk/disk.go so that logic can be easily unit-tested
// without requiring specific OS environments.
type DiskAPI struct{}

// ensure that DiskAPI implements the exposed API
var _ HostAPI = &DiskAPI{}

func New() DiskAPI {
	return DiskAPI{}
}

// ListDiskLocations - constructs a map with the disk number as the key and the DiskLocation structure
// as the value. The DiskLocation struct has various fields like the Adapter, Bus, Target and LUNID.
func (DiskAPI) ListDiskLocations() (map[uint32]DiskLocation, error) {
	// sample response
	// [{
	//    "number":  0,
	//    "location":  "PCI Slot 3 : Adapter 0 : Port 0 : Target 1 : LUN 0"
	// }, ...]
	cmd := "ConvertTo-Json @(Get-Disk | select Number, Location)"
	out, err := utils.RunPowershellCmd(cmd)
	if err != nil {
		return nil, fmt.Errorf("failed to list disk location. cmd: %q, output: %q, err %v", cmd, string(out), err)
	}

	var getDisk []map[string]interface{}
	err = json.Unmarshal(out, &getDisk)
	if err != nil {
		return nil, err
	}

	m := make(map[uint32]DiskLocation)
	for _, v := range getDisk {
		str := v["Location"].(string)
		num := v["Number"].(float64)

		found := false
		s := strings.Split(str, ":")
		if len(s) >= 5 {
			var d DiskLocation
			for _, item := range s {
				item = strings.TrimSpace(item)
				itemSplit := strings.Split(item, " ")
				if len(itemSplit) == 2 {
					found = true
					switch strings.TrimSpace(itemSplit[0]) {
					case "Adapter":
						d.Adapter = strings.TrimSpace(itemSplit[1])
					case "Target":
						d.Target = strings.TrimSpace(itemSplit[1])
					case "LUN":
						d.LUNID = strings.TrimSpace(itemSplit[1])
					default:
						klog.Warningf("Got unknown field : %s=%s", itemSplit[0], itemSplit[1])
					}
				}
			}

			if found {
				m[uint32(num)] = d
			}
		}
	}
	return m, nil
}

func (DiskAPI) Rescan() error {
	cmd := "Update-HostStorageCache"
	out, err := utils.RunPowershellCmd(cmd)
	if err != nil {
		return fmt.Errorf("error updating host storage cache output: %q, err: %v", string(out), err)
	}
	return nil
}

func (DiskAPI) IsDiskInitialized(diskNumber uint32) (bool, error) {
	cmd := fmt.Sprintf("Get-Disk -Number %d | Where partitionstyle -eq 'raw'", diskNumber)
	out, err := utils.RunPowershellCmd(cmd)
	if err != nil {
		return false, fmt.Errorf("error checking initialized status of disk %d: %v, %v", diskNumber, out, err)
	}
	if len(out) == 0 {
		// disks with raw initialization not detected
		return true, nil
	}
	return false, nil
}

func (DiskAPI) InitializeDisk(diskNumber uint32) error {
	cmd := fmt.Sprintf("Initialize-Disk -Number %d -PartitionStyle GPT", diskNumber)
	out, err := utils.RunPowershellCmd(cmd)
	if err != nil {
		return fmt.Errorf("error initializing disk %d: %v, %v", diskNumber, string(out), err)
	}
	return nil
}

func (DiskAPI) BasicPartitionsExist(diskNumber uint32) (bool, error) {
	cmd := fmt.Sprintf("Get-Partition | Where DiskNumber -eq %d | Where Type -ne Reserved", diskNumber)
	out, err := utils.RunPowershellCmd(cmd)
	if err != nil {
		return false, fmt.Errorf("error checking presence of partitions on disk %d: %v, %v", diskNumber, out, err)
	}
	if len(out) > 0 {
		// disk has partitions in it
		return true, nil
	}
	return false, nil
}

func (DiskAPI) CreateBasicPartition(diskNumber uint32) error {
	cmd := fmt.Sprintf("New-Partition -DiskNumber %d -UseMaximumSize", diskNumber)
	out, err := utils.RunPowershellCmd(cmd)
	if err != nil {
		return fmt.Errorf("error creating partition on disk %d: %v, %v", diskNumber, out, err)
	}
	return nil
}

func (DiskAPI) GetDiskNumber(disk syscall.Handle) (uint32, error) {
	var bytes uint32
	devNum := StorageDeviceNumber{}
	buflen := uint32(unsafe.Sizeof(devNum.DeviceType)) + uint32(unsafe.Sizeof(devNum.DeviceNumber)) + uint32(unsafe.Sizeof(devNum.PartitionNumber))

	err := syscall.DeviceIoControl(disk, IOCTL_STORAGE_GET_DEVICE_NUMBER, nil, 0, (*byte)(unsafe.Pointer(&devNum)), buflen, &bytes, nil)

	return devNum.DeviceNumber, err
}

func (DiskAPI) GetDiskPage83ID(disk syscall.Handle) (string, error) {
	query := StoragePropertyQuery{}

	bufferSize := uint32(4 * 1024)
	buffer := make([]byte, 4*1024)
	var size uint32
	var n uint32
	var m uint16

	query.QueryType = PropertyStandardQuery
	query.PropertyID = StorageDeviceIDProperty

	querySize := uint32(unsafe.Sizeof(query))
	err := syscall.DeviceIoControl(disk, IOCTL_STORAGE_QUERY_PROPERTY, (*byte)(unsafe.Pointer(&query)), querySize, (*byte)(unsafe.Pointer(&buffer[0])), bufferSize, &size, nil)
	if err != nil {
		return "", fmt.Errorf("IOCTL_STORAGE_QUERY_PROPERTY failed: %v", err)
	}

	devIDDesc := (*StorageDeviceIDDescriptor)(unsafe.Pointer(&buffer[0]))

	pID := (*StorageIdentifier)(unsafe.Pointer(&devIDDesc.Identifiers[0]))

	page83ID := []byte{}
	byteSize := unsafe.Sizeof(byte(0))
	for n = 0; n < devIDDesc.NumberOfIdentifiers; n++ {
		if pID.Association == StorageIDAssocDevice && (pID.CodeSet == StorageIDCodeSetBinary || pID.CodeSet == StorageIDCodeSetASCII) {
			for m = 0; m < pID.IdentifierSize; m++ {
				page83ID = append(page83ID, *(*byte)(unsafe.Pointer(uintptr(unsafe.Pointer(&pID.Identifier[0])) + byteSize*uintptr(m))))
			}

			if pID.CodeSet == StorageIDCodeSetASCII {
				return string(page83ID), nil
			} else if pID.CodeSet == StorageIDCodeSetBinary {
				return hex.EncodeToString(page83ID), nil
			}
		}
		pID = (*StorageIdentifier)(unsafe.Pointer(uintptr(unsafe.Pointer(pID)) + byteSize*uintptr(pID.NextOffset)))
	}
	return "", nil
}

func (imp DiskAPI) GetDiskNumberWithID(page83ID string) (uint32, error) {
	cmd := "ConvertTo-Json @(Get-Disk | Select Path)"
	out, err := utils.RunPowershellCmd(cmd)
	if err != nil {
		return 0, fmt.Errorf("Could not query disk paths")
	}

	outString := string(out)
	disks := []Disk{}
	err = json.Unmarshal([]byte(outString), &disks)
	if err != nil {
		return 0, err
	}

	for i := range disks {
		diskNumber, diskPage83ID, err := imp.GetDiskNumberAndPage83ID(disks[i].Path)
		if err != nil {
			return 0, err
		}

		if diskPage83ID == page83ID {
			return diskNumber, nil
		}
	}

	return 0, fmt.Errorf("Could not find disk with Page83 ID %s", page83ID)
}

func (imp DiskAPI) GetDiskNumberAndPage83ID(path string) (uint32, string, error) {
	h, err := syscall.Open(path, syscall.O_RDONLY, 0)
	defer syscall.Close(h)
	if err != nil {
		return 0, "", err
	}

	diskNumber, err := imp.GetDiskNumber(h)
	if err != nil {
		return 0, "", err
	}

	page83ID, err := imp.GetDiskPage83ID(h)
	if err != nil {
		return 0, "", err
	}

	return diskNumber, page83ID, nil
}

// ListDiskIDs - constructs a map with the disk number as the key and the DiskID structure
// as the value. The DiskID struct has a field for the page83 ID.
func (imp DiskAPI) ListDiskIDs() (map[uint32]DiskIDs, error) {
	// sample response
	// [
	// {
	//     "Path":  "\\\\?\\scsi#disk\u0026ven_google\u0026prod_persistentdisk#4\u002621cb0360\u00260\u0026000100#{53f56307-b6bf-11d0-94f2-00a0c91efb8b}",
	//     "SerialNumber":  "                    "
	// },
	// {
	//     "Path":  "\\\\?\\scsi#disk\u0026ven_msft\u0026prod_virtual_disk#2\u00261f4adffe\u00260\u0026000001#{53f56307-b6bf-11d0-94f2-00a0c91efb8b}",
	//     "SerialNumber":  null
	// }, ]
	cmd := "ConvertTo-Json @(Get-Disk | Select Path, SerialNumber)"
	out, err := utils.RunPowershellCmd(cmd)
	if err != nil {
		return nil, fmt.Errorf("Could not query disk paths: %v", err)
	}

	outString := string(out)
	disks := []Disk{}
	err = json.Unmarshal([]byte(outString), &disks)
	if err != nil {
		return nil, err
	}

	m := make(map[uint32]DiskIDs)

	for i := range disks {
		diskNumber, page83, err := imp.GetDiskNumberAndPage83ID(disks[i].Path)
		if err != nil {
			return nil, err
		}

		m[diskNumber] = DiskIDs{
			Page83:       page83,
			SerialNumber: disks[i].SerialNumber,
		}
	}

	return m, nil
}

func (imp DiskAPI) GetDiskStats(diskNumber uint32) (int64, error) {
	cmd := fmt.Sprintf("(Get-Disk -Number %d).Size", diskNumber)
	out, err := utils.RunPowershellCmd(cmd)
	if err != nil || len(out) == 0 {
		return -1, fmt.Errorf("error getting size of disk. cmd: %s, output: %s, error: %v", cmd, string(out), err)
	}

	reg, err := regexp.Compile("[^0-9]+")
	if err != nil {
		return -1, fmt.Errorf("error compiling regex. err: %v", err)
	}
	diskSizeOutput := reg.ReplaceAllString(string(out), "")

	diskSize, err := strconv.ParseInt(diskSizeOutput, 10, 64)

	if err != nil {
		return -1, fmt.Errorf("error parsing size of disk. cmd: %s, output: %s, error: %v", cmd, diskSizeOutput, err)
	}

	return diskSize, nil
}

func (imp DiskAPI) SetDiskState(diskNumber uint32, isOnline bool) error {
	cmd := fmt.Sprintf("(Get-Disk -Number %d) | Set-Disk -IsOffline $%t", diskNumber, !isOnline)
	out, err := utils.RunPowershellCmd(cmd)
	if err != nil {
		return fmt.Errorf("error setting disk attach state. cmd: %s, output: %s, error: %v", cmd, string(out), err)
	}

	return nil
}

func (imp DiskAPI) GetDiskState(diskNumber uint32) (bool, error) {
	cmd := fmt.Sprintf("(Get-Disk -Number %d) | Select-Object -ExpandProperty IsOffline", diskNumber)
	out, err := utils.RunPowershellCmd(cmd)
	if err != nil {
		return false, fmt.Errorf("error getting disk state. cmd: %s, output: %s, error: %v", cmd, string(out), err)
	}

	sout := strings.TrimSpace(string(out))
	isOffline, err := strconv.ParseBool(sout)
	if err != nil {
		return false, fmt.Errorf("error parsing disk state. output: %s, error: %v", sout, err)
	}

	return !isOffline, nil
}
