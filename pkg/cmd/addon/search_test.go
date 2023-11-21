/*
Copyright (C) 2022-2023 ApeCloud Co., Ltd

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

package addon

import (
	"bytes"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/apecloud/kubeblocks/pkg/constant"

	"k8s.io/cli-runtime/pkg/genericiooptions"
)

var _ = Describe("search test", func() {
	var streams genericiooptions.IOStreams
	var out *bytes.Buffer
	const (
		testAddonName       = "apecloud-mysql"
		testAddonNotExisted = "fake-addon"
		testIndexDir        = "./testdata"
	)
	BeforeEach(func() {
		streams, _, out, _ = genericiooptions.NewTestIOStreams()
	})
	It("test search cmd", func() {
		Expect(newSearchCmd(streams)).ShouldNot(BeNil())
	})

	It("test search", func() {
		cmd := newSearchCmd(streams)
		cmd.Run(cmd, []string{testAddonNotExisted})
		Expect(out.String()).Should(Equal("fake-addon addon not found. Please update your index or check the addon name"))
	})

	It("test addon search", func() {
		expect := []struct {
			index     string
			kind      string
			addonName string
			version   string
		}{
			{
				"kubeblocks", "Addon", "apecloud-mysql", "0.7.0",
			},
			{
				"kubeblocks", "Addon", "apecloud-mysql", "0.8.0-alpha.5",
			},
			{
				"kubeblocks", "Addon", "apecloud-mysql", "0.8.0-alpha.6",
			},
		}
		result, err := searchAddon(testAddonName, testIndexDir)
		Expect(err).Should(Succeed())
		Expect(result).Should(HaveLen(3))
		for i := range result {
			Expect(result[i].index.name).Should(Equal(expect[i].index))
			Expect(result[i].addon.Name).Should(Equal(expect[i].addonName))
			Expect(result[i].addon.Kind).Should(Equal(expect[i].kind))
			Expect(result[i].addon.Labels[constant.AppVersionLabelKey]).Should(Equal(expect[i].version))
		}
	})

})
