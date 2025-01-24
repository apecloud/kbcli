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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	clientfake "k8s.io/client-go/rest/fake"
	cmdtesting "k8s.io/kubectl/pkg/cmd/testing"

	extensionsv1alpha1 "github.com/apecloud/kubeblocks/apis/extensions/v1alpha1"
	"github.com/apecloud/kubeblocks/pkg/constant"

	"github.com/apecloud/kbcli/pkg/testing"
)

var _ = Describe("test addon upgrade", func() {
	var streams genericiooptions.IOStreams
	var tf *cmdtesting.TestFactory

	BeforeEach(func() {
		streams, _, _, _ = genericiooptions.NewTestIOStreams()
		tf = cmdtesting.NewTestFactory().WithNamespace(testing.Namespace)
		tf.FakeDynamicClient = testing.FakeDynamicClient()
		tf.Client = &clientfake.RESTClient{}
	})

	AfterEach(func() {
		tf.Cleanup()
	})

	It("text upgrade cmd", func() {
		Expect(newUpgradeCmd(tf, streams)).ShouldNot(BeNil())
	})

	It("test upgrade complete", func() {
		Expect(addDefaultIndex()).Should(BeNil())
		option := newUpgradeOption(tf, streams)
		Expect(option).ShouldNot(BeNil())

		option.name = "not existed addon"
		Expect(option.Complete()).Should(HaveOccurred())
		// not found in K8s
		option.name = "apecloud-mysql"
		Expect(option.Complete()).Should(HaveOccurred())

		tf.FakeDynamicClient = testing.FakeDynamicClient(testing.FakeAddon("apecloud-mysql"))
		option = newUpgradeOption(tf, streams)
		option.name = "apecloud-mysql"
		option.version = "0.7.0"
		Expect(option.Complete()).Should(Succeed())
	})

	It("test upgrade validate", func() {
		option := newUpgradeOption(tf, streams)
		option.addon = &extensionsv1alpha1.Addon{
			ObjectMeta: metav1.ObjectMeta{
				Labels: map[string]string{},
			},
		}
		option.Client = testing.FakeClientSet(testing.FakeKBDeploy("0.7.0"))
		// no target version
		Expect(option.Validate()).Should(HaveOccurred())
		option.addon.Labels[constant.AppVersionLabelKey] = "0.7.0"
		// no current version
		Expect(option.Validate()).Should(HaveOccurred())
		option.currentVersion = "99.0.0"
	})
})
