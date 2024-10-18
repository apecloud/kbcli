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

	appsv1 "github.com/apecloud/kubeblocks/apis/apps/v1"
	"github.com/apecloud/kubeblocks/pkg/common"
	jsonpatch "github.com/evanphx/json-patch"
	"github.com/spf13/cobra"
	"github.com/stoewer/go-strcase"
	"golang.org/x/exp/slices"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	apiruntime "k8s.io/apimachinery/pkg/runtime"
	apitypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/json"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	"k8s.io/kube-openapi/pkg/validation/spec"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/kubectl/pkg/util/templates"
	"sigs.k8s.io/controller-runtime/pkg/client"

	opsv1alpha1 "github.com/apecloud/kubeblocks/apis/operations/v1alpha1"
	"github.com/apecloud/kubeblocks/pkg/constant"

	"github.com/apecloud/kbcli/pkg/action"
	"github.com/apecloud/kbcli/pkg/cluster"
	"github.com/apecloud/kbcli/pkg/printer"
	"github.com/apecloud/kbcli/pkg/types"
	"github.com/apecloud/kbcli/pkg/util"
	"github.com/apecloud/kbcli/pkg/util/flags"
	"github.com/apecloud/kbcli/pkg/util/prompt"
)

type OperationsOptions struct {
	action.CreateOptions  `json:"-"`
	HasComponentNamesFlag bool `json:"-"`
	// AutoApprove when set true, skip the double check.
	AutoApprove            bool     `json:"-"`
	ComponentNames         []string `json:"componentNames,omitempty"`
	OpsRequestName         string   `json:"opsRequestName"`
	InstanceTPLNames       []string `json:"instanceTPLNames,omitempty"`
	TTLSecondsAfterSucceed int      `json:"ttlSecondsAfterSucceed"`
	Force                  bool     `json:"force"`

	// OpsType operation type
	OpsType opsv1alpha1.OpsType `json:"type"`

	// OpsTypeLower lower OpsType
	OpsTypeLower string `json:"typeLower"`

	ComponentDefinitionName string `json:"componentDefinitionName"`
	ServiceVersion          string `json:"serviceVersion"`

	// VerticalScaling options
	CPU    string `json:"cpu"`
	Memory string `json:"memory"`

	// HorizontalScaling options
	Replicas                 string   `json:"replicas"`
	ScaleOut                 bool     `json:"scaleOut"`
	OfflineInstancesToOnline []string `json:"offlineInstancesToOnline,omitempty"`
	OnlineInstancesToOffline []string `json:"onlineInstancesToOffline,omitempty"`

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
	ExposeType    string                   `json:"-"`
	ExposeSubType string                   `json:"-"`
	ExposeEnabled string                   `json:"exposeEnabled,omitempty"`
	Services      []opsv1alpha1.OpsService `json:"services,omitempty"`

	// Switchover options
	Component           string                        `json:"component"`
	Instance            string                        `json:"instance"`
	BackupName          string                        `json:"-"`
	Inplace             bool                          `json:"-"`
	InstanceNames       []string                      `json:"-"`
	Nodes               []string                      `json:"-"`
	RebuildInstanceFrom []opsv1alpha1.RebuildInstance `json:"rebuildInstanceFrom,omitempty"`
	Env                 []string                      `json:"-"`
}

