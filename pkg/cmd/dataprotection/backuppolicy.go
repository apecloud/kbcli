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

package dataprotection

import (
	"context"
	"fmt"
	"reflect"
	"strconv"

	dpv1alpha1 "github.com/apecloud/kubeblocks/apis/dataprotection/v1alpha1"
	"github.com/apecloud/kubeblocks/pkg/constant"
	dptypes "github.com/apecloud/kubeblocks/pkg/dataprotection/types"
	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	"k8s.io/client-go/dynamic"
	clientset "k8s.io/client-go/kubernetes"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/kubectl/pkg/util/templates"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/apecloud/kbcli/pkg/action"
	"github.com/apecloud/kbcli/pkg/printer"
	"github.com/apecloud/kbcli/pkg/types"
	"github.com/apecloud/kbcli/pkg/util"
)

var (
	listBackupPolicyExample = templates.Examples(`
		# list all backup policies
		kbcli dp list-backup-policy

		# using short cmd to list backup policy of the specified cluster
        kbcli dp list-bp mycluster
	`)

	describeBackupPolicyExample = templates.Examples(`
		# describe the default backup policy of the cluster
		kbcli dp describe-backup-policy cluster-name

		# describe the backup policy of the cluster with specified name
		kbcli dp describe-backup-policy cluster-name --name backup-policy-name
	`)

	editExample = templates.Examples(`
		# edit backup policy
		kbcli dp edit-backup-policy <backup-policy-name>
	`)
)

type EditBackupPolicyOptions struct {
	Namespace string
	Name      string
	Dynamic   dynamic.Interface
	Client    clientset.Interface
	Factory   cmdutil.Factory

	GVR schema.GroupVersionResource
	genericiooptions.IOStreams
	isTest bool
}

func (o *EditBackupPolicyOptions) Complete(args []string) error {
	var err error
	if len(args) == 0 {
		return fmt.Errorf("missing backupPolicy name")
	}
	if len(args) > 1 {
		return fmt.Errorf("only support to update one backupPolicy or quote cronExpression")
	}
	o.Name = args[0]
	if o.Namespace, _, err = o.Factory.ToRawKubeConfigLoader().Namespace(); err != nil {
		return err
	}
	if o.Dynamic, err = o.Factory.DynamicClient(); err != nil {
		return err
	}
	if o.Client, err = o.Factory.KubernetesClientSet(); err != nil {
		return err
	}
	return nil
}

func (o *EditBackupPolicyOptions) RunEditBackupPolicy() error {
	backupPolicy := &dpv1alpha1.BackupPolicy{}
	key := client.ObjectKey{
		Name:      o.Name,
		Namespace: o.Namespace,
	}
	err := util.GetResourceObjectFromGVR(types.BackupPolicyGVR(), key, o.Dynamic, &backupPolicy)
	if err != nil {
		return err
	}
	oldBackupPolicy := backupPolicy.DeepCopy()
	customEdit := action.NewCustomEditOptions(o.Factory, o.IOStreams, action.EditForPatched)
	if err := customEdit.Run(backupPolicy); err != nil {
		return err
	}
	return o.applyChanges(oldBackupPolicy, backupPolicy)
}

// applyChanges applies the changes of backupPolicy.
func (o *EditBackupPolicyOptions) applyChanges(oldBackupPolicy, backupPolicy *dpv1alpha1.BackupPolicy) error {
	// if no changes, return.
	if reflect.DeepEqual(oldBackupPolicy, backupPolicy) {
		fmt.Fprintln(o.Out, "updated (no change)")
		return nil
	}
	obj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(backupPolicy)
	if err != nil {
		return err
	}
	if _, err = o.Dynamic.Resource(types.BackupPolicyGVR()).Namespace(backupPolicy.Namespace).Update(context.TODO(),
		&unstructured.Unstructured{Object: obj}, metav1.UpdateOptions{}); err != nil {
		return err
	}
	fmt.Fprintln(o.Out, "updated")
	return nil
}

func newListBackupPolicyCmd(f cmdutil.Factory, streams genericclioptions.IOStreams) *cobra.Command {
	o := action.NewListOptions(f, streams, types.BackupPolicyGVR())
	clusterName := ""
	cmd := &cobra.Command{
		Use:               "list-backup-policy",
		Short:             "List backup policies",
		Aliases:           []string{"list-bp"},
		Example:           listBackupPolicyExample,
		ValidArgsFunction: util.ResourceNameCompletionFunc(f, types.BackupPolicyGVR()),
		Run: func(cmd *cobra.Command, args []string) {
			if clusterName != "" {
				o.LabelSelector = util.BuildLabelSelectorByNames(o.LabelSelector, []string{clusterName})
			}
			o.Names = args
			cmdutil.BehaviorOnFatal(printer.FatalWithRedColor)
			util.CheckErr(o.Complete())
			util.CheckErr(PrintBackupPolicyList(o))
		},
	}
	cmd.Flags().StringVar(&clusterName, "cluster", "", "The cluster name")
	o.AddFlags(cmd)

	return cmd
}

