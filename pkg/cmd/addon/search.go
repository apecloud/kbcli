/*
Copyright (C) 2022-2026 ApeCloud Co., Ltd

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
		Use:     "search [ADDON_NAME]",
		Short:   "Search the addon from index",
		Example: addonSearchExample,
		Args:    cobra.MaximumNArgs(1),
		PersistentPreRun: func(cmd *cobra.Command, _ []string) {
			util.CheckErr(util.EnableLogToFile(cmd.Flags()))
			util.CheckErr(addDefaultIndex())
		},
		ValidArgsFunction: addonNameCompletionFunc,
		Run: func(_ *cobra.Command, args []string) {
			if len(args) == 1 {
				o.name = args[0]
			} else {
				o.name = ""
			}
			util.CheckErr(o.Run(streams.Out, f))
		},
	}
	cmd.Flags().StringVar(&o.path, "path", "", "the local directory contains addon CRs")
	return cmd
}

func (o *searchOpts) Run(out io.Writer, f cmdutil.Factory) error {
	listOpt := &addonListOpts{
		ListOptions: action.NewListOptions(f, genericiooptions.IOStreams{Out: out}, types.AddonGVR()),
	}

	// Determine the directory to search in
	dir := o.path
	if dir == "" {
		var err error
		dir, err = util.GetCliAddonDir()
		if err != nil {
			return err
		}
	}

	// Search for addons based on the name or all addons if no name is specified
	results, err := searchAddon(o.name, dir, "")
	if err != nil {
		return err
	}

	// Display results based on whether a name is specified or not
	if o.name == "" {
		return o.displayAllAddons(out, results, listOpt)
	} else {
		return o.displaySingleAddon(out, results)
	}
}

// displayAllAddons lists all addons and indicates whether they are installed
func (o *searchOpts) displayAllAddons(out io.Writer, results []searchResult, listOpt *addonListOpts) error {
	results = uniqueByName(results)
	err := checkAddonInstalled(&results, listOpt)
	if err != nil {
		return err
	}

	tbl := printer.NewTablePrinter(out)
	tbl.AddRow("ADDON", "STATUS")

	statusMap := map[bool]string{
		true:  "installed",
		false: "uninstalled",
	}

	for _, res := range results {
		tbl.AddRow(res.addon.Name, statusMap[res.isInstalled])
	}
	tbl.Print()

	return nil
}

// displaySingleAddon shows detailed information for a specified addon
func (o *searchOpts) displaySingleAddon(out io.Writer, results []searchResult) error {
	tbl := printer.NewTablePrinter(out)
	tbl.AddRow("ADDON", "VERSION", "INDEX")

	if len(results) == 0 {
		fmt.Fprintf(out, "%s addon not found. Please update your index or check the addon name.\n"+
			"You can use the command 'kbcli addon index update --all=true' to update all indexes,\n"+
			"or specify a local path containing addons with the command 'kbcli addon search --path=/path/to/local/addons'\n", o.name)
		return nil
	}

	for _, res := range results {
		tbl.AddRow(res.addon.Name, getAddonVersion(res.addon), res.index.name)
	}
	tbl.Print()

	return nil
}

// searchAddon searches for addons that meet the specified criteria in a given directory
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
