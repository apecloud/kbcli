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

package view

import (
	"fmt"

	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/kubectl/pkg/util/templates"

	"github.com/apecloud/kbcli/pkg/action"
	"github.com/apecloud/kbcli/pkg/types"
	"github.com/apecloud/kbcli/pkg/util"
)

var (
	deleteExamples = templates.Examples(`
		# Delete a view
		kbcli view delete pg-cluster`)
)

func newDeleteCmd(f cmdutil.Factory, streams genericiooptions.IOStreams) *cobra.Command {
	o := action.NewDeleteOptions(f, streams, types.ViewGVR())
	cmd := &cobra.Command{
		Use:               "delete view-name",
		Short:             "Delete a view.",
		Example:           deleteExamples,
		ValidArgsFunction: util.ResourceNameCompletionFunc(f, types.ViewGVR()),
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(complete(o, args))
			cmdutil.CheckErr(o.Run())
		},
	}
	return cmd
}

func complete(o *action.DeleteOptions, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("missing view name")
	}
	if len(args) > 1 {
		return fmt.Errorf("can't delete multiple views at once")
	}
	o.Names = args
	return nil
}
