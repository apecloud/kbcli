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

package addon

import (
	"context"
	"fmt"
	"io"
	"regexp"
	"sort"

	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/kubectl/pkg/util/templates"

	kbv1alpha1 "github.com/apecloud/kubeblocks/apis/apps/v1alpha1"
	"github.com/apecloud/kubeblocks/apis/extensions/v1alpha1"

	"github.com/apecloud/kbcli/pkg/util/prompt"

	"github.com/apecloud/kbcli/pkg/types"
	"github.com/apecloud/kbcli/pkg/util"
)

const (
	versionPattern         = `(\d+\.\d+\.\d+(?:-[a-zA-Z0-9.-]+)?(?:\+[a-zA-Z0-9.-]+)?)$`
	helmReleaseNameKey     = "meta.helm.sh/release-name"
	helmReleaseNamePrefix  = "kb-addon-"
	helmResourcePolicyKey  = "helm.sh/resource-policy"
	helmResourcePolicyKeep = "keep"

	versionErrorTemplate = "failed to retrieve versions for resource %s: %v"
)

// GVRsToPurge Resource types to be processed for deletion
var GVRsToPurge = []schema.GroupVersionResource{
	types.CompDefGVR(),
	types.ConfigmapGVR(),
	types.ConfigConstraintGVR(),
	types.ConfigConstraintOldGVR(),
}

var addonPurgeResourcesExample = templates.Examples(`
	# Purge specific versions of redis addon resources
	kbcli addon purge redis --versions=0.9.1,0.9.2

	# Purge all unused and outdated resources of redis addon
	kbcli addon purge redis --all

	# Print the resources that would be purged, and no resource is actually purged
	kbcli addon purge redis --dry-run
`)

// InUseVersionInfo defines a struct to hold version and associated cluster name
type InUseVersionInfo struct {
	Version          string
	ClusterName      string
	ClusterNamespace string
}

type purgeResourcesOption struct {
	*baseOption
	name          string
	versions      []string
	versionsInUse []InUseVersionInfo
	all           bool
	dryRun        bool
	autoApprove   bool

	// if set to true, the newest resources will also be deleted, and this flag is not open to user, only used to delete all the resources while addon uninstalling.
	deleteNewestVersion bool
}

func newPurgeResourcesOption(f cmdutil.Factory, streams genericiooptions.IOStreams) *purgeResourcesOption {
	return &purgeResourcesOption{
		baseOption: &baseOption{
			Factory:   f,
			IOStreams: streams,
			GVR:       types.AddonGVR(),
		},
		all:                 false,
		deleteNewestVersion: false,
		autoApprove:         false,
	}
}

func newPurgeResourcesCmd(f cmdutil.Factory, streams genericiooptions.IOStreams) *cobra.Command {
	o := newPurgeResourcesOption(f, streams)
	cmd := &cobra.Command{
		Use:               "purge",
		Short:             "Purge the sub-resources of specified addon and versions",
		Example:           addonPurgeResourcesExample,
		ValidArgsFunction: util.ResourceNameCompletionFunc(f, o.GVR),
		Run: func(cmd *cobra.Command, args []string) {
			util.CheckErr(o.baseOption.complete())
			util.CheckErr(o.Complete(args))
			util.CheckErr(o.Validate())
			util.CheckErr(o.Run())
		},
	}
	cmd.Flags().StringSliceVar(&o.versions, "versions", nil, "Specify the versions of resources to purge.")
	cmd.Flags().BoolVar(&o.all, "all", false, "If set to true, all resources will be purged, including those that are unused and not the newest version.")
	cmd.Flags().BoolVar(&o.dryRun, "dry-run", false, "If set to true, only print the resources that would be purged, and no resource is actually purged.")
	cmd.Flags().BoolVar(&o.autoApprove, "auto-approve", false, "Skip interactive approval before deleting")

	return cmd
}

func (o *purgeResourcesOption) Complete(args []string) error {
	if args == nil {
		return fmt.Errorf("no addon provided; please specify the name of addon")
	}
	o.name = args[0]
	versions, err := o.getExistedVersions(o.name)
	if err != nil {
		return fmt.Errorf(versionErrorTemplate, o.name, err)
	}
	newestVersion, err := o.getNewestVersion(o.name)
	if err != nil {
		return fmt.Errorf(versionErrorTemplate, o.name, err)
	}
	o.versionsInUse, err = o.getInUseVersions(o.name)
	if err != nil {
		return fmt.Errorf(versionErrorTemplate, o.name, err)
	}

	// If --all flag is set, gather versions that should be purged
	if o.all {
		for k := range versions {
			if !isVersionInUse(k, o.versionsInUse) && k != newestVersion {
				o.versions = append(o.versions, k)
			}
		}
		if o.deleteNewestVersion {
			o.versions = append(o.versions, newestVersion)
		}
	}
	return nil
}

