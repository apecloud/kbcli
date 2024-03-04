/*
Copyright (C) 2022-2023 ApeCloud Co., Ltd

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
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/apecloud/kubeblocks/pkg/configuration/core"

	appsv1alpha1 "github.com/apecloud/kubeblocks/apis/apps/v1alpha1"
	cfgutil "github.com/apecloud/kubeblocks/pkg/configuration/util"

	"github.com/apecloud/kbcli/pkg/action"
	"github.com/apecloud/kbcli/pkg/cluster"
	"github.com/apecloud/kbcli/pkg/types"
	"github.com/apecloud/kbcli/pkg/util"
)

type configWrapper struct {
	action.CreateOptions
	*appsv1alpha1.Cluster

	clusterName   string
	updatedParams map[string]*string

	// autofill field
	componentName  string
	configSpecName string
	configFileKey  string

	configTemplateSpec appsv1alpha1.ComponentConfigSpec

	// clusterDefObj *appsv1alpha1.ClusterDefinition
	// clusterVerObj *appsv1alpha1.ClusterVersion

	// 0.8 KubeBlocks API
	// comps    []*appsv1alpha1.Component
	// compDefs []*appsv1alpha1.ComponentDefinition
}

func (w *configWrapper) ConfigTemplateSpec() *appsv1alpha1.ComponentConfigSpec {
	return &w.configTemplateSpec
}

func (w *configWrapper) ConfigSpecName() string {
	return w.configSpecName
}

func (w *configWrapper) ComponentName() string {
	return w.componentName
}

func (w *configWrapper) ConfigFile() string {
	return w.configFileKey
}

// AutoFillRequiredParam auto fills required param.
func (w *configWrapper) AutoFillRequiredParam() error {
	if err := w.fillComponent(); err != nil {
		return err
	}
	if err := w.fillConfigSpec(); err != nil {
		return err
	}
	return w.fillConfigFile()
}

// ValidateRequiredParam validates required param.
func (w *configWrapper) ValidateRequiredParam(forceReplace bool) error {
	// step1: check existence of component.
	if w.Spec.GetComponentByName(w.componentName) == nil {
		return makeComponentNotExistErr(w.clusterName, w.componentName)
	}

	// step2: check existence of configmap
	cmObj := corev1.ConfigMap{}
	cmKey := client.ObjectKey{
		Name:      core.GetComponentCfgName(w.clusterName, w.componentName, w.configSpecName),
		Namespace: w.Namespace,
	}
	if err := util.GetResourceObjectFromGVR(types.ConfigmapGVR(), cmKey, w.Dynamic, &cmObj); err != nil {
		return err
	}

	// step3: check existence of config file
	if _, ok := cmObj.Data[w.configFileKey]; !ok {
		return makeNotFoundConfigFileErr(w.configFileKey, w.configSpecName, cfgutil.ToSet(cmObj.Data).AsSlice())
	}

	if !forceReplace && !core.IsSupportConfigFileReconfigure(w.configTemplateSpec, w.configFileKey) {
		return makeNotSupportConfigFileUpdateErr(w.configFileKey, w.configTemplateSpec)
	}
	return nil
}

func (w *configWrapper) fillComponent() error {
	if w.componentName != "" {
		return nil
	}
	componentNames, err := util.GetComponentsFromResource(w.Namespace, w.clusterName, w.Spec.ComponentSpecs, w.Dynamic)
	if err != nil {
		return err
	}
	if len(componentNames) != 1 {
		return core.MakeError(multiComponentsErrorMessage)
	}
	w.componentName = componentNames[0]
	return nil
}

func (w *configWrapper) fillConfigSpec() error {
	foundConfigSpec := func(configSpecs []appsv1alpha1.ComponentConfigSpec, name string) *appsv1alpha1.ComponentConfigSpec {
		for _, configSpec := range configSpecs {
			if configSpec.Name == name {
				w.configTemplateSpec = configSpec
				return &configSpec
			}
		}
		return nil
	}

	configSpecs, err := util.GetConfigSpecsFromComponentName(w.GetNamespace(), w.clusterName, w.componentName, w.configSpecName == "", w.Dynamic)
	if err != nil {
		return err
	}
	if len(configSpecs) == 0 {
		return makeNotFoundTemplateErr(w.clusterName, w.componentName)
	}

	if w.configSpecName != "" {
		if foundConfigSpec(configSpecs, w.configSpecName) == nil {
			return makeConfigSpecNotExistErr(w.clusterName, w.componentName, w.configSpecName)
		}
		return nil
	}

	w.configTemplateSpec = configSpecs[0]
	if len(configSpecs) == 1 {
		w.configSpecName = configSpecs[0].Name
		return nil
	}

	if len(w.updatedParams) == 0 {
		return core.MakeError(multiConfigTemplateErrorMessage)
	}
	supportUpdatedTpl := make([]appsv1alpha1.ComponentConfigSpec, 0)
	for _, configSpec := range configSpecs {
		if ok, err := util.IsSupportReconfigureParams(configSpec, w.updatedParams, w.Dynamic); err == nil && ok {
			supportUpdatedTpl = append(supportUpdatedTpl, configSpec)
		}
	}
	if len(supportUpdatedTpl) == 1 {
		w.configTemplateSpec = configSpecs[0]
		w.configSpecName = supportUpdatedTpl[0].Name
		return nil
	}
	return core.MakeError(multiConfigTemplateErrorMessage)
}

func (w *configWrapper) fillConfigFile() error {
	if w.configFileKey != "" {
		return nil
	}

	if w.configTemplateSpec.TemplateRef == "" {
		return makeNotFoundTemplateErr(w.clusterName, w.componentName)
	}

	cmObj := corev1.ConfigMap{}
	cmKey := client.ObjectKey{
		Name:      core.GetComponentCfgName(w.clusterName, w.componentName, w.configSpecName),
		Namespace: w.Namespace,
	}
	if err := util.GetResourceObjectFromGVR(types.ConfigmapGVR(), cmKey, w.Dynamic, &cmObj); err != nil {
		return err
	}
	if len(cmObj.Data) == 0 {
		return core.MakeError("not supported reconfiguring because there is no config file.")
	}

	keys := w.filterForReconfiguring(cmObj.Data)
	if len(keys) == 1 {
		w.configFileKey = keys[0]
		return nil
	}
	return core.MakeError(multiConfigFileErrorMessage)
}

func (w *configWrapper) filterForReconfiguring(data map[string]string) []string {
	keys := make([]string, 0, len(data))
	for configFileKey := range data {
		if core.IsSupportConfigFileReconfigure(w.configTemplateSpec, configFileKey) {
			keys = append(keys, configFileKey)
		}
	}
	return keys
}

func newConfigWrapper(baseOptions action.CreateOptions, componentName, configSpec, configKey string, params map[string]*string) (*configWrapper, error) {
	var err error
	var clusterObj *appsv1alpha1.Cluster

	if clusterObj, err = cluster.GetClusterByName(baseOptions.Dynamic, baseOptions.Name, baseOptions.Namespace); err != nil {
		return nil, err
	}
	return &configWrapper{
		CreateOptions:  baseOptions,
		Cluster:        clusterObj,
		clusterName:    baseOptions.Name,
		componentName:  componentName,
		configSpecName: configSpec,
		configFileKey:  configKey,
		updatedParams:  params,
	}, nil
}
