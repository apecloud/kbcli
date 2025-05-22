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
	"fmt"
	"strconv"

	dpv1alpha1 "github.com/apecloud/kubeblocks/apis/dataprotection/v1alpha1"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	apiruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/kubectl/pkg/util/templates"

	"github.com/apecloud/kbcli/pkg/action"
	dp "github.com/apecloud/kbcli/pkg/cmd/dataprotection"
	"github.com/apecloud/kbcli/pkg/printer"
	"github.com/apecloud/kbcli/pkg/types"
	"github.com/apecloud/kbcli/pkg/util"
)

var (
	listBackupPolicyExample = templates.Examples(`
		# list all backup policies
		kbcli cluster list-backup-policies

		# using short cmd to list backup policy of the specified cluster
        kbcli cluster list-bp mycluster
	`)
	editExample = templates.Examples(`
		# edit backup policy
		kbcli cluster edit-backup-policy <backup-policy-name>
	`)
	createBackupExample = templates.Examples(`
		# Create a backup for the cluster, use the default backup policy and volume snapshot backup method
		kbcli cluster backup mycluster

		# create a backup with a specified method, run "kbcli cluster desc-backup-policy mycluster" to show supported backup methods
		kbcli cluster backup mycluster --method volume-snapshot

		# create a backup with specified backup policy, run "kbcli cluster list-backup-policies mycluster" to show the cluster supported backup policies
		kbcli cluster backup mycluster --method volume-snapshot --policy <backup-policy-name>

		# create a backup from a parent backup
		kbcli cluster backup mycluster --parent-backup parent-backup-name
	`)
	listBackupExample = templates.Examples(`
		# list all backups
		kbcli cluster list-backups

	   # list all backups of the cluster 
		kbcli cluster list-backups <clusterName>
      
        # list the specified backups 
		kbcli cluster list-backups --names b1,b2
	`)
	deleteBackupExample = templates.Examples(`
		# delete a backup named backup-name
		kbcli cluster delete-backup cluster-name --name backup-name
	`)
	createRestoreExample = templates.Examples(`
		# restore a new cluster from a backup
		kbcli cluster restore new-cluster-name --backup backup-name
	`)
	listRestoreExample = templates.Examples(`
		# list all restores
		kbcli cluster list-restores

	   # list all restores of the cluster 
		kbcli cluster list-restores <clusterName>
      
        # list the specified restores 
		kbcli cluster list-restores --names r1,r2
	`)
	describeBackupExample = templates.Examples(`
		# describe backups of the cluster
		kbcli cluster describe-backup <clusterName>

		# describe a backup
		kbcli cluster describe-backup --names <backupName>
	`)
	describeBackupPolicyExample = templates.Examples(`
		# describe the default backup policy of the cluster
		kbcli cluster describe-backup-policy cluster-name

		# describe the backup policy of the cluster with specified name
		kbcli cluster describe-backup-policy cluster-name --name backup-policy-name
	`)
	describeRestoreExample = templates.Examples(`
		# describe a restore
		kbcli cluster describe-restore <restoreName>
	`)
)

func NewCreateBackupCmd(f cmdutil.Factory, streams genericiooptions.IOStreams) *cobra.Command {
	customOutPut := func(opt *action.CreateOptions) {
		output := fmt.Sprintf("Backup %s created successfully, you can view the progress:", opt.Name)
		printer.PrintLine(output)
		nextLine := fmt.Sprintf("\tkbcli cluster list-backups --name=%s -n %s", opt.Name, opt.Namespace)
		printer.PrintLine(nextLine)
	}

	o := &dp.CreateBackupOptions{
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
		Use:               "backup NAME",
		Short:             "Create a backup for the cluster.",
		Example:           createBackupExample,
		ValidArgsFunction: util.ResourceNameCompletionFunc(f, types.ClusterGVR()),
		Run: func(cmd *cobra.Command, args []string) {
			o.Args = args
			cmdutil.BehaviorOnFatal(printer.FatalWithRedColor)
			cmdutil.CheckErr(o.CompleteBackup())
			cmdutil.CheckErr(o.Validate())
			cmdutil.CheckErr(o.Run())
		},
	}
	o.AddCommonFlags(cmd)
	cmd.Flags().StringVar(&o.BackupSpec.BackupMethod, "method", "", "Backup methods are defined in backup policy (required), if only one backup method in backup policy, use it as default backup method, if multiple backup methods in backup policy, use method which volume snapshot is true as default backup method")
	cmd.Flags().StringVar(&o.BackupSpec.BackupName, "name", "", "Backup name")
	cmd.Flags().StringVar(&o.BackupSpec.BackupPolicyName, "policy", "", "Backup policy name, if not specified, use the cluster default backup policy")
	cmd.Flags().StringVar(&o.BackupSpec.DeletionPolicy, "deletion-policy", "Delete", "Deletion policy for backup, determine whether the backup content in backup repo will be deleted after the backup is deleted, supported values: [Delete, Retain]")
	cmd.Flags().StringVar(&o.BackupSpec.RetentionPeriod, "retention-period", "", "Retention period for backup, supported values: [1y, 1mo, 1d, 1h, 1m] or combine them [1y1mo1d1h1m], if not specified, the backup will not be automatically deleted, you need to manually delete it.")
	cmd.Flags().StringVar(&o.BackupSpec.ParentBackupName, "parent-backup", "", "Parent backup name, used for incremental backup")
	// register backup flag completion func
	o.RegisterBackupFlagCompletionFunc(cmd, f)
	return cmd
}

