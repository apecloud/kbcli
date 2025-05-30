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
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/replicatedhq/troubleshoot/pkg/preflight"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"golang.org/x/exp/maps"
	"helm.sh/helm/v3/pkg/cli/values"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/kubectl/pkg/util/templates"

	extensionsv1alpha1 "github.com/apecloud/kubeblocks/apis/extensions/v1alpha1"
	"github.com/apecloud/kubeblocks/pkg/viperx"

	"github.com/apecloud/kbcli/pkg/spinner"
	"github.com/apecloud/kbcli/pkg/types"
	"github.com/apecloud/kbcli/pkg/util"
	"github.com/apecloud/kbcli/pkg/util/helm"
	"github.com/apecloud/kbcli/pkg/util/prompt"
	"github.com/apecloud/kbcli/version"
)

const (
	kNodeAffinity                     = "affinity.nodeAffinity=%s"
	kPodAntiAffinity                  = "affinity.podAntiAffinity=%s"
	kTolerations                      = "tolerations=%s"
	defaultTolerationsForInstallation = "kb-controller=true:NoSchedule"
)

type Options struct {
	genericiooptions.IOStreams

	HelmCfg *helm.Config

	// Namespace is the current namespace the command running in
	Namespace  string
	Client     kubernetes.Interface
	Dynamic    dynamic.Interface
	Timeout    time.Duration
	Wait       bool
	WaitAddons bool
}

type InstallOptions struct {
	Options
	OldVersion      string
	Version         string
	Quiet           bool
	CreateNamespace bool
	Check           bool
	// autoApprove for KubeBlocks upgrade
	autoApprove bool
	ValueOpts   values.Options

	// ConfiguredOptions is the options that kubeblocks
	PodAntiAffinity string
	TopologyKeys    []string
	NodeLabels      map[string]string
	TolerationsRaw  []string
	upgradeFrom09   bool
	kb09Namespace   string
}

type addonStatus struct {
	allEnabled  bool
	allDisabled bool
	hasFailed   bool
	outputMsg   string
}

var (
	installExample = templates.Examples(`
	# Install KubeBlocks, the default version is same with the kbcli version, the default namespace is kb-system
	kbcli kubeblocks install

	# Install KubeBlocks with specified version
	kbcli kubeblocks install --version=0.4.0

	# Install KubeBlocks with ignoring preflight checks
	kbcli kubeblocks install --force

	# Install KubeBlocks with specified namespace, if the namespace is not present, it will be created
	kbcli kubeblocks install --namespace=my-namespace --create-namespace

	# Install KubeBlocks with other settings, for example, set replicaCount to 3
	kbcli kubeblocks install --set replicaCount=3`)

	spinnerMsg = func(format string, a ...any) spinner.Option {
		return spinner.WithMessage(fmt.Sprintf("%-50s", fmt.Sprintf(format, a...)))
	}
)

