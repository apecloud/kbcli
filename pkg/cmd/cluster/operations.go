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
	"errors"
	"fmt"
	"strings"

	"github.com/apecloud/kubeblocks/pkg/common"
	jsonpatch "github.com/evanphx/json-patch"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/stoewer/go-strcase"
	"golang.org/x/exp/slices"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	apitypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/json"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/kubectl/pkg/util/templates"
	"sigs.k8s.io/controller-runtime/pkg/client"

	appsv1alpha1 "github.com/apecloud/kubeblocks/apis/apps/v1alpha1"
	"github.com/apecloud/kubeblocks/pkg/constant"

	"github.com/apecloud/kbcli/pkg/action"
	"github.com/apecloud/kbcli/pkg/cluster"
	"github.com/apecloud/kbcli/pkg/printer"
	"github.com/apecloud/kbcli/pkg/types"
	"github.com/apecloud/kbcli/pkg/util"
	"github.com/apecloud/kbcli/pkg/util/flags"
	"github.com/apecloud/kbcli/pkg/util/prompt"
)

const oceanbase = "oceanbase"

type OperationsOptions struct {
	action.CreateOptions  `json:"-"`
	HasComponentNamesFlag bool `json:"-"`
	// AutoApprove when set true, skip the double check.
	AutoApprove            bool     `json:"-"`
	ComponentNames         []string `json:"componentNames,omitempty"`
	OpsRequestName         string   `json:"opsRequestName"`
	TTLSecondsAfterSucceed int      `json:"ttlSecondsAfterSucceed"`
	Force                  bool     `json:"force"`

	// OpsType operation type
	OpsType appsv1alpha1.OpsType `json:"type"`

	// OpsTypeLower lower OpsType
	OpsTypeLower string `json:"typeLower"`

	// Upgrade options
	ClusterVersionRef       string `json:"clusterVersionRef"`
	ComponentDefinitionName string `json:"componentDefinitionName"`
	ServiceVersion          string `json:"serviceVersion"`

	// VerticalScaling options
	CPU    string `json:"cpu"`
	Memory string `json:"memory"`

	// HorizontalScaling options
	Replicas int `json:"replicas"`

	// Reconfiguring options
	KeyValues       map[string]*string `json:"keyValues"`
	CfgTemplateName string             `json:"cfgTemplateName"`
	CfgFile         string             `json:"cfgFile"`
	ForceRestart    bool               `json:"forceRestart"`
	FileContent     string             `json:"fileContent"`
	HasPatch        bool               `json:"hasPatch"`

	// VolumeExpansion options.
	// VCTNames VolumeClaimTemplate names
	VCTNames []string `json:"vctNames,omitempty"`
	Storage  string   `json:"storage"`

	// Expose options
	ExposeType    string                    `json:"-"`
	ExposeSubType string                    `json:"-"`
	ExposeEnabled string                    `json:"exposeEnabled,omitempty"`
	Services      []appsv1alpha1.OpsService `json:"services,omitempty"`

	// Switchover options
	Component           string                         `json:"component"`
	Instance            string                         `json:"instance"`
	Primary             string                         `json:"-"`
	CharacterType       string                         `json:"-"`
	LorryHAEnabled      bool                           `json:"-"`
	ExecPod             *corev1.Pod                    `json:"-"`
	BackupName          string                         `json:"-"`
	InstanceNames       []string                       `json:"-"`
	Nodes               []string                       `json:"-"`
	RebuildInstanceFrom []appsv1alpha1.RebuildInstance `json:"rebuildInstanceFrom,omitempty"`
	Env                 []string                       `json:"-"`
}

func newBaseOperationsOptions(f cmdutil.Factory, streams genericiooptions.IOStreams,
	opsType appsv1alpha1.OpsType, hasComponentNamesFlag bool) *OperationsOptions {
	customOutPut := func(opt *action.CreateOptions) {
		output := fmt.Sprintf("OpsRequest %s created successfully, you can view the progress:", opt.Name)
		printer.PrintLine(output)
		nextLine := fmt.Sprintf("\tkbcli cluster describe-ops %s -n %s", opt.Name, opt.Namespace)
		printer.PrintLine(nextLine)
	}

	o := &OperationsOptions{
		// nil cannot be set to a map struct in CueLang, so init the map of KeyValues.
		KeyValues:             map[string]*string{},
		HasPatch:              true,
		OpsType:               opsType,
		HasComponentNamesFlag: hasComponentNamesFlag,
		AutoApprove:           false,
		CreateOptions: action.CreateOptions{
			Factory:         f,
			IOStreams:       streams,
			CueTemplateName: "cluster_operations_template.cue",
			GVR:             types.OpsGVR(),
			CustomOutPut:    customOutPut,
		},
	}

	o.OpsTypeLower = strings.ToLower(string(o.OpsType))
	o.CreateOptions.Options = o
	return o
}

// addCommonFlags adds common flags for operations command
func (o *OperationsOptions) addCommonFlags(cmd *cobra.Command, f cmdutil.Factory) {
	// add print flags
	printer.AddOutputFlagForCreate(cmd, &o.Format, false)
	cmd.Flags().BoolVar(&o.Force, "force", false, " skip the pre-checks of the opsRequest to run the opsRequest forcibly")
	cmd.Flags().StringVar(&o.OpsRequestName, "name", "", "OpsRequest name. if not specified, it will be randomly generated")
	cmd.Flags().IntVar(&o.TTLSecondsAfterSucceed, "ttlSecondsAfterSucceed", 0, "Time to live after the OpsRequest succeed")
	cmd.Flags().StringVar(&o.DryRun, "dry-run", "none", `Must be "client", or "server". If with client strategy, only print the object that would be sent, and no data is actually sent. If with server strategy, submit the server-side request, but no data is persistent.`)
	cmd.Flags().Lookup("dry-run").NoOptDefVal = "unchanged"
	if o.HasComponentNamesFlag {
		flags.AddComponentsFlag(f, cmd, &o.ComponentNames, "Component names to this operations")
	}
}

// CompleteRestartOps restarts all components of the cluster
// we should set all component names to ComponentNames flag.
func (o *OperationsOptions) CompleteRestartOps() error {
	if o.Name == "" {
		return makeMissingClusterNameErr()
	}
	if len(o.ComponentNames) != 0 {
		return nil
	}
	clusterObj, err := cluster.GetClusterByName(o.Dynamic, o.Name, o.Namespace)
	if err != nil {
		return err
	}
	componentSpecs := clusterObj.Spec.ComponentSpecs
	o.ComponentNames = make([]string, 0)
	for i := range componentSpecs {
		o.ComponentNames = append(o.ComponentNames, componentSpecs[i].Name)
	}
	for i := range clusterObj.Spec.ShardingSpecs {
		o.ComponentNames = append(o.ComponentNames, clusterObj.Spec.ShardingSpecs[i].Name)
	}
	return nil
}

