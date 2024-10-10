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
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/kubectl/pkg/util/templates"

	extensionsv1alpha1 "github.com/apecloud/kubeblocks/apis/extensions/v1alpha1"

	"github.com/apecloud/kbcli/pkg/action"
	"github.com/apecloud/kbcli/pkg/printer"
	"github.com/apecloud/kbcli/pkg/types"
	"github.com/apecloud/kbcli/pkg/util"
)

var addonSearchExample = templates.Examples(`
	# search the addons of all index
	kbcli addon search

	# search the addons from a specified local path 
	kbcli addon search --path /path/to/local/chart

	# search different versions and indexes of an addon
	kbcli addon search apecloud-mysql
`)

type searchResult struct {
	index       index
	addon       *extensionsv1alpha1.Addon
	isInstalled bool
}

type searchOpts struct {
	// the name of addon to search, if not specified returns all the addons in the indexes
	name string
	// the local path contains addon CRs and needs to be specified when operating offline
	path string
}

func newSearchCmd(f cmdutil.Factory, streams genericiooptions.IOStreams) *cobra.Command {
	o := &searchOpts{}
	cmd := &cobra.Command{
		Use:     "search",
		Short:   "Search the addon from index",
		Example: addonSearchExample,
		PersistentPreRun: func(cmd *cobra.Command, _ []string) {
			util.CheckErr(util.EnableLogToFile(cmd.Flags()))
			util.CheckErr(addDefaultIndex())
		},
		Run: func(_ *cobra.Command, args []string) {
			if len(args) == 0 {
				o.name = ""
			} else {
				o.name = args[0]
			}
			util.CheckErr(o.search(streams.Out, &addonListOpts{
				ListOptions: action.NewListOptions(f, streams, types.AddonGVR()),
			}))
		},
	}
	cmd.Flags().StringVar(&o.path, "path", "", "the local directory contains addon CRs")
	return cmd
}

func (o *searchOpts) search(out io.Writer, addonListOpts *addonListOpts) error {
	var (
		err     error
		results []searchResult
	)
	if o.path == "" {
		dir, err := util.GetCliAddonDir()
		if err != nil {
			return err
		}
		if results, err = searchAddon(o.name, dir, ""); err != nil {
			return err
		}
	} else {
		if results, err = searchAddon(o.name, filepath.Dir(o.path), filepath.Base(o.path)); err != nil {
			return err
		}
	}

	tbl := printer.NewTablePrinter(out)
	if o.name == "" {
		tbl.AddRow("ADDON", "STATUS")
		statusMap := map[bool]string{
			true:  "installed",
			false: "uninstalled",
		}
		results = uniqueByName(results)
		err := checkAddonInstalled(&results, addonListOpts)
		if err != nil {
			return err
		}
		for _, res := range results {
			tbl.AddRow(res.addon.Name, statusMap[res.isInstalled])
		}
	} else {
		tbl.AddRow("ADDON", "VERSION", "INDEX")
		if len(results) == 0 {
			fmt.Fprintf(out, "%s addon not found. Please update your index or check the addon name.\n"+
				"You can use the command 'kbcli addon index update --all=true' to update all indexes,\n"+
				"or specify a local path containing addons with the command 'kbcli addon search --path=/path/to/local/chart'", o.name)
			return nil
		}
		for _, res := range results {
			tbl.AddRow(res.addon.Name, getAddonVersion(res.addon), res.index.name)
		}
	}
	tbl.Print()
	return nil
}

// searchAddon function will search for the addons with the specified name in the index of the specified directory and return them.
func searchAddon(name string, indexDir string, theIndex string) ([]searchResult, error) {
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
				if name == "" || addon.Name == name {
					res = append(res, searchResult{i, addon, false})
				}
			}
			return nil
		})
	}

	var indexes []index
	var err error
	if theIndex == "" {
		// search all the indexes from the dir
		indexes, err = getAllIndexes(indexDir)
		if err != nil {
			return nil, err
		}
	} else {
		// add the specified index
		indexes = append(indexes, index{
			name: theIndex,
			url:  "",
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
