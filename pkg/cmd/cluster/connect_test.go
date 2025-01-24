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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	"k8s.io/cli-runtime/pkg/resource"
	"k8s.io/client-go/kubernetes/scheme"
	clientfake "k8s.io/client-go/rest/fake"
	cmdtesting "k8s.io/kubectl/pkg/cmd/testing"

	"github.com/apecloud/kubeblocks/pkg/constant"

	"github.com/apecloud/kbcli/pkg/action"
	"github.com/apecloud/kbcli/pkg/testing"
	"github.com/apecloud/kbcli/pkg/types"
)

var _ = Describe("connection", func() {
	var (
		streams genericiooptions.IOStreams
		tf      *cmdtesting.TestFactory
	)

	BeforeEach(func() {
		tf = cmdtesting.NewTestFactory().WithNamespace(testing.Namespace)
		codec := scheme.Codecs.LegacyCodec(scheme.Scheme.PrioritizedVersionsAllGroups()...)
		cluster := testing.FakeCluster(testing.ClusterName, testing.Namespace)
		pods := testing.FakePods(3, testing.Namespace, testing.ClusterName)
		httpResp := func(obj runtime.Object) *http.Response {
			return &http.Response{StatusCode: http.StatusOK, Header: cmdtesting.DefaultHeader(), Body: cmdtesting.ObjBody(codec, obj)}
		}
		services := testing.FakeServices()
		secrets := testing.FakeSecrets(testing.Namespace, testing.ClusterName)
		tf.UnstructuredClient = &clientfake.RESTClient{
			GroupVersion:         schema.GroupVersion{Group: types.AppsAPIGroup, Version: types.AppsAPIVersion},
			NegotiatedSerializer: resource.UnstructuredPlusDefaultContentConfig().NegotiatedSerializer,
			Client: clientfake.CreateHTTPClient(func(req *http.Request) (*http.Response, error) {
				urlPrefix := "/api/v1/namespaces/" + testing.Namespace
				return map[string]*http.Response{
					urlPrefix + "/services":                   httpResp(services),
					urlPrefix + "/secrets":                    httpResp(secrets),
					urlPrefix + "/pods":                       httpResp(pods),
					urlPrefix + "/pods/" + pods.Items[0].Name: httpResp(findPod(pods, pods.Items[0].Name)),
				}[req.URL.Path], nil
			}),
		}

		tf.Client = tf.UnstructuredClient
		tf.FakeDynamicClient = testing.FakeDynamicClient(cluster, testing.FakeCompDef(),
			&services.Items[0], &pods.Items[0], &pods.Items[1], &pods.Items[2])
		streams = genericiooptions.NewTestIOStreamsDiscard()
	})

	AfterEach(func() {
		tf.Cleanup()
	})

	It("new connection command", func() {
		cmd := NewConnectCmd(tf, streams)
		Expect(cmd).ShouldNot(BeNil())
	})

	It("validate", func() {
		o := &ConnectOptions{ExecOptions: action.NewExecOptions(tf, streams)}

		By("specified more than one cluster")
		Expect(o.Validate([]string{"c1", "c2"})).Should(HaveOccurred())

		By("without cluster name")
		Expect(o.Validate(nil)).Should(HaveOccurred())

		Expect(o.Validate([]string{testing.ClusterName})).Should(Succeed())

		// set instance name and cluster name, should fail
		o.PodName = "test-pod-0"
		Expect(o.Validate([]string{testing.ClusterName})).Should(HaveOccurred())
		o.clusterComponentName = "test-component"
		Expect(o.Validate([]string{})).Should(HaveOccurred())

		// unset pod name
		o.PodName = ""
		Expect(o.Validate([]string{testing.ClusterName})).Should(Succeed())
		// unset component name
		o.clusterComponentName = ""
		Expect(o.Validate([]string{testing.ClusterName})).Should(Succeed())
	})

	It("complete by cluster name", func() {
		o := &ConnectOptions{ExecOptions: action.NewExecOptions(tf, streams)}
		Expect(o.Validate([]string{testing.ClusterName})).Should(Succeed())
		Expect(o.Complete()).Should(Succeed())
	})

	It("complete by pod name", func() {
		o := &ConnectOptions{ExecOptions: action.NewExecOptions(tf, streams)}
		o.PodName = constant.GenerateWorkloadNamePattern(testing.ClusterName, testing.ComponentName) + "-0"
		Expect(o.Validate([]string{})).Should(Succeed())
		Expect(o.Complete()).Should(Succeed())
		Expect(o.Pod).ShouldNot(BeNil())
	})

	It("show example", func() {
		initOption := func(setOption func(o *ConnectOptions)) *ConnectOptions {
			o := &ConnectOptions{ExecOptions: action.NewExecOptions(tf, streams)}
			if setOption != nil {
				setOption(o)
			}
			args := []string{testing.ClusterName}
			if o.PodName != "" {
				args = nil
			}
			Expect(o.Validate(args)).Should(Succeed())
			Expect(o.Complete()).Should(Succeed())
			return o
		}

		By("specify one cluster")
		o := initOption(nil)
		Expect(o.runShowExample()).Should(Succeed())
		Expect(o.services).Should(HaveLen(4))
		Expect(o.accounts).Should(HaveLen(2))

		By("specify one component")
		o = initOption(func(o *ConnectOptions) {
			o.clusterComponentName = testing.ComponentName
		})
		Expect(o.runShowExample()).Should(Succeed())
		Expect(o.services).Should(HaveLen(4))
		Expect(o.accounts).Should(HaveLen(1))

		By("specify one pod")
		o = initOption(func(o *ConnectOptions) {
			o.PodName = constant.GenerateWorkloadNamePattern(testing.ClusterName, testing.ComponentName) + "-0"
		})
		Expect(o.runShowExample()).Should(Succeed())
		// get one headless service
		Expect(o.services).Should(HaveLen(1))
		Expect(o.services[0].Name).Should(Equal(constant.GenerateDefaultComponentHeadlessServiceName(testing.ClusterName, testing.ComponentName)))
		Expect(o.accounts).Should(HaveLen(1))
	})
})

func findPod(pods *corev1.PodList, name string) *corev1.Pod {
	for i, pod := range pods.Items {
		if pod.Name == name {
			return &pods.Items[i]
		}
	}
	return nil
}