// CompleteComponentsFlag when components flag is null and the cluster only has one component, auto complete it.
func (o *OperationsOptions) CompleteComponentsFlag() error {
	if o.Name == "" {
		return makeMissingClusterNameErr()
	}
	if len(o.ComponentNames) != 0 {
		return nil
	}
	clusterObj, err := cluster.GetClusterByName(o.Dynamic, o.Name, o.Namespace)
	if err != nil {
		return err
	}
	if len(clusterObj.Spec.ComponentSpecs) == 1 {
		o.ComponentNames = []string{clusterObj.Spec.ComponentSpecs[0].Name}
	}
	return nil
}

func (o *OperationsOptions) CompletePromoteOps() error {
	clusterObj, err := cluster.GetClusterByName(o.Dynamic, o.Name, o.Namespace)
	if err != nil {
		return err
	}

	if o.Component == "" {
		if len(clusterObj.Spec.ComponentSpecs) > 1 {
			return fmt.Errorf("there are multiple components in cluster, please use --component to specify the component for promote")
		}
		o.Component = clusterObj.Spec.ComponentSpecs[0].Name
	}
	o.CompleteHaEnabled()
	return o.CompleteCharacterType(clusterObj)
}

// CompleteCharacterType will get the cluster character type compatible with 0.7
// If both componentDefRef and componentDef are provided, the componentDef will take precedence over componentDefRef.
func (o *OperationsOptions) CompleteCharacterType(clusterObj *appsv1alpha1.Cluster) error {
	var primaryRoles []string
	var componentSpec appsv1alpha1.ClusterComponentSpec
	for _, compSpec := range clusterObj.Spec.ComponentSpecs {
		if compSpec.Name == o.Component {
			componentSpec = compSpec
			break
		}
	}

	if componentSpec.ComponentDef != "" {
		componentDefV2 := &appsv1alpha1.ComponentDefinition{}
		if err := util.GetK8SClientObject(o.Dynamic, componentDefV2, types.CompDefGVR(), "", componentSpec.ComponentDef); err != nil {
			return err
		}
		o.CharacterType = componentDefV2.Spec.ServiceKind

		primaryRole, _ := func(roles []appsv1alpha1.ReplicaRole) (string, error) {
			targetRole := ""
			if len(roles) == 0 {
				return targetRole, fmt.Errorf("component has no roles definition, does not support switchover")
			}
			for _, role := range roles {
				if role.Serviceable && role.Writable {
					if targetRole != "" {
						return targetRole, fmt.Errorf("componentDefinition has more than role is serviceable and writable, does not support switchover")
					}
					targetRole = role.Name
				}
			}
			return targetRole, nil
		}(componentDefV2.Spec.Roles)
		primaryRoles = append(primaryRoles, primaryRole)
	} else {
		clusterDefObj := appsv1alpha1.ClusterDefinition{}
		clusterDefKey := client.ObjectKey{
			Namespace: "",
			Name:      clusterObj.Spec.ClusterDefRef,
		}
		if err := util.GetResourceObjectFromGVR(types.ClusterDefGVR(), clusterDefKey, o.Dynamic, &clusterDefObj); err != nil {
			return err
		}
		compDefName := componentSpec.ComponentDefRef
		if compDefName == "" {
			compDefName = componentSpec.ComponentDef
		}
		componentDef := clusterDefObj.GetComponentDefByName(compDefName)
		if componentDef == nil {
			return fmt.Errorf("failed to get component def :%s", componentSpec.ComponentDefRef)
		}
		o.CharacterType = componentDef.CharacterType
		primaryRoles = []string{constant.Primary, constant.Leader}
	}

	if o.Instance != "" && o.CharacterType != oceanbase {
		pod, err := o.Client.CoreV1().Pods(o.Namespace).Get(context.Background(), o.Instance, metav1.GetOptions{})
		if err != nil {
			return err
		}
		o.ExecPod = pod
		return nil
	}

	if len(primaryRoles) == 0 {
		return nil
	}

	labelsMap := map[string]string{
		constant.AppInstanceLabelKey:    o.Name,
		constant.AppManagedByLabelKey:   "kubeblocks",
		constant.KBAppComponentLabelKey: o.Component,
	}
	selector := labels.SelectorFromSet(labelsMap)
	podList, err := o.Client.CoreV1().Pods(o.Namespace).List(context.Background(), metav1.ListOptions{LabelSelector: selector.String()})
	if err != nil {
		return err
	}
	for _, pod := range podList.Items {
		podRole, ok := pod.Labels[constant.RoleLabelKey]
		for _, role := range primaryRoles {
			if ok && podRole == role {
				o.ExecPod = pod.DeepCopy()
				o.Primary = pod.Name
				break
			}
		}
	}
	if o.ExecPod == nil {
		return fmt.Errorf("component %s has no primary", o.Component)
	}

	return nil
}

func (o *OperationsOptions) CompleteHaEnabled() {
	cmName := fmt.Sprintf("%s-%s-haconfig", o.Name, o.Component)

	cm, err := o.Client.CoreV1().ConfigMaps(o.Namespace).Get(context.Background(), cmName, metav1.GetOptions{})
	if err != nil {
		return
	}
	enable, ok := cm.Annotations["enable"]
	if ok && strings.EqualFold(enable, "true") {
		o.LorryHAEnabled = true
	}
}

func (o *OperationsOptions) validateUpgrade() error {
	if len(o.ClusterVersionRef) > 0 {
		return nil
	}
	if len(o.ComponentNames) > 0 {
		return nil
	}
	return fmt.Errorf("missing cluster-version or components")
}

func (o *OperationsOptions) validateVolumeExpansion() error {
	if len(o.VCTNames) == 0 {
		return fmt.Errorf("missing volume-claim-templates")
	}
	if len(o.Storage) == 0 {
		return fmt.Errorf("missing storage")
	}

	for _, cName := range o.ComponentNames {
		for _, vctName := range o.VCTNames {
			labels := fmt.Sprintf("%s=%s,%s=%s,%s=%s",
				constant.AppInstanceLabelKey, o.Name,
				constant.KBAppComponentLabelKey, cName,
				constant.VolumeClaimTemplateNameLabelKey, vctName,
			)
			pvcs, err := o.Client.CoreV1().PersistentVolumeClaims(o.Namespace).List(context.Background(),
				metav1.ListOptions{LabelSelector: labels, Limit: 1})
			if err != nil {
				return err
			}
			if len(pvcs.Items) == 0 {
				continue
			}
			pvc := pvcs.Items[0]
			specStorage := pvc.Spec.Resources.Requests.Storage()
			statusStorage := pvc.Status.Capacity.Storage()
			targetStorage, err := resource.ParseQuantity(o.Storage)
			if err != nil {
				return fmt.Errorf("cannot parse '%v', %v", o.Storage, err)
			}
			// determine whether the opsRequest is a recovery action for volume expansion failure
			if specStorage.Cmp(targetStorage) > 0 &&
				statusStorage.Cmp(targetStorage) <= 0 {
				o.AutoApprove = false
				fmt.Fprintln(o.Out, printer.BoldYellow("Warning: this opsRequest is a recovery action for volume expansion failure and will re-create the PersistentVolumeClaims when RECOVER_VOLUME_EXPANSION_FAILURE=false"))
				break
			}
		}
	}
	return nil
}

