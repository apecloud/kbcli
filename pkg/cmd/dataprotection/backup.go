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
	"strings"
	"time"

	dpv1alpha1 "github.com/apecloud/kubeblocks/apis/dataprotection/v1alpha1"
	opsv1alpha1 "github.com/apecloud/kubeblocks/apis/operations/v1alpha1"
	"github.com/apecloud/kubeblocks/pkg/constant"
	dptypes "github.com/apecloud/kubeblocks/pkg/dataprotection/types"
	"github.com/spf13/cobra"
	"github.com/stoewer/go-strcase"
	"golang.org/x/exp/maps"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/duration"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	"k8s.io/client-go/kubernetes/scheme"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/kubectl/pkg/util/templates"

	"github.com/apecloud/kbcli/pkg/action"
	"github.com/apecloud/kbcli/pkg/printer"
	"github.com/apecloud/kbcli/pkg/types"
	"github.com/apecloud/kbcli/pkg/util"
)

var (
	createBackupExample = templates.Examples(`
		# Create a backup for the cluster, use the default backup policy and volume snapshot backup method
		kbcli dp backup mybackup --cluster mycluster

		# create a backup with a specified method, run "kbcli cluster desc-backup-policy mycluster" to show supported backup methods
		kbcli dp backup mybackup --cluster mycluster --method mymethod

		# create a backup with specified backup policy, run "kbcli cluster list-backup-policy mycluster" to show the cluster supported backup policies
		kbcli dp backup mybackup --cluster mycluster --policy mypolicy

		# create a backup from a parent backup
		kbcli dp backup mybackup --cluster mycluster --parent-backup myparentbackup
	`)

	deleteBackupExample = templates.Examples(`
		# delete a backup
		kbcli dp delete-backup mybackup
	`)

	describeBackupExample = templates.Examples(`
		# describe a backup
		kbcli dp describe-backup mybackup
	`)

	listBackupExample = templates.Examples(`
		# list all backups
		kbcli dp list-backup

		# list all backups of specified cluster
		kbcli dp list-backup --cluster mycluster
	`)
)

type CreateBackupOptions struct {
	BackupSpec     opsv1alpha1.Backup `json:"backupSpec"`
	ClusterName    string             `json:"clusterName"`
	OpsType        string             `json:"opsType"`
	OpsRequestName string             `json:"opsRequestName"`
	Force          bool               `json:"force"`

	action.CreateOptions `json:"-"`
}

func (o *CreateBackupOptions) CompleteBackup() error {
	if err := o.Complete(); err != nil {
		return err
	}
	// generate backupName
	if len(o.BackupSpec.BackupName) == 0 {
		o.BackupSpec.BackupName = strings.Join([]string{"backup", o.Namespace, o.Name, time.Now().Format("20060102150405")}, "-")
	}

	// set ops type, ops request name and clusterName
	o.OpsType = string(opsv1alpha1.BackupType)
	o.OpsRequestName = o.BackupSpec.BackupName
	o.ClusterName = o.Name

	return o.CreateOptions.Complete()
}

