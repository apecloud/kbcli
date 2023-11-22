/*
Copyright (C) 2022-2023 ApeCloud Co., Ltd

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
	"fmt"

	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/kubectl/pkg/util/templates"

	"github.com/apecloud/kbcli/pkg/action"
	"github.com/apecloud/kbcli/pkg/cmd/cluster"
	"github.com/apecloud/kbcli/pkg/printer"
	"github.com/apecloud/kbcli/pkg/types"
	"github.com/apecloud/kbcli/pkg/util"
)

var (
	createRestoreExample = templates.Examples(`
		# restore a new cluster from a backup
		kbcli dp restore mybackup --cluster cluster-name`)
)

func newRestoreCommand(f cmdutil.Factory, streams genericiooptions.IOStreams) *cobra.Command {
	customOutPut := func(opt *action.CreateOptions) {
		output := fmt.Sprintf("Cluster %s created", opt.Name)
		printer.PrintLine(output)
	}

	o := &cluster.CreateRestoreOptions{}
	o.CreateOptions = action.CreateOptions{
		IOStreams:       streams,
		Factory:         f,
		Options:         o,
		GVR:             types.OpsGVR(),
		CueTemplateName: "opsrequest_template.cue",
		CustomOutPut:    customOutPut,
	}

	clusterName := ""

	cmd := &cobra.Command{
		Use:               "restore",
		Short:             "Restore a new cluster from backup",
		ValidArgsFunction: util.ResourceNameCompletionFunc(f, types.BackupGVR()),
		Example:           createRestoreExample,
		Run: func(cmd *cobra.Command, args []string) {
			if len(args) > 0 {
				o.RestoreSpec.BackupName = args[0]
			}
			if clusterName != "" {
				o.Args = []string{clusterName}
			}
			cmdutil.BehaviorOnFatal(printer.FatalWithRedColor)
			util.CheckErr(o.Complete())
			util.CheckErr(o.Validate())
			util.CheckErr(o.Run())
		},
	}

	cmd.Flags().StringVar(&clusterName, "cluster", "", "The cluster to restore")
	cmd.Flags().StringVar(&o.RestoreSpec.RestoreTimeStr, "restore-to-time", "", "point in time recovery(PITR)")
	cmd.Flags().StringVar(&o.RestoreSpec.VolumeRestorePolicy, "volume-restore-policy", "Parallel", "the volume claim restore policy, supported values: [Serial, Parallel]")
	return cmd
}
