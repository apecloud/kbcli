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

package kubeblocks

import (
	"bytes"
	"context"
	"io"

	"github.com/apecloud/kubeblocks/pkg/constant"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/spf13/cobra"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	clientfake "k8s.io/client-go/rest/fake"
	cmdtesting "k8s.io/kubectl/pkg/cmd/testing"

	"github.com/apecloud/kbcli/pkg/testing"
	"github.com/apecloud/kbcli/pkg/types"
	"github.com/apecloud/kbcli/pkg/util/helm"
)

var _ = Describe("kubeblocks uninstall", func() {
	var cmd *cobra.Command
	var streams genericiooptions.IOStreams
	var tf *cmdtesting.TestFactory
	var in *bytes.Buffer

	BeforeEach(func() {
		streams, in, _, _ = genericiooptions.NewTestIOStreams()
		tf = cmdtesting.NewTestFactory().WithNamespace(namespace)
		tf.Client = &clientfake.RESTClient{}
	})

	AfterEach(func() {
		tf.Cleanup()
	})

	It("check uninstall", func() {
		var cfg string
		cmd = newUninstallCmd(tf, streams)
		Expect(cmd).ShouldNot(BeNil())

		cmd.Flags().StringVar(&cfg, "kubeconfig", "", "Path to the kubeconfig file to use for CLI requests.")
		cmd.Flags().StringVar(&cfg, "context", "", "The name of the kubeconfig context to use.")
		Expect(cmd.HasSubCommands()).Should(BeFalse())

		o := &Options{
			IOStreams: streams,
		}
		Expect(o.Complete(tf, cmd)).Should(Succeed())
		Expect(o.HelmCfg).ShouldNot(BeNil())
	})

	It("run uninstall", func() {
		o := UninstallOptions{
			Options: Options{
				IOStreams: streams,
				HelmCfg:   helm.NewFakeConfig(namespace),
				Namespace: "default",
				Client:    testing.FakeClientSet(),
				Dynamic:   testing.FakeDynamicClient(testing.FakeVolumeSnapshotClass()),
			},
			AutoApprove: true,
			force:       false,
		}
		Expect(o.Uninstall()).Should(Succeed())
	})

	It("checkResources", func() {
		fakeDynamic := testing.FakeDynamicClient()
		Expect(checkResources(fakeDynamic)).Should(Succeed())
	})

	It("do preCheck", func() {
		deploy := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      types.KubeBlocksChartName,
				Namespace: types.DefaultNamespace,
				Labels: map[string]string{
					constant.AppNameLabelKey:      types.KubeBlocksChartName,
					constant.AppComponentLabelKey: "apps",
					constant.AppVersionLabelKey:   kb09Version,
				},
			},
		}
		helmSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "kubeblocks-helm-secret",
				Namespace: types.DefaultNamespace,
				Labels: map[string]string{
					"name":  types.KubeBlocksChartName,
					"owner": "helm",
				},
			},
		}
		o := &UninstallOptions{
			Options: Options{
				Client:    testing.FakeClientSet(deploy, helmSecret),
				Dynamic:   testing.FakeDynamicClient(),
				IOStreams: streams,
			},
			AutoApprove: false,
		}

		By("auto approve is false")
		in.Write([]byte("uninstall-kubeblocks\n"))
		o.In = io.NopCloser(in)
		err := o.PreCheck()
		Expect(err).Should(BeNil())
		Expect(o.Namespace).Should(Equal(types.DefaultNamespace))

		By("auto approve is true")
		o.AutoApprove = true
		err = o.PreCheck()
		Expect(err).Should(BeNil())
		Expect(o.Namespace).Should(Equal(types.DefaultNamespace))

		By("multiple kubeblocks deployments")
		deploy2 := deploy.DeepCopy()
		deploy2.Name = "kubeblocks-2"
		deploy2.Namespace = "kb-system-2"
		deploy2.Labels[constant.AppVersionLabelKey] = "1.0.0"
		_, err = o.Client.AppsV1().Deployments(deploy2.Namespace).Create(context.TODO(), deploy2, metav1.CreateOptions{})
		Expect(err).Should(BeNil())

		o.Namespace = "kb-system"
		err = o.PreCheck()
		Expect(err).Should(BeNil())
		Expect(o.existMultiKB).Should(BeTrue())

		By("uninstall 1.0 first when existing multiple kubeblocks")
		o.Namespace = "kb-system-2"
		err = o.PreCheck()
		Expect(err.Error()).Should(Equal("only can uninstall KubeBlocks 0.9 when existing multi KubeBlocks"))
	})
})