func (o *CreateBackupOptions) Validate() error {
	if o.Name == "" {
		return fmt.Errorf("missing cluster name")
	}

	// if backup policy is not specified, use the default backup policy
	if o.BackupSpec.BackupPolicyName == "" {
		if err := o.completeDefaultBackupPolicy(); err != nil {
			return err
		}
	}

	// check if backup policy exists
	backupPolicy := &dpv1alpha1.BackupPolicy{}
	if err := util.GetK8SClientObject(o.Dynamic, backupPolicy, types.BackupPolicyGVR(), o.Namespace, o.BackupSpec.BackupPolicyName); err != nil {
		return err
	}

	if o.BackupSpec.BackupMethod == "" {
		return fmt.Errorf("backup method can not be empty, you can specify it by --method")
	}

	// check if the backup method exists in backup policy
	exist, availableMethods := false, make([]string, 0)
	for _, method := range backupPolicy.Spec.BackupMethods {
		availableMethods = append(availableMethods, method.Name)
		if o.BackupSpec.BackupMethod == method.Name {
			exist = true
			break
		}
	}
	if !exist {
		return fmt.Errorf("specified backup method %s does not exist in backup policy %s, available methods: [%s]",
			o.BackupSpec.BackupMethod, backupPolicy.Name, strings.Join(availableMethods, ", "))
	}

	// check if the backup repo exists in backup policy
	backupRepoName := backupPolicy.Spec.BackupRepoName
	if backupRepoName != nil {
		_, err := o.Dynamic.Resource(types.BackupRepoGVR()).Get(context.Background(), *backupRepoName, metav1.GetOptions{})
		if err != nil {
			return err
		}
	} else {
		backupRepoList, err := o.Dynamic.Resource(types.BackupRepoGVR()).List(context.Background(), metav1.ListOptions{})
		if err != nil {
			return err
		}
		if len(backupRepoList.Items) == 0 {
			return fmt.Errorf("no backuprepo found")
		}
		var defaultBackupRepos []unstructured.Unstructured
		for _, item := range backupRepoList.Items {
			if item.GetAnnotations()[dptypes.DefaultBackupRepoAnnotationKey] == "true" {
				defaultBackupRepos = append(defaultBackupRepos, item)
			}
		}
		if len(defaultBackupRepos) == 0 {
			return fmt.Errorf("no default backuprepo exists")
		}
		if len(defaultBackupRepos) > 1 {
			return fmt.Errorf("cluster %s has multiple default backuprepos", o.Name)
		}
	}
	// TODO: check if pvc exists

	// valid retention period
	if o.BackupSpec.RetentionPeriod != "" {
		_, err := dpv1alpha1.RetentionPeriod(o.BackupSpec.RetentionPeriod).ToDuration()
		if err != nil {
			return fmt.Errorf("invalid retention period, please refer to examples [1y, 1m, 1d, 1h, 1m] or combine them [1y1m1d1h1m]")
		}
	}

	// check if parent backup exists
	if o.BackupSpec.ParentBackupName != "" {
		parentBackup := &dpv1alpha1.Backup{}
		if err := util.GetK8SClientObject(o.Dynamic, parentBackup, types.BackupGVR(), o.Namespace, o.BackupSpec.ParentBackupName); err != nil {
			return err
		}
		if parentBackup.Status.Phase != dpv1alpha1.BackupPhaseCompleted {
			return fmt.Errorf("parent backup %s is not completed", o.BackupSpec.ParentBackupName)
		}
		if parentBackup.Labels[constant.AppInstanceLabelKey] != o.Name {
			return fmt.Errorf("parent backup %s is not belong to cluster %s", o.BackupSpec.ParentBackupName, o.Name)
		}
	}
	return nil
}

// completeDefaultBackupPolicy completes the default backup policy.
func (o *CreateBackupOptions) completeDefaultBackupPolicy() error {
	defaultBackupPolicyName, err := o.getDefaultBackupPolicy()
	if err != nil {
		return err
	}
	o.BackupSpec.BackupPolicyName = defaultBackupPolicyName
	return nil
}

func (o *CreateBackupOptions) getDefaultBackupPolicy() (string, error) {
	clusterObj, err := o.Dynamic.Resource(types.ClusterGVR()).Namespace(o.Namespace).Get(context.TODO(), o.Name, metav1.GetOptions{})
	if err != nil {
		return "", err
	}

	// TODO: support multiple components backup, add --componentDef flag
	opts := metav1.ListOptions{
		LabelSelector: fmt.Sprintf("%s=%s",
			constant.AppInstanceLabelKey, clusterObj.GetName()),
	}
	objs, err := o.Dynamic.
		Resource(types.BackupPolicyGVR()).Namespace(o.Namespace).
		List(context.TODO(), opts)
	if err != nil {
		return "", err
	}
	if len(objs.Items) == 0 {
		return "", fmt.Errorf(`not found any backup policy for cluster "%s"`, o.Name)
	}
	var defaultBackupPolicies []unstructured.Unstructured
	for _, obj := range objs.Items {
		if obj.GetAnnotations()[dptypes.DefaultBackupPolicyAnnotationKey] == TrueValue {
			defaultBackupPolicies = append(defaultBackupPolicies, obj)
		}
	}
	if len(defaultBackupPolicies) == 0 {
		return "", fmt.Errorf(`not found any default backup policy for cluster "%s"`, o.Name)
	}
	if len(defaultBackupPolicies) > 1 {
		return "", fmt.Errorf(`cluster "%s" has multiple default backup policies`, o.Name)
	}
	return defaultBackupPolicies[0].GetName(), nil
}

