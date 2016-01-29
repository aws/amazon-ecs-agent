// Copyright 2014-2015 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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

package config

import (
	"errors"
	"os"
	"time"
	"reflect"
	"testing"

	"github.com/aws/amazon-ecs-agent/agent/ec2"
	"github.com/aws/amazon-ecs-agent/agent/ec2/mocks"
	"github.com/aws/amazon-ecs-agent/agent/engine/dockerclient"

	"github.com/golang/mock/gomock"
)

func TestMerge(t *testing.T) {
	conf1 := &Config{Cluster: "Foo"}
	conf2 := Config{Cluster: "ignored", APIEndpoint: "Bar"}
	conf3 := Config{AWSRegion: "us-west-2"}

	conf1.Merge(conf2).Merge(conf3)

	if conf1.Cluster != "Foo" {
		t.Error("The cluster should not have been overridden")
	}
	if conf1.APIEndpoint != "Bar" {
		t.Error("The APIEndpoint should have been merged in")
	}
	if conf1.AWSRegion != "us-west-2" {
		t.Error("Incorrect region")
	}
}

func TestBrokenEC2Metadata(t *testing.T) {
	os.Clearenv()
	ctrl := gomock.NewController(t)
	mockEc2Metadata := mock_ec2.NewMockEC2MetadataClient(ctrl)
	mockEc2Metadata.EXPECT().InstanceIdentityDocument().Return(nil, errors.New("err"))

	_, err := NewConfig(mockEc2Metadata)
	if err == nil {
		t.Fatal("Expected error when region isn't set and metadata doesn't work")
	}
}

func TestBrokenEC2MetadataEndpoint(t *testing.T) {
	os.Clearenv()
	ctrl := gomock.NewController(t)
	mockEc2Metadata := mock_ec2.NewMockEC2MetadataClient(ctrl)

	mockEc2Metadata.EXPECT().InstanceIdentityDocument().Return(nil, errors.New("err"))
	os.Setenv("AWS_DEFAULT_REGION", "us-west-2")

	config, err := NewConfig(mockEc2Metadata)
	if err != nil {
		t.Fatal("Expected no error")
	}
	if config.AWSRegion != "us-west-2" {
		t.Fatal("Wrong region: " + config.AWSRegion)
	}
	if config.APIEndpoint != "" {
		t.Fatal("Endpoint env variable not set; endpoint should be blank")
	}
}

func TestEnvironmentConfig(t *testing.T) {
	os.Setenv("ECS_CLUSTER", "myCluster")
	os.Setenv("ECS_RESERVED_PORTS_UDP", "[42,99]")
	os.Setenv("ECS_RESERVED_MEMORY", "20")
	os.Setenv("ECS_CONTAINER_STOP_TIMEOUT", "30")
	os.Setenv("ECS_AVAILABLE_LOGGING_DRIVERS", "[\""+string(dockerclient.SyslogDriver)+"\"]")
	os.Setenv("ECS_SELINUX_CAPABLE", "true")
	os.Setenv("ECS_APPARMOR_CAPABLE", "true")
	os.Setenv("ECS_DISABLE_PRIVILEGED", "true")

	conf := EnvironmentConfig()
	if conf.Cluster != "myCluster" {
		t.Error("Wrong value for cluster ", conf.Cluster)
	}
	if len(conf.ReservedPortsUDP) != 2 {
		t.Error("Wrong length for ReservedPortsUDP")
	}
	if conf.ReservedPortsUDP[0] != 42 || conf.ReservedPortsUDP[1] != 99 {
		t.Error("Wrong value for ReservedPortsUDP ", conf.ReservedPortsUDP)
	}
	if conf.ReservedMemory != 20 {
		t.Error("Wrong value for ReservedMemory", conf.ReservedMemory)
	}
	expectedDuration, _ := time.ParseDuration("30s")
	if conf.DockerStopTimeoutSeconds != expectedDuration {
		t.Error("Wrong value for DockerStopTimeoutSeconds", conf.DockerStopTimeoutSeconds)
	}
	if !reflect.DeepEqual(conf.AvailableLoggingDrivers, []dockerclient.LoggingDriver{dockerclient.SyslogDriver}) {
		t.Error("Wrong value for AvailableLoggingDrivers", conf.AvailableLoggingDrivers)
	}
	if !conf.PrivilegedDisabled {
		t.Error("Wrong value for PrivilegedDisabled")
	}
	if !conf.SELinuxCapable {
		t.Error("Wrong value for SELinuxCapable")
	}
	if !conf.AppArmorCapable {
		t.Error("Wrong value for AppArmorCapable")
	}
}

