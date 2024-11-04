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

package cluster

import (
	"fmt"
	"net/http"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/version"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	"k8s.io/cli-runtime/pkg/resource"
	fakediscovery "k8s.io/client-go/discovery/fake"
	"k8s.io/client-go/kubernetes/scheme"
	clientfake "k8s.io/client-go/rest/fake"
	cmdtesting "k8s.io/kubectl/pkg/cmd/testing"

	kbappsv1 "github.com/apecloud/kubeblocks/apis/apps/v1"

	"github.com/apecloud/kbcli/pkg/action"
	"github.com/apecloud/kbcli/pkg/cluster"
	"github.com/apecloud/kbcli/pkg/printer"
	"github.com/apecloud/kbcli/pkg/testing"
	"github.com/apecloud/kbcli/pkg/types"
)

var _ = Describe("Cluster", func() {
	const (
		clusterType = "apecloud-mysql"
		clusterName = "test"
		namespace   = "default"
	)
	var (
		tf            *cmdtesting.TestFactory
		streams       genericiooptions.IOStreams
		createOptions *action.CreateOptions
		mockClient    = func(data runtime.Object) *cmdtesting.TestFactory {
			tf = testing.NewTestFactory(testing.Namespace)
			codec := scheme.Codecs.LegacyCodec(scheme.Scheme.PrioritizedVersionsAllGroups()...)
			tf.UnstructuredClient = &clientfake.RESTClient{
				NegotiatedSerializer: resource.UnstructuredPlusDefaultContentConfig().NegotiatedSerializer,
				GroupVersion:         schema.GroupVersion{Group: types.AppsAPIGroup, Version: types.AppsAPIVersion},
				Resp:                 &http.Response{StatusCode: http.StatusOK, Header: cmdtesting.DefaultHeader(), Body: cmdtesting.ObjBody(codec, data)},
			}
			tf.Client = tf.UnstructuredClient
			tf.FakeDynamicClient = testing.FakeDynamicClient(data)
			tf.WithDiscoveryClient(cmdtesting.NewFakeCachedDiscoveryClient())
			return tf
		}
	)

	BeforeEach(func() {
		_ = kbappsv1.AddToScheme(scheme.Scheme)
		_ = metav1.AddMetaToScheme(scheme.Scheme)
		streams, _, _, _ = genericiooptions.NewTestIOStreams()
		tf = mockClient(testing.FakeCompDef())
		createOptions = &action.CreateOptions{
			IOStreams: streams,
			Factory:   tf,
		}
	})

	AfterEach(func() {
		tf.Cleanup()
	})

	Context("create", func() {
		It("without name", func() {
			o, err := NewSubCmdsOptions(createOptions, clusterType)
			Expect(err).Should(Succeed())
			Expect(o).ShouldNot(BeNil())
			Expect(o.ChartInfo).ShouldNot(BeNil())
			o.Format = printer.YAML

			Expect(o.CreateOptions.Complete()).To(Succeed())
			o.Client = testing.FakeClientSet()
			fakeDiscovery1, _ := o.Client.Discovery().(*fakediscovery.FakeDiscovery)
			fakeDiscovery1.FakedServerVersion = &version.Info{Major: "1", Minor: "27", GitVersion: "v1.27.0"}
			Expect(o.Complete(nil)).To(Succeed())
			Expect(o.Validate()).To(Succeed())
			Expect(o.Name).ShouldNot(BeEmpty())
			Expect(o.Run()).Should(Succeed())
		})
		It("with schedulingPolicy", func() {
			o, err := NewSubCmdsOptions(createOptions, clusterType)
			o.Tenancy = "SharedNode"
			o.TopologyKeys = []string{"zone", "hostname"}
			o.NodeLabels = map[string]string{"environment": "environment", "region": "region"}
			o.TolerationsRaw = []string{"key=value:effect", " key:effect"}
			o.PodAntiAffinity = "Preferred"

			Expect(err).Should(Succeed())
			Expect(o).ShouldNot(BeNil())
			Expect(o.ChartInfo).ShouldNot(BeNil())
			o.Format = printer.YAML

			Expect(o.CreateOptions.Complete()).To(Succeed())
			o.Client = testing.FakeClientSet()
			fakeDiscovery1, _ := o.Client.Discovery().(*fakediscovery.FakeDiscovery)
			fakeDiscovery1.FakedServerVersion = &version.Info{Major: "1", Minor: "27", GitVersion: "v1.27.0"}
			Expect(o.Complete(nil)).To(Succeed())
			Expect(o.Validate()).To(Succeed())
			Expect(o.Name).ShouldNot(BeEmpty())
			Expect(o.Run()).Should(Succeed())
		})
	})

	Context("create validate", func() {
		var o *CreateSubCmdsOptions
		BeforeEach(func() {
			o = &CreateSubCmdsOptions{
				CreateOptions: &action.CreateOptions{
					Factory:   tf,
					Namespace: namespace,
					Dynamic:   tf.FakeDynamicClient,
					IOStreams: streams,
				},
			}
			o.Name = "mycluster"
			o.ChartInfo, _ = cluster.BuildChartInfo(clusterType)
		})

		It("can validate the cluster name must begin with a letter and can only contain lowercase letters, numbers, and '-'.", func() {
			type fn func()
			var succeed = func(name string) fn {
				return func() {
					o.Name = name
					Expect(o.Validate()).Should(Succeed())
				}
			}
			var failed = func(name string) fn {
				return func() {
					o.Name = name
					Expect(o.Validate()).Should(HaveOccurred())
				}
			}
			// more case to add
			invalidCase := []string{
				"1abcd", "abcd-", "-abcd", "abc#d", "ABCD", "*&(&%",
			}

			validCase := []string{
				"abcd", "abcd1", "a1-2b-3d",
			}

			for i := range invalidCase {
				failed(invalidCase[i])
			}

			for i := range validCase {
				succeed(validCase[i])
			}

		})

		It("can validate whether the name is not longer than 16 characters when create a new cluster", func() {
			Expect(len(o.Name)).Should(BeNumerically("<=", 16))
			Expect(o.Validate()).Should(Succeed())
			moreThan16 := 17
			bytes := make([]byte, 0)
			var clusterNameMoreThan16 string
			for i := 0; i < moreThan16; i++ {
				bytes = append(bytes, byte(i%26+'a'))
			}
			clusterNameMoreThan16 = string(bytes)
			Expect(len(clusterNameMoreThan16)).Should(BeNumerically(">", 16))
			o.Name = clusterNameMoreThan16
			Expect(o.Validate()).Should(HaveOccurred())
		})

	})

	Context("delete cluster", func() {
		var o *action.DeleteOptions

		BeforeEach(func() {
			tf = testing.NewTestFactory(namespace)

			_ = kbappsv1.AddToScheme(scheme.Scheme)
			codec := scheme.Codecs.LegacyCodec(scheme.Scheme.PrioritizedVersionsAllGroups()...)
			clusters := testing.FakeClusterList()

			tf.UnstructuredClient = &clientfake.RESTClient{
				GroupVersion:         schema.GroupVersion{Group: types.AppsAPIGroup, Version: types.AppsAPIVersion},
				NegotiatedSerializer: resource.UnstructuredPlusDefaultContentConfig().NegotiatedSerializer,
				Client: clientfake.CreateHTTPClient(func(req *http.Request) (*http.Response, error) {
					return &http.Response{StatusCode: http.StatusOK, Header: cmdtesting.DefaultHeader(), Body: cmdtesting.ObjBody(codec, &clusters.Items[0])}, nil
				}),
			}

			tf.Client = tf.UnstructuredClient
			o = &action.DeleteOptions{
				Factory:     tf,
				IOStreams:   streams,
				GVR:         types.ClusterGVR(),
				AutoApprove: true,
			}
		})

		It("validata delete cluster by name", func() {
			Expect(deleteCluster(o, []string{})).Should(HaveOccurred())
			Expect(deleteCluster(o, []string{clusterName})).Should(Succeed())
			o.LabelSelector = fmt.Sprintf("clusterdefinition.kubeblocks.io/name=%s", testing.ClusterDefName)
			// todo:  there is an issue with rendering the name of the "info" element, and efforts are being made to resolve it.
			// Expect(deleteCluster(o, []string{})).Should(Succeed())
			Expect(deleteCluster(o, []string{clusterName})).Should(HaveOccurred())
		})

	})
	It("delete", func() {
		cmd := NewDeleteCmd(tf, streams)
		Expect(cmd).ShouldNot(BeNil())
	})

	It("cluster", func() {
		cmd := NewClusterCmd(tf, streams)
		Expect(cmd).ShouldNot(BeNil())
		Expect(cmd.HasSubCommands()).To(BeTrue())
	})

	It("connect", func() {
		cmd := NewConnectCmd(tf, streams)
		Expect(cmd).ShouldNot(BeNil())
	})

	It("list-logs-type", func() {
		cmd := NewListLogsCmd(tf, streams)
		Expect(cmd).ShouldNot(BeNil())
	})

	It("logs", func() {
		cmd := NewLogsCmd(tf, streams)
		Expect(cmd).ShouldNot(BeNil())
	})
})
