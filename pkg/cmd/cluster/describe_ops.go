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
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/exp/maps"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	"k8s.io/client-go/dynamic"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/kubectl/pkg/util/templates"

	opsv1alpha1 "github.com/apecloud/kubeblocks/apis/operations/v1alpha1"

	"github.com/apecloud/kbcli/pkg/printer"
	"github.com/apecloud/kbcli/pkg/types"
	"github.com/apecloud/kbcli/pkg/util"
)

var (
	describeOpsExample = templates.Examples(`
		# describe a specified OpsRequest
		kbcli cluster describe-ops mysql-restart-82zxv`)
)

type describeOpsOptions struct {
	factory   cmdutil.Factory
	client    clientset.Interface
	dynamic   dynamic.Interface
	namespace string

	// resource type and names
	gvr   schema.GroupVersionResource
	names []string

	genericiooptions.IOStreams
}

func newDescribeOpsOptions(f cmdutil.Factory, streams genericiooptions.IOStreams) *describeOpsOptions {
	return &describeOpsOptions{
		factory:   f,
		IOStreams: streams,
		gvr:       types.OpsGVR(),
	}
}

func NewDescribeOpsCmd(f cmdutil.Factory, streams genericiooptions.IOStreams) *cobra.Command {
	o := newDescribeOpsOptions(f, streams)
	cmd := &cobra.Command{
		Use:               "describe-ops",
		Short:             "Show details of a specific OpsRequest.",
		Aliases:           []string{"desc-ops"},
		Example:           describeOpsExample,
		ValidArgsFunction: util.ResourceNameCompletionFunc(f, types.OpsGVR()),
		Run: func(cmd *cobra.Command, args []string) {
			util.CheckErr(o.complete(args))
			util.CheckErr(o.run())
		},
	}
	return cmd
}

func (o *describeOpsOptions) complete(args []string) error {
	var err error

	if len(args) == 0 {
		return fmt.Errorf("OpsRequest name should be specified")
	}

	o.names = args

	if o.client, err = o.factory.KubernetesClientSet(); err != nil {
		return err
	}

	if o.dynamic, err = o.factory.DynamicClient(); err != nil {
		return err
	}

	if o.namespace, _, err = o.factory.ToRawKubeConfigLoader().Namespace(); err != nil {
		return err
	}
	return nil
}

func (o *describeOpsOptions) run() error {
	for _, name := range o.names {
		if err := o.describeOps(name); err != nil {
			return err
		}
	}
	return nil
}

// describeOps gets the OpsRequest by name and describes it.
func (o *describeOpsOptions) describeOps(name string) error {
	opsRequest := &opsv1alpha1.OpsRequest{}
	if err := util.GetK8SClientObject(o.dynamic, opsRequest, o.gvr, o.namespace, name); err != nil {
		return err
	}
	return o.printOpsRequest(opsRequest)
}

// printOpsRequest prints the information of OpsRequest for describing command.
func (o *describeOpsOptions) printOpsRequest(ops *opsv1alpha1.OpsRequest) error {
	printer.PrintLine("Spec:")
	printer.PrintLineWithTabSeparator(
		// first pair string
		printer.NewPair("  Name", ops.Name),
		printer.NewPair("NameSpace", ops.Namespace),
		printer.NewPair("Cluster", ops.Spec.GetClusterName()),
		printer.NewPair("Type", string(ops.Spec.Type)),
	)

	o.printOpsCommand(ops)

	// print the last configuration of the cluster.
	o.printLastConfiguration(ops.Status.LastConfiguration, ops.Spec.Type)

	// print the OpsRequest.status
	o.printOpsRequestStatus(&ops.Status)

	// print the OpsRequest.status.conditions
	printer.PrintConditions(ops.Status.Conditions, o.Out)

	// get all events about cluster
	events, err := o.client.CoreV1().Events(o.namespace).Search(scheme.Scheme, ops)
	if err != nil {
		return err
	}

	// print the warning events
	printer.PrintAllWarningEvents(events, o.Out)

	return nil
}

