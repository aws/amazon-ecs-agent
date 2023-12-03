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

//go:build !windows && unit
// +build !windows,unit

package data

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/aws/amazon-ecs-agent/ecs-agent/data"
	"github.com/aws/amazon-ecs-agent/ecs-agent/metrics"
	mock_metrics "github.com/aws/amazon-ecs-agent/ecs-agent/metrics/mocks"

	"github.com/golang/mock/gomock"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
	bolt "go.etcd.io/bbolt"
)

// TestNoDuplicatePortNumber will attempt to allocate all available
// ports, and that no duplicates will exist.
func TestNoDuplicatePortNumber(t *testing.T) {
	c, cleanup := newTestClient(t)
	defer cleanup()
	vni := "ABCDE"

	assigned := make(map[uint16]bool)
	requiredPortsCount := geneveDstPortMax - geneveDstPortMin

	for i := 0; i < requiredPortsCount; i++ {
		portNo, err := c.AssignGeneveDstPort(vni)
		require.NoError(t, err)

		require.False(t, assigned[portNo])
		assigned[portNo] = true
	}

	require.Equal(t, requiredPortsCount, len(assigned))
}

// TestReleasePortNumber verifies the ReleaseGeneveDstPort operation releases port numbers
// so as to be used again.
func TestReleasePortNumber(t *testing.T) {
	c, cleanup := newTestClient(t)
	defer cleanup()
	vni := "123456"

	impl, ok := c.(*networkDataClient)
	require.True(t, ok)

	portToRelease, err := impl.assignPort(geneveDstPortMin, geneveDstPortMax, vni, geneveDstPortDefault)
	require.NoError(t, err)

	err = impl.ReleaseGeneveDstPort(portToRelease, vni)
	require.NoError(t, err)

	portNo, err := impl.assignPort(geneveDstPortMin, geneveDstPortMax, vni, int(portToRelease))
	require.NoError(t, err)
	require.Equal(t, portToRelease, portNo)
}

// TestNoPortNumber verifies that if the available ports get exhausted,
// further assignment will cause failure metric to be fired.
func TestNoPortNumber(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockMetricsFactory := mock_metrics.NewMockEntryFactory(ctrl)
	c, cleanup := newTestClientWithMetricsFactory(t, mockMetricsFactory)
	defer cleanup()
	vni := "123456"

	impl, ok := c.(*networkDataClient)
	require.True(t, ok)
	requiredPortsCount := geneveDstPortMax - geneveDstPortMin

	assigned := make(map[uint16]bool)
	for i := 0; i < requiredPortsCount; i++ {
		portNo, err := impl.assignPort(geneveDstPortMin, geneveDstPortMax, vni, geneveDstPortDefault)
		require.NoError(t, err)

		require.False(t, assigned[portNo])
		assigned[portNo] = true
	}
	require.Equal(t, requiredPortsCount, len(assigned))

	// In case we try to assign ports from a range that is already exhausted, it should create metrics
	// signifying the event.
	mockEntry := mock_metrics.NewMockEntry(ctrl)
	mockMetricsFactory.EXPECT().New(metrics.AssignGeneveDstPortMetricName).Return(mockEntry).Times(1)
	mockEntry.EXPECT().Done(errNoValue).Times(1)

	mockEntry = mock_metrics.NewMockEntry(ctrl)
	mockMetricsFactory.EXPECT().New(metrics.V2NDestinationPortExhaustedMetricName).Return(mockEntry).Times(1)
	mockEntry.EXPECT().Done(errNoValue).Times(1)

	portNo, err := impl.AssignGeneveDstPort(vni)
	require.Equal(t, errNoValue, errors.Cause(err))
	require.Equal(t, 0, int(portNo))
}

func newTestClient(t testing.TB) (NetworkDataClient, func()) {
	return newTestClientWithMetricsFactory(t, metrics.NewNopEntryFactory())
}

func newTestClientWithMetricsFactory(t testing.TB, metricsFactory metrics.EntryFactory) (NetworkDataClient, func()) {
	ioPath := filepath.Join(t.TempDir(), "test.db")

	dataClient := testClient(t, ioPath)
	netDao := NewNetworkDataClient(dataClient, metricsFactory)
	cleanup := func() {
		dataClient.DB.Close()
		os.RemoveAll(ioPath)
	}
	return netDao, cleanup
}

func testClient(t testing.TB, dbFilePath string) data.Client {
	db, err := bolt.Open(dbFilePath, os.FileMode(0600), nil)
	require.NoError(t, err)

	err = db.Update(func(tx *bolt.Tx) error {
		if _, err := txCreateBucket(tx, portBucketName, true); err != nil {
			return err
		}
		return nil
	})
	require.NoError(t, err)
	return data.Client{
		DB:       db,
		Accessor: data.DBAccessor{},
	}
}

func txCreateBucket(tx *bolt.Tx, bucketName string, reset bool) (*bolt.Bucket, error) {
	if reset {
		_ = tx.DeleteBucket([]byte(bucketName))
	}
	b, err := tx.CreateBucketIfNotExists([]byte(bucketName))
	if err != nil {
		return nil, errors.Wrapf(err, "db: failed to create %s bucket", bucketName)
	}

	return b, nil
}
