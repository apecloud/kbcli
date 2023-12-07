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

package addon

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/kubectl/pkg/util/templates"

	"github.com/apecloud/kbcli/pkg/types"
	"github.com/apecloud/kbcli/pkg/util"
)

var addonUninstallExample = templates.Examples(`
	# uninstall an addon 
	kbcli addon uninstall apecloud-mysql 

	# uninstall an addon with a specified version
	kbcli addon uninstall apecloud-mysql --version 0.7.0
`)

type uninstallOption struct {
	*baseOption
	// addon name
	name string
	// the version
	version string
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
		Short:             "uninstall an existed addon",
		Args:              cobra.ExactArgs(1),
		Example:           addonUninstallExample,
		ValidArgsFunction: util.ResourceNameCompletionFunc(f, o.GVR),
		Run: func(cmd *cobra.Command, args []string) {
			o.name = args[0]
			util.CheckErr(o.baseOption.complete())
			// util.CheckErr(o.Validate())
			util.CheckErr(o.Run())
		},
	}

	cmd.Flags().StringVar(&o.version, "version", "", "must be specified if the addon owns multiple version")
	return cmd
}

func (o *uninstallOption) Run() error {
	err := o.Dynamic.Resource(o.GVR).Delete(context.Background(), o.name, metav1.DeleteOptions{})
	if err != nil {
		return err
	}
	fmt.Fprintf(o.Out, "%s uninstall succssed", o.name)
	return nil
}
