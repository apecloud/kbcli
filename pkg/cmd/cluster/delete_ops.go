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

	opsv1alpha1 "github.com/apecloud/kubeblocks/apis/operations/v1alpha1"
	jsonpatch "github.com/evanphx/json-patch"
	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	apitypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/json"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/kubectl/pkg/util/templates"

	"github.com/apecloud/kbcli/pkg/action"
	"github.com/apecloud/kbcli/pkg/types"
	"github.com/apecloud/kbcli/pkg/util"
)

var (
	deleteOpsExample = templates.Examples(`
		# delete all ops belong the specified cluster 
		kbcli cluster delete-ops mycluster

		# delete the specified ops belong the specify cluster 
		kbcli cluster delete-ops --name=mysql-restart-82zxv
`)
)

func NewDeleteOpsCmd(f cmdutil.Factory, streams genericiooptions.IOStreams) *cobra.Command {
	o := action.NewDeleteOptions(f, streams, types.OpsGVR())
	o.PreDeleteHook = preDeleteOps
	cmd := &cobra.Command{
		Use:               "delete-ops",
		Short:             "Delete an OpsRequest.",
		Example:           deleteOpsExample,
		ValidArgsFunction: util.ResourceNameCompletionFunc(f, types.ClusterGVR()),
		Run: func(cmd *cobra.Command, args []string) {
			util.CheckErr(completeForDeleteOps(o, args))
			util.CheckErr(o.Run())
		},
	}
	cmd.Flags().StringSliceVar(&o.Names, "name", []string{}, "OpsRequest names")
	_ = cmd.RegisterFlagCompletionFunc("name", util.ResourceNameCompletionFunc(f, types.OpsGVR()))
	o.AddFlags(cmd)
	return cmd
}

func preDeleteOps(o *action.DeleteOptions, obj runtime.Object) error {
	unstructured := obj.(*unstructured.Unstructured)
	opsRequest := &opsv1alpha1.OpsRequest{}
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(unstructured.Object, opsRequest); err != nil {
		return err
	}
	if opsRequest.Status.Phase != opsv1alpha1.OpsRunningPhase {
		return nil
	}
	if !o.Force {
		return fmt.Errorf(`OpsRequest "%s" is Running, you can specify "--force" to delete it`, opsRequest.Name)
	}
	// remove the finalizers
	dynamic, err := o.Factory.DynamicClient()
	if err != nil {
		return err
	}
	oldOps := opsRequest.DeepCopy()
	opsRequest.Finalizers = []string{}
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
	_, err = dynamic.Resource(types.OpsGVR()).Namespace(opsRequest.Namespace).Patch(context.TODO(),
		opsRequest.Name, apitypes.MergePatchType, patchBytes, metav1.PatchOptions{})
	return err
}

// completeForDeleteOps completes cmd for delete OpsRequest, if resource name
// is not specified, construct a label selector based on the cluster name to
// delete all OpeRequests belonging to the cluster.
func completeForDeleteOps(o *action.DeleteOptions, args []string) error {
	// If resource name is not empty, delete these resources by name, do not need
	// to construct the label selector.
	if len(o.Names) > 0 {
		o.ConfirmedNames = o.Names
		return nil
	}

	if len(args) == 0 {
		return fmt.Errorf("missing cluster name")
	}

	if len(args) > 1 {
		return fmt.Errorf("only support to delete the OpsRequests of one cluster")
	}

	o.ConfirmedNames = args
	// If OpsRequest name is unset and cluster name is set, delete all OpsRequests belonging to the cluster
	o.LabelSelector = util.BuildLabelSelectorByNames(o.LabelSelector, args)
	return nil
}
