/*
Copyright (C) 2022-2026 ApeCloud Co., Ltd

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
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	"k8s.io/client-go/dynamic"
	clientset "k8s.io/client-go/kubernetes"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"

	"github.com/apecloud/kbcli/pkg/util"
)

const (
	TrueValue = "true"

	DPEnvRestoreKeyPatterns     = "DP_RESTORE_KEY_PATTERNS"
	DPEnvRestoreKeyIgnoreErrors = "DP_RESTORE_KEY_IGNORE_ERRORS"
)

type DescribeDPOptions struct {
	Factory   cmdutil.Factory
	Client    clientset.Interface
	Dynamic   dynamic.Interface
	Namespace string
	Names     []string

	// resource type and names
	Gvr schema.GroupVersionResource

	genericiooptions.IOStreams
}

func (o *DescribeDPOptions) Validate(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("%s name should be specified", o.Gvr.Resource)
	}
	return nil
}

func (o *DescribeDPOptions) ValidateForClusterCmd(args []string) error {
	// must specify one of the cluster name or backup policy name
	if len(args) == 0 && len(o.Names) == 0 {
		return fmt.Errorf("missing cluster name or %s name", o.Gvr.Resource)
	}

	return nil
}
func (o *DescribeDPOptions) GetObjListByArgs(args []string) (*unstructured.UnstructuredList, error) {
	objList, err := o.Dynamic.Resource(o.Gvr).Namespace(o.Namespace).List(context.TODO(), metav1.ListOptions{
		LabelSelector: util.BuildLabelSelectorByNames("", args),
	})
	if err != nil {
		return nil, err
	}
	if len(objList.Items) == 0 {
		return nil, fmt.Errorf("can not found any %s", o.Gvr.Resource)
	}
	return objList, err
}

func (o *DescribeDPOptions) Complete() error {
	var err error

	if o.Client, err = o.Factory.KubernetesClientSet(); err != nil {
		return err
	}

	if o.Dynamic, err = o.Factory.DynamicClient(); err != nil {
		return err
	}

	if o.Namespace, _, err = o.Factory.ToRawKubeConfigLoader().Namespace(); err != nil {
		return err
	}
	return nil
}
