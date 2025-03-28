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
	"bytes"
	"net/http"
	"strings"

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

	dpv1alpha1 "github.com/apecloud/kubeblocks/apis/dataprotection/v1alpha1"

	"github.com/apecloud/kbcli/pkg/testing"
	"github.com/apecloud/kbcli/pkg/types"
)

var _ = Describe("Expose", func() {
	const (
		namespace   = "test"
		clusterName = "test"
	)

	var (
		streams genericiooptions.IOStreams
		tf      *cmdtesting.TestFactory
		cluster = testing.FakeCluster(clusterName, namespace)
		pods    = testing.FakePods(3, namespace, clusterName)
	)
	BeforeEach(func() {
		streams, _, _, _ = genericiooptions.NewTestIOStreams()
		tf = testing.NewTestFactory(namespace)
		codec := scheme.Codecs.LegacyCodec(scheme.Scheme.PrioritizedVersionsAllGroups()...)
		httpResp := func(obj runtime.Object) *http.Response {
			return &http.Response{StatusCode: http.StatusOK, Header: cmdtesting.DefaultHeader(), Body: cmdtesting.ObjBody(codec, obj)}
		}

		tf.UnstructuredClient = &clientfake.RESTClient{
			GroupVersion:         schema.GroupVersion{Group: types.AppsAPIGroup, Version: types.AppsV1APIVersion},
			NegotiatedSerializer: resource.UnstructuredPlusDefaultContentConfig().NegotiatedSerializer,
			Client: clientfake.CreateHTTPClient(func(req *http.Request) (*http.Response, error) {
				urlPrefix := "/api/v1/namespaces/" + namespace
				mapping := map[string]*http.Response{
					"/api/v1/nodes/" + testing.NodeName:   httpResp(testing.FakeNode()),
					urlPrefix + "/services":               httpResp(&corev1.ServiceList{}),
					urlPrefix + "/events":                 httpResp(&corev1.EventList{}),
					urlPrefix + "/persistentvolumeclaims": httpResp(&corev1.PersistentVolumeClaimList{}),
					urlPrefix + "/pods":                   httpResp(pods),
				}
				return mapping[req.URL.Path], nil
			}),
		}

		tf.Client = tf.UnstructuredClient
		tf.FakeDynamicClient = testing.FakeDynamicClient(cluster, testing.FakeClusterDef())
	})

	AfterEach(func() {
		tf.Cleanup()
	})

	It("describe", func() {
		cmd := NewDescribeCmd(tf, streams)
		Expect(cmd).ShouldNot(BeNil())
	})

	It("complete", func() {
		o := newOptions(tf, streams)
		Expect(o.complete(nil)).Should(HaveOccurred())
		Expect(o.complete([]string{clusterName})).Should(Succeed())
		Expect(o.names).Should(Equal([]string{clusterName}))
		Expect(o.client).ShouldNot(BeNil())
		Expect(o.dynamic).ShouldNot(BeNil())
		Expect(o.namespace).Should(Equal(namespace))
	})

	It("run", func() {
		o := newOptions(tf, streams)
		Expect(o.complete([]string{clusterName})).Should(Succeed())
		Expect(o.run()).Should(Succeed())
	})

	It("showEvents", func() {
		out := &bytes.Buffer{}
		showEvents("test-cluster", namespace, out)
		Expect(out.String()).ShouldNot(BeEmpty())
	})

	It("showDataProtections", func() {
		out := &bytes.Buffer{}
		fakeBackupPolicies := []dpv1alpha1.BackupPolicy{
			*testing.FakeBackupPolicy("backup-policy-test", "test-cluster"),
		}
		fakeBackupSchedules := []dpv1alpha1.BackupSchedule{
			*testing.FakeBackupSchedule("backup-schedule-test", "backup-policy-test"),
		}

		showDataProtection(fakeBackupPolicies, fakeBackupSchedules, "test-repository", "", "", out)
		strs := strings.Split(out.String(), "\n")
		Expect(strs).ShouldNot(BeEmpty())
	})
})
