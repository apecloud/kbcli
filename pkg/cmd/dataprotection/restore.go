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
	"fmt"
	"strconv"

	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/duration"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes/scheme"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/kubectl/pkg/util/templates"

	dpv1alpha1 "github.com/apecloud/kubeblocks/apis/dataprotection/v1alpha1"
	opsv1alpha1 "github.com/apecloud/kubeblocks/apis/operations/v1alpha1"
	"github.com/apecloud/kubeblocks/pkg/constant"

	"github.com/apecloud/kbcli/pkg/action"
	"github.com/apecloud/kbcli/pkg/cluster"
	"github.com/apecloud/kbcli/pkg/printer"
	"github.com/apecloud/kbcli/pkg/types"
	"github.com/apecloud/kbcli/pkg/util"
)

var (
	createRestoreExample = templates.Examples(`
		# restore a new cluster from a backup
		kbcli dp restore mybackup --cluster cluster-name`)

	describeRestoreExample = templates.Examples(`
		# describe a restore 
		kbcli dp describe-restore <restoreName>`)

	listRestoreExample = templates.Examples(`
		# list all restores
		kbcli dp list-restore`)
)

type CreateRestoreOptions struct {
	RestoreSpec    opsv1alpha1.Restore `json:"restoreSpec"`
	ClusterName    string              `json:"clusterName"`
	OpsType        string              `json:"opsType"`
	OpsRequestName string              `json:"opsRequestName"`
	Force          bool                `json:"force"`

	action.CreateOptions `json:"-"`
}

func (o *CreateRestoreOptions) Validate() error {
	if o.RestoreSpec.BackupName == "" {
		return fmt.Errorf("must be specified one of the --backup ")
	}
	backup, err := GetBackupByName(o.Dynamic, o.RestoreSpec.BackupName, o.Namespace)
	if backup == nil || err != nil {
		return fmt.Errorf("failed to find the backup, please confirm the specified name and namespace of backup. %s", err)
	}

	if o.Name == "" {
		name, err := cluster.GenerateClusterName(o.Dynamic, o.Namespace)
		if err != nil {
			return err
		}
		if name == "" {
			return fmt.Errorf("failed to generate a random cluster name")
		}
		o.Name = name
	}

	// set ops type, ops request name and clusterName
	o.OpsType = string(opsv1alpha1.RestoreType)
	o.ClusterName = o.Name
	o.OpsRequestName = o.Name

	return nil
}

func newRestoreCommand(f cmdutil.Factory, streams genericiooptions.IOStreams) *cobra.Command {
	restoreKey := ""
	restoreKeyIgnoreErrors := false

	customOutPut := func(opt *action.CreateOptions) {
		output := fmt.Sprintf("Cluster %s created", opt.Name)
		printer.PrintLine(output)
	}

	o := &CreateRestoreOptions{}
	o.CreateOptions = action.CreateOptions{
		IOStreams:       streams,
		Factory:         f,
		Options:         o,
		GVR:             types.OpsGVR(),
		CueTemplateName: "opsrequest_template.cue",
		CustomOutPut:    customOutPut,
	}

	clusterName := ""

	cmd := &cobra.Command{
		Use:               "restore",
		Short:             "Restore a new cluster from backup",
		ValidArgsFunction: util.ResourceNameCompletionFunc(f, types.BackupGVR()),
		Example:           createRestoreExample,
		Run: func(cmd *cobra.Command, args []string) {
			if len(args) > 0 {
				o.RestoreSpec.BackupName = args[0]
			}
			if clusterName != "" {
				o.Args = []string{clusterName}
			}
			if restoreKey != "" {
				o.RestoreSpec.Env = append(o.RestoreSpec.Env, corev1.EnvVar{
					Name:  DPEnvRestoreKeyPatterns,
					Value: restoreKey,
				})
			}
			if restoreKeyIgnoreErrors {
				o.RestoreSpec.Env = append(o.RestoreSpec.Env, corev1.EnvVar{
					Name:  DPEnvRestoreKeyIgnoreErrors,
					Value: strconv.FormatBool(restoreKeyIgnoreErrors),
				})
			}
			cmdutil.BehaviorOnFatal(printer.FatalWithRedColor)
			util.CheckErr(o.Complete())
			util.CheckErr(o.Validate())
			util.CheckErr(o.Run())
		},
	}

	cmd.Flags().StringVar(&clusterName, "cluster", "", "The cluster to restore")
	cmd.Flags().StringVar(&o.RestoreSpec.RestorePointInTime, "restore-to-time", "", "point in time recovery(PITR)")
	cmd.Flags().StringVar(&restoreKey, "restore-key", "", "specify the key to restore in kv database, support multiple keys split by comma with wildcard pattern matching")
	cmd.Flags().BoolVar(&restoreKeyIgnoreErrors, "restore-key-ignore-errors", false, "whether or not to ignore errors when restore kv database by keys")
	cmd.Flags().StringVar(&o.RestoreSpec.VolumeRestorePolicy, "volume-restore-policy", "Parallel", "the volume claim restore policy, supported values: [Serial, Parallel]")
	return cmd
}

