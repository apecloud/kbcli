package addon

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"regexp"

	"github.com/apecloud/kbcli/pkg/printer"
	"github.com/apecloud/kbcli/pkg/types"
	"github.com/apecloud/kbcli/pkg/util"
	"github.com/apecloud/kbcli/pkg/util/gitutil"
	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	"k8s.io/klog/v2"
)

type Index struct {
	Name string
	URL  string
}

func init() {
	addonDir, err := util.GetCliAddonDir()
	if err != nil {
		klog.V(1).ErrorS(err, "can't get the addon ")
	}

	defaultIndexDir := path.Join(addonDir, types.KubeBlocksReleaseName)
	if _, err := os.Stat(defaultIndexDir); os.IsNotExist(err) {
		err = gitutil.EnsureCloned(types.DefaultAddonIndexURL, defaultIndexDir)
		if err != nil {
			klog.V(1).ErrorS(err, "can't pull the DefaultAddonIndexURL", types.DefaultAddonIndexURL)
		}
	} else {
		klog.V(1).ErrorS(err, "can't get the kbcli addon index dir")
	}
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
		Short:   "Add a new index",
		Long:    "Configure a new index to install KubeBlocks addon from.",
		Example: "kbcli index add KubeBlocks " + types.DefaultAddonIndexURL,
		Args:    cobra.ExactArgs(2),
		Run: func(_ *cobra.Command, args []string) {
			util.CheckErr(addAddonIndex(args))
			fmt.Printf("You have added a new index from %q\nThe addons in this index are not audited by ApeCloud.", args[1])
		},
	}
	return indexAddCmd
}

func newIndexDeleteCmd() *cobra.Command {
	indexDeleteCmd := &cobra.Command{
		Use:   "remove",
		Short: "Remove an addon index",
		Long:  `Remove a configured addon index.`,

		Args: cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			util.CheckErr(indexDelete(args[0]))
		},
	}

	return indexDeleteCmd
}

func newIndexCmd(streams genericiooptions.IOStreams) *cobra.Command {
	indexCmd := &cobra.Command{
		Use:   "index",
		Short: "Manage custom plugin indexes",
		Long:  "Manage which repositories are used to discover and install plugins from.",
		Args:  cobra.NoArgs,
	}
	indexCmd.AddCommand(
		newIndexAddCmd(),
		newIndexDeleteCmd(),
		newIndexListCmd(streams),
	)

	return indexCmd
}

// IsValidIndexName validates if an index name contains invalid characters
func IsValidIndexName(name string) bool {
	var validNamePattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)
	return validNamePattern.MatchString(name)
}

func addAddonIndex(args []string) error {
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
		return gitutil.EnsureCloned(url, index)
	} else if err != nil {
		return err
	}
	return errors.New("index already exists")
}

func listIndexes(out io.Writer) error {
	tbl := printer.NewTablePrinter(out)
	tbl.SortBy(1)
	tbl.SetHeader("INDEX", "URL")

	dir, err := util.GetCliAddonDir()
	if err != nil {
		return err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("failed to list directory: %s", err.Error())
	}

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		indexName := e.Name()
		remote, err := gitutil.GetRemoteURL(path.Join(dir, indexName))
		if err != nil {
			return fmt.Errorf("failed to list the remote URL for index %s due to %s", indexName, err.Error())
		}
		tbl.AddRow(indexName, remote)
	}
	tbl.Print()
	return nil
}

func indexDelete(index string) error {
	if IsValidIndexName(index) {
		return errors.New("invalid index name")
	}

	addonDir, err := util.GetCliAddonDir()
	if err != nil {
		return err
	}
	indexDir := path.Join(addonDir, index)
	if _, err := os.Stat(indexDir); err == nil {
		return os.RemoveAll(indexDir)
	} else {
		if os.IsNotExist(err) {
			return fmt.Errorf("index %s does not exist", index)
		}
		return fmt.Errorf("error while removing the addon index: %s", err.Error())
	}

}
