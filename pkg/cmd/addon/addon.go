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

package addon

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"path"
	"sort"
	"strconv"
	"strings"

	extensionsv1alpha1 "github.com/apecloud/kubeblocks/apis/extensions/v1alpha1"
	"github.com/apecloud/kubeblocks/pkg/constant"
	viper "github.com/apecloud/kubeblocks/pkg/viperx"
	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	discoverycli "k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/kubectl/pkg/util/templates"
	"k8s.io/utils/strings/slices"

	"github.com/apecloud/kbcli/pkg/action"
	clusterCmd "github.com/apecloud/kbcli/pkg/cmd/cluster"
	"github.com/apecloud/kbcli/pkg/cmd/plugin"
	"github.com/apecloud/kbcli/pkg/printer"
	"github.com/apecloud/kbcli/pkg/types"
	"github.com/apecloud/kbcli/pkg/util"
)

type addonEnableFlags struct {
	MemorySets       []string
	CPUSets          []string
	StorageSets      []string
	ReplicaCountSets []string
	StorageClassSets []string
	TolerationsSet   []string
	SetValues        []string
	Force            bool
}

func (r *addonEnableFlags) useDefault() bool {
	return len(r.MemorySets) == 0 &&
		len(r.CPUSets) == 0 &&
		len(r.StorageSets) == 0 &&
		len(r.ReplicaCountSets) == 0 &&
		len(r.StorageClassSets) == 0 &&
		len(r.TolerationsSet) == 0
}

type addonCmdOpts struct {
	genericiooptions.IOStreams

	Factory cmdutil.Factory
	dynamic dynamic.Interface
	client  kubernetes.Interface

	addon extensionsv1alpha1.Addon

	*action.PatchOptions
	addonEnableFlags *addonEnableFlags

	autoApprove bool
	complete    func(self *addonCmdOpts, cmd *cobra.Command, args []string) error
}

type addonListOpts struct {
	*action.ListOptions

	// status is used to filter addons by status
	status []string
	// listEngines is used to list engine addons
	listEngines bool
}

// NewAddonCmd for addon functions
func NewAddonCmd(f cmdutil.Factory, streams genericiooptions.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "addon COMMAND",
		Short: "Addon command.",
	}
	cmd.AddCommand(
		newListCmd(f, streams),
		newDescribeCmd(f, streams),
		newEnableCmd(f, streams),
		newDisableCmd(f, streams),
		newIndexCmd(streams),
		newSearchCmd(f, streams),
		newInstallCmd(f, streams),
		newUninstallCmd(f, streams),
		newUpgradeCmd(f, streams),
		newPurgeResourcesCmd(f, streams),
	)
	return cmd
}

func newListCmd(f cmdutil.Factory, streams genericiooptions.IOStreams) *cobra.Command {
	o := &addonListOpts{
		ListOptions: action.NewListOptions(f, streams, types.AddonGVR()),
	}
	cmd := &cobra.Command{
		Use:               "list",
		Short:             "List addons.",
		Aliases:           []string{"ls"},
		ValidArgsFunction: util.ResourceNameCompletionFunc(f, o.ListOptions.GVR),
		Run: func(cmd *cobra.Command, args []string) {
			o.ListOptions.Names = args
			util.CheckErr(addonListRun(o))
		},
	}
	o.ListOptions.AddFlags(cmd, true)
	cmd.Flags().StringArrayVar(&o.status, "status", []string{}, "Filter addons by status")
	cmd.Flags().BoolVar(&o.listEngines, "engines", false, "List engine addons only")
	return cmd
}

func newDescribeCmd(f cmdutil.Factory, streams genericiooptions.IOStreams) *cobra.Command {
	o := &addonCmdOpts{
		PatchOptions: action.NewPatchOptions(f, streams, types.AddonGVR()),
		Factory:      f,
		IOStreams:    streams,
		complete:     addonDescribeHandler,
	}
	cmd := &cobra.Command{
		Use:               "describe ADDON_NAME",
		Short:             "Describe an addon specification.",
		Args:              cobra.ExactArgs(1),
		Aliases:           []string{"desc"},
		ValidArgsFunction: util.ResourceNameCompletionFunc(f, types.AddonGVR()),
		Run: func(cmd *cobra.Command, args []string) {
			util.CheckErr(o.init(args))
			util.CheckErr(o.fetchAddonObj())
			util.CheckErr(o.complete(o, cmd, args))
		},
	}
	return cmd
}

