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
	"strings"

	appsv1alpha1 "github.com/apecloud/kubeblocks/apis/apps/v1alpha1"
	appsv1beta1 "github.com/apecloud/kubeblocks/apis/apps/v1beta1"
	"github.com/apecloud/kubeblocks/pkg/constant"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/klog/v2"

	"github.com/apecloud/kbcli/pkg/testing"
	"github.com/apecloud/kbcli/pkg/types"
	"github.com/apecloud/kbcli/pkg/util"
)

const (
	ComponentNameEmpty = ""
)

// GetSimpleInstanceInfos returns simple instance info that only contains instance name and role, the default
// instance should be the first element in the returned array.
func GetSimpleInstanceInfos(dynamic dynamic.Interface, name, namespace string) []*InstanceInfo {
	return GetSimpleInstanceInfosForComponent(dynamic, name, ComponentNameEmpty, namespace)
}

// GetSimpleInstanceInfosForComponent returns simple instance info that only contains instance name and role for a component
func GetSimpleInstanceInfosForComponent(dynamic dynamic.Interface, name, componentName, namespace string) []*InstanceInfo {
	// missed in the status, try to list all pods and build instance info
	return getInstanceInfoByList(dynamic, name, componentName, namespace)
}

// getInstanceInfoByList gets instances info by listing all pods
func getInstanceInfoByList(dynamic dynamic.Interface, name, componentName, namespace string) []*InstanceInfo {
	var infos []*InstanceInfo
	// filter by cluster name
	labels := util.BuildLabelSelectorByNames("", []string{name})
	// filter by component name
	if len(componentName) > 0 {
		labels = util.BuildComponentNameLabels(labels, []string{componentName})
	}

	objs, err := dynamic.Resource(schema.GroupVersionResource{Group: corev1.GroupName, Version: types.K8sCoreAPIVersion, Resource: "pods"}).
		Namespace(namespace).List(context.TODO(), metav1.ListOptions{LabelSelector: labels})

	if err != nil {
		return nil
	}

	for _, o := range objs.Items {
		info := &InstanceInfo{Name: o.GetName()}
		podLabels := o.GetLabels()
		role, ok := podLabels[constant.RoleLabelKey]
		if ok {
			info.Role = role
		}
		if role == constant.Primary || role == constant.Leader {
			infos = append([]*InstanceInfo{info}, infos...)
		} else {
			infos = append(infos, info)
		}
	}
	return infos
}

// FindClusterComp finds component in cluster object based on the component definition name
func FindClusterComp(cluster *appsv1alpha1.Cluster, compDefName string) *appsv1alpha1.ClusterComponentSpec {
	for i, c := range cluster.Spec.ComponentSpecs {
		if c.ComponentDefRef == compDefName {
			return &cluster.Spec.ComponentSpecs[i]
		}
	}
	return nil
}

// GetComponentEndpoints gets component internal and external endpoints
func GetComponentEndpoints(svcList *corev1.ServiceList, c *appsv1alpha1.ClusterComponentSpec) ([]string, []string) {
	var (
		internalEndpoints []string
		externalEndpoints []string
	)

	getEndpoints := func(ip string, ports []corev1.ServicePort) []string {
		var result []string
		for _, port := range ports {
			result = append(result, fmt.Sprintf("%s:%d", ip, port.Port))
		}
		return result
	}

	internalSvcs, externalSvcs := GetComponentServices(svcList, c)
	for _, svc := range internalSvcs {
		dns := fmt.Sprintf("%s.%s.svc.cluster.local", svc.Name, svc.Namespace)
		internalEndpoints = append(internalEndpoints, getEndpoints(dns, svc.Spec.Ports)...)
	}

	for _, svc := range externalSvcs {
		externalEndpoints = append(externalEndpoints, getEndpoints(GetExternalAddr(svc), svc.Spec.Ports)...)
	}
	return internalEndpoints, externalEndpoints
}

