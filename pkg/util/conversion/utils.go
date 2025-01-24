/*
Copyright (C) 2022-2025 ApeCloud Co., Ltd

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

package conversion

import (
	appsv1alpha1 "github.com/apecloud/kubeblocks/apis/apps/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apiruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func ResourcesWithGVR(versionMeta *VersionConversionMeta, gvr schema.GroupVersionResource, listOptions metav1.ListOptions) ([]appsv1alpha1.ConfigConstraint, error) {
	var resourcesList []appsv1alpha1.ConfigConstraint

	objList, err := versionMeta.Resource(gvr).List(versionMeta.Ctx, listOptions)
	if err != nil {
		return nil, err
	}
	for _, v := range objList.Items {
		obj := appsv1alpha1.ConfigConstraint{}
		if err := apiruntime.DefaultUnstructuredConverter.FromUnstructured(v.Object, &obj); err != nil {
			return nil, err
		}
		resourcesList = append(resourcesList, obj)
	}
	return resourcesList, nil
}
