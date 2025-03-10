/*
Copyright (C) 2022-2025 ApeCloud Co., Ltd

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

	appsv1 "github.com/apecloud/kubeblocks/apis/apps/v1"
	"github.com/apecloud/kubeblocks/pkg/client/clientset/versioned"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type ConfigObjectsWrapper struct {
	namespace   string
	clusterName string
	components  []string

	rctxMap map[string]*ReconfigureContext
}

func GetCluster(clientSet *versioned.Clientset, ns, clusterName string) (*appsv1.Cluster, error) {
	clusterObj, err := clientSet.AppsV1().Clusters(ns).Get(context.TODO(), clusterName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	return clusterObj, nil
}

func New(clusterName string, namespace string, options *describeOpsOptions, components ...string) (*ConfigObjectsWrapper, error) {
	clientSet, err := GetClientFromOptions(options.factory)
	if err != nil {
		return nil, err
	}
	clusterObj, err := GetCluster(clientSet, namespace, clusterName)
	if err != nil {
		return nil, err
	}
	if len(components) == 0 {
		components = getComponentNames(clusterObj)
	}

	rctxAsMap := make(map[string]*ReconfigureContext)
	for _, compName := range components {
		rctx, err := generateReconfigureContext(context.TODO(), clientSet, clusterName, compName, namespace)
		if err != nil {
			return nil, err
		}
		rctxAsMap[rctx.CompName] = rctx
	}

	return &ConfigObjectsWrapper{
		namespace:   namespace,
		components:  components,
		clusterName: clusterName,
		rctxMap:     rctxAsMap,
	}, nil
}