func newBackupCommand(f cmdutil.Factory, streams genericiooptions.IOStreams) *cobra.Command {
	customOutPut := func(opt *action.CreateOptions) {
		output := fmt.Sprintf("Backup %s created successfully, you can view the progress:", opt.Name)
		printer.PrintLine(output)
		nextLine := fmt.Sprintf("\tkbcli dp list-backup %s -n %s", opt.Name, opt.Namespace)
		printer.PrintLine(nextLine)
	}

	clusterName := ""

	o := &CreateBackupOptions{
		CreateOptions: action.CreateOptions{
			IOStreams:       streams,
			Factory:         f,
			GVR:             types.OpsGVR(),
			CueTemplateName: "opsrequest_template.cue",
			CustomOutPut:    customOutPut,
		},
	}
	o.CreateOptions.Options = o

	cmd := &cobra.Command{
		Use:     "backup NAME",
		Short:   "Create a backup for the cluster.",
		Example: createBackupExample,
		Run: func(cmd *cobra.Command, args []string) {
			if len(args) > 0 {
				o.BackupSpec.BackupName = args[0]
			}
			if clusterName != "" {
				o.Args = []string{clusterName}
			}
			cmdutil.BehaviorOnFatal(printer.FatalWithRedColor)
			cmdutil.CheckErr(o.CompleteBackup())
			cmdutil.CheckErr(o.Validate())
			cmdutil.CheckErr(o.Run())
		},
	}

	cmd.Flags().StringVar(&o.BackupSpec.BackupMethod, "method", "", "Backup methods are defined in backup policy (required), if only one backup method in backup policy, use it as default backup method, if multiple backup methods in backup policy, use method which volume snapshot is true as default backup method")
	cmd.Flags().StringVar(&clusterName, "cluster", "", "Cluster name")
	cmd.Flags().StringVar(&o.BackupSpec.BackupPolicyName, "policy", "", "Backup policy name, if not specified, use the cluster default backup policy")
	cmd.Flags().StringVar(&o.BackupSpec.DeletionPolicy, "deletion-policy", "Delete", "Deletion policy for backup, determine whether the backup content in backup repo will be deleted after the backup is deleted, supported values: [Delete, Retain]")
	cmd.Flags().StringVar(&o.BackupSpec.RetentionPeriod, "retention-period", "", "Retention period for backup, supported values: [1y, 1mo, 1d, 1h, 1m] or combine them [1y1mo1d1h1m], if not specified, the backup will not be automatically deleted, you need to manually delete it.")
	cmd.Flags().StringVar(&o.BackupSpec.ParentBackupName, "parent-backup", "", "Parent backup name, used for incremental backup")
	util.RegisterClusterCompletionFunc(cmd, f)
	o.RegisterBackupFlagCompletionFunc(cmd, f)

	return cmd
}

func (o *CreateBackupOptions) RegisterBackupFlagCompletionFunc(cmd *cobra.Command, f cmdutil.Factory) {
	util.CheckErr(cmd.RegisterFlagCompletionFunc(
		"deletion-policy",
		func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			return []string{string(dpv1alpha1.BackupDeletionPolicyRetain), string(dpv1alpha1.BackupDeletionPolicyDelete)}, cobra.ShellCompDirectiveNoFileComp
		}))

	util.CheckErr(cmd.RegisterFlagCompletionFunc(
		"policy",
		func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			label := fmt.Sprintf("%s=%s", constant.AppInstanceLabelKey, util.GetClusterNameFromArgsOrFlag(cmd, args))
			return util.CompGetResourceWithLabels(f, cmd, util.GVRToString(types.BackupPolicyGVR()), []string{label}, toComplete), cobra.ShellCompDirectiveNoFileComp
		}))

	util.CheckErr(cmd.RegisterFlagCompletionFunc(
		"method",
		func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			namespace, _ := cmd.Flags().GetString("namespace")
			if namespace == "" {
				namespace, _, _ = f.ToRawKubeConfigLoader().Namespace()
			}
			var (
				labelSelector string
				clusterName   = util.GetClusterNameFromArgsOrFlag(cmd, args)
			)
			if clusterName != "" {
				labelSelector = fmt.Sprintf("%s=%s", constant.AppInstanceLabelKey, clusterName)
			}
			dynamicClient, _ := f.DynamicClient()
			objs, _ := dynamicClient.Resource(types.BackupPolicyGVR()).Namespace(namespace).List(context.TODO(), metav1.ListOptions{
				LabelSelector: labelSelector,
			})
			methodMap := map[string]struct{}{}
			for _, v := range objs.Items {
				backupPolicy := &dpv1alpha1.BackupPolicy{}
				_ = runtime.DefaultUnstructuredConverter.FromUnstructured(v.Object, backupPolicy)
				for _, m := range backupPolicy.Spec.BackupMethods {
					methodMap[m.Name] = struct{}{}
				}
			}
			return maps.Keys(methodMap), cobra.ShellCompDirectiveNoFileComp
		}))
}

