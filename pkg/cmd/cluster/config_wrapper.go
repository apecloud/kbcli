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
	"errors"
	"fmt"

	kbappsv1 "github.com/apecloud/kubeblocks/apis/apps/v1"
	parametersv1alpha1 "github.com/apecloud/kubeblocks/apis/parameters/v1alpha1"
	"github.com/apecloud/kubeblocks/pkg/client/clientset/versioned"
	intctrlutil "github.com/apecloud/kubeblocks/pkg/controllerutil"
	"github.com/apecloud/kubeblocks/pkg/generics"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"

	"github.com/apecloud/kbcli/pkg/action"
)

type ReconfigureContext struct {
	Client  *versioned.Clientset
	Context context.Context

	Cluster        *kbappsv1.Cluster
	Cmpd           *kbappsv1.ComponentDefinition
	ConfigRender   *parametersv1alpha1.ParamConfigRenderer
	ParametersDefs []*parametersv1alpha1.ParametersDefinition

	Sharding bool
	CompName string
}

type ReconfigureWrapper struct {
	action.CreateOptions

	rctx *ReconfigureContext

	updatedParams map[string]*string

	// autofill field
	configSpecName string
	configFileKey  string

	configTemplateSpec kbappsv1.ComponentTemplateSpec
}

func (w *ReconfigureWrapper) ConfigSpecName() string {
	if w.configFileKey != "" {
		return w.configFileKey
	}
	file := w.ConfigFile()
	if file != "" && w.rctx.ConfigRender != nil {
		config := intctrlutil.GetComponentConfigDescription(&w.rctx.ConfigRender.Spec, file)
		if config != nil {
			return config.TemplateName
		}
	}
	return ""
}

func (w *ReconfigureWrapper) ComponentName() string {
	return w.rctx.CompName
}

func (w *ReconfigureWrapper) ConfigFile() string {
	if w.configFileKey != "" {
		return w.configFileKey
	}
	if w.rctx.ConfigRender != nil && len(w.rctx.ConfigRender.Spec.Configs) > 0 {
		return w.rctx.ConfigRender.Spec.Configs[0].Name
	}
	return ""
}

// AutoFillRequiredParam auto fills required param.
// func (w *ReconfigureWrapper) AutoFillRequiredParam() error {
// 	if err := w.fillConfigSpec(); err != nil {
// 		return err
// 	}
// 	return w.fillConfigFile()
// }

// ValidateRequiredParam validates required param.
// func (w *ReconfigureWrapper) ValidateRequiredParam(forceReplace bool) error {
// 	// step1: check existence of component.
// 	if w.Spec.GetComponentByName(w.componentName) == nil {
// 		return makeComponentNotExistErr(w.clusterName, w.componentName)
// 	}
//
// 	// step2: check existence of configmap
// 	cmObj := corev1.ConfigMap{}
// 	cmKey := client.ObjectKey{
// 		Name:      core.GetComponentCfgName(w.clusterName, w.componentName, w.configSpecName),
// 		Namespace: w.Namespace,
// 	}
// 	if err := util.GetResourceObjectFromGVR(types.ConfigmapGVR(), cmKey, w.Dynamic, &cmObj); err != nil {
// 		return err
// 	}
//
// 	// step3: check existence of config file
// 	if _, ok := cmObj.Data[w.configFileKey]; !ok {
// 		return makeNotFoundConfigFileErr(w.configFileKey, w.configSpecName, cfgutil.ToSet(cmObj.Data).AsSlice())
// 	}
//
// 	if !forceReplace && !util.IsSupportConfigFileReconfigure(w.configTemplateSpec, w.configFileKey) {
// 		return makeNotSupportConfigFileUpdateErr(w.configFileKey, w.configTemplateSpec)
// 	}
// 	return nil
// }