// printOpsCommand prints the kbcli command by OpsRequest.spec.
func (o *describeOpsOptions) printOpsCommand(opsRequest *opsv1alpha1.OpsRequest) {
	if opsRequest == nil {
		return
	}
	var commands []string
	switch opsRequest.Spec.Type {
	case opsv1alpha1.RestartType:
		commands = o.getRestartCommand(opsRequest.Spec)
	case opsv1alpha1.UpgradeType:
		commands = o.getUpgradeCommand(opsRequest.Spec)
	case opsv1alpha1.HorizontalScalingType:
		commands = o.getHorizontalScalingCommand(opsRequest.Spec)
	case opsv1alpha1.VerticalScalingType:
		commands = o.getVerticalScalingCommand(opsRequest.Spec)
	case opsv1alpha1.ReconfiguringType:
		commands = o.getReconfiguringCommand(opsRequest.Spec)
	case opsv1alpha1.StopType, opsv1alpha1.StartType:
		commands = []string{fmt.Sprintf("kbcli cluster %s %s", strings.ToLower(string(opsRequest.Spec.Type)), opsRequest.Spec.ClusterName)}
	}
	if len(commands) == 0 {
		printer.PrintLine("\nCommand: " + printer.NoneString)
		return
	}
	printer.PrintTitle("Command")
	for i := range commands {
		command := fmt.Sprintf("%s --namespace=%s", commands[i], opsRequest.Namespace)
		printer.PrintLine("  " + command)
	}
}

// getRestartCommand gets the command of the Restart OpsRequest.
func (o *describeOpsOptions) getRestartCommand(spec opsv1alpha1.OpsRequestSpec) []string {
	if len(spec.RestartList) == 0 {
		return nil
	}
	componentNames := make([]string, len(spec.RestartList))
	for i, v := range spec.RestartList {
		componentNames[i] = v.ComponentName
	}
	return []string{
		fmt.Sprintf("kbcli cluster restart %s --components=%s", spec.GetClusterName(),
			strings.Join(componentNames, ",")),
	}
}

// getUpgradeCommand gets the command of the Upgrade OpsRequest.
func (o *describeOpsOptions) getUpgradeCommand(spec opsv1alpha1.OpsRequestSpec) []string {
	var (
		commands       []string
		componentNames []string
		mergeCommand   bool
	)
	for _, v := range spec.Upgrade.Components {
		componentNames = append(componentNames, v.ComponentName)
		command := o.buildOpsBaseCommand("upgrade", spec)
		command += o.getStringPFlag("service-version", v.ServiceVersion)
		command += o.getStringPFlag("component-def", v.ComponentDefinitionName)
		commands = append(commands, command)
		if command != commands[0] {
			mergeCommand = false
		}
	}
	return o.buildComponentsCommand(commands, componentNames, mergeCommand)
}

func (o *describeOpsOptions) buildComponentsCommand(commands []string, componentNames []string, mergeCommand bool) []string {
	if len(commands) == 0 {
		return nil
	}
	if mergeCommand {
		return []string{fmt.Sprintf("%s --components %s", commands[0], strings.Join(componentNames, ","))}
	}
	for i := range commands {
		commands[i] += o.getStringFlag("components", componentNames[i])
	}
	return commands
}

func (o *describeOpsOptions) baseStringFlag(key string, value any, condition bool, covert func(value any) any) string {
	if condition {
		if covert != nil {
			value = covert(value)
		}
		return fmt.Sprintf(" --%s=%v", key, value)
	}
	return ""
}

// addResourceFlag adds resource flag for VerticalScaling OpsRequest.
func (o *describeOpsOptions) getStringPFlag(key string, value *string) string {
	return o.baseStringFlag(key, value, value != nil && *value != "", func(value any) any {
		return *value.(*string)
	})
}

func (o *describeOpsOptions) getStringFlag(key string, value string) string {
	return o.baseStringFlag(key, value, len(value) > 0, nil)
}

func (o *describeOpsOptions) getSliceFlag(key string, value []string) string {
	return o.baseStringFlag(key, strings.Join(value, ","), len(value) > 0, nil)
}

func (o *describeOpsOptions) getInt32PFlag(key string, value *int32) string {
	return o.baseStringFlag(key, value, value != nil, func(value any) any {
		return *value.(*int32)
	})
}

// addResourceFlag adds resource flag for VerticalScaling OpsRequest.
func (o *describeOpsOptions) getResourceFlag(key string, value *resource.Quantity) string {
	return o.baseStringFlag(key, value, !value.IsZero(), nil)
}

