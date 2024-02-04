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
	"encoding/json"
	"fmt"
	"time"

	"github.com/hashicorp/go-version"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apitypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/kubectl/pkg/util/templates"

	"github.com/apecloud/kbcli/pkg/printer"
	"github.com/apecloud/kbcli/pkg/spinner"
	"github.com/apecloud/kbcli/pkg/types"
	"github.com/apecloud/kbcli/pkg/util"
	"github.com/apecloud/kbcli/pkg/util/breakingchange"
	"github.com/apecloud/kbcli/pkg/util/helm"
	"github.com/apecloud/kbcli/pkg/util/prompt"
)

var (
	upgradeExample = templates.Examples(`
	# Upgrade KubeBlocks to specified version
	kbcli kubeblocks upgrade --version=0.4.0

	# Upgrade KubeBlocks other settings, for example, set replicaCount to 3
	kbcli kubeblocks upgrade --set replicaCount=3`)
)

type getDeploymentFunc func(client kubernetes.Interface) (*appsv1.Deployment, error)

func newUpgradeCmd(f cmdutil.Factory, streams genericiooptions.IOStreams) *cobra.Command {
	o := &InstallOptions{
		Options: Options{
			IOStreams: streams,
		},
	}

	cmd := &cobra.Command{
		Use:     "upgrade",
		Short:   "Upgrade KubeBlocks.",
		Args:    cobra.NoArgs,
		Example: upgradeExample,
		Run: func(cmd *cobra.Command, args []string) {
			util.CheckErr(o.Complete(f, cmd))
			util.CheckErr(o.Upgrade())
		},
	}

	cmd.Flags().StringVar(&o.Version, "version", "", "Set KubeBlocks version")
	cmd.Flags().BoolVar(&o.Check, "check", true, "Check kubernetes environment before upgrade")
	cmd.Flags().DurationVar(&o.Timeout, "timeout", 300*time.Second, "Time to wait for upgrading KubeBlocks, such as --timeout=10m")
	cmd.Flags().BoolVar(&o.Wait, "wait", true, "Wait for KubeBlocks to be ready. It will wait for a --timeout period")
	cmd.Flags().BoolVar(&o.autoApprove, "auto-approve", false, "Skip interactive approval before upgrading KubeBlocks")
	helm.AddValueOptionsFlags(cmd.Flags(), &o.ValueOpts)

	return cmd
}

func (o *InstallOptions) Upgrade() error {
	klog.V(1).Info("##### Start to upgrade KubeBlocks #####")
	if o.HelmCfg.Namespace() == "" {
		ns, err := util.GetKubeBlocksNamespace(o.Client)
		if err != nil || ns == "" {
			printer.Warning(o.Out, "Failed to find deployed KubeBlocks.\n\n")
			fmt.Fprint(o.Out, "Use \"kbcli kubeblocks install\" to install KubeBlocks.\n")
			fmt.Fprintf(o.Out, "Use \"kbcli kubeblocks status\" to get information in more details.\n")
			return nil
		}
		o.HelmCfg.SetNamespace(ns)
	}

	o.Version = util.TrimVersionPrefix(o.Version)
	// check flags already been set
	if o.Version == "" && helm.ValueOptsIsEmpty(&o.ValueOpts) {
		fmt.Fprint(o.Out, "Nothing to upgrade, --set, --version should be specified.\n")
		return nil
	}

	// check if KubeBlocks has been installed
	v, err := util.GetVersionInfo(o.Client)
	if err != nil {
		return err
	}

	kbVersion := v.KubeBlocks
	if kbVersion == "" {
		return errors.New("KubeBlocks does not exist, try to run \"kbcli kubeblocks install\" to install")
	}

	if kbVersion == o.Version && helm.ValueOptsIsEmpty(&o.ValueOpts) {
		fmt.Fprintf(o.Out, "Current version %s is same with the upgraded version, no need to upgrade.\n", o.Version)
		return nil
	}
	fmt.Fprintf(o.Out, "Current KubeBlocks version %s.\n", v.KubeBlocks)

	// check installing version exists
	if err = o.checkVersion(v); err != nil {
		return err
	}
	o.OldVersion = kbVersion
	// double check when KubeBlocks version change
	if !o.autoApprove && o.Version != "" {
		oldVersion, err := version.NewVersion(kbVersion)
		if err != nil {
			return err
		}
		newVersion, err := version.NewVersion(o.Version)
		if err != nil {
			return err
		}
		upgradeWarn := ""
		switch {
		case oldVersion.GreaterThan(newVersion):
			upgradeWarn = printer.BoldYellow(fmt.Sprintf("Warning: You're attempting to downgrade KubeBlocks version from %s to %s, this action may cause your clusters and some KubeBlocks feature unavailable.\nEnsure you proceed after reviewing detailed release notes at https://github.com/apecloud/kubeblocks/releases.", kbVersion, o.Version))
		default:
			if err = breakingchange.ValidateUpgradeVersion(kbVersion, o.Version); err != nil {
				return err
			}
			upgradeWarn = fmt.Sprintf("Upgrade KubeBlocks from %s to %s", kbVersion, o.Version)
		}
		if err = prompt.Confirm(nil, o.In, upgradeWarn, "Please type 'Yes/yes' to confirm your operation:"); err != nil {
			return err
		}
	}

	// add helm repo
	s := spinner.New(o.Out, spinnerMsg("Add and update repo "+types.KubeBlocksChartName))
	defer s.Fail()
	// Add repo, if exists, will update it
	if err = helm.AddRepo(newHelmRepoEntry()); err != nil {
		return err
	}
	s.Success()

	// it's time to upgrade
	msg := ""
	if o.Version != "" {
		// keep addons before upgrade KubeBlocks avoid the addons been deleted
		s = spinner.New(o.Out, spinnerMsg("Keep addons"))
		defer s.Fail()
		if err = o.keepAddons(); err != nil {
			return err
		}
		s.Success()

		// stop the old version KubeBlocks, otherwise the old version KubeBlocks will reconcile the
		// new version resources, which may be not compatible. helm will start the new version
		// KubeBlocks after upgrade.
		s = spinner.New(o.Out, spinnerMsg("Stop KubeBlocks "+kbVersion))
		defer s.Fail()
		if err = o.stopDeployment(util.GetKubeBlocksDeploy); err != nil {
			return err
		}
		s.Success()

		// stop the data protection deployment
		s = spinner.New(o.Out, spinnerMsg("Stop DataProtection"))
		defer s.Fail()
		if err = o.stopDeployment(util.GetDataProtectionDeploy); err != nil {
			return err
		}
		s.Success()

		msg = "to " + o.Version
	}

	// save resources of old version before upgrade the crds
	o.upgrader = breakingchange.Upgrader{
		FromVersion: o.OldVersion,
		ToVersion:   o.Version,
		Dynamic:     o.Dynamic,
	}
	if err = o.upgrader.SaveOldResources(); err != nil {
		return err
	}

	// create or update crds
	s = spinner.New(o.Out, spinnerMsg("Upgrade CRDs"))
	defer s.Fail()
	if err = createOrUpdateCRDS(o.Dynamic, o.Version); err != nil {
		return fmt.Errorf("upgrade crds failed: %s", err.Error())
	}
	s.Success()

	s = spinner.New(o.Out, spinnerMsg("Upgrading KubeBlocks "+msg))
	defer s.Fail()
	// upgrade KubeBlocks chart
	if err = o.upgradeChart(); err != nil {
		return err
	}
	// successfully upgraded
	s.Success()

	if !o.Quiet {
		fmt.Fprintf(o.Out, "\nKubeBlocks has been upgraded %s SUCCESSFULLY!\n", msg)
		o.printNotes()
	}
	return nil
}

