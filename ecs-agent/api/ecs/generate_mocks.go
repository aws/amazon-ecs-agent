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

package ecs

//go:generate mockgen -destination=mocks/api_mocks.go -copyright_file=../../../scripts/copyright_file github.com/aws/amazon-ecs-agent/ecs-agent/api/ecs ECSStandardSDK,ECSSubmitStateSDK,ECSClient,ECSTaskProtectionSDK
//go:generate mockgen -destination=mocks/statechange/statechange_mocks.go -package=mock_statechange -copyright_file=../../../scripts/copyright_file github.com/aws/amazon-ecs-agent/ecs-agent/api/ecs ContainerMetadataGetter,TaskMetadataGetter
//go:generate mockgen -destination=mocks/client/client_mocks.go -package=mock_client -copyright_file=../../../scripts/copyright_file github.com/aws/amazon-ecs-agent/ecs-agent/api/ecs/client ECSClientFactory