func TestTrimWhitespace(t *testing.T) {
	os.Setenv("ECS_CLUSTER", "default \r")
	os.Setenv("ECS_ENGINE_AUTH_TYPE", "dockercfg\r")

	cfg, err := NewConfig(ec2.NewBlackholeEC2MetadataClient())
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Cluster != "default" {
		t.Error("Wrong cluster: " + cfg.Cluster)
	}
	if cfg.EngineAuthType != "dockercfg" {
		t.Error("Wrong auth type: " + cfg.EngineAuthType)
	}

	cfg = &Config{
		Cluster:   " asdf ",
		AWSRegion: " us-east-1\r\t",
		DataDir:   "/trailing/space/directory ",
	}
	cfg.trimWhitespace()
	if !reflect.DeepEqual(cfg, &Config{Cluster: "asdf", AWSRegion: "us-east-1", DataDir: "/trailing/space/directory "}) {
		t.Error("Did not match expected", *cfg)
	}
}

func TestConfigBoolean(t *testing.T) {
	os.Setenv("ECS_DISABLE_METRICS", "true")
	cfg, err := NewConfig(ec2.NewBlackholeEC2MetadataClient())
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.DisableMetrics {
		t.Error("DisableMetrics not set to true")
	}
}

func TestConfigDefault(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.DockerEndpoint != "unix:///var/run/docker.sock" {
		t.Error("Default docker endpoint set incorrectly")
	}
	if cfg.DataDir != "/data/" {
		t.Error("Default datadir set incorrectly")
	}
	if cfg.DisableMetrics {
		t.Error("Default disablemetrics set incorrectly")
	}
	if len(cfg.ReservedPorts) != 4 {
		t.Error("Default resered ports set incorrectly")
	}
	if cfg.DockerGraphPath != "/var/lib/docker" {
		t.Error("Default docker graph path set incorrectly")
	}
	if cfg.ReservedMemory != 0 {
		t.Error("Default reserved memory set incorrectly")
	}
	expectedTimeout, _ := time.ParseDuration("30s")
	if cfg.DockerStopTimeoutSeconds != expectedTimeout {
		t.Error("Default docker stop container timeout set incorrectly", cfg.DockerStopTimeoutSeconds)
	}
	if cfg.PrivilegedDisabled {
		t.Error("Default PrivilegedDisabled set incorrectly")
	}
	if !reflect.DeepEqual(cfg.AvailableLoggingDrivers, []dockerclient.LoggingDriver{dockerclient.JsonFileDriver}) {
		t.Error("Default logging drivers set incorrectly")
	}
}

func TestBadLoggingDriverSerialization(t *testing.T) {
	os.Setenv("ECS_AVAILABLE_LOGGING_DRIVERS", "[\"malformed]")

	conf := EnvironmentConfig()
	if len(conf.AvailableLoggingDrivers) != 0 {
		t.Error("Wrong value for AvailableLoggingDrivers", conf.AvailableLoggingDrivers)
	}
}

func TestInvalidLoggingDriver(t *testing.T) {
	conf := DefaultConfig()
	conf.AWSRegion = "us-west-2"
	conf.AvailableLoggingDrivers = []dockerclient.LoggingDriver{"invalid-logging-driver"}

	err := conf.validate()
	if err == nil {
		t.Error("Should be error with invalid-logging-driver")
	}
}
