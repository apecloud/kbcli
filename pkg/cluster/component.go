/*
Copyright (C) 2022-2024 ApeCloud Co., Ltd

This file is part of KubeBlocks project

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program.  If not, see <http://www.gnu.org/licenses/>.
*/

package cluster

import (
	"context"
	"fmt"

	kbappsv1 "github.com/apecloud/kubeblocks/apis/apps/v1"
	appsv1alpha1 "github.com/apecloud/kubeblocks/apis/apps/v1alpha1"
	"github.com/apecloud/kubeblocks/pkg/constant"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic"

	"github.com/apecloud/kbcli/pkg/types"
)

type ComponentPair struct {
	// a unique name identifier for component object,
	// and labeled with "apps.kubeblocks.io/component-name"
	ComponentName    string
	ComponentDefName string
	ShardingName     string
}

func ListShardingComponents(dynamic dynamic.Interface, clusterName, clusterNamespace, componentName string) ([]*appsv1alpha1.Component, error) {
	unstructuredObjList, err := dynamic.Resource(types.ComponentGVR()).Namespace(clusterNamespace).List(context.TODO(), metav1.ListOptions{
		LabelSelector: fmt.Sprintf("%s=%s,%s=%s", constant.AppInstanceLabelKey, clusterName, constant.KBAppShardingNameLabelKey, componentName),
	})
	if err != nil {
		return nil, nil
	}
	var components []*appsv1alpha1.Component
	for i := range unstructuredObjList.Items {
		comp := &appsv1alpha1.Component{}
		if err = runtime.DefaultUnstructuredConverter.FromUnstructured(unstructuredObjList.Items[i].UnstructuredContent(), comp); err != nil {
			return nil, err
		}
		components = append(components, comp)
	}
	return components, nil
}

func BuildShardingComponentName(shardingCompName, componentName string) string {
	if shardingCompName == "" {
		return componentName
	}
	return fmt.Sprintf("%s(%s)", shardingCompName, componentName)
}

func GetCompSpecAndCheckSharding(cluster *kbappsv1.Cluster, componentName string) (*kbappsv1.ClusterComponentSpec, bool) {
	compSpec := cluster.Spec.GetComponentByName(componentName)
	if compSpec != nil {
		return compSpec, false
	}
	shardingSpec := cluster.Spec.GetShardingByName(componentName)
	if shardingSpec == nil {
		return nil, false
	}
	return &shardingSpec.Template, true
}

func GetClusterComponentPairs(dynamicClient dynamic.Interface, cluster *kbappsv1.Cluster) ([]ComponentPair, error) {
	var componentPairs []ComponentPair
	for _, compSpec := range cluster.Spec.ComponentSpecs {
		componentPairs = append(componentPairs, ComponentPair{
			ComponentName:    compSpec.Name,
			ComponentDefName: compSpec.ComponentDef,
		})
	}
	for _, shardingSpec := range cluster.Spec.ShardingSpecs {
		shardingComponentPairs, err := GetShardingComponentPairs(dynamicClient, cluster, shardingSpec)
		if err != nil {
			return nil, err
		}
		componentPairs = append(componentPairs, shardingComponentPairs...)
	}
	return componentPairs, nil
}

func GetShardingComponentPairs(dynamicClient dynamic.Interface, cluster *kbappsv1.Cluster, shardingSpec kbappsv1.ShardingSpec) ([]ComponentPair, error) {
	var componentPairs []ComponentPair
	shardingComps, err := ListShardingComponents(dynamicClient, cluster.Name, cluster.Namespace, shardingSpec.Name)
	if err != nil {
		return nil, err
	}
	if len(shardingComps) == 0 {
		return nil, fmt.Errorf(`cannot find any component objects for sharding component "%s"`, shardingSpec.Name)
	}
	for i := range shardingComps {
		compName := shardingComps[i].Labels[constant.KBAppComponentLabelKey]
		componentPairs = append(componentPairs, ComponentPair{
			ComponentName:    compName,
			ComponentDefName: shardingSpec.Template.ComponentDef,
			ShardingName:     shardingSpec.Name,
		})
	}
	return componentPairs, nil
}
