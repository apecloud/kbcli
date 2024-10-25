/*
Copyright (C) 2022-2024 ApeCloud Co., Ltd

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package cmpv

import (
	"fmt"
	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	"k8s.io/client-go/dynamic"
	clientset "k8s.io/client-go/kubernetes"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/kubectl/pkg/util/templates"

	"github.com/apecloud/kbcli/pkg/types"
	"github.com/apecloud/kbcli/pkg/util"
)

var (
	describeExample = templates.Examples(`
		# describe a specified cluster definition
		kbcli clusterdefinition describe myclusterdef`)
)

type describeOptions struct {
	factory   cmdutil.Factory
	client    clientset.Interface
	dynamic   dynamic.Interface
	namespace string

	names []string
	genericiooptions.IOStreams
}

func NewDescribeCmd(f cmdutil.Factory, streams genericiooptions.IOStreams) *cobra.Command {
	o := &describeOptions{
		factory:   f,
		IOStreams: streams,
	}
	cmd := &cobra.Command{
		Use:               "describe",
		Short:             "Describe ClusterDefinition.",
		Example:           describeExample,
		Aliases:           []string{"desc"},
		ValidArgsFunction: util.ResourceNameCompletionFunc(f, types.ClusterDefGVR()),
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(o.complete(args))
			cmdutil.CheckErr(o.run())
		},
	}
	return cmd
}

func (o *describeOptions) complete(args []string) error {
	var err error

	if len(args) == 0 {
		return fmt.Errorf("cluster definition name should be specified")
	}
	o.names = args

	if o.client, err = o.factory.KubernetesClientSet(); err != nil {
		return err
	}

	if o.dynamic, err = o.factory.DynamicClient(); err != nil {
		return err
	}

	if o.namespace, _, err = o.factory.ToRawKubeConfigLoader().Namespace(); err != nil {
		return err
	}

	return nil
}

func (o *describeOptions) run() error {
	for _, name := range o.names {
		if err := o.describeCmpd(name); err != nil {
			return err
		}
	}
	return nil
}

func (o *describeOptions) describeCmpd(name string) error {
	return nil
}