func (o *OperationsOptions) validateVScale(cluster *appsv1alpha1.Cluster) error {
	if o.CPU == "" && o.Memory == "" {
		return fmt.Errorf("cpu or memory must be specified")
	}

	fillResource := func(comp *appsv1alpha1.ClusterComponentSpec) error {
		requests := make(corev1.ResourceList)
		if o.CPU != "" {
			cpu, err := resource.ParseQuantity(o.CPU)
			if err != nil {
				return fmt.Errorf("cannot parse '%v', %v", o.CPU, err)
			}
			requests[corev1.ResourceCPU] = cpu
		}
		if o.Memory != "" {
			memory, err := resource.ParseQuantity(o.Memory)
			if err != nil {
				return fmt.Errorf("cannot parse '%v', %v", o.Memory, err)
			}
			requests[corev1.ResourceMemory] = memory
		}
		requests.DeepCopyInto(&comp.Resources.Requests)
		requests.DeepCopyInto(&comp.Resources.Limits)
		return nil
	}

	for _, name := range o.ComponentNames {
		for _, comp := range cluster.Spec.ComponentSpecs {
			if comp.Name != name {
				continue
			}
			if err := fillResource(&comp); err != nil {
				return err
			}
		}
	}

	return nil
}

// Validate command flags or args is legal
func (o *OperationsOptions) Validate() error {
	if o.Name == "" {
		return makeMissingClusterNameErr()
	}
	// check if cluster exist
	cluster, err := cluster.GetClusterByName(o.Dynamic, o.Name, o.Namespace)
	if err != nil {
		return err
	}

	// common validate for componentOps
	if o.HasComponentNamesFlag && len(o.ComponentNames) == 0 {
		return fmt.Errorf(`missing components, please specify the "--components" flag for the cluster`)
	}

	switch o.OpsType {
	case appsv1alpha1.VolumeExpansionType:
		if err = o.validateVolumeExpansion(); err != nil {
			return err
		}
	case appsv1alpha1.UpgradeType:
		if err = o.validateUpgrade(); err != nil {
			return err
		}
	case appsv1alpha1.VerticalScalingType:
		if err = o.validateVScale(cluster); err != nil {
			return err
		}
	case appsv1alpha1.ExposeType:
		if err = o.validateExpose(); err != nil {
			return err
		}
	case appsv1alpha1.SwitchoverType:
		if err = o.validatePromote(cluster); err != nil {
			return err
		}
	}
	if !o.AutoApprove && o.DryRun == "none" {
		return prompt.Confirm([]string{o.Name}, o.In, "", "")
	}
	return nil
}

func (o *OperationsOptions) validatePromote(cluster *appsv1alpha1.Cluster) error {
	var (
		clusterDefObj = appsv1alpha1.ClusterDefinition{}
		compDefObj    = appsv1alpha1.ComponentDefinition{}
		podObj        = &corev1.Pod{}
		componentName = o.Component
	)

	if len(cluster.Spec.ComponentSpecs) == 0 {
		return fmt.Errorf("cluster.Spec.ComponentSpecs cannot be empty")
	}

	getAndValidatePod := func(targetRoles ...string) error {
		// if the instance is not specified, do not need to check the validity of the instance
		if o.Instance == "" || o.CharacterType == oceanbase {
			return nil
		}
		// checks the validity of the instance whether it belongs to the current component and ensure it is not the primary or leader instance currently.
		podKey := client.ObjectKey{
			Namespace: cluster.Namespace,
			Name:      o.Instance,
		}
		if err := util.GetResourceObjectFromGVR(types.PodGVR(), podKey, o.Dynamic, podObj); err != nil || podObj == nil {
			return fmt.Errorf("instance %s not found, please check the validity of the instance using \"kbcli cluster list-instances\"", o.Instance)
		}
		v, ok := podObj.Labels[constant.RoleLabelKey]
		if !ok || v == "" {
			return fmt.Errorf("instance %s cannot be promoted because it had a invalid role label", o.Instance)
		}
		for _, targetRole := range targetRoles {
			if v == targetRole {
				return fmt.Errorf("instanceName %s cannot be promoted because it is already the targetRole %s instance", o.Instance, targetRole)
			}
		}
		if !strings.HasPrefix(podObj.Name, fmt.Sprintf("%s-%s", cluster.Name, componentName)) {
			return fmt.Errorf("instanceName %s does not belong to the current component, please check the validity of the instance using \"kbcli cluster list-instances\"", o.Instance)
		}
		return nil
	}

	// TODO(xingran): this will be removed in the future.
	validateBaseOnClusterCompDef := func() error {
		// check clusterDefinition exist
		clusterDefKey := client.ObjectKey{
			Namespace: "",
			Name:      cluster.Spec.ClusterDefRef,
		}
		if err := util.GetResourceObjectFromGVR(types.ClusterDefGVR(), clusterDefKey, o.Dynamic, &clusterDefObj); err != nil {
			return err
		}

		var clusterCompDefObj *appsv1alpha1.ClusterComponentDefinition
		for _, clusterCompDef := range clusterDefObj.Spec.ComponentDefs {
			if clusterCompDef.Name == cluster.Spec.GetComponentDefRefName(componentName) {
				clusterCompDefObj = &clusterCompDef
				break
			}
		}
		if clusterCompDefObj == nil {
			return fmt.Errorf("cluster component %s is invalid", componentName)
		}
		if !o.LorryHAEnabled && o.CharacterType != oceanbase {
			if clusterCompDefObj.SwitchoverSpec == nil {
				return fmt.Errorf("cluster component %s does not support switchover", componentName)
			}
			switch o.Instance {
			case "":
				if clusterCompDefObj.SwitchoverSpec.WithoutCandidate == nil {
					return fmt.Errorf("cluster component %s does not support promote without specifying an instance. Please specify a specific instance for the promotion", componentName)
				}
			default:
				if clusterCompDefObj.SwitchoverSpec.WithCandidate == nil {
					return fmt.Errorf("cluster component %s does not support specifying an instance for promote. If you want to perform a promote operation, please do not specify an instance", componentName)
				}
			}
		}
		targetRoles := []string{constant.Primary, constant.Leader}
		if err := getAndValidatePod(targetRoles...); err != nil {
			return err
		}
		return nil
	}

	validateBaseOnCompDef := func(compDef string) error {
		getTargetRole := func(roles []appsv1alpha1.ReplicaRole) (string, error) {
			targetRole := ""
			if len(roles) == 0 {
				return targetRole, fmt.Errorf("component has no roles definition, does not support switchover")
			}
			for _, role := range roles {
				if role.Serviceable && role.Writable {
					if targetRole != "" {
						return targetRole, fmt.Errorf("componentDefinition has more than role is serviceable and writable, does not support switchover")
					}
					targetRole = role.Name
				}
			}
			return targetRole, nil
		}

		// check componentDefinition exist
		compDefKey := client.ObjectKey{
			Namespace: "",
			Name:      compDef,
		}
		if err := util.GetResourceObjectFromGVR(types.CompDefGVR(), compDefKey, o.Dynamic, &compDefObj); err != nil {
			return err
		}
		if !o.LorryHAEnabled && o.CharacterType != oceanbase {
			if compDefObj.Spec.LifecycleActions == nil || compDefObj.Spec.LifecycleActions.Switchover == nil {
				return fmt.Errorf("this cluster component %s does not support switchover", componentName)
			}
			switch o.Instance {
			case "":
				if compDefObj.Spec.LifecycleActions.Switchover.WithoutCandidate == nil {
					return fmt.Errorf("this cluster component %s does not support promote without specifying an instance. Please specify a specific instance for the promotion", componentName)
				}
			default:
				if compDefObj.Spec.LifecycleActions.Switchover.WithCandidate == nil {
					return fmt.Errorf("this cluster component %s does not support specifying an instance for promote. If you want to perform a promote operation, please do not specify an instance", componentName)
				}
			}
		}
		targetRole, err := getTargetRole(compDefObj.Spec.Roles)
		if err != nil {
			return err
		}
		if targetRole == "" {
			return fmt.Errorf("componentDefinition has no role is serviceable and writable, does not support switchover")
		}
		if err := getAndValidatePod(targetRole); err != nil {
			return err
		}
		return nil
	}

	if cluster.Spec.ComponentSpecs[0].ComponentDef != "" {
		return validateBaseOnCompDef(cluster.Spec.ComponentSpecs[0].ComponentDef)
	} else {
		return validateBaseOnClusterCompDef()
	}
}

