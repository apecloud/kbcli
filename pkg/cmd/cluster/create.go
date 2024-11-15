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
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/kubectl/pkg/util/templates"

	"github.com/apecloud/kbcli/pkg/action"
	"github.com/apecloud/kbcli/pkg/types"
)

var clusterCreateExample = templates.Examples(`
	# Create a postgresql 
	kbcli cluster create postgresql my-cluster

   # Get the cluster yaml by dry-run
	kbcli cluster create postgresql my-cluster --dry-run

	# Edit cluster yaml before creation.
	kbcli cluster create mycluster --edit
`)

type CreateOptions struct {
	Cmd *cobra.Command `json:"-"`

	action.CreateOptions `json:"-"`
}

func NewCreateCmd(f cmdutil.Factory, streams genericiooptions.IOStreams) *cobra.Command {
	o := NewCreateOptions(f, streams)
	cmd := &cobra.Command{
		Use:     "create [NAME]",
		Short:   "Create a cluster.",
		Example: clusterCreateExample,
		Run: func(cmd *cobra.Command, args []string) {
			println("no implement")
		},
	}

	// add all subcommands for supported cluster type
	cmd.AddCommand(buildCreateSubCmds(&o.CreateOptions)...)

	o.Cmd = cmd

	return cmd
}

func NewCreateOptions(f cmdutil.Factory, streams genericiooptions.IOStreams) *CreateOptions {
	o := &CreateOptions{CreateOptions: action.CreateOptions{
		Factory:   f,
		IOStreams: streams,
		GVR:       types.ClusterGVR(),
	}}
	return o
}

// MultipleSourceComponents gets component data from multiple source, such as stdin, URI and local file
func MultipleSourceComponents(fileName string, in io.Reader) ([]byte, error) {
	var data io.Reader
	switch {
	case fileName == "-":
		data = in
	case strings.Index(fileName, "http://") == 0 || strings.Index(fileName, "https://") == 0:
		resp, err := http.Get(fileName)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		data = resp.Body
	default:
		f, err := os.Open(fileName)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		data = f
	}
	return io.ReadAll(data)
}