func newBaseOperationsOptions(f cmdutil.Factory, streams genericiooptions.IOStreams,
	opsType opsv1alpha1.OpsType, hasComponentNamesFlag bool) *OperationsOptions {
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
	o.AddCommonFlags(cmd)
	cmd.Flags().BoolVar(&o.Force, "force", false, " skip the pre-checks of the opsRequest to run the opsRequest forcibly")
	cmd.Flags().StringVar(&o.OpsRequestName, "name", "", "OpsRequest name. if not specified, it will be randomly generated")
	cmd.Flags().IntVar(&o.TTLSecondsAfterSucceed, "ttlSecondsAfterSucceed", 0, "Time to live after the OpsRequest succeed")
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

func (o *OperationsOptions) CompleteSwitchoverOps() error {
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
	return nil
}

func (o *OperationsOptions) validateUpgrade(cluster *appsv1.Cluster) error {
	if o.ComponentDefinitionName == "nil" && o.ServiceVersion == "nil" {
		return fmt.Errorf("missing component-def or service-version")
	}
	validateCompSpec := func(comSpec appsv1.ClusterComponentSpec, compName string) error {
		if (o.ComponentDefinitionName == "nil" || o.ComponentDefinitionName == comSpec.ComponentDef) &&
			(o.ServiceVersion == "nil" || o.ServiceVersion == comSpec.ServiceVersion) {
			return fmt.Errorf(`no any changes of the componentDef and serviceVersion for component "%s"`, compName)
		}
		return nil
	}
	return o.handleComponentOps(cluster, validateCompSpec)
}

func (o *OperationsOptions) handleComponentOps(cluster *appsv1.Cluster, handleF func(compSpec appsv1.ClusterComponentSpec, compName string) error) error {
	for _, v := range cluster.Spec.ComponentSpecs {
		if !slices.Contains(o.ComponentNames, v.Name) {
			continue
		}
		if err := handleF(v, v.Name); err != nil {
			return err
		}
	}
	for _, v := range cluster.Spec.ShardingSpecs {
		if !slices.Contains(o.ComponentNames, v.Name) {
			continue
		}
		if err := handleF(v.Template, v.Name); err != nil {
			return err
		}
	}
	return nil
}

func (o *OperationsOptions) validateVolumeExpansion(clusterObj *appsv1.Cluster) error {
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
				cluster.ComponentNameLabelKey(clusterObj, cName), cName,
				constant.VolumeClaimTemplateNameLabelKey, vctName,
			)
			pvcs, err := o.Client.CoreV1().PersistentVolumeClaims(o.Namespace).List(context.Background(),
				metav1.ListOptions{LabelSelector: labels, Limit: 20})
			if err != nil {
				return err
			}
			var pvc *corev1.PersistentVolumeClaim
			for _, pvcItem := range pvcs.Items {
				if pvcItem.Labels[constant.KBAppComponentInstanceTemplateLabelKey] == "" {
					pvc = &pvcItem
					break
				}
			}
			if pvc == nil {
				return nil
			}
			specStorage := pvc.Spec.Resources.Requests.Storage()
			statusStorage := pvc.Status.Capacity.Storage()
			targetStorage, err := resource.ParseQuantity(o.Storage)
			if err != nil {
				return fmt.Errorf("cannot parse '%v', %v", o.Storage, err)
			}
			if targetStorage.Cmp(*statusStorage) < 0 {
				return fmt.Errorf(`requested storage size of volumeClaimTemplate "%s" can not less than status.capacity.storage "%s" `,
					vctName, statusStorage.String())
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

func (o *OperationsOptions) validateVScale(cluster *appsv1.Cluster) error {
	if o.CPU == "" && o.Memory == "" {
		return fmt.Errorf("cpu or memory must be specified")
	}

	fillResource := func(comp appsv1.ClusterComponentSpec, compName string) error {
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
		return nil
	}
	return o.handleComponentOps(cluster, fillResource)
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
	if err = o.validateComponents(cluster); err != nil {
		return err
	}
	switch o.OpsType {
	case opsv1alpha1.VolumeExpansionType:
		if err = o.validateVolumeExpansion(cluster); err != nil {
			return err
		}
	case opsv1alpha1.UpgradeType:
		if err = o.validateUpgrade(cluster); err != nil {
			return err
		}
	case opsv1alpha1.VerticalScalingType:
		if err = o.validateVScale(cluster); err != nil {
			return err
		}
	case opsv1alpha1.ExposeType:
		if err = o.validateExpose(); err != nil {
			return err
		}
	case opsv1alpha1.SwitchoverType:
		if err = o.validatePromote(cluster); err != nil {
			return err
		}
	case opsv1alpha1.HorizontalScalingType:
		if err = o.validateHScale(cluster); err != nil {
			return err
		}
	}
	if !o.AutoApprove && o.DryRun == "none" {
		return prompt.Confirm([]string{o.Name}, o.In, "", "")
	}
	return nil
}

func (o *OperationsOptions) validateComponents(clusterObj *appsv1.Cluster) error {
	validateInstances := func(instances []appsv1.InstanceTemplate, componentName string) error {
		for _, v := range o.InstanceTPLNames {
			var exist bool
			for _, ins := range instances {
				if v == ins.Name {
					exist = true
					break
				}
			}
			if !exist {
				return fmt.Errorf(`can not found the instance template "%s" in the component "%s"`, v, componentName)
			}
		}
		return nil
	}
	for _, compName := range o.ComponentNames {
		compSpec := cluster.GetComponentSpec(clusterObj, compName)
		if compSpec == nil {
			return fmt.Errorf(`can not found the component "%s" in cluster "%s"`, compName, clusterObj.Name)
		}
		if err := validateInstances(compSpec.Instances, compName); err != nil {
			return err
		}
	}
	return nil
}

func (o *OperationsOptions) validateHScale(cluster *appsv1.Cluster) error {
	if o.ScaleOut {
		if o.Replicas == "" && len(o.OfflineInstancesToOnline) == 0 {
			return fmt.Errorf("at least one of --replicas or --online-instances is required")
		}
		return o.handleComponentOps(cluster, func(compSpec appsv1.ClusterComponentSpec, compName string) error {
			for _, podName := range o.OfflineInstancesToOnline {
				if !slices.Contains(compSpec.OfflineInstances, podName) {
					return fmt.Errorf(`pod "%s" not in the offlineInstances of the component "%s"`, podName, compName)
				}
			}
			return nil
		})
	}
	if o.Replicas == "" && len(o.OnlineInstancesToOffline) == 0 {
		return fmt.Errorf("at least one of --replicas or --offline-instances is required")
	}
	return nil
}

func (o *OperationsOptions) validatePromote(clusterObj *appsv1.Cluster) error {
	var (
		compDefObj = appsv1.ComponentDefinition{}
		podObj     = &corev1.Pod{}
	)

	if o.Component == "" && o.Instance == "" {
		return fmt.Errorf("at least one of --component or --instance is required")
	}
	// if the instance is not specified, do not need to check the validity of the instance
	if o.Instance != "" {
		// checks the validity of the instance whether it belongs to the current component and ensure it is not the primary or leader instance currently.
		podKey := client.ObjectKey{
			Namespace: clusterObj.Namespace,
			Name:      o.Instance,
		}
		if err := util.GetResourceObjectFromGVR(types.PodGVR(), podKey, o.Dynamic, podObj); err != nil || podObj == nil {
			return fmt.Errorf("instance %s not found, please check the validity of the instance using \"kbcli cluster list-instances\"", o.Instance)
		}
		if o.Component == "" {
			o.Component = cluster.GetPodComponentName(podObj)
		}
	}

	getAndValidatePod := func(targetRoles ...string) error {
		// if the instance is not specified, do not need to check the validity of the instance
		if o.Instance == "" {
			return nil
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
		if cluster.GetPodComponentName(podObj) != o.Component || podObj.Labels[constant.AppInstanceLabelKey] != clusterObj.Name {
			return fmt.Errorf("instanceName %s does not belong to the current component, please check the validity of the instance using \"kbcli cluster list-instances\"", o.Instance)
		}
		return nil
	}

	getTargetRole := func(roles []appsv1.ReplicaRole) (string, error) {
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
		Name:      cluster.GetComponentSpec(clusterObj, o.Component).ComponentDef,
	}
	if err := util.GetResourceObjectFromGVR(types.CompDefGVR(), compDefKey, o.Dynamic, &compDefObj); err != nil {
		return err
	}
	if compDefObj.Spec.LifecycleActions == nil || compDefObj.Spec.LifecycleActions.Switchover == nil {
		return fmt.Errorf(`this component "%s does not support switchover, you can define the switchover action in the componentDef "%s"`, o.Component, compDefKey.Name)
	}
	targetRole, err := getTargetRole(compDefObj.Spec.Roles)
	if err != nil {
		return err
	}
	if targetRole == "" {
		return fmt.Errorf("componentDefinition has no role is serviceable and writable, does not support switchover")
	}
	return getAndValidatePod(targetRole)
}

func (o *OperationsOptions) validateExpose() error {
	switch util.ExposeType(o.ExposeType) {
	case "", util.ExposeToIntranet, util.ExposeToInternet:
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

	var componentSpec *appsv1.ClusterComponentSpec
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

	svc := opsv1alpha1.OpsService{
		// currently, we use the expose type as service name
		Name:        string(exposeType),
		Annotations: annotations,
	}
	if exposeType == util.ExposeToIntranet {
		if o.ExposeSubType == "" {
			svc.ServiceType = corev1.ServiceTypeLoadBalancer
		} else {
			svc.ServiceType = corev1.ServiceType(o.ExposeSubType)
		}
	} else {
		svc.ServiceType = corev1.ServiceTypeLoadBalancer
	}

	roleSelector, err := util.GetDefaultRoleSelector(o.Dynamic, clusterObj, componentSpec.ComponentDef, componentSpec.ComponentDef)
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
	o := newBaseOperationsOptions(f, streams, opsv1alpha1.RestartType, true)
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
		# upgrade the component to the target version
		kbcli cluster upgrade mycluster --service-version=8.0.30 --components my-comp

		# upgrade the component with new component definition
		kbcli cluster upgrade mycluster --component-def=8.0.30 --components my-comp

		# upgrade the component with new component definition and specified service version
		kbcli cluster upgrade mycluster --component-def=8.0.30 --service-version=8.0.30  --components my-comp
`)

// NewUpgradeCmd creates an upgrade command
func NewUpgradeCmd(f cmdutil.Factory, streams genericiooptions.IOStreams) *cobra.Command {
	o := newBaseOperationsOptions(f, streams, opsv1alpha1.UpgradeType, true)
	compDefFlag := "component-def"
	serviceVersionFlag := "service-version"
	cmd := &cobra.Command{
		Use:               "upgrade NAME",
		Short:             "Upgrade the service version(only support to upgrade minor version).",
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
	cmd.Flags().StringVar(&o.ComponentDefinitionName, compDefFlag, "nil", "Referring to the ComponentDefinition")
	cmd.Flags().StringVar(&o.ServiceVersion, serviceVersionFlag, "nil", "Referring to the serviceVersion that is provided by ComponentDefinition and ComponentVersion")
	cmd.Flags().BoolVar(&o.AutoApprove, "auto-approve", false, "Skip interactive approval before upgrading the cluster")
	return cmd
}

var verticalScalingExample = templates.Examples(`
		# scale the computing resources of specified components, separate with commas for multiple components
		kbcli cluster vscale mycluster --components=mysql --cpu=500m --memory=500Mi

        # scale the computing resources of instance template, separate with commas for multiple components
		kbcli cluster vscale mycluster --components=mysql --cpu=500m --memory=500Mi --instance-tpl default
`)

// NewVerticalScalingCmd creates a vertical scaling command
func NewVerticalScalingCmd(f cmdutil.Factory, streams genericiooptions.IOStreams) *cobra.Command {
	o := newBaseOperationsOptions(f, streams, opsv1alpha1.VerticalScalingType, true)
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
	cmd.Flags().StringSliceVar(&o.InstanceTPLNames, "instance-tpl", nil, "vertically scaling the specified instance template in the specified component")
	util.CheckErr(flags.CompletedInstanceTemplatesFlag(cmd, f, "instance-tpl"))
	cmd.Flags().StringVar(&o.CPU, "cpu", "", "Request and limit size of component cpu")
	cmd.Flags().StringVar(&o.Memory, "memory", "", "Request and limit size of component memory")
	cmd.Flags().BoolVar(&o.AutoApprove, "auto-approve", false, "Skip interactive approval before vertically scaling the cluster")
	_ = cmd.MarkFlagRequired("components")
	return cmd
}

var scaleInExample = templates.Examples(`
		# scale in 2 replicas
		kbcli cluster scale-in mycluster --components=mysql --replicas=2

		# offline specified instances
		kbcli cluster scale-in mycluster --components=mysql --offline-instances pod1

        # scale in 2 replicas, one of them is specified by "--offline-instances".
		kbcli cluster scale-out mycluster --components=mysql --replicas=2 --offline-instances pod1
`)

// NewScaleInCmd creates a scale in command
func NewScaleInCmd(f cmdutil.Factory, streams genericiooptions.IOStreams) *cobra.Command {
	o := newBaseOperationsOptions(f, streams, opsv1alpha1.HorizontalScalingType, true)
	cmd := &cobra.Command{
		Use:               "scale-in Replicas",
		Short:             "scale in replicas of the specified components in the cluster.",
		Example:           scaleInExample,
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
	// TODO: supports to scale out replicas of the instance templates?
	o.addCommonFlags(cmd, f)
	cmd.Flags().StringVar(&o.Replicas, "replicas", "", "Replicas with the specified components")
	cmd.Flags().StringSliceVar(&o.OnlineInstancesToOffline, "offline-instances", nil, "offline the specified instances")
	cmd.Flags().BoolVar(&o.AutoApprove, "auto-approve", false, "Skip interactive approval before horizontally scaling the cluster")
	_ = cmd.MarkFlagRequired("components")
	return cmd
}

var scaleOutExample = templates.Examples(`
		# scale out 2 replicas
		kbcli cluster scale-out mycluster --components=mysql --replicas=2

		# to bring the offline instances specified in compSpec.offlineInstances online.
		kbcli cluster scale-out mycluster --components=mysql --online-instances pod1

        # scale out 2 replicas, one of which is an instance that has already been taken offline.
		kbcli cluster scale-out mycluster --components=mysql --replicas=2 --online-instances pod1
`)

func NewScaleOutCmd(f cmdutil.Factory, streams genericiooptions.IOStreams) *cobra.Command {
	o := newBaseOperationsOptions(f, streams, opsv1alpha1.HorizontalScalingType, true)
	o.ScaleOut = true
	cmd := &cobra.Command{
		Use:               "scale-out Replicas",
		Short:             "scale out replicas of the specified components in the cluster.",
		Example:           scaleOutExample,
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
	// TODO: supports to scale in replicas of the instance templates?
	o.addCommonFlags(cmd, f)
	cmd.Flags().StringVar(&o.Replicas, "replicas", "", "Replica changes with the specified components")
	cmd.Flags().BoolVar(&o.AutoApprove, "auto-approve", false, "Skip interactive approval before horizontally scaling the cluster")
	cmd.Flags().StringSliceVar(&o.OfflineInstancesToOnline, "online-instances", nil, "online the specified instances which have been offline")
	_ = cmd.MarkFlagRequired("components")
	return cmd
}

var volumeExpansionExample = templates.Examples(`
		# restart specifies the component, separate with commas for multiple components
		kbcli cluster volume-expand mycluster --components=mysql --volume-claim-templates=data --storage=10Gi
`)

// NewVolumeExpansionCmd creates a volume expanding command
func NewVolumeExpansionCmd(f cmdutil.Factory, streams genericiooptions.IOStreams) *cobra.Command {
	o := newBaseOperationsOptions(f, streams, opsv1alpha1.VolumeExpansionType, true)
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
	// TODO: supports to volume expand the vcts of the instance templates?
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
		# Expose a cluster to intranet
		kbcli cluster expose mycluster --type intranet --enable=true

		# Expose a cluster to public internet
		kbcli cluster expose mycluster --type internet --enable=true

		# Stop exposing a cluster
		kbcli cluster expose mycluster --type intranet --enable=false
	`)
)

// NewExposeCmd creates an expose command
func NewExposeCmd(f cmdutil.Factory, streams genericiooptions.IOStreams) *cobra.Command {
	o := newBaseOperationsOptions(f, streams, opsv1alpha1.ExposeType, true)
	cmd := &cobra.Command{
		Use:               "expose NAME --enable=[true|false] --type=[intranet|internet]",
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
	cmd.Flags().StringVar(&o.ExposeType, "type", "", "Expose type, currently supported types are 'intranet', 'internet'")
	cmd.Flags().StringVar(&o.ExposeSubType, "sub-type", "LoadBalancer", "Expose sub type, currently supported types are 'NodePort', 'LoadBalancer', only available if type is intranet")
	cmd.Flags().StringVar(&o.ExposeEnabled, "enable", "", "Enable or disable the expose, values can be true or false")
	cmd.Flags().BoolVar(&o.AutoApprove, "auto-approve", false, "Skip interactive approval before exposing the cluster")

	util.CheckErr(cmd.RegisterFlagCompletionFunc("type", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{string(util.ExposeToIntranet), string(util.ExposeToInternet)}, cobra.ShellCompDirectiveNoFileComp
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
	o := newBaseOperationsOptions(f, streams, opsv1alpha1.StopType, false)
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
	o := newBaseOperationsOptions(f, streams, opsv1alpha1.StartType, false)
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
	opsRequest := &opsv1alpha1.OpsRequest{}
	if err := util.GetK8SClientObject(o.Dynamic, opsRequest, o.GVR, o.Namespace, o.Name); err != nil {
		return err
	}
	notSupportedPhases := []opsv1alpha1.OpsPhase{opsv1alpha1.OpsFailedPhase, opsv1alpha1.OpsSucceedPhase, opsv1alpha1.OpsCancelledPhase}
	if slices.Contains(notSupportedPhases, opsRequest.Status.Phase) {
		return fmt.Errorf("can not cancel the opsRequest when phase is %s", opsRequest.Status.Phase)
	}
	if opsRequest.Status.Phase == opsv1alpha1.OpsCancellingPhase {
		return fmt.Errorf(`opsRequest "%s" is cancelling`, opsRequest.Name)
	}
	supportedType := []opsv1alpha1.OpsType{opsv1alpha1.HorizontalScalingType, opsv1alpha1.VerticalScalingType}
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
	o := newBaseOperationsOptions(f, streams, opsv1alpha1.SwitchoverType, false)
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
			cmdutil.CheckErr(o.CompleteSwitchoverOps())
			cmdutil.CheckErr(o.Validate())
			cmdutil.CheckErr(o.Run())
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
	OpsDefinitionName string                  `json:"opsDefinitionName"`
	Params            []opsv1alpha1.Parameter `json:"params"`
	SchemaProperties  *apiextensionsv1.JSONSchemaProps
}

func NewCustomOpsCmd(f cmdutil.Factory, streams genericiooptions.IOStreams) *cobra.Command {
	o := newBaseOperationsOptions(f, streams, opsv1alpha1.CustomType, false)
	cmd := &cobra.Command{
		Use:     "custom-ops OpsDef --cluster <clusterName> <your custom params>",
		Example: customOpsExample,
		Run: func(cmd *cobra.Command, args []string) {
			println(`execute "kbcli cluster custom-ops --help" to list the supported command`)
		},
	}
	cmds := buildCustomOpsCmds(o)
	if len(cmds) > 0 {
		cmd.AddCommand(cmds...)
	}
	return cmd
}

func buildCustomOpsExamples(t unstructured.Unstructured) string {
	opsDef := &opsv1alpha1.OpsDefinition{}
	_ = apiruntime.DefaultUnstructuredConverter.FromUnstructured(t.UnstructuredContent(), opsDef)
	parametersSchema := opsDef.Spec.ParametersSchema
	commandName := strcase.KebabCase(t.GetName())
	baseCommand := fmt.Sprintf(`
	# Create a %s ops 
	kbcli cluster custom-ops %s <clusterName> --component <componentName>`, commandName, commandName)
	if parametersSchema == nil {
		return templates.Examples(baseCommand)
	}

	for _, name := range parametersSchema.OpenAPIV3Schema.Required {
		if name == "component" {
			baseCommand += fmt.Sprintf(" --%s-fork=<%s>", name, name)
		} else {
			baseCommand += fmt.Sprintf(" --%s=<%s>", strcase.KebabCase(name), name)
		}
	}
	return templates.Examples(baseCommand)
}

func buildCustomOpsCmds(option *OperationsOptions) []*cobra.Command {
	dynamic, _ := option.Factory.DynamicClient()
	opsDefs, _ := dynamic.Resource(types.OpsDefinitionGVR()).List(context.TODO(), metav1.ListOptions{})
	if opsDefs == nil {
		return nil
	}
	var cmds []*cobra.Command
	for _, t := range opsDefs.Items {
		o := &CustomOperations{
			OperationsOptions: option,
			OpsDefinitionName: t.GetName(),
		}
		// set options to build cue struct
		o.CreateOptions.Options = o
		cmd := &cobra.Command{
			Use:               strcase.KebabCase(t.GetName()) + " <ClusterName>",
			Short:             fmt.Sprintf("Create a custom ops with opsDef %s", t.GetName()),
			Example:           buildCustomOpsExamples(t),
			ValidArgsFunction: util.ResourceNameCompletionFunc(option.Factory, types.ClusterGVR()),
			Run: func(cmd *cobra.Command, args []string) {
				o.Args = args
				cmdutil.BehaviorOnFatal(printer.FatalWithRedColor)
				cmdutil.CheckErr(o.Complete())
				cmdutil.CheckErr(o.validateAndCompleteComponentName())
				cmdutil.CheckErr(o.completeCustomSpec(cmd))
				cmdutil.CheckErr(o.Run())
			},
		}
		o.addCommonFlags(cmd, option.Factory)
		flags.AddComponentFlag(option.Factory, cmd, &o.Component, "Specify the component name of the cluster. if not specified, using the first component which referenced the defined componentDefinition.")
		cmd.Flags().BoolVar(&o.AutoApprove, "auto-approve", false, "Skip interactive approval before promote the instance")
		// build opsDef flags
		util.CheckErr(o.addOpsDefFlags(cmd, t))
		cmds = append(cmds, cmd)
	}
	return cmds
}

func (o *CustomOperations) addOpsDefFlags(cmd *cobra.Command, t unstructured.Unstructured) error {
	opsDef := &opsv1alpha1.OpsDefinition{}
	_ = apiruntime.DefaultUnstructuredConverter.FromUnstructured(t.UnstructuredContent(), opsDef)
	parametersSchema := opsDef.Spec.ParametersSchema
	if parametersSchema == nil {
		return nil
	}
	o.SchemaProperties = parametersSchema.OpenAPIV3Schema.DeepCopy()
	newProperties := map[string]apiextensionsv1.JSONSchemaProps{}
	for name := range parametersSchema.OpenAPIV3Schema.Properties {
		value := parametersSchema.OpenAPIV3Schema.Properties[name]
		if name == "component" {
			name = fmt.Sprintf("%s-fork", name)
		}
		newProperties[name] = value
	}
	parametersSchema.OpenAPIV3Schema.Properties = newProperties
	// Convert apiextensionsv1.JSONSchemaProps to spec.Schema
	schemaData, err := json.Marshal(parametersSchema.OpenAPIV3Schema)
	if err != nil {
		return err
	}
	schema := &spec.Schema{}
	if err = json.Unmarshal(schemaData, schema); err != nil {
		return err
	}
	return flags.BuildFlagsBySchema(cmd, schema)
}

func (o *CustomOperations) validateAndCompleteComponentName() error {
	if o.Name == "" {
		return makeMissingClusterNameErr()
	}
	clusterObj, err := cluster.GetClusterByName(o.Dynamic, o.Name, o.Namespace)
	if err != nil {
		return err
	}
	opsDef := &opsv1alpha1.OpsDefinition{}
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

func (o *CustomOperations) completeCustomSpec(cmd *cobra.Command) error {
	var (
		params   = make([]opsv1alpha1.Parameter, 0, 0)
		paramMap = map[string]string{}
	)
	// Construct config and credential map from flags
	if o.SchemaProperties != nil {
		fromFlags := flags.FlagsToValues(cmd.LocalNonPersistentFlags(), true)
		for name := range o.SchemaProperties.Properties {
			flagName := strcase.KebabCase(name)
			if name == "component" {
				flagName = fmt.Sprintf("%s-fork", flagName)
			}
			if val, ok := fromFlags[flagName]; ok {
				params = append(params, opsv1alpha1.Parameter{
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
		# rebuild instance by creating new instances and remove the specified instances after the new instances are ready.
		kbcli cluster rebuild-instance mycluster --instances pod1,pod2

	   # rebuild instance to a new node.
		kbcli cluster rebuild-instance mycluster --instances pod1 --node nodeName.

	   # rebuild instance with the same pod name.
		kbcli cluster rebuild-instance mycluster --instances pod1 --in-place

		# rebuild instance from backup and with the same pod name
		kbcli cluster rebuild-instance mycluster --instances pod1,pod2 --backupName <backup> --in-place
`)

// NewRebuildInstanceCmd creates a rebuildInstance command
func NewRebuildInstanceCmd(f cmdutil.Factory, streams genericiooptions.IOStreams) *cobra.Command {
	o := newBaseOperationsOptions(f, streams, opsv1alpha1.RebuildInstanceType, false)
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
			insCompName, ok := pod.Labels[constant.KBAppShardingNameLabelKey]
			if ok {
				if !o.Inplace {
					return fmt.Errorf("sharding cluster only supports to rebuild instance in place")
				}
			} else {
				insCompName = pod.Labels[constant.KBAppComponentLabelKey]
			}
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
		var instances []opsv1alpha1.Instance
		for _, insName := range o.InstanceNames {
			instances = append(instances, opsv1alpha1.Instance{
				Name:           insName,
				TargetNodeName: nodeMap[insName],
			})
		}
		o.RebuildInstanceFrom = []opsv1alpha1.RebuildInstance{
			{
				ComponentOps: opsv1alpha1.ComponentOps{
					ComponentName: compName,
				},
				Instances:  instances,
				InPlace:    o.Inplace,
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
	cmd.Flags().BoolVar(&o.Inplace, "in-place", false, "rebuild the instance with the same pod name. if not set, will create a new instance by horizontalScaling and remove the instance after the new instance is ready")
	cmd.Flags().StringVar(&o.BackupName, "backup", "", "instances will be rebuild by the specified backup.")
	cmd.Flags().StringSliceVar(&o.InstanceNames, "instances", nil, "instances which need to rebuild.")
	util.CheckErr(flags.CompletedInstanceFlag(cmd, f, "instances"))
	cmd.Flags().StringSliceVar(&o.Nodes, "node", nil, `specified the target node which rebuilds the instance on the node otherwise will rebuild on a random node. format: insName1=nodeName,insName2=nodeName`)
	cmd.Flags().StringArrayVar(&o.Env, "restore-env", []string{}, "provide the necessary env for the 'Restore' operation from the backup. format: key1=value, key2=value")
	return cmd
}
