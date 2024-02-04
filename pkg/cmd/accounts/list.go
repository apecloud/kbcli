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

package accounts

import (
	"context"
	"errors"
	"fmt"

	"k8s.io/cli-runtime/pkg/genericiooptions"
	"k8s.io/klog/v2"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"

	"github.com/apecloud/kubeblocks/pkg/lorry/client"
)

type ListUserOptions struct {
	*AccountBaseOptions
	UsersInfo []map[string]interface{}
}

func NewListUserOptions(f cmdutil.Factory, streams genericiooptions.IOStreams) *ListUserOptions {
	return &ListUserOptions{
		AccountBaseOptions: NewAccountBaseOptions(f, streams),
	}
}
func (o ListUserOptions) Validate(args []string) error {
	return o.AccountBaseOptions.Validate(args)
}

func (o *ListUserOptions) Complete() error {
	return o.AccountBaseOptions.Complete()
}

func (o *ListUserOptions) Run() error {
	klog.V(1).Info(fmt.Sprintf("connect to cluster %s, component %s, instance %s\n", o.ClusterName, o.ComponentName, o.PodName))
	lorryClient, err := client.NewK8sExecClientWithPod(o.Config, o.Pod)
	if err != nil {
		return err
	}
	if lorryClient == nil {
		return errors.New("not support yet")
	}

	users, err := lorryClient.ListUsers(context.Background())
	if err != nil {
		o.printGeneralInfo("fail", err.Error())
		return err
	}
	o.UsersInfo = users
	o.printUserInfo(users)
	return nil
}

func (o *ListUserOptions) Exec() error {
	if err := o.Complete(); err != nil {
		return err
	}
	if err := o.Run(); err != nil {
		return err
	}
	return nil
}
