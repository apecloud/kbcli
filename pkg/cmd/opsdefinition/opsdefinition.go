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

package opsdefinition

import (
	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/kubectl/pkg/util/templates"

	"github.com/apecloud/kbcli/pkg/action"
	"github.com/apecloud/kbcli/pkg/types"
	"github.com/apecloud/kbcli/pkg/util"
)

var listExample = templates.Examples(`
		# list all ops-definitions
		kbcli ops-definition list
	
		# list all ops-definitions by alias
		kbcli ops-def list
`)

func NewOpsDefinitionCmd(f cmdutil.Factory, streams genericiooptions.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "ops-definition",
		Short:   "ops-definitions command.",
		Aliases: []string{"ops-def"},
	}

	cmd.AddCommand(NewListCmd(f, streams))
	cmd.AddCommand(NewDescribeCmd(f, streams))
	return cmd
}

func NewListCmd(f cmdutil.Factory, streams genericiooptions.IOStreams) *cobra.Command {
	o := action.NewListOptions(f, streams, types.OpsDefinitionGVR())
	cmd := &cobra.Command{
		Use:               "list",
		Short:             "List OpsDefinition.",
		Example:           listExample,
		Aliases:           []string{"ls"},
		ValidArgsFunction: util.ResourceNameCompletionFunc(f, o.GVR),
		Run: func(cmd *cobra.Command, args []string) {
			o.Names = args
			_, err := o.Run()
			util.CheckErr(err)
		},
	}
	o.AddFlags(cmd, false)
	return cmd
}
