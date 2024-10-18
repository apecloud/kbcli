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

	"bytes"
	"fmt"
	"strings"

	appsv1 "github.com/apecloud/kubeblocks/apis/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	clientfake "k8s.io/client-go/rest/fake"
	cmdtesting "k8s.io/kubectl/pkg/cmd/testing"

	opsv1alpha1 "github.com/apecloud/kubeblocks/apis/operations/v1alpha1"
	"github.com/apecloud/kubeblocks/pkg/constant"

	"github.com/apecloud/kbcli/pkg/testing"
)

var _ = Describe("operations", func() {
	const (
		clusterName            = "cluster-ops"
		clusterName1           = "cluster-ops1"
		clusterNameWithCompDef = "cluster-ops-with-comp-def"
		opsDefName             = "test-ops-def"
	)
	var (
		streams genericiooptions.IOStreams
		tf      *cmdtesting.TestFactory
		in      *bytes.Buffer
	)

	BeforeEach(func() {
		streams, in, _, _ = genericiooptions.NewTestIOStreams()
		tf = cmdtesting.NewTestFactory().WithNamespace(testing.Namespace)
		// init cluster with two components
		clusterWithTwoComps := testing.FakeCluster(clusterName, testing.Namespace)

		// init cluster with one component
		clusterWithOneComp := clusterWithTwoComps.DeepCopy()
		clusterWithOneComp.Name = clusterName1
		clusterWithOneComp.Spec.ComponentSpecs = []appsv1.ClusterComponentSpec{
			clusterWithOneComp.Spec.ComponentSpecs[0],
		}

		// init cluster with one component and componentDefinition
		clusterWithCompDef := clusterWithOneComp.DeepCopy()
		clusterWithCompDef.Name = clusterNameWithCompDef
		clusterWithCompDef.Spec.ComponentSpecs = []appsv1.ClusterComponentSpec{
			clusterWithCompDef.Spec.ComponentSpecs[0],
		}
		clusterWithCompDef.Spec.ComponentSpecs[0].ComponentDef = testing.CompDefName

		// init opsDefinition
		opsDef := &opsv1alpha1.OpsDefinition{
			ObjectMeta: metav1.ObjectMeta{Name: opsDefName},
			Spec: opsv1alpha1.OpsDefinitionSpec{
				ComponentInfos: []opsv1alpha1.ComponentInfo{
					{ComponentDefinitionName: testing.CompDefName},
				},
				ParametersSchema: &opsv1alpha1.ParametersSchema{
					OpenAPIV3Schema: &apiextensionsv1.JSONSchemaProps{
						Properties: map[string]apiextensionsv1.JSONSchemaProps{
							"p1": {Type: "string"},
							"p2": {Type: "integer"},
						},
						Required: []string{"p1"},
					},
				},
			},
		}
		pods := testing.FakePods(2, clusterWithOneComp.Namespace, clusterName)
		pod1s := testing.FakePods(2, clusterWithOneComp.Namespace, clusterName1)
		podsWithCompDef := testing.FakePods(2, clusterWithCompDef.Namespace, clusterNameWithCompDef)
		tf.Client = &clientfake.RESTClient{}
		tf.FakeDynamicClient = testing.FakeDynamicClient(testing.FakeClusterDef(),
			testing.FakeCompDef(), clusterWithTwoComps, clusterWithOneComp, clusterWithCompDef,
			&pods.Items[0], &pods.Items[1], &pod1s.Items[0], &pod1s.Items[1], &podsWithCompDef.Items[0], &podsWithCompDef.Items[1], opsDef)
	})

	AfterEach(func() {
		tf.Cleanup()
	})

	initCommonOperationOps := func(opsType opsv1alpha1.OpsType, clusterName string, hasComponentNamesFlag bool, objs ...runtime.Object) *OperationsOptions {
		o := newBaseOperationsOptions(tf, streams, opsType, hasComponentNamesFlag)
		o.Dynamic = tf.FakeDynamicClient
		o.Client = testing.FakeClientSet(objs...)
		o.Name = clusterName
		o.Namespace = testing.Namespace
		return o
	}

	getOpsName := func(opsType opsv1alpha1.OpsType, phase opsv1alpha1.OpsPhase) string {
		return strings.ToLower(string(opsType)) + "-" + strings.ToLower(string(phase))
	}

	generationOps := func(opsType opsv1alpha1.OpsType, phase opsv1alpha1.OpsPhase) *opsv1alpha1.OpsRequest {
		return &opsv1alpha1.OpsRequest{
			ObjectMeta: metav1.ObjectMeta{
				Name:      getOpsName(opsType, phase),
				Namespace: testing.Namespace,
			},
			Spec: opsv1alpha1.OpsRequestSpec{
				ClusterName: "test-cluster",
				Type:        opsType,
			},
			Status: opsv1alpha1.OpsRequestStatus{
				Phase: phase,
			},
		}

	}

	It("Upgrade Ops", func() {
		o := newBaseOperationsOptions(tf, streams, opsv1alpha1.UpgradeType, false)
		o.Dynamic = tf.FakeDynamicClient

		By("validate o.name is null")
		Expect(o.Validate()).To(MatchError(missingClusterArgErrMassage))

		By("validate upgrade when cluster-version is null")
		o.Namespace = testing.Namespace
		o.Name = clusterName
		o.OpsType = opsv1alpha1.UpgradeType
		o.ComponentDefinitionName = "nil"
		o.ServiceVersion = "nil"
		Expect(o.Validate()).To(MatchError("missing component-def or service-version"))

		By("expect to validate success")
		o.ComponentDefinitionName = "fake-comp-def-1"
		in.Write([]byte(o.Name + "\n"))
		Expect(o.Validate()).Should(Succeed())
	})

	It("VolumeExpand Ops", func() {
		vctName := "data"
		persistentVolumeClaim := &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("%s-%s-%s-%d", vctName, clusterName, testing.ComponentName, 0),
				Namespace: testing.Namespace,
				Labels: map[string]string{
					constant.AppInstanceLabelKey:             clusterName,
					constant.VolumeClaimTemplateNameLabelKey: vctName,
					constant.KBAppComponentLabelKey:          testing.ComponentName,
				},
			},
			Spec: corev1.PersistentVolumeClaimSpec{
				AccessModes: []corev1.PersistentVolumeAccessMode{
					corev1.ReadWriteOnce,
				},
				Resources: corev1.VolumeResourceRequirements{
					Requests: corev1.ResourceList{
						"storage": resource.MustParse("3Gi"),
					},
				},
			},
			Status: corev1.PersistentVolumeClaimStatus{
				Capacity: map[corev1.ResourceName]resource.Quantity{
					"storage": resource.MustParse("1Gi"),
				},
			},
		}
		o := initCommonOperationOps(opsv1alpha1.VolumeExpansionType, clusterName, true, persistentVolumeClaim)
		By("validate volumeExpansion when components is null")
		Expect(o.Validate()).To(MatchError(`missing components, please specify the "--components" flag for the cluster`))

		By("validate volumeExpansion when vct-names is null")
		o.ComponentNames = []string{testing.ComponentName}
		Expect(o.Validate()).To(MatchError("missing volume-claim-templates"))

		By("validate volumeExpansion when storage is null")
		o.VCTNames = []string{vctName}
		Expect(o.Validate()).To(MatchError("missing storage"))

		By("validate recovery from volume expansion failure")
		o.Storage = "2Gi"
		Expect(o.Validate()).Should(Succeed())
		Expect(o.Out.(*bytes.Buffer).String()).To(ContainSubstring("Warning: this opsRequest is a recovery action for volume expansion failure and will re-create the PersistentVolumeClaims when RECOVER_VOLUME_EXPANSION_FAILURE=false"))

		By("validate passed")
		o.Storage = "4Gi"
		in.Write([]byte(o.Name + "\n"))
		Expect(o.Validate()).Should(Succeed())
	})

	It("Vscale Ops", func() {
		o := initCommonOperationOps(opsv1alpha1.VerticalScalingType, clusterName1, true)
		By("test CompleteComponentsFlag function")
		o.ComponentNames = nil
		By("expect to auto complete components when cluster has only one component")
		Expect(o.CompleteComponentsFlag()).Should(Succeed())
		Expect(o.ComponentNames[0]).Should(Equal(testing.ComponentName))

		By("validate invalid resource")
		o.CPU = "1g"
		o.Memory = "100Gi"
		in.Write([]byte(o.Name + "\n"))
		Expect(o.Validate()).Should(HaveOccurred())

		By("validate invalid resource")
		o.CPU = "1"
		o.Memory = "100MB"
		in.Write([]byte(o.Name + "\n"))
		Expect(o.Validate()).Should(HaveOccurred())
	})

	It("HScale Ops", func() {
		o := initCommonOperationOps(opsv1alpha1.HorizontalScalingType, clusterName1, true)
		By("test CompleteComponentsFlag function")
		o.ComponentNames = nil
		By("expect to auto complete components when cluster has only one component")
		Expect(o.CompleteComponentsFlag()).Should(Succeed())
		Expect(o.ComponentNames[0]).Should(Equal(testing.ComponentName))

		By("expect to Validate failed")
		in.Write([]byte(o.Name + "\n"))
		Expect(o.Validate()).Should(MatchError("at least one of --replicas or --offline-instances is required"))

		By("expect to Validate failed when scaling out replicas")
		o.ScaleOut = true
		o.OfflineInstancesToOnline = []string{"no-exist-pod"}
		in.Write([]byte(o.Name + "\n"))
		Expect(o.Validate()).Should(MatchError(fmt.Sprintf(`pod "%s" not in the offlineInstances of the component "%s"`, o.OfflineInstancesToOnline[0], testing.ComponentName)))

		By("expect to Validate success")
		o.Replicas = "1"
		o.OfflineInstancesToOnline = nil
		in.Write([]byte(o.Name + "\n"))
		Expect(o.Validate()).Should(Succeed())

		By("expect for componentNames is nil when cluster has only two component")
		o.Name = clusterName
		o.ComponentNames = nil
		Expect(o.CompleteComponentsFlag()).Should(Succeed())
		Expect(o.ComponentNames).Should(BeEmpty())
	})

	It("Restart ops", func() {
		o := initCommonOperationOps(opsv1alpha1.RestartType, clusterName, true)
		By("expect for not found error")
		o.Args = []string{clusterName + "2"}
		Expect(o.Complete())
		Expect(o.CompleteRestartOps().Error()).Should(ContainSubstring("not found"))

		By("expect for complete success")
		o.Name = clusterName
		Expect(o.CompleteRestartOps()).Should(Succeed())

		By("test Restart command")
		restartCmd := NewRestartCmd(tf, streams)
		_, _ = in.Write([]byte(clusterName + "\n"))
		done := testing.Capture()
		restartCmd.Run(restartCmd, []string{clusterName})
		capturedOutput, _ := done()
		Expect(testing.ContainExpectStrings(capturedOutput, "kbcli cluster describe-ops")).Should(BeTrue())
	})

	It("cancel ops", func() {
		By("init some opsRequests which are needed for canceling opsRequest")
		completedPhases := []opsv1alpha1.OpsPhase{opsv1alpha1.OpsCancelledPhase, opsv1alpha1.OpsSucceedPhase, opsv1alpha1.OpsFailedPhase}
		supportedOpsType := []opsv1alpha1.OpsType{opsv1alpha1.VerticalScalingType, opsv1alpha1.HorizontalScalingType}
		notSupportedOpsType := []opsv1alpha1.OpsType{opsv1alpha1.RestartType, opsv1alpha1.UpgradeType}
		processingPhases := []opsv1alpha1.OpsPhase{opsv1alpha1.OpsPendingPhase, opsv1alpha1.OpsCreatingPhase, opsv1alpha1.OpsRunningPhase}
		opsList := make([]runtime.Object, 0)
		for _, opsType := range supportedOpsType {
			for _, phase := range completedPhases {
				opsList = append(opsList, generationOps(opsType, phase))
			}
			for _, phase := range processingPhases {
				opsList = append(opsList, generationOps(opsType, phase))
			}
			// mock cancelling opsRequest
			opsList = append(opsList, generationOps(opsType, opsv1alpha1.OpsCancellingPhase))
		}

		for _, opsType := range notSupportedOpsType {
			opsList = append(opsList, generationOps(opsType, opsv1alpha1.OpsRunningPhase))
		}
		tf.FakeDynamicClient = testing.FakeDynamicClient(opsList...)

		By("expect an error for not supported phase")
		o := newBaseOperationsOptions(tf, streams, "", false)
		o.Dynamic = tf.FakeDynamicClient
		o.Namespace = testing.Namespace
		o.AutoApprove = true
		for _, phase := range completedPhases {
			for _, opsType := range supportedOpsType {
				o.Name = getOpsName(opsType, phase)
				Expect(cancelOps(o).Error()).Should(Equal(fmt.Sprintf("can not cancel the opsRequest when phase is %s", phase)))
			}
		}

		By("expect an error for not supported opsType")
		for _, opsType := range notSupportedOpsType {
			o.Name = getOpsName(opsType, opsv1alpha1.OpsRunningPhase)
			Expect(cancelOps(o).Error()).Should(Equal(fmt.Sprintf("opsRequest type: %s not support cancel action", opsType)))
		}

		By("expect an error for cancelling opsRequest")
		for _, opsType := range supportedOpsType {
			o.Name = getOpsName(opsType, opsv1alpha1.OpsCancellingPhase)
			Expect(cancelOps(o).Error()).Should(Equal(fmt.Sprintf(`opsRequest "%s" is cancelling`, o.Name)))
		}

		By("expect succeed for canceling the opsRequest which is processing")
		for _, phase := range processingPhases {
			for _, opsType := range supportedOpsType {
				o.Name = getOpsName(opsType, phase)
				Expect(cancelOps(o)).Should(Succeed())
			}
		}
	})

	It("Switchover ops base on cluster component definition", func() {
		o := initCommonOperationOps(opsv1alpha1.SwitchoverType, clusterName1, false)
		By("expect to auto complete components when cluster has only one component")
		Expect(o.CompleteComponentsFlag()).Should(Succeed())
		Expect(o.ComponentNames[0]).Should(Equal(testing.ComponentName))

		By("expect for componentNames is nil when cluster has only two component")
		o.Name = clusterName
		o.ComponentNames = nil
		Expect(o.CompleteComponentsFlag()).Should(Succeed())
		Expect(o.ComponentNames).Should(BeEmpty())

		By("validate failed because there are multi-components in cluster and not specify the component")
		Expect(o.CompleteComponentsFlag()).Should(Succeed())
		Expect(o.CompleteSwitchoverOps()).ShouldNot(Succeed())
		Expect(testing.ContainExpectStrings(o.CompleteSwitchoverOps().Error(), "there are multiple components in cluster, please use --component to specify the component for promote")).Should(BeTrue())

		By("validate failed because o.Instance is illegal ")
		o.Name = clusterName1
		o.Component = testing.ComponentName
		o.Instance = fmt.Sprintf("%s-%s-%d", clusterName1, testing.ComponentName, 5)
		Expect(o.Validate()).ShouldNot(Succeed())
		Expect(testing.ContainExpectStrings(o.Validate().Error(), "not found")).Should(BeTrue())

		By("validate failed because o.Instance is already leader and cannot be promoted")
		o.Instance = fmt.Sprintf("%s-%s-%d", clusterName1, testing.ComponentName, 0)
		Expect(o.Validate()).ShouldNot(Succeed())
		Expect(testing.ContainExpectStrings(o.Validate().Error(), "cannot be promoted because it is already the targetRole")).Should(BeTrue())

		By("validate failed because o.Instance does not belong to the current component")
		o.Instance = fmt.Sprintf("%s-%s-%d", clusterName, testing.ComponentName, 1)
		Expect(o.Validate()).ShouldNot(Succeed())
		Expect(testing.ContainExpectStrings(o.Validate().Error(), "does not belong to the current component")).Should(BeTrue())
	})

	It("Switchover ops base on component definition", func() {
		o := initCommonOperationOps(opsv1alpha1.SwitchoverType, clusterNameWithCompDef, false)
		By("expect to auto complete components when cluster has only one component")
		Expect(o.CompleteComponentsFlag()).Should(Succeed())
		Expect(o.ComponentNames[0]).Should(Equal(testing.ComponentName))

		By("expect for componentNames is nil when cluster has only two component")
		o.Name = clusterName
		o.ComponentNames = nil
		Expect(o.CompleteComponentsFlag()).Should(Succeed())
		Expect(o.ComponentNames).Should(BeEmpty())

		By("validate failed because there are multi-components in cluster and not specify the component")
		Expect(o.CompleteComponentsFlag()).Should(Succeed())
		Expect(o.CompleteSwitchoverOps()).ShouldNot(Succeed())
		Expect(testing.ContainExpectStrings(o.CompleteSwitchoverOps().Error(), "there are multiple components in cluster, please use --component to specify the component for promote")).Should(BeTrue())

		By("validate failed because o.Instance is illegal ")
		o.Name = clusterNameWithCompDef
		o.Instance = fmt.Sprintf("%s-%s-%d", clusterNameWithCompDef, testing.ComponentName, 5)
		Expect(o.Validate()).ShouldNot(Succeed())
		Expect(testing.ContainExpectStrings(o.Validate().Error(), "not found")).Should(BeTrue())

		By("validate failed because o.Instance is already leader and cannot be promoted")
		o.Instance = fmt.Sprintf("%s-%s-%d", clusterNameWithCompDef, testing.ComponentName, 0)
		Expect(o.Validate()).ShouldNot(Succeed())
		Expect(testing.ContainExpectStrings(o.Validate().Error(), "cannot be promoted because it is already the targetRole")).Should(BeTrue())

		By("validate failed because o.Instance does not belong to the current component")
		o.Instance = fmt.Sprintf("%s-%s-%d", clusterName1, testing.ComponentName, 1)
		o.Component = testing.ComponentName
		Expect(o.Validate()).ShouldNot(Succeed())
		Expect(testing.ContainExpectStrings(o.Validate().Error(), "does not belong to the current component")).Should(BeTrue())

	})

	It("Custom ops base on component definition", func() {
		o := initCommonOperationOps(opsv1alpha1.CustomType, clusterNameWithCompDef, false)
		_ = NewCustomOpsCmd(tf, streams)
		cmds := buildCustomOpsCmds(o)
		Expect(cmds).Should(HaveLen(1))

		By("expect required flags")
		cmd := cmds[0]
		Expect(cmd.Flag("p1")).ShouldNot(BeNil())
		Expect(cmd.Flag("p2")).ShouldNot(BeNil())
		Expect(cmd.Flag("p1").Annotations[cobra.BashCompOneRequiredFlag]).ShouldNot(BeEmpty())

		By("expect to create custom ops successfully")
		done := testing.Capture()
		_ = cmd.Flags().Set("p1", "test")
		cmd.Run(cmd, []string{clusterNameWithCompDef})
		capturedOutput, _ := done()
		Expect(testing.ContainExpectStrings(capturedOutput, "kbcli cluster describe-ops")).Should(BeTrue())
	})
})