func (o *InstallOptions) upgradeChart() error {
	return o.buildChart().Upgrade(o.HelmCfg)
}

// stopDeployment stops the deployment by setting the replicas to 0
func (o *InstallOptions) stopDeployment(getDeployFn getDeploymentFunc) error {
	deploy, err := getDeployFn(o.Client)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	}

	if deploy == nil {
		klog.V(1).Info("deployment is not found, no need to stop")
		return nil
	}

	if _, err = o.Client.AppsV1().Deployments(deploy.Namespace).Patch(
		context.TODO(), deploy.Name, apitypes.JSONPatchType,
		[]byte(`[{"op": "replace", "path": "/spec/replicas", "value": 0}]`),
		metav1.PatchOptions{}); err != nil {
		return err
	}

	// wait for deployment to be stopped
	return wait.PollUntilContextTimeout(context.Background(), 5*time.Second, o.Timeout, true,
		func(_ context.Context) (bool, error) {
			deploy, err = util.GetKubeBlocksDeploy(o.Client)
			if err != nil {
				return false, err
			}
			if *deploy.Spec.Replicas == 0 &&
				deploy.Status.Replicas == 0 &&
				deploy.Status.AvailableReplicas == 0 {
				return true, nil
			}
			return false, nil
		})
}

// keepAddons set the addons to keep when upgrade KubeBlocks avoid the addons been deleted
func (o *InstallOptions) keepAddons() error {
	const (
		helmResourcePolicyKey  = "helm.sh/resource-policy"
		helmResourcePolicyKeep = "keep"
	)

	klog.V(1).Info("start to keep addons")
	addons, err := o.Dynamic.Resource(types.AddonGVR()).List(context.Background(), metav1.ListOptions{
		LabelSelector: buildKubeBlocksSelectorLabels(),
	})
	if err != nil && !apierrors.IsNotFound(err) {
		return err
	}

	if addons == nil || len(addons.Items) == 0 {
		return nil
	}

	for _, addon := range addons.Items {
		if addon.GetDeletionTimestamp() != nil {
			continue
		}
		annotations := addon.GetAnnotations()
		if annotations == nil {
			annotations = make(map[string]string)
		}
		if addon.GetAnnotations()[helmResourcePolicyKey] == helmResourcePolicyKeep {
			continue
		}
		klog.V(1).Info("keep addon: ", addon.GetName())
		annotations[helmResourcePolicyKey] = helmResourcePolicyKeep
		patchBytes, _ := json.Marshal(map[string]interface{}{"metadata": map[string]interface{}{"annotations": annotations}})
		if _, err = o.Dynamic.Resource(types.AddonGVR()).Namespace(addon.GetNamespace()).Patch(context.Background(),
			addon.GetName(), apitypes.MergePatchType, patchBytes, metav1.PatchOptions{}); err != nil {
			return err
		}
	}
	return nil
}