func (o *OperationsOptions) validateExpose() error {
	switch util.ExposeType(o.ExposeType) {
	case "", util.ExposeToVPC, util.ExposeToInternet:
	default:
		return fmt.Errorf("invalid expose type %q", o.ExposeType)
	}

	switch o.ExposeSubType {
	case util.LoadBalancer, util.NodePort:
	default:
		return fmt.Errorf("invalid expose subtype %q", o.ExposeSubType)
	}

	switch strings.ToLower(o.ExposeEnabled) {
	case util.EnableValue, util.DisableValue:
	default:
		return fmt.Errorf("invalid value for enable flag: %s", o.ExposeEnabled)
	}

	return nil
}

func (o *OperationsOptions) fillExpose() error {
	version, err := util.GetK8sVersion(o.Client.Discovery())
	if err != nil {
		return err
	}
	provider, err := util.GetK8sProvider(version, o.Client)
	if err != nil {
		return err
	}

	if len(o.ComponentNames) == 0 {
		return fmt.Errorf("there are multiple components in cluster, please use --components to specify the component for expose")
	}
	if len(o.ComponentNames) > 1 {
		return fmt.Errorf("only one component can be exposed at a time")
	}
	componentName := o.ComponentNames[0]

	// default expose to internet
	exposeType := util.ExposeType(o.ExposeType)
	if exposeType == "" {
		exposeType = util.ExposeToInternet
	}

	clusterObj, err := cluster.GetClusterByName(o.Dynamic, o.Name, o.Namespace)
	if err != nil {
		return err
	}

	var componentSpec *appsv1alpha1.ClusterComponentSpec
	for _, compSpec := range clusterObj.Spec.ComponentSpecs {
		if compSpec.Name == componentName {
			componentSpec = &compSpec
			break
		}
	}
	if componentSpec == nil {
		return fmt.Errorf("component %s not found", componentName)
	}

	annotations, err := util.GetExposeAnnotations(provider, exposeType)
	if err != nil {
		return err
	}

	svc := appsv1alpha1.OpsService{
		// currently, we use the expose type as service name
		Name:        string(exposeType),
		Annotations: annotations,
	}
	if exposeType == util.ExposeToVPC {
		if o.ExposeSubType == "" {
			svc.ServiceType = corev1.ServiceTypeLoadBalancer
		} else {
			svc.ServiceType = corev1.ServiceType(o.ExposeSubType)
		}
	} else {
		svc.ServiceType = corev1.ServiceTypeLoadBalancer
	}

	roleSelector, err := util.GetDefaultRoleSelector(o.Dynamic, clusterObj, componentSpec.ComponentDef, componentSpec.ComponentDefRef)
	if err != nil {
		return err
	}
	if len(roleSelector) > 0 {
		svc.RoleSelector = roleSelector
	}
	o.Services = append(o.Services, svc)
	return nil
}

var restartExample = templates.Examples(`
		# restart all components
		kbcli cluster restart mycluster

		# specified component to restart, separate with commas for multiple components
		kbcli cluster restart mycluster --components=mysql
`)

// NewRestartCmd creates a restart command
func NewRestartCmd(f cmdutil.Factory, streams genericiooptions.IOStreams) *cobra.Command {
	o := newBaseOperationsOptions(f, streams, appsv1alpha1.RestartType, true)
	cmd := &cobra.Command{
		Use:               "restart NAME",
		Short:             "Restart the specified components in the cluster.",
		Example:           restartExample,
		ValidArgsFunction: util.ResourceNameCompletionFunc(f, types.ClusterGVR()),
		Run: func(cmd *cobra.Command, args []string) {
			o.Args = args
			cmdutil.BehaviorOnFatal(printer.FatalWithRedColor)
			cmdutil.CheckErr(o.Complete())
			cmdutil.CheckErr(o.CompleteRestartOps())
			cmdutil.CheckErr(o.Validate())
			cmdutil.CheckErr(o.Run())
		},
	}
	o.addCommonFlags(cmd, f)
	cmd.Flags().BoolVar(&o.AutoApprove, "auto-approve", false, "Skip interactive approval before restarting the cluster")
	return cmd
}

var upgradeExample = templates.Examples(`
		# upgrade the cluster to the target version
		kbcli cluster upgrade mycluster --cluster-version=ac-mysql-8.0.30
`)

// NewUpgradeCmd creates an upgrade command
func NewUpgradeCmd(f cmdutil.Factory, streams genericiooptions.IOStreams) *cobra.Command {
	o := newBaseOperationsOptions(f, streams, appsv1alpha1.UpgradeType, false)
	compDefFlag := "component-definition"
	serviceVersionFlag := "service-version"
	cmd := &cobra.Command{
		Use:               "upgrade NAME",
		Short:             "Upgrade the cluster version.",
		Example:           upgradeExample,
		ValidArgsFunction: util.ResourceNameCompletionFunc(f, types.ClusterGVR()),
		Run: func(cmd *cobra.Command, args []string) {
			o.Args = args
			cmdutil.BehaviorOnFatal(printer.FatalWithRedColor)
			cmdutil.CheckErr(o.Complete())
			cmdutil.CheckErr(o.Validate())
			cmdutil.CheckErr(o.Run())
		},
	}
	o.addCommonFlags(cmd, f)
	cmd.Flags().StringVar(&o.ClusterVersionRef, "cluster-version", "", "Referring to the ClusterVersion CR(deprecated)")
	cmd.Flags().StringVar(&o.ComponentDefinitionName, compDefFlag, "nil", "Referring to the ComponentDefinition")
	cmd.Flags().StringVar(&o.ServiceVersion, serviceVersionFlag, "nil", "Referring to the serviceVersion that is provided by ComponentDefinition and ComponentVersion")
	cmd.Flags().BoolVar(&o.AutoApprove, "auto-approve", false, "Skip interactive approval before upgrading the cluster")
	flags.AddComponentsFlag(f, cmd, &o.ComponentNames, "Component names to this operations")
	return cmd
}

