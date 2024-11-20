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

package kubeblocks

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/spf13/cobra"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/release"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	clientfake "k8s.io/client-go/rest/fake"
	cmdtesting "k8s.io/kubectl/pkg/cmd/testing"

	"github.com/apecloud/kbcli/pkg/testing"
	"github.com/apecloud/kbcli/pkg/types"
	"github.com/apecloud/kbcli/pkg/util/helm"
	"github.com/apecloud/kbcli/version"
)

var _ = Describe("kubeblocks upgrade", func() {
	var cmd *cobra.Command
	var streams genericiooptions.IOStreams
	var tf *cmdtesting.TestFactory
	var actionCfg *action.Configuration
	var cfg *helm.Config

	Context("Upgrade", func() {
		BeforeEach(func() {
			streams, _, _, _ = genericiooptions.NewTestIOStreams()
			tf = cmdtesting.NewTestFactory().WithNamespace(namespace)
			tf.Client = &clientfake.RESTClient{}
			cfg = helm.NewFakeConfig(namespace)
			actionCfg, _ = helm.NewActionConfig(cfg)
			err := actionCfg.Releases.Create(&release.Release{
				Name:      testing.KubeBlocksChartName,
				Namespace: namespace,
				Version:   1,
				Info: &release.Info{
					Status: release.StatusDeployed,
				},
				Chart: &chart.Chart{
					Metadata: &chart.Metadata{
						Version: "0.3.0",
					},
				},
			})
			Expect(err).Should(BeNil())

		})

		AfterEach(func() {
			helm.ResetFakeActionConfig()
			tf.Cleanup()
		})

		mockKubeBlocksDeploy := func() *appsv1.Deployment {
			deploy := &appsv1.Deployment{}
			deploy.SetLabels(map[string]string{
				"app.kubernetes.io/component": "apps",
				"app.kubernetes.io/name":      types.KubeBlocksChartName,
				"app.kubernetes.io/version":   "0.3.0",
			})
			return deploy
		}

		It("check upgrade", func() {
			var cfg string
			cmd = newUpgradeCmd(tf, streams)
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

		It("double-check when version change", func() {
			o := &InstallOptions{
				Options: Options{
					IOStreams: streams,
					HelmCfg:   helm.NewFakeConfig(namespace),
					Namespace: "default",
					Client:    testing.FakeClientSet(mockKubeBlocksDeploy()),
					Dynamic:   testing.FakeDynamicClient(),
				},
				Version: "0.5.0-fake",
				Check:   false,
			}
			Expect(o.Upgrade()).Should(HaveOccurred())
			// o.In = bytes.NewBufferString("fake-version") mock input error
			// Expect(o.Upgrade()).Should(Succeed())
			o.autoApprove = true
			Expect(o.Upgrade()).Should(Succeed())

		})

		It("helm ValueOpts upgrade", func() {
			o := &InstallOptions{
				Options: Options{
					IOStreams: streams,
					HelmCfg:   helm.NewFakeConfig(namespace),
					Namespace: "default",
					Client:    testing.FakeClientSet(mockKubeBlocksDeploy()),
					Dynamic:   testing.FakeDynamicClient(),
				},
				Version: "",
			}
			o.ValueOpts.Values = []string{"replicaCount=2"}
			Expect(o.Upgrade()).Should(Succeed())
		})

		It("run upgrade", func() {
			o := &InstallOptions{
				Options: Options{
					IOStreams: streams,
					HelmCfg:   cfg,
					Namespace: "default",
					Client:    testing.FakeClientSet(mockKubeBlocksDeploy()),
					Dynamic:   testing.FakeDynamicClient(),
				},
				Version: version.DefaultKubeBlocksVersion,
				Check:   false,
			}
			Expect(o.Upgrade()).Should(Succeed())
			Expect(len(o.ValueOpts.Values)).To(Equal(0))
			Expect(o.upgradeChart()).Should(Succeed())
		})

		It("run upgrade when KB deploy is deleted", func() {
			o := &InstallOptions{
				Options: Options{
					IOStreams: streams,
					HelmCfg:   cfg,
					Namespace: "default",
					Client:    testing.FakeClientSet(),
					Dynamic:   testing.FakeDynamicClient(),
				},
				Version: version.DefaultKubeBlocksVersion,
				Check:   false,
			}
			Expect(o.Upgrade()).Should(Succeed())
			Expect(len(o.ValueOpts.Values)).To(Equal(0))
			Expect(o.upgradeChart()).Should(Succeed())
		})
	})

})
