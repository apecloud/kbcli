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

package cluster

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	cmdtesting "k8s.io/kubectl/pkg/cmd/testing"

	"github.com/apecloud/kbcli/pkg/testing"
)

var _ = Describe("cluster update", func() {
	var streams genericiooptions.IOStreams
	var tf *cmdtesting.TestFactory

	BeforeEach(func() {
		streams, _, _, _ = genericiooptions.NewTestIOStreams()
		tf = cmdtesting.NewTestFactory().WithNamespace("default")
	})

	AfterEach(func() {
		tf.Cleanup()
	})

	It("update command", func() {
		cmd := NewUpdateCmd(tf, streams)
		Expect(cmd).ShouldNot(BeNil())
	})

	Context("complete", func() {
		var o *UpdateOptions
		var cmd *cobra.Command
		var args []string
		BeforeEach(func() {
			cmd = NewUpdateCmd(tf, streams)
			o = NewUpdateOptions(tf, streams)
			args = []string{"c1"}

		})

		It("args is empty", func() {
			Expect(o.CmdComplete(cmd, nil)).Should(Succeed())
			Expect(o.Validate()).Should(HaveOccurred())
		})

		It("the length of args greater than 1", func() {
			Expect(o.CmdComplete(cmd, []string{"c1", "c2"})).Should(Succeed())
			Expect(o.Validate()).Should(HaveOccurred())
		})

		It("args only contains one cluster name", func() {
			Expect(o.CmdComplete(cmd, args)).Should(Succeed())
			Expect(o.Names[0]).Should(Equal("c1"))
		})

		It("set termination-policy", func() {
			Expect(cmd.Flags().Set("termination-policy", "Delete")).Should(Succeed())
			Expect(o.CmdComplete(cmd, args)).Should(Succeed())
			Expect(o.Complete()).Should(Succeed())
			Expect(o.namespace).Should(Equal("default"))
			Expect(o.dynamic).ShouldNot(BeNil())
			Expect(o.Patch).Should(ContainSubstring("terminationPolicy"))
		})

		It("set monitoring interval", func() {
			fakeCluster := testing.FakeCluster("c1", "default")
			fakeClusterDef := testing.FakeClusterDef()
			tf.FakeDynamicClient = testing.FakeDynamicClient(fakeCluster, fakeClusterDef)
			Expect(cmd.Flags().Set("disable-exporter", "false")).Should(Succeed())
			Expect(o.CmdComplete(cmd, args)).Should(Succeed())
			Expect(o.Complete()).Should(Succeed())
			Expect(o.Patch).Should(ContainSubstring("\"disableExporter\":false"))
		})

		It("set enable-all-logs", func() {
			fakeCluster := testing.FakeCluster("c1", "default")
			tf.FakeDynamicClient = testing.FakeDynamicClient(fakeCluster)
			Expect(cmd.Flags().Set("enable-all-logs", "false")).Should(Succeed())
			Expect(o.CmdComplete(cmd, args)).Should(Succeed())
			Expect(o.Complete()).Should(Succeed())
		})

		It("set node-labels", func() {
			fakeCluster := testing.FakeCluster("c1", "default")
			tf.FakeDynamicClient = testing.FakeDynamicClient(fakeCluster)
			Expect(cmd.Flags().Set("node-labels", "k1=v1,k2=v2")).Should(Succeed())
			Expect(o.CmdComplete(cmd, args)).Should(Succeed())
			Expect(o.Complete()).Should(Succeed())
			Expect(o.Patch).Should(ContainSubstring("k1"))
		})
	})
	/*
			Context("logs variables reconfiguring tests", func() {
				var (
					c        *kbappsv1.Cluster
					cd       *kbappsv1.ClusterDefinition
					myConfig string
				)
				BeforeEach(func() {
					c = testing.FakeCluster("c1", "default")
					cd = testing.FakeClusterDef()
					myConfig = `
		{{ block "logsBlock" . }}
		log_statements_unsafe_for_binlog=OFF
		log_error_verbosity=2
		log_output=FILE
		{{- if hasKey $.component "enabledLogs" }}
		{{- if mustHas "error" $.component.enabledLogs }}
		log_error=/data/mysql/log/mysqld-error.log
		{{- end }}
		{{- if mustHas "slow" $.component.enabledLogs }}
		slow_query_log=ON
		long_query_time=5
		slow_query_log_file=/data/mysql/log/mysqld-slowquery.log
		{{- end }}
		{{- if mustHas "general" $.component.enabledLogs }}
		general_log=ON
		general_log_file=/data/mysql/log/mysqld.log
		{{- end }}
		{{- end }}
		{{ end }}
		`
				})

				It("findFirstConfigSpec tests", func() {
					tests := []struct {
						compSpecs []kbappsv1.ClusterComponentSpec
						// cdCompSpecs []appsv1alpha1.ClusterComponentDefinition
						compName    string
						expectedErr bool
					}{
						{
							compSpecs: nil,
							//  cdCompSpecs: nil,
							compName:    "name",
							expectedErr: true,
						},
						{
							compSpecs: c.Spec.ComponentSpecs,
							//  cdCompSpecs: cd.Spec.ComponentDefs,
							compName:    testing.ComponentName,
							expectedErr: false,
						},
						{
							compSpecs: c.Spec.ComponentSpecs,
							//  cdCompSpecs: cd.Spec.ComponentDefs,
							compName:    "error-name",
							expectedErr: true,
						},
					}
					for _, test := range tests {
						configSpec, err := findFirstConfigSpec(test.compSpecs, test.cdCompSpecs, test.compName)
						if test.expectedErr {
							Expect(err).Should(HaveOccurred())
						} else {
							Expect(configSpec).ShouldNot(BeNil())
							Expect(err).ShouldNot(HaveOccurred())
						}
					}
				})

				It("findConfigTemplateInfo tests", func() {
					tests := []struct {
						dynamic     dynamic.Interface
						configSpec  *appsv1alpha1.ComponentConfigSpec
						expectedErr bool
					}{{
						dynamic:     nil,
						configSpec:  nil,
						expectedErr: true,
					}, {
						dynamic: testing.FakeDynamicClient(testing.FakeConfigMap("config-template", testing.Namespace, map[string]string{"fake": "fake"})),
						configSpec: &appsv1alpha1.ComponentConfigSpec{
							ComponentTemplateSpec: appsv1alpha1.ComponentTemplateSpec{
								TemplateRef: "config-template",
								Namespace:   testing.Namespace,
							},
						},
						expectedErr: true,
					}, {
						dynamic: testing.FakeDynamicClient(testing.FakeConfigMap("config-template", testing.Namespace, map[string]string{"fake": "fake"})),
						configSpec: &appsv1alpha1.ComponentConfigSpec{
							ComponentTemplateSpec: appsv1alpha1.ComponentTemplateSpec{
								TemplateRef: "config-template",
								Namespace:   testing.Namespace,
							},
						},
						expectedErr: true,
					}, {
						dynamic: testing.FakeDynamicClient(testing.FakeConfigMap("config-template", testing.Namespace, map[string]string{"fake": "fake"}), testing.FakeConfigConstraint("config-constraint")),
						configSpec: &appsv1alpha1.ComponentConfigSpec{
							ComponentTemplateSpec: appsv1alpha1.ComponentTemplateSpec{
								TemplateRef: "config-template",

								Namespace: testing.Namespace,
							},
							ConfigConstraintRef: "config-constraint",
						},
						expectedErr: false,
					}}
					for _, test := range tests {
						cm, format, err := findConfigTemplateInfo(test.dynamic, test.configSpec)
						if test.expectedErr {
							Expect(err).Should(HaveOccurred())
						} else {
							Expect(cm).ShouldNot(BeNil())
							Expect(format).ShouldNot(BeNil())
							Expect(err).ShouldNot(HaveOccurred())
						}
					}
				})

				It("findLogsBlockTPL tests", func() {
					tests := []struct {
						confData    map[string]string
						keyName     string
						expectedErr bool
						expectedNil bool
					}{{
						confData:    nil,
						keyName:     "",
						expectedErr: false,
						expectedNil: true,
					}, {
						confData: map[string]string{
							"test.cnf": "test",
							"my.cnf":   "{{ logsBlock",
						},
						keyName:     "my.cnf",
						expectedErr: true,
					}, {
						confData: map[string]string{
							"my.cnf": myConfig,
						},
						keyName:     "my.cnf",
						expectedErr: false,
					},
					}
					for _, test := range tests {
						key, tpl, err := findLogsBlockTPL(test.confData)
						if test.expectedErr {
							Expect(err).Should(HaveOccurred())
						} else {
							Expect(key).Should(Equal(test.keyName))
							if !test.expectedNil {
								Expect(tpl).ShouldNot(BeNil())
							}
							Expect(err).ShouldNot(HaveOccurred())
						}
					}
				})

				It("buildLogsTPLValues tests", func() {
					configSpec := testing.FakeCluster("test", "test").Spec.ComponentSpecs[0]
					tplValue, err := buildLogsTPLValues(&configSpec)
					Expect(err).ShouldNot(HaveOccurred())
					Expect(tplValue).ShouldNot(BeNil())
				})

				It("buildLogsReconfiguringOps tests", func() {
					opsRequest := buildLogsReconfiguringOps("clusterName", "namespace", "compName", "configName", "keyName", map[string]string{"key1": "value1", "key2": "value2"})
					Expect(opsRequest).ShouldNot(BeNil())
					Expect(opsRequest.Spec.Reconfigures[0].ComponentName).Should(Equal("compName"))
					Expect(opsRequest.Spec.Reconfigures[0].Configurations).Should(HaveLen(1))
					Expect(opsRequest.Spec.Reconfigures[0].Configurations[0].Keys).Should(HaveLen(1))
					Expect(opsRequest.Spec.Reconfigures[0].Configurations[0].Keys[0].Parameters).Should(HaveLen(2))
				})

			})
	*/
})
