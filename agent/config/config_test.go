package config

import "testing"

func TestMerge(t *testing.T) {
	conf1 := &Config{ClusterArn: "Foo"}
	conf2 := Config{ClusterArn: "ignored", APIEndpoint: "Bar"}
	conf3 := Config{APIPort: 99}

	conf1.Merge(conf2).Merge(conf3)

	if conf1.ClusterArn != "Foo" {
		t.Error("The cluster should not have been overridden")
	}
	if conf1.APIEndpoint != "Bar" {
		t.Error("The APIEndpoint should have been merged in")
	}
	if conf1.APIPort != 99 {
		t.Error("The APIPort should have been 99")
	}
}
