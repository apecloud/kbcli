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

package kubeblocks

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/apecloud/kubeblocks/pkg/constant"
	"golang.org/x/mod/semver"
	"helm.sh/helm/v3/pkg/cli/values"
	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/apecloud/kbcli/pkg/spinner"
	"github.com/apecloud/kbcli/pkg/types"
	"github.com/apecloud/kbcli/pkg/util"
)

const (
	// TODO: tidy up the versions when 0.9.2 released
	KBVersion092 = "v0.9.2-beta.16"
	KBVersion100 = "v1.0.0-beta.1"
)

func (o *InstallOptions) checkUpgradeFrom09(kbVersions string) bool {
	installedVersions := strings.Split(kbVersions, ",")
	if len(installedVersions) == 0 || len(installedVersions) > 1 {
		// not repeated installation
		return false
	}
	installVersion := util.BuildSemverVersion(o.Version)
	if semver.Compare(installVersion, KBVersion100) == -1 {
		// install version < 1.0.0
		return false
	}
	for _, v := range installedVersions {
		installedVersion := util.BuildSemverVersion(v)
		if semver.Major(installedVersion) != "v0" {
			// not 0.x.x
			return false
		}
		if semver.Compare(installedVersion, KBVersion092) == -1 {
			// 0.x.x < 0.9.2
			return false
		}
	}
	return true
}

func (o *InstallOptions) preInstallWhenUpgradeFrom09() error {
	// enable webhooks.conversionEnabled
	o.ValueOpts.Values = append(o.ValueOpts.Values, "webhooks.conversionEnabled=true")
	stopDeployments := func(getDeploys func(client kubernetes.Interface) ([]appsv1.Deployment, error), msgKey string) error {
		kbDeploys, err := getDeploys(o.Client)
		if err != nil {
			return err
		}
		for i := range kbDeploys {
			deploy := &kbDeploys[i]
			kbVersion := deploy.Labels[constant.AppVersionLabelKey]
			o.kb09Namespace = deploy.Namespace
			s := spinner.New(o.Out, spinnerMsg(fmt.Sprintf("Stop %s %s", msgKey, kbVersion)))
			if err = o.stopDeploymentObject(s, deploy); err != nil {
				return err
			}
		}
		return nil
	}

	// 1. update 092 chart values
	if err := o.configKB09(); err != nil {
		return err
	}
	// 2. stop 0.9 KubeBlocks
	if err := stopDeployments(util.GetKubeBlocksDeploys, "KubeBlocks"); err != nil {
		return err
	}
	// 3. stop 0.9 DataProtection
	if err := stopDeployments(util.GetDataProtectionDeploys, "DataProtection"); err != nil {
		return err
	}
	// 4. Set global resources helm owner to 1.0 KB
	if err := o.setGlobalResourcesHelmOwner(); err != nil {
		return err
	}
	return o.setCRDAPIVersion()
}

func (o *InstallOptions) configKB09() error {
	helmConfig := *o.HelmCfg
	helmConfig.SetNamespace(o.kb09Namespace)
	configOpt := &InstallOptions{
		Options: Options{
			IOStreams: o.IOStreams,
			Wait:      true,
			Dynamic:   o.Dynamic,
			Client:    o.Client,
			HelmCfg:   &helmConfig,
		},
		ValueOpts: values.Options{
			Values: []string{
				"dualOperatorsMode=true",
				"keepAddons=true",
				"keepGlobalResources=true",
			},
		},
	}
	return configKBRelease(configOpt)
}

func (o *InstallOptions) stopDeploymentObject(s spinner.Interface, deploy *appsv1.Deployment) error {
	defer s.Fail()
	ctx := context.TODO()
	if deploy.Spec.Replicas != nil && *deploy.Spec.Replicas != 0 {
		patch := []byte(fmt.Sprintf(`{"spec":{"replicas":%d}}`, 0))
		_, err := o.Client.AppsV1().Deployments(deploy.Namespace).Patch(
			ctx,
			deploy.Name,
			k8stypes.StrategicMergePatchType,
			patch,
			metav1.PatchOptions{},
		)
		if err != nil {
			return fmt.Errorf("failed to patch deployment %s/%s: %v",
				deploy.Namespace, deploy.Name, err)
		}
	}
	// wait for deployment to be stopped
	if err := wait.PollUntilContextTimeout(context.Background(), 5*time.Second, 1*time.Minute, true,
		func(_ context.Context) (bool, error) {
			deployment, err := o.Client.AppsV1().Deployments(deploy.Namespace).Get(ctx, deploy.Name, metav1.GetOptions{})
			if err != nil {
				return false, err
			}
			if deployment.Status.Replicas == 0 && deployment.Status.AvailableReplicas == 0 {
				return true, nil
			}
			return false, err
		}); err != nil {
		return err
	}
	s.Success()
	return nil
}

