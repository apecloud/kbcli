/*
copyright (c) 2022-2024 apecloud co., ltd

this file is part of kubeblocks project

this program is free software: you can redistribute it and/or modify
it under the terms of the gnu affero general public license as published by
the free software foundation, either version 3 of the license, or
(at your option) any later version.

this program is distributed in the hope that it will be useful
but without any warranty; without even the implied warranty of
merchantability or fitness for a particular purpose.  see the
gnu affero general public license for more details.

you should have received a copy of the gnu affero general public license
along with this program.  if not, see <http://www.gnu.org/licenses/>.
*/

package cluster

import (
	"fmt"

	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	"k8s.io/client-go/dynamic"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
)

type ConvertToV1Options struct {
	Cmd *cobra.Command `json:"-"`

	f         cmdutil.Factory
	Dynamic   dynamic.Interface
	Name      string
	Namespace string
	genericiooptions.IOStreams
}

func NewConvertToV1Option(f cmdutil.Factory, streams genericiooptions.IOStreams) *ConvertToV1Options {
	return &ConvertToV1Options{
		f:         f,
		IOStreams: streams,
	}
}

func NewConvertToV1Cmd(f cmdutil.Factory, streams genericiooptions.IOStreams) *cobra.Command {
	o := NewConvertToV1Option(f, streams)
	cmd := &cobra.Command{
		Use:     "create [NAME]",
		Short:   "Create a cluster.",
		Example: clusterCreateExample,
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(o.complete(args))
			cmdutil.CheckErr(o.Run())
		},
	}
	return cmd
}

func (o *ConvertToV1Options) complete(args []string) error {

	if len(args) == 0 {
		return fmt.Errorf("must specify cluster name")
	}
	o.Name = args[0]
	o.Namespace, _, _ = o.f.ToRawKubeConfigLoader().Namespace()
	o.Dynamic, _ = o.f.DynamicClient()
	return nil
}

func (o *ConvertToV1Options) Run() error {
	return nil
}
