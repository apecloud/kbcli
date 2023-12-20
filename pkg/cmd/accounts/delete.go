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

package accounts

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	"k8s.io/klog/v2"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"

	"github.com/apecloud/kubeblocks/pkg/lorry/client"

	"github.com/apecloud/kbcli/pkg/util/prompt"
)

type DeleteUserOptions struct {
	*AccountBaseOptions
	AutoApprove bool
	UserName    string
}

func NewDeleteUserOptions(f cmdutil.Factory, streams genericiooptions.IOStreams) *DeleteUserOptions {
	return &DeleteUserOptions{
		AccountBaseOptions: NewAccountBaseOptions(f, streams),
	}
}

func (o *DeleteUserOptions) AddFlags(cmd *cobra.Command) {
	o.AccountBaseOptions.AddFlags(cmd)
	cmd.Flags().StringVar(&o.UserName, "name", "", "Required user name, please specify it.")
	_ = cmd.MarkFlagRequired("name")
}

func (o *DeleteUserOptions) Validate() error {
	if len(o.UserName) == 0 {
		return errMissingUserName
	}
	if o.AutoApprove {
		return nil
	}
	if err := prompt.Confirm([]string{o.UserName}, o.In, "", ""); err != nil {
		return err
	}
	return nil
}

func (o *DeleteUserOptions) Complete() error {
	var err error
	if err = o.AccountBaseOptions.Complete(); err != nil {
		return err
	}
	return err
}

func (o *DeleteUserOptions) Run() error {
	klog.V(1).Info(fmt.Sprintf("connect to cluster %s, component %s, instance %s\n", o.ClusterName, o.ComponentName, o.PodName))
	lorryClient, err := client.NewK8sExecClientWithPod(o.Pod)
	if err != nil {
		return err
	}

	err = lorryClient.DeleteUser(context.Background(), o.UserName)
	if err != nil {
		o.printGeneralInfo("fail", err.Error())
		return err
	}
	o.printGeneralInfo("success", "")
	return nil
}

func (o *DeleteUserOptions) Exec() error {
	if err := o.Validate(); err != nil {
		return err
	}
	if err := o.Complete(); err != nil {
		return err
	}
	if err := o.Run(); err != nil {
		return err
	}
	return nil
}
