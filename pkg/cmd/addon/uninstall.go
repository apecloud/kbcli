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

package addon

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/kubectl/pkg/util/templates"

	"github.com/apecloud/kbcli/pkg/cluster"
	"github.com/apecloud/kbcli/pkg/types"
	"github.com/apecloud/kbcli/pkg/util"
)

var addonUninstallExample = templates.Examples(`
	# uninstall an addon 
	kbcli addon uninstall apecloud-mysql 

	# uninstall more than one addons
	kbcli addon uninstall apecloud-mysql postgresql
`)

type uninstallOption struct {
	*baseOption
	// addon names
	names       []string
	autoApprove bool
}

func newUninstallOption(f cmdutil.Factory, streams genericiooptions.IOStreams) *uninstallOption {
	return &uninstallOption{
		baseOption: &baseOption{
			Factory:   f,
			IOStreams: streams,
			GVR:       types.AddonGVR(),
		},
	}
}

func newUninstallCmd(f cmdutil.Factory, streams genericiooptions.IOStreams) *cobra.Command {
	o := newUninstallOption(f, streams)
	cmd := &cobra.Command{
		Use:               "uninstall",
		Short:             "Uninstall an existed addon",
		Example:           addonUninstallExample,
		ValidArgsFunction: util.ResourceNameCompletionFunc(f, o.GVR),
		Run: func(cmd *cobra.Command, args []string) {
			o.names = args
			util.CheckErr(o.baseOption.complete())
			util.CheckErr(o.checkBeforeUninstall())
			util.CheckErr(o.Run())
		},
	}
	cmd.Flags().BoolVar(&o.autoApprove, "auto-approve", false, "Skip interactive approval before uninstalling addon")
	return cmd
}

func (o *uninstallOption) Run() error {
	for _, name := range o.names {
		err := o.Dynamic.Resource(o.GVR).Delete(context.Background(), name, metav1.DeleteOptions{})
		if err != nil {
			return err
		}
		fmt.Fprintf(o.Out, "addon %s uninstalled successfully\n", name)
		cluster.ClearCharts(cluster.ClusterType(name))
		fmt.Fprintf(o.Out, "cluster chart removed successfully\n")
	}
	return nil
}

func (o *uninstallOption) checkBeforeUninstall() error {
	if o.autoApprove {
		return nil
	}
	return CheckAddonUsedByCluster(o.Dynamic, o.names, o.In)
}
