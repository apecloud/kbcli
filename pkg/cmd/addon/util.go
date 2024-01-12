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
	"io"
	"strings"

	"golang.org/x/exp/maps"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic"

	"github.com/apecloud/kbcli/pkg/printer"
	"github.com/apecloud/kbcli/pkg/types"
	"github.com/apecloud/kbcli/pkg/util"
	"github.com/apecloud/kubeblocks/apis/apps/v1alpha1"
	extensionsv1alpha1 "github.com/apecloud/kubeblocks/apis/extensions/v1alpha1"
	"github.com/apecloud/kubeblocks/pkg/constant"

	"github.com/apecloud/kbcli/pkg/util/prompt"
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

func CheckAddonUsedByCluster(dynamic dynamic.Interface, addons []string, in io.Reader) error {
	labelSelecotor := util.BuildClusterLabel("", addons)
	list, err := dynamic.Resource(types.ClusterGVR()).List(context.Background(), metav1.ListOptions{LabelSelector: labelSelecotor})
	if err != nil {
		return err
	}
	if list != nil && len(list.Items) != 0 {
		msg := "There are addons are being used by K8s clusters:\n"
		usedAddons := make(map[string]struct{})
		for _, item := range list.Items {
			var cluster v1alpha1.Cluster
			if err = runtime.DefaultUnstructuredConverter.FromUnstructured(item.Object, &cluster); err != nil {
				return err
			}
			msg += fmt.Sprintf("cluster name: %s namespace: %s addon: %s\n", printer.BoldGreen(item.GetName()), printer.BoldYellow(item.GetNamespace()), printer.BoldRed(cluster.Labels[constant.ClusterDefLabelKey]))
			labels := item.GetLabels()
			usedAddons[labels[constant.ClusterDefLabelKey]] = struct{}{}
		}
		msg += fmt.Sprintf("In used addons [%s] to be deleted", printer.BoldRed(strings.Join(maps.Keys(usedAddons), ",")))
		return prompt.Confirm(maps.Keys(usedAddons), in, msg, "")
	}
	return nil
}
