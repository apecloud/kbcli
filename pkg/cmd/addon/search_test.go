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

package addon

import (
	"bytes"
	"path/filepath"

	kbappsv1 "github.com/apecloud/kubeblocks/apis/apps/v1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	"k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes/scheme"
	cmdtesting "k8s.io/kubectl/pkg/cmd/testing"

	"github.com/apecloud/kbcli/pkg/testing"
)

var _ = Describe("search test", func() {
	var streams genericiooptions.IOStreams
	var tf *cmdtesting.TestFactory
	var out *bytes.Buffer
	const (
		testAddonName       = "apecloud-mysql"
		testAddonNotExisted = "fake-addon"
		testIndexDir        = "./testdata"
		testLocalPath       = "./testdata/kubeblocks"
	)
	BeforeEach(func() {
		streams, _, out, _ = genericiooptions.NewTestIOStreams()
		tf = cmdtesting.NewTestFactory().WithNamespace("default")
		_ = kbappsv1.AddToScheme(scheme.Scheme)
		addonObj := testing.FakeAddon(testAddonName)
		tf.FakeDynamicClient = fake.NewSimpleDynamicClient(
			scheme.Scheme, addonObj)
	})

	It("test search cmd Run", func() {
		cmd := newSearchCmd(tf, streams)
		Expect(cmd.Flags().Set("path", testLocalPath)).Should(Succeed())
		cmd.Run(cmd, []string{})
		Expect(out.String()).Should(ContainSubstring(testAddonName))
		Expect(out.String()).Should(ContainSubstring("uninstalled"))
	})

	It("test search cmd Run with addon specified", func() {
		cmd := newSearchCmd(tf, streams)
		cmd.Run(cmd, []string{testAddonNotExisted})
		Expect(out.String()).Should(ContainSubstring("Please update your index"))
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
		result, err := searchAddon(testAddonName, testIndexDir, "")
		Expect(err).Should(Succeed())
		Expect(result).Should(HaveLen(3))
		for i := range result {
			Expect(result[i].index.name).Should(Equal(expect[i].index))
			Expect(result[i].addon.Name).Should(Equal(expect[i].addonName))
			Expect(result[i].addon.Kind).Should(Equal(expect[i].kind))
			Expect(getAddonVersion(result[i].addon)).Should(Equal(expect[i].version))
		}
	})

	It("test addon search specify local path", func() {
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
		result, err := searchAddon(testAddonName, filepath.Dir(testLocalPath), filepath.Base(testLocalPath))
		Expect(err).Should(Succeed())
		Expect(result).Should(HaveLen(3))
		for i := range result {
			Expect(result[i].index.name).Should(Equal(expect[i].index))
			Expect(result[i].addon.Name).Should(Equal(expect[i].addonName))
			Expect(result[i].addon.Kind).Should(Equal(expect[i].kind))
			Expect(getAddonVersion(result[i].addon)).Should(Equal(expect[i].version))
		}
	})

})
