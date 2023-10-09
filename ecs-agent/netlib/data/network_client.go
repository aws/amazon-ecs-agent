package data

import (
	"encoding/json"

	"github.com/aws/amazon-ecs-agent/ecs-agent/data"
	"github.com/aws/amazon-ecs-agent/ecs-agent/metrics"
	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/tasknetworkconfig"

	bolt "go.etcd.io/bbolt"
)

const (
	networkNamespaceBucketName = "networkNamespaceConfiguration"
)

type NetworkDataClient interface {
}

type networkDataClient struct {
	data.Client
	metricsFactory metrics.EntryFactory
}

func NewNetworkDataClient(db data.Client, metricsFactory metrics.EntryFactory) NetworkDataClient {
	return networkDataClient{
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