func newInstallCmd(f cmdutil.Factory, streams genericiooptions.IOStreams) *cobra.Command {
	o := &InstallOptions{
		Options: Options{
			IOStreams: streams,
		},
	}

	p := &PreflightOptions{
		PreflightFlags: preflight.NewPreflightFlags(),
		IOStreams:      streams,
	}
	*p.Interactive = false
	*p.Format = "kbcli"

	cmd := &cobra.Command{
		Use:     "install",
		Short:   "Install KubeBlocks.",
		Args:    cobra.NoArgs,
		Example: installExample,
		Run: func(cmd *cobra.Command, args []string) {
			util.CheckErr(o.Complete(f, cmd))
			util.CheckErr(o.PreCheck())
			util.CheckErr(o.CompleteInstallOptions())
			util.CheckErr(p.Preflight(f, args, o.ValueOpts))
			util.CheckErr(o.Install())
		},
	}

	cmd.Flags().StringVar(&o.Version, "version", version.DefaultKubeBlocksVersion, "KubeBlocks version")
	cmd.Flags().StringVarP(&o.Namespace, "namespace", "n", types.DefaultNamespace, "KubeBlocks namespace")
	cmd.Flags().BoolVar(&o.CreateNamespace, "create-namespace", false, "Create the namespace if not present")
	cmd.Flags().BoolVar(&o.Check, "check", true, "Check kubernetes environment before installation")
	cmd.Flags().DurationVar(&o.Timeout, "timeout", 1800*time.Second, "Time to wait for installing KubeBlocks, such as --timeout=10m")
	cmd.Flags().BoolVar(&o.Wait, "wait", true, "Wait for KubeBlocks to be ready, including all the auto installed add-ons. It will wait for a --timeout period")
	cmd.Flags().BoolVar(&o.WaitAddons, "wait-addons", true, "Wait for auto installed add-ons. It will wait for a --timeout period")
	cmd.Flags().BoolVar(&p.force, flagForce, p.force, "If present, just print fail item and continue with the following steps")
	cmd.Flags().StringVar(&o.PodAntiAffinity, "pod-anti-affinity", "", "Pod anti-affinity type, one of: (Preferred, Required)")
	cmd.Flags().StringArrayVar(&o.TopologyKeys, "topology-keys", nil, "Topology keys for affinity")
	cmd.Flags().StringToStringVar(&o.NodeLabels, "node-labels", nil, "Node label selector")
	cmd.Flags().StringSliceVar(&o.TolerationsRaw, "tolerations", nil, `Tolerations for Kubeblocks, such as '"dev=true:NoSchedule,large=true:NoSchedule"'`)
	helm.AddValueOptionsFlags(cmd.Flags(), &o.ValueOpts)

	return cmd
}

func (o *Options) Complete(f cmdutil.Factory, cmd *cobra.Command) error {
	var err error

	// default write log to file
	if err = util.EnableLogToFile(cmd.Flags()); err != nil {
		fmt.Fprintf(o.Out, "Failed to enable the log file %s", err.Error())
	}

	config, err := cmd.Flags().GetString("kubeconfig")
	if err != nil {
		return err
	}

	ctx, err := cmd.Flags().GetString("context")
	if err != nil {
		return err
	}

	o.HelmCfg = helm.NewConfig(o.Namespace, config, ctx, klog.V(1).Enabled())
	if o.Dynamic, err = f.DynamicClient(); err != nil {
		return err
	}

	o.Client, err = f.KubernetesClientSet()
	return err
}

func (o *InstallOptions) PreCheck() error {
	// check whether the namespace exists
	if err := o.checkNamespace(); err != nil {
		return err
	}
	return o.PreCheckKBVersion()
}

func (o *InstallOptions) PreCheckKBVersion() error {
	o.Version = util.TrimVersionPrefix(o.Version)
	v, err := util.GetVersionInfo(o.Client)
	if err != nil {
		return err
	}
	o.upgradeFrom09 = o.checkUpgradeFrom09(v.KubeBlocks)
	if o.upgradeFrom09 {
		if o.upgradeFrom09 {
			deploys, err := util.GetKubeBlocksDeploys(o.Client)
			if err != nil {
				return err
			}
			for _, deploy := range deploys {
				if deploy.Namespace == o.HelmCfg.Namespace() {
					return fmt.Errorf(`cannot install KubeBlocks in the same namespace "%s" with KubeBlocks 0.9`, o.HelmCfg.Namespace())
				}
			}
		}
		installWarn := fmt.Sprintf("You will Install KubeBlocks %s When existing KubeBlocks %s ", o.Version, v.KubeBlocks)
		if err = prompt.Confirm(nil, o.In, installWarn, "Please type 'Yes/yes' to confirm your operation:"); err != nil {
			return err
		}
	} else {
		// check if KubeBlocks has been installed
		if v.KubeBlocks != "" {
			return fmt.Errorf("KubeBlocks %s already exists, repeated installation is not supported", v.KubeBlocks)
		}

		// check whether there are remained resource left by previous KubeBlocks installation, if yes,
		// output the resource name
		if err = o.checkRemainedResource(); err != nil {
			return err
		}
	}
	if err = o.checkVersion(v); err != nil {
		return err
	}
	return nil
}

