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

package cluster

import (
	"context"
	"fmt"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"

	"github.com/apecloud/kubeblocks/pkg/constant"

	"github.com/apecloud/kbcli/pkg/types"
)

// ValidateClusterVersion validates the cluster version.
/*func ValidateClusterVersion(dynamic dynamic.Interface, cd string, cv string) error {
	versions, err := GetVersionByClusterDef(dynamic, cd)
	if err != nil {
		return err
	}

	// check cluster version exists or not
	for _, item := range versions.Items {
		if item.Name == cv {
			return nil
		}
	}
	return fmt.Errorf("failed to find cluster version \"%s\"", cv)
}*/

func ValidateClusterVersionByComponentDef(dynamic dynamic.Interface, compDefs []string, cv string) error {
	for _, compDef := range compDefs {
		comp, err := dynamic.Resource(types.CompDefGVR()).Get(context.Background(), compDef, metav1.GetOptions{})
		if err != nil {
			return err
		}
		labels := comp.GetLabels()
		cvSplit := strings.Split(cv, "-")
		// todo: add cv label to compDef or remove cv in the future
		if labels != nil && labels[constant.AppNameLabelKey] == cvSplit[0] && labels[constant.AppVersionLabelKey] == cvSplit[1] {
			continue
		}
		return fmt.Errorf("failed to find cluster version referencing component definition %s", compDef)
	}
	return nil
}