func newEnableCmd(f cmdutil.Factory, streams genericiooptions.IOStreams) *cobra.Command {
	o := &addonCmdOpts{
		PatchOptions:     action.NewPatchOptions(f, streams, types.AddonGVR()),
		Factory:          f,
		IOStreams:        streams,
		addonEnableFlags: &addonEnableFlags{},
		complete:         addonEnableDisableHandler,
	}

	o.PatchOptions.OutputOperation = func(didPatch bool) string {
		if didPatch {
			return "enabled"
		}
		return "enabled (no change)"
	}

	// # kbcli addon enable flags:
	// # [--memory [extraName:]<request>/<limit> (can specify multiple if has extra items)]
	// # [--cpu [extraName:]<request>/<limit> (can specify multiple if has extra items)]
	// # [--storage [extraName:]<request> (can specify multiple if has extra items)]
	// # [--replicas [extraName:]<number> (can specify multiple if has extra items)]
	// # [--storage-class [extraName:]<storage class name> (can specify multiple if has extra items)]
	// # [--tolerations [extraName:]<toleration JSON list items> (can specify multiple if has extra items)]
	// # [--dry-run] # TODO

	cmd := &cobra.Command{
		Use:               "enable ADDON_NAME",
		Short:             "Enable an addon.",
		Args:              cobra.MinimumNArgs(1),
		ValidArgsFunction: util.ResourceNameCompletionFunc(f, types.AddonGVR()),
		Example: templates.Examples(`
    	# Enabled "prometheus" addon
    	kbcli addon enable prometheus
    
        # Enabled "prometheus" addon with custom resources settings
    	kbcli addon enable prometheus --memory 512Mi/4Gi --storage 8Gi --replicas 2
    
        # Enabled "prometheus" addon and its extra alertmanager component with custom resources settings 
    	kbcli addon enable prometheus --memory 512Mi/4Gi --storage 8Gi --replicas 2 \
  			--memory alertmanager:16Mi/256Mi --storage alertmanager:1Gi --replicas alertmanager:2 

        # Enabled "prometheus" addon with tolerations 
    	kbcli addon enable prometheus \
			--tolerations '[{"key":"taintkey","operator":"Equal","effect":"NoSchedule","value":"true"}]' \
			--tolerations 'alertmanager:[{"key":"taintkey","operator":"Equal","effect":"NoSchedule","value":"true"}]'

		# Enabled "prometheus" addon with helm like custom settings
		kbcli addon enable prometheus --set prometheus.alertmanager.image.tag=v0.24.0

		# Force enabled "csi-s3" addon
		kbcli addon enable csi-s3 --force

		# Enable addons in batch
		kbcli addon enable prometheus csi-s3
`),
		Run: func(cmd *cobra.Command, args []string) {
			for _, name := range args { // This loop will fetch all the addons mentioned in the command and will install them one by one.
				util.CheckErr(o.init([]string{name}))
				util.CheckErr(o.fetchAddonObj())
				util.CheckErr(o.validate())
				util.CheckErr(o.complete(o, cmd, []string{name}))
				util.CheckErr(o.PatchOptions.CmdComplete(cmd))
				util.CheckErr(o.PatchOptions.Run())
				if isEngineAddon(&o.addon) {
					util.CheckErr(clusterCmd.RegisterClusterChart(f, streams, "", name, getAddonVersion(&o.addon), types.ClusterChartsRepoURL))
				}
			}
		},
	}
	cmd.Flags().StringArrayVar(&o.addonEnableFlags.MemorySets, "memory", []string{},
		"Sets addon memory resource values (--memory [extraName:]<request>/<limit>) (can specify multiple if has extra items))")
	cmd.Flags().StringArrayVar(&o.addonEnableFlags.CPUSets, "cpu", []string{},
		"Sets addon CPU resource values (--cpu [extraName:]<request>/<limit>) (can specify multiple if has extra items))")
	cmd.Flags().StringArrayVar(&o.addonEnableFlags.StorageSets, "storage", []string{},
		`Sets addon storage size (--storage [extraName:]<request>) (can specify multiple if has extra items)). 
Additional notes:
1. Specify '0' value will remove storage values settings and explicitly disable 'persistentVolumeEnabled' attribute.
2. For Helm type Addon, that resizing storage will fail if modified value is a storage request size 
that belongs to StatefulSet's volume claim template, to resolve 'Failed' Addon status possible action is disable and 
re-enable the addon (More info on how-to resize a PVC: https://kubernetes.io/docs/concepts/storage/persistent-volumes#resources).
`)
	cmd.Flags().StringArrayVar(&o.addonEnableFlags.ReplicaCountSets, "replicas", []string{},
		"Sets addon component replica count (--replicas [extraName:]<number>) (can specify multiple if has extra items))")
	cmd.Flags().StringArrayVar(&o.addonEnableFlags.StorageClassSets, "storage-class", []string{},
		"Sets addon storage class name (--storage-class [extraName:]<storage class name>) (can specify multiple if has extra items))")
	cmd.Flags().StringArrayVar(&o.addonEnableFlags.TolerationsSet, "tolerations", []string{},
		"Sets addon pod tolerations (--tolerations [extraName:]<toleration JSON list items>) (can specify multiple if has extra items))")
	cmd.Flags().StringArrayVar(&o.addonEnableFlags.SetValues, "set", []string{},
		"set values on the command line (can specify multiple or separate values with commas: key1=val1,key2=val2), it's only being processed if addon's type is helm.")
	cmd.Flags().BoolVar(&o.addonEnableFlags.Force, "force", false, "ignoring the installable restrictions and forcefully enabling.")

	o.PatchOptions.AddFlags(cmd)
	return cmd
}

