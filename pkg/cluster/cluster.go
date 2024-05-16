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

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/kubectl/pkg/util/resource"

	appsv1alpha1 "github.com/apecloud/kubeblocks/apis/apps/v1alpha1"
	dpv1alpha1 "github.com/apecloud/kubeblocks/apis/dataprotection/v1alpha1"
	"github.com/apecloud/kubeblocks/pkg/constant"
	dptypes "github.com/apecloud/kubeblocks/pkg/dataprotection/types"

	"github.com/apecloud/kbcli/pkg/types"
	"github.com/apecloud/kbcli/pkg/util"
)

// ConditionsError cluster displays this status on list cmd when the status of ApplyResources or ProvisioningStarted condition is "False".
const ConditionsError = "ConditionsError"

type TypeNeed int

const (
	NoNeed = iota
	Need
	Maybe
)

type GetOptions struct {
	WithClusterDef     TypeNeed
	WithClusterVersion TypeNeed
	WithConfigMap      TypeNeed
	WithPVC            TypeNeed
	WithService        TypeNeed
	WithSecret         TypeNeed
	WithPod            TypeNeed
	WithEvent          TypeNeed
	WithDataProtection TypeNeed
	WithCompDef        TypeNeed
	WithComp           TypeNeed
}

type ObjectsGetter struct {
	Client    clientset.Interface
	Dynamic   dynamic.Interface
	Name      string
	Namespace string
	GetOptions
}

func NewClusterObjects() *ClusterObjects {
	return &ClusterObjects{
		Cluster:  &appsv1alpha1.Cluster{},
		Nodes:    []*corev1.Node{},
		CompDefs: []*appsv1alpha1.ComponentDefinition{},
	}
}

func listResources[T any](dynamic dynamic.Interface, gvr schema.GroupVersionResource, ns string, opts metav1.ListOptions, items *[]T) error {
	if *items == nil {
		*items = []T{}
	}
	obj, err := dynamic.Resource(gvr).Namespace(ns).List(context.TODO(), opts)
	if err != nil {
		return err
	}
	for _, i := range obj.Items {
		var object T
		if err = runtime.DefaultUnstructuredConverter.FromUnstructured(i.Object, &object); err != nil {
			return err
		}
		*items = append(*items, object)
	}
	return nil
}

