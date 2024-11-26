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
	"fmt"
	"net/http"
	"time"

	appsv1 "github.com/apecloud/kubeblocks/apis/apps/v1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/utils/pointer"

	corev1 "k8s.io/api/core/v1"
	apiresource "k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	"k8s.io/cli-runtime/pkg/resource"
	"k8s.io/client-go/kubernetes/scheme"
	clientfake "k8s.io/client-go/rest/fake"
	cmdtesting "k8s.io/kubectl/pkg/cmd/testing"

	opsv1alpha1 "github.com/apecloud/kubeblocks/apis/operations/v1alpha1"

	clitesting "github.com/apecloud/kbcli/pkg/testing"
	"github.com/apecloud/kbcli/pkg/types"
)

var _ = Describe("Expose", func() {
	const (
		namespace      = "test"
		opsName        = "test-ops"
		componentName  = "test_stateful"
		componentName1 = "test_stateless"
	)

	var (
		streams genericiooptions.IOStreams
		tf      *cmdtesting.TestFactory
	)

	BeforeEach(func() {
		streams, _, _, _ = genericiooptions.NewTestIOStreams()
		tf = clitesting.NewTestFactory(namespace)
		codec := scheme.Codecs.LegacyCodec(scheme.Scheme.PrioritizedVersionsAllGroups()...)
		httpResp := func(obj runtime.Object) *http.Response {
			return &http.Response{StatusCode: http.StatusOK, Header: cmdtesting.DefaultHeader(), Body: cmdtesting.ObjBody(codec, obj)}
		}

		tf.UnstructuredClient = &clientfake.RESTClient{
			GroupVersion:         schema.GroupVersion{Group: types.AppsAPIGroup, Version: types.AppsAPIVersion},
			NegotiatedSerializer: resource.UnstructuredPlusDefaultContentConfig().NegotiatedSerializer,
			Client: clientfake.CreateHTTPClient(func(req *http.Request) (*http.Response, error) {
				urlPrefix := "/api/v1/namespaces/" + namespace
				mapping := map[string]*http.Response{
					"/api/v1/nodes/" + clitesting.NodeName: httpResp(clitesting.FakeNode()),
					urlPrefix + "/services":                httpResp(&corev1.ServiceList{}),
					urlPrefix + "/events":                  httpResp(&corev1.EventList{}),
					// urlPrefix + "/pods":                 httpResp(pods),
				}
				return mapping[req.URL.Path], nil
			}),
		}

		tf.Client = tf.UnstructuredClient
	})

	AfterEach(func() {
		tf.Cleanup()
	})

	It("describe", func() {
		cmd := NewDescribeOpsCmd(tf, streams)
		Expect(cmd).ShouldNot(BeNil())
	})

	It("complete", func() {
		o := newDescribeOpsOptions(tf, streams)
		Expect(o.complete(nil).Error()).Should(Equal("OpsRequest name should be specified"))
		Expect(o.complete([]string{opsName})).Should(Succeed())
		Expect(o.names).Should(Equal([]string{opsName}))
		Expect(o.client).ShouldNot(BeNil())
		Expect(o.dynamic).ShouldNot(BeNil())
		Expect(o.namespace).Should(Equal(namespace))
	})

	generateOpsObject := func(opsName string, opsType opsv1alpha1.OpsType) *opsv1alpha1.OpsRequest {
		return &opsv1alpha1.OpsRequest{
			ObjectMeta: metav1.ObjectMeta{
				Name:      opsName,
				Namespace: namespace,
			},
			Spec: opsv1alpha1.OpsRequestSpec{
				ClusterName: "test-cluster",
				Type:        opsType,
			},
		}
	}

	describeOps := func(opsType opsv1alpha1.OpsType, completeOps func(ops *opsv1alpha1.OpsRequest)) {
		randomStr := clitesting.GetRandomStr()
		ops := generateOpsObject(opsName+randomStr, opsType)
		completeOps(ops)
		tf.FakeDynamicClient = clitesting.FakeDynamicClient(ops)
		o := newDescribeOpsOptions(tf, streams)
		Expect(o.complete([]string{opsName + randomStr})).Should(Succeed())
		Expect(o.run()).Should(Succeed())
	}

	fakeOpsStatusAndProgress := func() opsv1alpha1.OpsRequestStatus {
		objectKey := "Pod/test-pod-wessxd"
		objectKey1 := "Pod/test-pod-xsdfwe"
		return opsv1alpha1.OpsRequestStatus{
			StartTimestamp:      metav1.NewTime(time.Now().Add(-1 * time.Minute)),
			CompletionTimestamp: metav1.NewTime(time.Now()),
			Progress:            "1/2",
			Phase:               opsv1alpha1.OpsFailedPhase,
			Components: map[string]opsv1alpha1.OpsRequestComponentStatus{
				componentName: {
					Phase: appsv1.FailedComponentPhase,
					ProgressDetails: []opsv1alpha1.ProgressStatusDetail{
						{
							ObjectKey: objectKey,
							Status:    opsv1alpha1.SucceedProgressStatus,
							StartTime: metav1.NewTime(time.Now().Add(-59 * time.Second)),
							EndTime:   metav1.NewTime(time.Now().Add(-39 * time.Second)),
							Message:   fmt.Sprintf("Successfully vertical scale Pod: %s in Component: %s", objectKey, componentName),
						},
						{
							ObjectKey: objectKey1,
							Status:    opsv1alpha1.FailedProgressStatus,
							StartTime: metav1.NewTime(time.Now().Add(-39 * time.Second)),
							EndTime:   metav1.NewTime(time.Now().Add(-1 * time.Second)),
							Message:   fmt.Sprintf("Failed to vertical scale Pod: %s in Component: %s", objectKey1, componentName),
						},
					},
				},
			},
			Conditions: []metav1.Condition{
				{
					Type:    "Processing",
					Reason:  "ProcessingOps",
					Status:  metav1.ConditionTrue,
					Message: "Start to process the OpsRequest.",
				},
				{
					Type:    "Failed",
					Reason:  "FailedScale",
					Status:  metav1.ConditionFalse,
					Message: "Failed to process the OpsRequest.",
				},
			},
		}
	}

	testPrintLastConfiguration := func(config opsv1alpha1.LastConfiguration,
		opsType opsv1alpha1.OpsType, expectStrings ...string) {
		o := newDescribeOpsOptions(tf, streams)
		if opsType == opsv1alpha1.UpgradeType {
			// capture stdout
			done := clitesting.Capture()
			o.printLastConfiguration(config, opsType)
			capturedOutput, err := done()
			Expect(err).Should(Succeed())
			Expect(clitesting.ContainExpectStrings(capturedOutput, expectStrings...)).Should(BeTrue())
			return
		}
		o.printLastConfiguration(config, opsType)
		out := o.Out.(*bytes.Buffer)
		Expect(clitesting.ContainExpectStrings(out.String(), expectStrings...)).Should(BeTrue())
	}

	It("run", func() {
		By("test describe Upgrade")
		describeOps(opsv1alpha1.UpgradeType, func(ops *opsv1alpha1.OpsRequest) {
			ops.Spec.Upgrade = &opsv1alpha1.Upgrade{
				Components: []opsv1alpha1.UpgradeComponent{
					{
						ComponentOps: opsv1alpha1.ComponentOps{
							ComponentName: componentName,
						},
						ServiceVersion: pointer.String("14.8.0"),
					},
				},
			}
		})

		By("test describe Restart")
		describeOps(opsv1alpha1.RestartType, func(ops *opsv1alpha1.OpsRequest) {
			ops.Spec.RestartList = []opsv1alpha1.ComponentOps{
				{ComponentName: componentName},
				{ComponentName: componentName1},
			}
		})

		By("test describe VerticalScaling")
		resourceRequirements := corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				"cpu":    apiresource.MustParse("100m"),
				"memory": apiresource.MustParse("200Mi"),
			},
			Limits: corev1.ResourceList{
				"cpu":    apiresource.MustParse("300m"),
				"memory": apiresource.MustParse("400Mi"),
			},
		}
		fakeVerticalScalingSpec := func() []opsv1alpha1.VerticalScaling {
			return []opsv1alpha1.VerticalScaling{
				{
					ComponentOps: opsv1alpha1.ComponentOps{
						ComponentName: componentName,
					},
					ResourceRequirements: resourceRequirements,
				},
			}
		}
		describeOps(opsv1alpha1.VerticalScalingType, func(ops *opsv1alpha1.OpsRequest) {
			ops.Spec.VerticalScalingList = fakeVerticalScalingSpec()
		})

		By("test describe HorizontalScaling")
		describeOps(opsv1alpha1.HorizontalScalingType, func(ops *opsv1alpha1.OpsRequest) {
			ops.Spec.HorizontalScalingList = []opsv1alpha1.HorizontalScaling{
				{
					ComponentOps: opsv1alpha1.ComponentOps{
						ComponentName: componentName,
					},
					ScaleOut: &opsv1alpha1.ScaleOut{
						ReplicaChanger: opsv1alpha1.ReplicaChanger{ReplicaChanges: pointer.Int32(1)},
					},
				},
			}
		})

		By("test describe VolumeExpansion and print OpsRequest status")
		volumeClaimTemplates := []opsv1alpha1.OpsRequestVolumeClaimTemplate{
			{
				Name:    "data",
				Storage: apiresource.MustParse("2Gi"),
			},
			{
				Name:    "log",
				Storage: apiresource.MustParse("4Gi"),
			},
		}
		describeOps(opsv1alpha1.VolumeExpansionType, func(ops *opsv1alpha1.OpsRequest) {
			ops.Spec.VolumeExpansionList = []opsv1alpha1.VolumeExpansion{
				{
					ComponentOps: opsv1alpha1.ComponentOps{
						ComponentName: componentName,
					},
					VolumeClaimTemplates: volumeClaimTemplates,
				},
			}
		})

		By("test printing OpsRequest status and conditions")
		describeOps(opsv1alpha1.VerticalScalingType, func(ops *opsv1alpha1.OpsRequest) {
			ops.Spec.VerticalScalingList = fakeVerticalScalingSpec()
			ops.Status = fakeOpsStatusAndProgress()
		})

		By("test verticalScaling last configuration")
		testPrintLastConfiguration(opsv1alpha1.LastConfiguration{
			Components: map[string]opsv1alpha1.LastComponentConfiguration{
				componentName: {
					ResourceRequirements: resourceRequirements,
				},
			},
		}, opsv1alpha1.VerticalScalingType, "100m", "200Mi", "300m", "400Mi",
			"REQUEST-CPU", "REQUEST-MEMORY", "LIMIT-CPU", "LIMIT-MEMORY")

		By("test HorizontalScaling last configuration")
		replicas := int32(2)
		testPrintLastConfiguration(opsv1alpha1.LastConfiguration{
			Components: map[string]opsv1alpha1.LastComponentConfiguration{
				componentName: {
					Replicas: &replicas,
				},
			},
		}, opsv1alpha1.HorizontalScalingType, "COMPONENT", "REPLICAS", componentName, "2")

		By("test VolumeExpansion last configuration")
		testPrintLastConfiguration(opsv1alpha1.LastConfiguration{
			Components: map[string]opsv1alpha1.LastComponentConfiguration{
				componentName: {
					VolumeClaimTemplates: volumeClaimTemplates,
				},
			},
		}, opsv1alpha1.VolumeExpansionType, "VOLUME-CLAIM-TEMPLATE", "STORAGE", "data", "2Gi", "log")

	})
})