var verticalScalingExample = templates.Examples(`
		# scale the computing resources of specified components, separate with commas for multiple components
		kbcli cluster vscale mycluster --components=mysql --cpu=500m --memory=500Mi
`)

// NewVerticalScalingCmd creates a vertical scaling command
func NewVerticalScalingCmd(f cmdutil.Factory, streams genericiooptions.IOStreams) *cobra.Command {
	o := newBaseOperationsOptions(f, streams, appsv1alpha1.VerticalScalingType, true)
	cmd := &cobra.Command{
		Use:               "vscale NAME",
		Short:             "Vertically scale the specified components in the cluster.",
		Example:           verticalScalingExample,
		ValidArgsFunction: util.ResourceNameCompletionFunc(f, types.ClusterGVR()),
		Run: func(cmd *cobra.Command, args []string) {
			o.Args = args
			cmdutil.BehaviorOnFatal(printer.FatalWithRedColor)
			cmdutil.CheckErr(o.Complete())
			cmdutil.CheckErr(o.CompleteComponentsFlag())
			cmdutil.CheckErr(o.Validate())
			cmdutil.CheckErr(o.Run())
		},
	}
	o.addCommonFlags(cmd, f)
	cmd.Flags().StringVar(&o.CPU, "cpu", "", "Request and limit size of component cpu")
	cmd.Flags().StringVar(&o.Memory, "memory", "", "Request and limit size of component memory")
	cmd.Flags().BoolVar(&o.AutoApprove, "auto-approve", false, "Skip interactive approval before vertically scaling the cluster")
	_ = cmd.MarkFlagRequired("components")
	return cmd
}

var horizontalScalingExample = templates.Examples(`
		# expand storage resources of specified components, separate with commas for multiple components
		kbcli cluster hscale mycluster --components=mysql --replicas=3
`)

// NewHorizontalScalingCmd creates a horizontal scaling command
func NewHorizontalScalingCmd(f cmdutil.Factory, streams genericiooptions.IOStreams) *cobra.Command {
	o := newBaseOperationsOptions(f, streams, appsv1alpha1.HorizontalScalingType, true)
	cmd := &cobra.Command{
		Use:               "hscale NAME",
		Short:             "Horizontally scale the specified components in the cluster.",
		Example:           horizontalScalingExample,
		ValidArgsFunction: util.ResourceNameCompletionFunc(f, types.ClusterGVR()),
		Run: func(cmd *cobra.Command, args []string) {
			o.Args = args
			cmdutil.BehaviorOnFatal(printer.FatalWithRedColor)
			cmdutil.CheckErr(o.Complete())
			cmdutil.CheckErr(o.CompleteComponentsFlag())
			cmdutil.CheckErr(o.Validate())
			cmdutil.CheckErr(o.Run())
		},
	}

	o.addCommonFlags(cmd, f)
	cmd.Flags().IntVar(&o.Replicas, "replicas", 0, "Replicas with the specified components")
	cmd.Flags().BoolVar(&o.AutoApprove, "auto-approve", false, "Skip interactive approval before horizontally scaling the cluster")
	_ = cmd.MarkFlagRequired("replicas")
	_ = cmd.MarkFlagRequired("components")
	return cmd
}

var volumeExpansionExample = templates.Examples(`
		# restart specifies the component, separate with commas for multiple components
		kbcli cluster volume-expand mycluster --components=mysql --volume-claim-templates=data --storage=10Gi
`)

// NewVolumeExpansionCmd creates a volume expanding command
func NewVolumeExpansionCmd(f cmdutil.Factory, streams genericiooptions.IOStreams) *cobra.Command {
	o := newBaseOperationsOptions(f, streams, appsv1alpha1.VolumeExpansionType, true)
	cmd := &cobra.Command{
		Use:               "volume-expand NAME",
		Short:             "Expand volume with the specified components and volumeClaimTemplates in the cluster.",
		Example:           volumeExpansionExample,
		ValidArgsFunction: util.ResourceNameCompletionFunc(f, types.ClusterGVR()),
		Run: func(cmd *cobra.Command, args []string) {
			o.Args = args
			cmdutil.BehaviorOnFatal(printer.FatalWithRedColor)
			cmdutil.CheckErr(o.Complete())
			cmdutil.CheckErr(o.CompleteComponentsFlag())
			cmdutil.CheckErr(o.Validate())
			cmdutil.CheckErr(o.Run())
		},
	}
	o.addCommonFlags(cmd, f)
	cmd.Flags().StringSliceVarP(&o.VCTNames, "volume-claim-templates", "t", nil, "VolumeClaimTemplate names in components (required)")
	cmd.Flags().StringVar(&o.Storage, "storage", "", "Volume storage size (required)")
	cmd.Flags().BoolVar(&o.AutoApprove, "auto-approve", false, "Skip interactive approval before expanding the cluster volume")
	_ = cmd.MarkFlagRequired("volume-claim-templates")
	_ = cmd.MarkFlagRequired("storage")
	_ = cmd.MarkFlagRequired("components")
	return cmd
}

var (
	exposeExamples = templates.Examples(`
		# Expose a cluster to vpc
		kbcli cluster expose mycluster --type vpc --enable=true

		# Expose a cluster to public internet
		kbcli cluster expose mycluster --type internet --enable=true

		# Stop exposing a cluster
		kbcli cluster expose mycluster --type vpc --enable=false
	`)
)