// GetComponentServices gets component services
func GetComponentServices(svcList *corev1.ServiceList, c *appsv1alpha1.ClusterComponentSpec) ([]*corev1.Service, []*corev1.Service) {
	if svcList == nil {
		return nil, nil
	}

	var internalSvcs, externalSvcs []*corev1.Service
	for i, svc := range svcList.Items {
		if svc.GetLabels()[constant.KBAppComponentLabelKey] != c.Name {
			continue
		}

		var (
			internalIP   = svc.Spec.ClusterIP
			externalAddr = GetExternalAddr(&svc)
		)
		if svc.Spec.Type == corev1.ServiceTypeClusterIP && internalIP != "" && internalIP != "None" {
			internalSvcs = append(internalSvcs, &svcList.Items[i])
		}
		if externalAddr != "" {
			externalSvcs = append(externalSvcs, &svcList.Items[i])
		}
	}
	return internalSvcs, externalSvcs
}

// GetExternalAddr gets external IP from service annotation
func GetExternalAddr(svc *corev1.Service) string {
	for _, ingress := range svc.Status.LoadBalancer.Ingress {
		if ingress.Hostname != "" {
			return ingress.Hostname
		}

		if ingress.IP != "" {
			return ingress.IP
		}
	}
	if svc.GetAnnotations()[types.ServiceHAVIPTypeAnnotationKey] != types.ServiceHAVIPTypeAnnotationValue {
		return ""
	}
	return svc.GetAnnotations()[types.ServiceFloatingIPAnnotationKey]
}

func GetClusterDefByName(dynamic dynamic.Interface, name string) (*appsv1alpha1.ClusterDefinition, error) {
	clusterDef := &appsv1alpha1.ClusterDefinition{}
	if err := util.GetK8SClientObject(dynamic, clusterDef, types.ClusterDefGVR(), "", name); err != nil {
		return nil, err
	}
	return clusterDef, nil
}

func GetComponentDefByName(dynamic dynamic.Interface, name string) (*appsv1alpha1.ComponentDefinition, error) {
	componentDef := &appsv1alpha1.ComponentDefinition{}
	if err := util.GetK8SClientObject(dynamic, componentDef, types.CompDefGVR(), "", name); err != nil {
		return nil, err
	}
	return componentDef, nil
}

func GetDefaultCompName(cd *appsv1alpha1.ClusterDefinition) (string, error) {
	if len(cd.Spec.ComponentDefs) >= 1 {
		return cd.Spec.ComponentDefs[0].Name, nil
	}
	return "", fmt.Errorf("failed to get the default component definition name")
}

func GetClusterByName(dynamic dynamic.Interface, name string, namespace string) (*appsv1alpha1.Cluster, error) {
	cluster := &appsv1alpha1.Cluster{}
	if err := util.GetK8SClientObject(dynamic, cluster, types.ClusterGVR(), namespace, name); err != nil {
		return nil, err
	}
	return cluster, nil
}

func GetVersionByClusterDef(dynamic dynamic.Interface, clusterDef string) (*appsv1alpha1.ClusterVersionList, error) {
	versionList := &appsv1alpha1.ClusterVersionList{}
	obj, err := dynamic.Resource(types.ClusterVersionGVR()).List(context.TODO(), metav1.ListOptions{
		LabelSelector: fmt.Sprintf("%s=%s", constant.ClusterDefLabelKey, clusterDef),
	})
	if err != nil {
		return nil, err
	}
	if obj == nil {
		return nil, fmt.Errorf("failed to find component version referencing cluster definition %s", clusterDef)
	}
	if err = runtime.DefaultUnstructuredConverter.FromUnstructured(obj.UnstructuredContent(), versionList); err != nil {
		return nil, err
	}
	return versionList, nil
}

func FakeClusterObjs() *ClusterObjects {
	clusterObjs := NewClusterObjects()
	clusterObjs.Cluster = testing.FakeCluster(testing.ClusterName, testing.Namespace)
	clusterObjs.ClusterDef = testing.FakeClusterDef()
	clusterObjs.Pods = testing.FakePods(3, testing.Namespace, testing.ClusterName)
	clusterObjs.Secrets = testing.FakeSecrets(testing.Namespace, testing.ClusterName)
	clusterObjs.Nodes = []*corev1.Node{testing.FakeNode()}
	clusterObjs.Services = testing.FakeServices()
	return clusterObjs
}

