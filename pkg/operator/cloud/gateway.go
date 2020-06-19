/*
Copyright 2020 Cortex Labs, Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package cloud

import (
	"github.com/cortexlabs/cortex/pkg/lib/aws"
	"github.com/cortexlabs/cortex/pkg/lib/urls"
	"github.com/cortexlabs/cortex/pkg/operator/config"
	ok8s "github.com/cortexlabs/cortex/pkg/operator/k8s"
	"github.com/cortexlabs/cortex/pkg/types/clusterconfig"
	"github.com/cortexlabs/cortex/pkg/types/spec"
	"github.com/cortexlabs/cortex/pkg/types/userconfig"
	kunstructured "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func AddAPIToAPIGateway(endpoint string, apiGatewayType userconfig.APIGatewayType) error {
	if apiGatewayType == userconfig.NoneAPIGatewayType {
		return nil
	}

	apiGatewayID := *config.Cluster.APIGateway.ApiId

	// check if API Gateway route already exists
	existingRoute, err := config.AWS.GetRoute(apiGatewayID, endpoint)
	if err != nil {
		return err
	} else if existingRoute != nil {
		return nil
	}

	if config.Cluster.APILoadBalancerScheme == clusterconfig.InternalLoadBalancerScheme {
		err = config.AWS.CreateRoute(apiGatewayID, *config.Cluster.VPCLinkIntegration.IntegrationId, endpoint)
		if err != nil {
			return err
		}
	}

	if config.Cluster.APILoadBalancerScheme == clusterconfig.InternetFacingLoadBalancerScheme {
		loadBalancerURL, err := ok8s.APILoadBalancerURL()
		if err != nil {
			return err
		}

		targetEndpoint := urls.Join(loadBalancerURL, endpoint)

		integrationID, err := config.AWS.CreateHTTPIntegration(apiGatewayID, targetEndpoint)
		if err != nil {
			return err
		}

		err = config.AWS.CreateRoute(apiGatewayID, integrationID, endpoint)
		if err != nil {
			return err
		}
	}

	return nil
}

func RemoveAPIFromAPIGateway(endpoint string, apiGatewayType userconfig.APIGatewayType) error {
	if apiGatewayType == userconfig.NoneAPIGatewayType {
		return nil
	}

	apiGatewayID := *config.Cluster.APIGateway.ApiId

	route, err := config.AWS.DeleteRoute(apiGatewayID, endpoint)
	if err != nil {
		return err
	}

	if config.Cluster.APILoadBalancerScheme == clusterconfig.InternetFacingLoadBalancerScheme && route != nil {
		integrationID := aws.ExtractRouteIntegrationID(route)
		if integrationID != "" {
			err = config.AWS.DeleteIntegration(apiGatewayID, integrationID)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func UpdateAPIGateway(
	prevEndpoint string,
	prevAPIGatewayType userconfig.APIGatewayType,
	newEndpoint string,
	newAPIGatewayType userconfig.APIGatewayType,
) error {

	if prevAPIGatewayType == userconfig.NoneAPIGatewayType && newAPIGatewayType == userconfig.NoneAPIGatewayType {
		return nil
	}

	if prevAPIGatewayType == userconfig.PublicAPIGatewayType && newAPIGatewayType == userconfig.NoneAPIGatewayType {
		return RemoveAPIFromAPIGateway(prevEndpoint, prevAPIGatewayType)
	}

	if prevAPIGatewayType == userconfig.NoneAPIGatewayType && newAPIGatewayType == userconfig.PublicAPIGatewayType {
		return AddAPIToAPIGateway(newEndpoint, newAPIGatewayType)
	}

	if prevEndpoint == newEndpoint {
		return nil
	}

	// the endpoint has changed
	if err := AddAPIToAPIGateway(newEndpoint, newAPIGatewayType); err != nil {
		return err
	}
	if err := RemoveAPIFromAPIGateway(prevEndpoint, prevAPIGatewayType); err != nil {
		return err
	}

	return nil
}

func RemoveAPIFromAPIGatewayK8s(virtualService *kunstructured.Unstructured) error {
	if virtualService == nil {
		return nil // API is not running
	}

	apiGatewayType, err := userconfig.APIGatewayFromAnnotations(virtualService)
	if err != nil {
		return err
	}

	endpoint, err := ok8s.GetEndpointFromVirtualService(virtualService)
	if err != nil {
		return err
	}

	return RemoveAPIFromAPIGateway(endpoint, apiGatewayType)
}

func UpdateAPIGatewayK8s(prevVirtualService *kunstructured.Unstructured, newAPI *spec.API) error {
	prevAPIGatewayType, err := userconfig.APIGatewayFromAnnotations(prevVirtualService)
	if err != nil {
		return err
	}

	prevEndpoint, err := ok8s.GetEndpointFromVirtualService(prevVirtualService)
	if err != nil {
		return err
	}

	return UpdateAPIGateway(prevEndpoint, prevAPIGatewayType, *newAPI.Endpoint, newAPI.Networking.APIGateway)
}
