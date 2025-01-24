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
	"fmt"

	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/kubectl/pkg/util/templates"

	dpv1alpha1 "github.com/apecloud/kubeblocks/apis/dataprotection/v1alpha1"

	"github.com/apecloud/kbcli/pkg/action"
	"github.com/apecloud/kbcli/pkg/types"
	"github.com/apecloud/kbcli/pkg/util"
)

var (
	deleteExample = templates.Examples(`
	# Delete a backuprepo
	kbcli backuprepo delete my-backuprepo
	`)
)

func newDeleteCommand(f cmdutil.Factory, streams genericiooptions.IOStreams) *cobra.Command {
	o := action.NewDeleteOptions(f, streams, types.BackupRepoGVR())
	o.PreDeleteHook = preDeleteBackupRepo
	cmd := &cobra.Command{
		Use:               "delete",
		Short:             "Delete a backup repository.",
		Example:           deleteExample,
		ValidArgsFunction: util.ResourceNameCompletionFunc(f, types.BackupRepoGVR()),
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(completeForDeleteBackupRepo(o, args))
			cmdutil.CheckErr(o.Run())
		},
	}
	return cmd
}

func preDeleteBackupRepo(o *action.DeleteOptions, obj runtime.Object) error {
	unstructured := obj.(*unstructured.Unstructured)
	repo := &dpv1alpha1.BackupRepo{}
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(unstructured.Object, repo); err != nil {
		return err
	}

	dynamic, err := o.Factory.DynamicClient()
	if err != nil {
		return err
	}
	backupNum, _, err := countBackupNumsAndSize(dynamic, repo)
	if err != nil {
		return err
	}
	if backupNum > 0 {
		return fmt.Errorf("this backup repository cannot be deleted because it is still containing %d backup(s)", backupNum)
	}
	return nil
}

func completeForDeleteBackupRepo(o *action.DeleteOptions, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("missing backup repository name")
	}
	if len(args) > 1 {
		return fmt.Errorf("can't delete multiple backup repositories at once")
	}
	o.Names = args
	return nil
}