func BuildStorageSize(storages []StorageInfo) string {
	var sizes []string
	for _, s := range storages {
		sizes = append(sizes, fmt.Sprintf("%s:%s", s.Name, s.Size))
	}
	return util.CheckEmpty(strings.Join(sizes, "\n"))
}

func BuildStorageClass(storages []StorageInfo) string {
	var scs []string
	for _, s := range storages {
		scs = append(scs, s.StorageClass)
	}
	return util.CheckEmpty(strings.Join(scs, "\n"))
}

// GetDefaultVersion gets the default cluster version that referencing the cluster definition.
// If only one version is found, it will be returned directly, otherwise the version with
// constant.DefaultClusterVersionAnnotationKey label will be returned.
func GetDefaultVersion(dynamic dynamic.Interface, clusterDef string) (string, error) {
	// if version already specified in the cluster definition, clusterVersion is not required
	cd, err := GetClusterDefByName(dynamic, clusterDef)
	if err != nil {
		return "", err
	}

	// check if all containers have been specified image
	podSpecWithImage := func(podSpec *corev1.PodSpec) bool {
		if podSpec == nil {
			return false
		}
		containers := podSpec.Containers
		containers = append(containers, podSpec.InitContainers...)
		for _, c := range containers {
			if c.Image == "" {
				return false
			}
		}
		return true
	}

	// check if all components have image
	allCompsWithVersion := true
	for _, compDef := range cd.Spec.ComponentDefs {
		if !podSpecWithImage(compDef.PodSpec) {
			allCompsWithVersion = false
			break
		}
	}
	if allCompsWithVersion {
		klog.V(1).Info("all components have been specified image, skip to get default cluster version")
		return "", nil
	}

	versionList, err := GetVersionByClusterDef(dynamic, clusterDef)
	if err != nil {
		return "", err
	}

	if len(versionList.Items) == 1 {
		return versionList.Items[0].Name, nil
	}

	defaultVersion := ""
	for _, item := range versionList.Items {
		if k, ok := item.Annotations[types.KBDefaultClusterVersionAnnotationKey]; !ok || k != "true" {
			continue
		}
		if defaultVersion != "" {
			return "", fmt.Errorf("found more than one default cluster version referencing cluster definition %s", clusterDef)
		}
		defaultVersion = item.Name
	}

	if defaultVersion == "" {
		return "", fmt.Errorf("failed to find default cluster version referencing cluster definition %s", clusterDef)
	}
	return defaultVersion, nil
}

type CompInfo struct {
	Component       *appsv1alpha1.ClusterComponentSpec
	ComponentStatus *appsv1alpha1.ClusterComponentStatus
	ComponentDef    *appsv1alpha1.ClusterComponentDefinition
}

func FillCompInfoByName(dynamic dynamic.Interface, namespace, clusterName, componentName string) (*CompInfo, error) {
	cluster, err := GetClusterByName(dynamic, clusterName, namespace)
	if err != nil {
		return nil, err
	}
	if cluster.Status.Phase != appsv1alpha1.RunningClusterPhase {
		return nil, fmt.Errorf("cluster %s is not running, please try again later", clusterName)
	}

	compInfo := &CompInfo{}
	// fill component
	if len(componentName) == 0 {
		compInfo.Component = &cluster.Spec.ComponentSpecs[0]
	} else {
		compInfo.Component = cluster.Spec.GetComponentByName(componentName)
	}

	if compInfo.Component == nil {
		return nil, fmt.Errorf("component %s not found in cluster %s", componentName, clusterName)
	}
	// fill component status
	for name, compStatus := range cluster.Status.Components {
		if name == compInfo.Component.Name {
			compInfo.ComponentStatus = &compStatus
			break
		}
	}
	if compInfo.ComponentStatus == nil {
		return nil, fmt.Errorf("componentStatus %s not found in cluster %s", componentName, clusterName)
	}

	// find cluster def
	clusterDef, err := GetClusterDefByName(dynamic, cluster.Spec.ClusterDefRef)
	if err != nil {
		return nil, err
	}
	// find component def by reference
	for _, compDef := range clusterDef.Spec.ComponentDefs {
		compRefName := compInfo.Component.ComponentDefRef
		if len(compRefName) == 0 {
			compRefName = compInfo.Component.ComponentDef
		}
		if compDef.Name == compRefName {
			compInfo.ComponentDef = &compDef
			break
		}
	}
	if compInfo.ComponentDef == nil {
		return nil, fmt.Errorf("componentDef %s not found in clusterDef %s", compInfo.Component.ComponentDefRef, clusterDef.Name)
	}
	return compInfo, nil
}

