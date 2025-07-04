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

package cluster

import (
	"net/http"

	kbappsv1 "github.com/apecloud/kubeblocks/apis/apps/v1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/spf13/cobra"
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

	"github.com/apecloud/kbcli/pkg/action"
	"github.com/apecloud/kbcli/pkg/printer"
	"github.com/apecloud/kbcli/pkg/testing"
	"github.com/apecloud/kbcli/pkg/types"
)

var _ = Describe("create cluster by cluster type", func() {
	const (
		clusterType    = "apecloud-mysql"
		redisCluster   = "redis"
		redisComponent = "redis-cluster"
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

	It("create mysql cluster command", func() {
		By("create commands")
		cmds := buildCreateSubCmds(createOptions)
		Expect(cmds).ShouldNot(BeNil())
		Expect(cmds[0].HasFlags()).Should(BeTrue())

		By("create command options")
		o, err := NewSubCmdsOptions(createOptions, clusterType)
		Expect(err).Should(Succeed())
		Expect(o).ShouldNot(BeNil())
		Expect(o.ChartInfo).ShouldNot(BeNil())
		o.PodAntiAffinity = "Preferred"
		o.Tenancy = "SharedNode"

		By("complete")
		var mysqlCmd *cobra.Command
		for _, c := range cmds {
			if c.Name() == clusterType {
				mysqlCmd = c
				break
			}
		}
		o.Format = printer.YAML
		Expect(o.CreateOptions.Complete()).Should(Succeed())
		o.DryRun = "client"
		o.Client = testing.FakeClientSet()
		fakeDiscovery1, _ := o.Client.Discovery().(*fakediscovery.FakeDiscovery)
		fakeDiscovery1.FakedServerVersion = &version.Info{Major: "1", Minor: "27", GitVersion: "v1.27.0"}
		Expect(o.Complete(mysqlCmd)).Should(Succeed())
		Expect(o.Name).ShouldNot(BeEmpty())
		Expect(o.Values).ShouldNot(BeNil())
		Expect(o.ChartInfo.ClusterDef).Should(Equal(apeCloudMysql))

		By("validate")
		o.Dynamic = testing.FakeDynamicClient()
		Expect(o.Validate()).Should(Succeed())

		By("run")
		o.DryRun = "client"
		o.Client = testing.FakeClientSet()
		fakeDiscovery, _ := o.Client.Discovery().(*fakediscovery.FakeDiscovery)
		fakeDiscovery.FakedServerVersion = &version.Info{Major: "1", Minor: "27", GitVersion: "v1.27.0"}
		Expect(o.Run()).Should(Succeed())
	})

	It("create sharding cluster command", func() {
		By("create commands")
		cmds := buildCreateSubCmds(createOptions)
		Expect(cmds).ShouldNot(BeNil())
		Expect(cmds[0].HasFlags()).Should(BeTrue())

		By("create command options")
		o, err := NewSubCmdsOptions(createOptions, redisCluster)
		Expect(err).Should(Succeed())
		Expect(o).ShouldNot(BeNil())
		Expect(o.ChartInfo).ShouldNot(BeNil())
		o.PodAntiAffinity = "Preferred"
		o.Tenancy = "SharedNode"

		By("complete")
		var shardCmd *cobra.Command
		for _, c := range cmds {
			if c.Name() == redisCluster {
				shardCmd = c
				break
			}
		}

		o.Format = printer.YAML
		Expect(o.CreateOptions.Complete()).Should(Succeed())
		o.DryRun = "client"
		o.Client = testing.FakeClientSet()
		fakeDiscovery1, _ := o.Client.Discovery().(*fakediscovery.FakeDiscovery)
		fakeDiscovery1.FakedServerVersion = &version.Info{Major: "1", Minor: "27", GitVersion: "v1.27.0"}

		Expect(shardCmd.Flags().Set("mode", "cluster")).Should(Succeed())
		Expect(o.Complete(shardCmd)).Should(Succeed())
		Expect(o.Name).ShouldNot(BeEmpty())
		Expect(o.Values).ShouldNot(BeNil())
		Expect(o.ChartInfo.ComponentDef[0]).Should(Equal(redisComponent))

		By("validate")
		o.Dynamic = testing.FakeDynamicClient()
		Expect(o.Validate()).Should(Succeed())

		By("run")
		o.DryRun = "client"
		o.Client = testing.FakeClientSet()
		fakeDiscovery, _ := o.Client.Discovery().(*fakediscovery.FakeDiscovery)
		fakeDiscovery.FakedServerVersion = &version.Info{Major: "1", Minor: "27", GitVersion: "v1.27.0"}
		Expect(o.Run()).Should(Succeed())
	})
})