func newListRestoreCommand(f cmdutil.Factory, streams genericiooptions.IOStreams) *cobra.Command {
	o := action.NewListOptions(f, streams, types.RestoreGVR())
	clusterName := ""
	cmd := &cobra.Command{
		Use:               "list-restore",
		Short:             "List restores.",
		Aliases:           []string{"ls-restores"},
		Example:           listRestoreExample,
		ValidArgsFunction: util.ResourceNameCompletionFunc(f, o.GVR),
		Run: func(cmd *cobra.Command, args []string) {
			if clusterName != "" {
				o.LabelSelector = util.BuildLabelSelectorByNames(o.LabelSelector, []string{clusterName})
			}
			o.Names = args
			cmdutil.BehaviorOnFatal(printer.FatalWithRedColor)
			cmdutil.CheckErr(o.Complete())
			cmdutil.CheckErr(PrintRestoreList(o))
		},
	}
	o.AddFlags(cmd, false)
	cmd.Flags().StringVar(&clusterName, "cluster", "", "List restores in the specified cluster")
	util.RegisterClusterCompletionFunc(cmd, f)
	return cmd
}

func PrintRestoreList(o *action.ListOptions) error {
	headers := []any{"NAME", "NAMESPACE", "CLUSTER", "BACKUP", "RESTORE-TIME", "STATUS", "DURATION", "CREATE-TIME", "COMPLETION-TIME"}
	return o.PrintObjectList(headers, func(tbl *printer.TablePrinter, unstructuredObj unstructured.Unstructured) error {
		restore := &dpv1alpha1.Restore{}
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(unstructuredObj.Object, restore); err != nil {
			return err
		}
		sourceCluster := restore.Labels[constant.AppInstanceLabelKey]
		durationStr := ""
		if restore.Status.Duration != nil {
			durationStr = duration.HumanDuration(restore.Status.Duration.Duration)
		}
		tbl.AddRow(restore.Name, restore.Namespace, sourceCluster, restore.Spec.Backup.Name, restore.Spec.RestoreTime, string(restore.Status.Phase),
			durationStr, util.TimeFormat(&restore.CreationTimestamp), util.TimeFormat(restore.Status.CompletionTimestamp))
		return nil
	})
}