// getVerticalScalingCommand gets the command of the VerticalScaling OpsRequest
func (o *describeOpsOptions) getVerticalScalingCommand(spec opsv1alpha1.OpsRequestSpec) []string {
	var (
		commands       []string
		componentNames []string
		mergeCommand   bool
	)
	canBuildCommand := func(resource corev1.ResourceRequirements) bool {
		if resource.Requests.Cpu().Value() != resource.Limits.Cpu().Value() {
			return false
		}
		if resource.Requests.Memory().Value() != resource.Limits.Memory().Value() {
			return false
		}
		return !resource.Limits.Cpu().IsZero() || !resource.Limits.Memory().IsZero()
	}
	for _, v := range spec.VerticalScalingList {
		if !canBuildCommand(v.ResourceRequirements) {
			return nil
		}
		if len(v.Instances) > 0 {
			return nil
		}
		componentNames = append(componentNames, v.ComponentName)
		command := o.buildOpsBaseCommand("vscale", spec)
		command += o.getResourceFlag("cpu", v.ResourceRequirements.Limits.Cpu())
		command += o.getResourceFlag("memory", v.ResourceRequirements.Limits.Memory())
		commands = append(commands, command)
		if command != commands[0] {
			mergeCommand = false
		}
	}
	return o.buildComponentsCommand(commands, componentNames, mergeCommand)
}

func (o *describeOpsOptions) buildOpsBaseCommand(cmd string, spec opsv1alpha1.OpsRequestSpec) string {
	return fmt.Sprintf("kbcli cluster %s %s", cmd, spec.ClusterName)
}

// getHorizontalScalingCommand gets the command of the HorizontalScaling OpsRequest.
func (o *describeOpsOptions) getHorizontalScalingCommand(spec opsv1alpha1.OpsRequestSpec) []string {
	var (
		commands       []string
		componentNames []string
		mergeCommand   bool
	)
	for _, v := range spec.HorizontalScalingList {
		if v.ScaleOut != nil && v.ScaleIn != nil {
			return nil
		}
		componentNames = append(componentNames, v.ComponentName)
		command := o.buildOpsBaseCommand("scale-in", spec)
		if v.ScaleOut != nil {
			if len(v.ScaleOut.Instances) > 0 || len(v.ScaleOut.NewInstances) > 0 {
				return nil
			}
			command = o.buildOpsBaseCommand("scale-out", spec)
			command += o.getInt32PFlag("replicas", v.ScaleOut.ReplicaChanges)
			command += o.getSliceFlag("online-instances", v.ScaleOut.OfflineInstancesToOnline)
		} else if v.ScaleIn != nil {
			if len(v.ScaleIn.Instances) > 0 {
				return nil
			}
			command += o.getInt32PFlag("replicas", v.ScaleIn.ReplicaChanges)
			command += o.getSliceFlag("offline-instances", v.ScaleIn.OnlineInstancesToOffline)
		}
		commands = append(commands, command)
		if command != commands[0] {
			mergeCommand = false
		}
	}
	return o.buildComponentsCommand(commands, componentNames, mergeCommand)
}

// getReconfiguringCommand gets the command of the VolumeExpansion command.
func (o *describeOpsOptions) getReconfiguringCommand(spec opsv1alpha1.OpsRequestSpec) []string {
	if spec.Reconfigures != nil {
		return generateReconfiguringCommand(spec.GetClusterName(), &spec.Reconfigures[0], []string{spec.Reconfigures[0].ComponentName})
	}

	if len(spec.Reconfigures) == 0 {
		return nil
	}
	components := make([]string, len(spec.Reconfigures))
	for i, reconfigure := range spec.Reconfigures {
		components[i] = reconfigure.ComponentName
	}
	return generateReconfiguringCommand(spec.GetClusterName(), &spec.Reconfigures[0], components)
}

func generateReconfiguringCommand(clusterName string, updatedParams *opsv1alpha1.Reconfigure, components []string) []string {
	if len(updatedParams.Parameters) == 0 {
		return nil
	}

	commandArgs := make([]string, 0)
	commandArgs = append(commandArgs, "kbcli")
	commandArgs = append(commandArgs, "cluster")
	commandArgs = append(commandArgs, "configure")
	commandArgs = append(commandArgs, clusterName)
	commandArgs = append(commandArgs, fmt.Sprintf("--components=%s", strings.Join(components, ",")))

	for _, p := range updatedParams.Parameters {
		if p.Value == nil {
			continue
		}
		commandArgs = append(commandArgs, fmt.Sprintf("--set %s=%s", p.Key, *p.Value))
	}
	return []string{strings.Join(commandArgs, " ")}
}