// Get all kubernetes objects belonging to the database cluster
func (o *ObjectsGetter) Get() (*ClusterObjects, error) {
	var err error

	objs := NewClusterObjects()
	ctx := context.TODO()
	client := o.Client.CoreV1()
	getResource := func(gvr schema.GroupVersionResource, name string, ns string, res interface{}) error {
		obj, err := o.Dynamic.Resource(gvr).Namespace(ns).Get(ctx, name, metav1.GetOptions{}, "")
		if err != nil {
			return err
		}
		return runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, res)
	}

	listOpts := func() metav1.ListOptions {
		return metav1.ListOptions{
			LabelSelector: fmt.Sprintf("%s=%s,%s=%s",
				constant.AppInstanceLabelKey, o.Name,
				constant.AppManagedByLabelKey, constant.AppName),
		}
	}

	// get cluster
	if err = getResource(types.ClusterGVR(), o.Name, o.Namespace, objs.Cluster); err != nil {
		return nil, err
	}

	provisionCondition := meta.FindStatusCondition(objs.Cluster.Status.Conditions, appsv1alpha1.ConditionTypeProvisioningStarted)
	if provisionCondition != nil && provisionCondition.Status == metav1.ConditionFalse {
		objs.Cluster.Status.Phase = ConditionsError
	}

	applyResourcesCondition := meta.FindStatusCondition(objs.Cluster.Status.Conditions, appsv1alpha1.ConditionTypeApplyResources)
	if applyResourcesCondition != nil && applyResourcesCondition.Status == metav1.ConditionFalse {
		objs.Cluster.Status.Phase = ConditionsError
	}
	// get cluster definition
	if o.WithClusterDef == Need || o.WithClusterDef == Maybe {
		cd := &appsv1alpha1.ClusterDefinition{}
		if err = getResource(types.ClusterDefGVR(), objs.Cluster.Spec.ClusterDefRef, "", cd); err != nil && o.WithClusterDef == Need {
			return nil, err
		}
		objs.ClusterDef = cd
	}

	// get cluster version
	if o.WithClusterVersion == Need {
		v := &appsv1alpha1.ClusterVersion{}
		if err = getResource(types.ClusterVersionGVR(), objs.Cluster.Spec.ClusterVersionRef, "", v); err != nil {
			return nil, err
		}
		objs.ClusterVersion = v
	}

	// get services
	if o.WithService == Need {
		if objs.Services, err = client.Services(o.Namespace).List(ctx, listOpts()); err != nil {
			return nil, err
		}
	}

	// get secrets
	if o.WithSecret == Need {
		if objs.Secrets, err = client.Secrets(o.Namespace).List(ctx, listOpts()); err != nil {
			return nil, err
		}
	}

	// get configmaps
	if o.WithConfigMap == Need {
		if objs.ConfigMaps, err = client.ConfigMaps(o.Namespace).List(ctx, listOpts()); err != nil {
			return nil, err
		}
	}

	// get PVCs
	if o.WithPVC == Need {
		if objs.PVCs, err = client.PersistentVolumeClaims(o.Namespace).List(ctx, listOpts()); err != nil {
			return nil, err
		}
	}

	// get pods
	if o.WithPod == Need {
		if objs.Pods, err = client.Pods(o.Namespace).List(ctx, listOpts()); err != nil {
			return nil, err
		}
		var podList []corev1.Pod
		// filter back-up job pod
		for _, pod := range objs.Pods.Items {
			labels := pod.GetLabels()
			if labels[dptypes.BackupNameLabelKey] == "" {
				podList = append(podList, pod)
			}
		}
		objs.Pods.Items = podList
		// get nodes where the pods are located
	podLoop:
		for _, pod := range objs.Pods.Items {
			for _, node := range objs.Nodes {
				if node.Name == pod.Spec.NodeName {
					continue podLoop
				}
			}

			nodeName := pod.Spec.NodeName
			if len(nodeName) == 0 {
				continue
			}

			node, err := client.Nodes().Get(ctx, nodeName, metav1.GetOptions{})
			if err != nil && !apierrors.IsNotFound(err) {
				return nil, err
			}

			if node != nil {
				objs.Nodes = append(objs.Nodes, node)
			}
		}
	}

	// get events
	if o.WithEvent == Need {
		// get all events of cluster
		if objs.Events, err = client.Events(o.Namespace).Search(scheme.Scheme, objs.Cluster); err != nil {
			return nil, err
		}

		// get all events of pods
		for _, pod := range objs.Pods.Items {
			events, err := client.Events(o.Namespace).Search(scheme.Scheme, &pod)
			if err != nil {
				return nil, err
			}
			if objs.Events == nil {
				objs.Events = events
			} else {
				objs.Events.Items = append(objs.Events.Items, events.Items...)
			}
		}
	}

	if o.WithDataProtection == Need {
		dpListOpts := metav1.ListOptions{
			LabelSelector: fmt.Sprintf("%s=%s",
				constant.AppInstanceLabelKey, o.Name),
		}
		if err = listResources(o.Dynamic, types.BackupPolicyGVR(), o.Namespace, dpListOpts, &objs.BackupPolicies); err != nil {
			return nil, err
		}
		if err = listResources(o.Dynamic, types.BackupScheduleGVR(), o.Namespace, dpListOpts, &objs.BackupSchedules); err != nil {
			return nil, err
		}
		var backups []dpv1alpha1.Backup
		if err = listResources(o.Dynamic, types.BackupGVR(), o.Namespace, dpListOpts, &backups); err != nil {
			return nil, err
		}
		// filter backups with cluster uid for excluding same cluster name
		for _, v := range backups {
			sourceClusterUID := v.Labels[dptypes.ClusterUIDLabelKey]
			if sourceClusterUID == "" || sourceClusterUID == string(objs.Cluster.UID) {
				objs.Backups = append(objs.Backups, v)
			}
		}
	}

	if o.WithCompDef == Need || o.WithCompDef == Maybe {
		compDefs := []*appsv1alpha1.ComponentDefinition{}
		if err = listResources(o.Dynamic, types.CompDefGVR(), "", metav1.ListOptions{}, &compDefs); err != nil && o.WithCompDef == Need {
			return nil, err
		}
		for _, compSpec := range objs.Cluster.Spec.ComponentSpecs {
			for _, comp := range compDefs {
				if compSpec.ComponentDef == comp.Name {
					objs.CompDefs = append(objs.CompDefs, comp)
					break
				}
			}
		}
	}

	if o.WithComp == Maybe || o.WithComp == Need {
		if err = listResources(o.Dynamic, types.ComponentGVR(), o.Namespace, listOpts(), &objs.Components); err != nil && o.WithComp == Need {
			return objs, err
		}
	}
	return objs, nil
}

