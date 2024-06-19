package api

type StorageDeviceNumber struct {
	DeviceType      DeviceType
	DeviceNumber    uint32
	PartitionNumber uint32
}
type DeviceType uint32

type StoragePropertyID uint32

const (
	StorageDeviceProperty StoragePropertyID = iota
	StorageAdapterProperty
	StorageDeviceIDProperty
	StorageDeviceUniqueIDProperty
	StorageDeviceWriteCacheProperty
	StorageMiniportProperty
	StorageAccessAlignmentProperty
	StorageDeviceSeekPenaltyProperty
	StorageDeviceTrimProperty
	StorageDeviceWriteAggregationProperty
	StorageDeviceDeviceTelemetryProperty
	StorageDeviceLBProvisioningProperty
	StorageDevicePowerProperty
	StorageDeviceCopyOffloadProperty
	StorageDeviceResiliencyProperty
	StorageDeviceMediumProductType
	StorageAdapterRpmbProperty
	StorageAdapterCryptoProperty
	StorageDeviceIoCapabilityProperty
	StorageAdapterProtocolSpecificProperty
	StorageDeviceProtocolSpecificProperty
	StorageAdapterTemperatureProperty
	StorageDeviceTemperatureProperty
	StorageAdapterPhysicalTopologyProperty
	StorageDevicePhysicalTopologyProperty
	StorageDeviceAttributesProperty
	StorageDeviceManagementStatus
	StorageAdapterSerialNumberProperty
	StorageDeviceLocationProperty
	StorageDeviceNumaProperty
	StorageDeviceZonedDeviceProperty
	StorageDeviceUnsafeShutdownCount
	StorageDeviceEnduranceProperty
)

type StorageQueryType uint32

const (
	PropertyStandardQuery StorageQueryType = iota
	PropertyExistsQuery
	PropertyMaskQuery
	PropertyQueryMaxDefined
)

type StoragePropertyQuery struct {
	PropertyID StoragePropertyID
	QueryType  StorageQueryType
	Byte       []AdditionalParameters
}

type AdditionalParameters byte

type StorageDeviceIDDescriptor struct {
	Version             uint32
	Size                uint32
	NumberOfIdentifiers uint32
	Identifiers         [1]byte
}

type StorageIdentifierCodeSet uint32

const (
	StorageIDCodeSetReserved StorageIdentifierCodeSet = iota
	StorageIDCodeSetBinary
	StorageIDCodeSetASCII
	StorageIDCodeSetUtf8
)

type StorageIdentifierType uint32

const (
	StorageIdTypeVendorSpecific StorageIdentifierType = iota
	StorageIDTypeVendorID
	StorageIDTypeEUI64
	StorageIDTypeFCPHName
	StorageIDTypePortRelative
	StorageIDTypeTargetPortGroup
	StorageIDTypeLogicalUnitGroup
	StorageIDTypeMD5LogicalUnitIdentifier
	StorageIDTypeScsiNameString
)

type StorageAssociationType uint32

const (
	StorageIDAssocDevice StorageAssociationType = iota
	StorageIDAssocPort
	StorageIDAssocTarget
)

type StorageIdentifier struct {
	CodeSet        StorageIdentifierCodeSet
	Type           StorageIdentifierType
	IdentifierSize uint16
	NextOffset     uint16
	Association    StorageAssociationType
	Identifier     [1]byte
}

type Disk struct {
	Path         string `json:"Path"`
	SerialNumber string `json:"SerialNumber"`
}

// DiskLocation definition
type DiskLocation struct {
	Adapter string
	Bus     string
	Target  string
	LUNID   string
}

// DiskIDs definition
type DiskIDs struct {
	Page83       string
	SerialNumber string
}