// CompleteInstallOptions complete options for real installation of kubeblocks
func (o *InstallOptions) CompleteInstallOptions() error {
	// add pod anti-affinity
	if o.PodAntiAffinity != "" || len(o.TopologyKeys) > 0 {
		podAntiAffinityJSON, err := json.Marshal(util.BuildPodAntiAffinity(o.PodAntiAffinity, o.TopologyKeys))
		if err != nil {
			return err
		}
		o.ValueOpts.JSONValues = append(o.ValueOpts.JSONValues, fmt.Sprintf(kPodAntiAffinity, podAntiAffinityJSON))
	}

	// add node affinity
	if len(o.NodeLabels) > 0 {
		nodeLabelsJSON, err := json.Marshal(util.BuildNodeAffinity(o.NodeLabels))
		if err != nil {
			return err
		}
		o.ValueOpts.JSONValues = append(o.ValueOpts.JSONValues, fmt.Sprintf(kNodeAffinity, string(nodeLabelsJSON)))
	}

	// add tolerations
	// parse tolerations and add to values, the default tolerations are defined in var defaultTolerationsForInstallation
	o.TolerationsRaw = append(o.TolerationsRaw, defaultTolerationsForInstallation)
	tolerations, err := util.BuildTolerations(o.TolerationsRaw)
	if err != nil {
		return err
	}
	tolerationsJSON, err := json.Marshal(tolerations)
	if err != nil {
		return err
	}
	o.ValueOpts.JSONValues = append(o.ValueOpts.JSONValues, fmt.Sprintf(kTolerations, string(tolerationsJSON)))
	return nil
}

func (o *InstallOptions) Install() error {
	var err error
	if o.upgradeFrom09 {
		if err = o.preInstallWhenUpgradeFrom09(); err != nil {
			return err
		}
	}
	// create or update crds
	s := spinner.New(o.Out, spinnerMsg("Create CRDs"))
	defer s.Fail()
	if err = createOrUpdateCRDS(o.Dynamic, o.Version); err != nil {
		return fmt.Errorf("install crds failed: %s", err.Error())
	}
	s.Success()

	// add helm repo
	s = spinner.New(o.Out, spinnerMsg("Add and update repo "+types.KubeBlocksRepoName))
	defer s.Fail()
	// Add repo, if exists, will update it
	if err = helm.AddRepo(newHelmRepoEntry()); err != nil {
		return err
	}
	s.Success()

	// install KubeBlocks
	s = spinner.New(o.Out, spinnerMsg("Install KubeBlocks "+o.Version))
	defer s.Fail()

	getImageRegistry := func() string {
		registry := viperx.GetString(types.CfgKeyImageRegistry)
		if registry != "" {
			return registry
		}

		// get from values options
		for _, s := range o.ValueOpts.Values {
			if split := strings.Split(s, "="); split[0] == types.ImageRegistryKey && len(split) == 2 {
				registry = split[1]
				break
			}
		}

		// user do not specify image registry, get default image registry based on K8s provider and region
		if registry == "" {
			registry, err = util.GetImageRegistryByProvider(o.Client)
			if err != nil {
				fmt.Fprintf(o.ErrOut, "Failed to get image registry by provider: %v\n", err)
				return ""
			}
		}
		return registry
	}
	imageRegistry := getImageRegistry()
	if imageRegistry != "" {
		klog.V(1).Infof("Use image registry %s", imageRegistry)
		o.ValueOpts.Values = append(o.ValueOpts.Values, fmt.Sprintf("%s=%s", types.ImageRegistryKey, imageRegistry))
	}

	if err = o.installChart(); err != nil {
		return err
	}

	// save KB image.registry config
	writeImageRegistryKey := func(registry string) error {
		viperx.Set(types.CfgKeyImageRegistry, registry)
		v := viperx.GetViper()
		err := v.WriteConfig()
		if errors.As(err, &viper.ConfigFileNotFoundError{}) {
			dir, err := util.GetCliHomeDir()
			if err != nil {
				return err
			}
			return viper.WriteConfigAs(filepath.Join(dir, "config.yaml"))
		}
		return err
	}

	// if imageRegistry is not empty, save it to config file and used by addon
	if imageRegistry != "" {
		if err := writeImageRegistryKey(imageRegistry); err != nil {
			return err
		}
	}
	s.Success()

	// wait for auto-install addons to be ready
	if err = o.waitAddonsEnabled(); err != nil {
		fmt.Fprintf(o.Out, "Failed to wait for auto-install addons to be enabled, run \"kbcli kubeblocks status\" to check the status\n")
		return err
	}

	if !o.Quiet {
		msg := fmt.Sprintf("\nKubeBlocks %s installed to namespace %s SUCCESSFULLY!\n", o.Version, o.HelmCfg.Namespace())
		if !o.Wait {
			msg = fmt.Sprintf(`
KubeBlocks %s is installing to namespace %s.
You can check the KubeBlocks status by running "kbcli kubeblocks status"
`, o.Version, o.HelmCfg.Namespace())
		}
		fmt.Fprint(o.Out, msg)
		o.printNotes()
	}
	if o.upgradeFrom09 {
		fmt.Fprint(o.Out, "Start KubeBlocks 0.9\n")
		if err = o.configKB09(); err != nil {
			return err
		}
	}
	return nil
}

