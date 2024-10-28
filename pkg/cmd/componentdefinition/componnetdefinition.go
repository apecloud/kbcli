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

package componentdefinition

import (
	"fmt"
	"github.com/apecloud/kbcli/pkg/printer"
	v1 "github.com/apecloud/kubeblocks/apis/apps/v1"
	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/kubectl/pkg/util/templates"

	"github.com/apecloud/kbcli/pkg/action"
	"github.com/apecloud/kbcli/pkg/types"
	"github.com/apecloud/kbcli/pkg/util"
)

var listExample = templates.Examples(`
		# list all ComponentDefinitions
		kbcli componentdefinition list
	
		# list all ComponentDefinitions by alias
		kbcli componentdefinition list
`)

func NewComponentDefinitionCmd(f cmdutil.Factory, streams genericiooptions.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "ComponentDefinition",
		Short:   "ComponentDefinition command.",
		Aliases: []string{"componentdefinition"},
	}

	cmd.AddCommand(NewListCmd(f, streams))
	cmd.AddCommand(NewDescribeCmd(f, streams))
	return cmd
}

func NewListCmd(f cmdutil.Factory, streams genericiooptions.IOStreams) *cobra.Command {
	o := action.NewListOptions(f, streams, types.CompDefGVR())
	cmd := &cobra.Command{
		Use:               "list",
		Short:             "List ComponentDefinition.",
		Example:           listExample,
		Aliases:           []string{"ls"},
		ValidArgsFunction: util.ResourceNameCompletionFunc(f, o.GVR),
		Run: func(cmd *cobra.Command, args []string) {
			o.Names = args
			util.CheckErr(cmpdListRun(o))
		},
	}
	o.AddFlags(cmd, true)
	return cmd
}

func cmpdListRun(o *action.ListOptions) error {
	// if format is JSON or YAML, use default printer to output the result.
	if o.Format == printer.JSON || o.Format == printer.YAML {
		_, err := o.Run()
		return err
	}

	// get and output the result
	o.Print = false
	r, err := o.Run()
	if err != nil {
		return err
	}

	infos, err := r.Infos()
	if err != nil {
		return err
	}

	if len(infos) == 0 {
		fmt.Fprintln(o.IOStreams.Out, "No clusterDefinition found")
		return nil
	}

	printRows := func(tbl *printer.TablePrinter) error {
		for _, info := range infos {
			cmpd := &v1.ComponentDefinition{}
			obj := info.Object.(*unstructured.Unstructured)
			if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, cmpd); err != nil {
				return err
			}

			tbl.AddRow(
				cmpd.Name,
				cmpd.Spec.ServiceKind,
				cmpd.Spec.ServiceVersion,
				cmpd.Spec.Provider,
				cmpd.Status.Phase,
				util.TimeFormat(&cmpd.CreationTimestamp),
			)
		}
		return nil
	}

	if err = printer.PrintTable(o.Out, nil, printRows,
		"NAME", "SERVICE", "SERVICE-VERSION", "PROVIDER", "STATUS", "CREATED-TIME"); err != nil {
		return err
	}
	return nil
}
