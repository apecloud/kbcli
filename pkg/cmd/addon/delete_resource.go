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
	"regexp"

	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/kubectl/pkg/util/templates"

	kbv1alpha1 "github.com/apecloud/kubeblocks/apis/apps/v1alpha1"
	v1alpha1 "github.com/apecloud/kubeblocks/apis/extensions/v1alpha1"

	"github.com/apecloud/kbcli/pkg/types"
	"github.com/apecloud/kbcli/pkg/util"
)

const (
	versionPattern         = `(\d+\.\d+\.\d+(?:-[a-zA-Z0-9.-]+)?(?:\+[a-zA-Z0-9.-]+)?)$`
	helmReleaseNameKey     = "meta.helm.sh/release-name"
	helmReleaseNamePrefix  = "kb-addon-"
	helmResourcePolicyKey  = "helm.sh/resource-policy"
	helmResourcePolicyKeep = "keep"
)

// Resource types to be processed for deletion
var resourceToDelete = []schema.GroupVersionResource{
	types.CompDefGVR(),
	types.ConfigmapGVR(),
	types.ConfigConstraintGVR(),
	types.ConfigConstraintOldGVR(),
}

var addonDeleteResourcesExample = templates.Examples(`
	# Delete specific versions of redis addon resources
	kbcli addon delete-resources-with-version redis --versions=0.9.1,0.9.2

	# Delete all unused and outdated resources of redis addon
	kbcli addon delete-resources-with-version redis --all-unused-versions=true
`)

type deleteResourcesOption struct {
	*baseOption
	name              string
	versions          []string
	allUnusedVersions bool

	// if set to true, the newest resources will also be deleted, and this flag is not open to user, only used to delete all the resources while addon uninstalling.
	deleteNewestVersion bool
}

func newDeleteResourcesOption(f cmdutil.Factory, streams genericiooptions.IOStreams) *deleteResourcesOption {
	return &deleteResourcesOption{
		baseOption: &baseOption{
			Factory:   f,
			IOStreams: streams,
			GVR:       types.AddonGVR(),
		},
		allUnusedVersions:   false,
		deleteNewestVersion: false,
	}
}

func newDeleteResourcesCmd(f cmdutil.Factory, streams genericiooptions.IOStreams) *cobra.Command {
	o := newDeleteResourcesOption(f, streams)
	cmd := &cobra.Command{
		Use:               "delete-resources-with-version",
		Short:             "Delete the sub-resources of specified addon and versions",
		Example:           addonDeleteResourcesExample,
		ValidArgsFunction: util.ResourceNameCompletionFunc(f, o.GVR),
		Run: func(cmd *cobra.Command, args []string) {
			util.CheckErr(o.baseOption.complete())
			util.CheckErr(o.Complete(args))
			util.CheckErr(o.Validate())
			util.CheckErr(o.Run())
		},
	}
	cmd.Flags().StringSliceVar(&o.versions, "versions", nil, "Specify the versions of resources to delete.")
	cmd.Flags().BoolVar(&o.allUnusedVersions, "all-unused-versions", false, "If set to true, all the resources "+
		"which are not currently used and not with the newest version will be deleted.")
	return cmd
}

func (o *deleteResourcesOption) Complete(args []string) error {
	if args == nil {
		return fmt.Errorf("no addon provided; please specify the name of addon")
	}
	o.name = args[0]
	versions, err := o.getExistedVersions(o.name)
	if err != nil {
		return fmt.Errorf("failed to retrieve versions for resource %s: %v", o.name, err)
	}
	newestVersion, err := o.getNewestVersion(o.name)
	if err != nil {
		return fmt.Errorf("failed to retrieve version for resource %s: %v", o.name, err)
	}
	versionInUse, err := o.getInUseVersions(o.name)
	if err != nil {
		return fmt.Errorf("failed to retrieve versions for resource %s: %v", o.name, err)
	}
	if o.allUnusedVersions {
		for k := range versions {
			if !versionInUse[k] && k != newestVersion {
				o.versions = append(o.versions, k)
			}
		}
		if o.deleteNewestVersion {
			o.versions = append(o.versions, newestVersion)
		}
	}
	return nil
}

