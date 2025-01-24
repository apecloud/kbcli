/*
Copyright (C) 2022-2025 ApeCloud Co., Ltd

# This file is part of KubeBlocks project

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
	"net/http"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	"k8s.io/cli-runtime/pkg/resource"
	"k8s.io/client-go/kubernetes/scheme"
	clientfake "k8s.io/client-go/rest/fake"
	cmdtesting "k8s.io/kubectl/pkg/cmd/testing"

	kbappsv1 "github.com/apecloud/kubeblocks/apis/apps/v1"

	"github.com/apecloud/kbcli/pkg/testing"
	"github.com/apecloud/kbcli/pkg/types"
)

var _ = Describe("addon util test", func() {
	var (
		tf      *cmdtesting.TestFactory
		streams genericiooptions.IOStreams
	)

	const (
		fakeAddonName = testing.ClusterDefName
		namespace     = testing.Namespace
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
		streams, _, _, _ = genericiooptions.NewTestIOStreams()
		_ = kbappsv1.AddToScheme(scheme.Scheme)
		cluster := testing.FakeCluster(testing.ClusterName, testing.Namespace)
		tf = mockClient(cluster)
	})
	It("text CheckAddonUsedByCluster", func() {
		Expect(CheckAddonUsedByCluster(tf.FakeDynamicClient, []string{fakeAddonName}, streams.In)).Should(HaveOccurred())
	})
})
