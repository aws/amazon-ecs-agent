// Copyright 2014-2018 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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

package taskresource

import (
	"testing"
	"time"
	"github.com/stretchr/testify/assert"
)

func TestMarshall(t *testing.T) {
	volumeStr := "{\"Name\":\"volumeName\",\"MountPoint\":\"mountPoint\",\"Driver\":\"drive\",\"Labels\":{}," +
		"\"CreatedAt\":\"0001-01-01T00:00:00Z\",\"DesiredStatus\":\"CREATED\",\"KnownStatus\":\"NONE\"}"
	name := "volumeName"
	mountPoint := "mountPoint"
	driver := "drive"

	labels := make(map[string]string)

	volume := NewVolumeResource(name, mountPoint, driver, labels)
	volume.SetDesiredStatus(VolumeCreated)
	volume.SetKnownStatus(VolumeStatusNone)

	bytes, err := volume.MarshalJSON()
	assert.NoError(t, err)
	assert.Equal(t, volumeStr, string(bytes[:]))
}
func TestUnmarshall(t *testing.T) {
	name := "volumeName"
	mountPoint := "mountPoint"
	driver := "drive"

	labels := map[string]string{
		"lab1": "label",
	}
	bytes := []byte("{\"Name\":\"volumeName\",\"MountPoint\":\"mountPoint\",\"Driver\":\"drive\",\"Labels\":{\"lab1\":\"label\"}," +
		"\"CreatedAt\":\"0001-01-01T00:00:00Z\",\"DesiredStatus\":\"CREATED\",\"KnownStatus\":\"NONE\"}")
	unmarshalledVolume := &VolumeResource{}
	err := unmarshalledVolume.UnmarshalJSON(bytes)
	assert.NoError(t, err)

	assert.Equal(t, name, unmarshalledVolume.Name)
	assert.Equal(t, mountPoint, unmarshalledVolume.Mountpoint)
	assert.Equal(t, driver, unmarshalledVolume.Driver)
	assert.Equal(t, labels, unmarshalledVolume.Labels)
	assert.Equal(t, time.Time{}, unmarshalledVolume.GetCreatedAt())
	assert.Equal(t, VolumeCreated, unmarshalledVolume.GetDesiredStatus())
	assert.Equal(t, VolumeStatusNone, unmarshalledVolume.GetKnownStatus())
}