func newBackupDeleteCommand(f cmdutil.Factory, streams genericiooptions.IOStreams) *cobra.Command {
	o := action.NewDeleteOptions(f, streams, types.BackupGVR())
	clusterName := ""
	cmd := &cobra.Command{
		Use:               "delete-backup",
		Short:             "Delete a backup.",
		Example:           deleteBackupExample,
		ValidArgsFunction: util.ResourceNameCompletionFunc(f, types.BackupGVR()),
		Run: func(cmd *cobra.Command, args []string) {
			o.Names = args
			cmdutil.BehaviorOnFatal(printer.FatalWithRedColor)
			util.CheckErr(completeForDeleteBackup(o, clusterName))
			util.CheckErr(o.Run())
		},
	}

	o.AddFlags(cmd)
	cmd.Flags().StringVar(&clusterName, "cluster", "", "The cluster name.")
	util.RegisterClusterCompletionFunc(cmd, f)

	return cmd
}

func completeForDeleteBackup(o *action.DeleteOptions, cluster string) error {
	if len(o.Names) == 0 {
		if cluster == "" {
			return fmt.Errorf("must give a backup name or cluster name")
		}
		o.LabelSelector = util.BuildLabelSelectorByNames(o.LabelSelector, []string{cluster})
	}
	return nil
}

func newListBackupCommand(f cmdutil.Factory, streams genericiooptions.IOStreams) *cobra.Command {
	o := action.NewListOptions(f, streams, types.BackupGVR())
	clusterName := ""
	cmd := &cobra.Command{
		Use:               "list-backup",
		Short:             "List backups.",
		Aliases:           []string{"ls-backups"},
		Example:           listBackupExample,
		ValidArgsFunction: util.ResourceNameCompletionFunc(f, o.GVR),
		Run: func(cmd *cobra.Command, args []string) {
			if clusterName != "" {
				o.LabelSelector = util.BuildLabelSelectorByNames(o.LabelSelector, []string{clusterName})
			}
			o.Names = args
			cmdutil.BehaviorOnFatal(printer.FatalWithRedColor)
			cmdutil.CheckErr(o.Complete())
			cmdutil.CheckErr(PrintBackupList(o))
		},
	}
	o.AddFlags(cmd, false)
	cmd.Flags().StringVar(&clusterName, "cluster", "", "List backups in the specified cluster")
	util.RegisterClusterCompletionFunc(cmd, f)

	return cmd
}

func PrintBackupList(o *action.ListOptions) error {
	headers := []any{"NAME", "NAMESPACE", "SOURCE-CLUSTER", "METHOD", "STATUS", "TOTAL-SIZE", "DURATION", "DELETION-POLICY", "CREATE-TIME", "COMPLETION-TIME", "EXPIRATION"}
	return o.PrintObjectList(headers, func(tbl *printer.TablePrinter, unstructuredObj unstructured.Unstructured) error {
		backup := &dpv1alpha1.Backup{}
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(unstructuredObj.Object, backup); err != nil {
			return err
		}
		sourceCluster := backup.Labels[constant.AppInstanceLabelKey]
		durationStr := ""
		if backup.Status.Duration != nil {
			durationStr = duration.HumanDuration(backup.Status.Duration.Duration)
		}
		statusString := string(backup.Status.Phase)
		var availableReplicas *int32
		for _, v := range backup.Status.Actions {
			if v.ActionType == dpv1alpha1.ActionTypeStatefulSet {
				availableReplicas = v.AvailableReplicas
				break
			}
		}
		if availableReplicas != nil {
			statusString = fmt.Sprintf("%s(AvailablePods: %d)", statusString, *availableReplicas)
		}
		tbl.AddRow(backup.Name, backup.Namespace, sourceCluster, backup.Spec.BackupMethod, statusString, backup.Status.TotalSize,
			durationStr, backup.Spec.DeletionPolicy, util.TimeFormat(&backup.CreationTimestamp), util.TimeFormat(backup.Status.CompletionTimestamp),
			util.TimeFormat(backup.Status.Expiration))
		return nil
	})
}

func newBackupDescribeCommand(f cmdutil.Factory, streams genericiooptions.IOStreams) *cobra.Command {
	o := &DescribeDPOptions{
		Factory:   f,
		IOStreams: streams,
		Gvr:       types.BackupGVR(),
	}
	cmd := &cobra.Command{
		Use:               "describe-backup NAME",
		Short:             "Describe a backup",
		Aliases:           []string{"desc-backup"},
		ValidArgsFunction: util.ResourceNameCompletionFunc(f, types.BackupGVR()),
		Example:           describeBackupExample,
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.BehaviorOnFatal(printer.FatalWithRedColor)
			util.CheckErr(o.Validate(args))
			util.CheckErr(o.Complete())
			util.CheckErr(DescribeBackups(o, args))
		},
	}
	return cmd
}

