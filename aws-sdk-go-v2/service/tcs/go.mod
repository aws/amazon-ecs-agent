module github.com/aws/aws-sdk-go-v2/service/tcs

go 1.22

toolchain go1.22.7

require (
	github.com/aws/aws-sdk-go-v2 v1.36.2
	github.com/aws/aws-sdk-go-v2/internal/configsources v1.3.29
	github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 v2.6.29
	github.com/aws/smithy-go v1.22.2
)

replace github.com/aws/aws-sdk-go-v2/service/tcs/types => ../aws-sdk-go-v2/service/tcs/types