func (o *deleteResourcesOption) Validate() error {
	if o.allUnusedVersions {
		return nil
	}
	if o.versions == nil {
		return fmt.Errorf("no versions specified and --all-versions flag is not set; please specify versions or set --all-unused-versions to true")
	}
	versions, err := o.getExistedVersions(o.name)
	if err != nil {
		return fmt.Errorf("failed to retrieve versions for resource %s: %v", o.name, err)
	}
	newestVersion, err := o.getNewestVersion(o.name)
	if err != nil {
		return fmt.Errorf("failed to retrieve version for resource %s: %v", o.name, err)
	}
	versionsInUse, err := o.getInUseVersions(o.name)
	if err != nil {
		return fmt.Errorf("failed to retrieve versions for resource %s: %v", o.name, err)
	}
	for _, v := range o.versions {
		if !versions[v] {
			return fmt.Errorf("specified version %s does not exist for resource %s", v, o.name)
		}
		if !o.deleteNewestVersion && v == newestVersion {
			return fmt.Errorf("specified version %s cannot be deleted as it is the newest version", v)
		}
		if versionsInUse[v] {
			return fmt.Errorf("specified version %s cannot be deleted as it is currently used", v)
		}
	}
	return nil
}

func (o *deleteResourcesOption) Run() error {
	return o.cleanSubResources(o.name, o.versions)
}

// extractVersion extracts the version from a resource name using the provided regex pattern.
func extractVersion(name string) string {
	versionRegex := regexp.MustCompile(versionPattern)
	return versionRegex.FindString(name)
}

// getExistedVersions get all the existed versions of specified addon by listing the componentDef.
func (o *deleteResourcesOption) getExistedVersions(addonName string) (map[string]bool, error) {
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
func (o *deleteResourcesOption) getNewestVersion(addonName string) (string, error) {
	addon := &v1alpha1.Addon{}
	err := util.GetK8SClientObject(o.Dynamic, addon, types.AddonGVR(), "", addonName)
	if err != nil {
		return "", fmt.Errorf("failed to get addon: %w", err)
	}
	return getAddonVersion(addon), nil
}

// getInUseVersions retrieves the versions of resources that are currently in use.
func (o *deleteResourcesOption) getInUseVersions(addonName string) (map[string]bool, error) {
	InUseVersions := map[string]bool{}
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
					InUseVersions[version] = true
				}
			}
		}
	}

	return InUseVersions, nil
}

// cleanSubResources Cleans up specified addon resources.
func (o *deleteResourcesOption) cleanSubResources(addon string, versionsToDelete []string) error {
	versions := make(map[string]bool)
	for _, v := range versionsToDelete {
		versions[v] = true
	}

	// Iterate through each resource type
	for _, gvr := range resourceToDelete {
		// List all resources of the current type
		resources, err := o.Dynamic.Resource(gvr).Namespace(metav1.NamespaceAll).List(context.Background(), metav1.ListOptions{})
		if err != nil {
			return fmt.Errorf("failed to list resources for %s: %w", gvr.Resource, err)
		}

		// Process each resource in the list
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

			// Delete the resource if it passes all checks, and only print msg when user calling.
			if !o.deleteNewestVersion {
				err := o.Dynamic.Resource(gvr).Namespace(item.GetNamespace()).Delete(context.Background(), name, metav1.DeleteOptions{})
				if err != nil {
					return fmt.Errorf("failed to delete resource %s/%s: %w", gvr.Resource, name, err)
				}
				fmt.Fprintf(o.Out, "Deleted resource: %s/%s\n", gvr.Resource, name)
			}
		}
	}

	return nil
}
