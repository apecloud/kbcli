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

package addon

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	"k8s.io/klog/v2"

	extensionsv1alpha1 "github.com/apecloud/kubeblocks/apis/extensions/v1alpha1"

	"github.com/apecloud/kbcli/pkg/printer"
	"github.com/apecloud/kbcli/pkg/util"
)

type searchResult struct {
	index index
	addon *extensionsv1alpha1.Addon
}

func newSearchCmd(streams genericiooptions.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "search",
		Short: "search the addon from index",
		Args:  cobra.ExactArgs(1),
		PersistentPreRun: func(cmd *cobra.Command, _ []string) {
			util.CheckErr(util.EnableLogToFile(cmd.Flags()))
		},
		Run: func(_ *cobra.Command, args []string) {
			util.CheckErr(search(args, streams.Out))
		},
	}
	return cmd
}

func search(args []string, out io.Writer) error {
	tbl := printer.NewTablePrinter(out)
	tbl.AddRow("ADDON", "VERSION", "INDEX")
	dir, err := util.GetCliAddonDir()
	if err != nil {
		return err
	}
	results, err := searchAddon(args[0], dir)
	if err != nil {
		return err
	}
	if len(results) == 0 {
		fmt.Fprintf(out, "%s addon not found. Please update your index or check the addon name", args[0])
		return nil
	}
	for _, res := range results {
		tbl.AddRow(res.addon.Name, getAddonVersion(res.addon), res.index.name)
	}
	tbl.Print()
	return nil
}

// searchAddon function will search for the addons with the specified name in the index of the specified directory and return them.
func searchAddon(name string, indexDir string) ([]searchResult, error) {
	indexes, err := getAllIndexes(indexDir)
	if err != nil {
		return nil, err
	}
	var res []searchResult
	searchInDir := func(i index) error {
		return filepath.WalkDir(filepath.Join(indexDir, i.name), func(path string, d fs.DirEntry, err error) error {
			// skip .git .github
			if ok, _ := regexp.MatchString(`^\..*`, d.Name()); ok && d.IsDir() {
				return filepath.SkipDir
			}
			if d.IsDir() {
				return nil
			}
			if err != nil {
				klog.V(2).Infof("read the file %s fail : %s", path, err.Error())
				return nil
			}
			if strings.HasSuffix(strings.ToLower(d.Name()), ".yaml") {
				addon := &extensionsv1alpha1.Addon{}
				content, _ := os.ReadFile(path)
				err = yaml.Unmarshal(content, addon)
				if err != nil {
					klog.V(2).Infof("unmarshal the yaml %s fail : %s", path, err.Error())
				}
				// if there are other types of resources in the current folder, skip it
				if addon.Kind != "Addon" {
					return filepath.SkipDir
				}
				if addon.Name == name {
					res = append(res, searchResult{i, addon})
				}
			}
			return nil
		})
	}

	for _, e := range indexes {
		err = searchInDir(e)
		if err != nil {
			klog.V(2).Infof("search addon failed due to %s", err.Error())
		}
	}
	return res, nil
}
