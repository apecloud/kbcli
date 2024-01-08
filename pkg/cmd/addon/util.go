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

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"

	extensionsv1alpha1 "github.com/apecloud/kubeblocks/apis/extensions/v1alpha1"
	"github.com/apecloud/kubeblocks/pkg/constant"

	"github.com/apecloud/kbcli/pkg/printer"
	"github.com/apecloud/kbcli/pkg/types"
	"github.com/apecloud/kbcli/pkg/util"
	"github.com/apecloud/kubeblocks/apis/apps/v1alpha1"
)

func getAddonVersion(addon *extensionsv1alpha1.Addon) string {
	if len(addon.Labels) == 0 {
		return ""
	}
	// get version by addon version label
	if version, ok := addon.Labels[types.AddonVersionLabelKey]; ok {
		return version
	}
	// get version by app version label
	if version, ok := addon.Labels[constant.AppVersionLabelKey]; ok {
		return version
	}
	return ""
}

func CheckBeforeDisableAddon(f cmdutil.Factory, addons []string) error {
	labelSelecotor := util.BuildClusterLabel("", addons)
	r := f.NewBuilder().
		Unstructured().
		AllNamespaces(true).
		LabelSelector(labelSelecotor).
		ResourceTypeOrNameArgs(true, append([]string{util.GVRToString(types.ClusterGVR())})...).
		ContinueOnError().
		Latest().
		Flatten().
		Do()
	infos, err := r.Infos()
	if err != nil {
		return err
	} else if len(infos) != 0 {
		errMsg := "There are addons are being used:\n"
		for _, info := range infos {
			var cluster v1alpha1.Cluster
			if err = runtime.DefaultUnstructuredConverter.FromUnstructured(info.Object.(*unstructured.Unstructured).Object, &cluster); err != nil {
				return err
			}
			errMsg += fmt.Sprintf("name: %s namespace: %s addon: %s\n", printer.BoldRed(info.Name), printer.BoldYellow(info.Namespace), printer.BoldRed(cluster.Labels[constant.ClusterDefLabelKey]))
		}
		errMsg += "please delete the cluster(s) first!\n"

		return fmt.Errorf(errMsg)
	}
	return nil
}
