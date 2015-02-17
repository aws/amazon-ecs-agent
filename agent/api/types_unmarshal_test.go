package api

import (
	"encoding/json"
	"testing"
)

func TestVolumesFromUnmarshal(t *testing.T) {
	var vols []VolumeFrom
	err := json.Unmarshal([]byte(`[{"sourceContainer":"c1"},{"sourceContainer":"c2","readOnly":true}]`), &vols)
	if err != nil {
		t.Fatal("Unable to unmarshal json")
	}
	if (vols[0] != VolumeFrom{SourceContainer: "c1", ReadOnly: false}) {
		t.Error("VolumeFrom 1 didn't match expected output")
	}
	if (vols[1] != VolumeFrom{SourceContainer: "c2", ReadOnly: true}) {
		t.Error("VolumeFrom 2 didn't match expected output")
	}
}
