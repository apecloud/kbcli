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
	"k8s.io/cli-runtime/pkg/genericiooptions"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/kubectl/pkg/util/templates"

	"github.com/apecloud/kbcli/pkg/action"
	"github.com/apecloud/kbcli/pkg/types"
	"github.com/apecloud/kbcli/pkg/util"
)

const (
	CueTemplateName = "trace_template.cue"
)

var (
	createExamples = templates.Examples(`
		# create a trace for cluster has the same name 'pg-cluster'
		kbcli trace create pg-cluster

		# create a trace for cluster has the name of 'pg-cluster'
		kbcli trace create pg-cluster-trace --cluster-name pg-cluster

		# create a trace with custom locale, stateEvaluationExpression
		kbcli trace create pg-cluster-trace --locale zh_cn --cel-state-evaluation-expression "has(object.status.phase) && object.status.phase == \"Running\""`)
)

type CreateOptions struct {
	action.CreateOptions         `json:"-"`
	ClusterName                  string `json:"clusterName,omitempty"`
	Locale                       string `json:"locale,omitempty"`
	Depth                        int64  `json:"depth,omitempty"`
	CelStateEvaluationExpression string `json:"celStateEvaluationExpression,omitempty"`
}

func (o *CreateOptions) Complete() error {
	o.CreateOptions.Options = o
	return o.CreateOptions.Complete()
}

func (o *CreateOptions) Run() error {
	return o.CreateOptions.Run()
}

func newCreateCmd(f cmdutil.Factory, streams genericiooptions.IOStreams) *cobra.Command {
	o := &CreateOptions{
		CreateOptions: action.CreateOptions{
			Factory:         f,
			IOStreams:       streams,
			CueTemplateName: CueTemplateName,
			GVR:             types.TraceGVR(),
		},
	}
	cmd := &cobra.Command{
		Use:               "create trace-name",
		Short:             "create a trace.",
		Example:           createExamples,
		Aliases:           []string{"c"},
		ValidArgsFunction: util.ResourceNameCompletionFunc(f, types.TraceGVR()),
		Run: func(cmd *cobra.Command, args []string) {
			o.CreateOptions.Args = args
			util.CheckErr(o.Complete())
			util.CheckErr(o.Run())
		},
	}

	cmd.Flags().StringVar(&o.ClusterName, "cluster-name", "", "Specify target cluster name.")
	cmd.Flags().StringVar(&o.Locale, "locale", "", "Specify locale.")
	cmd.Flags().Int64Var(&o.Depth, "depth", 0, "Specify object tree depth to display.")
	cmd.Flags().StringVar(&o.CelStateEvaluationExpression, "cel-state-evaluation-expression", "", "Specify CEL state evaluation expression.")

	return cmd
}
