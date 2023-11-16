package addon

import (
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
	"github.com/apecloud/kubeblocks/pkg/constant"

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
		PersistentPreRun: func(_ *cobra.Command, _ []string) {
			klog.V(2).Enabled()
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
	results, err := searchAddon(args[0])
	if err != nil {
		return err
	}
	for _, res := range results {
		label := res.addon.Labels
		tbl.AddRow(res.addon.Name, label[constant.AppVersionLabelKey], res.index.name)
	}
	tbl.Print()
	return nil
}

func searchAddon(name string) ([]searchResult, error) {
	addonDir, err := util.GetCliAddonDir()
	if err != nil {
		return nil, err
	}
	indexes, err := getAllIndexes()
	if err != nil {
		return nil, err
	}
	var res []searchResult

	searchInDir := func(i index) error {
		return filepath.WalkDir(filepath.Join(addonDir, i.name), func(path string, d fs.DirEntry, err error) error {
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