func newDisableCmd(f cmdutil.Factory, streams genericiooptions.IOStreams) *cobra.Command {
	o := &addonCmdOpts{
		PatchOptions: action.NewPatchOptions(f, streams, types.AddonGVR()),
		Factory:      f,
		IOStreams:    streams,
		complete:     addonEnableDisableHandler,
	}

	o.PatchOptions.OutputOperation = func(didPatch bool) string {
		if didPatch {
			return "disabled"
		}
		return "disabled (no change)"
	}

	cmd := &cobra.Command{
		Use:               "disable ADDON_NAME",
		Short:             "Disable an addon.",
		Args:              cobra.MinimumNArgs(1), // This will accept more than one argument rather than keeping the args to exact
		ValidArgsFunction: util.ResourceNameCompletionFunc(f, types.AddonGVR()),
		Example: templates.Examples(`
			# Disable "prometheus" addon
			kbcli addon disable prometheus

			# Disable addons in batch
			kbcli addon disable prometheus csi-s3
`),
		Run: func(cmd *cobra.Command, args []string) {
			for _, name := range args { // This loop will fetch all the addons mentioned in the command and will remove them one by one.
				util.CheckErr(o.init([]string{name}))
				util.CheckErr(o.fetchAddonObj())
				util.CheckErr(o.checkBeforeDisable())
				util.CheckErr(o.complete(o, cmd, []string{name}))
				util.CheckErr(o.CmdComplete(cmd))
				util.CheckErr(o.PatchOptions.Run())
			}
		},
	}
	cmd.Flags().BoolVar(&o.autoApprove, "auto-approve", false, "Skip interactive approval before disabling addon")
	o.PatchOptions.AddFlags(cmd)
	return cmd
}

func (o *addonCmdOpts) init(args []string) error {
	o.PatchOptions.Names = args
	if o.dynamic == nil {
		var err error
		if o.dynamic, err = o.Factory.DynamicClient(); err != nil {
			return err
		}
	}
	var err error
	if o.client, err = o.Factory.KubernetesClientSet(); err != nil {
		return err
	}

	// setup _KUBE_SERVER_INFO
	if viper.Get(constant.CfgKeyServerInfo) == nil {
		cfg, _ := o.Factory.ToRESTConfig()
		cli, err := discoverycli.NewDiscoveryClientForConfig(cfg)
		if err != nil {
			return err
		}
		ver, err := cli.ServerVersion()
		if err != nil {
			return err
		}
		viper.SetDefault(constant.CfgKeyServerInfo, *ver)
	}

	return nil
}

func (o *addonCmdOpts) fetchAddonObj() error {
	ctx := context.TODO()
	obj, err := o.dynamic.Resource(o.GVR).Get(ctx, o.Names[0], metav1.GetOptions{})
	if err != nil {
		return err
	}
	if err = runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, &o.addon); err != nil {
		return err
	}
	return nil
}

func (o *addonCmdOpts) validate() error {
	if o.addonEnableFlags.Force {
		return nil
	}
	if o.addon.Spec.Installable == nil {
		return nil
	}
	for _, s := range o.addon.Spec.Installable.Selectors {
		if !s.MatchesFromConfig() {
			return fmt.Errorf("addon %s INSTALLABLE-SELECTOR has no matching requirement", o.PatchOptions.Names)
		}
	}

	if err := o.installAndUpgradePlugins(); err != nil {
		fmt.Fprintf(o.Out, "failed to install/upgrade plugins: %v\n", err)
	}

	return nil
}

