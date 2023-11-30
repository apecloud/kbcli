/*
Copyright (C) 2022-2023 ApeCloud Co., Ltd

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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	clientfake "k8s.io/client-go/rest/fake"
	cmdtesting "k8s.io/kubectl/pkg/cmd/testing"

	extensionsv1alpha1 "github.com/apecloud/kubeblocks/apis/extensions/v1alpha1"

	"github.com/apecloud/kbcli/pkg/testing"
	"github.com/apecloud/kbcli/pkg/types"
)

var _ = Describe("index test", func() {
	var streams genericiooptions.IOStreams
	var tf *cmdtesting.TestFactory
	var addonName = "apecloud-mysql"
	BeforeEach(func() {
		streams, _, _, _ = genericiooptions.NewTestIOStreams()
		tf = cmdtesting.NewTestFactory().WithNamespace(testNamespace)
		tf.FakeDynamicClient = testing.FakeDynamicClient()
		tf.Client = &clientfake.RESTClient{}
	})

	AfterEach(func() {
		tf.Cleanup()
	})

	It("text install cmd", func() {
		Expect(newInstallCmd(tf, streams)).ShouldNot(BeNil())
	})

	It("test baseOption complete", func() {
		option := newInstallOption(tf, streams)
		Expect(option).ShouldNot(BeNil())
		Expect(option.baseOption.complete()).Should(Succeed())
	})

	It("test install complete", func() {
		option := newInstallOption(tf, streams)
		option.name = "not-existed"
		Expect(option.Complete()).Should(HaveOccurred())

		Expect(addDefaultIndex()).Should(BeNil())
		option.name = addonName
		option.version = "0.7.0"
		Expect(option.Complete()).Should(Succeed())

		option.addon = nil
		option.version = "0.0.0"
		Expect(option.Complete()).Should(HaveOccurred())

		option.addon = nil
		option.version = "error-version"
		Expect(option.Complete()).Should(HaveOccurred())

		option.name = addonName
		option.version = ""
		option.source = "not-existed"
		Expect(option.Complete()).Should(HaveOccurred())
	})

	It("test install validate", func() {
		option := newInstallOption(tf, streams)
		Expect(option.Validate()).Should(HaveOccurred())
		option.addon = &extensionsv1alpha1.Addon{}
		option.Client = testing.FakeClientSet(testing.FakeKBDeploy("0.7.0"))
		Expect(option.Validate()).Should(Succeed())

		By("validate version", func() {
			var (
				ok  bool
				err error
			)
			ok, err = validateVersion("0.7.0", "0.7.1")
			Expect(err).Should(BeNil())
			Expect(ok).Should(BeFalse())

			ok, err = validateVersion("0.7.0", "0.7.0")
			Expect(err).Should(BeNil())
			Expect(ok).Should(BeTrue())

			ok, err = validateVersion(">=0.7.0", "0.7.1")
			Expect(err).Should(BeNil())
			Expect(ok).Should(BeTrue())

			ok, err = validateVersion(">=0.7.0", "0.6.0")
			Expect(err).Should(BeNil())
			Expect(ok).Should(BeFalse())

			ok, err = validateVersion(">=0.7.0,<=0.8.0", "0.9.0")
			Expect(err).Should(BeNil())
			Expect(ok).Should(BeFalse())

			ok, err = validateVersion(">=0.7.0 || <=0.5.0", "0.3.0")
			Expect(err).Should(BeNil())
			Expect(ok).Should(BeTrue())

			ok, err = validateVersion("", "0.3.0")
			Expect(err).Should(HaveOccurred())
			Expect(ok).Should(BeFalse())

			ok, err = validateVersion(">=0.7.0", "")
			Expect(err).Should(HaveOccurred())
			Expect(ok).Should(BeFalse())
		})

		By("validate --force")
		option.addon = &extensionsv1alpha1.Addon{
			ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{
				types.KBVersionValidateAnnotationKey: "0.8.0",
			}},
		}
		option.Client = testing.FakeClientSet(testing.FakeKBDeploy("0.7.0"))
		option.force = true
		Expect(option.Validate()).Should(Succeed())
	})
})
