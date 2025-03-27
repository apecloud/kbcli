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

package addon

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Masterminds/semver/v3"
	extensionsv1alpha1 "github.com/apecloud/kubeblocks/apis/extensions/v1alpha1"
	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ktypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/kubectl/pkg/util/templates"

	"github.com/apecloud/kbcli/pkg/printer"
	"github.com/apecloud/kbcli/pkg/types"
	"github.com/apecloud/kbcli/pkg/util"
)

var addonUpgradeExample = templates.Examples(`
	# upgrade an addon from default index to latest version
	kbcli addon upgrade apecloud-mysql 

	# upgrade an addon from default index to latest version and skip KubeBlocks version compatibility check
	kbcli addon upgrade apecloud-mysql --force

	# upgrade an addon to latest version from a specified index
	kbcli addon upgrade apecloud-mysql --index my-index

	# upgrade an addon with a specified version default index 
	kbcli addon upgrade apecloud-mysql --version 0.7.0

	# upgrade an addon with a specified version, default index and a different version of cluster chart
	kbcli addon upgrade apecloud-mysql --version 0.7.0 --cluster-chart-version 0.7.1

	# non-inplace upgrade an addon with a specified version
	kbcli addon upgrade apecloud-mysql  --inplace=false --version 0.7.0

	# non-inplace upgrade an addon with a specified addon name
	kbcli addon upgrade apecloud-mysql --inplace=false --name apecloud-mysql-0.7.0
`)

// upgradeOption storage the info to upgrade an addon
type upgradeOption struct {
	*installOption

	// currentVersion is the addon current version in KubeBlocks
	currentVersion string
	// if inplace is false will retain the existing addon and reinstall the new version of the addon.
	// otherwise the upgrade will be in-place. It's true in default
	inplace bool
	// rename is the new version addon name need to set by user when inplace is false, it also will be used as resourceNamePrefix of an addon with multiple version.
	// If it's not be specified by user, use `addon-version` by default
	rename string
}

func newUpgradeOption(f cmdutil.Factory, streams genericiooptions.IOStreams) *upgradeOption {
	return &upgradeOption{
		installOption:  newInstallOption(f, streams),
		currentVersion: "",
		inplace:        true,
		rename:         "",
	}
}

func newUpgradeCmd(f cmdutil.Factory, streams genericiooptions.IOStreams) *cobra.Command {
	o := newUpgradeOption(f, streams)
	cmd := &cobra.Command{
		Use:               "upgrade",
		Short:             "Upgrade an existed addon to latest version or a specified version",
		Args:              cobra.ExactArgs(1),
		Example:           addonUpgradeExample,
		ValidArgsFunction: util.ResourceNameCompletionFunc(f, o.GVR),
		Run: func(cmd *cobra.Command, args []string) {
			o.name = args[0]
			util.CheckErr(o.Complete())
			util.CheckErr(o.Validate())
			if strings.HasPrefix(o.currentVersion, "0.9") {
				util.CheckErr(o.process09ClusterDefAndComponentVersions())
			}
			util.CheckErr(o.Run(f, streams))
		},
	}
	cmd.Flags().BoolVar(&o.force, "force", false, "force upgrade the addon and ignore the version check")
	cmd.Flags().StringVar(&o.version, "version", "", "specify the addon version")
	cmd.Flags().StringVar(&o.index, "index", types.DefaultIndexName, "specify the addon index index, use 'kubeblocks' by default")
	cmd.Flags().BoolVar(&o.inplace, "inplace", true, "when inplace is false, it will retain the existing addon and reinstall the new version of the addon, otherwise the upgrade will be in-place. The default is true.")
	cmd.Flags().StringVar(&o.rename, "name", "", "name is the new version addon name need to set by user when inplace is false, it also will be used as resourceNamePrefix of an addon with multiple version.")
	cmd.Flags().StringVar(&o.clusterChartVersion, "cluster-chart-version", "", "specify the cluster chart version, use the same version as the addon by default")
	cmd.Flags().StringVar(&o.clusterChartRepo, "cluster-chart-repo", types.ClusterChartsRepoURL, "specify the repo of cluster chart, use the url of 'kubeblocks-addons' by default")
	cmd.Flags().StringVar(&o.path, "path", "", "specify the local path contains addon CRs and needs to be specified when operating offline")
	return cmd
}

func (o *upgradeOption) Complete() error {
	if err := o.installOption.Complete(); err != nil {
		return err
	}
	addon := extensionsv1alpha1.Addon{}
	err := util.GetK8SClientObject(o.Dynamic, &addon, o.GVR, "", o.name)
	if err != nil {
		return fmt.Errorf("addon %s not found. please use 'kbcli addon install %s' first", o.name, o.name)
	}
	o.currentVersion = getAddonVersion(&addon)
	return nil
}

// Validate will check if the current version is already the latest version compared to installOption.Validate()
func (o *upgradeOption) Validate() error {
	if o.version == "" {
		o.version = getAddonVersion(o.addon)
	}
	if !o.inplace && o.rename == "" {
		o.rename = fmt.Sprintf("%s-%s", o.name, o.version)
		fmt.Printf("--name is not specified by user when upgrade is non-inplace, use \"%s\" by default\n", o.rename)
	}
	target, err := semver.NewVersion(o.version)
	if err != nil {
		return err
	}
	current, err := semver.NewVersion(o.currentVersion)
	if err != nil {
		return err
	}
	if !target.GreaterThan(current) {
		fmt.Printf("%s addon %s current version %s is either the latest or newer than the expected version %s.\n", printer.BoldYellow("Warn:"), o.name, o.currentVersion, o.version)
	}
	return o.installOption.Validate()
}

func (o *upgradeOption) Run(f cmdutil.Factory, streams genericiooptions.IOStreams) error {
	if !o.inplace {
		if o.addon.Spec.Helm.InstallValues.SetValues != nil {
			o.addon.Spec.Helm.InstallValues.SetValues = append(o.addon.Spec.Helm.InstallValues.SetValues, fmt.Sprintf("%s=%s", types.AddonResourceNamePrefix, o.rename))
		}
		o.addon.Spec.Helm.InstallValues.SetValues = []string{fmt.Sprintf("%s=%s", types.AddonResourceNamePrefix, o.rename)}
		o.addon.Name = o.rename
		err := o.installOption.Run(f, streams)
		if err == nil {
			fmt.Printf("Addon %s-%s upgrade successed.\n", o.rename, o.version)
		}
		return err
	}
	// in-place upgrade
	newData, err := json.Marshal(o.addon)
	if err != nil {
		return err
	}
	_, err = o.Dynamic.Resource(o.GVR).Patch(context.Background(), o.name, ktypes.MergePatchType, newData, metav1.PatchOptions{})
	if err == nil {
		fmt.Printf("Addon %s-%s upgrade successed.\n", o.name, o.version)
	}
	return err
}
