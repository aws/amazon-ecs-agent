package awsrulesfn

// aws-sdk-go-v2 does not export partition metadata, so copy the files from vendor to make it accessible.

//go:generate cp ../../vendor/github.com/aws/aws-sdk-go-v2/internal/endpoints/awsrulesfn/partition.go ../../vendor/github.com/aws/aws-sdk-go-v2/internal/endpoints/awsrulesfn/partitions.go .