func (o *ClusterObjects) GetClusterInfo() *ClusterInfo {
	c := o.Cluster
	cluster := &ClusterInfo{
		Name:              c.Name,
		Namespace:         c.Namespace,
		ClusterVersion:    c.Spec.ClusterVersionRef,
		ClusterDefinition: c.Spec.ClusterDefRef,
		TerminationPolicy: string(c.Spec.TerminationPolicy),
		Status:            string(c.Status.Phase),
		CreatedTime:       util.TimeFormat(&c.CreationTimestamp),
		InternalEP:        types.None,
		ExternalEP:        types.None,
		Labels:            util.CombineLabels(c.Labels),
	}

	if o.ClusterDef == nil {
		return cluster
	}

	primaryComponent := FindClusterComp(o.Cluster, o.ClusterDef.Spec.ComponentDefs[0].Name)
	internalEndpoints, externalEndpoints := GetComponentEndpoints(o.Services, primaryComponent)
	if len(internalEndpoints) > 0 {
		cluster.InternalEP = strings.Join(internalEndpoints, ",")
	}
	if len(externalEndpoints) > 0 {
		cluster.ExternalEP = strings.Join(externalEndpoints, ",")
	}
	return cluster
}

func (o *ClusterObjects) GetComponentInfo() []*ComponentInfo {
	var comps []*ComponentInfo
	for _, c := range o.Cluster.Spec.ComponentSpecs {
		// get all pods belonging to current component
		var pods []corev1.Pod
		for _, p := range o.Pods.Items {
			if n, ok := p.Labels[constant.KBAppComponentLabelKey]; ok && n == c.Name {
				pods = append(pods, p)
			}
		}

		// current component has no derived pods
		if len(pods) == 0 {
			continue
		}

		image := types.None
		if len(pods) > 0 {
			image = pods[0].Spec.Containers[0].Image
		}

		running, waiting, succeeded, failed := util.GetPodStatus(pods)
		comp := &ComponentInfo{
			Name:      c.Name,
			NameSpace: o.Cluster.Namespace,
			Type:      c.ComponentDefRef,
			Cluster:   o.Cluster.Name,
			Replicas:  fmt.Sprintf("%d / %d", c.Replicas, len(pods)),
			Status:    fmt.Sprintf("%d / %d / %d / %d ", running, waiting, succeeded, failed),
			Image:     image,
		}
		comp.CPU, comp.Memory = getResourceInfo(c.Resources.Requests, c.Resources.Limits)
		comp.Storage = o.getStorageInfo(&c)
		comps = append(comps, comp)
	}
	return comps
}

func (o *ClusterObjects) GetInstanceInfo() []*InstanceInfo {
	var instances []*InstanceInfo
	for _, pod := range o.Pods.Items {
		instance := &InstanceInfo{
			Name:        pod.Name,
			Namespace:   pod.Namespace,
			Cluster:     getLabelVal(pod.Labels, constant.AppInstanceLabelKey),
			Component:   getLabelVal(pod.Labels, constant.KBAppComponentLabelKey),
			Status:      o.getPodPhase(&pod),
			Role:        getLabelVal(pod.Labels, constant.RoleLabelKey),
			AccessMode:  getLabelVal(pod.Labels, constant.ConsensusSetAccessModeLabelKey),
			CreatedTime: util.TimeFormat(&pod.CreationTimestamp),
		}

		var component *appsv1alpha1.ClusterComponentSpec
		for i, c := range o.Cluster.Spec.ComponentSpecs {
			if c.Name == instance.Component {
				component = &o.Cluster.Spec.ComponentSpecs[i]
			}
		}
		instance.Storage = o.getStorageInfo(component)
		getInstanceNodeInfo(o.Nodes, &pod, instance)
		instance.CPU, instance.Memory = getResourceInfo(resource.PodRequestsAndLimits(&pod))
		instances = append(instances, instance)
	}
	return instances
}

