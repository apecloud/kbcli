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

package cloudprovider

import (
	"context"
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("playground", func() {
	var (
		provider    = newLocalCloudProvider(os.Stdout, os.Stderr)
		clusterName = "k3d-kb-test"
		image       = "test.com/k3d-proxy:5.4.4"
	)

	Context("k3d util function", func() {
		It("with name", func() {
			config, err := buildClusterRunConfig("test", "", "")
			Expect(err).ShouldNot(HaveOccurred())
			Expect(config.Name).Should(ContainSubstring("test"))
			Expect(setUpK3d(context.Background(), nil)).Should(HaveOccurred())
			Expect(provider.DeleteK8sCluster(&K8sClusterInfo{ClusterName: clusterName})).Should(HaveOccurred())
		})

		It("with name and k3s image", func() {
			config, err := buildClusterRunConfig("test", image, "")
			Expect(err).ShouldNot(HaveOccurred())
			Expect(config.Cluster.Nodes[1].Image).Should(ContainSubstring("test.com"))
		})

		It("with name and k3d proxy image", func() {
			config, err := buildClusterRunConfig("test", "", image)
			Expect(err).ShouldNot(HaveOccurred())
			Expect(config.ServerLoadBalancer.Node.Image).Should(ContainSubstring("test.com"))
		})

	})

})
