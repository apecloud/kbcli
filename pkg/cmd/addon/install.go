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
	"github.com/apecloud/kubeblocks/pkg/constant"
	"sort"

	"github.com/Masterminds/semver/v3"
	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"

	extensionsv1alpha1 "github.com/apecloud/kubeblocks/apis/extensions/v1alpha1"

	"github.com/apecloud/kbcli/pkg/printer"
	"github.com/apecloud/kbcli/pkg/types"
	"github.com/apecloud/kbcli/pkg/util"
)

type baseOption struct {
	Factory cmdutil.Factory
	genericiooptions.IOStreams

	Dynamic dynamic.Interface
	Client  kubernetes.Interface

	// GVR is the GroupVersionResource of the resource to be created
	GVR schema.GroupVersionResource

	nameSpace string
}

func (o *baseOption) complete() error {
	var err error
	if o.nameSpace, _, err = o.Factory.ToRawKubeConfigLoader().Namespace(); err != nil {
		return err
	}
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
	// the version w
	version string
	// the source index name, if not specified use `kubeblocks` default
	source string

	addon *extensionsv1alpha1.Addon
}

func newInstallOption(f cmdutil.Factory, streams genericiooptions.IOStreams) *installOption {
	return &installOption{
		baseOption: baseOption{
			Factory:   f,
			IOStreams: streams,
			GVR:       types.AddonGVR(),
		},
		force:  false,
		source: types.DefaultIndexName,
	}
}

func (o *installOption) complete() error {
	var err error
	if err = o.baseOption.complete(); err != nil {
		return err
	}
	// search specified addon and match its index
	dir, err := util.GetCliAddonDir()
	if err != nil {
		return err
	}
	addons, err := searchAddon(o.name, dir)
	if err != nil {
		return err
	}

	getVersion := func(item *extensionsv1alpha1.Addon) string {
		if item.Labels == nil {
			return ""
		}
		return item.Labels[constant.AppVersionLabelKey]
	}

	sort.Slice(addons, func(i, j int) bool {
		vi, _ := semver.NewVersion(getVersion(addons[i].addon))
		vj, _ := semver.NewVersion(getVersion(addons[j].addon))
		return vi.LessThan(vj)

	})
	// descending order of versions
	for _, item := range addons {
		if item.index.name == o.source {
			// if the version not specified, use the latest version
			if o.version == "" {
				o.addon = item.addon
				break
			} else if o.version == getVersion(item.addon) {
				o.addon = item.addon
				break
			}
		}
	}
	if o.addon == nil {
		return fmt.Errorf("addon '%s' not found in the index '%s'", o.name, o.source)
	}
	return nil
}

func newInstallCmd(f cmdutil.Factory, streams genericiooptions.IOStreams) *cobra.Command {
	o := newInstallOption(f, streams)
	cmd := &cobra.Command{
		Use:   "install",
		Short: "install KubeBlocks addon",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			o.name = args[0]
			util.CheckErr(o.complete())
			util.CheckErr(o.validate())
			util.CheckErr(o.run())
		},
	}

	return cmd
}

func (o *installOption) validate() error {
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
		fmt.Fprintf(o.Out, printer.BoldYellow(fmt.Sprintf("Warning: --force flag will skip version checks, which may result in the cluster not running correctly.")))
		return nil
	}

	if o.addon.Annotations == nil || len(o.addon.Annotations[types.KBVersionValidateAnnotationKey]) == 0 {
		fmt.Fprintf(o.Out, printer.BoldYellow(fmt.Sprintf(`Warning: The addon %s is missing annotations to validate KubeBlocks versions.
It will automatically skip version checks, which may result in the cluster not running correctly.`, o.name)))
	} else if ok, err = validateVersion(o.addon.Annotations[types.KBVersionValidateAnnotationKey], v.KubeBlocks); err == nil && !ok {
		return fmt.Errorf("KubeBlocks version %s does not meet the requirements for addon installation", v.KubeBlocks)
	}

	return err

}

func (o *installOption) run() error {
	item, err := runtime.DefaultUnstructuredConverter.ToUnstructured(o.addon)
	if err != nil {
		return err
	}
	_, err = o.Dynamic.Resource(o.GVR).Create(context.Background(), &unstructured.Unstructured{Object: item}, metav1.CreateOptions{})
	if err != nil {
		return err
	}
	fmt.Fprintf(o.Out, "%s install successed", o.name)
	return nil
}

func validateVersion(annotations, KBVersion string) (bool, error) {
	constraint, err := semver.NewConstraint(annotations)
	if err != nil {
		return false, err
	}
	v, err := semver.NewVersion(KBVersion)
	if err != nil {
		return false, err
	}
	validate, errors := constraint.Validate(v)
	if len(errors) != 0 {
		return false, utilerrors.NewAggregate(errors)
	}
	return validate, nil
}