func addonDescribeHandler(o *addonCmdOpts, cmd *cobra.Command, args []string) error {
	printRow := func(tbl *printer.TablePrinter, name string, item *extensionsv1alpha1.AddonInstallSpecItem) {
		pvEnabled := ""
		replicas := ""

		if item.PVEnabled != nil {
			pvEnabled = fmt.Sprintf("%v", *item.PVEnabled)
		}
		if item.Replicas != nil {
			replicas = fmt.Sprintf("%d", *item.Replicas)
		}

		printQuantity := func(q resource.Quantity, ok bool) string {
			if ok {
				return q.String()
			}
			return ""
		}

		q, ok := item.Resources.Requests[corev1.ResourceStorage]
		storageVal := printQuantity(q, ok)

		q, ok = item.Resources.Requests[corev1.ResourceCPU]
		cpuVal := printQuantity(q, ok)
		q, ok = item.Resources.Limits[corev1.ResourceCPU]
		cpuVal = fmt.Sprintf("%s/%s", cpuVal, printQuantity(q, ok))

		q, ok = item.Resources.Requests[corev1.ResourceMemory]
		memVal := printQuantity(q, ok)
		q, ok = item.Resources.Limits[corev1.ResourceMemory]
		memVal = fmt.Sprintf("%s/%s", memVal, printQuantity(q, ok))

		tbl.AddRow(name,
			replicas,
			storageVal,
			cpuVal,
			memVal,
			item.StorageClass,
			item.Tolerations,
			pvEnabled,
		)
	}
	printInstalled := func(tbl *printer.TablePrinter) error {
		installSpec := o.addon.Spec.InstallSpec
		printRow(tbl, "main", &installSpec.AddonInstallSpecItem)
		for _, e := range installSpec.ExtraItems {
			printRow(tbl, e.Name, &e.AddonInstallSpecItem)
		}
		return nil
	}

	var labels []string
	for k, v := range o.addon.Labels {
		if strings.Contains(k, constant.APIGroup) {
			labels = append(labels, fmt.Sprintf("%s=%s", k, v))
		}
	}
	printer.PrintPairStringToLine("Name", o.addon.Name, 0)
	printer.PrintPairStringToLine("Description", o.addon.Spec.Description, 0)
	printer.PrintPairStringToLine("Labels", strings.Join(labels, ","), 0)
	printer.PrintPairStringToLine("Type", string(o.addon.Spec.Type), 0)
	if len(o.addon.GetExtraNames()) > 0 {
		printer.PrintPairStringToLine("Extras", strings.Join(o.addon.GetExtraNames(), ","), 0)
	}
	printer.PrintPairStringToLine("Status", string(o.addon.Status.Phase), 0)
	var autoInstall bool
	if o.addon.Spec.Installable != nil {
		autoInstall = o.addon.Spec.Installable.AutoInstall
	}
	printer.PrintPairStringToLine("Auto-install", strconv.FormatBool(autoInstall), 0)
	if len(o.addon.Spec.Installable.GetSelectorsStrings()) > 0 {
		printer.PrintPairStringToLine("Auto-install selector", strings.Join(o.addon.Spec.Installable.GetSelectorsStrings(), ","), 0)
	}

	switch o.addon.Status.Phase {
	case extensionsv1alpha1.AddonEnabled:
		printer.PrintTitle("Installed Info")
		printer.PrintLineWithTabSeparator()
		if err := printer.PrintTable(o.Out, nil, printInstalled,
			"NAME", "REPLICAS", "STORAGE", "CPU (REQ/LIMIT)", "MEMORY (REQ/LIMIT)", "STORAGE-CLASS",
			"TOLERATIONS", "PV-ENABLED"); err != nil {
			return err
		}
	default:
		printer.PrintLineWithTabSeparator()
		for _, di := range o.addon.Spec.GetSortedDefaultInstallValues() {
			printInstallable := func(tbl *printer.TablePrinter) error {
				if len(di.Selectors) == 0 {
					printer.PrintLineWithTabSeparator(
						printer.NewPair("Default install selector", "NONE"),
					)
				} else {
					printer.PrintLineWithTabSeparator(
						printer.NewPair("Default install selector", strings.Join(di.GetSelectorsStrings(), ",")),
					)
				}
				installSpec := di.AddonInstallSpec
				printRow(tbl, "main", &installSpec.AddonInstallSpecItem)
				for _, e := range installSpec.ExtraItems {
					printRow(tbl, e.Name, &e.AddonInstallSpecItem)
				}
				return nil
			}
			if err := printer.PrintTable(o.Out, nil, printInstallable,
				"NAME", "REPLICAS", "STORAGE", "CPU (REQ/LIMIT)", "MEMORY (REQ/LIMIT)", "STORAGE-CLASS",
				"TOLERATIONS", "PV-ENABLED"); err != nil {
				return err
			}
			printer.PrintLineWithTabSeparator()
		}
	}

	// print failed message
	if o.addon.Status.Phase == extensionsv1alpha1.AddonFailed {
		var tbl *printer.TablePrinter
		printHeader := true
		for _, c := range o.addon.Status.Conditions {
			if c.Status == metav1.ConditionTrue {
				continue
			}
			if printHeader {
				fmt.Fprintln(o.Out, "Failed Message")
				tbl = printer.NewTablePrinter(o.Out)
				tbl.Tbl.SetColumnConfigs([]table.ColumnConfig{
					{Number: 3, WidthMax: 120},
				})
				tbl.SetHeader("TIME", "REASON", "MESSAGE")
				printHeader = false
			}
			tbl.AddRow(util.TimeFormat(&c.LastTransitionTime), c.Reason, c.Message)
		}
		tbl.Print()
	}

	return nil
}

