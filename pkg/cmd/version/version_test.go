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

package version

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	gv "github.com/hashicorp/go-version"
	"k8s.io/client-go/rest/fake"
	cmdtesting "k8s.io/kubectl/pkg/cmd/testing"
)

var _ = Describe("version", func() {
	It("version", func() {
		tf := cmdtesting.NewTestFactory()
		tf.Client = &fake.RESTClient{}
		By("testing version command")
		cmd := NewVersionCmd(tf)
		Expect(cmd).ShouldNot(BeNil())

		By("testing run")
		o := &versionOptions{}
		o.Run(tf)
	})

	It("version comparison", func() {
		kbVersion, _ := gv.NewVersion("0.6.0-alpha.23")
		cliVersion, _ := gv.NewVersion("v0.6.0-alpha.23")
		Expect(kbVersion.Equal(cliVersion)).Should(BeTrue())

		kbVersion, _ = gv.NewVersion("0.6.0-alpha.23")
		cliVersion, _ = gv.NewVersion("v0.6.23")
		Expect(kbVersion.Equal(cliVersion)).Should(BeFalse())

		kbVersion, _ = gv.NewVersion("0.6.0-alpha.23")
		cliVersion, _ = gv.NewVersion("0.6.0-beta.23")
		Expect(kbVersion.Equal(cliVersion)).Should(BeFalse())

		kbVersion, _ = gv.NewVersion("0.6.3")
		cliVersion, _ = gv.NewVersion("v0.6.3")
		Expect(kbVersion.Equal(cliVersion)).Should(BeTrue())
	})

	It("version match", func() {
		kbVersion, _ := gv.NewVersion("0.6.0-alpha.23")
		cliVersion, _ := gv.NewVersion("0.6.1-alpha.23")
		Expect(checkVersionMatch(cliVersion, kbVersion)).Should(BeFalse())

		kbVersion, _ = gv.NewVersion("0.6.0-alpha.23")
		cliVersion, _ = gv.NewVersion("v0.6.0-beta.17")
		Expect(checkVersionMatch(cliVersion, kbVersion)).Should(BeTrue())

		kbVersion, _ = gv.NewVersion("0.7.0-alpha.23")
		cliVersion, _ = gv.NewVersion("0.6-beta.23")
		Expect(checkVersionMatch(cliVersion, kbVersion)).Should(BeFalse())
	})
})