func (o *InstallOptions) setGlobalResourcesHelmOwner() error {
	fmt.Fprintf(o.Out, "Change the release owner for the global resources\n")

	// update ClusterRoles
	if err := util.SetHelmOwner(o.Dynamic, types.ClusterRoleGVR(), types.KubeBlocksChartName, o.HelmCfg.Namespace(), []string{
		"kubeblocks-cluster-pod-role",
		types.KubeBlocksChartName,
		fmt.Sprintf("%s-cluster-editor-role", types.KubeBlocksChartName),
		fmt.Sprintf("%s-clusterdefinition-editor-role", types.KubeBlocksChartName),
		fmt.Sprintf("%s-configconstraint-editor-role", types.KubeBlocksChartName),
		fmt.Sprintf("%s-metrics-reader", types.KubeBlocksChartName),
		fmt.Sprintf("%s-proxy-role", types.KubeBlocksChartName),
		fmt.Sprintf("%s-patroni-pod-role", types.KubeBlocksChartName),
		fmt.Sprintf("%s-manager-role", types.KubeBlocksChartName),
		fmt.Sprintf("%s-dataprotection-worker-role", types.KubeBlocksChartName),
		fmt.Sprintf("%s-helmhook-role", types.KubeBlocksChartName),
		fmt.Sprintf("%s-backup-editor-role", types.KubeBlocksChartName),
		fmt.Sprintf("%s-backuppolicy-editor-role", types.KubeBlocksChartName),
		fmt.Sprintf("%s-dataprotection-exec-worker-role", types.KubeBlocksChartName),
		fmt.Sprintf("%s-restore-editor-role", types.KubeBlocksChartName),
		fmt.Sprintf("%s-nodecountscaler-editor-role", types.KubeBlocksChartName),
		fmt.Sprintf("%s-editor-role", types.KubeBlocksChartName),
		fmt.Sprintf("%s-leader-election-role", types.KubeBlocksChartName),
		fmt.Sprintf("%s-rbac-manager-role", types.KubeBlocksChartName),
		fmt.Sprintf("%s-instanceset-editor-role", types.KubeBlocksChartName),
	}); err != nil {
		return err
	}
	// update Addons
	if err := util.SetHelmOwner(o.Dynamic, types.AddonGVR(), types.KubeBlocksChartName, o.HelmCfg.Namespace(), []string{
		"apecloud-mysql", "etcd", "kafka", "llm",
		"mongodb", "mysql", "postgresql", "pulsar",
		"qdrant", "redis", "alertmanager-webhook-adaptor",
		"aws-load-balancer-controller", "csi-driver-nfs",
		"csi-hostpath-driver", "grafana", "prometheus",
		"snapshot-controller", "victoria-metrics-agent",
	}); err != nil {
		return err
	}
	// update StorageProviders
	if err := util.SetHelmOwner(o.Dynamic, types.StorageProviderGVR(), types.KubeBlocksChartName, o.HelmCfg.Namespace(), []string{
		"cos", "ftp", "gcs-s3comp", "minio", "nfs",
		"obs", "oss", "pvc", "s3",
	}); err != nil {
		return err
	}
	_, err := o.Dynamic.Resource(types.StorageClassGVR()).Namespace("").Get(context.TODO(), "kb-default-sc", metav1.GetOptions{})
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return err
		}
	} else {
		if err = util.SetHelmOwner(o.Dynamic, types.StorageClassGVR(), types.KubeBlocksChartName, o.HelmCfg.Namespace(), []string{
			"kb-default-sc",
		}); err != nil {
			return err
		}
	}
	// update BackupRepo
	return util.SetHelmOwner(o.Dynamic, types.BackupRepoGVR(), types.KubeBlocksChartName, o.HelmCfg.Namespace(), []string{fmt.Sprintf("%s-backuprepo", types.KubeBlocksChartName)})
}

func (o *InstallOptions) setCRDAPIVersion() error {
	setCRDAPIVersionAnnotation := func(list *unstructured.UnstructuredList, version string) error {
		patchOP := fmt.Sprintf(`[{"op": "replace", "path": "/metadata/annotations/kubeblocks.io~1crd-api-version", "value": "apps.kubeblocks.io/%s"}]`, version)
		for _, v := range list.Items {
			if v.GetLabels()[constant.CRDAPIVersionAnnotationKey] != "" {
				continue
			}
			if _, err := o.Dynamic.Resource(types.CompDefAlpha1GVR()).Namespace("").Patch(context.TODO(), v.GetName(),
				k8stypes.JSONPatchType, []byte(patchOP), metav1.PatchOptions{}); client.IgnoreNotFound(err) != nil {
				return err
			}
		}
		return nil
	}
	compDefs, err := o.Dynamic.Resource(types.CompDefAlpha1GVR()).Namespace("").List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return err
	}
	if err = setCRDAPIVersionAnnotation(compDefs, types.AppsAPIVersion); err != nil {
		return err
	}
	return nil
}
