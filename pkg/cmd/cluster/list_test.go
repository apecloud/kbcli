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

package cluster

import (
	"bytes"
	"net/http"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	"k8s.io/cli-runtime/pkg/resource"
	"k8s.io/client-go/kubernetes/scheme"
	clientfake "k8s.io/client-go/rest/fake"
	cmdtesting "k8s.io/kubectl/pkg/cmd/testing"

	kbappsv1 "github.com/apecloud/kubeblocks/apis/apps/v1"
	appsv1alpha1 "github.com/apecloud/kubeblocks/apis/apps/v1alpha1"

	"github.com/apecloud/kbcli/pkg/cluster"
	"github.com/apecloud/kbcli/pkg/testing"
	"github.com/apecloud/kbcli/pkg/types"
)

var _ = Describe("list", func() {
	const (
		namespace             = "test"
		clusterName           = "test"
		clusterName1          = "test1"
		clusterName2          = "test2"
		verticalScalingReason = "VerticalScaling"
	)

	var (
		streams genericiooptions.IOStreams
		out     *bytes.Buffer
		tf      *cmdtesting.TestFactory
	)

	// httpResp returns a *http.Response for the given runtime.Object.
	httpResp := func(codec runtime.Codec, obj runtime.Object) *http.Response {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     cmdtesting.DefaultHeader(),
			Body:       cmdtesting.ObjBody(codec, obj),
		}
	}

	BeforeEach(func() {
		streams, _, out, _ = genericiooptions.NewTestIOStreams()
		tf = testing.NewTestFactory(namespace)

		// Prepare schemes
		Expect(kbappsv1.AddToScheme(scheme.Scheme)).Should(Succeed())
		codec := scheme.Codecs.LegacyCodec(scheme.Scheme.PrioritizedVersionsAllGroups()...)

		By("Creating test clusters and pods")
		// Prepare test data
		baseCluster := testing.FakeCluster(clusterName, namespace, metav1.Condition{
			Type:   appsv1alpha1.ConditionTypeApplyResources,
			Status: metav1.ConditionFalse,
			Reason: "HorizontalScaleFailed",
		})
		clusterWithVerticalScaling := testing.FakeCluster(clusterName1, namespace, metav1.Condition{
			Type:   appsv1alpha1.ConditionTypeReady,
			Status: metav1.ConditionFalse,
			Reason: verticalScalingReason,
		})
		clusterWithVerticalScaling.Status.Phase = kbappsv1.UpdatingClusterPhase

		clusterWithAbnormalPhase := testing.FakeCluster(clusterName2, namespace)
		clusterWithAbnormalPhase.Status.Phase = kbappsv1.AbnormalClusterPhase

		pods := testing.FakePods(3, namespace, clusterName)

		By("Configuring the fake REST client responses")
		tf.UnstructuredClient = &clientfake.RESTClient{
			GroupVersion:         schema.GroupVersion{Group: types.AppsAPIGroup, Version: types.AppsAPIVersion},
			NegotiatedSerializer: resource.UnstructuredPlusDefaultContentConfig().NegotiatedSerializer,
			Client: clientfake.CreateHTTPClient(func(req *http.Request) (*http.Response, error) {
				urlPrefix := "/api/v1/namespaces/" + namespace
				pathToResponse := map[string]*http.Response{
					"/namespaces/" + namespace + "/clusters":                 httpResp(codec, &kbappsv1.ClusterList{Items: []kbappsv1.Cluster{*baseCluster}}),
					"/namespaces/" + namespace + "/clusters/" + clusterName:  httpResp(codec, baseCluster),
					"/namespaces/" + namespace + "/clusters/" + clusterName1: httpResp(codec, clusterWithVerticalScaling),
					"/namespaces/" + namespace + "/clusters/" + clusterName2: httpResp(codec, clusterWithAbnormalPhase),
					"/namespaces/" + namespace + "/secrets":                  httpResp(codec, testing.FakeSecrets(namespace, clusterName)),
					"/api/v1/nodes/" + testing.NodeName:                      httpResp(codec, testing.FakeNode()),
					urlPrefix + "/services":                                  httpResp(codec, &corev1.ServiceList{}),
					urlPrefix + "/secrets":                                   httpResp(codec, testing.FakeSecrets(namespace, clusterName)),
					urlPrefix + "/pods":                                      httpResp(codec, pods),
					urlPrefix + "/events":                                    httpResp(codec, testing.FakeEvents()),
				}
				return pathToResponse[req.URL.Path], nil
			}),
		}

		tf.Client = tf.UnstructuredClient
		tf.FakeDynamicClient = testing.FakeDynamicClient(
			baseCluster,
			clusterWithVerticalScaling,
			clusterWithAbnormalPhase,
			testing.FakeClusterDef(),
		)
	})

	AfterEach(func() {
		By("Cleaning up the test factory")
		tf.Cleanup()
	})

	// Helper to run command and return output
	runCmd := func(cmd *cobra.Command, args ...string) string {
		out.Reset()
		cmd.Run(cmd, args)
		return out.String()
	}

	It("list clusters by name", func() {
		By("Running list command with cluster names")
		cmd := NewListCmd(tf, streams)
		output := runCmd(cmd, clusterName, clusterName1, clusterName2)

		By("Checking output for expected clusters and statuses")
		Expect(output).Should(ContainSubstring(clusterName))
		Expect(output).Should(ContainSubstring(string(appsv1alpha1.UpdatingClusterPhase)))
		Expect(output).Should(ContainSubstring(cluster.ConditionsError))
		Expect(output).Should(ContainSubstring(string(appsv1alpha1.AbnormalClusterPhase)))
	})

	It("list instances for a specific cluster", func() {
		By("Running list-instances command for a given cluster")
		cmd := NewListInstancesCmd(tf, streams)
		output := runCmd(cmd, clusterName)

		By("Checking output for expected node name in instances list")
		Expect(output).Should(ContainSubstring(testing.NodeName))
	})

	It("list components for a specific cluster", func() {
		By("Running list-components command for a given cluster")
		cmd := NewListComponentsCmd(tf, streams)
		output := runCmd(cmd, clusterName)

		By("Checking output for expected component name")
		Expect(output).Should(ContainSubstring(testing.ComponentName))
	})

	It("list events for a specific cluster", func() {
		By("Running list-events command for a given cluster")
		cmd := NewListEventsCmd(tf, streams)
		output := runCmd(cmd, clusterName)

		By("Verifying that multiple events are returned")
		Expect(len(strings.Split(strings.TrimSpace(output), "\n")) > 1).Should(BeTrue())
	})

	It("output wide with cluster name", func() {
		By("Setting output format to wide")
		cmd := NewListCmd(tf, streams)
		Expect(cmd.Flags().Set("output", "wide")).Should(Succeed())

		By("Running the command with a specific cluster")
		output := runCmd(cmd, clusterName)
		Expect(output).Should(ContainSubstring(clusterName))
	})

	It("output wide without specifying cluster names", func() {
		By("Setting output format to wide without arguments")
		cmd := NewListCmd(tf, streams)
		Expect(cmd.Flags().Set("output", "wide")).Should(Succeed())

		output := runCmd(cmd)
		Expect(output).Should(ContainSubstring(clusterName))
	})

	It("should list clusters sorted and filtered by status", func() {
		By("Preparing multiple clusters for sorting and filtering test")
		clusterA := testing.FakeCluster("clusterA", "ns1", metav1.Condition{
			Type:   appsv1alpha1.ConditionTypeReady,
			Status: metav1.ConditionTrue,
			Reason: "Ready",
		})
		clusterA.Status.Phase = kbappsv1.RunningClusterPhase
		clusterA.Spec.ClusterDef = "cd-mysql"

		clusterB := testing.FakeCluster("clusterB", "ns1", metav1.Condition{
			Type:   appsv1alpha1.ConditionTypeReady,
			Status: metav1.ConditionTrue,
			Reason: "Ready",
		})
		clusterB.Status.Phase = kbappsv1.CreatingClusterPhase
		clusterB.Spec.ClusterDef = "cd-mysql"

		clusterC := testing.FakeCluster("clusterC", "ns2", metav1.Condition{
			Type:   appsv1alpha1.ConditionTypeReady,
			Status: metav1.ConditionFalse,
			Reason: "Scaling",
		})
		clusterC.Status.Phase = kbappsv1.UpdatingClusterPhase
		clusterC.Spec.ClusterDef = "cd-postgres"

		codec := scheme.Codecs.LegacyCodec(scheme.Scheme.PrioritizedVersionsAllGroups()...)
		tf.UnstructuredClient = &clientfake.RESTClient{
			GroupVersion:         schema.GroupVersion{Group: types.AppsAPIGroup, Version: types.AppsAPIVersion},
			NegotiatedSerializer: resource.UnstructuredPlusDefaultContentConfig().NegotiatedSerializer,

			Client: clientfake.CreateHTTPClient(func(req *http.Request) (*http.Response, error) {
				pathToResponse := map[string]*http.Response{
					"/namespaces/" + namespace + "/clusters": httpResp(codec, &kbappsv1.ClusterList{
						Items: []kbappsv1.Cluster{*clusterA, *clusterB, *clusterC},
					}),
				}
				return pathToResponse[req.URL.Path], nil
			}),
		}

		tf.Client = tf.UnstructuredClient

		tf.FakeDynamicClient = testing.FakeDynamicClient(clusterA, clusterB, clusterC, testing.FakeClusterDef())

		By("Listing all clusters across all namespaces")
		cmd := NewListCmd(tf, streams)
		output := runCmd(cmd)

		lines := strings.Split(strings.TrimSpace(output), "\n")
		Expect(lines).To(ContainElements(
			ContainSubstring("clusterB"), // Creating
			ContainSubstring("clusterA"), // Running
			ContainSubstring("clusterC"), // Updating
		))

		// Extract indices for validation
		var bIndex, aIndex, cIndex int
		for i, line := range lines {
			switch {
			case strings.Contains(line, "clusterB"):
				bIndex = i
			case strings.Contains(line, "clusterA"):
				aIndex = i
			case strings.Contains(line, "clusterC"):
				cIndex = i
			}
		}

		Expect(bIndex).To(BeNumerically("<", aIndex)) // Creating < Running
		Expect(aIndex).To(BeNumerically("<", cIndex)) // Running < Updating

		By("Filtering clusters by status Running")
		out.Reset()
		cmd = NewListCmd(tf, streams)
		Expect(cmd.Flags().Set("status", "Running")).Should(Succeed())
		output = runCmd(cmd)

		// Only clusterA should remain (Running)
		Expect(output).To(ContainSubstring("clusterA"))
		Expect(output).NotTo(ContainSubstring("clusterB"))
		Expect(output).NotTo(ContainSubstring("clusterC"))
	})
})
