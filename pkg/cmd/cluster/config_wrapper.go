/*
Copyright (C) 2022-2026 ApeCloud Co., Ltd

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
)

type ReconfigureContext struct {
	Client  versioned.Interface
	Context context.Context

	Cluster        *kbappsv1.Cluster
	Cmpd           *kbappsv1.ComponentDefinition
	ConfigRender   *parametersv1alpha1.ParamConfigRenderer
	ParametersDefs []*parametersv1alpha1.ParametersDefinition

	CompName string
}

type ReconfigureWrapper struct {
	rctx *ReconfigureContext

	updatedParams map[string]*string

	// autofill field
	configSpecName string
	configFileKey  string
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

func GetClientFromOptionsOrDie(factory cmdutil.Factory) versioned.Interface {
	config, err := factory.ToRESTConfig()
	if err != nil {
		panic(err)
	}
	return versioned.NewForConfigOrDie(config)
}

func newConfigWrapper(clientSet versioned.Interface, ns, clusterName, componentName, templateName, fileName string, params map[string]*string) (*ReconfigureWrapper, error) {
	rctx, err := generateReconfigureContext(context.TODO(), clientSet, clusterName, componentName, ns)
	if err != nil {
		return nil, err
	}
	if len(rctx.ParametersDefs) == 0 && rctx.ConfigRender == nil {
		return nil, fmt.Errorf("the referenced component[%s] has no ParametersDefinitions or ParamConfigRenderer, and disable reconfigure", componentName)
	}

	return &ReconfigureWrapper{
		rctx:           rctx,
		configSpecName: templateName,
		configFileKey:  fileName,
		updatedParams:  params,
	}, nil
}

func generateReconfigureContext(ctx context.Context, clientSet versioned.Interface, clusterName, componentName, ns string) (*ReconfigureContext, error) {
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

	cmpd, err := resolveComponentDefObj(ctx, clientSet, clusterObj, componentName)
	if err != nil {
		return nil, err
	}
	rctx := &ReconfigureContext{
		Context:  ctx,
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

func resolveComponentDefObj(ctx context.Context, client versioned.Interface, clusterObj *kbappsv1.Cluster, componentName string) (*kbappsv1.ComponentDefinition, error) {
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
		return resolveCmpd(compSpec.ComponentDef)
	}

	shardingSpec := clusterObj.Spec.GetShardingByName(componentName)
	if shardingSpec == nil {
		return nil, makeComponentNotExistErr(clusterObj.Name, componentName)
	}
	if shardingSpec.ShardingDef != "" {
		return resolveShardingCmpd(shardingSpec.ShardingDef)
	}
	return resolveCmpd(shardingSpec.Template.ComponentDef)
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