func (o *purgeResourcesOption) Validate() error {
	if !o.all && o.versions == nil {
		return fmt.Errorf("no versions specified and --all flag is not set; please specify versions or use --all")
	}
	versions, err := o.getExistedVersions(o.name)
	if err != nil {
		return fmt.Errorf(versionErrorTemplate, o.name, err)
	}
	newestVersion, err := o.getNewestVersion(o.name)
	if err != nil {
		return fmt.Errorf(versionErrorTemplate, o.name, err)
	}
	// Validate if versions are correct and not in use
	for _, v := range o.versions {
		if !versions[v] {
			return fmt.Errorf("specified version %s does not exist for resource %s", v, o.name)
		}
		if !o.deleteNewestVersion && v == newestVersion {
			return fmt.Errorf("specified version %s cannot be purged as it is the newest version", v)
		}
		if isVersionInUse(v, o.versionsInUse) {
			return fmt.Errorf("specified version %s cannot be purged as it is currently in use", v)
		}
	}

	if newestVersion != "" {
		fmt.Fprintf(o.Out, "The following version is the newest version:\n  - %s\n", newestVersion)
	}
	if len(o.versionsInUse) > 0 {
		fmt.Fprintf(o.Out, "The following versions are currently in use:\n")
		var lastVersion string
		for _, v := range o.versionsInUse {
			if v.Version != lastVersion {
				fmt.Fprintf(o.Out, "  - Version: %s\n", v.Version)
				lastVersion = v.Version
			}
			fmt.Fprintf(o.Out, "    - Cluster: %s, Namespace: %s\n", v.ClusterName, v.ClusterNamespace)
		}
	}
	if len(o.versions) > 0 {
		fmt.Fprintf(o.Out, "The following versions will be purged:\n")
		for _, version := range o.versions {
			fmt.Fprintf(o.Out, "  - %s\n", version)
		}
	} else {
		fmt.Fprintf(o.Out, "No resources need to be purged:\n")
	}

	return nil
}

func (o *purgeResourcesOption) Run() error {
	return o.cleanSubResources(o.name, o.versions)
}

// isVersionInUse checks if a version is in use based on current clusters
func isVersionInUse(version string, inUseVersions []InUseVersionInfo) bool {
	for _, inUse := range inUseVersions {
		if inUse.Version == version {
			return true
		}
	}
	return false
}

// extractVersion extracts the version from a resource name using the provided regex pattern
func extractVersion(name string) string {
	versionRegex := regexp.MustCompile(versionPattern)
	return versionRegex.FindString(name)
}

// getExistedVersions gets all the existed versions of the specified addon by listing the component definitions
func (o *purgeResourcesOption) getExistedVersions(addonName string) (map[string]bool, error) {
	resources, err := o.Dynamic.Resource(types.CompDefGVR()).Namespace(metav1.NamespaceAll).List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list resources for %s: %w", types.CompDefGVR(), err)
	}
	totalVersions := make(map[string]bool)
	for _, item := range resources.Items {
		annotations := item.GetAnnotations()
		if annotations[helmReleaseNameKey] != helmReleaseNamePrefix+addonName {
			continue
		}

		version := extractVersion(item.GetName())
		if version != "" {
			totalVersions[version] = true
		}
	}

	return totalVersions, nil
}

// getNewestVersion retrieves the newest version of the addon
func (o *purgeResourcesOption) getNewestVersion(addonName string) (string, error) {
	addon := &v1alpha1.Addon{}
	err := util.GetK8SClientObject(o.Dynamic, addon, types.AddonGVR(), "", addonName)
	if err != nil {
		return "", fmt.Errorf("failed to get addon: %w", err)
	}
	return getAddonVersion(addon), nil
}