// NewExposeCmd creates an expose command
func NewExposeCmd(f cmdutil.Factory, streams genericiooptions.IOStreams) *cobra.Command {
	o := newBaseOperationsOptions(f, streams, appsv1alpha1.ExposeType, true)
	cmd := &cobra.Command{
		Use:               "expose NAME --enable=[true|false] --type=[vpc|internet]",
		Short:             "Expose a cluster with a new endpoint, the new endpoint can be found by executing 'kbcli cluster describe NAME'.",
		Example:           exposeExamples,
		ValidArgsFunction: util.ResourceNameCompletionFunc(f, types.ClusterGVR()),
		Run: func(cmd *cobra.Command, args []string) {
			o.Args = args
			cmdutil.BehaviorOnFatal(printer.FatalWithRedColor)
			cmdutil.CheckErr(o.Complete())
			cmdutil.CheckErr(o.CompleteComponentsFlag())
			cmdutil.CheckErr(o.fillExpose())
			cmdutil.CheckErr(o.Validate())
			cmdutil.CheckErr(o.Run())
		},
	}
	o.addCommonFlags(cmd, f)
	cmd.Flags().StringVar(&o.ExposeType, "type", "", "Expose type, currently supported types are 'vpc', 'internet'")
	cmd.Flags().StringVar(&o.ExposeSubType, "sub-type", "LoadBalancer", "Expose sub type, currently supported types are 'NodePort', 'LoadBalancer', only available if type is vpc")
	cmd.Flags().StringVar(&o.ExposeEnabled, "enable", "", "Enable or disable the expose, values can be true or false")
	cmd.Flags().BoolVar(&o.AutoApprove, "auto-approve", false, "Skip interactive approval before exposing the cluster")

	util.CheckErr(cmd.RegisterFlagCompletionFunc("type", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{string(util.ExposeToVPC), string(util.ExposeToInternet)}, cobra.ShellCompDirectiveNoFileComp
	}))
	util.CheckErr(cmd.RegisterFlagCompletionFunc("sub-type", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{util.NodePort, util.LoadBalancer}, cobra.ShellCompDirectiveNoFileComp
	}))
	util.CheckErr(cmd.RegisterFlagCompletionFunc("enable", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"true", "false"}, cobra.ShellCompDirectiveNoFileComp
	}))

	_ = cmd.MarkFlagRequired("enable")
	return cmd
}

var stopExample = templates.Examples(`
		# stop the cluster and release all the pods of the cluster
		kbcli cluster stop mycluster
`)

// NewStopCmd creates a stop command
func NewStopCmd(f cmdutil.Factory, streams genericiooptions.IOStreams) *cobra.Command {
	o := newBaseOperationsOptions(f, streams, appsv1alpha1.StopType, false)
	cmd := &cobra.Command{
		Use:               "stop NAME",
		Short:             "Stop the cluster and release all the pods of the cluster.",
		Example:           stopExample,
		ValidArgsFunction: util.ResourceNameCompletionFunc(f, types.ClusterGVR()),
		Run: func(cmd *cobra.Command, args []string) {
			o.Args = args
			cmdutil.BehaviorOnFatal(printer.FatalWithRedColor)
			cmdutil.CheckErr(o.Complete())
			cmdutil.CheckErr(o.Validate())
			cmdutil.CheckErr(o.Run())
		},
	}
	o.addCommonFlags(cmd, f)
	cmd.Flags().BoolVar(&o.AutoApprove, "auto-approve", false, "Skip interactive approval before stopping the cluster")
	return cmd
}

var startExample = templates.Examples(`
		# start the cluster when cluster is stopped
		kbcli cluster start mycluster
`)

// NewStartCmd creates a start command
func NewStartCmd(f cmdutil.Factory, streams genericiooptions.IOStreams) *cobra.Command {
	o := newBaseOperationsOptions(f, streams, appsv1alpha1.StartType, false)
	o.AutoApprove = true
	cmd := &cobra.Command{
		Use:               "start NAME",
		Short:             "Start the cluster if cluster is stopped.",
		Example:           startExample,
		ValidArgsFunction: util.ResourceNameCompletionFunc(f, types.ClusterGVR()),
		Run: func(cmd *cobra.Command, args []string) {
			o.Args = args
			cmdutil.BehaviorOnFatal(printer.FatalWithRedColor)
			cmdutil.CheckErr(o.Complete())
			cmdutil.CheckErr(o.Validate())
			cmdutil.CheckErr(o.Run())
		},
	}
	o.addCommonFlags(cmd, f)
	return cmd
}

var cancelExample = templates.Examples(`
		# cancel the opsRequest which is not completed.
		kbcli cluster cancel-ops <opsRequestName>
`)

func cancelOps(o *OperationsOptions) error {
	opsRequest := &appsv1alpha1.OpsRequest{}
	if err := util.GetK8SClientObject(o.Dynamic, opsRequest, o.GVR, o.Namespace, o.Name); err != nil {
		return err
	}
	notSupportedPhases := []appsv1alpha1.OpsPhase{appsv1alpha1.OpsFailedPhase, appsv1alpha1.OpsSucceedPhase, appsv1alpha1.OpsCancelledPhase}
	if slices.Contains(notSupportedPhases, opsRequest.Status.Phase) {
		return fmt.Errorf("can not cancel the opsRequest when phase is %s", opsRequest.Status.Phase)
	}
	if opsRequest.Status.Phase == appsv1alpha1.OpsCancellingPhase {
		return fmt.Errorf(`opsRequest "%s" is cancelling`, opsRequest.Name)
	}
	supportedType := []appsv1alpha1.OpsType{appsv1alpha1.HorizontalScalingType, appsv1alpha1.VerticalScalingType}
	if !slices.Contains(supportedType, opsRequest.Spec.Type) {
		return fmt.Errorf("opsRequest type: %s not support cancel action", opsRequest.Spec.Type)
	}
	if !o.AutoApprove {
		if err := prompt.Confirm([]string{o.Name}, o.In, "", ""); err != nil {
			return err
		}
	}
	oldOps := opsRequest.DeepCopy()
	opsRequest.Spec.Cancel = true
	oldData, err := json.Marshal(oldOps)
	if err != nil {
		return err
	}
	newData, err := json.Marshal(opsRequest)
	if err != nil {
		return err
	}
	patchBytes, err := jsonpatch.CreateMergePatch(oldData, newData)
	if err != nil {
		return err
	}
	if _, err = o.Dynamic.Resource(types.OpsGVR()).Namespace(opsRequest.Namespace).Patch(context.TODO(),
		opsRequest.Name, apitypes.MergePatchType, patchBytes, metav1.PatchOptions{}); err != nil {
		return err
	}
	fmt.Fprintf(o.Out, "start to cancel opsRequest \"%s\", you can view the progress:\n\tkbcli cluster list-ops --name %s\n", o.Name, o.Name)
	return nil
}

func NewCancelCmd(f cmdutil.Factory, streams genericiooptions.IOStreams) *cobra.Command {
	o := newBaseOperationsOptions(f, streams, "", false)
	cmd := &cobra.Command{
		Use:               "cancel-ops NAME",
		Short:             "Cancel the pending/creating/running OpsRequest which type is vscale or hscale.",
		Example:           cancelExample,
		ValidArgsFunction: util.ResourceNameCompletionFunc(f, types.OpsGVR()),
		Run: func(cmd *cobra.Command, args []string) {
			o.Args = args
			cmdutil.BehaviorOnFatal(printer.FatalWithRedColor)
			cmdutil.CheckErr(o.Complete())
			cmdutil.CheckErr(cancelOps(o))
		},
	}
	cmd.Flags().BoolVar(&o.AutoApprove, "auto-approve", false, "Skip interactive approval before cancel the opsRequest")
	return cmd
}

