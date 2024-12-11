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
	"bytes"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	clientfake "k8s.io/client-go/rest/fake"
	clienttesting "k8s.io/client-go/testing"
	cmdtesting "k8s.io/kubectl/pkg/cmd/testing"

	kbv1alpha1 "github.com/apecloud/kubeblocks/apis/apps/v1alpha1"
	v1alpha1 "github.com/apecloud/kubeblocks/apis/extensions/v1alpha1"
	"github.com/apecloud/kubeblocks/pkg/constant"

	"github.com/apecloud/kbcli/pkg/testing"
	"github.com/apecloud/kbcli/pkg/types"
)

var _ = Describe("delete_resources_with_version test", func() {
	var (
		streams              genericiooptions.IOStreams
		tf                   *cmdtesting.TestFactory
		bufOut, bufErr       *bytes.Buffer
		addonName            = "redis"
		newestVersion        = "0.9.3"
		inUseVersion         = "0.9.2"
		unusedVersion        = "0.9.1"
		testAddonGVR         = types.AddonGVR()
		testCompDefGVR       = types.CompDefGVR()
		testClusterGVR       = types.ClusterGVR()
		testUnusedConfigGVR  = types.ConfigmapGVR()
		testResourceAnnotKey = helmReleaseNameKey
	)

	// Helper functions to create fake resources
	createAddon := func(name, version string) *unstructured.Unstructured {
		addon := &v1alpha1.Addon{
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
				Labels: map[string]string{
					types.AddonVersionLabelKey: version,
				},
			},
			Spec: v1alpha1.AddonSpec{
				Version: version,
			},
		}
		obj, _ := runtime.DefaultUnstructuredConverter.ToUnstructured(addon)
		u := &unstructured.Unstructured{Object: obj}
		u.SetGroupVersionKind(testAddonGVR.GroupVersion().WithKind("Addon"))
		return u
	}

	createComponentDef := func(name, addon, version string) *unstructured.Unstructured {
		u := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": testCompDefGVR.GroupVersion().String(),
				"kind":       "ComponentDefinition",
				"metadata": map[string]interface{}{
					"name": name + "-" + version,
					"annotations": map[string]interface{}{
						testResourceAnnotKey:  helmReleaseNamePrefix + addon,
						helmResourcePolicyKey: helmResourcePolicyKeep,
					},
				},
			},
		}
		return u
	}

	createCluster := func(name, addon, version string) *unstructured.Unstructured {
		cluster := &kbv1alpha1.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
				Labels: map[string]string{
					constant.ClusterDefLabelKey: addon,
				},
			},
			Spec: kbv1alpha1.ClusterSpec{
				ComponentSpecs: []kbv1alpha1.ClusterComponentSpec{
					{
						ComponentDef: fmt.Sprintf("redis-%s", version),
					},
				},
			},
		}
		obj, _ := runtime.DefaultUnstructuredConverter.ToUnstructured(cluster)
		u := &unstructured.Unstructured{Object: obj}
		u.SetGroupVersionKind(testClusterGVR.GroupVersion().WithKind("Cluster"))
		return u
	}

	createUnusedConfig := func(addon, version string) *unstructured.Unstructured {
		u := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": testUnusedConfigGVR.GroupVersion().String(),
				"kind":       "ConfigMap",
				"metadata": map[string]interface{}{
					"name": "config-" + unusedVersion,
					"annotations": map[string]interface{}{
						testResourceAnnotKey:  helmReleaseNamePrefix + addon,
						helmResourcePolicyKey: helmResourcePolicyKeep,
					},
				},
			},
		}
		return u
	}

	BeforeEach(func() {
		bufOut = new(bytes.Buffer)
		bufErr = new(bytes.Buffer)
		streams = genericiooptions.IOStreams{Out: bufOut, ErrOut: bufErr}
		tf = cmdtesting.NewTestFactory().WithNamespace(testNamespace)
		tf.FakeDynamicClient = testing.FakeDynamicClient()
		tf.Client = &clientfake.RESTClient{}

		// Populate dynamic client with test resources
		// Addon
		_, _ = tf.FakeDynamicClient.Invokes(clienttesting.NewCreateAction(testAddonGVR, "", createAddon(addonName, newestVersion)), nil)

		// ComponentDefs with different versions
		_, _ = tf.FakeDynamicClient.Invokes(clienttesting.NewCreateAction(testCompDefGVR, types.DefaultNamespace, createComponentDef("redis", addonName, newestVersion)), nil)
		_, _ = tf.FakeDynamicClient.Invokes(clienttesting.NewCreateAction(testCompDefGVR, types.DefaultNamespace, createComponentDef("redis", addonName, inUseVersion)), nil)
		_, _ = tf.FakeDynamicClient.Invokes(clienttesting.NewCreateAction(testCompDefGVR, types.DefaultNamespace, createComponentDef("redis", addonName, unusedVersion)), nil)

		// Cluster using inUseVersion
		_, _ = tf.FakeDynamicClient.Invokes(clienttesting.NewCreateAction(testClusterGVR, testNamespace, createCluster("test-cluster", addonName, inUseVersion)), nil)

		// Unused config resource for unusedVersion
		_, _ = tf.FakeDynamicClient.Invokes(clienttesting.NewCreateAction(testUnusedConfigGVR, types.DefaultNamespace, createUnusedConfig(addonName, unusedVersion)), nil)
	})

	AfterEach(func() {
		tf.Cleanup()
	})

	It("test delete_resources_with_versions cmd creation", func() {
		Expect(newDeleteResourcesCmd(tf, streams)).ShouldNot(BeNil())
	})

	It("test baseOption complete", func() {
		option := newDeleteResourcesOption(tf, streams)
		Expect(option).ShouldNot(BeNil())
		Expect(option.baseOption.complete()).Should(Succeed())
	})

	It("test no addon name provided", func() {
		option := newDeleteResourcesOption(tf, streams)
		err := option.Complete(nil)
		Expect(err).Should(HaveOccurred())
		Expect(err.Error()).Should(ContainSubstring("no addon provided"))
	})

	It("test no versions and no --all-unused-versions", func() {
		option := newDeleteResourcesOption(tf, streams)
		option.Dynamic = tf.FakeDynamicClient
		option.Factory = tf
		err := option.Complete([]string{addonName})
		Expect(err).ShouldNot(HaveOccurred())

		// Validate should fail due to no versions and no all-unused-versions
		err = option.Validate()
		Expect(err).Should(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("please specify versions or set --all-unused-versions to true"))
	})

	It("test specifying a non-existent version", func() {
		option := newDeleteResourcesOption(tf, streams)
		option.versions = []string{"1.0.0"}
		option.Dynamic = tf.FakeDynamicClient
		option.Factory = tf
		err := option.Complete([]string{addonName})
		Expect(err).ShouldNot(HaveOccurred())

		err = option.Validate()
		Expect(err).Should(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("does not exist"))
	})

	It("test specifying newest version without deleteNewestVersion flag", func() {
		option := newDeleteResourcesOption(tf, streams)
		option.versions = []string{newestVersion} // newest version
		option.Dynamic = tf.FakeDynamicClient
		option.Factory = tf
		err := option.Complete([]string{addonName})
		Expect(err).ShouldNot(HaveOccurred())

		err = option.Validate()
		Expect(err).Should(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("cannot be deleted as it is the newest version"))
	})

	It("test specifying an in-use version", func() {
		option := newDeleteResourcesOption(tf, streams)
		option.versions = []string{inUseVersion}
		option.Dynamic = tf.FakeDynamicClient
		option.Factory = tf
		err := option.Complete([]string{addonName})
		Expect(err).ShouldNot(HaveOccurred())

		err = option.Validate()
		Expect(err).Should(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("cannot be deleted as it is currently used"))
	})

	It("test specifying an unused old version directly", func() {
		option := newDeleteResourcesOption(tf, streams)
		option.versions = []string{unusedVersion}
		option.Dynamic = tf.FakeDynamicClient
		option.Factory = tf
		err := option.Complete([]string{addonName})
		Expect(err).ShouldNot(HaveOccurred())

		// Validate should succeed
		err = option.Validate()
		Expect(err).ShouldNot(HaveOccurred())

		// Run should delete resources associated with unusedVersion
		err = option.Run()
		Expect(err).ShouldNot(HaveOccurred())
		Expect(bufOut.String()).To(ContainSubstring("Deleted resource: configmaps/" + "config-" + unusedVersion))
	})

	It("test using --all-unused-versions", func() {
		option := newDeleteResourcesOption(tf, streams)
		option.allUnusedVersions = true
		option.Dynamic = tf.FakeDynamicClient
		option.Factory = tf
		err := option.Complete([]string{addonName})
		Expect(err).ShouldNot(HaveOccurred())

		// Validate should succeed now that we have automatically set unused versions
		err = option.Validate()
		Expect(err).ShouldNot(HaveOccurred())

		// Run should delete all unused and non-newest versions. In this case, unusedVersion = "0.9.1"
		err = option.Run()
		Expect(err).ShouldNot(HaveOccurred())
		Expect(bufOut.String()).To(ContainSubstring("Deleted resource: configmaps/" + "config-" + unusedVersion))
	})
})
