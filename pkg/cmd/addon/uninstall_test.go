/*
Copyright (C) 2022-2026 ApeCloud Co., Ltd

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
	"k8s.io/cli-runtime/pkg/genericiooptions"
	clientfake "k8s.io/client-go/rest/fake"
	cmdtesting "k8s.io/kubectl/pkg/cmd/testing"

	"github.com/apecloud/kbcli/pkg/testing"
)

var _ = Describe("uninstall test", func() {
	var (
		streams genericiooptions.IOStreams
		tf      *cmdtesting.TestFactory
	)

	BeforeEach(func() {
		streams, _, _, _ = genericiooptions.NewTestIOStreams()
		tf = cmdtesting.NewTestFactory().WithNamespace(testNamespace)
		tf.FakeDynamicClient = testing.FakeDynamicClient()
		tf.Client = &clientfake.RESTClient{}
	})

	AfterEach(func() {
		tf.Cleanup()
	})

	It("text uninstall cmd", func() {
		Expect(newUninstallCmd(tf, streams)).ShouldNot(BeNil())
	})

	It("test baseOption complete", func() {
		option := newUninstallOption(tf, streams)
		Expect(option).ShouldNot(BeNil())
		Expect(option.baseOption.complete()).Should(Succeed())
	})
})
