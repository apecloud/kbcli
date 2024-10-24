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
	dpv1alpha1 "github.com/apecloud/kubeblocks/apis/dataprotection/v1alpha1"
	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/kubectl/pkg/util/templates"

	"github.com/apecloud/kbcli/pkg/action"
	"github.com/apecloud/kbcli/pkg/printer"
	"github.com/apecloud/kbcli/pkg/types"
	"github.com/apecloud/kbcli/pkg/util"
)

var listActionSetExample = templates.Examples(`
		# list all action sets
		kbcli dp list-as
	`)

func newListActionSetCmd(f cmdutil.Factory, streams genericclioptions.IOStreams) *cobra.Command {
	o := action.NewListOptions(f, streams, types.ActionSetGVR())
	headers := []any{"NAME", "BACKUP-TYPE", "STATUS", "CREATED-TIME"}
	cmd := &cobra.Command{
		Use:               "list-action-set",
		Short:             "List actionsets",
		Aliases:           []string{"list-as"},
		Example:           listActionSetExample,
		ValidArgsFunction: util.ResourceNameCompletionFunc(f, o.GVR),
		Run: func(cmd *cobra.Command, args []string) {
			o.Names = args
			o.AllNamespaces = true
			cmdutil.BehaviorOnFatal(printer.FatalWithRedColor)
			util.CheckErr(o.Complete())
			util.CheckErr(o.PrintObjectList(headers, func(tbl *printer.TablePrinter, unstructuredObj unstructured.Unstructured) error {
				actionSet := &dpv1alpha1.ActionSet{}
				if err := runtime.DefaultUnstructuredConverter.FromUnstructured(unstructuredObj.Object, actionSet); err != nil {
					return err
				}
				tbl.AddRow(actionSet.GetName(), actionSet.Spec.BackupType, string(actionSet.Status.Phase), util.TimeFormat(&actionSet.CreationTimestamp))
				return nil
			}))
		},
	}
	o.AddFlags(cmd, true)
	return cmd
}