// printOpsRequestStatus prints the OpsRequest status infos.
func (o *describeOpsOptions) printOpsRequestStatus(opsStatus *opsv1alpha1.OpsRequestStatus) {
	printer.PrintTitle("Status")
	startTime := opsStatus.StartTimestamp
	if !startTime.IsZero() {
		printer.PrintPairStringToLine("Start Time", util.TimeFormat(&startTime))
	}
	completeTime := opsStatus.CompletionTimestamp
	if !completeTime.IsZero() {
		printer.PrintPairStringToLine("Completion Time", util.TimeFormat(&completeTime))
	}
	if !startTime.IsZero() {
		printer.PrintPairStringToLine("Duration", util.GetHumanReadableDuration(startTime, completeTime))
	}
	printer.PrintPairStringToLine("Status", string(opsStatus.Phase))
	o.printProgressDetails(opsStatus)
}

// printLastConfiguration prints the last configuration of the cluster before doing the OpsRequest.
func (o *describeOpsOptions) printLastConfiguration(configuration opsv1alpha1.LastConfiguration, opsType opsv1alpha1.OpsType) {
	if reflect.DeepEqual(configuration, opsv1alpha1.LastConfiguration{}) {
		return
	}
	printer.PrintTitle("Last Configuration")
	switch opsType {
	case opsv1alpha1.UpgradeType:
		// printer.PrintPairStringToLine("Cluster Version", configuration.ClusterVersionRef)
	case opsv1alpha1.VerticalScalingType:
		handleVScale := func(tbl *printer.TablePrinter, cName string, compConf opsv1alpha1.LastComponentConfiguration) {
			tbl.AddRow(cName, compConf.Requests.Cpu().String(), compConf.Requests.Memory().String(), compConf.Limits.Cpu().String(), compConf.Limits.Memory().String())
		}
		headers := []interface{}{"COMPONENT", "REQUEST-CPU", "REQUEST-MEMORY", "LIMIT-CPU", "LIMIT-MEMORY"}
		o.printLastConfigurationByOpsType(configuration, headers, handleVScale)
	case opsv1alpha1.HorizontalScalingType:
		handleHScale := func(tbl *printer.TablePrinter, cName string, compConf opsv1alpha1.LastComponentConfiguration) {
			tbl.AddRow(cName, *compConf.Replicas)
		}
		headers := []interface{}{"COMPONENT", "REPLICAS"}
		o.printLastConfigurationByOpsType(configuration, headers, handleHScale)
	case opsv1alpha1.VolumeExpansionType:
		handleVolumeExpansion := func(tbl *printer.TablePrinter, cName string, compConf opsv1alpha1.LastComponentConfiguration) {
			vcts := compConf.VolumeClaimTemplates
			for _, v := range vcts {
				tbl.AddRow(cName, v.Name, v.Storage.String())
			}
		}
		headers := []interface{}{"COMPONENT", "VOLUME-CLAIM-TEMPLATE", "STORAGE"}
		o.printLastConfigurationByOpsType(configuration, headers, handleVolumeExpansion)
	}
}

// printLastConfigurationByOpsType prints the last configuration by ops type.
func (o *describeOpsOptions) printLastConfigurationByOpsType(configuration opsv1alpha1.LastConfiguration,
	headers []interface{},
	handleOpsObject func(tbl *printer.TablePrinter, cName string, compConf opsv1alpha1.LastComponentConfiguration),
) {
	tbl := printer.NewTablePrinter(o.Out)
	tbl.SetHeader(headers...)
	keys := maps.Keys(configuration.Components)
	sort.Strings(keys)
	for _, cName := range keys {
		handleOpsObject(tbl, cName, configuration.Components[cName])
	}
	tbl.Print()
}

// printProgressDetails prints the progressDetails of all components in this OpsRequest.
func (o *describeOpsOptions) printProgressDetails(opsStatus *opsv1alpha1.OpsRequestStatus) {
	printer.PrintPairStringToLine("Progress", opsStatus.Progress)
	keys := maps.Keys(opsStatus.Components)
	sort.Strings(keys)
	tbl := printer.NewTablePrinter(o.Out)
	tbl.SetHeader(fmt.Sprintf("%-22s%s", "", "OBJECT-KEY"), "STATUS", "DURATION", "MESSAGE")
	for _, cName := range keys {
		progressDetails := opsStatus.Components[cName].ProgressDetails
		for _, v := range progressDetails {
			var groupStr string
			if len(v.Group) > 0 {
				groupStr = fmt.Sprintf("(%s)", v.Group)
			}
			tbl.AddRow(fmt.Sprintf("%-22s%s%s", "", v.ObjectKey, groupStr),
				v.Status, util.GetHumanReadableDuration(v.StartTime, v.EndTime), v.Message)
		}
	}
	//  "-/-" is the progress default value.
	if opsStatus.Progress != "-/-" {
		tbl.Print()
	}
}
