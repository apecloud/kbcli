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
	"fmt"

	"github.com/Masterminds/semver/v3"
	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/kubectl/pkg/util/templates"

	extensionsv1alpha1 "github.com/apecloud/kubeblocks/apis/extensions/v1alpha1"
	"github.com/apecloud/kubeblocks/pkg/constant"

	"github.com/apecloud/kbcli/pkg/cluster"
	"github.com/apecloud/kbcli/pkg/types"
	"github.com/apecloud/kbcli/pkg/util"
)

var addonUpgradeExample = templates.Examples(`
	# upgrade an addon from default index to latest version
	kbcli addon upgrade apecloud-mysql 

	# upgrade an addon from default index to latest version and skip KubeBlocks version compatibility check
	kbcli addon upgrade apecloud-mysql --force

	# upgrade an addon to latest version from a specified index
	kbcli addon upgrade apecloud-mysql --source my-index

	# upgrade an addon with a specified version default index 
	kbcli addon upgrade apecloud-mysql --version 0.7.0
`)

// upgradeOption storage the info to upgrade an addon
type upgradeOption struct {
	*installOption
	// currentVersion is the addon current version in KubeBlocks
	currentVersion string
}

func newUpgradeOption(f cmdutil.Factory, streams genericiooptions.IOStreams) *upgradeOption {
	return &upgradeOption{
		installOption:  newInstallOption(f, streams),
		currentVersion: "",
	}
}

func newUpgradeCmd(f cmdutil.Factory, streams genericiooptions.IOStreams) *cobra.Command {
	o := newUpgradeOption(f, streams)
	cmd := &cobra.Command{
		Use:     "upgrade",
		Short:   "Upgrade an existed addon to latest version or a specified version",
		Args:    cobra.ExactArgs(1),
		Example: addonUpgradeExample,
		Run: func(cmd *cobra.Command, args []string) {
			o.name = args[0]
			util.CheckErr(o.Complete())
			util.CheckErr(o.Validate())
			util.CheckErr(o.Run())
		},
	}
	cmd.Flags().BoolVar(&o.force, "force", false, "force upgrade the addon and ignore the version check")
	cmd.Flags().StringVar(&o.version, "version", "", "specify the addon version")
	cmd.Flags().StringVar(&o.source, "source", types.DefaultIndexName, "specify the addon index source, use 'kubeblocks' by default")
	return cmd
}

func (o *upgradeOption) Complete() error {
	if err := o.installOption.Complete(); err != nil {
		return err
	}
	addon := extensionsv1alpha1.Addon{}
	err := cluster.GetK8SClientObject(o.Dynamic, &addon, types.AddonGVR(), o.nameSpace, o.name)
	if err != nil {
		return fmt.Errorf("addon %s not found. please use 'kbcli addon install %s' first", o.name, o.name)
	}
	o.currentVersion = addon.Labels[constant.AppVersionLabelKey]
	return nil
}

// Validate will check if the current version is already the latest version compared to installOption.Validate()
func (o *upgradeOption) Validate() error {
	targetV := o.addon.Labels[constant.AppVersionLabelKey]
	target, err := semver.NewVersion(targetV)
	if err != nil {
		return err
	}
	current, err := semver.NewVersion(o.currentVersion)
	if err != nil {
		return err
	}
	if !target.GreaterThan(current) {
		return fmt.Errorf(`addon %s current version %s is either the latest or newer than the expected version %s. you can use 'kbcli addon index update' first and try upgrade again`, o.name, o.currentVersion, o.version)
	}
	return o.installOption.Validate()
}
