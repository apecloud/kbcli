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

package util

import (
	"context"
	"regexp"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
)

const (
	topologyRegionLabel = "topology.kubernetes.io/region"

	dockerRegistry = "docker.io"
)

type K8sProvider string

const (
	EKSProvider     K8sProvider = "EKS"
	GKEProvider     K8sProvider = "GKE"
	AKSProvider     K8sProvider = "AKS"
	ACKProvider     K8sProvider = "ACK"
	TKEProvider     K8sProvider = "TKE"
	KINDProvider    K8sProvider = "Kind"
	K3SProvider     K8sProvider = "K3S"
	UnknownProvider K8sProvider = "unknown"
)

func (p K8sProvider) IsCloud() bool {
	return p != UnknownProvider
}

var (
	/*
		EKS version info:
		WARNING: This version information is deprecated and will be replaced with the output from kubectl version --short.  Use --output=yaml|json to get the full version.
		Client Version: version.Info{Major:"1", Minor:"26", GitVersion:"v1.26.1", GitCommit:"8f94681cd294aa8cfd3407b8191f6c70214973a4", GitTreeState:"clean", BuildDate:"2023-01-18T15:51:24Z", GoVersion:"go1.19.5", Compiler:"gc", Platform:"darwin/arm64"}
		Kustomize Version: v4.5.7
		Server Version: version.Info{Major:"1", Minor:"24+", GitVersion:"v1.24.10-eks-48e63af", GitCommit:"9176fb99b52f8d5ff73d67fea27f3a638f679f8a", GitTreeState:"clean", BuildDate:"2023-01-24T19:17:48Z", GoVersion:"go1.19.5", Compiler:"gc", Platform:"linux/amd64"}
		WARNING: version difference between client (1.26) and server (1.24) exceeds the supported minor version skew of +/-1

		GKE version info:
		WARNING: This version information is deprecated and will be replaced with the output from kubectl version --short.  Use --output=yaml|json to get the full version.
		Client Version: version.Info{Major:"1", Minor:"26", GitVersion:"v1.26.1", GitCommit:"8f94681cd294aa8cfd3407b8191f6c70214973a4", GitTreeState:"clean", BuildDate:"2023-01-18T15:51:24Z", GoVersion:"go1.19.5", Compiler:"gc", Platform:"darwin/arm64"}
		Kustomize Version: v4.5.7
		Server Version: version.Info{Major:"1", Minor:"24", GitVersion:"v1.24.9-gke.3200", GitCommit:"92ea556d4e7418d0e7b5db1ee576a73f8fc47e91", GitTreeState:"clean", BuildDate:"2023-01-20T09:29:29Z", GoVersion:"go1.18.9b7", Compiler:"gc", Platform:"linux/amd64"}
		WARNING: version difference between client (1.26) and server (1.24) exceeds the supported minor version skew of +/-1

		ACK version info:
		WARNING: This version information is deprecated and will be replaced with the output from kubectl version --short.  Use --output=yaml|json to get the full version.
		Client Version: version.Info{Major:"1", Minor:"26", GitVersion:"v1.26.1", GitCommit:"8f94681cd294aa8cfd3407b8191f6c70214973a4", GitTreeState:"clean", BuildDate:"2023-01-18T15:51:24Z", GoVersion:"go1.19.5", Compiler:"gc", Platform:"darwin/arm64"}
		Kustomize Version: v4.5.7
		Server Version: version.Info{Major:"1", Minor:"24+", GitVersion:"v1.24.6-aliyun.1", GitCommit:"e0e067a81f9fa91d46792937d79ec41ec79762eb", GitTreeState:"clean", BuildDate:"2023-02-28T12:15:08Z", GoVersion:"go1.18.6", Compiler:"gc", Platform:"linux/amd64"}
		WARNING: version difference between client (1.26) and server (1.24) exceeds the supported minor version skew of +/-1

		TKE version info:
		WARNING: This version information is deprecated and will be replaced with the output from kubectl version --short.  Use --output=yaml|json to get the full version.
		Client Version: version.Info{Major:"1", Minor:"26", GitVersion:"v1.26.1", GitCommit:"8f94681cd294aa8cfd3407b8191f6c70214973a4", GitTreeState:"clean", BuildDate:"2023-01-18T15:51:24Z", GoVersion:"go1.19.5", Compiler:"gc", Platform:"darwin/arm64"}
		Kustomize Version: v4.5.7
		Server Version: version.Info{Major:"1", Minor:"24+", GitVersion:"v1.24.4-tke.5", GitCommit:"c52d4f7343b73cbdf73e5bf0ca82ccdc2d54a07a", GitTreeState:"clean", BuildDate:"2023-02-07T01:40:47Z", GoVersion:"go1.18.8", Compiler:"gc", Platform:"linux/amd64"}
		WARNING: version difference between client (1.26) and server (1.24) exceeds the supported minor version skew of +/-1
	*/
	k8sVersionRegex = map[K8sProvider]string{
		EKSProvider: "v.*-eks-.*",
		GKEProvider: "v.*-gke.*",
		ACKProvider: "v.*-aliyun.*",
		TKEProvider: "v.*-tke.*",
	}
)

