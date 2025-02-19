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
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/exp/slices"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/kubectl/pkg/util/templates"

	opsv1alpha1 "github.com/apecloud/kubeblocks/apis/operations/v1alpha1"

	"github.com/apecloud/kbcli/pkg/action"
	"github.com/apecloud/kbcli/pkg/printer"
	"github.com/apecloud/kbcli/pkg/types"
	"github.com/apecloud/kbcli/pkg/util"
)

var (
	listOpsExample = templates.Examples(`
		# list all opsRequests
		kbcli cluster list-ops

		# list all opsRequests of specified cluster
		kbcli cluster list-ops mycluster`)

	defaultDisplayPhase = []string{"pending", "creating", "running", "canceling", "failed"}
)

type opsListOptions struct {
	*action.ListOptions
	status         []string
	opsType        []string
	opsRequestName string
}

func NewListOpsCmd(f cmdutil.Factory, streams genericiooptions.IOStreams) *cobra.Command {
	o := &opsListOptions{
		ListOptions: action.NewListOptions(f, streams, types.OpsGVR()),
	}
	cmd := &cobra.Command{
		Use:               "list-ops",
		Short:             "List all opsRequests.",
		Aliases:           []string{"ls-ops"},
		Example:           listOpsExample,
		ValidArgsFunction: util.ResourceNameCompletionFunc(f, types.ClusterGVR()),
		Run: func(cmd *cobra.Command, args []string) {
			// build label selector for listing ops
			o.LabelSelector = util.BuildLabelSelectorByNames(o.LabelSelector, args)
			// args are the cluster names. we only use the label selector to get ops, so resources names
			// are not needed.
			o.Names = nil
			util.CheckErr(o.Complete())
			util.CheckErr(o.printOpsList())
		},
	}
	o.AddFlags(cmd)
	cmd.Flags().StringSliceVar(&o.opsType, "type", nil, "The OpsRequest type")
	cmd.Flags().StringSliceVar(&o.status, "status", defaultDisplayPhase, fmt.Sprintf("Options include all, %s. by default, outputs the %s OpsRequest.",
		strings.Join(defaultDisplayPhase, ", "), strings.Join(defaultDisplayPhase, "/")))
	cmd.Flags().StringVar(&o.opsRequestName, "name", "", "The OpsRequest name to get the details.")
	return cmd
}

func (o *opsListOptions) printOpsList() error {
	// if format is JSON or YAML, use default printer to output the result.
	if o.Format == printer.JSON || o.Format == printer.YAML {
		if o.opsRequestName != "" {
			o.Names = []string{o.opsRequestName}
		}
		_, err := o.Run()
		return err
	}

	dynamic, err := o.Factory.DynamicClient()
	if err != nil {
		return err
	}
	listOptions := metav1.ListOptions{
		LabelSelector: o.LabelSelector,
		FieldSelector: o.FieldSelector,
	}
	if o.AllNamespaces {
		o.Namespace = ""
	}
	opsList, err := dynamic.Resource(types.OpsGVR()).Namespace(o.Namespace).List(context.TODO(), listOptions)
	if err != nil {
		return err
	}
	if len(opsList.Items) == 0 {
		o.PrintNotFoundResources()
		return nil
	}
	// sort the unstructured objects with the creationTimestamp in positive order
	sort.Sort(action.UnstructuredList(opsList.Items))

	// check if specified with "all" keyword for status.
	isAllStatus := o.isAllStatus()
	tblPrinter := printer.NewTablePrinter(o.Out)
	tblPrinter.SetHeader("NAME", "NAMESPACE", "TYPE", "CLUSTER", "COMPONENT", "STATUS", "PROGRESS", "CREATED-TIME")
	for _, obj := range opsList.Items {
		ops := &opsv1alpha1.OpsRequest{}
		if err = runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, ops); err != nil {
			return err
		}
		phase := string(ops.Status.Phase)
		opsType := string(ops.Spec.Type)
		if len(o.opsRequestName) != 0 {
			if ops.Name == o.opsRequestName {
				tblPrinter.AddRow(ops.Name, ops.GetNamespace(), opsType, ops.Spec.GetClusterName(), getComponentNameFromOps(ops), phase, ops.Status.Progress, util.TimeFormat(&ops.CreationTimestamp))
			}
			continue
		}
		// if the OpsRequest phase is not expected, continue
		if !isAllStatus && !o.containsIgnoreCase(o.status, phase) {
			continue
		}

		if len(o.opsType) != 0 && !o.containsIgnoreCase(o.opsType, opsType) {
			continue
		}
		tblPrinter.AddRow(ops.Name, ops.GetNamespace(), opsType, ops.Spec.GetClusterName(), getComponentNameFromOps(ops), phase, ops.Status.Progress, util.TimeFormat(&ops.CreationTimestamp))
	}
	if tblPrinter.Tbl.Length() != 0 {
		tblPrinter.Print()
		return nil
	}
	message := "No opsRequests found"
	if len(o.opsRequestName) == 0 && !o.isAllStatus() {
		message += ", you can try as follows:\n\tkbcli cluster list-ops --status all"
	}
	printer.PrintLine(message)
	return nil
}

func getComponentNameFromOps(ops *opsv1alpha1.OpsRequest) string {
	components := make([]string, 0)
	opsSpec := ops.Spec
	switch opsSpec.Type {
	case opsv1alpha1.ReconfiguringType:
		if opsSpec.Reconfigures != nil {
			components = append(components, opsSpec.Reconfigures[0].ComponentName)
		}
		for _, item := range opsSpec.Reconfigures {
			components = append(components, item.ComponentName)
		}
	case opsv1alpha1.HorizontalScalingType:
		for _, item := range opsSpec.HorizontalScalingList {
			components = append(components, item.ComponentName)
		}
	case opsv1alpha1.VolumeExpansionType:
		for _, item := range opsSpec.VolumeExpansionList {
			components = append(components, item.ComponentName)
		}
	case opsv1alpha1.RestartType:
		for _, item := range opsSpec.RestartList {
			components = append(components, item.ComponentName)
		}
	case opsv1alpha1.VerticalScalingType:
		for _, item := range opsSpec.VerticalScalingList {
			components = append(components, item.ComponentName)
		}
	default:
		for k := range ops.Status.Components {
			components = append(components, k)
		}
		slices.Sort(components)
	}
	return strings.Join(components, ",")
}

func getTemplateNameFromOps(ops opsv1alpha1.OpsRequestSpec) string {
	if ops.Type != opsv1alpha1.ReconfiguringType {
		return ""
	}

	tpls := make([]string, 0)
	// TODO: support reconfigures
	for _, config := range ops.Reconfigures[0].Configurations {
		tpls = append(tpls, config.Name)
	}
	return strings.Join(tpls, ",")
}

func getKeyNameFromOps(ops opsv1alpha1.OpsRequestSpec) string {
	if ops.Type != opsv1alpha1.ReconfiguringType {
		return ""
	}

	keys := make([]string, 0)
	for _, config := range ops.Reconfigures[0].Configurations {
		for _, key := range config.Keys {
			keys = append(keys, key.Key)
		}
	}
	return strings.Join(keys, ",")
}

func (o *opsListOptions) containsIgnoreCase(s []string, e string) bool {
	for i := range s {
		if strings.EqualFold(s[i], e) {
			return true
		}
	}
	return false
}

// isAllStatus checks if the status flag contains "all" keyword.
func (o *opsListOptions) isAllStatus() bool {
	return slices.Contains(o.status, "all")
}