func newEditBackupPolicyCmd(f cmdutil.Factory, streams genericiooptions.IOStreams) *cobra.Command {
	o := EditBackupPolicyOptions{Factory: f, IOStreams: streams, GVR: types.BackupPolicyGVR()}
	cmd := &cobra.Command{
		Use:                   "edit-backup-policy",
		DisableFlagsInUseLine: true,
		Aliases:               []string{"edit-bp"},
		Short:                 "Edit backup policy",
		Example:               editExample,
		ValidArgsFunction:     util.ResourceNameCompletionFunc(f, types.BackupPolicyGVR()),
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.BehaviorOnFatal(printer.FatalWithRedColor)
			cmdutil.CheckErr(o.Complete(args))
			cmdutil.CheckErr(o.RunEditBackupPolicy())
		},
	}
	return cmd
}

func newDescribeBackupPolicyCmd(f cmdutil.Factory, streams genericclioptions.IOStreams) *cobra.Command {
	o := &DescribeDPOptions{
		IOStreams: streams,
		Factory:   f,
		Gvr:       types.BackupPolicyGVR(),
	}
	cmd := &cobra.Command{
		Use:               "describe-backup-policy",
		Short:             "Describe a backup policy",
		Aliases:           []string{"desc-backup-policy"},
		ValidArgsFunction: util.ResourceNameCompletionFunc(f, types.BackupPolicyGVR()),
		Example:           describeBackupPolicyExample,
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.BehaviorOnFatal(printer.FatalWithRedColor)
			util.CheckErr(o.Validate(args))
			util.CheckErr(o.Complete())
			util.CheckErr(DescribeBackupPolicies(o, args))
		},
	}
	return cmd
}

func DescribeBackupPolicies(o *DescribeDPOptions, bpNames []string) error {
	for _, bpName := range bpNames {
		obj := &dpv1alpha1.BackupPolicy{}
		if err := util.GetK8SClientObject(o.Dynamic, obj, o.Gvr, o.Namespace, bpName); err != nil {
			return err
		}
		PrintBackupPolicyDescribe(o, obj)
	}
	return nil
}

func PrintBackupPolicyDescribe(o *DescribeDPOptions, obj *dpv1alpha1.BackupPolicy) {

	printer.PrintLine("Summary:")
	realPrintPairStringToLine("Name", obj.Name)
	realPrintPairStringToLine("Cluster", obj.Labels[constant.AppInstanceLabelKey])
	realPrintPairStringToLine("Component", obj.Labels[constant.KBAppComponentLabelKey])
	realPrintPairStringToLine("Namespace", obj.Namespace)
	realPrintPairStringToLine("Default", strconv.FormatBool(obj.Annotations[dptypes.DefaultBackupPolicyAnnotationKey] == "true"))
	if obj.Spec.BackupRepoName != nil {
		realPrintPairStringToLine("Backup Repo Name", *obj.Spec.BackupRepoName)
	}

	printer.PrintLine("\nBackup Methods:")
	p := printer.NewTablePrinter(o.Out)
	p.SetHeader("Name", "ActionSet", "snapshot-volumes")
	for _, v := range obj.Spec.BackupMethods {
		p.AddRow(v.Name, v.ActionSetName, strconv.FormatBool(*v.SnapshotVolumes))
	}
	p.Print()
}

// PrintBackupPolicyList prints the backup policy list.
func PrintBackupPolicyList(o *action.ListOptions) error {
	headers := []any{"NAME", "NAMESPACE", "DEFAULT", "CLUSTER", "COMPONENT", "CREATE-TIME", "STATUS"}
	return o.PrintObjectList(headers, func(tbl *printer.TablePrinter, unstructuredObj unstructured.Unstructured) error {
		backupPolicy := &dpv1alpha1.BackupPolicy{}
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(unstructuredObj.Object, backupPolicy); err != nil {
			return err
		}
		defaultPolicy, ok := backupPolicy.GetAnnotations()[dptypes.DefaultBackupPolicyAnnotationKey]
		if !ok {
			defaultPolicy = "false"
		}
		createTime := backupPolicy.GetCreationTimestamp()
		tbl.AddRow(backupPolicy.GetName(), backupPolicy.GetNamespace(), defaultPolicy, backupPolicy.GetLabels()[constant.AppInstanceLabelKey],
			backupPolicy.GetLabels()[constant.KBAppComponentLabelKey], util.TimeFormat(&createTime), backupPolicy.Status.Phase)
		return nil
	})
}