var promoteExample = templates.Examples(`
		# Promote the instance mycluster-mysql-1 as the new primary or leader.
		kbcli cluster promote mycluster --instance mycluster-mysql-1

		# Promote a non-primary or non-leader instance as the new primary or leader, the new primary or leader is determined by the system.
		kbcli cluster promote mycluster

		# If the cluster has multiple components, you need to specify a component, otherwise an error will be reported.
	    kbcli cluster promote mycluster --component=mysql --instance mycluster-mysql-1
`)

// NewPromoteCmd creates a promote command
func NewPromoteCmd(f cmdutil.Factory, streams genericiooptions.IOStreams) *cobra.Command {
	o := newBaseOperationsOptions(f, streams, appsv1alpha1.SwitchoverType, false)
	cmd := &cobra.Command{
		Use:               "promote NAME [--component=<comp-name>] [--instance <instance-name>]",
		Short:             "Promote a non-primary or non-leader instance as the new primary or leader of the cluster",
		Example:           promoteExample,
		ValidArgsFunction: util.ResourceNameCompletionFunc(f, types.ClusterGVR()),
		Run: func(cmd *cobra.Command, args []string) {
			o.Args = args
			cmdutil.BehaviorOnFatal(printer.FatalWithRedColor)
			cmdutil.CheckErr(o.Complete())
			cmdutil.CheckErr(o.CompleteComponentsFlag())
			cmdutil.CheckErr(o.CompletePromoteOps())
			cmdutil.CheckErr(o.Validate())
			if (o.LorryHAEnabled || o.CharacterType == oceanbase) && o.ExecPod != nil {
				// lorryCli, err := lorryclient.NewK8sExecClientWithPod(nil, o.ExecPod)
				// cmdutil.CheckErr(err)
				// cmdutil.CheckErr(lorryCli.Switchover(context.Background(), o.Primary, o.Instance))
				customOpr := &CustomOperations{OperationsOptions: o}
				customOpr.OpsType = "Custom"
				customOpr.OpsTypeLower = strings.ToLower(string(o.OpsType))
				customOpr.OpsDefinitionName = "switchover"
				customOpr.Params = []appsv1alpha1.Parameter{
					{
						Name:  "primary",
						Value: o.Primary,
					},
					{
						Name:  "candidate",
						Value: o.Instance,
					},
				}
				customOpr.CreateOptions.Options = customOpr
				cmdutil.CheckErr(customOpr.Run())
			} else {
				cmdutil.CheckErr(o.Run())
			}
		},
	}
	flags.AddComponentFlag(f, cmd, &o.Component, "Specify the component name of the cluster, if the cluster has multiple components, you need to specify a component")
	cmd.Flags().StringVar(&o.Instance, "instance", "", "Specify the instance name as the new primary or leader of the cluster, you can get the instance name by running \"kbcli cluster list-instances\"")
	cmd.Flags().BoolVar(&o.AutoApprove, "auto-approve", false, "Skip interactive approval before promote the instance")
	o.addCommonFlags(cmd, f)
	return cmd
}

var customOpsExample = templates.Examples(`
        # custom ops cli format
        kbcli cluster custom-ops <opsDefName> --cluster <clusterName> <your params of this opsDef>

		# example for kafka topic
		kbcli cluster custom-ops kafka-topic --cluster mycluster --type create --topic test --partition 3 --replicas 3

		# example for kafka acl
		kbcli cluster custom-ops kafka-user-acl --cluster mycluster --type add --operations "Read,Writer,Delete,Alter,Describe" --allowUsers client --topic "*"

		# example for kafka quota
        kbcli cluster custom-ops kafka-quota --cluster mycluster --user client --producerByteRate 1024 --consumerByteRate 2048
`)

type CustomOperations struct {
	*OperationsOptions
	OpsDefinitionName string                   `json:"opsDefinitionName"`
	Params            []appsv1alpha1.Parameter `json:"params"`
	SchemaProperties  *apiextensionsv1.JSONSchemaProps
}

func NewCustomOpsCmd(f cmdutil.Factory, streams genericiooptions.IOStreams) *cobra.Command {
	o := &CustomOperations{
		OperationsOptions: newBaseOperationsOptions(f, streams, "Custom", false),
	}
	// set options to build cue struct
	o.CreateOptions.Options = o
	cmd := &cobra.Command{
		Use:                "custom-ops OpsDef --cluster <clusterName> <your custom params>",
		Example:            customOpsExample,
		ValidArgsFunction:  util.ResourceNameCompletionFunc(f, types.OpsDefinitionGVR()),
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			o.Args = args
			cmdutil.BehaviorOnFatal(printer.FatalWithRedColor)
			cmdutil.CheckErr(o.init())
			err := o.parseOpsDefinitionAndParams(cmd, args)
			if errors.Is(err, pflag.ErrHelp) {
				return err
			} else {
				util.CheckErr(err)
			}
			cmdutil.CheckErr(o.validateAndCompleteComponentName())
			cmdutil.CheckErr(o.completeCustomSpec(cmd))
			cmdutil.CheckErr(o.Run())
			return nil
		},
	}
	o.addCommonFlags(cmd, f)
	flags.AddComponentFlag(f, cmd, &o.Component, "Specify the component name of the cluster. if not specified, using the first component which referenced the defined componentDefinition.")
	cmd.Flags().BoolVar(&o.AutoApprove, "auto-approve", false, "Skip interactive approval before promote the instance")
	cmd.Flags().StringVar(&o.Name, "cluster", "", "Specify the cluster name")
	_ = cmd.MarkFlagRequired("cluster")
	return cmd
}

func (o *CustomOperations) init() error {
	var err error
	if o.Namespace == "" {
		if o.Namespace, _, err = o.Factory.ToRawKubeConfigLoader().Namespace(); err != nil {
			return err
		}
	}
	if o.Dynamic, err = o.Factory.DynamicClient(); err != nil {
		return err
	}
	if o.Client, err = o.Factory.KubernetesClientSet(); err != nil {
		return err
	}
	return nil
}

func (o *CustomOperations) validateAndCompleteComponentName() error {
	if o.Name == "" {
		return makeMissingClusterNameErr()
	}
	clusterObj, err := cluster.GetClusterByName(o.Dynamic, o.Name, o.Namespace)
	if err != nil {
		return err
	}
	opsDef := &appsv1alpha1.OpsDefinition{}
	if err = util.GetK8SClientObject(o.Dynamic, opsDef, types.OpsDefinitionGVR(), "", o.OpsDefinitionName); err != nil {
		return err
	}
	// check if the custom ops supports the component or complete the component for the cluster
	supportedComponentDefs := map[string]struct{}{}
	for _, v := range opsDef.Spec.ComponentInfos {
		supportedComponentDefs[v.ComponentDefinitionName] = struct{}{}
	}
	if len(supportedComponentDefs) > 0 {
		// check if the ops supports the input component
		var isSupported bool
		for _, v := range clusterObj.Spec.ComponentSpecs {
			if v.ComponentDef == "" {
				continue
			}
			if _, ok := supportedComponentDefs[v.ComponentDef]; ok {
				if o.Component == "" {
					o.Component = v.Name
					isSupported = true
					break
				} else if o.Component == v.Name {
					isSupported = true
					break
				}
			}
		}
		if !isSupported {
			return fmt.Errorf(`this custom ops "%s" not supports the component "%s"`, o.OpsDefinitionName, o.Component)
		}
	} else if o.Component == "" {
		return fmt.Errorf("component name can not be empty")
	}
	return nil
}

