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
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Masterminds/semver/v3"
	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/kubectl/pkg/util/templates"

	extensionsv1alpha1 "github.com/apecloud/kubeblocks/apis/extensions/v1alpha1"

	clusterCmd "github.com/apecloud/kbcli/pkg/cmd/cluster"
	"github.com/apecloud/kbcli/pkg/printer"
	"github.com/apecloud/kbcli/pkg/types"
	"github.com/apecloud/kbcli/pkg/util"
)

var addonInstallExample = templates.Examples(`
	# install an addon from default index
	kbcli addon install apecloud-mysql 

	# install an addon from default index and skip KubeBlocks version compatibility check
	kbcli addon install apecloud-mysql --force

	# install an addon from a specified index
	kbcli addon install apecloud-mysql --index my-index

	# install an addon with a specified version default index
	kbcli addon install apecloud-mysql --version 0.7.0

	# install an addon with a specified version and cluster chart of different version.
	kbcli addon install apecloud-mysql --version 0.7.0 --cluster-chart-version 0.7.1

	# install an addon with a specified version and local path.
	kbcli addon install apecloud-mysql --version 0.7.0 --path /path/to/local/chart
`)

type baseOption struct {
	Factory cmdutil.Factory
	genericiooptions.IOStreams

	Dynamic dynamic.Interface
	Client  kubernetes.Interface

	// GVR is the GroupVersionResource of the resource to be created
	GVR schema.GroupVersionResource
}

func (o *baseOption) complete() error {
	var err error
	if o.Dynamic, err = o.Factory.DynamicClient(); err != nil {
		return err
	}

	if o.Client, err = o.Factory.KubernetesClientSet(); err != nil {
		return err
	}
	return nil
}

type installOption struct {
	baseOption
	// addon name
	name string
	// install the addon force
	force bool
	// the addon version we want to install
	version string
	// the index name, if not specified use `kubeblocks` default
	index string
	// the version of cluster chart to register, if not specified use the same version as the addon by default
	clusterChartVersion string
	// the repo url of cluster charts, if not specified use 'kubeblocks-addons' by default
	clusterChartRepo string
	// the local path contains addon CRs and needs to be specified when operating offline
	path string

	addon *extensionsv1alpha1.Addon
}

func newInstallOption(f cmdutil.Factory, streams genericiooptions.IOStreams) *installOption {
	return &installOption{
		baseOption: baseOption{
			Factory:   f,
			IOStreams: streams,
			GVR:       types.AddonGVR(),
		},
		force: false,
		index: types.DefaultIndexName,
	}
}

func newInstallCmd(f cmdutil.Factory, streams genericiooptions.IOStreams) *cobra.Command {
	o := newInstallOption(f, streams)
	cmd := &cobra.Command{
		Use:     "install",
		Short:   "Install KubeBlocks addon",
		Args:    cobra.ExactArgs(1),
		Example: addonInstallExample,
		PersistentPreRun: func(_ *cobra.Command, _ []string) {
			util.CheckErr(addDefaultIndex())
		},
		ValidArgsFunction: addonNameCompletionFunc,
		Run: func(cmd *cobra.Command, args []string) {
			o.name = args[0]
			util.CheckErr(o.Complete())
			util.CheckErr(o.Validate())
			util.CheckErr(o.Run(f, streams))
			// avoid unnecessary messages for upgrade
			fmt.Fprintf(o.Out, "addon %s installed successfully\n", o.name)
			if err := clusterCmd.RegisterClusterChart(f, streams, "", o.name, o.clusterChartVersion, o.clusterChartRepo); err != nil {
				util.CheckErr(err)
			}
		},
	}
	cmd.Flags().BoolVar(&o.force, "force", false, "force install the addon and ignore the version check")
	cmd.Flags().StringVar(&o.version, "version", "", "specify the addon version to install, run 'kbcli addon search <addon-name>' to get the available versions")
	cmd.Flags().StringVar(&o.index, "index", types.DefaultIndexName, "specify the addon index, use 'kubeblocks' by default")
	cmd.Flags().StringVar(&o.clusterChartVersion, "cluster-chart-version", "", "specify the cluster chart version, use the same version as the addon by default")
	cmd.Flags().StringVar(&o.clusterChartRepo, "cluster-chart-repo", types.ClusterChartsRepoURL, "specify the repo of cluster chart, use the url of 'kubeblocks-addons' by default")
	cmd.Flags().StringVar(&o.path, "path", "", "specify the local path contains addon CRs and needs to be specified when operating offline")

	_ = cmd.MarkFlagRequired("version")

	return cmd
}