func GetPodClusterName(pod *corev1.Pod) string {
	if pod.Labels == nil {
		return ""
	}
	return pod.Labels[constant.AppInstanceLabelKey]
}

func GetPodComponentName(pod *corev1.Pod) string {
	if pod.Labels == nil {
		return ""
	}
	return pod.Labels[constant.KBAppComponentLabelKey]
}

func GetConfigMapByName(dynamic dynamic.Interface, namespace, name string) (*corev1.ConfigMap, error) {
	cmObj := &corev1.ConfigMap{}
	if err := util.GetK8SClientObject(dynamic, cmObj, types.ConfigmapGVR(), namespace, name); err != nil {
		return nil, err
	}
	return cmObj, nil
}

func GetConfigConstraintByName(dynamic dynamic.Interface, name string) (*appsv1beta1.ConfigConstraint, error) {
	ccObj := &appsv1beta1.ConfigConstraint{}
	if err := util.GetK8SClientObject(dynamic, ccObj, types.ConfigConstraintGVR(), "", name); err != nil {
		return nil, err
	}
	return ccObj, nil
}

func GetServiceRefs(cd *appsv1alpha1.ClusterDefinition) []string {
	var serviceRefs []string
	for _, compDef := range cd.Spec.ComponentDefs {
		if compDef.ServiceRefDeclarations == nil {
			continue
		}

		for _, ref := range compDef.ServiceRefDeclarations {
			serviceRefs = append(serviceRefs, ref.Name)
		}
	}
	return serviceRefs
}

// GetDefaultServiceRef will return the ServiceRefDeclarations in cluster-definition when the cluster-definition contains only one ServiceRefDeclaration
func GetDefaultServiceRef(cd *appsv1alpha1.ClusterDefinition) (string, error) {
	serviceRefs := GetServiceRefs(cd)
	if len(serviceRefs) != 1 {
		return "", fmt.Errorf("failed to get the cluster default service reference name")
	}
	return serviceRefs[0], nil
}

func GetDefaultVersionByCompDefs(dynamic dynamic.Interface, compDefs []string) (string, error) {
	cv := ""
	if compDefs == nil {
		return "", fmt.Errorf("failed to find default cluster version referencing the nil compDefs")
	}
	for _, compDef := range compDefs {
		comp, err := dynamic.Resource(types.CompDefGVR()).Get(context.Background(), compDef, metav1.GetOptions{})
		if err != nil {
			return "", fmt.Errorf("fail to get cluster version due to: %s", err.Error())
		}
		labels := comp.GetLabels()
		kind := labels[constant.AppNameLabelKey]
		version := labels[constant.AppVersionLabelKey]
		// todo: fix cv like:  mongodb-sharding-5.0, ac-mysql-8.0.30-auditlog
		if cv == "" {
			cv = fmt.Sprintf("%s-%s", kind, version)
		} else if cv != fmt.Sprintf("%s-%s", kind, version) {
			return "", fmt.Errorf("can't get the same cluster version by component definition:[%s]", strings.Join(compDefs, ","))
		}
	}
	return cv, nil
}
