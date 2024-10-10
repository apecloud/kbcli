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
	"bytes"
	"io"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	clientfake "k8s.io/client-go/rest/fake"
	cmdtesting "k8s.io/kubectl/pkg/cmd/testing"

	kbappsv1 "github.com/apecloud/kubeblocks/apis/apps/v1"
	appsv1beta1 "github.com/apecloud/kubeblocks/apis/apps/v1beta1"
	cfgcore "github.com/apecloud/kubeblocks/pkg/configuration/core"
	"github.com/apecloud/kubeblocks/pkg/controller/builder"
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
			ns                  = "default"
			clusterName         = "test-cluster"
			statefulCompDefName = "replicasets"
			statefulCompName    = "mysql"
			configSpecName      = "mysql-config-tpl"
			configVolumeName    = "mysql-config"
		)

		By("Create configmap and config constraint obj")
		configmap := testapps.NewCustomizedObj("resources/mysql-config-template.yaml", &corev1.ConfigMap{}, testapps.WithNamespace(ns))
		constraint := testapps.NewCustomizedObj("resources/mysql-config-constraint.yaml",
			&appsv1beta1.ConfigConstraint{})
		componentConfig := testapps.NewConfigMap(ns, cfgcore.GetComponentCfgName(clusterName, statefulCompName, configSpecName), testapps.SetConfigMapData("my.cnf", ""))
		By("Create a configuration obj")
		configObj := builder.NewConfigurationBuilder(ns, cfgcore.GenerateComponentConfigurationName(clusterName, statefulCompName)).
			ClusterRef(clusterName).
			Component(statefulCompName).
			AddConfigurationItem(kbappsv1.ComponentConfigSpec{
				ComponentTemplateSpec: kbappsv1.ComponentTemplateSpec{
					Name:        configSpecName,
					TemplateRef: configmap.Name,
					Namespace:   ns,
					VolumeName:  configVolumeName,
				},
				ConfigConstraintRef: constraint.Name,
			}).
			GetObject()
		By("creating a cluster")
		clusterObj := testapps.NewClusterFactory(ns, clusterName, "").
			AddComponent(statefulCompName, statefulCompDefName).GetObject()

		objs := []runtime.Object{configmap, constraint, clusterObj, componentConfig, configObj}
		ttf, ops := NewFakeOperationsOptions(ns, clusterObj.Name, objs...)
		o := &configOpsOptions{
			// nil cannot be set to a map struct in CueLang, so init the map of KeyValues.
			OperationsOptions: &OperationsOptions{
				CreateOptions: *ops,
			},
		}
		o.KeyValues = make(map[string]*string)
		o.HasPatch = true
		defer ttf.Cleanup()

		By("validate reconfiguring parameters")
		o.ComponentNames = []string{statefulCompName}
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

})