// Complete will finalize the basic K8s client configuration and find the corresponding addon from the index
func (o *installOption) Complete() error {
	var (
		err    error
		addons []searchResult
	)

	if err = o.baseOption.complete(); err != nil {
		return err
	}

	if o.version == "" {
		return fmt.Errorf("please specify the version, run 'kbcli addon search %s' to get the available versions", o.name)
	}

	// search specified addon and match its index
	if _, err = semver.NewVersion(o.version); err != nil && o.version != "" {
		return fmt.Errorf("the version %s does not comply with the standards", o.version)
	}

	// If no local path is specified, we search all indexes in the default index directory.
	// When a local path is specified, we assume the last part of the path is the index and search only that.
	if o.path == "" {
		dir, err := util.GetCliAddonDir()
		if err != nil {
			return err
		}
		if addons, err = searchAddon(o.name, dir, ""); err != nil {
			return err
		}
	} else {
		if addons, err = searchAddon(o.name, filepath.Dir(o.path), filepath.Base(o.path)); err != nil {
			return err
		}
	}

	sort.Slice(addons, func(i, j int) bool {
		vi, _ := semver.NewVersion(getAddonVersion(addons[i].addon))
		vj, _ := semver.NewVersion(getAddonVersion(addons[j].addon))
		if vi == nil || vj == nil {
			return false
		}
		return vi.GreaterThan(vj)
	})

	// descending order of versions
	for _, item := range addons {
		if o.path != "" || item.index.name == o.index {
			if o.version == getAddonVersion(item.addon) {
				o.addon = item.addon
				break
			}
		}
	}

	// complete the version of the cluster chart
	if o.clusterChartVersion == "" {
		o.clusterChartVersion = o.version
	}

	if o.addon == nil {
		var addonInfo = o.name
		addonInfo += "-" + o.version
		return fmt.Errorf("addon '%s' not found in the index '%s'", addonInfo, o.index)
	}
	return nil
}

// Validate will check if the KubeBlocks environment meets the requirements that the installing addon need
func (o *installOption) Validate() error {
	var (
		err error
		ok  bool
	)

	v, err := util.GetVersionInfo(o.Client)
	if err != nil {
		return err
	}
	if len(v.KubeBlocks) == 0 {
		return fmt.Errorf("KubeBlocks is not yet installed，please install it first")
	}
	if o.force {
		fmt.Fprint(o.Out, printer.BoldYellow("Warning: --force flag will skip version checks, which may result in the cluster not running correctly.\n"))
		return nil
	}
	if o.addon.Annotations == nil || len(o.addon.Annotations[types.KBVersionValidateAnnotationKey]) == 0 {
		fmt.Fprint(o.Out, printer.BoldYellow(fmt.Sprintf(`Warning: The addon %s is missing annotations to validate KubeBlocks versions.
It will automatically skip version checks, which may result in the cluster not running correctly.
`, o.name)))
	} else {
		kbVersions := strings.Split(v.KubeBlocks, ",")
		for _, kbVersion := range kbVersions {
			if ok, err = validateVersion(o.addon.Annotations[types.KBVersionValidateAnnotationKey], kbVersion); err == nil && !ok {
				return fmt.Errorf("KubeBlocks version %s does not meet the requirements \"%s\" for addon installation\nUse --force option to skip this check", v.KubeBlocks, o.addon.Annotations[types.KBVersionValidateAnnotationKey])
			}
		}
	}
	return err
}

// Run will apply the addon.yaml to K8s
func (o *installOption) Run(f cmdutil.Factory, streams genericiooptions.IOStreams) error {
	item, err := runtime.DefaultUnstructuredConverter.ToUnstructured(o.addon)
	if err != nil {
		return err
	}
	_, err = o.Dynamic.Resource(o.GVR).Create(context.Background(), &unstructured.Unstructured{Object: item}, metav1.CreateOptions{})
	if err != nil {
		return err
	}
	return nil
}

// validateVersion will check if the kbVersion meets the version constraint defined by annotations
// rules：
// 1.0.0-alpha < 1.0.0-alpha.1 < 1.0.0-alpha.beta < 1.0.0-beta < 1.0.0-beta.2 < 1.0.0-beta.11 < 1.0.0-rc.1 < 1.0.0.
// https://semver.org/
func validateVersion(annotations, kbVersion string) (bool, error) {
	// if kb version is a pre-release version, we will break the rules for developing
	if strings.Contains(kbVersion, "-") {
		addPreReleaseInfo := func(constrain string) string {
			constrain = strings.Trim(constrain, " ")
			split := strings.Split(constrain, "-")
			// adjust '>= 0.7.0' to '>= 0.7.0-0'
			// https://github.com/Masterminds/semver?tab=readme-ov-file#checking-version-constraints
			if len(split) == 1 && (strings.HasPrefix(constrain, ">") || strings.Contains(constrain, "<")) {
				constrain += "-0"
			}
			return constrain
		}
		rules := strings.Split(annotations, ",")
		for i := range rules {
			rules[i] = addPreReleaseInfo(rules[i])
		}
		annotations = strings.Join(rules, ",")
	}
	constraint, err := semver.NewConstraint(annotations)
	if err != nil {
		return false, err
	}
	v, err := semver.NewVersion(kbVersion)
	if err != nil {
		return false, err
	}
	validate, _ := constraint.Validate(v)
	return validate, nil
}