// port from https://github.com/kubernetes/kubernetes/blob/master/pkg/printers/internalversion/printers.go#L860
func (o *ClusterObjects) getPodPhase(pod *corev1.Pod) string {
	reason := string(pod.Status.Phase)
	if pod.Status.Reason != "" {
		reason = pod.Status.Reason
	}

	// If the Pod carries {type:PodScheduled, reason:WaitingForGates}, set reason to 'SchedulingGated'.
	for _, condition := range pod.Status.Conditions {
		if condition.Type == corev1.PodScheduled && condition.Reason == corev1.PodReasonSchedulingGated {
			reason = corev1.PodReasonSchedulingGated
		}
	}
	hasPodReadyCondition := func(conditions []corev1.PodCondition) bool {
		for _, condition := range conditions {
			if condition.Type == corev1.PodReady && condition.Status == corev1.ConditionTrue {
				return true
			}
		}
		return false
	}
	initializing := false
	for i := range pod.Status.InitContainerStatuses {
		container := pod.Status.InitContainerStatuses[i]
		switch {
		case container.State.Terminated != nil && container.State.Terminated.ExitCode == 0:
			continue
		case container.State.Terminated != nil:
			// initialization is failed
			if len(container.State.Terminated.Reason) == 0 {
				if container.State.Terminated.Signal != 0 {
					reason = fmt.Sprintf("Init:Signal:%d", container.State.Terminated.Signal)
				} else {
					reason = fmt.Sprintf("Init:ExitCode:%d", container.State.Terminated.ExitCode)
				}
			} else {
				reason = "Init:" + container.State.Terminated.Reason
			}
			initializing = true
		case container.State.Waiting != nil && len(container.State.Waiting.Reason) > 0 && container.State.Waiting.Reason != "PodInitializing":
			reason = "Init:" + container.State.Waiting.Reason
			initializing = true
		default:
			reason = fmt.Sprintf("Init:%d/%d", i, len(pod.Spec.InitContainers))
			initializing = true
		}
		break
	}
	if !initializing {
		hasRunning := false
		for i := len(pod.Status.ContainerStatuses) - 1; i >= 0; i-- {
			container := pod.Status.ContainerStatuses[i]
			if container.State.Waiting != nil && container.State.Waiting.Reason != "" {
				reason = container.State.Waiting.Reason
			} else if container.State.Terminated != nil && container.State.Terminated.Reason != "" {
				reason = container.State.Terminated.Reason
			} else if container.State.Terminated != nil && container.State.Terminated.Reason == "" {
				if container.State.Terminated.Signal != 0 {
					reason = fmt.Sprintf("Signal:%d", container.State.Terminated.Signal)
				} else {
					reason = fmt.Sprintf("ExitCode:%d", container.State.Terminated.ExitCode)
				}
			} else if container.Ready && container.State.Running != nil {
				hasRunning = true
			}
		}

		// change pod status back to "Running" if there is at least one container still reporting as "Running" status
		if reason == "Completed" && hasRunning {
			if hasPodReadyCondition(pod.Status.Conditions) {
				reason = "Running"
			} else {
				reason = "NotReady"
			}
		}
	}

	if pod.DeletionTimestamp != nil && pod.Status.Reason == "NodeLost" {
		reason = "Unknown"
	} else if pod.DeletionTimestamp != nil {
		reason = "Terminating"
	}
	return reason
}

