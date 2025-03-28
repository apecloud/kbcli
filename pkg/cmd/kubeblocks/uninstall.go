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
	"bytes"
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	intctrlutil "github.com/apecloud/kubeblocks/pkg/controllerutil"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"golang.org/x/exp/maps"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	k8sapitypes "k8s.io/apimachinery/pkg/types"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	"k8s.io/client-go/dynamic"
	"k8s.io/klog/v2"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/kubectl/pkg/util/templates"

	extensionsv1alpha1 "github.com/apecloud/kubeblocks/apis/extensions/v1alpha1"

	"github.com/apecloud/kbcli/pkg/printer"
	"github.com/apecloud/kbcli/pkg/spinner"
	"github.com/apecloud/kbcli/pkg/types"
	"github.com/apecloud/kbcli/pkg/util"
	"github.com/apecloud/kbcli/pkg/util/helm"
)

var (
	uninstallExample = templates.Examples(`
		# uninstall KubeBlocks
        kbcli kubeblocks uninstall`)
)

type UninstallOptions struct {
	Factory cmdutil.Factory
	Options

	// AutoApprove if true, skip interactive approval
	AutoApprove     bool
	removePVs       bool
	removePVCs      bool
	RemoveNamespace bool
	addons          []*extensionsv1alpha1.Addon
	Quiet           bool
	force           bool
	existMultiKB    bool
}

func newUninstallCmd(f cmdutil.Factory, streams genericiooptions.IOStreams) *cobra.Command {
	o := &UninstallOptions{
		Options: Options{
			IOStreams: streams,
		},
		Factory: f,
		force:   true,
	}
	cmd := &cobra.Command{
		Use:     "uninstall",
		Short:   "Uninstall KubeBlocks.",
		Args:    cobra.NoArgs,
		Example: uninstallExample,
		Run: func(cmd *cobra.Command, args []string) {
			util.CheckErr(o.Complete(f, cmd))
			util.CheckErr(o.PreCheck())
			util.CheckErr(o.Uninstall())
		},
	}
	cmd.Flags().StringVarP(&o.Namespace, "namespace", "n", "", "KubeBlocks namespace")
	cmd.Flags().BoolVar(&o.AutoApprove, "auto-approve", false, "Skip interactive approval before uninstalling KubeBlocks")
	cmd.Flags().BoolVar(&o.removePVs, "remove-pvs", false, "Remove PersistentVolume or not")
	cmd.Flags().BoolVar(&o.removePVCs, "remove-pvcs", false, "Remove PersistentVolumeClaim or not")
	cmd.Flags().BoolVar(&o.RemoveNamespace, "remove-namespace", false, "Remove default created \"kb-system\" namespace or not")
	cmd.Flags().DurationVar(&o.Timeout, "timeout", 600*time.Second, "Time to wait for uninstalling KubeBlocks, such as --timeout=5m")
	cmd.Flags().BoolVar(&o.Wait, "wait", true, "Wait for KubeBlocks to be uninstalled, including all the add-ons. It will wait for a --timeout period")
	return cmd
}

func (o *UninstallOptions) PreCheck() error {
	// wait user to confirm
	if !o.AutoApprove {
		printer.Warning(o.Out, "this action will remove all KubeBlocks resources.\n")
		if err := confirmUninstall(o.In); err != nil {
			return err
		}
	}
	var err error
	o.existMultiKB, err = util.ExistMultiKubeBlocks(o.Client)
	if err != nil {
		return err
	}
	if o.existMultiKB {
		if o.Namespace != "" {
			version, err := util.GetKubeBlocksVersion(o.Client, o.Namespace)
			if err != nil {
				return err
			}
			if strings.HasPrefix(version, "1") {
				return errors.Errorf("only can uninstall KubeBlocks 0.9 when existing multi KubeBlocks")
			}
		}
	} else {
		// check if there is any resource should be removed first when only one KB installed, if so, return error
		// and ask user to remove them manually
		if err = checkResources(o.Dynamic); err != nil {
			return err
		}
	}

	// verify where kubeblocks is installed
	kbNamespace, err := util.GetKubeBlocksNamespace(o.Client, o.Namespace)
	if err != nil {
		printer.Warning(o.Out, "failed to locate KubeBlocks meta, will clean up all KubeBlocks resources.\n")
		if !o.Quiet {
			fmt.Fprintf(o.Out, "to find out the namespace where KubeBlocks is installed, please use:\n\t'kbcli kubeblocks status'\n")
			fmt.Fprintf(o.Out, "to uninstall KubeBlocks completely, please use:\n\t`kbcli kubeblocks uninstall -n <namespace>`\n")
		}
		if intctrlutil.IsTargetError(err, intctrlutil.ErrorTypeFatal) {
			return err
		}
	}

	o.Namespace = kbNamespace
	if kbNamespace != "" {
		fmt.Fprintf(o.Out, "Uninstall KubeBlocks in namespace \"%s\"\n", kbNamespace)
	}

	return nil
}

