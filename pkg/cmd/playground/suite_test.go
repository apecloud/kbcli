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

package playground

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	cp "github.com/apecloud/kbcli/pkg/cloudprovider"
	clitesting "github.com/apecloud/kbcli/pkg/testing"
	"github.com/apecloud/kbcli/pkg/types"
)

func TestPlayground(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "PlayGround Suite")
}

var _ = BeforeSuite(func() {
	// set fake image info
	cp.K3sImageDefault = "fake-k3s-image"
	cp.K3dProxyImageDefault = "fake-k3d-proxy-image"

	// set default cluster name to test
	types.K3dClusterName = "kb-playground-test"
	kbClusterName = "kb-playground-test-cluster"

	// use a fake URL to test
	types.KubeBlocksRepoName = clitesting.KubeBlocksRepoName
	types.KubeBlocksChartName = clitesting.KubeBlocksChartName
	types.KubeBlocksChartURL = clitesting.KubeBlocksChartURL
})
