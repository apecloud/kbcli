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

package class

import (
	"context"
	"fmt"

	"github.com/apecloud/kubeblocks/pkg/controller/component"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic"

	"github.com/apecloud/kubeblocks/apis/apps/v1alpha1"
	"github.com/apecloud/kubeblocks/pkg/constant"

	"github.com/apecloud/kbcli/pkg/types"
)

// GetManager gets a class manager which manages default classes and user custom classes
func GetManager(client dynamic.Interface, cdName string) (*component.Manager, *v1alpha1.ComponentResourceConstraintList, error) {
	selector := fmt.Sprintf("%s=%s,%s", constant.ClusterDefLabelKey, cdName, types.ClassProviderLabelKey)
	classObjs, err := client.Resource(types.ComponentClassDefinitionGVR()).Namespace("").List(context.TODO(), metav1.ListOptions{
		LabelSelector: selector,
	})
	if err != nil {
		return nil, nil, err
	}
	var classDefinitionList v1alpha1.ComponentClassDefinitionList
	if err = runtime.DefaultUnstructuredConverter.FromUnstructured(classObjs.UnstructuredContent(), &classDefinitionList); err != nil {
		return nil, nil, err
	}

	constraintObjs, err := client.Resource(types.ComponentResourceConstraintGVR()).Namespace("").List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, nil, err
	}
	var resourceConstraintList v1alpha1.ComponentResourceConstraintList
	if err = runtime.DefaultUnstructuredConverter.FromUnstructured(constraintObjs.UnstructuredContent(), &resourceConstraintList); err != nil {
		return nil, nil, err
	}
	mgr, err := component.NewManager(classDefinitionList, resourceConstraintList)
	return mgr, &resourceConstraintList, err
}

// GetResourceConstraints gets all resource constraints
func GetResourceConstraints(dynamic dynamic.Interface) (map[string]*v1alpha1.ComponentResourceConstraint, error) {
	objs, err := dynamic.Resource(types.ComponentResourceConstraintGVR()).List(context.TODO(), metav1.ListOptions{
		// LabelSelector: types.ResourceConstraintProviderLabelKey,
	})
	if err != nil {
		return nil, err
	}
	var constraintsList v1alpha1.ComponentResourceConstraintList
	if err = runtime.DefaultUnstructuredConverter.FromUnstructured(objs.UnstructuredContent(), &constraintsList); err != nil {
		return nil, err
	}

	result := make(map[string]*v1alpha1.ComponentResourceConstraint)
	for idx := range constraintsList.Items {
		cf := constraintsList.Items[idx]
		if _, ok := cf.GetLabels()[constant.ResourceConstraintProviderLabelKey]; !ok {
			continue
		}
		result[cf.GetName()] = &cf
	}
	return result, nil
}

// GetCustomClassObjectName returns the name of the ComponentClassDefinition object containing the custom classes
func GetCustomClassObjectName(cdName string, componentName string) string {
	return fmt.Sprintf("kb.classes.custom.%s.%s", cdName, componentName)
}

func ValidateResources(clsMGR *component.Manager,
	resourceConstraintList *v1alpha1.ComponentResourceConstraintList,
	clusterDefRef string,
	comp *v1alpha1.ClusterComponentSpec) error {
	if comp.ClassDefRef != nil && comp.ClassDefRef.Class != "" {
		if clsMGR.HasClass(comp.ComponentDefRef, *comp.ClassDefRef) {
			return nil
		}
		return fmt.Errorf("class not found")
	}

	var rules []v1alpha1.ResourceConstraintRule
	for _, constraint := range resourceConstraintList.Items {
		rules = append(rules, constraint.FindRules(clusterDefRef, comp.ComponentDefRef)...)
	}
	if len(rules) == 0 {
		return nil
	}

	for _, rule := range rules {
		if !rule.ValidateResources(comp.Resources.Requests) {
			continue
		}

		// validate volume
		match := true
		// all volumes should match the rules
		for _, volume := range comp.VolumeClaimTemplates {
			if !rule.ValidateStorage(volume.Spec.Resources.Requests.Storage()) {
				match = false
				break
			}
		}
		if match {
			return nil
		}
	}
	return fmt.Errorf("resource is not conform to the constraints, please check the ComponentResourceConstraint API")
}