func newRestoreDescribeCommand(f cmdutil.Factory, streams genericiooptions.IOStreams) *cobra.Command {
	o := &DescribeDPOptions{
		Factory:   f,
		IOStreams: streams,
		Gvr:       types.RestoreGVR(),
	}
	cmd := &cobra.Command{
		Use:               "describe-restore NAME",
		Short:             "Describe a restore",
		Aliases:           []string{"desc-restore"},
		ValidArgsFunction: util.ResourceNameCompletionFunc(f, types.RestoreGVR()),
		Example:           describeRestoreExample,
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.BehaviorOnFatal(printer.FatalWithRedColor)
			util.CheckErr(o.Validate(args))
			util.CheckErr(o.Complete())
			util.CheckErr(DescribeRestores(o, args))
		},
	}
	return cmd
}

func DescribeRestores(o *DescribeDPOptions, restoreNames []string) error {
	for _, restoreName := range restoreNames {
		obj := &dpv1alpha1.Restore{}
		if err := util.GetK8SClientObject(o.Dynamic, obj, o.Gvr, o.Namespace, restoreName); err != nil {
			return err
		}
		if err := PrintRestoreDescribe(o, obj); err != nil {
			return err
		}
	}
	return nil
}

func PrintRestoreDescribe(o *DescribeDPOptions, obj *dpv1alpha1.Restore) error {
	printer.PrintLine("Summary:")
	realPrintPairStringToLine("Name", obj.Name)
	realPrintPairStringToLine("Cluster", obj.Labels[constant.AppInstanceLabelKey])
	realPrintPairStringToLine("Component", obj.Labels[constant.KBAppComponentLabelKey])
	realPrintPairStringToLine("Namespace", obj.Namespace)
	realPrintPairStringToLine("Phase", string(obj.Status.Phase))
	if obj.Status.Duration != nil {
		realPrintPairStringToLine("Duration", duration.HumanDuration(obj.Status.Duration.Duration))
	}
	printer.PrintLine("\nBackup:")
	realPrintPairStringToLine("Backup Name", obj.Spec.Backup.Name)
	realPrintPairStringToLine("Backup Namespace", obj.Spec.Backup.Namespace)
	realPrintPairStringToLine("Restore Time", obj.Spec.RestoreTime)
	realPrintPairStringToLine("Source Target", obj.Spec.Backup.SourceTargetName)
	if len(obj.Spec.Env) > 0 {
		printer.PrintLine("\nRestore Env:")
		for _, v := range obj.Spec.Env {
			realPrintPairStringToLine(v.Name, v.Value)
		}
	}
	printAction := func(v dpv1alpha1.RestoreStatusAction, i int) {
		fmt.Printf("=================== %d ===================\n", i+1)
		realPrintPairStringToLine("Action Name", v.Name)
		realPrintPairStringToLine("Backup Name", v.BackupName)
		realPrintPairStringToLine("Workload Name", v.ObjectKey)
		realPrintPairStringToLine("Status", string(v.Status))
		realPrintPairStringToLine("Message", v.Message)
		realPrintPairStringToLine("Start Time", util.TimeFormat(&v.StartTime))
		realPrintPairStringToLine("End Time", util.TimeFormat(&v.EndTime))
	}
	if obj.Spec.PrepareDataConfig != nil {
		printer.PrintLine("\nPrepareData Actions:")
		for i, v := range obj.Status.Actions.PrepareData {
			printAction(v, i)
		}
	}
	if obj.Spec.ReadyConfig != nil {
		printer.PrintLine("\nPostReady Actions:")
		for i, v := range obj.Status.Actions.PostReady {
			printAction(v, i)
		}
	}

	// get all events about backup
	events, err := o.Client.CoreV1().Events(o.Namespace).Search(scheme.Scheme, obj)
	if err != nil {
		return err
	}

	// print the warning events
	printer.PrintAllWarningEvents(events, o.Out)
	return nil
}

func GetBackupByName(dynamic dynamic.Interface, name string, namespace string) (*dpv1alpha1.Backup, error) {
	backup := &dpv1alpha1.Backup{}
	if err := util.GetK8SClientObject(dynamic, backup, types.BackupGVR(), namespace, name); err != nil {
		return nil, err
	}
	return backup, nil
}
