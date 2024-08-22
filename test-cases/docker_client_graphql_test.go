/*
 * Copyright Skyramp Authors 2024
 */
package client_test

import (
	"testing"
	"time"

	"github.com/letsramp/client-go/client"
	"github.com/letsramp/client-go/types"
	"github.com/stretchr/testify/assert"
)

func TestSpaceXGraphQL(t *testing.T) {
	var err error
	dockerClient, err = client.NewDockerClient("localhost:35242", "")
	if err != nil {
		t.Fatal(err)
	}
	updateWorker(dockerClient)
	err = dockerClient.InstallWorker()
	assert.Equal(t, nil, err)
	defer dockerClient.UninstallWorker()

	time.Sleep(3 * time.Second)
	service := &types.Service{
		Name:     "spacex",
		Protocol: types.Graphql,
		Port:     443,
		Addr:     "spacex-production.up.railway.app",
	}
	ep := client.NewRestEndpoint("spacex", "/graphql", "", service)
	req := client.NewRequest("capsule", ep, "POST", "")
	gql := &types.GraphqlParam{
		Query: `query Capsules($capsuleId: ID!) {
                  capsule(id: $capsuleId) {
                    type
                    original_launch
                    status
                  }
                }`,
		Variables: map[string]interface{}{
			"capsuleId": "5e9e2c5bf35918ed873b2664",
		},
		OperationName: "Capsules",
	}
	req.SetGraphqlQuery(gql)
	// Create a Scenario for testing
	scenario := client.NewScenario("spacex")
	scenario.SetRequest(req)
	response, err := dockerClient.TesterStart([]*client.Scenario{scenario}, "graphql_spacex_test", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Log(response)
}