func DescribeBackups(o *DescribeDPOptions, backupNames []string) error {
	for _, backupName := range backupNames {
		obj := &dpv1alpha1.Backup{}
		if err := util.GetK8SClientObject(o.Dynamic, obj, o.Gvr, o.Namespace, backupName); err != nil {
			return err
		}
		if err := PrintBackupObjDescribe(o, obj); err != nil {
			return err
		}
	}
	return nil
}

func PrintBackupObjDescribe(o *DescribeDPOptions, obj *dpv1alpha1.Backup) error {
	targetCluster := obj.Labels[constant.AppInstanceLabelKey]
	printer.PrintLineWithTabSeparator(
		printer.NewPair("Name", obj.Name),
		printer.NewPair("Cluster", targetCluster),
		printer.NewPair("Namespace", obj.Namespace),
	)
	printer.PrintLine("\nSpec:")
	realPrintPairStringToLine("Method", obj.Spec.BackupMethod)
	realPrintPairStringToLine("Policy Name", obj.Spec.BackupPolicyName)

	printer.PrintLine("\nActions:")
	for _, v := range obj.Status.Actions {
		printer.PrintPairStringToLine(v.Name, "")
		realPrintPairStringToLine("ActionType", string(v.ActionType), 4)
		realPrintPairStringToLine("WorkloadName", v.ObjectRef.Name, 4)
		realPrintPairStringToLine("TargetPodName", v.TargetPodName, 4)
		realPrintPairStringToLine("Phase", string(v.Phase), 4)
		realPrintPairStringToLine("FailureReason", v.FailureReason, 4)
		realPrintPairStringToLine("Start Time", util.TimeFormat(v.StartTimestamp), 4)
		realPrintPairStringToLine("Completion Time", util.TimeFormat(v.CompletionTimestamp), 4)
	}
	if len(obj.Status.Extras) > 0 {
		printer.PrintLine("\nExtras:")
		for i, extra := range obj.Status.Extras {
			fmt.Printf("=================== %d ===================\n", i+1)
			for key, value := range extra {
				realPrintPairStringToLine(strcase.LowerCamelCase(key), value)
			}
		}
	}
	printer.PrintLine("\nStatus:")
	realPrintPairStringToLine("Phase", string(obj.Status.Phase))
	realPrintPairStringToLine("FailureReason", obj.Status.FailureReason)
	realPrintPairStringToLine("Total Size", obj.Status.TotalSize)
	if obj.Status.BackupMethod != nil {
		realPrintPairStringToLine("ActionSet Name", obj.Status.BackupMethod.ActionSetName)
	}
	realPrintPairStringToLine("Repository", obj.Status.BackupRepoName)
	realPrintPairStringToLine("PVC Name", obj.Status.PersistentVolumeClaimName)
	if obj.Status.Duration != nil {
		realPrintPairStringToLine("Duration", duration.HumanDuration(obj.Status.Duration.Duration))
	}
	realPrintPairStringToLine("Expiration Time", util.TimeFormat(obj.Status.Expiration))
	realPrintPairStringToLine("Start Time", util.TimeFormat(obj.Status.StartTimestamp))
	realPrintPairStringToLine("Completion Time", util.TimeFormat(obj.Status.CompletionTimestamp))
	// print failure reason, ignore error
	realPrintPairStringToLine("Failure Reason", obj.Status.FailureReason)
	realPrintPairStringToLine("Path", obj.Status.Path)

	if obj.Status.TimeRange != nil {
		realPrintPairStringToLine("Time Range Start", util.TimeFormat(obj.Status.TimeRange.Start))
		realPrintPairStringToLine("Time Range End", util.TimeFormat(obj.Status.TimeRange.End))
	}

	if len(obj.Status.VolumeSnapshots) > 0 {
		printer.PrintLine("\nVolume Snapshots:")
		for _, v := range obj.Status.VolumeSnapshots {
			realPrintPairStringToLine("Name", v.Name)
			realPrintPairStringToLine("Content Name", v.ContentName)
			realPrintPairStringToLine("Volume Name:", v.VolumeName)
			realPrintPairStringToLine("Size", v.Size)
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

func realPrintPairStringToLine(name, value string, spaceCount ...int) {
	if value != "" {
		printer.PrintPairStringToLine(name, value, spaceCount...)
	}
}
