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
	kbappsv1 "github.com/apecloud/kubeblocks/apis/apps/v1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/client-go/kubernetes/scheme"

	"fmt"
	"net/http"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	"k8s.io/cli-runtime/pkg/resource"
	clientfake "k8s.io/client-go/rest/fake"
	cmdtesting "k8s.io/kubectl/pkg/cmd/testing"

	"github.com/apecloud/kbcli/pkg/action"
	"github.com/apecloud/kbcli/pkg/testing"
	"github.com/apecloud/kbcli/pkg/types"
)

var _ = Describe("Cluster", func() {

	const (
		clusterName = "test"
		namespace   = "default"
	)
	var streams genericiooptions.IOStreams
	var tf *cmdtesting.TestFactory
	fakeConfigData := map[string]string{
		"config.yaml": `# the default storage class name.
    DEFAULT_STORAGE_CLASS: ""`,
	}
	BeforeEach(func() {
		streams, _, _, _ = genericiooptions.NewTestIOStreams()
		tf = cmdtesting.NewTestFactory().WithNamespace(namespace)
		cd := testing.FakeClusterDef()
		fakeDefaultStorageClass := testing.FakeStorageClass(testing.StorageClassName, testing.IsDefault)
		// TODO: remove unused codes?
		tf.FakeDynamicClient = testing.FakeDynamicClient(cd, fakeDefaultStorageClass, testing.FakeConfigMap("kubeblocks-manager-config", types.DefaultNamespace, fakeConfigData), testing.FakeSecret(types.DefaultNamespace, clusterName))
		tf.Client = &clientfake.RESTClient{}
	})

	AfterEach(func() {
		tf.Cleanup()
	})

	// TODO: add create cluster case
	Context("delete cluster", func() {

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
