// Copyright Amazon.com Inc. or its affiliates. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License"). You may
// not use this file except in compliance with the License. A copy of the
// License is located at
//
//     http://aws.amazon.com/apache2.0/
//
// or in the "license" file accompanying this file. This file is distributed
// on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either
// express or implied. See the License for the specific language governing
// permissions and limitations under the License.

package gql

import (
	"github.com/aws/amazon-ecs-agent/agent/api"
	"github.com/aws/amazon-ecs-agent/agent/engine/dockerstate"
	"github.com/aws/amazon-ecs-agent/agent/handlers/utils"
	v3 "github.com/aws/amazon-ecs-agent/agent/handlers/v3"
	"github.com/aws/amazon-ecs-agent/agent/stats"
	"github.com/graphql-go/graphql"
)

var ContainerMetadataPath = "/graphql/" + utils.ConstructMuxVar(v3.V3EndpointIDMuxName, utils.AnythingButSlashRegEx)

func CreateSchema(
	state dockerstate.TaskEngineState,
	ecsClient api.ECSClient,
	statsEngine stats.Engine,
	cluster string,
	availabilityZone string,
	containerInstanceArn string) (graphql.Schema, error) {

	// Create GraphQL Schema with 'Container' Query, which resolves to the dockerID
	fields := graphql.Fields{
		"Container": &graphql.Field{
			Type:    graphql.String,
			Resolve: dockerIDResolver,
		},
	}
	rootQuery := graphql.ObjectConfig{Name: "RootQuery", Fields: fields}
	schemaConfig := graphql.SchemaConfig{Query: graphql.NewObject(rootQuery)}

	return graphql.NewSchema(schemaConfig)
}

func dockerIDResolver(p graphql.ResolveParams) (interface{}, error) {
	dockerID, ok := p.Context.Value(DockerID).(string)
	if !ok {
		return "", nil
	}

	return dockerID, nil
}
