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

package cluster

import (
	"bytes"
	"io"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	cmdtesting "k8s.io/kubectl/pkg/cmd/testing"

	opsv1alpha1 "github.com/apecloud/kubeblocks/apis/operations/v1alpha1"

	"github.com/apecloud/kbcli/pkg/action"
	clitesting "github.com/apecloud/kbcli/pkg/testing"
	"github.com/apecloud/kbcli/pkg/types"
)

var _ = Describe("Expose", func() {
	const (
		namespace         = "test"
		pending           = "pending"
		running           = "running"
		failed            = "failed"
		succeed           = "succeed"
		all               = "all"
		statelessCompName = "stateless"
		statefulCompName  = "stateful"
	)

	var (
		streams genericiooptions.IOStreams
		tf      *cmdtesting.TestFactory
		opsName string
	)

	BeforeEach(func() {
		streams, _, _, _ = genericiooptions.NewTestIOStreams()
		tf = clitesting.NewTestFactory(namespace)
	})

	AfterEach(func() {
		tf.Cleanup()
	})

	generateOpsObject := func(opsType opsv1alpha1.OpsType, phase opsv1alpha1.OpsPhase) *opsv1alpha1.OpsRequest {
		ops := &opsv1alpha1.OpsRequest{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "list-ops-" + clitesting.GetRandomStr(),
				Namespace: namespace,
			},
			Spec: opsv1alpha1.OpsRequestSpec{
				ClusterName: "test-cluster",
				Type:        opsType,
			},
			Status: opsv1alpha1.OpsRequestStatus{
				Phase: phase,
			},
		}
		ops.Status.Components = map[string]opsv1alpha1.OpsRequestComponentStatus{
			statelessCompName: {},
			statefulCompName:  {},
		}
		return ops
	}

	initOpsRequests := func() {
		opsKeys := []struct {
			opsType opsv1alpha1.OpsType
			phase   opsv1alpha1.OpsPhase
		}{
			{opsv1alpha1.UpgradeType, opsv1alpha1.OpsPendingPhase},
			{opsv1alpha1.HorizontalScalingType, opsv1alpha1.OpsFailedPhase},
			{opsv1alpha1.HorizontalScalingType, opsv1alpha1.OpsSucceedPhase},
			{opsv1alpha1.RestartType, opsv1alpha1.OpsSucceedPhase},
			{opsv1alpha1.VerticalScalingType, opsv1alpha1.OpsRunningPhase},
			{opsv1alpha1.VerticalScalingType, opsv1alpha1.OpsFailedPhase},
			{opsv1alpha1.VerticalScalingType, opsv1alpha1.OpsRunningPhase},
		}
		opsList := make([]runtime.Object, len(opsKeys))
		for i := range opsKeys {
			opsList[i] = generateOpsObject(opsKeys[i].opsType, opsKeys[i].phase)
		}
		opsName = opsList[0].(*opsv1alpha1.OpsRequest).Name
		tf.FakeDynamicClient = clitesting.FakeDynamicClient(opsList...)
	}

	getStdoutLinesCount := func(out io.Writer) int {
		b := out.(*bytes.Buffer).String()
		b = strings.Trim(b, "\n")
		return len(strings.Split(b, "\n"))
	}

	initOpsOption := func(status []string, opsTypes []string) *opsListOptions {
		streams, _, _, _ = genericiooptions.NewTestIOStreams()
		return &opsListOptions{
			ListOptions: action.NewListOptions(tf, streams, types.OpsGVR()),
			status:      status,
			opsType:     opsTypes,
		}
	}

	It("list ops", func() {
		By("new list ops command")
		cmd := NewListOpsCmd(tf, streams)
		Expect(cmd).ShouldNot(BeNil())

		By("init opsRequests for testing")
		initOpsRequests()

		By("test run cmd")
		cmd.Run(cmd, nil)

		By("test status flag with default values")
		o := initOpsOption([]string{pending, running, failed}, nil)
		Expect(o.printOpsList()).Should(Succeed())
		// title + filter ops
		Expect(getStdoutLinesCount(o.Out)).Should(Equal(6))

		By("test status flag with `all` keyword")
		o = initOpsOption([]string{all}, nil)
		Expect(o.printOpsList()).Should(Succeed())
		// title + filter ops
		Expect(getStdoutLinesCount(o.Out)).Should(Equal(8))

		By("test status flag with custom inputs")
		o = initOpsOption([]string{succeed}, nil)
		Expect(o.printOpsList()).Should(Succeed())
		// title + filter ops
		Expect(getStdoutLinesCount(o.Out)).Should(Equal(3))

		o = initOpsOption([]string{failed}, nil)
		Expect(o.printOpsList()).Should(Succeed())
		// title + filter ops
		Expect(getStdoutLinesCount(o.Out)).Should(Equal(3))

		By("test type flag")
		o = initOpsOption([]string{all}, []string{string(opsv1alpha1.RestartType)})
		Expect(o.printOpsList()).Should(Succeed())
		// title + filter ops
		Expect(getStdoutLinesCount(o.Out)).Should(Equal(2))

		o = initOpsOption([]string{all}, []string{string(opsv1alpha1.RestartType), string(opsv1alpha1.VerticalScalingType)})
		Expect(o.printOpsList()).Should(Succeed())
		// title + filter ops
		Expect(getStdoutLinesCount(o.Out)).Should(Equal(5))

		By("test component for upgrade ops")
		o = initOpsOption([]string{all}, []string{string(opsv1alpha1.UpgradeType)})
		Expect(o.printOpsList()).Should(Succeed())
		Expect(o.Out).Should(ContainSubstring(statefulCompName + "," + statelessCompName))

		By("list-ops with specified name")
		o = initOpsOption(nil, nil)
		o.opsRequestName = opsName
		Expect(o.printOpsList()).Should(Succeed())
		Expect(getStdoutLinesCount(o.Out)).Should(Equal(2))

		By("list-ops with not exist ops")
		o = initOpsOption(nil, nil)
		o.opsRequestName = "not-exist-ops"
		done := clitesting.Capture()
		Expect(o.printOpsList()).Should(Succeed())
		capturedOutput, _ := done()
		Expect(clitesting.ContainExpectStrings(capturedOutput, "No opsRequests found")).Should(BeTrue())

		By("list-ops with not exist ops")
		o = initOpsOption([]string{pending}, []string{string(opsv1alpha1.RestartType)})
		done = clitesting.Capture()
		Expect(o.printOpsList()).Should(Succeed())
		capturedOutput, _ = done()
		Expect(clitesting.ContainExpectStrings(capturedOutput, "kbcli cluster list-ops --status all")).Should(BeTrue())
	})

})