func addonEnableDisableHandler(o *addonCmdOpts, cmd *cobra.Command, args []string) error {
	// record the flags that been set by user
	var flags []*pflag.Flag
	cmd.Flags().Visit(func(flag *pflag.Flag) {
		flags = append(flags, flag)
	})
	return o.buildPatch(flags)
}

func (o *addonCmdOpts) buildEnablePatch(flags []*pflag.Flag, spec, install map[string]interface{}) (err error) {
	extraNames := o.addon.GetExtraNames()
	installSpec := extensionsv1alpha1.AddonInstallSpec{
		Enabled:              true,
		AddonInstallSpecItem: extensionsv1alpha1.NewAddonInstallSpecItem(),
	}
	// only using named return value in defer function
	defer func() {
		if err != nil {
			return
		}
		var b []byte
		b, err = json.Marshal(&installSpec)
		if err != nil {
			return
		}
		if err = json.Unmarshal(b, &install); err != nil {
			return
		}
	}()

	if o.addonEnableFlags.useDefault() {
		return nil
	}

	// extractInstallSpecExtraItem extracts extensionsv1alpha1.AddonInstallExtraItem
	// for the matching arg name, if not found, appends extensionsv1alpha1.AddonInstallExtraItem
	// item to installSpec.ExtraItems and returns its pointer.
	extractInstallSpecExtraItem := func(name string) (*extensionsv1alpha1.AddonInstallExtraItem, error) {
		var pItem *extensionsv1alpha1.AddonInstallExtraItem
		for i, eItem := range installSpec.ExtraItems {
			if eItem.Name == name {
				pItem = &installSpec.ExtraItems[i]
				break
			}
		}
		if pItem == nil {
			if !slices.Contains(extraNames, name) {
				return nil, fmt.Errorf("invalid extra item name [%s]", name)
			}
			installSpec.ExtraItems = append(installSpec.ExtraItems, extensionsv1alpha1.AddonInstallExtraItem{
				Name:                 name,
				AddonInstallSpecItem: extensionsv1alpha1.NewAddonInstallSpecItem(),
			})
			pItem = &installSpec.ExtraItems[len(installSpec.ExtraItems)-1]
		}
		return pItem, nil
	}

	_tuplesProcessor := func(t []string, s, flag string,
		valueTransformer func(s, flag string) (interface{}, error),
		valueAssigner func(*extensionsv1alpha1.AddonInstallSpecItem, interface{}),
	) error {
		l := len(t)
		var name string
		var result interface{}
		switch l {
		case 2:
			name = t[0]
			fallthrough
		case 1:
			if valueTransformer != nil {
				result, err = valueTransformer(t[l-1], flag)
				if err != nil {
					return err
				}
			} else {
				result = t[l-1]
			}
		default:
			return fmt.Errorf("wrong flag value --%s=%s", flag, s)
		}
		name = strings.TrimSpace(name)
		if name == "" {
			valueAssigner(&installSpec.AddonInstallSpecItem, result)
		} else {
			pItem, err := extractInstallSpecExtraItem(name)
			if err != nil {
				return err
			}
			valueAssigner(&pItem.AddonInstallSpecItem, result)
		}
		return nil
	}

	twoTuplesProcessor := func(s, flag string,
		valueTransformer func(s, flag string) (interface{}, error),
		valueAssigner func(*extensionsv1alpha1.AddonInstallSpecItem, interface{}),
	) error {
		t := strings.SplitN(s, ":", 2)
		return _tuplesProcessor(t, s, flag, valueTransformer, valueAssigner)
	}

	twoTuplesJSONProcessor := func(s, flag string,
		valueTransformer func(s, flag string) (interface{}, error),
		valueAssigner func(*extensionsv1alpha1.AddonInstallSpecItem, interface{}),
	) error {
		var jsonArray []map[string]interface{}
		var t []string

		err := json.Unmarshal([]byte(s), &jsonArray)
		if err != nil {
			// not a valid JSON array treat it a 2 tuples
			t = strings.SplitN(s, ":", 2)
		} else {
			t = []string{s}
		}
		return _tuplesProcessor(t, s, flag, valueTransformer, valueAssigner)
	}

	reqLimitResTransformer := func(s, flag string) (interface{}, error) {
		t := strings.SplitN(s, "/", 2)
		if len(t) != 2 {
			return nil, fmt.Errorf("wrong flag value --%s=%s", flag, s)
		}
		reqLim := [2]resource.Quantity{}
		processTuple := func(i int) error {
			if t[i] == "" {
				return nil
			}
			q, err := resource.ParseQuantity(t[i])
			if err != nil {
				return err
			}
			reqLim[i] = q
			return nil
		}
		for i := range t {
			if err := processTuple(i); err != nil {
				return nil, fmt.Errorf("wrong flag value --%s=%s, with error %v", flag, s, err)
			}
		}
		return reqLim, nil
	}

	f := o.addonEnableFlags
	for _, v := range f.ReplicaCountSets {
		if err := twoTuplesProcessor(v, "replicas", func(s, flag string) (interface{}, error) {
			v, err := strconv.Atoi(s)
			if err != nil {
				return nil, fmt.Errorf("wrong flag value --%s=%s, with error %v", flag, s, err)
			}
			if v < 0 {
				return nil, fmt.Errorf("wrong flag value --%s=%s replica count value", flag, s)
			}
			if v > math.MaxInt32 {
				return nil, fmt.Errorf("wrong flag value --%s=%s replica count exceed max. value (%d) ", flag, s, math.MaxInt32)
			}
			r := int32(v)
			return &r, nil
		}, func(item *extensionsv1alpha1.AddonInstallSpecItem, i interface{}) {
			item.Replicas = i.(*int32)
		}); err != nil {
			return err
		}
	}

	for _, v := range f.StorageClassSets {
		if err := twoTuplesProcessor(v, "storage-class", nil, func(item *extensionsv1alpha1.AddonInstallSpecItem, i interface{}) {
			item.StorageClass = i.(string)
		}); err != nil {
			return err
		}
	}

	for _, v := range f.TolerationsSet {
		if err := twoTuplesJSONProcessor(v, "tolerations", nil, func(item *extensionsv1alpha1.AddonInstallSpecItem, i interface{}) {
			item.Tolerations = i.(string)
		}); err != nil {
			return err
		}
	}

	for _, v := range f.StorageSets {
		if err := twoTuplesProcessor(v, "storage", func(s, flag string) (interface{}, error) {
			q, err := resource.ParseQuantity(s)
			if err != nil {
				return nil, fmt.Errorf("wrong flag value --%s=%s, with error %v", flag, s, err)
			}
			return q, nil
		}, func(item *extensionsv1alpha1.AddonInstallSpecItem, i interface{}) {
			q := i.(resource.Quantity)
			// for 0 storage size, remove storage request value and explicitly disable `persistentVolumeEnabled`
			if v, _ := q.AsInt64(); v == 0 {
				delete(item.Resources.Requests, corev1.ResourceStorage)
				b := false
				item.PVEnabled = &b
				return
			}
			item.Resources.Requests[corev1.ResourceStorage] = q
			// explicitly enable `persistentVolumeEnabled` if with provided storage size setting
			b := true
			item.PVEnabled = &b
		}); err != nil {
			return err
		}
	}

	for _, v := range f.CPUSets {
		if err := twoTuplesProcessor(v, "cpu", reqLimitResTransformer, func(item *extensionsv1alpha1.AddonInstallSpecItem, i interface{}) {
			reqLim := i.([2]resource.Quantity)
			item.Resources.Requests[corev1.ResourceCPU] = reqLim[0]
			item.Resources.Limits[corev1.ResourceCPU] = reqLim[1]
		}); err != nil {
			return err
		}
	}

	for _, v := range f.MemorySets {
		if err := twoTuplesProcessor(v, "memory", reqLimitResTransformer, func(item *extensionsv1alpha1.AddonInstallSpecItem, i interface{}) {
			reqLim := i.([2]resource.Quantity)
			item.Resources.Requests[corev1.ResourceMemory] = reqLim[0]
			item.Resources.Limits[corev1.ResourceMemory] = reqLim[1]
		}); err != nil {
			return err
		}
	}

	return nil
}

