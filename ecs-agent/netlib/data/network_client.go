// Copyright Amazon.com Inc. or its affiliates. All Rights Reserved.
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

package data

import (
	"crypto/rand"
	"encoding/json"
	"math/big"
	"strconv"

	"github.com/aws/amazon-ecs-agent/ecs-agent/data"
	"github.com/aws/amazon-ecs-agent/ecs-agent/metrics"
	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/tasknetworkconfig"

	"github.com/pkg/errors"
	bolt "go.etcd.io/bbolt"
)

const (
	networkNamespaceBucketName = "networkNamespaceConfiguration"
	portBucketName             = "port"

	// geneveDstPortMin and geneveDstPortMax specifies the range from which the GENEVE destination port will be selected.
	// The range is currently set from 6081 to 6151 which gives us 70 ports per baremetal host to access the same subnet.
	// The goal while choosing these values were to keep the range wide enough so as to avoid port exhaustion and narrow
	// enough to reduce the attack surface since the same port range has to be opened up in the security group
	// assigned to the connectivity provider ENI. A rough calculation that explains why 70 ports were chosen as the range is:
	// Our largest customers have been seen to launch somewhat about 10,000 tasks around a given time. Ideally there
	// are 3 AZs per region with 2 cells. Each cell has 8 instances. We will also establish tunnels between the target
	// subnet and at least 3 random connectivity providers per cell. This means we'll be able to spread customer
	// tasks across at least 16 hosts and across 6 tunnels per AZ. Assuming even distribution, then this puts us
	// at 69 uVMS per host accessing the same subnet (10000 / [6 * 8 * 3]).
	// We start with 6081 since that is the default destination port for GENEVE interfaces.
	geneveDstPortMin = 6081
	geneveDstPortMax = 6151

	// geneveDstPortDefault is the default port to be assigned if not taken already.
	geneveDstPortDefault = 6081
)

var (
	trueValue       = []byte("1")
	errNoValue      = errors.New("all values in the given range are currently in use")
	errInvalidInput = errors.New("invalid random number requested")
)

type NetworkDataClient interface {
	GetNetworkNamespacesByTaskID(taskID string) ([]*tasknetworkconfig.NetworkNamespace, error)
	SaveNetworkNamespace(netNS *tasknetworkconfig.NetworkNamespace) error
	GetNetworkNamespace(netNSName string) (*tasknetworkconfig.NetworkNamespace, error)

	// AssignGeneveDstPort returns an unused destination port number for GENEVE interfaces.
	// By default for a particular VNI, it will return the default GENEVE destination port - 6081.
	// In case port 6081 is taken by another interface using the same VNI, it will chose a
	// random port from within the pre-configured range.
	AssignGeneveDstPort(vni string) (uint16, error)

	// ReleaseGeneveDstPort tells the client that the port is no longer in use by the interface having
	// the mentioned VNI. The port could be reused later.
	ReleaseGeneveDstPort(port uint16, vni string) error
}

type networkDataClient struct {
	data.Client
	metricsFactory metrics.EntryFactory
}

func NewNetworkDataClient(db data.Client, metricsFactory metrics.EntryFactory) NetworkDataClient {
	return &networkDataClient{
		Client:         db,
		metricsFactory: metricsFactory,
	}
}

func (ndc *networkDataClient) GetNetworkNamespacesByTaskID(taskID string) ([]*tasknetworkconfig.NetworkNamespace, error) {
	var netNSs []*tasknetworkconfig.NetworkNamespace
	metricEntry := ndc.metricsFactory.New(metrics.GetNetworkConfigurationByTaskMetricName)
	err := ndc.DB.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(networkNamespaceBucketName))
		return ndc.Accessor.WalkPrefix(bucket, taskID, func(id string, data []byte) error {
			netNS := &tasknetworkconfig.NetworkNamespace{}
			if err := json.Unmarshal(data, netNS); err != nil {
				return err
			}
			netNSs = append(netNSs, netNS)
			return nil
		})
	})
	metricEntry.Done(err)
	if err != nil {
		return nil, err
	}

	return netNSs, nil
}

func (ndc *networkDataClient) SaveNetworkNamespace(netNS *tasknetworkconfig.NetworkNamespace) error {
	metricEntry := ndc.metricsFactory.New(metrics.SaveNetworkNamespaceMetricName)
	err := ndc.DB.Update(func(tx *bolt.Tx) error {
		bucket, err := ndc.Accessor.GetBucket(tx, networkNamespaceBucketName)
		if err != nil {
			return err
		}
		return ndc.Accessor.PutObject(bucket, netNS.Name, netNS)
	})
	metricEntry.Done(err)
	return err
}