func (w *ReconfigureWrapper) fillConfigSpec() error {
	var rctx = w.rctx

	if rctx.ConfigRender == nil || len(rctx.ConfigRender.Spec.Configs) == 0 {
		return makeNotFoundTemplateErr(w.Name, rctx.CompName)
	}

	var configs []parametersv1alpha1.ComponentConfigDescription
	if w.configSpecName != "" {
		configs = intctrlutil.GetComponentConfigDescriptions(&rctx.ConfigRender.Spec, w.configSpecName)
		if len(configs) == 0 {
			return makeConfigSpecNotExistErr(w.Name, rctx.CompName, w.configSpecName)
		}
	}
	return nil
}

// func (w *ReconfigureWrapper) fillConfigFile() error {
// 	if w.configFileKey != "" {
// 		return nil
// 	}
//
// 	if w.configTemplateSpec.TemplateRef == "" {
// 		return makeNotFoundTemplateErr(w.clusterName, w.componentName)
// 	}
//
// 	cmObj := corev1.ConfigMap{}
// 	cmKey := client.ObjectKey{
// 		Name:      core.GetComponentCfgName(w.clusterName, w.componentName, w.configSpecName),
// 		Namespace: w.Namespace,
// 	}
// 	if err := util.GetResourceObjectFromGVR(types.ConfigmapGVR(), cmKey, w.Dynamic, &cmObj); err != nil {
// 		return err
// 	}
// 	if len(cmObj.Data) == 0 {
// 		return core.MakeError("not supported reconfiguring because there is no config file.")
// 	}
//
// 	keys := w.filterForReconfiguring(cmObj.Data)
// 	if len(keys) == 1 {
// 		w.configFileKey = keys[0]
// 		return nil
// 	}
// 	return core.MakeError(multiConfigFileErrorMessage)
// }

// func (w *ReconfigureWrapper) filterForReconfiguring(data map[string]string) []string {
// 	keys := make([]string, 0, len(data))
// 	for configFileKey := range data {
// 		if util.IsSupportConfigFileReconfigure(w.configTemplateSpec, configFileKey) {
// 			keys = append(keys, configFileKey)
// 		}
// 	}
// 	return keys
// }

func GetClientFromOptions(factory cmdutil.Factory) (*versioned.Clientset, error) {
	config, err := factory.ToRESTConfig()
	if err != nil {
		return nil, err
	}

	return versioned.NewForConfigOrDie(config), err
}

func newConfigWrapper(baseOptions action.CreateOptions, componentName, templateName, fileName string, params map[string]*string) (*ReconfigureWrapper, error) {
	cli, err := GetClientFromOptions(baseOptions.Factory)
	if err != nil {
		return nil, err
	}

	rctx, err := generateReconfigureContext(context.TODO(), cli, baseOptions.Name, componentName, baseOptions.Namespace)
	if err != nil {
		return nil, err
	}
	if len(rctx.ParametersDefs) == 0 && rctx.ConfigRender == nil {
		return nil, fmt.Errorf("the referenced component[%s] has no ParametersDefinitions or ParamConfigRenderer, and disable reconfigure", componentName)
	}

	return &ReconfigureWrapper{
		CreateOptions:  baseOptions,
		rctx:           rctx,
		configSpecName: templateName,
		configFileKey:  fileName,
		updatedParams:  params,
	}, nil
}