func (o *addonCmdOpts) buildHelmPatch(result map[string]interface{}) error {
	var helmSpec extensionsv1alpha1.HelmTypeInstallSpec
	if o.addon.Spec.Helm == nil {
		helmSpec = extensionsv1alpha1.HelmTypeInstallSpec{
			InstallValues: extensionsv1alpha1.HelmInstallValues{
				SetValues: o.addonEnableFlags.SetValues,
			},
		}
	} else {
		helmSpec = *o.addon.Spec.Helm
		helmSpec.InstallValues.SetValues = o.addonEnableFlags.SetValues
	}
	// use default KB registry source
	declared := false
	for _, s := range helmSpec.InstallValues.SetValues {
		if split := strings.Split(s, "="); split[0] == types.ImageRegistryKey && len(split) == 2 {
			declared = true
			break
		}
	}

	if defaultRegistry := viper.Get(types.CfgKeyImageRegistry); !declared && defaultRegistry != nil && defaultRegistry != "" {
		helmSpec.InstallValues.SetValues = append(helmSpec.InstallValues.SetValues, fmt.Sprintf("%s=%s", types.ImageRegistryKey, defaultRegistry))
	}

	b, err := json.Marshal(&helmSpec)
	if err != nil {
		return err
	}
	if err = json.Unmarshal(b, &result); err != nil {
		return err
	}
	return nil
}

