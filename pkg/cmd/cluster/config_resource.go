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
	"golang.org/x/exp/slices"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/dynamic"
	"sigs.k8s.io/controller-runtime/pkg/client"

	appsv1alpha1 "github.com/apecloud/kubeblocks/apis/apps/v1alpha1"
	appsv1beta1 "github.com/apecloud/kubeblocks/apis/apps/v1beta1"
	"github.com/apecloud/kubeblocks/pkg/configuration/core"

	"github.com/apecloud/kbcli/pkg/types"
	"github.com/apecloud/kbcli/pkg/util"
)

type configSpecsType []*configSpecMeta

type configSpecMeta struct {
	Spec      appsv1alpha1.ComponentTemplateSpec
	ConfigMap *corev1.ConfigMap

	ConfigSpec       *appsv1alpha1.ComponentConfigSpec
	ConfigConstraint *appsv1beta1.ConfigConstraint
}

type ConfigRelatedObjects struct {
	Cluster        *appsv1alpha1.Cluster
	ConfigSpecs    map[string]configSpecsType
	Configurations []*appsv1alpha1.Configuration
}

type configObjectsWrapper struct {
	namespace   string
	clusterName string
	components  []string

	err error
	cli dynamic.Interface
}

func (c configSpecsType) findByName(name string) *configSpecMeta {
	for _, spec := range c {
		if spec.Spec.Name == name {
			return spec
		}
	}
	return nil
}

func (c configSpecsType) listConfigSpecs(ccFilter bool) []string {
	var names []string
	for _, spec := range c {
		if spec.ConfigSpec != nil && (!ccFilter || spec.ConfigConstraint != nil) {
			names = append(names, spec.Spec.Name)
		}
	}
	return names
}

func New(clusterName string, namespace string, cli dynamic.Interface, component ...string) *configObjectsWrapper {
	return &configObjectsWrapper{namespace, clusterName, component, nil, cli}
}

func (w *configObjectsWrapper) GetObjects() (*ConfigRelatedObjects, error) {
	objects := &ConfigRelatedObjects{}
	err := w.cluster(objects).
		// clusterDefinition(objects).
		// clusterVersion(objects).
		compConfigurations(objects).
		// comps(objects).
		configSpecsObjects(objects).
		finish()
	if err != nil {
		return nil, err
	}
	return objects, nil
}

func (w *configObjectsWrapper) configMap(specName string, component string, out *configSpecMeta) *configObjectsWrapper {
	fn := func() error {
		key := client.ObjectKey{
			Namespace: w.namespace,
			Name:      core.GetComponentCfgName(w.clusterName, component, specName),
		}
		out.ConfigMap = &corev1.ConfigMap{}
		return util.GetResourceObjectFromGVR(types.ConfigmapGVR(), key, w.cli, out.ConfigMap)
	}
	return w.objectWrapper(fn)
}

func (w *configObjectsWrapper) configConstraint(specName string, out *configSpecMeta) *configObjectsWrapper {
	fn := func() error {
		if specName == "" {
			return nil
		}
		key := client.ObjectKey{
			Namespace: "",
			Name:      specName,
		}
		out.ConfigConstraint = &appsv1beta1.ConfigConstraint{}
		return util.GetResourceObjectFromGVR(types.ConfigConstraintGVR(), key, w.cli, out.ConfigConstraint)
	}
	return w.objectWrapper(fn)
}

func (w *configObjectsWrapper) cluster(objects *ConfigRelatedObjects) *configObjectsWrapper {
	fn := func() error {
		clusterKey := client.ObjectKey{
			Namespace: w.namespace,
			Name:      w.clusterName,
		}
		objects.Cluster = &appsv1alpha1.Cluster{}
		if err := util.GetResourceObjectFromGVR(types.ClusterGVR(), clusterKey, w.cli, objects.Cluster); err != nil {
			return makeClusterNotExistErr(w.clusterName)
		}
		return nil
	}
	return w.objectWrapper(fn)
}

func (w *configObjectsWrapper) compConfigurations(object *ConfigRelatedObjects) *configObjectsWrapper {
	fn := func() error {
		for _, comp := range object.Cluster.Spec.ComponentSpecs {
			configKey := client.ObjectKey{
				Namespace: w.namespace,
				Name:      core.GenerateComponentConfigurationName(w.clusterName, comp.Name),
			}
			config := appsv1alpha1.Configuration{}
			if err := util.GetResourceObjectFromGVR(types.ConfigurationGVR(), configKey, w.cli, &config); err != nil {
				return err
			}
			object.Configurations = append(object.Configurations, &config)
		}
		return nil
	}
	return w.objectWrapper(fn)
}

func (w *configObjectsWrapper) configSpecsObjects(objects *ConfigRelatedObjects) *configObjectsWrapper {
	fn := func() error {
		configSpecs := make(map[string]configSpecsType, len(objects.Configurations))
		for _, component := range objects.Configurations {
			componentName := component.Spec.ComponentName
			if len(w.components) != 0 && !slices.Contains(w.components, componentName) {
				continue
			}
			if _, ok := configSpecs[componentName]; !ok {
				configSpecs[componentName] = make(configSpecsType, 0)
			}
			componentConfigSpecs, err := w.genConfigSpecsByConfiguration(&component.Spec)
			if err != nil {
				return err
			}
			configSpecs[componentName] = append(configSpecs[componentName], componentConfigSpecs...)
		}
		objects.ConfigSpecs = configSpecs
		return nil
	}
	return w.objectWrapper(fn)
}

func (w *configObjectsWrapper) finish() error {
	return w.err
}

func (w *configObjectsWrapper) genConfigSpecsByConfiguration(config *appsv1alpha1.ConfigurationSpec) (rets []*configSpecMeta, err error) {
	if len(config.ConfigItemDetails) == 0 {
		return
	}

	for _, item := range config.ConfigItemDetails {
		if item.ConfigSpec == nil {
			continue
		}
		specMeta, err := w.transformConfigSpecMeta(*item.ConfigSpec, config.ComponentName)
		if err != nil {
			return nil, err
		}
		rets = append(rets, specMeta)
	}
	return
}

func (w *configObjectsWrapper) transformConfigSpecMeta(spec appsv1alpha1.ComponentConfigSpec, component string) (*configSpecMeta, error) {
	specMeta := &configSpecMeta{
		Spec:       spec.ComponentTemplateSpec,
		ConfigSpec: spec.DeepCopy(),
	}
	err := w.configMap(spec.Name, component, specMeta).
		configConstraint(spec.ConfigConstraintRef, specMeta).
		finish()
	if err != nil {
		return nil, err
	}
	return specMeta, nil
}

func (w *configObjectsWrapper) objectWrapper(fn func() error) *configObjectsWrapper {
	if w.err != nil {
		return w
	}
	w.err = fn()
	return w
}