func generateReconfigureContext(ctx context.Context, clientSet *versioned.Clientset, clusterName, componentName, ns string) (*ReconfigureContext, error) {
	defaultCompName := func(clusterSpec kbappsv1.ClusterSpec) string {
		switch {
		case len(clusterSpec.ComponentSpecs) != 0:
			return clusterSpec.ComponentSpecs[0].Name
		case len(clusterSpec.Shardings) == 0:
			return clusterSpec.Shardings[0].Name
		default:
			panic("cluster not have any component or sharding")
		}
	}

	clusterObj, err := clientSet.AppsV1().Clusters(ns).Get(ctx, clusterName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	if componentName == "" {
		componentName = defaultCompName(clusterObj.Spec)
	}

	sharding, cmpd, err := resolveComponentDefObj(ctx, clientSet, clusterObj, componentName)
	if err != nil {
		return nil, err
	}
	rctx := &ReconfigureContext{
		Context:  ctx,
		Sharding: sharding,
		Cmpd:     cmpd,
		Cluster:  clusterObj,
		Client:   clientSet,
		CompName: componentName,
	}

	if err = resolveCmpdParametersDefs(rctx); err != nil {
		return nil, err
	}
	return rctx, nil
}

func resolveComponentDefObj(ctx context.Context, client *versioned.Clientset, clusterObj *kbappsv1.Cluster, componentName string) (sharding bool, cmpd *kbappsv1.ComponentDefinition, err error) {
	resolveCmpd := func(cmpdName string) (*kbappsv1.ComponentDefinition, error) {
		if cmpdName == "" {
			return nil, errors.New("the referenced ComponentDefinition is empty")
		}
		return client.AppsV1().
			ComponentDefinitions().
			Get(ctx, cmpdName, metav1.GetOptions{})
	}
	resolveShardingCmpd := func(cmpdName string) (*kbappsv1.ComponentDefinition, error) {
		shardingCmpd, err := client.AppsV1().
			ShardingDefinitions().
			Get(ctx, cmpdName, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
		if shardingCmpd.Spec.Template.CompDef == "" {
			return nil, errors.New("the referenced ShardingDefinition has no ComponentDefinition")
		}
		return resolveCmpd(shardingCmpd.Spec.Template.CompDef)
	}

	compSpec := clusterObj.Spec.GetComponentByName(componentName)
	if compSpec != nil {
		cmpd, err = resolveCmpd(compSpec.ComponentDef)
		return
	}
	shardingSpec := clusterObj.Spec.GetShardingByName(componentName)
	if shardingSpec == nil {
		err = makeComponentNotExistErr(clusterObj.Name, componentName)
		return
	}

	sharding = true
	if shardingSpec.ShardingDef != "" {
		cmpd, err = resolveShardingCmpd(shardingSpec.ShardingDef)
	}

	cmpd, err = resolveCmpd(shardingSpec.Template.ComponentDef)
	return
}

func resolveCmpdParametersDefs(rctx *ReconfigureContext) error {
	configRender, err := resolveComponentConfigRender(rctx, rctx.Cmpd)
	if err != nil {
		return err
	}
	if configRender == nil || len(configRender.Spec.ParametersDefs) == 0 {
		return nil
	}
	rctx.ConfigRender = configRender
	for _, defName := range configRender.Spec.ParametersDefs {
		pd, err := rctx.Client.ParametersV1alpha1().ParametersDefinitions().Get(rctx.Context, defName, metav1.GetOptions{})
		if err != nil {
			return err
		}
		rctx.ParametersDefs = append(rctx.ParametersDefs, pd)
	}
	return nil
}

func resolveComponentConfigRender(rctx *ReconfigureContext, cmpd *kbappsv1.ComponentDefinition) (*parametersv1alpha1.ParamConfigRenderer, error) {
	pcrList, err := rctx.Client.ParametersV1alpha1().ParamConfigRenderers().List(rctx.Context, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	var prcs []parametersv1alpha1.ParamConfigRenderer
	for i, item := range pcrList.Items {
		if item.Spec.ComponentDef != cmpd.Name {
			continue
		}
		if item.Spec.ServiceVersion == "" || item.Spec.ServiceVersion == cmpd.Spec.ServiceVersion {
			prcs = append(prcs, pcrList.Items[i])
		}
	}
	if len(prcs) == 1 {
		return &prcs[0], nil
	}
	if len(prcs) > 1 {
		return nil, fmt.Errorf("the ParamConfigRenderer is ambiguous which referenced cmpd[%s], prcs: [%s]", cmpd.Namespace,
			generics.Map(prcs, func(pcr parametersv1alpha1.ParamConfigRenderer) string { return pcr.Name }))
	}
	return nil, nil
}
