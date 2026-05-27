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

package preflight

import (
	troubleshoot "github.com/replicatedhq/troubleshoot/pkg/apis/troubleshoot/v1beta2"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("load_spec_test", func() {
	It("LoadPreflightSpec test, and expect success", func() {
		yamlCheckFiles := []string{"../testing/testdata/preflight.yaml"}
		Eventually(func(g Gomega) {
			preflightSpec, preflightName, err := LoadPreflightSpec(yamlCheckFiles, nil)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(len(preflightSpec.Spec.Analyzers)).Should(Equal(1))
			g.Expect(preflightName).NotTo(BeNil())
		}).Should(Succeed())
	})

	It("LoadPreflightSpec parses native troubleshoot preflight specs without requiring apiVersion", func() {
		preflightYAML := []byte(`
kind: Preflight
metadata:
  name: internal-preflight
spec:
  analyzers:
    - clusterVersion:
        outcomes:
          - pass:
              message: ok
`)

		preflightSpec, preflightName, err := LoadPreflightSpec(nil, [][]byte{preflightYAML})

		Expect(err).NotTo(HaveOccurred())
		Expect(preflightSpec).NotTo(BeNil())
		Expect(preflightSpec).To(BeAssignableToTypeOf(&troubleshoot.Preflight{}))
		Expect(preflightSpec.Name).To(Equal("internal-preflight"))
		Expect(preflightName).To(Equal("internal-preflight"))
	})

	It("LoadPreflightSpec rejects host preflight specs", func() {
		hostPreflightYAML := []byte(`
kind: HostPreflight
metadata:
  name: internal-host-preflight
spec:
  analyzers:
    - cpu:
        outcomes:
          - pass:
              message: ok
`)

		preflightSpec, preflightName, err := LoadPreflightSpec(nil, [][]byte{hostPreflightYAML})

		Expect(err).To(MatchError(`unsupported preflight kind "HostPreflight"`))
		Expect(preflightSpec).To(BeNil())
		Expect(preflightName).To(BeEmpty())
	})
})