// waitAddonsEnabled waits for auto-install addons status to be enabled
func (o *InstallOptions) waitAddonsEnabled() error {
	if !o.Wait || !o.WaitAddons {
		return nil
	}

	addons := make(map[string]*extensionsv1alpha1.Addon)
	fetchAddons := func() error {
		objs, err := o.Dynamic.Resource(types.AddonGVR()).List(context.TODO(), metav1.ListOptions{
			LabelSelector: buildKubeBlocksSelectorLabels(),
		})
		if err != nil && !apierrors.IsNotFound(err) {
			return err
		}
		if objs == nil || len(objs.Items) == 0 {
			klog.V(1).Info("No Addons found")
			return nil
		}

		for _, obj := range objs.Items {
			addon := &extensionsv1alpha1.Addon{}
			if err = runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, addon); err != nil {
				return err
			}

			if addon.Status.ObservedGeneration == 0 {
				klog.V(1).Infof("Addon %s is not observed yet", addon.Name)
				continue
			}

			if !addon.Spec.InstallSpec.GetEnabled() {
				continue
			}

			// addon should be auto installed, check its status
			addons[addon.Name] = addon
			if addon.Status.Phase != extensionsv1alpha1.AddonEnabled {
				klog.V(1).Infof("Addon %s is not enabled yet, status %s", addon.Name, addon.Status.Phase)
			}
			if addon.Status.Phase == extensionsv1alpha1.AddonFailed {
				klog.V(1).Infof("Addon %s failed:", addon.Name)
				for _, c := range addon.Status.Conditions {
					klog.V(1).Infof("  %s: %s", c.Reason, c.Message)
				}
			}
		}
		return nil
	}

	suffixMsg := func(msg string) string {
		return fmt.Sprintf("%-50s", msg)
	}

	// create spinner
	msg := ""
	header := "Wait for addons to be enabled"
	failedErr := errors.New("some addons are failed to be enabled")
	s := spinner.New(o.Out, spinnerMsg(header))

	var (
		err         error
		spinnerDone = func() {
			s.SetFinalMsg(msg)
			s.Done("")
			fmt.Fprintln(o.Out)
		}
	)

	conditionFunc := func(_ context.Context) (bool, error) {
		if err = fetchAddons(); err != nil || len(addons) == 0 {
			return false, err
		}
		status := checkAddons(maps.Values(addons), true)
		msg = suffixMsg(fmt.Sprintf("%s\n  %s", header, status.outputMsg))
		s.SetMessage(msg)
		if status.allEnabled {
			spinnerDone()
			return true, nil
		} else if status.hasFailed {
			return false, failedErr
		}
		return false, nil
	}

	// wait all addons to be enabled, or timeout
	if err = wait.PollUntilContextTimeout(context.Background(), 5*time.Second, o.Timeout, true, conditionFunc); err != nil {
		spinnerDone()
		printAddonMsg(o.Out, maps.Values(addons), true)
		return err
	}

	return nil
}