func NewListBackupCmd(f cmdutil.Factory, streams genericiooptions.IOStreams) *cobra.Command {
	o := action.NewListOptions(f, streams, types.BackupGVR())
	cmd := &cobra.Command{
		Use:               "list-backups",
		Short:             "List backups.",
		Aliases:           []string{"ls-backup"},
		Example:           listBackupExample,
		ValidArgsFunction: util.ResourceNameCompletionFunc(f, types.ClusterGVR()),
		Run: func(cmd *cobra.Command, args []string) {
			o.LabelSelector = util.BuildLabelSelectorByNames(o.LabelSelector, args)
			cmdutil.BehaviorOnFatal(printer.FatalWithRedColor)
			util.CheckErr(o.Complete())
			util.CheckErr(dp.PrintBackupList(o))
		},
	}
	o.AddFlags(cmd)
	cmd.Flags().StringSliceVar(&o.Names, "names", nil, "The backup name to get the details.")
	return cmd
}

func NewDescribeBackupCmd(f cmdutil.Factory, streams genericiooptions.IOStreams) *cobra.Command {
	o := &dp.DescribeDPOptions{
		Factory:   f,
		IOStreams: streams,
		Gvr:       types.BackupGVR(),
	}
	cmd := &cobra.Command{
		Use:               "describe-backup BACKUP-NAME",
		Short:             "Describe a backup.",
		Aliases:           []string{"desc-backup"},
		Example:           describeBackupExample,
		ValidArgsFunction: util.ResourceNameCompletionFunc(f, types.BackupGVR()),
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.BehaviorOnFatal(printer.FatalWithRedColor)
			util.CheckErr(o.ValidateForClusterCmd(args))
			util.CheckErr(o.Complete())
			util.CheckErr(describeBackups(o, args))
		},
	}
	cmd.Flags().StringSliceVar(&o.Names, "names", []string{}, "Backup names")
	return cmd
}

func describeBackups(o *dp.DescribeDPOptions, args []string) error {
	if len(o.Names) > 0 {
		return dp.DescribeBackups(o, o.Names)
	}
	backupLIst, err := o.GetObjListByArgs(args)
	if err != nil {
		return err
	}
	for _, v := range backupLIst.Items {
		obj := &dpv1alpha1.Backup{}
		if err = apiruntime.DefaultUnstructuredConverter.FromUnstructured(v.UnstructuredContent(), obj); err != nil {
			return err
		}
		if err = dp.PrintBackupObjDescribe(o, obj); err != nil {
			return err
		}
	}
	return nil
}

func NewDeleteBackupCmd(f cmdutil.Factory, streams genericiooptions.IOStreams) *cobra.Command {
	o := action.NewDeleteOptions(f, streams, types.BackupGVR())
	cmd := &cobra.Command{
		Use:               "delete-backup",
		Short:             "Delete a backup.",
		Example:           deleteBackupExample,
		ValidArgsFunction: util.ResourceNameCompletionFunc(f, types.ClusterGVR()),
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.BehaviorOnFatal(printer.FatalWithRedColor)
			util.CheckErr(completeForDeleteBackup(o, args))
			util.CheckErr(o.Run())
		},
	}
	cmd.Flags().StringSliceVar(&o.Names, "name", []string{}, "Backup names")
	o.AddFlags(cmd)
	return cmd
}

