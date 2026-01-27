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
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"reflect"
	"strings"

	"github.com/apecloud/kubeblocks/pkg/constant"
	gv "github.com/hashicorp/go-version"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/kubernetes"

	"github.com/apecloud/kbcli/pkg/types"
	"github.com/apecloud/kbcli/version"
)

const (
	// KubeblocksAppComponent the value of app.kubernetes.io/component label for KubeBlocks deployment
	KubeblocksAppComponent = "apps"
	// DataprotectionAppComponent the value of app.kubernetes.io/component label for DataProtection deployment
	DataprotectionAppComponent = "dataprotection"
)

type Version struct {
	KubeBlocks string
	Kubernetes string
	Cli        string
}

// GetVersionInfo gets version include KubeBlocks, CLI and kubernetes
func GetVersionInfo(client kubernetes.Interface) (Version, error) {
	var err error
	v := Version{
		Cli: version.GetVersion(),
	}

	if client == nil || reflect.ValueOf(client).IsNil() {
		return v, nil
	}

	if v.Kubernetes, err = GetK8sVersion(client.Discovery()); err != nil {
		return v, err
	}

	if v.KubeBlocks, err = GetKubeBlocksVersion(client, metav1.NamespaceAll); err != nil {
		return v, err
	}

	return v, nil
}

// GetKubeBlocksVersion gets KubeBlocks version
func GetKubeBlocksVersion(client kubernetes.Interface, namespace string) (string, error) {
	deploys, err := GetKBDeploys(client, KubeblocksAppComponent, namespace)
	if err != nil || len(deploys) == 0 {
		return "", err
	}
	var versions []string
	for _, deploy := range deploys {
		labels := deploy.GetLabels()
		if labels == nil {
			return "", fmt.Errorf("KubeBlocks deployment has no labels")
		}

		v, ok := labels[constant.AppVersionLabelKey]
		if !ok {
			return "", fmt.Errorf("KubeBlocks deployment has no version label")
		}
		versions = append(versions, v)
	}

	return strings.Join(versions, ","), nil
}

// GetK8sVersion gets k8s server version
func GetK8sVersion(discoveryClient discovery.DiscoveryInterface) (string, error) {
	if discoveryClient == nil {
		return "", nil
	}

	serverVersion, err := discoveryClient.ServerVersion()
	if err != nil {
		return "", err
	}

	if serverVersion != nil {
		return serverVersion.GitVersion, nil
	}
	return "", nil
}

// GetKBDeploys gets KubeBlocks deployments, now one kubernetes cluster
// only support one KubeBlocks
func GetKBDeploys(client kubernetes.Interface, appName string, namespace string) ([]appsv1.Deployment, error) {
	labelSelector := fmt.Sprintf("app.kubernetes.io/name=%s,app.kubernetes.io/component=%s",
		types.KubeBlocksChartName, appName)
	deploys, err := client.AppsV1().Deployments(namespace).List(context.Background(),
		metav1.ListOptions{
			LabelSelector: labelSelector,
		})
	if err != nil {
		return nil, err
	}
	if deploys == nil || len(deploys.Items) == 0 {
		return nil, nil
	}
	return deploys.Items, nil
}

func GetKubeBlocksDeploys(client kubernetes.Interface) ([]appsv1.Deployment, error) {
	return GetKBDeploys(client, KubeblocksAppComponent, metav1.NamespaceAll)
}

func GetKubeBlocksDeploy(client kubernetes.Interface) (*appsv1.Deployment, error) {
	deploys, err := GetKBDeploys(client, KubeblocksAppComponent, metav1.NamespaceAll)
	if err != nil || len(deploys) == 0 {
		return nil, err
	}
	return &deploys[0], nil
}

func GetDataProtectionDeploys(client kubernetes.Interface) ([]appsv1.Deployment, error) {
	return GetKBDeploys(client, DataprotectionAppComponent, metav1.NamespaceAll)
}

// GetDataProtectionDeploy gets DataProtection deployments, now one kubernetes cluster
// only support one DataProtection
func GetDataProtectionDeploy(client kubernetes.Interface) (*appsv1.Deployment, error) {
	deploys, err := GetKBDeploys(client, DataprotectionAppComponent, metav1.NamespaceAll)
	if err != nil || len(deploys) == 0 {
		return nil, err
	}
	return &deploys[0], nil
}

// GetDockerVersion get Docker Version
func GetDockerVersion() (*gv.Version, error) {
	// exec cmd to get output from docker info --format '{{.ServerVersion}}'
	cmd := exec.Command("docker", "info", "--format", "{{.ServerVersion}}")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil || stderr.String() != "" {
		errMsg := stderr.String()
		if errMsg == "" {
			errMsg = err.Error()
		}
		return nil, fmt.Errorf("failed to get the docker version by executing \"docker info --format {{.ServerVersion}}\": %s", errMsg)
	}
	return gv.NewVersion(strings.TrimSpace(string(out)))
}

func GetMajorMinorVersion(version string) string {
	vs := strings.Split(version, ".")
	if len(vs) < 2 {
		return ""
	}
	return vs[0] + vs[1]
}

// BuildSemverVersion build semver version which starts with "v", such as "v1.0.0".
func BuildSemverVersion(version string) string {
	if version == "" {
		return version
	}
	if !strings.HasPrefix(version, "v") {
		return "v" + version
	}
	return version
}

func ExistMultiKubeBlocks(client kubernetes.Interface) (bool, error) {
	kbVersions, err := GetKubeBlocksVersion(client, metav1.NamespaceAll)
	if err != nil {
		return false, err
	}
	return len(strings.Split(kbVersions, ",")) > 1, nil
}