func (ndc *networkDataClient) GetNetworkNamespace(netNSName string) (*tasknetworkconfig.NetworkNamespace, error) {
	metricEntry := ndc.metricsFactory.New(metrics.GetNetworkNamespaceMetricName)
	netNS := &tasknetworkconfig.NetworkNamespace{}
	err := ndc.DB.View(func(tx *bolt.Tx) error {
		return ndc.Accessor.GetObject(tx, networkNamespaceBucketName, netNSName, netNS)
	})
	metricEntry.Done(err)

	if err != nil {
		return nil, err
	}

	return netNS, nil
}

// AssignGeneveDstPort returns an unused destination port number for GENEVE interfaces.
// By default for a particular VNI, it will return the default GENEVE destination port - 6081.
// In case port 6081 is taken by another interface using the same VNI, it will chose a
// random port from within the pre-configured range.
// Once a port number is assigned by this function, it won't be used until
// ReleaseGeneveDstPort is called with the port and VNI.
func (ndc *networkDataClient) AssignGeneveDstPort(vni string) (uint16, error) {
	metricEntry := ndc.metricsFactory.New(metrics.AssignGeneveDstPortMetricName)
	port, err := ndc.assignPort(geneveDstPortMin, geneveDstPortMax, vni, geneveDstPortDefault)
	metricEntry.Done(err)

	if errNoValue == errors.Cause(err) {
		ndc.metricsFactory.New(metrics.V2NDestinationPortExhaustedMetricName).Done(err)
	}
	return port, err
}

func (ndc *networkDataClient) assignPort(min, max int, vni string, defaultValue int) (uint16, error) {
	var value int

	err := ndc.DB.Update(func(tx *bolt.Tx) error {
		var e error

		// Destination ports are required to be unique only per VNI. Hence we create nested buckets
		// inside the port bucket with the VNI as the name.
		portBucket := tx.Bucket([]byte(portBucketName))
		if vni != "" {
			if portBucket, e = portBucket.CreateBucketIfNotExists([]byte(vni)); e != nil {
				return e
			}
		}

		value, e = findRandomUnusedValue(portBucket, min, max, defaultValue)
		return e
	})

	return uint16(value), err
}

func (ndc *networkDataClient) releasePort(port uint16, namespace string) error {
	return ndc.DB.Update(func(tx *bolt.Tx) error {
		var e error
		portBucket := tx.Bucket([]byte(portBucketName))
		if namespace != "" {
			if portBucket, e = portBucket.CreateBucketIfNotExists([]byte(namespace)); e != nil {
				return e
			}
		}

		key := []byte(strconv.Itoa(int(port)))
		return portBucket.Delete(key)
	})
}

func (ndc *networkDataClient) ReleaseGeneveDstPort(port uint16, vni string) error {
	metricEntry := ndc.metricsFactory.New(metrics.ReleaseGeneveDstPortMetricName)
	err := ndc.releasePort(port, vni)
	metricEntry.Done(err)
	return err
}

// findRandomUnusedValue returns a random integer value that was previously unused.
// You may specify a defaultValue if you have a preferred value. If the defaultValue is already used,
// it will return a random unused value from within the range. An example for when we use the defaultValue
// is while assigning destination ports for GENEVE interfaces. By default we need to assign port 6081 unless
// it is used by another existing interface. If 6081 is taken, we need to pick a random port from the specified
// range of [min, max).
// The function makes use of a data store bucket to store all the used values.
func findRandomUnusedValue(bucket *bolt.Bucket, min, max, defaultValue int) (int, error) {
	// Validate user input to this method with range > 0 to avoid rand.Int panic.
	// It is still possible for max-min to be positive if either min or max are <= 0.
	if max-min <= 0 || max <= 0 || min <= 0 {
		return 0, errInvalidInput
	}

	// Check the base case where a valid default value is provided.
	key := []byte(strconv.Itoa(defaultValue))

	if defaultValue >= min &&
		defaultValue < max &&
		bucket.Get(key) == nil {
		err := bucket.Put(key, trueValue)
		if err != nil {
			return 0, err
		}
		return defaultValue, nil
	}

	// Set the start to be a random value between max and min.
	randStart, err := rand.Int(rand.Reader, big.NewInt((int64(max - min))))
	if err != nil {
		return 0, err
	}
	start := int(randStart.Int64()) + min
	value := start

	for {
		key = []byte(strconv.Itoa(value))

		// Check if this value is taken.
		// If so, keep iterating through the range.
		if bucket.Get(key) == nil {
			err := bucket.Put(key, trueValue)
			if err != nil {
				return 0, err
			}
			return value, nil
		}

		// Increment up the range, wrapping around if we reach max.
		value++
		if value >= max {
			value = min
		}

		// If value is equal to start, we have looped through the entire range
		// without finding a valid port number.
		if value == start {
			break
		}
	}
	return 0, errNoValue
}
