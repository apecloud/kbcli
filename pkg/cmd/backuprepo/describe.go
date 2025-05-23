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

package backuprepo

import (
	"context"
	"fmt"
	"strings"

	"github.com/dustin/go-humanize"
	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	"k8s.io/client-go/dynamic"
	clientset "k8s.io/client-go/kubernetes"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/kubectl/pkg/util/templates"

	dpv1alpha1 "github.com/apecloud/kubeblocks/apis/dataprotection/v1alpha1"
	"github.com/apecloud/kubeblocks/pkg/dataprotection/utils"

	"github.com/apecloud/kbcli/pkg/printer"
	"github.com/apecloud/kbcli/pkg/types"
	"github.com/apecloud/kbcli/pkg/util"
)

var (
	describeExample = templates.Examples(`
	# Describe a backuprepo
	kbcli backuprepo describe my-backuprepo 
	`)
)

type describeBackupRepoOptions struct {
	factory   cmdutil.Factory
	client    clientset.Interface
	dynamic   dynamic.Interface
	namespace string

	// resource type and names
	gvr   schema.GroupVersionResource
	names []string

	genericiooptions.IOStreams
}

func newDescribeCommand(f cmdutil.Factory, streams genericiooptions.IOStreams) *cobra.Command {
	o := &describeBackupRepoOptions{
		factory:   f,
		IOStreams: streams,
		gvr:       types.BackupRepoGVR(),
	}
	cmd := &cobra.Command{
		Use:               "describe",
		Short:             "Describe a backup repository.",
		Example:           describeExample,
		ValidArgsFunction: util.ResourceNameCompletionFunc(f, types.BackupRepoGVR()),
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.BehaviorOnFatal(printer.FatalWithRedColor)
			cmdutil.CheckErr(o.complete(args))
			cmdutil.CheckErr(o.run())
		},
	}
	return cmd
}

func (o *describeBackupRepoOptions) complete(args []string) error {
	var err error

	if len(args) == 0 {
		return fmt.Errorf("must specify a backuprepo name")
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

func (o *describeBackupRepoOptions) run() error {
	var backupRepoNameMap = make(map[string]bool)
	for _, name := range o.names {
		backupRepoNameMap[name] = true
	}

	for _, name := range o.names {
		backupRepoObj, err := o.dynamic.Resource(types.BackupRepoGVR()).Get(context.TODO(), name, metav1.GetOptions{})
		if err != nil {
			return err
		}
		backupRepo := &dpv1alpha1.BackupRepo{}
		if err = runtime.DefaultUnstructuredConverter.FromUnstructured(backupRepoObj.Object, backupRepo); err != nil {
			return err
		}
		if err = o.printBackupRepo(backupRepo); err != nil {
			return err
		}
	}

	return nil
}

func (o *describeBackupRepoOptions) printBackupRepo(backupRepo *dpv1alpha1.BackupRepo) error {
	printer.PrintLine("Summary:")
	printer.PrintPairStringToLine("Name", backupRepo.Name)
	printer.PrintPairStringToLine("Provider", backupRepo.Spec.StorageProviderRef)
	backups, backupSize, err := countBackupNumsAndSize(o.dynamic, backupRepo)
	if err != nil {
		return err
	}
	printer.PrintPairStringToLine("Backups", fmt.Sprintf("%d", backups))
	printer.PrintPairStringToLine("Total Data Size", backupSize)

	printer.PrintLine("\nSpec:")
	printer.PrintPairStringToLine("AccessMethod", string(backupRepo.Spec.AccessMethod))
	printer.PrintPairStringToLine("PvReclaimPolicy", string(backupRepo.Spec.PVReclaimPolicy))
	printer.PrintPairStringToLine("StorageProviderRef", backupRepo.Spec.StorageProviderRef)
	printer.PrintPairStringToLine("VolumeCapacity", backupRepo.Spec.VolumeCapacity.String())
	printer.PrintLine("  Config:")
	for k, v := range backupRepo.Spec.Config {
		printer.PrintPairStringToLine(k, v, 4)
	}

	printer.PrintLine("\nStatus:")
	printer.PrintPairStringToLine("Phase", string(backupRepo.Status.Phase))
	printer.PrintPairStringToLine("BackupPVCName", backupRepo.Status.BackupPVCName)
	printer.PrintPairStringToLine("ObservedGeneration", fmt.Sprintf("%d", backupRepo.Status.ObservedGeneration))

	printer.PrintLine("\n  Conditions:")
	for _, cond := range backupRepo.Status.Conditions {
		printer.PrintLine("    " + cond.Type + ":")
		printer.PrintPairStringToLine("    Status", string(cond.Status))
		printer.PrintPairStringToLine("    Reason", cond.Reason)
		if !strings.Contains(cond.Message, "\n") {
			if cond.Message != "" {
				printer.PrintPairStringToLine("    Message", cond.Message)
			}
		} else {
			printer.PrintLine("      Message:")
			printer.PrintLine(utils.PrependSpaces(cond.Message, 8))
		}
	}

	return nil
}

func countBackupNumsAndSize(dynamic dynamic.Interface, backupRepo *dpv1alpha1.BackupRepo) (int, string, error) {
	var size uint64
	count := 0

	backupList, err := dynamic.Resource(types.BackupGVR()).List(context.TODO(), metav1.ListOptions{
		LabelSelector: fmt.Sprintf("%s=%s", associatedBackupRepoKey, backupRepo.Name),
	})
	if err != nil {
		return count, humanize.Bytes(size), err
	}
	count = len(backupList.Items)

	for _, obj := range backupList.Items {
		backup := &dpv1alpha1.Backup{}
		if err = runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, backup); err != nil {
			return count, humanize.Bytes(size), err
		}
		// if backup doesn't complete, we don't count it's size
		if backup.Status.Phase != dpv1alpha1.BackupPhaseCompleted {
			continue
		}
		backupSize, err := humanize.ParseBytes(backup.Status.TotalSize)
		if err != nil {
			return count, humanize.Bytes(size), fmt.Errorf("failed to parse the %s of totalSize, %s, %s", backup.Name, backup.Status.TotalSize, err)
		}
		size += backupSize
	}
	return count, humanize.Bytes(size), nil
}
