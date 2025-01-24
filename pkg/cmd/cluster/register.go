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

package cluster

import (
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/asaskevich/govalidator"
	"github.com/spf13/cobra"
	"helm.sh/helm/v3/pkg/repo"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/kubectl/pkg/util/templates"

	"github.com/apecloud/kbcli/pkg/cluster"
	"github.com/apecloud/kbcli/pkg/types"
	"github.com/apecloud/kbcli/pkg/util"
	"github.com/apecloud/kbcli/pkg/util/helm"
)

var clusterRegisterExample = templates.Examples(`
	# Pull a cluster type to local and register it to "kbcli cluster create" sub-cmd from specified URL
	kbcli cluster register orioledb --source https://github.com/apecloud/helm-charts/releases/download/orioledb-cluster-0.6.0-beta.44/orioledb-cluster-0.6.0-beta.44.tgz

	# Register a cluster type from a local path file
	kbcli cluster register neon --source pkg/cli/cluster/charts/neon-cluster.tgz

	# Register a cluster type from a Helm repository, specifying the version and engine.
	kbcli cluster register mysql --engine mysql --version 0.9.0 --repo https://jihulab.com/api/v4/projects/150246/packages/helm/stable
`)

type registerOption struct {
	Factory cmdutil.Factory
	genericiooptions.IOStreams

	clusterType cluster.ClusterType
	source      string
	alias       string
	// cachedName is the filename cached locally
	cachedName string
	engine     string
	repo       string
	version    string
}

func newRegisterOption(f cmdutil.Factory, streams genericiooptions.IOStreams) *registerOption {
	o := &registerOption{
		Factory:   f,
		IOStreams: streams,
	}
	return o
}

func newRegisterCmd(f cmdutil.Factory, streams genericiooptions.IOStreams) *cobra.Command {
	o := newRegisterOption(f, streams)
	cmd := &cobra.Command{
		Use:     "register [NAME]",
		Short:   "Pull the cluster chart to the local cache and register the type to 'create' sub-command",
		Example: clusterRegisterExample,
		Args:    cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			o.clusterType = cluster.ClusterType(args[0])
			cmdutil.CheckErr(o.validate())
			cmdutil.CheckErr(o.run())
			fmt.Fprint(streams.Out, BuildRegisterSuccessExamples(o.clusterType))
		},
	}
	cmd.Flags().StringVarP(&o.source, "source", "S", "", "Specify the cluster type chart source, support a URL or a local file path")
	cmd.Flags().StringVar(&o.alias, "alias", "", "Set the cluster type alias")
	cmd.Flags().StringVar(&o.engine, "engine", "", "Specify the cluster chart name in helm repo")
	cmd.Flags().StringVar(&o.repo, "repo", types.ClusterChartsRepoURL, "Specify the url of helm repo which contains cluster charts")
	cmd.Flags().StringVar(&o.version, "version", "", "Specify the version of cluster chart to register")

	return cmd
}

// RegisterClusterChart a util function to register cluster chart used by addon install, upgrade and enable.
func RegisterClusterChart(f cmdutil.Factory, streams genericiooptions.IOStreams, source, engine, version, repo string) error {
	o := &registerOption{
		Factory:     f,
		IOStreams:   streams,
		source:      source,
		engine:      engine,
		clusterType: cluster.ClusterType(engine),
		repo:        repo,
		version:     version,
	}
	if err := o.validate(); err != nil {
		return err
	}
	if err := o.run(); err != nil {
		cluster.ClearCharts(cluster.ClusterType(engine))
		return err
	}
	if _, err := fmt.Fprint(streams.Out, BuildRegisterSuccessExamples(o.clusterType)); err != nil {
		return err
	}
	return nil
}

// validate will check the flags of command
func (o *registerOption) validate() error {
	re := regexp.MustCompile(`^[a-zA-Z0-9-]{1,16}$`)
	if !re.MatchString(o.clusterType.String()) {
		return fmt.Errorf("cluster type %s is not appropriate as a subcommand", o.clusterType.String())
	}

	if o.isSourceMethod() {
		if validateSource(o.source) != nil {
			return fmt.Errorf("your entered `--source` %s, which is neither a URL nor a file that can be found locally", o.source)
		}
		o.cachedName = filepath.Base(o.source)
	} else {
		o.cachedName = fmt.Sprintf("%s-cluster-%s.tgz", o.engine, o.version)
	}

	return nil
}

func (o *registerOption) run() error {
	localChartPath := filepath.Join(cluster.CliChartsCacheDir, o.cachedName)
	if o.isSourceMethod() {
		if govalidator.IsURL(o.source) {
			// source is URL
			chartsDownloader, err := helm.NewDownloader(helm.NewConfig("default", "", "", false))
			if err != nil {
				return err
			}
			// DownloadTo can't specify the saved name, so download it to TempDir and rename it when copy
			tempPath, _, err := chartsDownloader.DownloadTo(o.source, "", os.TempDir())
			if err != nil {
				return err
			}
			err = copyFile(tempPath, localChartPath)
			if err != nil {
				return err
			}
			_ = os.Remove(tempPath)
		} else {
			if err := copyFile(o.source, localChartPath); err != nil {
				return err
			}
		}
	} else {
		// specify the url of the repo containing cluster charts, and add it as the repo name kubeblocks-addons
		if err := helm.AddRepo(&repo.Entry{
			Name: types.ClusterChartsRepoName,
			URL:  o.repo,
		}); err != nil {
			return err
		}
		err := helm.PullChart(types.ClusterChartsRepoName, o.engine+"-cluster", o.version, cluster.CliChartsCacheDir)
		if err != nil {
			return err
		}
	}

	// read the cluster chart and validate it
	instance := &cluster.TypeInstance{
		Name:      o.clusterType,
		URL:       o.source,
		Alias:     o.alias,
		ChartName: o.cachedName,
	}
	if validated, err := instance.ValidateChartSchema(); !validated {
		_ = os.Remove(localChartPath)
		return err
	}
	// register this chart into Cluster_types
	return instance.PatchNewClusterType()
}

func validateSource(source string) error {
	var err error
	if _, err = url.ParseRequestURI(source); err == nil {
		return nil
	}

	if _, err = os.Stat(source); err == nil {
		return nil
	}
	return err
}

func (o *registerOption) isSourceMethod() bool {
	return o.source != ""
}

func copyFile(src, dest string) error {
	if src == dest {
		return nil
	}
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destFile, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, sourceFile)
	return err
}

// BuildRegisterSuccessExamples builds the creation examples for the specified ClusterType type.
func BuildRegisterSuccessExamples(t cluster.ClusterType) string {
	exampleTpl := `

cluster type "{{ .ClusterType }}" register successful.
Use "kbcli cluster create {{ .ClusterType }}" to create a {{ .ClusterType }} cluster
`

	var builder strings.Builder
	_ = util.PrintGoTemplate(&builder, exampleTpl, map[string]interface{}{
		"ClusterType": t.String(),
	})
	return templates.Examples(builder.String()) + "\n"
}
