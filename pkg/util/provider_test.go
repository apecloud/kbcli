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

package util

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/apecloud/kbcli/pkg/testing"
)

var _ = Describe("provider util", func() {

	buildNodes := func(provider string, labels map[string]string) *corev1.NodeList {
		return &corev1.NodeList{
			Items: []corev1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Labels: labels,
					},
					Spec: corev1.NodeSpec{
						ProviderID: fmt.Sprintf("%s://blabla", provider),
					},
				},
			},
		}
	}
	It("GetK8sProvider", func() {
		cases := []struct {
			description    string
			version        string
			expectVersion  string
			expectProvider K8sProvider
			isCloud        bool
			nodes          *corev1.NodeList
		}{
			{
				"unknown provider without providerID and unique version identifier",
				"v1.25.0",
				"1.25.0",
				UnknownProvider,
				false,
				buildNodes("", nil),
			},
			{
				"EKS with unique version identifier",
				"v1.25.0-eks-123456",
				"1.25.0",
				EKSProvider,
				true,
				buildNodes("", nil),
			},
			{
				"EKS with providerID",
				"1.25.0",
				"1.25.0",
				EKSProvider,
				true,
				buildNodes("aws", nil),
			},
			{
				"GKE with unique version identifier",
				"v1.24.9-gke.3200",
				"1.24.9",
				GKEProvider,
				true,
				buildNodes("", nil),
			},
			{
				"GKE with providerID",
				"v1.24.9",
				"1.24.9",
				GKEProvider,
				true,
				buildNodes("gce", nil),
			},
			{
				"TKE with unique version identifier",
				"v1.24.4-tke.5",
				"1.24.4",
				TKEProvider,
				true,
				buildNodes("", nil),
			},
			{
				"TKE with providerID",
				"v1.24.9",
				"1.24.9",
				TKEProvider,
				true,
				buildNodes("qcloud", nil),
			},
			{
				"ACK with unique version identifier, as ACK don't have providerID",
				"v1.24.6-aliyun.1",
				"1.24.6",
				ACKProvider,
				true,
				buildNodes("", nil),
			},
			{
				"AKS with providerID, as AKS don't have unique version identifier",
				"v1.24.9",
				"1.24.9",
				AKSProvider,
				true,
				buildNodes("azure", nil),
			},
		}

		for _, c := range cases {
			By(c.description)
			Expect(GetK8sSemVer(c.version)).Should(Equal(c.expectVersion))
			client := testing.FakeClientSet(c.nodes)
			p, err := GetK8sProvider(c.version, client)
			Expect(err).ShouldNot(HaveOccurred())
			Expect(p).Should(Equal(c.expectProvider))
			Expect(p.IsCloud()).Should(Equal(c.isCloud))
		}
	})

	It("GetImageRegistry", func() {
		buildNodeWithRegion := func(provider, region string) *corev1.NodeList {
			labels := map[string]string{
				topologyRegionLabel: region,
			}
			return buildNodes(provider, labels)
		}

		cases := []struct {
			description           string
			version               string
			region                string
			expectedImageRegistry string
			nodes                 *corev1.NodeList
		}{
			{
				"Unknown provider",
				"v1.25.0",
				"",
				"",
				buildNodes("", nil),
			},
			{
				"EKS with region us-west-2",
				"v1.25.0-eks-123456",
				"us-west-2",
				"docker.io",
				buildNodeWithRegion("aws", "us-west-2"),
			},
			{
				// GCP DOES NOT have region with 'cn-*' prefix, so it should always
				// use 'docker.io' as the default registry.
				"GKE with region cn-north-1",
				"v1.24.9-gke.3200",
				"cn-north-1",
				"docker.io",
				buildNodeWithRegion("gce", "cn-north-1"),
			},
			{
				"TKE with region ap-guangzhou",
				"v1.24.4-tke.5",
				"ap-guangzhou",
				"",
				buildNodeWithRegion("qcloud", "ap-guangzhou"),
			},
			{
				"ACK with region cn-zhangjiakou",
				"v1.24.6-aliyun.1",
				"cn-zhangjiakou",
				"",
				buildNodeWithRegion("", "cn-zhangjiakou"),
			},
		}

		for _, c := range cases {
			By(c.description)
			client := testing.FakeClientSet(c.nodes)
			registry, err := GetImageRegistryByProvider(client)
			Expect(err).ShouldNot(HaveOccurred())
			Expect(registry).Should(Equal(c.expectedImageRegistry))
		}
	})
})
