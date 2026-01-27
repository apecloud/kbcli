/*
Copyright (C) 2022-2026 ApeCloud Co., Ltd

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

package trace

import (
	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/kubectl/pkg/util/templates"

	"github.com/apecloud/kbcli/pkg/action"
	"github.com/apecloud/kbcli/pkg/types"
	"github.com/apecloud/kbcli/pkg/util"
)

var (
	updateExamples = templates.Examples(`
		# update a trace with custom locale, stateEvaluationExpression
		kbcli trace update pg-cluster-trace --locale zh_cn --cel-state-evaluation-expression "has(object.status.phase) && object.status.phase == \"Running\""`)
)

type UpdateOptions struct {
	*action.PatchOptions
	Locale                       string `json:"locale,omitempty"`
	Depth                        int64  `json:"depth,omitempty"`
	CelStateEvaluationExpression string `json:"celStateEvaluationExpression,omitempty"`
}

func (o *UpdateOptions) CmdComplete(cmd *cobra.Command) error {
	if err := o.PatchOptions.CmdComplete(cmd); err != nil {
		return err
	}
	return o.buildPatch()
}

func (o *UpdateOptions) buildPatch() error {
	spec := make(map[string]interface{})
	if o.Depth >= 0 {
		spec["depth"] = o.Depth
	}
	if o.Locale != "" {
		spec["locale"] = o.Locale
	}
	if o.CelStateEvaluationExpression != "" {
		spec["celStateEvaluationExpression"] = o.CelStateEvaluationExpression
	}
	obj := unstructured.Unstructured{
		Object: map[string]interface{}{
			"spec": spec,
		},
	}
	bytes, err := obj.MarshalJSON()
	if err != nil {
		return err
	}
	o.PatchOptions.Patch = string(bytes)
	return nil
}

func newUpdateCmd(f cmdutil.Factory, streams genericiooptions.IOStreams) *cobra.Command {
	o := &UpdateOptions{
		PatchOptions: action.NewPatchOptions(f, streams, types.TraceGVR()),
	}
	cmd := &cobra.Command{
		Use:               "update trace-name",
		Short:             "update a trace.",
		Example:           updateExamples,
		Aliases:           []string{"u"},
		ValidArgsFunction: util.ResourceNameCompletionFunc(f, types.TraceGVR()),
		Run: func(cmd *cobra.Command, args []string) {
			o.PatchOptions.Names = args
			util.CheckErr(o.CmdComplete(cmd))
			util.CheckErr(o.PatchOptions.Run())
		},
	}

	cmd.Flags().StringVar(&o.Locale, "locale", "", "Specify locale.")
	cmd.Flags().Int64Var(&o.Depth, "depth", -1, "Specify object tree depth to display.")
	cmd.Flags().StringVar(&o.CelStateEvaluationExpression, "cel-state-evaluation-expression", "", "Specify CEL state evaluation expression.")

	o.PatchOptions.AddFlags(cmd)

	return cmd
}
