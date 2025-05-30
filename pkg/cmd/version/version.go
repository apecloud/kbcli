/*
Copyright (C) 2022-2025 ApeCloud Co., Ltd

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

package version

import (
	"fmt"
	"reflect"
	"runtime"

	gv "github.com/hashicorp/go-version"
	"github.com/spf13/cobra"
	"k8s.io/klog/v2"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"

	"github.com/apecloud/kbcli/pkg/util"
	"github.com/apecloud/kbcli/version"
)

type versionOptions struct {
	verbose bool
}

// NewVersionCmd the version command
func NewVersionCmd(f cmdutil.Factory) *cobra.Command {
	o := &versionOptions{}
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Print the version information, include kubernetes, KubeBlocks and kbcli version.",
		Run: func(cmd *cobra.Command, args []string) {
			o.Run(f)
		},
	}
	cmd.Flags().BoolVar(&o.verbose, "verbose", false, "print detailed kbcli information")
	return cmd
}

func (o *versionOptions) Run(f cmdutil.Factory) {
	client, err := f.KubernetesClientSet()
	if err != nil {
		klog.V(1).Infof("failed to get clientset: %v", err)
	}

	v, _ := util.GetVersionInfo(client)
	if v.Kubernetes != "" {
		fmt.Printf("Kubernetes: %s\n", v.Kubernetes)
	}
	if v.KubeBlocks != "" {
		fmt.Printf("KubeBlocks: %s\n", v.KubeBlocks)
	}
	fmt.Printf("kbcli: %s\n", v.Cli)
	if o.verbose {
		fmt.Printf("  BuildDate: %s\n", version.BuildDate)
		fmt.Printf("  GitCommit: %s\n", version.GitCommit)
		fmt.Printf("  GitTag: %s\n", version.GitVersion)
		fmt.Printf("  GoVersion: %s\n", runtime.Version())
		fmt.Printf("  Compiler: %s\n", runtime.Compiler)
		fmt.Printf("  Platform: %s/%s\n", runtime.GOOS, runtime.GOARCH)
	}

	kbVersion, err := gv.NewVersion(v.KubeBlocks)
	if err != nil {
		klog.V(1).Infof("failed to parse KubeBlocks version: %v", err)
		return
	}
	cliVersion, err := gv.NewVersion(v.Cli)
	if err != nil {
		klog.V(1).Infof("failed to parse kbcli version: %v", err)
		return
	}

	if !checkVersionMatch(kbVersion, cliVersion) {
		fmt.Printf("WARNING: version difference between kbcli (%s) and kubeblocks (%s) \n", v.Cli, v.KubeBlocks)
	}
}

func checkVersionMatch(cliVersion *gv.Version, kbVersion *gv.Version) bool {
	return reflect.DeepEqual(cliVersion.Segments64(), kbVersion.Segments64())
}