func (o *CustomOperations) parseOpsDefinitionAndParams(cmd *cobra.Command, args []string) error {
	fmt.Printf("args: %v\n", args)
	if len(args) == 0 {
		return fmt.Errorf("please specify the custom ops which you want to do")
	}
	_ = flags.NewTmpFlagSet()
	return flags.BuildFlagsWithOpenAPISchema(cmd, args, func() (*apiextensionsv1.JSONSchemaProps, error) {
		o.OpsDefinitionName = args[0]
		// Get ops Definition from API server
		opsDef := &appsv1alpha1.OpsDefinition{}
		if err := util.GetK8SClientObject(o.Dynamic, opsDef, types.OpsDefinitionGVR(), "", o.OpsDefinitionName); err != nil {
			if apierrors.IsNotFound(err) {
				return nil, fmt.Errorf("OpsDefintion \"%s\" is not found", o.OpsDefinitionName)
			}
			return nil, err
		}
		parametersSchema := opsDef.Spec.ParametersSchema
		if parametersSchema == nil {
			return nil, nil
		}
		o.SchemaProperties = parametersSchema.OpenAPIV3Schema
		return parametersSchema.OpenAPIV3Schema, nil
	})
}

func (o *CustomOperations) completeCustomSpec(cmd *cobra.Command) error {
	var (
		params   []appsv1alpha1.Parameter
		paramMap = map[string]string{}
	)
	// Construct config and credential map from flags
	if o.SchemaProperties != nil {
		fromFlags := flags.FlagsToValues(cmd.LocalNonPersistentFlags(), true)
		for name := range o.SchemaProperties.Properties {
			flagName := strcase.KebabCase(name)
			if val, ok := fromFlags[flagName]; ok {
				params = append(params, appsv1alpha1.Parameter{
					Name:  name,
					Value: val.String(),
				})
				paramMap[name] = val.String()
			}
		}
		// validate if flags values are legal.
		data, err := common.CoverStringToInterfaceBySchemaType(o.SchemaProperties, paramMap)
		if err != nil {
			return err
		}
		if err = common.ValidateDataWithSchema(o.SchemaProperties, data); err != nil {
			return err
		}
	}
	o.Params = params
	return nil
}

var rebuildExample = templates.Examples(`
		# rebuild instance without backup
		kbcli cluster rebuild-instance mycluster --instances pod1,pod2

		# rebuild instance from backup
		kbcli cluster rebuild-instance mycluster --instances pod1,pod2 --backupName <backup>
`)

// NewRebuildInstanceCmd creates a rebuildInstance command
func NewRebuildInstanceCmd(f cmdutil.Factory, streams genericiooptions.IOStreams) *cobra.Command {
	o := newBaseOperationsOptions(f, streams, appsv1alpha1.RebuildInstanceType, false)
	completedRebuildOps := func() error {
		if o.Name == "" {
			return makeMissingClusterNameErr()
		}
		if len(o.InstanceNames) == 0 {
			return fmt.Errorf("instances can not be empty")
		}
		_, err := cluster.GetClusterByName(o.Dynamic, o.Name, o.Namespace)
		if err != nil {
			return err
		}
		var compName string
		for _, podName := range o.InstanceNames {
			pod := &corev1.Pod{}
			if err = util.GetK8SClientObject(o.Dynamic, pod, types.PodGVR(), o.Namespace, podName); err != nil {
				return err
			}
			clusterName := pod.Labels[constant.AppInstanceLabelKey]
			if clusterName != o.Name {
				return fmt.Errorf(`the instance "%s" not belongs the cluster "%s"`, podName, o.Name)
			}
			insCompName := pod.Labels[constant.KBAppComponentLabelKey]
			if compName != "" && compName != insCompName {
				return fmt.Errorf("these instances do not belong to the same component")
			}
			compName = insCompName
		}
		// covert envs
		var envVars []corev1.EnvVar
		for _, v := range o.Env {
			for _, envVar := range strings.Split(v, ",") {
				kv := strings.Split(envVar, "=")
				if len(kv) != 2 {
					return fmt.Errorf("unknown format for env: %s", envVar)
				}
				envVars = append(envVars, corev1.EnvVar{
					Name:  kv[0],
					Value: kv[1],
				})
			}
		}
		// covert instances
		nodeMap := map[string]string{}
		for _, node := range o.Nodes {
			kv := strings.Split(node, "=")
			if len(kv) != 2 {
				return fmt.Errorf("unknown format for node: %s", node)
			}
			nodeMap[kv[0]] = kv[1]
		}
		var instances []appsv1alpha1.Instance
		for _, insName := range o.InstanceNames {
			instances = append(instances, appsv1alpha1.Instance{
				Name:           insName,
				TargetNodeName: nodeMap[insName],
			})
		}
		o.RebuildInstanceFrom = []appsv1alpha1.RebuildInstance{
			{
				ComponentOps: appsv1alpha1.ComponentOps{
					ComponentName: compName,
				},
				Instances:  instances,
				BackupName: o.BackupName,
				RestoreEnv: envVars,
			},
		}
		return nil
	}
	cmd := &cobra.Command{
		Use:               "rebuild-instance NAME",
		Short:             "Rebuild the specified instances in the cluster.",
		Example:           rebuildExample,
		ValidArgsFunction: util.ResourceNameCompletionFunc(f, types.ClusterGVR()),
		Run: func(cmd *cobra.Command, args []string) {
			o.Args = args
			cmdutil.BehaviorOnFatal(printer.FatalWithRedColor)
			cmdutil.CheckErr(o.Complete())
			cmdutil.CheckErr(completedRebuildOps())
			cmdutil.CheckErr(o.Validate())
			cmdutil.CheckErr(o.Run())
		},
	}
	o.addCommonFlags(cmd, f)
	cmd.Flags().BoolVar(&o.AutoApprove, "auto-approve", false, "Skip interactive approval before rebuilding the instances.gi")
	cmd.Flags().StringVar(&o.BackupName, "backup", "", "instances will be rebuild by the specified backup.")
	cmd.Flags().StringSliceVar(&o.InstanceNames, "instances", nil, "instance which need to rebuild.")
	cmd.Flags().StringSliceVar(&o.Nodes, "node", nil, "specified the target node which rebuilds the instance on the node otherwise will rebuild on a randon node. format: insName1=nodeName,insName2=nodeName")
	cmd.Flags().StringArrayVar(&o.Env, "env", []string{}, "provide the necessary env for the 'Restore' operation from the backup. format: key1=value, key2=value")
	return cmd
}