func (o *UninstallOptions) Uninstall() error {
	printSpinner := func(s spinner.Interface, err error) {
		if err == nil || apierrors.IsNotFound(err) ||
			strings.Contains(err.Error(), "release: not found") {
			s.Success()
			return
		}
		s.Fail()
		fmt.Fprintf(o.Out, "  %s\n", err.Error())
	}
	newSpinner := func(msg string) spinner.Interface {
		return spinner.New(o.Out, spinner.WithMessage(fmt.Sprintf("%-50s", msg)))
	}

	// uninstall helm release that will delete custom resources, but since finalizers is not empty,
	// custom resources will not be deleted, so we will remove finalizers later.
	v, _ := util.GetVersionInfo(o.Client)
	chart := helm.InstallOpts{
		Name:           types.KubeBlocksChartName,
		Namespace:      o.Namespace,
		ForceUninstall: o.force,
		// KubeBlocks chart has a hook to delete addons, but we have already deleted addons,
		// and that webhook may fail, so we need to disable hooks.
		DisableHooks: true,
	}
	if o.existMultiKB {
		// TODO: remove the 0.9 addons.
		// remove KB release
		printSpinner(newSpinner("Uninstall helm release "+types.KubeBlocksReleaseName+" 0.9"),
			chart.Uninstall(o.HelmCfg))

	} else {
		// uninstall all KubeBlocks addons
		if err := o.uninstallAddons(); err != nil {
			fmt.Fprintf(o.Out, "Failed to uninstall addons, run \"kbcli kubeblocks uninstall\" to retry.\n")
			return err
		}

		// get KubeBlocks objects, then try to remove them
		objs, err := getKBObjects(o.Dynamic, o.Namespace, o.addons)
		if err != nil {
			fmt.Fprintf(o.ErrOut, "Failed to get KubeBlocks objects %s", err.Error())
		}

		// remove finalizers of custom resources, then that will be deleted
		printSpinner(newSpinner("Remove built-in custom resources"), removeCustomResources(o.Dynamic, objs))

		var gvrs []schema.GroupVersionResource
		for k := range objs {
			gvrs = append(gvrs, k)
		}
		sort.SliceStable(gvrs, func(i, j int) bool {
			g1 := gvrs[i]
			g2 := gvrs[j]
			return strings.Compare(g1.Resource, g2.Resource) < 0
		})

		for _, gvr := range gvrs {
			if gvr == types.PVCGVR() && !o.removePVCs {
				continue
			}
			if gvr == types.PVGVR() && !o.removePVs {
				continue
			}
			if v, ok := objs[gvr]; !ok || len(v.Items) == 0 {
				continue
			}
			printSpinner(newSpinner("Remove "+gvr.Resource), deleteObjects(o.Dynamic, gvr, objs[gvr]))
		}

		// remove KB release
		printSpinner(newSpinner("Uninstall helm release "+types.KubeBlocksReleaseName+" "+v.KubeBlocks),
			chart.Uninstall(o.HelmCfg))
	}

	// delete namespace if it is default namespace
	if o.Namespace == types.DefaultNamespace && o.RemoveNamespace {
		printSpinner(newSpinner("Remove namespace "+types.DefaultNamespace),
			deleteNamespace(o.Client, types.DefaultNamespace))
	}

	if o.Wait {
		fmt.Fprintln(o.Out, "Uninstall KubeBlocks done.")
	} else {
		fmt.Fprintf(o.Out, "KubeBlocks is uninstalling, run \"kbcli kubeblocks status -A\" to check kubeblocks resources.\n")
	}
	return nil
}

