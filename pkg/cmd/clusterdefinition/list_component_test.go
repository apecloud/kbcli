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

package clusterdefinition

import (
	"net/http"

	kbappsv1 "github.com/apecloud/kubeblocks/apis/apps/v1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	"k8s.io/cli-runtime/pkg/resource"
	clientfake "k8s.io/client-go/rest/fake"
	cmdtesting "k8s.io/kubectl/pkg/cmd/testing"
	"k8s.io/kubectl/pkg/scheme"

	"github.com/apecloud/kbcli/pkg/action"
	"github.com/apecloud/kbcli/pkg/testing"
	"github.com/apecloud/kbcli/pkg/types"
)

var _ = Describe("clusterdefinition list components", func() {
	var (
		// cmd     *cobra.Command
		streams genericiooptions.IOStreams
		// out     *bytes.Buffer
		tf *cmdtesting.TestFactory
	)

	const (
		namespace             = testing.Namespace
		clusterdefinitionName = testing.ClusterDefName
	)

	mockClient := func(data runtime.Object) *cmdtesting.TestFactory {
		tf := testing.NewTestFactory(namespace)
		codec := scheme.Codecs.LegacyCodec(scheme.Scheme.PrioritizedVersionsAllGroups()...)
		tf.UnstructuredClient = &clientfake.RESTClient{
			NegotiatedSerializer: resource.UnstructuredPlusDefaultContentConfig().NegotiatedSerializer,
			GroupVersion:         schema.GroupVersion{Group: types.AppsAPIGroup, Version: types.AppsAPIVersion},
			Resp:                 &http.Response{StatusCode: http.StatusOK, Header: cmdtesting.DefaultHeader(), Body: cmdtesting.ObjBody(codec, data)},
		}
		tf.Client = tf.UnstructuredClient
		tf.FakeDynamicClient = testing.FakeDynamicClient(data)
		return tf
	}

	BeforeEach(func() {
		_ = kbappsv1.AddToScheme(scheme.Scheme)
		clusterDef := testing.FakeClusterDef()
		tf = mockClient(clusterDef)
		// streams, _, out, _ = genericiooptions.NewTestIOStreams()
		// cmd = NewListComponentsCmd(tf, streams)
	})

	AfterEach(func() {
		tf.Cleanup()
	})

	It("create list-components cmd", func() {
		cmd := NewListComponentsCmd(tf, streams)
		Expect(cmd).ShouldNot(BeNil())
	})

	It("list-components requires a clusterdefinition Name", func() {
		Expect(validate([]string{})).Should(HaveOccurred())
	})

	It("cd list-components when the cd do not exist", func() {
		o := action.NewListOptions(tf, streams, types.ClusterDefGVR())
		o.AllNamespaces = true
		codec := scheme.Codecs.LegacyCodec(scheme.Scheme.PrioritizedVersionsAllGroups()...)
		tf.UnstructuredClient = &clientfake.RESTClient{
			NegotiatedSerializer: resource.UnstructuredPlusDefaultContentConfig().NegotiatedSerializer,
			GroupVersion:         schema.GroupVersion{Group: types.AppsAPIGroup, Version: types.AppsAPIVersion},
			Resp:                 &http.Response{StatusCode: http.StatusOK, Header: cmdtesting.DefaultHeader(), Body: cmdtesting.ObjBody(codec, testing.FakeResourceNotFound(types.ClusterDefGVR(), clusterdefinitionName+"-no-exist"))},
		}
		Expect(listComponents(o)).Should(HaveOccurred())

	})

	// TODO: update with new API
	/*It("list-components", func() {
			cmd.Run(cmd, []string{clusterdefinitionName})
			expected := `NAME                    WORKLOAD-TYPE   CHARACTER-TYPE   CLUSTER-DEFINITION        IS-MAIN
	fake-component-type                     mysql            fake-cluster-definition   true
	fake-component-type-1                   mysql            fake-cluster-definition   false
	`
			Expect(expected).Should(Equal(out.String()))
			fmt.Println(out.String())
		})*/
})