func (o *InstallOptions) checkVersion(v util.Version) error {
	if !o.Check {
		return nil
	}

	// check installing version exists
	if exists, err := versionExists(o.Version); !exists {
		if err != nil {
			return err
		}
		return fmt.Errorf("version %s does not exist, please use \"kbcli kubeblocks list-versions --devel\" to show the available versions", o.Version)
	}

	versionErr := fmt.Errorf("failed to get kubernetes version")
	k8sVersionStr := v.Kubernetes
	if k8sVersionStr == "" {
		return versionErr
	}

	semVer := util.GetK8sSemVer(k8sVersionStr)
	if len(semVer) == 0 {
		return versionErr
	}

	// output kubernetes version
	fmt.Fprintf(o.Out, "Kubernetes version %s\n", ""+semVer)

	// disable or enable some features according to the kubernetes environment
	provider, err := util.GetK8sProvider(k8sVersionStr, o.Client)
	if err != nil {
		return fmt.Errorf("failed to get kubernetes provider: %v", err)
	}
	if provider.IsCloud() {
		fmt.Fprintf(o.Out, "Kubernetes provider %s\n", provider)
	}

	// check kbcli version, now do nothing
	fmt.Fprintf(o.Out, "kbcli version %s\n", v.Cli)

	return nil
}

func (o *InstallOptions) checkNamespace() error {
	// target namespace is not specified, use default namespace
	if o.HelmCfg.Namespace() == "" {
		o.HelmCfg.SetNamespace(o.Namespace)
	}
	if o.Namespace == types.DefaultNamespace {
		o.CreateNamespace = true
	}
	fmt.Fprintf(o.Out, "KubeBlocks will be installed to namespace \"%s\"\n", o.HelmCfg.Namespace())
	// check if namespace exists
	if !o.CreateNamespace {
		_, err := o.Client.CoreV1().Namespaces().Get(context.TODO(), o.Namespace, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			return fmt.Errorf("namespace %s not found, please use --create-namespace to create it", o.Namespace)
		}
		return err
	}
	return nil
}

func (o *InstallOptions) checkRemainedResource() error {
	if !o.Check {
		return nil
	}

	ns, _ := util.GetKubeBlocksNamespace(o.Client, o.Namespace)
	if ns == "" {
		ns = o.Namespace
	}

	// Now, we only check whether there are resources left by KubeBlocks, ignore
	// the addon resources.
	objs, err := getKBObjects(o.Dynamic, ns, nil)
	if err != nil {
		fmt.Fprintf(o.ErrOut, "Failed to get resources left by KubeBlocks before: %s\n", err.Error())
	}

	res := getRemainedResource(objs)
	if len(res) == 0 {
		return nil
	}

	// output remained resource
	var keys []string
	for k := range res {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	resStr := &bytes.Buffer{}
	for _, k := range keys {
		resStr.WriteString(fmt.Sprintf("  %s: %s\n", k, strings.Join(res[k], ",")))
	}
	return fmt.Errorf("there are resources left by previous KubeBlocks version, try to run \"kbcli kubeblocks uninstall\" to clean up\n%s", resStr.String())
}

func (o *InstallOptions) installChart() error {
	_, err := o.buildChart().Install(o.HelmCfg)
	return err
}

func (o *InstallOptions) printNotes() {
	fmt.Fprintf(o.Out, `
-> Basic commands for cluster:
    kbcli cluster create -h     # help information about creating a database cluster
    kbcli cluster list          # list all database clusters
    kbcli cluster describe <cluster name>  # get cluster information

-> Uninstall KubeBlocks:
    kbcli kubeblocks uninstall
`)
}

func (o *InstallOptions) buildChart() *helm.InstallOpts {
	return &helm.InstallOpts{
		Name:            types.KubeBlocksChartName,
		Chart:           types.KubeBlocksChartName + "/" + types.KubeBlocksChartName,
		Wait:            o.Wait,
		Version:         o.Version,
		Namespace:       o.HelmCfg.Namespace(),
		ValueOpts:       &o.ValueOpts,
		TryTimes:        2,
		CreateNamespace: o.CreateNamespace,
		Timeout:         o.Timeout,
		Atomic:          false,
	}
}

func (o *InstallOptions) disableHelmPreHookJob() {
	// disable kubeblocks helm pre hook job
	o.ValueOpts.Values = append(o.ValueOpts.Values, "crd.enabled=false")
}

func versionExists(version string) (bool, error) {
	if version == "" {
		return true, nil
	}

	allVers, err := getHelmChartVersions(types.KubeBlocksChartName)
	if err != nil {
		return false, err
	}

	for _, v := range allVers {
		if v.String() == version {
			return true, nil
		}
	}
	return false, nil
}
