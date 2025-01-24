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
	"fmt"

	appsv1alpha1 "github.com/apecloud/kubeblocks/apis/apps/v1alpha1"
	appsv1beta1 "github.com/apecloud/kubeblocks/apis/apps/v1beta1"
	"github.com/apecloud/kubeblocks/pkg/constant"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	apiruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/apecloud/kbcli/pkg/types"
	"github.com/apecloud/kbcli/pkg/util"
)

const (
	OldVersion = "08"
	NewVersion = "09"
)

func FetchAndConversionResources(versionMeta *VersionConversionMeta) ([]unstructured.Unstructured, error) {
	var resources []unstructured.Unstructured

	if versionMeta.FromVersion == versionMeta.ToVersion {
		return nil, nil
	}

	if versionMeta.FromVersion != OldVersion || versionMeta.ToVersion != NewVersion {
		klog.V(1).Infof("not to convert configconstraint multiversion")
		return nil, nil
	}

	oldResources, err := ResourcesWithGVR(versionMeta, types.ConfigConstraintOldGVR(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	for i := range oldResources {
		oldObj := oldResources[i]
		newObj := &appsv1beta1.ConfigConstraint{
			TypeMeta: metav1.TypeMeta{
				Kind:       types.KindConfigConstraint,
				APIVersion: types.ConfigConstraintGVR().GroupVersion().String(),
			},
		}
		klog.V(1).Infof("convert configconstraint[%s] cr from v1alpha1 to v1beta1", oldObj.GetName())
		// If v1alpha1 is converted from v1beta1 version
		if hasConversionVersion(&oldObj) {
			klog.V(1).Infof("configconstraint[%s] v1alpha1 is converted from v1beta1 version and ignore.",
				client.ObjectKeyFromObject(&oldObj).String())
			continue
		}
		// If the converted version v1beta1 already exists
		if hasValidBetaVersion(&oldObj, versionMeta) {
			klog.V(1).Infof("configconstraint[%s] v1beta1 already exist and ignore.",
				client.ObjectKeyFromObject(&oldObj).String())
			continue
		}
		newObj, err = convert(&oldObj)
		if err != nil {
			return nil, err
		}
		item, err := apiruntime.DefaultUnstructuredConverter.ToUnstructured(&newObj)
		if err != nil {
			return nil, err
		}
		resources = append(resources, unstructured.Unstructured{Object: item})
	}
	return resources, nil
}

func hasValidBetaVersion(obj *appsv1alpha1.ConfigConstraint, dynamic dynamic.Interface) bool {
	newObj := appsv1beta1.ConfigConstraint{}
	if err := util.GetResourceObjectFromGVR(types.ConfigConstraintGVR(), client.ObjectKeyFromObject(obj), dynamic, &newObj); err != nil {
		return false
	}

	return hasConversionVersion(&newObj)
}

func hasConversionVersion(obj client.Object) bool {
	annotations := obj.GetAnnotations()
	if len(annotations) == 0 {
		return false
	}
	return annotations[constant.KubeblocksAPIConversionTypeAnnotationName] == constant.MigratedAPIVersion
}

func UpdateNewVersionResources(versionMeta *VersionConversionMeta, targetObjects []unstructured.Unstructured) error {
	if len(targetObjects) == 0 {
		return nil
	}
	if versionMeta.FromVersion == versionMeta.ToVersion {
		return nil
	}

	for _, unstructuredObj := range targetObjects {
		klog.V(1).Infof("update CR %s", unstructuredObj.GetName())
		_, err := versionMeta.Resource(types.ConfigConstraintGVR()).
			Update(versionMeta.Ctx, &unstructuredObj, metav1.UpdateOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				if _, err := versionMeta.Resource(types.ConfigConstraintGVR()).
					Create(versionMeta.Ctx, &unstructuredObj, metav1.CreateOptions{}); err != nil {
					klog.V(1).ErrorS(err, "failed to create configConstraint")
					return err
				}
				continue
			}
			klog.V(1).ErrorS(err, fmt.Sprintf("failed to update configconstraint cr[%v]", unstructuredObj.GetName()))
			return err
		}
	}
	return nil
}

func convert(from *appsv1alpha1.ConfigConstraint) (*appsv1beta1.ConfigConstraint, error) {
	newObj := &appsv1beta1.ConfigConstraint{
		TypeMeta: metav1.TypeMeta{
			Kind:       from.Kind,
			APIVersion: types.ConfigConstraintGVR().GroupVersion().String(),
		},
	}
	if err := convertImpl(from, newObj); err != nil {
		return nil, err
	}
	return newObj, nil
}

func convertImpl(source *appsv1alpha1.ConfigConstraint, target *appsv1beta1.ConfigConstraint) error {
	target.ObjectMeta = source.ObjectMeta
	if target.Annotations == nil {
		target.Annotations = make(map[string]string)
	}
	target.Annotations[constant.KubeblocksAPIConversionTypeAnnotationName] = constant.MigratedAPIVersion
	target.Annotations[constant.SourceAPIVersionAnnotationName] = appsv1alpha1.GroupVersion.Version
	convertToConstraintSpec(&source.Spec, &target.Spec)
	return nil
}

func convertToConstraintSpec(source *appsv1alpha1.ConfigConstraintSpec, target *appsv1beta1.ConfigConstraintSpec) {
	target.MergeReloadAndRestart = source.DynamicActionCanBeMerged
	target.ReloadStaticParamsBeforeRestart = source.ReloadStaticParamsBeforeRestart
	target.DownwardAPIChangeTriggeredActions = source.DownwardAPIOptions
	target.StaticParameters = source.StaticParameters
	target.DynamicParameters = source.DynamicParameters
	target.ImmutableParameters = source.ImmutableParameters
	target.FileFormatConfig = source.FormatterConfig
	convertDynamicReloadAction(source.ReloadOptions, target, source.ToolsImageSpec, source.ScriptConfigs, source.Selector)
	convertSchema(source.ConfigurationSchema, source.CfgSchemaTopLevelName, target)
}

func convertDynamicReloadAction(options *appsv1alpha1.ReloadOptions, target *appsv1beta1.ConfigConstraintSpec,
	toolsSetup *appsv1beta1.ToolsSetup, configs []appsv1beta1.ScriptConfig, selector *metav1.LabelSelector) {
	if options == nil {
		return
	}
	target.ReloadAction = &appsv1beta1.ReloadAction{
		UnixSignalTrigger: options.UnixSignalTrigger,
		ShellTrigger:      options.ShellTrigger,
		TPLScriptTrigger:  options.TPLScriptTrigger,
		AutoTrigger:       options.AutoTrigger,
		TargetPodSelector: selector,
	}
	if target.ReloadAction.ShellTrigger != nil {
		target.ReloadAction.ShellTrigger.ToolsSetup = toolsSetup
		if len(configs) > 0 {
			target.ReloadAction.ShellTrigger.ScriptConfig = configs[0].DeepCopy()
		}
	}
}

func convertSchema(schema *appsv1alpha1.CustomParametersValidation, topLevelKey string, target *appsv1beta1.ConfigConstraintSpec) {
	if schema == nil {
		return
	}
	target.ParametersSchema = &appsv1beta1.ParametersSchema{
		TopLevelKey:  topLevelKey,
		CUE:          schema.CUE,
		SchemaInJSON: schema.Schema,
	}
}