// uninstallAddons uninstalls all KubeBlocks addons
func (o *UninstallOptions) uninstallAddons() error {
	var (
		allErrs []error
		err     error
		header  = "Wait for addons to be disabled"
		s       spinner.Interface
		msg     string
	)

	addons := make(map[string]*extensionsv1alpha1.Addon)
	processAddons := func(uninstall bool) error {
		objects, err := o.Dynamic.Resource(types.AddonGVR()).List(context.TODO(), metav1.ListOptions{})
		if err != nil && !apierrors.IsNotFound(err) {
			klog.V(1).Infof("Failed to get KubeBlocks addons %s", err.Error())
			allErrs = append(allErrs, err)
			return utilerrors.NewAggregate(allErrs)
		}
		if objects == nil {
			return nil
		}

		for _, obj := range objects.Items {
			addon := &extensionsv1alpha1.Addon{}
			if err = runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, addon); err != nil {
				klog.V(1).Infof("Failed to convert KubeBlocks addon %s", err.Error())
				allErrs = append(allErrs, err)
				continue
			}

			if uninstall {
				// we only need to uninstall addons that are not disabled
				if addon.Spec.InstallSpec.IsDisabled() {
					continue
				}
				addons[addon.Name] = addon
				o.addons = append(o.addons, addon)

				// uninstall addons
				if err = disableAddon(o.Dynamic, addon); err != nil {
					klog.V(1).Infof("Failed to uninstall KubeBlocks addon %s %s", addon.Name, err.Error())
					allErrs = append(allErrs, err)
				}
			} else {
				// update cached addon if exists
				if _, ok := addons[addon.Name]; ok {
					addons[addon.Name] = addon
				}
			}
		}
		return utilerrors.NewAggregate(allErrs)
	}

	suffixMsg := func(msg string) string {
		return fmt.Sprintf("%-50s", msg)
	}

	if !o.Wait {
		s = spinner.New(o.Out, spinner.WithMessage(fmt.Sprintf("%-50s", "Uninstall KubeBlocks addons")))
	} else {
		s = spinner.New(o.Out, spinner.WithMessage(fmt.Sprintf("%-50s", header)))
	}

	// get all addons and uninstall them
	if err = processAddons(true); err != nil {
		s.Fail()
		return err
	}

	if len(addons) == 0 || !o.Wait {
		s.Success()
		return nil
	}

	spinnerDone := func() {
		s.SetFinalMsg(msg)
		s.Done("")
		fmt.Fprintln(o.Out)
	}

	// check if all addons are disabled, if so, then we will stop checking addons
	// status otherwise, we will wait for a while and check again
	if err = wait.PollUntilContextTimeout(context.Background(), 5*time.Second,
		o.Timeout, true, func(_ context.Context) (bool, error) {
			// we only check addons status, do not try to uninstall addons again
			if err = processAddons(false); err != nil {
				return false, err
			}
			status := checkAddons(maps.Values(addons), false)
			msg = suffixMsg(fmt.Sprintf("%s\n  %s", header, status.outputMsg))
			s.SetMessage(msg)
			if status.allDisabled {
				spinnerDone()
				return true, nil
			} else if status.hasFailed {
				return false, errors.New("some addons are failed to disabled")
			}
			return false, nil
		}); err != nil {
		spinnerDone()
		printAddonMsg(o.Out, maps.Values(addons), false)
		allErrs = append(allErrs, err)
	}
	return utilerrors.NewAggregate(allErrs)
}

func checkResources(dynamic dynamic.Interface) error {
	ctx := context.Background()
	gvrList := []schema.GroupVersionResource{
		types.ClusterGVR(),
		types.BackupGVR(),
	}

	crs := map[string][]string{}
	for _, gvr := range gvrList {
		objList, err := dynamic.Resource(gvr).List(ctx, metav1.ListOptions{})
		if err != nil && !apierrors.IsNotFound(err) {
			return err
		}

		if objList == nil {
			continue
		}

		for _, item := range objList.Items {
			crs[gvr.Resource] = append(crs[gvr.Resource], item.GetName())
		}
	}

	if len(crs) > 0 {
		errMsg := bytes.NewBufferString("failed to uninstall, the following resources need to be removed first\n")
		for k, v := range crs {
			errMsg.WriteString(fmt.Sprintf("  %s: %s\n", k, strings.Join(v, " ")))
		}
		return errors.Errorf(errMsg.String())
	}
	return nil
}

func disableAddon(dynamic dynamic.Interface, addon *extensionsv1alpha1.Addon) error {
	klog.V(1).Infof("Uninstall %s, status %s", addon.Name, addon.Status.Phase)
	if _, err := dynamic.Resource(types.AddonGVR()).Patch(context.TODO(), addon.Name, k8sapitypes.JSONPatchType,
		[]byte("[{\"op\": \"replace\", \"path\": \"/spec/install/enabled\", \"value\": false }]"),
		metav1.PatchOptions{}); err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	return nil
}
