package ecsacs

type AccessDeniedException struct {
	Message *string `locationName:"message" type:"string"`

	metadataAccessDeniedException `json:"-", xml:"-"`
}

type metadataAccessDeniedException struct {
	SDKShapeTraits bool `type:"structure"`
}

type AckRequest struct {
	Cluster *string `locationName:"cluster" type:"string"`

	ContainerInstance *string `locationName:"containerInstance" type:"string"`

	MessageId *string `locationName:"messageId" type:"string"`

	metadataAckRequest `json:"-", xml:"-"`
}

type metadataAckRequest struct {
	SDKShapeTraits bool `type:"structure"`
}

type BadRequestException struct {
	Message *string `locationName:"message" type:"string"`

	metadataBadRequestException `json:"-", xml:"-"`
}

type metadataBadRequestException struct {
	SDKShapeTraits bool `type:"structure"`
}

type CloseMessage struct {
	Message *string `locationName:"message" type:"string"`

	metadataCloseMessage `json:"-", xml:"-"`
}

type metadataCloseMessage struct {
	SDKShapeTraits bool `type:"structure"`
}

type Container struct {
	Command []*string `locationName:"command" type:"list"`

	Cpu *int64 `locationName:"cpu" type:"integer"`

	EntryPoint []*string `locationName:"entryPoint" type:"list"`

	Environment *map[string]*string `locationName:"environment" type:"map"`

	Essential *bool `locationName:"essential" type:"boolean"`

	Image *string `locationName:"image" type:"string"`

	Links []*string `locationName:"links" type:"list"`

	Memory *int64 `locationName:"memory" type:"integer"`

	MountPoints []*MountPoint `locationName:"mountPoints" type:"list"`

	Name *string `locationName:"name" type:"string"`

	Overrides *string `locationName:"overrides" type:"string"`

	PortMappings []*PortMapping `locationName:"portMappings" type:"list"`

	VolumesFrom []*VolumeFrom `locationName:"volumesFrom" type:"list"`

	metadataContainer `json:"-", xml:"-"`
}

type metadataContainer struct {
	SDKShapeTraits bool `type:"structure"`
}

type HeartbeatMessage struct {
	Healthy *bool `locationName:"healthy" type:"boolean"`

	metadataHeartbeatMessage `json:"-", xml:"-"`
}

type metadataHeartbeatMessage struct {
	SDKShapeTraits bool `type:"structure"`
}

type HeartbeatOutput struct {
	metadataHeartbeatOutput `json:"-", xml:"-"`
}

type metadataHeartbeatOutput struct {
	SDKShapeTraits bool `type:"structure"`
}

type HostVolumeProperties struct {
	SourcePath *string `locationName:"sourcePath" type:"string"`

	metadataHostVolumeProperties `json:"-", xml:"-"`
}

type metadataHostVolumeProperties struct {
	SDKShapeTraits bool `type:"structure"`
}

type InactiveInstanceException struct {
	Message *string `locationName:"message" type:"string"`

	metadataInactiveInstanceException `json:"-", xml:"-"`
}

type metadataInactiveInstanceException struct {
	SDKShapeTraits bool `type:"structure"`
}

type InvalidClusterException struct {
	Message *string `locationName:"message" type:"string"`

	metadataInvalidClusterException `json:"-", xml:"-"`
}

type metadataInvalidClusterException struct {
	SDKShapeTraits bool `type:"structure"`
}

type InvalidInstanceException struct {
	Message *string `locationName:"message" type:"string"`

	metadataInvalidInstanceException `json:"-", xml:"-"`
}

type metadataInvalidInstanceException struct {
	SDKShapeTraits bool `type:"structure"`
}

type MountPoint struct {
	ContainerPath *string `locationName:"containerPath" type:"string"`

	ReadOnly *bool `locationName:"readOnly" type:"boolean"`

	SourceVolume *string `locationName:"sourceVolume" type:"string"`

	metadataMountPoint `json:"-", xml:"-"`
}

type metadataMountPoint struct {
	SDKShapeTraits bool `type:"structure"`
}

type PayloadMessage struct {
	ClusterArn *string `locationName:"clusterArn" type:"string"`

	ContainerInstanceArn *string `locationName:"containerInstanceArn" type:"string"`

	GeneratedAt *int64 `locationName:"generatedAt" type:"long"`

	MessageId *string `locationName:"messageId" type:"string"`

	SeqNum *int64 `locationName:"seqNum" type:"integer"`

	Tasks []*Task `locationName:"tasks" type:"list"`

	metadataPayloadMessage `json:"-", xml:"-"`
}

type metadataPayloadMessage struct {
	SDKShapeTraits bool `type:"structure"`
}

type PollRequest struct {
	Cluster *string `locationName:"cluster" type:"string"`

	ContainerInstance *string `locationName:"containerInstance" type:"string"`

	SeqNum *int64 `locationName:"seqNum" type:"integer"`

	metadataPollRequest `json:"-", xml:"-"`
}

type metadataPollRequest struct {
	SDKShapeTraits bool `type:"structure"`
}

type PortMapping struct {
	ContainerPort *int64 `locationName:"containerPort" type:"integer"`

	HostPort *int64 `locationName:"hostPort" type:"integer"`

	metadataPortMapping `json:"-", xml:"-"`
}

type metadataPortMapping struct {
	SDKShapeTraits bool `type:"structure"`
}

type ServerException struct {
	Message *string `locationName:"message" type:"string"`

	metadataServerException `json:"-", xml:"-"`
}

type metadataServerException struct {
	SDKShapeTraits bool `type:"structure"`
}

type Task struct {
	Arn *string `locationName:"arn" type:"string"`

	Containers []*Container `locationName:"containers" type:"list"`

	DesiredStatus *string `locationName:"desiredStatus" type:"string"`

	Family *string `locationName:"family" type:"string"`

	Overrides *string `locationName:"overrides" type:"string"`

	TaskDefinitionAccountId *string `locationName:"taskDefinitionAccountId" type:"string"`

	Version *string `locationName:"version" type:"string"`

	Volumes []*Volume `locationName:"volumes" type:"list"`

	metadataTask `json:"-", xml:"-"`
}

type metadataTask struct {
	SDKShapeTraits bool `type:"structure"`
}

type Volume struct {
	Host *HostVolumeProperties `locationName:"host" type:"structure"`

	Name *string `locationName:"name" type:"string"`

	metadataVolume `json:"-", xml:"-"`
}

type metadataVolume struct {
	SDKShapeTraits bool `type:"structure"`
}

type VolumeFrom struct {
	ReadOnly *bool `locationName:"readOnly" type:"boolean"`

	SourceContainer *string `locationName:"sourceContainer" type:"string"`

	metadataVolumeFrom `json:"-", xml:"-"`
}

type metadataVolumeFrom struct {
	SDKShapeTraits bool `type:"structure"`
}