// completeForDeleteBackup completes cmd for delete backup
func completeForDeleteBackup(o *action.DeleteOptions, args []string) error {
	if len(args) == 0 {
		return errors.New("Missing cluster name")
	}
	if len(args) > 1 {
		return errors.New("Only supported delete the Backup of one cluster")
	}
	if !o.Force && len(o.Names) == 0 {
		return errors.New("Missing --name as backup name.")
	}
	if o.Force && len(o.Names) == 0 {
		// do force action, for --force and --name unset, delete all backups of the cluster
		// if backup name unset and cluster name set, delete all backups of the cluster
		o.LabelSelector = util.BuildLabelSelectorByNames(o.LabelSelector, args)
		o.ConfirmedNames = args
	}
	o.ConfirmedNames = o.Names
	return nil
}

func NewCreateRestoreCmd(f cmdutil.Factory, streams genericiooptions.IOStreams) *cobra.Command {
	restoreKey := ""
	restoreKeyIgnoreErrors := false

	customOutPut := func(opt *action.CreateOptions) {
		output := fmt.Sprintf("Cluster %s created", opt.Name)
		printer.PrintLine(output)
	}

	o := &dp.CreateRestoreOptions{}
	o.CreateOptions = action.CreateOptions{
		IOStreams:       streams,
		Factory:         f,
		Options:         o,
		GVR:             types.OpsGVR(),
		CueTemplateName: "opsrequest_template.cue",
		CustomOutPut:    customOutPut,
	}

	cmd := &cobra.Command{
		Use:     "restore",
		Short:   "Restore a new cluster from backup.",
		Example: createRestoreExample,
		Run: func(cmd *cobra.Command, args []string) {
			o.Args = args
			if restoreKey != "" {
				o.RestoreSpec.Env = append(o.RestoreSpec.Env, corev1.EnvVar{
					Name:  dp.DPEnvRestoreKeyPatterns,
					Value: restoreKey,
				})
			}
			if restoreKeyIgnoreErrors {
				o.RestoreSpec.Env = append(o.RestoreSpec.Env, corev1.EnvVar{
					Name:  dp.DPEnvRestoreKeyIgnoreErrors,
					Value: strconv.FormatBool(restoreKeyIgnoreErrors),
				})
			}
			cmdutil.BehaviorOnFatal(printer.FatalWithRedColor)
			util.CheckErr(o.Complete())
			util.CheckErr(o.Validate())
			util.CheckErr(o.Run())
		},
	}
	o.AddCommonFlags(cmd)
	cmd.Flags().StringVar(&o.RestoreSpec.BackupName, "backup", "", "Backup name")
	cmd.Flags().StringVar(&o.RestoreSpec.BackupNamespace, "backup-namespace", "", "Backup namespace")
	cmd.Flags().StringVar(&o.RestoreSpec.RestorePointInTime, "restore-to-time", "", "point in time recovery(PITR)")
	cmd.Flags().StringVar(&restoreKey, "restore-key", "", "specify the key to restore in kv database, support multiple keys split by comma with wildcard pattern matching")
	cmd.Flags().BoolVar(&restoreKeyIgnoreErrors, "restore-key-ignore-errors", false, "whether or not to ignore errors when restore kv database by keys")
	cmd.Flags().StringVar(&o.RestoreSpec.VolumeRestorePolicy, "volume-restore-policy", "Parallel", "the volume claim restore policy, supported values: [Serial, Parallel]")
	cmd.Flags().BoolVar(&o.RestoreSpec.DeferPostReadyUntilClusterRunning, "restore-after-cluster-running", false, "do the postReady phase when the cluster is Running rather than the component is Running.")
	return cmd
}

func NewListBackupPolicyCmd(f cmdutil.Factory, streams genericiooptions.IOStreams) *cobra.Command {
	o := action.NewListOptions(f, streams, types.BackupPolicyGVR())
	cmd := &cobra.Command{
		Use:               "list-backup-policies",
		Short:             "List backups policies.",
		Aliases:           []string{"list-bp"},
		Example:           listBackupPolicyExample,
		ValidArgsFunction: util.ResourceNameCompletionFunc(f, types.ClusterGVR()),
		Run: func(cmd *cobra.Command, args []string) {
			o.LabelSelector = util.BuildLabelSelectorByNames(o.LabelSelector, args)
			cmdutil.BehaviorOnFatal(printer.FatalWithRedColor)
			util.CheckErr(o.Complete())
			util.CheckErr(dp.PrintBackupPolicyList(o))
		},
	}
	o.AddFlags(cmd)
	cmd.Flags().StringSliceVar(&o.Names, "names", nil, "The backup policy name to get the details.")
	return cmd
}

