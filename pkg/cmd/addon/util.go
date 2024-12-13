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
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/exp/maps"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/dynamic"
	"k8s.io/klog/v2"

	"github.com/apecloud/kubeblocks/apis/apps/v1alpha1"
	extensionsv1alpha1 "github.com/apecloud/kubeblocks/apis/extensions/v1alpha1"
	"github.com/apecloud/kubeblocks/pkg/constant"

	"github.com/apecloud/kbcli/pkg/printer"
	"github.com/apecloud/kbcli/pkg/types"
	"github.com/apecloud/kbcli/pkg/util"
	"github.com/apecloud/kbcli/pkg/util/prompt"
)

func getAddonVersion(addon *extensionsv1alpha1.Addon) string {
	if len(addon.Labels) == 0 {
		return ""
	}
	// get version by addon version label
	if version, ok := addon.Labels[types.AddonVersionLabelKey]; ok {
		return version
	}
	// get version by app version label
	if version, ok := addon.Labels[constant.AppVersionLabelKey]; ok {
		return version
	}
	return ""
}

func uniqueByName(objects []searchResult) []searchResult {
	seen := make(map[string]bool)
	var unique []searchResult
	for _, obj := range objects {
		if _, ok := seen[obj.addon.Name]; !ok {
			seen[obj.addon.Name] = true
			unique = append(unique, obj)
		}
	}
	return unique
}

func checkAddonInstalled(objects *[]searchResult, o *addonListOpts) error {
	// list installed addons
	var installedAddons []string
	o.Print = false
	// get and output the result
	o.Print = false
	r, _ := o.Run()
	if r == nil {
		return nil
	}
	infos, err := r.Infos()
	if err != nil {
		return err
	}
	if len(infos) == 0 {
		fmt.Fprintln(o.IOStreams.Out, "No installed addon found")
		return nil
	}
	for _, info := range infos {
		listItem := &extensionsv1alpha1.Addon{}
		obj := info.Object.(*unstructured.Unstructured)
		err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, listItem)
		if err != nil {
			return err
		}
		installedAddons = append(installedAddons, listItem.Name)
	}

	// mark installed on addons from the list result
	sort.Strings(installedAddons)
	sort.Slice(*objects, func(i, j int) bool {
		return (*objects)[i].addon.Name < (*objects)[j].addon.Name
	})
	for i, j := 0, 0; i < len(installedAddons) && j < len(*objects); {
		switch {
		case installedAddons[i] == (*objects)[j].addon.Name:
			{
				(*objects)[j].isInstalled = true
				i++
				j++
			}
		case installedAddons[i] < (*objects)[j].addon.Name:
			{
				i++
			}
		case installedAddons[i] > (*objects)[j].addon.Name:
			{
				j++
			}
		}
	}

	// sort by the priority of 'uninstalled < installed'
	sort.Slice(*objects, func(i, j int) bool {
		return (*objects)[j].isInstalled
	})
	return nil
}

func CheckAddonUsedByCluster(dynamic dynamic.Interface, addons []string, in io.Reader) error {
	labelSelecotor := util.BuildClusterLabel("", addons)
	list, err := dynamic.Resource(types.ClusterGVR()).Namespace(metav1.NamespaceAll).List(context.Background(), metav1.ListOptions{LabelSelector: labelSelecotor})
	if err != nil {
		return err
	}
	if list != nil && len(list.Items) != 0 {
		msg := "There are addons are being used by clusters:\n"
		usedAddons := make(map[string]struct{})
		for _, item := range list.Items {
			var cluster v1alpha1.Cluster
			if err = runtime.DefaultUnstructuredConverter.FromUnstructured(item.Object, &cluster); err != nil {
				return err
			}
			msg += fmt.Sprintf("cluster name: %s namespace: %s addon: %s\n", printer.BoldGreen(item.GetName()), printer.BoldYellow(item.GetNamespace()), printer.BoldRed(cluster.Labels[constant.ClusterDefLabelKey]))
			labels := item.GetLabels()
			usedAddons[labels[constant.ClusterDefLabelKey]] = struct{}{}
		}
		msg += fmt.Sprintf("In used addons [%s] to be deleted", printer.BoldRed(strings.Join(maps.Keys(usedAddons), ",")))
		return prompt.Confirm(maps.Keys(usedAddons), in, msg, "")
	}
	return nil
}

// addonNameCompletionFunc provides name auto-completion for addons
func addonNameCompletionFunc(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	fmt.Printf("toComplete: %s\n", toComplete)
	var addonDir string
	var err error
	addonDir, err = util.GetCliAddonDir()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	// Retrieve all addon names
	allAddons, err := getAllAddonNames(addonDir)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	matches := fuzzyMatch(allAddons, toComplete)
	return matches, cobra.ShellCompDirectiveNoFileComp
}

// getAllAddonNames retrieves all addon names from the given directory
func getAllAddonNames(dir string) ([]string, error) {
	var addonNames []string
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		// Skip hidden directories
		if strings.HasPrefix(d.Name(), ".") && d.IsDir() {
			return filepath.SkipDir
		}
		if d.IsDir() {
			return nil
		}
		if strings.HasSuffix(strings.ToLower(d.Name()), ".yaml") {
			content, err := os.ReadFile(path)
			if err != nil {
				klog.V(2).Infof("read file %s error: %v", path, err)
				return nil
			}
			addon := &extensionsv1alpha1.Addon{}
			if yaml.Unmarshal(content, addon) != nil {
				return nil
			}
			if addon.Kind == "Addon" && addon.Name != "" {
				addonNames = append(addonNames, addon.Name)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	// Remove duplicates
	nameSet := make(map[string]struct{})
	for _, n := range addonNames {
		nameSet[n] = struct{}{}
	}
	var unique []string
	for n := range nameSet {
		unique = append(unique, n)
	}
	return unique, nil
}

// fuzzyMatch performs simple substring fuzzy matching on names
func fuzzyMatch(names []string, prefix string) []string {
	if prefix == "" {
		return names
	}
	var matches []string
	lp := strings.ToLower(prefix)
	for _, n := range names {
		if strings.Contains(strings.ToLower(n), lp) {
			matches = append(matches, n)
		}
	}
	return matches
}
