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

package kubeblocks

import (
	"context"
	"os"
	"testing"

	"github.com/apecloud/kubeblocks/pkg/constant"
	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	dyFake "k8s.io/client-go/dynamic/fake"
	kubeFake "k8s.io/client-go/kubernetes/fake"
	testclient "k8s.io/client-go/testing"
	"k8s.io/utils/pointer"

	"github.com/apecloud/kbcli/pkg/spinner"
	"github.com/apecloud/kbcli/pkg/types"
	"github.com/apecloud/kbcli/pkg/util"
	"github.com/apecloud/kbcli/pkg/util/helm"
)

const (
	kb09Version        = "0.9.2"
	testKubeConfigPath = "./testdata/kubeconfig"
)

func TestCheckUpgradeFrom09(t *testing.T) {
	tests := []struct {
		name       string
		versions   string
		installVer string
		want       bool
	}{
		{
			name:       "single 0.9.2 version to 1.0.0",
			versions:   "v0.9.2",
			installVer: "v1.0.0",
			want:       true,
		},
		{
			name:       "multiple versions",
			versions:   "v0.9.2,v0.9.3",
			installVer: "v1.0.0",
			want:       false,
		},
		{
			name:       "version < 0.9.2",
			versions:   "v0.9.1",
			installVer: "v1.0.0",
			want:       false,
		},
		{
			name:       "install version < 1.0.0",
			versions:   "v0.9.2",
			installVer: "v0.9.3",
			want:       false,
		},
		{
			name:       "install version 1.0.2",
			versions:   "v1.0.1",
			installVer: "v1.0.2",
			want:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o := &InstallOptions{
				Version: tt.installVer,
			}
			got := o.checkUpgradeFrom09(tt.versions)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestStopDeployment(t *testing.T) {
	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-deploy",
			Namespace: namespace,
			Labels: map[string]string{
				constant.AppVersionLabelKey: kb09Version,
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: pointer.Int32(1),
		},
	}

	client := kubeFake.NewSimpleClientset(deploy)
	o := &InstallOptions{
		Options: Options{
			Client:    client,
			IOStreams: genericiooptions.IOStreams{In: os.Stdin, Out: os.Stdout, ErrOut: os.Stderr},
		},
	}

	s := spinner.New(o.Out, spinnerMsg("Stop KubeBlocks Deployment"))
	err := o.stopDeployment(s, deploy)
	assert.NoError(t, err)

	updatedDeploy, err := client.AppsV1().Deployments(namespace).Get(context.TODO(), "test-deploy", metav1.GetOptions{})
	assert.NoError(t, err)
	assert.Equal(t, int32(0), *updatedDeploy.Spec.Replicas)
}

func TestSetGlobalResourcesHelmOwner(t *testing.T) {
	scheme := runtime.NewScheme()
	dynamicClient := dyFake.NewSimpleDynamicClient(scheme)

	o := &InstallOptions{
		Options: Options{
			Dynamic:   dynamicClient,
			Namespace: namespace,
			IOStreams: genericiooptions.IOStreams{In: os.Stdin, Out: os.Stdout, ErrOut: os.Stderr},
			HelmCfg:   helm.NewFakeConfig(namespace),
		},
	}

	var patchedResources []string
	dynamicClient.PrependReactor("patch", "*", func(action testclient.Action) (bool, runtime.Object, error) {
		patchAction := action.(testclient.PatchAction)
		patchedResources = append(patchedResources, patchAction.GetName())
		return true, nil, nil
	})

	err := o.setGlobalResourcesHelmOwner()
	assert.NoError(t, err)

	expectedResources := []string{
		"kubeblocks-cluster-pod-role",
		types.KubeBlocksChartName,
	}

	for _, expected := range expectedResources {
		found := false
		for _, patched := range patchedResources {
			if patched == expected {
				found = true
				break
			}
		}
		assert.True(t, found, "Expected resource %s to be patched", expected)
	}
}

func TestPreInstallWhenUpgradeFrom09(t *testing.T) {

	deploy1 := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      types.KubeBlocksChartName,
			Namespace: namespace,
			Labels: map[string]string{
				constant.AppVersionLabelKey:   kb09Version,
				constant.AppComponentLabelKey: util.KubeblocksAppComponent,
				constant.AppNameLabelKey:      types.KubeBlocksChartName,
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: pointer.Int32(1),
		},
	}

	deploy2 := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "dataprotection",
			Namespace: namespace,
			Labels: map[string]string{
				constant.AppVersionLabelKey:   kb09Version,
				constant.AppComponentLabelKey: util.DataprotectionAppComponent,
				constant.AppNameLabelKey:      types.KubeBlocksChartName,
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: pointer.Int32(1),
		},
	}
	helmSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kubeblocks-helm-secret",
			Namespace: namespace,
			Labels: map[string]string{
				"name":  types.KubeBlocksChartName,
				"owner": "helm",
			},
		},
	}
	client := kubeFake.NewSimpleClientset(deploy1, deploy2, helmSecret)
	scheme := runtime.NewScheme()
	dynamicClient := dyFake.NewSimpleDynamicClient(scheme)

	o := &InstallOptions{
		Options: Options{
			Client:    client,
			Dynamic:   dynamicClient,
			Namespace: namespace,
			IOStreams: genericiooptions.IOStreams{In: os.Stdin, Out: os.Stdout, ErrOut: os.Stderr},
			HelmCfg:   helm.NewFakeConfig(namespace),
		},
	}

	err := o.preInstallWhenUpgradeFrom09()
	assert.NotEqual(t, err, nil)
}
