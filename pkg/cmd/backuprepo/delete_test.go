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

package backuprepo

import (
	"bytes"
	"net/http"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	"k8s.io/cli-runtime/pkg/resource"
	clientfake "k8s.io/client-go/rest/fake"
	cmdtesting "k8s.io/kubectl/pkg/cmd/testing"

	dpv1alpha1 "github.com/apecloud/kubeblocks/apis/dataprotection/v1alpha1"

	"github.com/apecloud/kbcli/pkg/action"
	"github.com/apecloud/kbcli/pkg/scheme"
	"github.com/apecloud/kbcli/pkg/testing"
	"github.com/apecloud/kbcli/pkg/types"
)

var _ = Describe("backuprepo delete command", func() {
	var streams genericiooptions.IOStreams
	var in *bytes.Buffer
	var tf *cmdtesting.TestFactory

	BeforeEach(func() {
		streams, in, _, _ = genericiooptions.NewTestIOStreams()
		tf = testing.NewTestFactory(testing.Namespace)
	})

	initClient := func(repoObj runtime.Object, otherObjs ...runtime.Object) {
		_ = dpv1alpha1.AddToScheme(scheme.Scheme)
		codec := scheme.Codecs.LegacyCodec(scheme.Scheme.PrioritizedVersionsAllGroups()...)
		httpResp := func(obj runtime.Object) *http.Response {
			return &http.Response{StatusCode: http.StatusOK, Header: cmdtesting.DefaultHeader(), Body: cmdtesting.ObjBody(codec, obj)}
		}

		tf.UnstructuredClient = &clientfake.RESTClient{
			GroupVersion:         schema.GroupVersion{Group: types.DPAPIGroup, Version: types.DPAPIVersion},
			NegotiatedSerializer: resource.UnstructuredPlusDefaultContentConfig().NegotiatedSerializer,
			Client: clientfake.CreateHTTPClient(func(req *http.Request) (*http.Response, error) {
				return httpResp(repoObj), nil
			}),
		}

		tf.FakeDynamicClient = testing.FakeDynamicClient(append([]runtime.Object{repoObj}, otherObjs...)...)
		tf.Client = tf.UnstructuredClient
	}

	AfterEach(func() {
		tf.Cleanup()
	})

	It("tests completeForDeleteBackupRepo function", func() {
		o := action.NewDeleteOptions(tf, streams, types.OpsGVR())

		By("case: it should fail if no name is specified")
		err := completeForDeleteBackupRepo(o, []string{})
		Expect(err).Should(HaveOccurred())

		By("case: it should fail if multiple names are specified")
		err = completeForDeleteBackupRepo(o, []string{"name1", "name2"})
		Expect(err).Should(HaveOccurred())

		By("case: it should ok")
		err = completeForDeleteBackupRepo(o, []string{"name1"})
		Expect(err).ShouldNot(HaveOccurred())
	})

	It("should refuse to delete if the backup repo is not clear", func() {
		const testBackupRepo = "test-backuprepo"
		repoObj := testing.FakeBackupRepo(testBackupRepo, false)
		backupObj := testing.FakeBackup("test-backup")
		backupObj.Labels = map[string]string{
			associatedBackupRepoKey: testBackupRepo,
		}
		initClient(repoObj, backupObj)
		// confirm
		in.Write([]byte(testBackupRepo + "\n"))

		o := action.NewDeleteOptions(tf, streams, types.BackupRepoGVR())
		o.PreDeleteHook = preDeleteBackupRepo
		err := completeForDeleteBackupRepo(o, []string{testBackupRepo})
		Expect(err).ShouldNot(HaveOccurred())

		err = o.Run()
		Expect(err).Should(HaveOccurred())
		Expect(err.Error()).Should(ContainSubstring("this backup repository cannot be deleted because it is still containing"))
	})

	It("should the backup repo", func() {
		const testBackupRepo = "test-backuprepo"
		repoObj := testing.FakeBackupRepo(testBackupRepo, false)
		repoObj.Status.Phase = dpv1alpha1.BackupRepoFailed
		initClient(repoObj)
		// confirm
		in.Write([]byte(testBackupRepo + "\n"))

		o := action.NewDeleteOptions(tf, streams, types.BackupRepoGVR())
		o.PreDeleteHook = preDeleteBackupRepo
		err := completeForDeleteBackupRepo(o, []string{testBackupRepo})
		Expect(err).ShouldNot(HaveOccurred())

		err = o.Run()
		Expect(err).ShouldNot(HaveOccurred())
	})
})
