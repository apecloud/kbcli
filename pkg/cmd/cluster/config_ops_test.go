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

package cluster

import (
	"bytes"
	"io"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	clientfake "k8s.io/client-go/rest/fake"
	cmdtesting "k8s.io/kubectl/pkg/cmd/testing"

	kbappsv1 "github.com/apecloud/kubeblocks/apis/apps/v1"
	parametersv1alpha1 "github.com/apecloud/kubeblocks/apis/parameters/v1alpha1"
	kbfakeclient "github.com/apecloud/kubeblocks/pkg/client/clientset/versioned/fake"
	cfgcore "github.com/apecloud/kubeblocks/pkg/parameters/core"
	testapps "github.com/apecloud/kubeblocks/pkg/testutil/apps"

	"github.com/apecloud/kbcli/pkg/testing"
)

var _ = Describe("reconfigure test", func() {
	const (
		clusterName  = "cluster-ops"
		clusterName1 = "cluster-ops1"
	)
	var (
		streams genericiooptions.IOStreams
		tf      *cmdtesting.TestFactory
		in      *bytes.Buffer
	)

	BeforeEach(func() {
		streams, in, _, _ = genericiooptions.NewTestIOStreams()
		tf = cmdtesting.NewTestFactory().WithNamespace(testing.Namespace)
		clusterWithTwoComps := testing.FakeCluster(clusterName, testing.Namespace)
		clusterWithOneComp := clusterWithTwoComps.DeepCopy()
		clusterWithOneComp.Name = clusterName1
		clusterWithOneComp.Spec.ComponentSpecs = []kbappsv1.ClusterComponentSpec{
			clusterWithOneComp.Spec.ComponentSpecs[0],
		}
		tf.FakeDynamicClient = testing.FakeDynamicClient(testing.FakeClusterDef(),
			clusterWithTwoComps, clusterWithOneComp)
		tf.Client = &clientfake.RESTClient{}
	})

	AfterEach(func() {
		tf.Cleanup()
	})

	It("check params for reconfiguring operations", func() {
		const (
			ns               = "default"
			clusterName      = "test-cluster"
			statefulCompName = "mysql"
			configSpecName   = "mysql-config-tpl"
		)

		By("Create configmap and config constraint obj")
		configmap := testapps.NewCustomizedObj("resources/mysql-config-template.yaml", &corev1.ConfigMap{}, testapps.WithNamespace(ns), testapps.WithName(testing.FakeMysqlTemplateName))
		componentConfig := testapps.NewConfigMap(ns, cfgcore.GetComponentCfgName(clusterName, statefulCompName, configSpecName), setConfigMapData("my.cnf", ""))
		objs := []runtime.Object{configmap, componentConfig}
		ttf, ops := NewFakeOperationsOptions(ns, clusterName, objs...)
		o := &configOpsOptions{
			// nil cannot be set to a map struct in CueLang, so init the map of KeyValues.
			OperationsOptions: &OperationsOptions{
				CreateOptions: *ops,
			},
		}
		o.KeyValues = make(map[string]*string)
		o.HasPatch = true
		o.clientSet = kbfakeclient.NewSimpleClientset(
			testing.FakeCluster(clusterName, ns),
			testing.FakeCompDef(),
			testing.FakeParameterDefinition(),
			testing.FakeParameterConfigRenderer(),
		)
		defer ttf.Cleanup()

		By("validate reconfiguring parameters")
		o.ComponentNames = []string{testing.ComponentName}
		_, err := o.parseUpdatedParams()
		Expect(err.Error()).To(ContainSubstring(missingUpdatedParametersErrMessage))
		o.Parameters = []string{"abcd"}

		_, err = o.parseUpdatedParams()
		Expect(err.Error()).To(ContainSubstring("updated parameter format"))
		o.Parameters = []string{"abcd=test"}
		o.CfgTemplateName = configSpecName
		o.IOStreams = streams
		in.Write([]byte(o.Name + "\n"))

		Expect(o.Complete()).Should(Succeed())

		in := &bytes.Buffer{}
		in.Write([]byte("yes\n"))

		o.CreateOptions.In = io.NopCloser(in)
		Expect(o.Validate()).Should(Succeed())
	})

	It("detects v1.2 and legacy dynamic reload support", func() {
		mergeReloadAndRestart := false
		Expect(supportsDynamicReload(&parametersv1alpha1.ParametersDefinitionSpec{
			MergeReloadAndRestart: &mergeReloadAndRestart,
		}, nil)).Should(BeFalse())

		Expect(supportsDynamicReload(&parametersv1alpha1.ParametersDefinitionSpec{}, &kbappsv1.ComponentFileTemplate{
			Reconfigure: &kbappsv1.Action{},
		})).Should(BeTrue())

		Expect(supportsDynamicReload(&parametersv1alpha1.ParametersDefinitionSpec{
			ReloadAction: &parametersv1alpha1.ReloadAction{
				ShellTrigger: &parametersv1alpha1.ShellTrigger{},
			},
		}, nil)).Should(BeTrue())

		Expect(supportsDynamicReload(&parametersv1alpha1.ParametersDefinitionSpec{}, nil)).Should(BeFalse())
	})

	It("uses config template name to transform typed parameter values", func() {
		value := "true"
		params := map[string]*parametersv1alpha1.ParametersInFile{
			"my.cnf": {
				Parameters: map[string]*string{
					"enabled": &value,
				},
			},
		}
		rctx := &ReconfigureContext{
			ConfigRender: &parametersv1alpha1.ParamConfigRenderer{
				Spec: parametersv1alpha1.ParamConfigRendererSpec{
					Configs: []parametersv1alpha1.ComponentConfigDescription{{
						Name:         "my.cnf",
						TemplateName: "mysql-config",
						FileFormatConfig: &parametersv1alpha1.FileFormatConfig{
							Format: parametersv1alpha1.JSON,
						},
					}},
				},
			},
			ParametersDefs: []*parametersv1alpha1.ParametersDefinition{{
				Spec: parametersv1alpha1.ParametersDefinitionSpec{
					FileName: "my.cnf",
					ParametersSchema: &parametersv1alpha1.ParametersSchema{
						SchemaInJSON: &apiextensionsv1.JSONSchemaProps{
							Type: "object",
							Properties: map[string]apiextensionsv1.JSONSchemaProps{
								"spec": {
									Type: "object",
									Properties: map[string]apiextensionsv1.JSONSchemaProps{
										"enabled": {Type: "boolean"},
									},
								},
							},
						},
					},
				},
			}},
		}

		result, err := transformConfigParams(rctx, "mysql-config", params)
		Expect(err).Should(Succeed())
		Expect(result).Should(HaveLen(1))
		Expect(result[0].Key).Should(Equal("my.cnf"))
		Expect(result[0].UpdatedParams["enabled"]).Should(BeTrue())
	})

})

func setConfigMapData(key string, value string) func(*corev1.ConfigMap) {
	return func(cm *corev1.ConfigMap) {
		cm.Data[key] = value
	}
}
