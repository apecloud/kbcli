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
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"regexp"
	"strings"

	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericiooptions"

	"github.com/apecloud/kbcli/pkg/printer"
	"github.com/apecloud/kbcli/pkg/types"
	"github.com/apecloud/kbcli/pkg/util"
)

type index struct {
	name string
	url  string
}

func newIndexListCmd(streams genericiooptions.IOStreams) *cobra.Command {
	indexListCmd := &cobra.Command{Use: "list",
		Short: "List addon indexes",
		Long: `Print a list of addon indexes.

This command prints a list of addon indexes. It shows the name and the remote URL for
each addon index in table format.`,
		Args: cobra.NoArgs,
		Run: func(_ *cobra.Command, _ []string) {
			util.CheckErr(listIndexes(streams.Out))
		},
	}
	return indexListCmd
}

func newIndexAddCmd() *cobra.Command {
	indexAddCmd := &cobra.Command{
		Use:     "add",
		Short:   "Add a new addon index",
		Long:    "Configure a new index to install KubeBlocks addon from.",
		Example: "kbcli addon index add kubeblocks " + types.DefaultAddonIndexURL,
		Args:    cobra.ExactArgs(2),
		PersistentPreRun: func(_ *cobra.Command, _ []string) {
			util.CheckErr(addDefaultIndex())
		},
		Run: func(_ *cobra.Command, args []string) {
			util.CheckErr(addIndex(args))
		},
	}

	return indexAddCmd
}

func newIndexDeleteCmd() *cobra.Command {
	indexDeleteCmd := &cobra.Command{
		Use:               "delete",
		Short:             "Delete an addon index",
		Long:              `Delete a configured addon index.`,
		ValidArgsFunction: indexCompletion(),
		Args:              cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			util.CheckErr(deleteIndex(args[0]))
		},
	}

	return indexDeleteCmd
}

func newIndexCmd(streams genericiooptions.IOStreams) *cobra.Command {
	indexCmd := &cobra.Command{
		Use:   "index",
		Short: "Manage custom addon indexes",
		Long:  "Manage which repositories are used to discover and install addon from.",
		Args:  cobra.NoArgs,
	}
	indexCmd.AddCommand(
		newIndexAddCmd(),
		newIndexDeleteCmd(),
		newIndexListCmd(streams),
		newIndexUpdateCmd(streams),
	)

	return indexCmd
}

type updateOption struct {
	names []string
	all   bool

	genericiooptions.IOStreams
}

// validate will check the update index whether existed
func (o *updateOption) validate(args []string) error {
	indexes, err := getAllIndexes()
	if err != nil {
		return err
	}

	if o.all {
		for _, e := range indexes {
			o.names = append(o.names, e.name)
		}
		return nil
	}
	if len(args) == 0 {
		return fmt.Errorf("you must specify one index or use --all flag update all indexes.\nuse `kbcli addon index list` list all available indexes")
	}
	indexMaps := make(map[string]struct{})
	for _, index := range indexes {
		indexMaps[index.name] = struct{}{}
	}
	for _, name := range args {
		if _, ok := indexMaps[name]; !ok {
			return fmt.Errorf("index %s don't existed", name)
		}
		o.names = append(o.names, name)
	}
	return nil
}

func (o *updateOption) run() error {
	addonDir, err := util.GetCliAddonDir()
	if err != nil {
		return err
	}
	for _, name := range o.names {

		if isLatest, err := util.IsRepoLatest(path.Join(addonDir, name)); err == nil && isLatest {
			fmt.Fprintf(o.Out, "index \"%s\" is already at the latest and requires no updates.\n", name)
			continue
		}

		err = util.UpdateAndCleanUntracked(path.Join(addonDir, name))
		if err != nil {
			return fmt.Errorf("failed to update index %s due to %s", name, err.Error())
		}
		fmt.Fprintf(o.Out, "index \"%s\" has been updated.\n", name)
	}

	return nil
}