func (o *addonCmdOpts) buildPatch(flags []*pflag.Flag) error {
	var err error
	spec := map[string]interface{}{}
	status := map[string]interface{}{}
	install := map[string]interface{}{}
	helm := map[string]interface{}{}

	if o.addonEnableFlags != nil {
		if o.addon.Status.Phase == extensionsv1alpha1.AddonFailed {
			status["phase"] = nil
		}
		if err = o.buildEnablePatch(flags, spec, install); err != nil {
			return err
		}

		if err = o.buildHelmPatch(helm); err != nil {
			return err
		}
	} else {
		if !o.addon.Spec.InstallSpec.GetEnabled() {
			fmt.Fprintf(o.Out, "%s/%s is already disabled\n", o.GVR.GroupResource().String(), o.Names[0])
			return cmdutil.ErrExit
		}
		install["enabled"] = false
	}

	if err = unstructured.SetNestedField(spec, install, "install"); err != nil {
		return err
	}

	if err = unstructured.SetNestedField(spec, helm, "helm"); err != nil {
		return err
	}

	obj := unstructured.Unstructured{
		Object: map[string]interface{}{
			"spec": spec,
		},
	}
	if len(status) > 0 {
		phase := ""
		if p, ok := status["phase"]; ok && p != nil {
			phase = p.(string)
		}
		fmt.Printf("patching addon 'status.phase=%s' to 'status.phase=%v' will result addon install spec (spec.install) not being updated\n",
			o.addon.Status.Phase, phase)
		obj.Object["status"] = status
		o.Subresource = "status"
	}
	bytes, err := obj.MarshalJSON()
	if err != nil {
		return err
	}
	o.PatchOptions.Patch = string(bytes)
	return nil
}

