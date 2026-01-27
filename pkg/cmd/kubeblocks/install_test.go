/*
Copyright (C) 2022-2026 ApeCloud Co., Ltd

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
	"fmt"
	"io"

	"github.com/apecloud/kubeblocks/pkg/constant"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/spf13/cobra"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	clientfake "k8s.io/client-go/rest/fake"
	cmdtesting "k8s.io/kubectl/pkg/cmd/testing"

	"github.com/apecloud/kbcli/pkg/testing"
	"github.com/apecloud/kbcli/pkg/types"
	"github.com/apecloud/kbcli/pkg/util"
	"github.com/apecloud/kbcli/pkg/util/helm"
	"github.com/apecloud/kbcli/version"
)

const namespace = "test"

var _ = Describe("kubeblocks install", func() {
	var (
		cmd     *cobra.Command
		streams genericiooptions.IOStreams
		tf      *cmdtesting.TestFactory
		in      *bytes.Buffer
	)

	BeforeEach(func() {
		streams, in, _, _ = genericiooptions.NewTestIOStreams()
		tf = cmdtesting.NewTestFactory().WithNamespace(namespace)
		tf.Client = &clientfake.RESTClient{}
	})

	AfterEach(func() {
		tf.Cleanup()
	})

	It("check install", func() {
		var cfg string
		cmd = newInstallCmd(tf, streams)
		Expect(cmd).ShouldNot(BeNil())
		Expect(cmd.HasSubCommands()).Should(BeFalse())

		o := &InstallOptions{
			Options: Options{
				IOStreams: streams,
			},
		}

		By("command without kubeconfig flag")
		Expect(o.Complete(tf, cmd)).Should(HaveOccurred())

		cmd.Flags().StringVar(&cfg, "kubeconfig", "", "Path to the kubeconfig file to use for CLI requests.")
		cmd.Flags().StringVar(&cfg, "context", "", "The name of the kubeconfig context to use.")
		Expect(o.Complete(tf, cmd)).To(Succeed())
		Expect(o.HelmCfg).ShouldNot(BeNil())
	})

	It("run install", func() {
		o := &InstallOptions{
			Options: Options{
				IOStreams: streams,
				HelmCfg:   helm.NewFakeConfig(namespace),
				Client:    testing.FakeClientSet(),
				Dynamic:   testing.FakeDynamicClient(),
			},
			Version:         version.DefaultKubeBlocksVersion,
			CreateNamespace: true,
		}
		Expect(o.Install()).Should(HaveOccurred())
		Expect(o.ValueOpts.Values).Should(HaveLen(0))
		Expect(o.installChart()).Should(HaveOccurred())
		o.printNotes()
	})

	It("checkVersion", func() {
		o := &InstallOptions{
			Options: Options{
				IOStreams: genericiooptions.NewTestIOStreamsDiscard(),
				Client:    testing.FakeClientSet(),
			},
			Check: true,
		}
		By("kubernetes version is empty")
		v := util.Version{}
		Expect(o.checkVersion(v).Error()).Should(ContainSubstring("failed to get kubernetes version"))

		By("kubernetes is provided by cloud provider")
		v.Kubernetes = "v1.25.0-eks"
		Expect(o.checkVersion(v)).Should(Succeed())

		By("kubernetes is not provided by cloud provider")
		v.Kubernetes = "v1.25.0"
		Expect(o.checkVersion(v)).Should(Succeed())
	})

	It("CompleteInstallOptions test", func() {
		o := &InstallOptions{
			Options: Options{
				IOStreams: streams,
				HelmCfg:   helm.NewFakeConfig(namespace),
				Client:    testing.FakeClientSet(),
				Dynamic:   testing.FakeDynamicClient(),
			},
			Version:         version.DefaultKubeBlocksVersion,
			CreateNamespace: true,
		}
		Expect(o.TolerationsRaw).Should(BeNil())
		Expect(o.ValueOpts.JSONValues).Should(BeNil())
		Expect(o.CompleteInstallOptions()).ShouldNot(HaveOccurred())
		Expect(o.TolerationsRaw).Should(Equal([]string{defaultTolerationsForInstallation}))
		Expect(o.ValueOpts.JSONValues).ShouldNot(BeNil())
	})

	It("do preCheck KB version", func() {
		o := &InstallOptions{
			Options: Options{
				IOStreams: streams,
				HelmCfg:   helm.NewFakeConfig(types.DefaultNamespace),
				Client:    testing.FakeClientSet(),
				Dynamic:   testing.FakeDynamicClient(),
			},
		}

		By("Testing fresh installation")
		err := o.PreCheckKBVersion()
		Expect(err).Should(BeNil())
		Expect(o.upgradeFrom09).Should(BeFalse())

		By("Testing when KubeBlocks already exists")
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
		_, err = o.Client.AppsV1().Deployments(types.DefaultNamespace).Create(context.TODO(), deploy, metav1.CreateOptions{})
		Expect(err).Should(BeNil())
		o.Version = "0.9.3"
		err = o.PreCheckKBVersion()
		Expect(err).Should(MatchError(fmt.Sprintf("KubeBlocks %s already exists, repeated installation is not supported", kb09Version)))

		By("Testing installation in same namespace with 0.9")
		o.Version = "1.0.0"
		in.Write([]byte("yes\n"))
		o.In = io.NopCloser(in)
		err = o.PreCheckKBVersion()
		Expect(err.Error()).Should(Equal(fmt.Sprintf(`cannot install KubeBlocks in the same namespace "%s" with KubeBlocks 0.9`, types.DefaultNamespace)))

		By("Testing upgrade from 0.9 with diff namespace")
		o.HelmCfg.SetNamespace(namespace)
		in.Write([]byte("yes\n"))
		o.In = io.NopCloser(in)
		err = o.PreCheckKBVersion()
		Expect(err).Should(BeNil())
		Expect(o.upgradeFrom09).Should(BeTrue())
	})
})