// getInUseVersions retrieves the versions of resources that are currently in use, along with their associated cluster names
func (o *purgeResourcesOption) getInUseVersions(addonName string) ([]InUseVersionInfo, error) {
	var inUseVersions []InUseVersionInfo
	labelSelector := util.BuildClusterLabel("", []string{addonName})
	clusterList, err := o.Dynamic.Resource(types.ClusterGVR()).Namespace(metav1.NamespaceAll).List(context.Background(), metav1.ListOptions{LabelSelector: labelSelector})
	if err != nil {
		return nil, fmt.Errorf("failed to list clusters: %w", err)
	}
	if clusterList != nil && len(clusterList.Items) > 0 {
		for _, item := range clusterList.Items {
			var cluster kbv1alpha1.Cluster
			if err = runtime.DefaultUnstructuredConverter.FromUnstructured(item.Object, &cluster); err != nil {
				return nil, fmt.Errorf("failed to convert cluster to structured object: %w", err)
			}
			for _, spec := range cluster.Spec.ComponentSpecs {
				version := extractVersion(spec.ComponentDef)
				if version != "" {
					inUseVersions = append(inUseVersions, InUseVersionInfo{
						Version:          version,
						ClusterName:      cluster.Name,
						ClusterNamespace: cluster.Namespace,
					})
				}
			}
		}
	}
	sort.SliceStable(inUseVersions, func(i, j int) bool {
		return inUseVersions[i].Version < inUseVersions[j].Version
	})
	return inUseVersions, nil
}

// cleanSubResources Purges specified addon resources
func (o *purgeResourcesOption) cleanSubResources(addon string, versionsToPurge []string) error {
	versions := make(map[string]bool)
	for _, v := range versionsToPurge {
		versions[v] = true
	}
	var resourcesToPurge []struct {
		gvr       schema.GroupVersionResource
		name      string
		namespace string
		version   string
	}
	// Check if resources are available to purge
	if len(versionsToPurge) == 0 {
		return nil
	}
	fmt.Fprintf(o.Out, "The following resources will be purged:\n")
	// Gather the resources to purge and print
	for _, gvr := range GVRsToPurge {
		resources, err := o.Dynamic.Resource(gvr).Namespace(metav1.NamespaceAll).List(context.Background(), metav1.ListOptions{})
		if err != nil {
			return fmt.Errorf("failed to list resources for %s: %w", gvr.Resource, err)
		}

		for _, item := range resources.Items {
			// Skip resources not belong to specified addon
			annotations := item.GetAnnotations()
			if annotations[helmReleaseNameKey] != helmReleaseNamePrefix+addon {
				continue
			}

			// Skip resources of other versions.
			name := item.GetName()
			extractedVersion := extractVersion(name)
			if extractedVersion == "" || !versions[extractedVersion] {
				continue
			}

			// Skip resources if the resource doesn't have the annotation helm.sh/resource-policy: keep
			if annotations[helmResourcePolicyKey] != helmResourcePolicyKeep {
				continue
			}

			// Cache the resource to purge
			resourcesToPurge = append(resourcesToPurge, struct {
				gvr       schema.GroupVersionResource
				name      string
				namespace string
				version   string
			}{
				gvr:       gvr,
				name:      name,
				namespace: item.GetNamespace(),
				version:   extractedVersion,
			})

			fmt.Fprintf(o.Out, "  - %s/%s\n", gvr.Resource, name)
		}
	}

	if o.dryRun {
		return nil
	}

	// Confirm Purge
	if !o.autoApprove {
		if err := confirmPurge(o.In, o.Out); err != nil {
			return err
		}
	}

	// Perform actual deletion
	for _, resource := range resourcesToPurge {
		err := o.Dynamic.Resource(resource.gvr).Namespace(resource.namespace).Delete(context.Background(), resource.name, metav1.DeleteOptions{})
		if err != nil {
			return fmt.Errorf("failed to purge resource %s/%s: %w", resource.gvr.Resource, resource.name, err)
		}
		fmt.Fprintf(o.Out, "Purged resource: %s/%s\n", resource.gvr.Resource, resource.name)
	}

	return nil
}

func confirmPurge(in io.Reader, out io.Writer) error {
	const confirmStr = "y"
	_, err := prompt.NewPrompt(fmt.Sprintf("Do you want to proceed with purging the above resources? Please type \"%s\" to confirm:", confirmStr),
		func(input string) error {
			if input != confirmStr {
				fmt.Fprintf(out, "Purge operation aborted.\n")
			}
			return nil
		}, in).Run()
	return err
}
