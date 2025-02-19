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

	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/kubectl/pkg/util/templates"

	appsv1alpha1 "github.com/apecloud/kubeblocks/apis/apps/v1alpha1"

	"github.com/apecloud/kbcli/pkg/action"
	"github.com/apecloud/kbcli/pkg/types"
	"github.com/apecloud/kbcli/pkg/util"
)

var (
	deleteExample = templates.Examples(`
		# delete a cluster named mycluster
		kbcli cluster delete mycluster
		# delete a cluster by label selector
		kbcli cluster delete --selector clusterdefinition.kubeblocks.io/name=apecloud-mysql
`)
)

func NewDeleteCmd(f cmdutil.Factory, streams genericiooptions.IOStreams) *cobra.Command {
	o := action.NewDeleteOptions(f, streams, types.ClusterGVR())
	o.PreDeleteHook = clusterPreDeleteHook
	o.PostDeleteHook = clusterPostDeleteHook

	cmd := &cobra.Command{
		Use:               "delete NAME",
		Short:             "Delete clusters.",
		Example:           deleteExample,
		ValidArgsFunction: util.ResourceNameCompletionFunc(f, types.ClusterGVR()),
		Run: func(cmd *cobra.Command, args []string) {
			util.CheckErr(deleteCluster(o, args))
		},
	}
	o.AddFlags(cmd)
	return cmd
}

func deleteCluster(o *action.DeleteOptions, args []string) error {
	if len(args) == 0 && len(o.LabelSelector) == 0 {
		return fmt.Errorf("missing cluster name or a lable selector")
	}
	o.Names = args
	return o.Run()
}

func clusterPreDeleteHook(o *action.DeleteOptions, object runtime.Object) error {
	if object == nil {
		return nil
	}

	cluster, err := getClusterFromObject(object)
	if err != nil {
		return err
	}
	if cluster.Spec.TerminationPolicy == appsv1alpha1.DoNotTerminate {
		return fmt.Errorf("cluster %s is protected by termination policy %s, skip deleting", cluster.Name, appsv1alpha1.DoNotTerminate)
	}
	return nil
}

func clusterPostDeleteHook(o *action.DeleteOptions, object runtime.Object) error {
	if object == nil {
		return nil
	}

	// currently no hook is defined

	return nil
}

func getClusterFromObject(object runtime.Object) (*appsv1alpha1.Cluster, error) {
	if object.GetObjectKind().GroupVersionKind().Kind != appsv1alpha1.ClusterKind {
		return nil, fmt.Errorf("object %s is not of kind %s", object.GetObjectKind().GroupVersionKind().Kind, appsv1alpha1.ClusterKind)
	}
	u := object.(*unstructured.Unstructured)
	cluster := &appsv1alpha1.Cluster{}
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(u.Object, cluster); err != nil {
		return nil, err
	}
	return cluster, nil
}