func NewEditBackupPolicyCmd(f cmdutil.Factory, streams genericiooptions.IOStreams) *cobra.Command {
	o := dp.EditBackupPolicyOptions{Factory: f, IOStreams: streams, GVR: types.BackupPolicyGVR()}
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

func NewDescribeBackupPolicyCmd(f cmdutil.Factory, streams genericiooptions.IOStreams) *cobra.Command {
	o := &dp.DescribeDPOptions{
		Factory:   f,
		IOStreams: streams,
		Gvr:       types.BackupPolicyGVR(),
	}
	cmd := &cobra.Command{
		Use:               "describe-backup-policy",
		Aliases:           []string{"desc-bp"},
		Short:             "Describe backup policy",
		Example:           describeBackupPolicyExample,
		ValidArgsFunction: util.ResourceNameCompletionFunc(f, types.BackupPolicyGVR()),
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.BehaviorOnFatal(printer.FatalWithRedColor)
			util.CheckErr(o.ValidateForClusterCmd(args))
			util.CheckErr(o.Complete())
			util.CheckErr(describeBackupPolicies(o, args))
		},
	}
	cmd.Flags().StringSliceVar(&o.Names, "names", []string{}, "Backup policy names")
	return cmd
}

func describeBackupPolicies(o *dp.DescribeDPOptions, args []string) error {
	if len(o.Names) > 0 {
		return dp.DescribeBackupPolicies(o, o.Names)
	}
	backupPolicyList, err := o.GetObjListByArgs(args)
	if err != nil {
		return err
	}
	for _, v := range backupPolicyList.Items {
		obj := &dpv1alpha1.BackupPolicy{}
		if err := apiruntime.DefaultUnstructuredConverter.FromUnstructured(v.UnstructuredContent(), obj); err != nil {
			return err
		}
		dp.PrintBackupPolicyDescribe(o, obj)
	}
	return nil
}

func NewListRestoreCommand(f cmdutil.Factory, streams genericiooptions.IOStreams) *cobra.Command {
	o := action.NewListOptions(f, streams, types.RestoreGVR())
	cmd := &cobra.Command{
		Use:               "list-restores",
		Short:             "List restores.",
		Aliases:           []string{"ls-restores"},
		Example:           listRestoreExample,
		ValidArgsFunction: util.ResourceNameCompletionFunc(f, types.ClusterGVR()),
		Run: func(cmd *cobra.Command, args []string) {
			o.LabelSelector = util.BuildLabelSelectorByNames(o.LabelSelector, args)
			cmdutil.BehaviorOnFatal(printer.FatalWithRedColor)
			cmdutil.CheckErr(o.Complete())
			cmdutil.CheckErr(dp.PrintRestoreList(o))
		},
	}
	o.AddFlags(cmd, false)
	cmd.Flags().StringSliceVar(&o.Names, "names", nil, "List restores in the specified cluster")
	return cmd
}

func NewRestoreDescribeCommand(f cmdutil.Factory, streams genericiooptions.IOStreams) *cobra.Command {
	o := &dp.DescribeDPOptions{
		Factory:   f,
		IOStreams: streams,
		Gvr:       types.RestoreGVR(),
	}
	cmd := &cobra.Command{
		Use:               "describe-restore NAME",
		Short:             "Describe a restore",
		Aliases:           []string{"desc-restore"},
		ValidArgsFunction: util.ResourceNameCompletionFunc(f, types.ClusterGVR()),
		Example:           describeRestoreExample,
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.BehaviorOnFatal(printer.FatalWithRedColor)
			util.CheckErr(o.ValidateForClusterCmd(args))
			util.CheckErr(o.Complete())
			util.CheckErr(describeRestores(o, args))
		},
	}
	return cmd
}

func describeRestores(o *dp.DescribeDPOptions, args []string) error {
	if len(o.Names) > 0 {
		return dp.DescribeRestores(o, o.Names)
	}
	restoreList, err := o.GetObjListByArgs(args)
	if err != nil {
		return err
	}
	for _, v := range restoreList.Items {
		obj := &dpv1alpha1.Restore{}
		if err := apiruntime.DefaultUnstructuredConverter.FromUnstructured(v.UnstructuredContent(), obj); err != nil {
			return err
		}
		if err = dp.PrintRestoreDescribe(o, obj); err != nil {
			return err
		}
	}
	return nil
}