// GetK8sProvider returns the k8s provider
func GetK8sProvider(version string, client kubernetes.Interface) (K8sProvider, error) {
	// get provider from version first
	provider := GetK8sProviderFromVersion(version)
	if provider != UnknownProvider {
		return provider, nil
	}

	// if provider is unknown, get provider from node
	nodes, err := client.CoreV1().Nodes().List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return UnknownProvider, err
	}
	return GetK8sProviderFromNodes(nodes), nil
}

// GetK8sProviderFromNodes get k8s provider from node.spec.providerID
func GetK8sProviderFromNodes(nodes *corev1.NodeList) K8sProvider {
	for _, node := range nodes.Items {
		parts := strings.SplitN(node.Spec.ProviderID, ":", 2)
		if len(parts) != 2 {
			continue
		}
		switch parts[0] {
		case "aws":
			return EKSProvider
		case "azure":
			return AKSProvider
		case "gce":
			return GKEProvider
		case "qcloud":
			return TKEProvider
		case "kind":
			return KINDProvider
		case "k3s":
			return K3SProvider
		}
	}
	return UnknownProvider
}

// GetK8sProviderFromVersion gets k8s provider from field GitVersion in cluster server version
func GetK8sProviderFromVersion(version string) K8sProvider {
	for provider, reg := range k8sVersionRegex {
		match, err := regexp.Match(reg, []byte(version))
		if err != nil {
			continue
		}
		if match {
			return provider
		}
	}
	return UnknownProvider
}

func GetK8sSemVer(version string) string {
	removeFirstChart := func(v string) string {
		if len(v) == 0 {
			return v
		}
		if v[0] == 'v' {
			return v[1:]
		}
		return v
	}

	if len(version) == 0 {
		return version
	}

	strArr := strings.Split(version, "-")
	if len(strArr) == 0 {
		return ""
	}
	return removeFirstChart(strArr[0])
}

// GetImageRegistryByProvider returns the image registry based on the k8s provider,
// for different providers, we will use different image registry.
//
// Now, KubeBlocks has two image registries: docker.io and apecloud-registry.cn-zhangjiakou.cr.aliyuncs.com.
// KubeBlocks default image registry is apecloud-registry.cn-zhangjiakou.cr.aliyuncs.com,
// for some providers, or some regions, we should use docker.io as the image registry.
func GetImageRegistryByProvider(client kubernetes.Interface) (string, error) {
	v, err := GetK8sVersion(client.Discovery())
	if err != nil {
		return "", err
	}

	getImageRegistryByProvider := func(p K8sProvider) string {
		switch p {
		case GKEProvider:
			return dockerRegistry
		case UnknownProvider, KINDProvider, K3SProvider:
			return ""
		default:
			return ""
		}
	}

	var nodes *corev1.NodeList
	provider := GetK8sProviderFromVersion(v)
	if provider == UnknownProvider {
		nodes, err = client.CoreV1().Nodes().List(context.Background(), metav1.ListOptions{})
		if err != nil {
			klog.Info("Failed to get nodes list", err)
			return "", err
		}
		provider = GetK8sProviderFromNodes(nodes)
	}

	// get image registry by kubernetes provider
	registry := getImageRegistryByProvider(provider)
	if registry != "" {
		return registry, nil
	}

	// can not get image registry by provider, get it by region
	if nodes == nil {
		nodes, err = client.CoreV1().Nodes().List(context.Background(), metav1.ListOptions{})
		if err != nil {
			klog.Info("Failed to get nodes list", err)
			return "", err
		}
	}

	getRegion := func() string {
		for _, node := range nodes.Items {
			region := node.Labels[topologyRegionLabel]
			if region != "" {
				return region
			}
		}
		return ""
	}

	region := getRegion()
	if region == "" {
		klog.Info("Failed to get region from nodes")
		return "", nil
	}

	// Region info:
	// aws: https://docs.aws.amazon.com/zh_cn/AWSEC2/latest/UserGuide/using-regions-availability-zones.html
	//   there are two regions in China: cn-north-1, cn-northwest-1
	// aliyun: https://help.aliyun.com/document_detail/40654.html
	// azure: https://azure.microsoft.com/en-us/explore/global-infrastructure/data-residency/#select-geography
	// huawei: https://developer.huaweicloud.com/endpoint
	// tencent: https://www.tencentcloud.com/zh/document/product/213/6091
	switch provider {
	case GKEProvider:
		registry = dockerRegistry
	case EKSProvider, ACKProvider:
		if !strings.HasPrefix(region, "cn-") {
			registry = dockerRegistry
		}
	case AKSProvider:
		if !strings.HasPrefix(region, "china") {
			registry = dockerRegistry
		}
	case TKEProvider:
		cnRegions := sets.New("ap-guangzhou", "ap-shanghai", "ap-nanjing", "ap-beijing", "ap-chengdu", "ap-chongqing")
		if !cnRegions.Has(region) {
			registry = dockerRegistry
		}
	}
	return registry, nil
}
