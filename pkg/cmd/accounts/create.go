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
	"errors"
	"fmt"

	"github.com/sethvargo/go-password/password"
	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	"k8s.io/klog/v2"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"

	"github.com/apecloud/kubeblocks/pkg/lorry/client"
)

type CreateUserOptions struct {
	*AccountBaseOptions
	UserName string
	Password string
}

func NewCreateUserOptions(f cmdutil.Factory, streams genericiooptions.IOStreams) *CreateUserOptions {
	return &CreateUserOptions{
		AccountBaseOptions: NewAccountBaseOptions(f, streams),
	}
}

func (o *CreateUserOptions) AddFlags(cmd *cobra.Command) {
	o.AccountBaseOptions.AddFlags(cmd)
	cmd.Flags().StringVar(&o.UserName, "name", "", "Required. Specify the name of user, which must be unique.")
	cmd.Flags().StringVarP(&o.Password, "password", "p", "", "Optional. Specify the password of user. The default value is empty, which means a random password will be generated.")
	_ = cmd.MarkFlagRequired("name")
	// TODO:@shanshan add expire flag if needed
	// cmd.Flags().DurationVar(&o.info.ExpireAt, "expire", 0, "Optional. Specify the expired time of password. The default value is 0, which means the user will never expire.")
}

func (o *CreateUserOptions) Validate() error {
	if len(o.UserName) == 0 {
		return errMissingUserName
	}
	return nil
}

func (o *CreateUserOptions) Complete() error {
	var err error
	if err = o.AccountBaseOptions.Complete(); err != nil {
		return err
	}
	// complete other options
	if len(o.Password) == 0 {
		o.Password, _ = password.Generate(10, 2, 0, false, false)
	}
	return err
}

func (o *CreateUserOptions) Run() error {
	klog.V(1).Info(fmt.Sprintf("connect to cluster %s, component %s, instance %s\n", o.ClusterName, o.ComponentName, o.PodName))
	lorryClient, err := client.NewK8sExecClientWithPod(o.Config, o.Pod)
	if err != nil {
		return err
	}
	if lorryClient == nil {
		return errors.New("not support yet")
	}

	err = lorryClient.CreateUser(context.Background(), o.UserName, o.Password, "")
	if err != nil {
		o.printGeneralInfo("fail", err.Error())
		return err
	}
	o.printGeneralInfo("success", "")
	return nil
}

func (o *CreateUserOptions) Exec() error {
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