func newIndexUpdateCmd(streams genericiooptions.IOStreams) *cobra.Command {
	o := &updateOption{
		names:     make([]string, 0),
		all:       false,
		IOStreams: streams,
	}
	indexUpdateCmd := &cobra.Command{
		Use:               "update",
		Short:             "update the specified index(es)",
		Long:              "Update existed index repository from index origin URL",
		Example:           "kbcli addon index update KubeBlocks",
		ValidArgsFunction: indexCompletion(),
		Run: func(cmd *cobra.Command, args []string) {
			util.CheckErr(o.validate(args))
			util.CheckErr(o.run())
		},
	}
	indexUpdateCmd.Flags().BoolVar(&o.all, "all", false, "Upgrade all addon index")
	return indexUpdateCmd
}

// IsValidIndexName validates if an index name contains invalid characters
func IsValidIndexName(name string) bool {
	var validNamePattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)
	return validNamePattern.MatchString(name)
}

func addIndex(args []string) error {
	name, url := args[0], args[1]
	if !IsValidIndexName(name) {
		return errors.New("invalid index name")
	}

	addonDir, err := util.GetCliAddonDir()
	if err != nil {
		return err
	}
	index := path.Join(addonDir, name)
	if _, err := os.Stat(index); os.IsNotExist(err) {
		if err = util.EnsureCloned(url, index); err != nil {
			return err
		}
		fmt.Printf("You have added a new index from %q\n", args[1])
		return err
	} else if err != nil {
		return err
	}
	return errors.New("index already exists")
}

func listIndexes(out io.Writer) error {
	tbl := printer.NewTablePrinter(out)
	tbl.SortBy(1)
	tbl.SetHeader("INDEX", "URL")

	indexes, err := getAllIndexes()
	if err != nil {
		return err
	}
	for _, e := range indexes {
		tbl.AddRow(e.name, e.url)
	}
	tbl.Print()
	return nil
}

func deleteIndex(index string) error {
	if !IsValidIndexName(index) {
		return errors.New("invalid index name")
	}

	addonDir, err := util.GetCliAddonDir()
	if err != nil {
		return err
	}
	indexDir := path.Join(addonDir, index)
	if _, err := os.Stat(indexDir); err == nil {
		if err = os.RemoveAll(indexDir); err != nil {
			return err
		}
		fmt.Printf("Index \"%s\" have been deleted", index)
		return nil
	} else {
		if os.IsNotExist(err) {
			return fmt.Errorf("index %s does not exist", index)
		}
		return fmt.Errorf("error while removing the addon index: %s", err.Error())
	}

}

func addDefaultIndex() error {
	addonDir, err := util.GetCliAddonDir()
	if err != nil {
		return fmt.Errorf("can't get the index dir : %s", err.Error())
	}

	defaultIndexDir := path.Join(addonDir, types.KubeBlocksReleaseName)
	if _, err := os.Stat(defaultIndexDir); err != nil && os.IsNotExist(err) {
		if err = util.EnsureCloned(types.DefaultAddonIndexURL, defaultIndexDir); err != nil {
			return err
		}
		fmt.Printf("Default addon index \"kubeblocks\" has been added.")
		return nil
	}
	return fmt.Errorf("default index %s:%s already exists", types.KubeBlocksReleaseName, types.DefaultAddonIndexURL)
}

func getAllIndexes() ([]index, error) {
	addonDir, err := util.GetCliAddonDir()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(addonDir)
	if err != nil {
		return nil, err
	}
	res := []index{}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		indexName := e.Name()
		remote, err := util.GitGetRemoteURL(path.Join(addonDir, indexName))
		if err != nil {
			return nil, err
		}
		res = append(res, index{
			name: indexName,
			url:  remote,
		})
	}
	return res, nil
}

func indexCompletion() func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	return func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		availableComps := []string{}
		indexes, err := getAllIndexes()
		if err != nil {
			return availableComps, cobra.ShellCompDirectiveNoFileComp
		}
		seen := make(map[string]struct{})
		for _, input := range args {
			seen[input] = struct{}{}
		}

		for _, e := range indexes {
			if _, ok := seen[e.name]; !ok && strings.HasPrefix(e.name, toComplete) {
				availableComps = append(availableComps, e.name)
			}
		}

		return availableComps, cobra.ShellCompDirectiveNoFileComp
	}
}