func addonListRun(o *addonListOpts) error {
	// if format is JSON or YAML, use default printer to output the result.
	if o.ListOptions.Format == printer.JSON || o.ListOptions.Format == printer.YAML {
		_, err := o.ListOptions.Run()
		return err
	}

	// get and output the result
	o.ListOptions.Print = false
	r, err := o.ListOptions.Run()
	if err != nil {
		return err
	}

	infos, err := r.Infos()
	if err != nil {
		return err
	}

	if len(infos) == 0 {
		fmt.Fprintln(o.IOStreams.Out, "No addon found")
		return nil
	}

	printRows := func(tbl *printer.TablePrinter) error {
		// sort addons with .status.Phase then .metadata.name
		sort.SliceStable(infos, func(i, j int) bool {
			toAddon := func(idx int) *extensionsv1alpha1.Addon {
				addon := &extensionsv1alpha1.Addon{}
				if err := runtime.DefaultUnstructuredConverter.FromUnstructured(infos[idx].Object.(*unstructured.Unstructured).Object, addon); err != nil {
					return nil
				}
				return addon
			}
			iAddon := toAddon(i)
			jAddon := toAddon(j)
			if iAddon == nil {
				return true
			}
			if jAddon == nil {
				return false
			}
			if iAddon.Status.Phase == jAddon.Status.Phase {
				return iAddon.GetName() < jAddon.GetName()
			}
			return iAddon.Status.Phase < jAddon.Status.Phase
		})
		for _, info := range infos {
			addon := &extensionsv1alpha1.Addon{}
			obj := info.Object.(*unstructured.Unstructured)
			if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, addon); err != nil {
				return err
			}
			extraNames := addon.GetExtraNames()
			var selectors []string
			var autoInstall bool
			if addon.Spec.Installable != nil {
				selectors = addon.Spec.Installable.GetSelectorsStrings()
				autoInstall = addon.Spec.Installable.AutoInstall
			}
			label := obj.GetLabels()
			provider := label[types.AddonProviderLabelKey]
			if provider == "" {
				provider = label[types.ProviderLabelKey]
			}
			version := getAddonVersion(addon)

			// only show addons with matching status
			if !matchStatus(addon, o.status) {
				continue
			}

			// only show engine addons
			if o.listEngines && !isEngineAddon(addon) {
				continue
			}

			if o.Format == printer.Wide {
				tbl.AddRow(addon.Name,
					version,
					provider,
					addon.Status.Phase,
					autoInstall,
					strings.Join(selectors, ";"),
					strings.Join(extraNames, ","),
				)
			} else {
				tbl.AddRow(addon.Name,
					version,
					provider,
					addon.Status.Phase,
					autoInstall,
				)
			}
		}
		return nil
	}

	if o.ListOptions.Format == printer.Wide {
		if err = printer.PrintTable(o.Out, nil, printRows,
			"NAME", "VERSION", "PROVIDER", "STATUS", "AUTO-INSTALL", "AUTO-INSTALLABLE-SELECTOR", "EXTRAS"); err != nil {
			return err
		}
	} else {
		if err = printer.PrintTable(o.Out, nil, printRows,
			"NAME", "VERSION", "PROVIDER", "STATUS", "AUTO-INSTALL"); err != nil {
			return err
		}
	}
	return nil
}

func (o *addonCmdOpts) installAndUpgradePlugins() error {
	if len(o.addon.Spec.CliPlugins) == 0 {
		return nil
	}

	plugin.InitPlugin()

	paths := plugin.GetKbcliPluginPath()
	indexes, err := plugin.ListIndexes(paths)
	if err != nil {
		return err
	}

	indexRepositoryToNme := make(map[string]string)
	for _, index := range indexes {
		indexRepositoryToNme[index.URL] = index.Name
	}

	var plugins []string
	var names []string
	for _, p := range o.addon.Spec.CliPlugins {
		names = append(names, p.Name)
		indexName, ok := indexRepositoryToNme[p.IndexRepository]
		if !ok {
			// index not found, add it
			_, indexName = path.Split(p.IndexRepository)
			if err := plugin.AddIndex(paths, indexName, p.IndexRepository); err != nil {
				return err
			}
		}
		plugins = append(plugins, fmt.Sprintf("%s/%s", indexName, p.Name))
	}

	installOption := &plugin.PluginInstallOption{
		IOStreams: o.IOStreams,
	}
	upgradeOption := &plugin.UpgradeOptions{
		IOStreams: o.IOStreams,
	}

	// install plugins
	if err := installOption.Complete(plugins); err != nil {
		return err
	}
	if err := installOption.Install(); err != nil {
		return err
	}

	// upgrade existed plugins
	if err := upgradeOption.Complete(names); err != nil {
		return err
	}
	if err := upgradeOption.Run(); err != nil {
		return err
	}

	return nil
}

func isEngineAddon(addon *extensionsv1alpha1.Addon) bool {
	labels := addon.GetLabels()
	if len(labels) == 0 {
		return false
	}
	if _, ok := labels[types.AddonModelLabelKey]; ok {
		return true
	}
	return false
}

func matchStatus(addon *extensionsv1alpha1.Addon, status []string) bool {
	if len(status) == 0 {
		return true
	}
	for _, s := range status {
		if strings.EqualFold(string(addon.Status.Phase), s) {
			return true
		}
	}
	return false
}

func (o *addonCmdOpts) checkBeforeDisable() error {
	if o.autoApprove {
		return nil
	}
	return CheckAddonUsedByCluster(o.dynamic, o.Names, o.In)
}
