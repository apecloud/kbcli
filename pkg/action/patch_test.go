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

package action

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	cmdtesting "k8s.io/kubectl/pkg/cmd/testing"

	"github.com/apecloud/kbcli/pkg/types"
)

var _ = Describe("Patch", func() {
	var streams genericiooptions.IOStreams
	var tf *cmdtesting.TestFactory

	BeforeEach(func() {
		streams, _, _, _ = genericiooptions.NewTestIOStreams()
		tf = cmdtesting.NewTestFactory().WithNamespace("default")
	})

	AfterEach(func() {
		tf.Cleanup()
	})

	It("complete", func() {
		cmd := &cobra.Command{}
		o := NewPatchOptions(tf, streams, types.ClusterGVR())
		o.AddFlags(cmd)
		Expect(o.complete()).Should(HaveOccurred())

		o.Names = []string{"c1"}
		Expect(o.complete()).Should(Succeed())
	})

	It("run", func() {
		cmd := &cobra.Command{}
		o := NewPatchOptions(tf, streams, types.CRDGVR())
		o.Names = []string{"test"}
		o.AddFlags(cmd)
		o.Patch = "{terminationPolicy: Delete}"
		// The resource "CRD" expect not found in mock K8s
		Expect(o.Run()).Should(HaveOccurred())
	})
})