func (o *ClusterObjects) getStorageInfo(component *appsv1alpha1.ClusterComponentSpec) []StorageInfo {
	if component == nil {
		return nil
	}

	getClassName := func(vcTpl *appsv1alpha1.ClusterComponentVolumeClaimTemplate) string {
		if vcTpl.Spec.StorageClassName != nil {
			return *vcTpl.Spec.StorageClassName
		}

		if o.PVCs == nil {
			return types.None
		}

		// get storage class name from PVC
		for _, pvc := range o.PVCs.Items {
			labels := pvc.Labels
			if len(labels) == 0 {
				continue
			}

			if labels[constant.KBAppComponentLabelKey] != component.Name {
				continue
			}

			if labels[constant.VolumeClaimTemplateNameLabelKey] != vcTpl.Name {
				continue
			}
			if pvc.Spec.StorageClassName != nil {
				return *pvc.Spec.StorageClassName
			} else {
				return types.None
			}
		}

		return types.None
	}

	var infos []StorageInfo
	for _, vcTpl := range component.VolumeClaimTemplates {
		s := StorageInfo{
			Name: vcTpl.Name,
		}
		val := vcTpl.Spec.Resources.Requests[corev1.ResourceStorage]
		s.StorageClass = getClassName(&vcTpl)
		s.Size = val.String()
		s.AccessMode = getAccessModes(vcTpl.Spec.AccessModes)
		infos = append(infos, s)
	}
	return infos
}

func getInstanceNodeInfo(nodes []*corev1.Node, pod *corev1.Pod, i *InstanceInfo) {
	i.Node, i.Region, i.AZ = types.None, types.None, types.None
	if pod.Spec.NodeName == "" {
		return
	}

	i.Node = strings.Join([]string{pod.Spec.NodeName, pod.Status.HostIP}, "/")
	node := util.GetNodeByName(nodes, pod.Spec.NodeName)
	if node == nil {
		return
	}

	i.Region = getLabelVal(node.Labels, constant.RegionLabelKey)
	i.AZ = getLabelVal(node.Labels, constant.ZoneLabelKey)
}

func getResourceInfo(reqs, limits corev1.ResourceList) (string, string) {
	var cpu, mem string
	names := []corev1.ResourceName{corev1.ResourceCPU, corev1.ResourceMemory}
	for _, name := range names {
		res := types.None
		limit, req := limits[name], reqs[name]

		// if request is empty and limit is not, set limit to request
		if util.ResourceIsEmpty(&req) && !util.ResourceIsEmpty(&limit) {
			req = limit
		}

		// if both limit and request are empty, only output none
		if !util.ResourceIsEmpty(&limit) || !util.ResourceIsEmpty(&req) {
			res = fmt.Sprintf("%s / %s", req.String(), limit.String())
		}

		switch name {
		case corev1.ResourceCPU:
			cpu = res
		case corev1.ResourceMemory:
			mem = res
		}
	}
	return cpu, mem
}

func getLabelVal(labels map[string]string, key string) string {
	val := labels[key]
	if len(val) == 0 {
		return types.None
	}
	return val
}

func getAccessModes(modes []corev1.PersistentVolumeAccessMode) string {
	modes = removeDuplicateAccessModes(modes)
	var modesStr []string
	if containsAccessMode(modes, corev1.ReadWriteOnce) {
		modesStr = append(modesStr, "RWO")
	}
	if containsAccessMode(modes, corev1.ReadOnlyMany) {
		modesStr = append(modesStr, "ROX")
	}
	if containsAccessMode(modes, corev1.ReadWriteMany) {
		modesStr = append(modesStr, "RWX")
	}
	return strings.Join(modesStr, ",")
}

func removeDuplicateAccessModes(modes []corev1.PersistentVolumeAccessMode) []corev1.PersistentVolumeAccessMode {
	var accessModes []corev1.PersistentVolumeAccessMode
	for _, m := range modes {
		if !containsAccessMode(accessModes, m) {
			accessModes = append(accessModes, m)
		}
	}
	return accessModes
}

func containsAccessMode(modes []corev1.PersistentVolumeAccessMode, mode corev1.PersistentVolumeAccessMode) bool {
	for _, m := range modes {
		if m == mode {
			return true
		}
	}
	return false
}
