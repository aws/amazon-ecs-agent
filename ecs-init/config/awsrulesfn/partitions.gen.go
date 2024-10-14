package awsrulesfn

import "regexp"

// aws-sdk-go-v2 does not export partition metadata, so copy the files from vendor to make it accessible.

//go:generate cp ../../vendor/github.com/aws/aws-sdk-go-v2/internal/endpoints/awsrulesfn/partition.go ../../vendor/github.com/aws/aws-sdk-go-v2/internal/endpoints/awsrulesfn/partitions.go .

// GetPartitionForRegion returns an AWS partition for the region provided.
// Unlike GetPartition, this function
// 1. returns a Partition instead of a PartitionConfig
// 2. returns nil instead of falling back to the default partition (aws) if no match is found
func GetPartitionForRegion(region string) *Partition {
	for _, partition := range partitions {
		if _, ok := partition.Regions[region]; ok {
			return &partition
		}

		regionRegex := regexp.MustCompile(partition.RegionRegex)
		if regionRegex.MatchString(region) {
			return &partition
		}
	}

	return nil
}
