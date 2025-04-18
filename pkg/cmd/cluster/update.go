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

package cluster

import (
	"context"
	"encoding/csv"
	"fmt"
	"strconv"
	"strings"

	kbappsv1 "github.com/apecloud/kubeblocks/apis/apps/v1"
	"github.com/robfig/cron/v3"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	"k8s.io/client-go/dynamic"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/kubectl/pkg/util/templates"

	dpv1alpha1 "github.com/apecloud/kubeblocks/apis/dataprotection/v1alpha1"
	cfgutil "github.com/apecloud/kubeblocks/pkg/configuration/util"
	"github.com/apecloud/kubeblocks/pkg/constant"
	"github.com/apecloud/kubeblocks/pkg/dataprotection/utils"

	"github.com/apecloud/kbcli/pkg/action"
	"github.com/apecloud/kbcli/pkg/cluster"
	"github.com/apecloud/kbcli/pkg/types"
	"github.com/apecloud/kbcli/pkg/util"
)

var clusterUpdateExample = templates.Examples(`
	# update cluster mycluster termination policy to Delete
	kbcli cluster update mycluster --termination-policy=Delete

	# enable cluster monitor
	kbcli cluster update mycluster --monitor=true

	# update cluster tolerations
	kbcli cluster update mycluster --tolerations='"key=engineType,value=mongo,operator=Equal,effect=NoSchedule","key=diskType,value=ssd,operator=Equal,effect=NoSchedule"'

	# edit cluster
	kbcli cluster update mycluster --edit

	# enable cluster monitor and edit
    # kbcli cluster update mycluster --monitor=true --edit

	# enable cluster auto backup
	kbcli cluster update mycluster --backup-enabled=true

	# update cluster backup retention period
	kbcli cluster update mycluster --backup-retention-period=1d

	# update cluster backup method
	kbcli cluster update mycluster --backup-method=snapshot

	# update cluster backup cron expression
	kbcli cluster update mycluster --backup-cron-expression="0 0 * * *"

	# update cluster backup starting deadline minutes
	kbcli cluster update mycluster --backup-starting-deadline-minutes=10

	# update cluster backup repo name
	kbcli cluster update mycluster --backup-repo-name=repo1

	# update cluster backup pitr enabled
	kbcli cluster update mycluster --pitr-enabled=true
`)

// UpdatableFlags is the flags that cat be updated by update command
type UpdatableFlags struct {
	// Options for cluster termination policy
	TerminationPolicy string `json:"terminationPolicy"`

	// Add-on switches for cluster observability
	DisableExporter bool `json:"monitor"`

	// Configuration and options for cluster affinity and tolerations
	PodAntiAffinity  string `json:"podAntiAffinity"`
	RuntimeClassName string `json:"runtimeClassName,omitempty"`
	// TopologyKeys if TopologyKeys is nil, add omitempty json tag, because CueLang can not covert null to list.
	TopologyKeys   []string          `json:"topologyKeys,omitempty"`
	NodeLabels     map[string]string `json:"nodeLabels,omitempty"`
	Tenancy        string            `json:"tenancy"`
	TolerationsRaw []string          `json:"-"`

	// backup config
	BackupEnabled                 bool   `json:"-"`
	BackupRetentionPeriod         string `json:"-"`
	BackupMethod                  string `json:"-"`
	BackupCronExpression          string `json:"-"`
	BackupStartingDeadlineMinutes int64  `json:"-"`
	BackupRepoName                string `json:"-"`
	BackupPITREnabled             bool   `json:"-"`
}

type UpdateOptions struct {
	namespace string
	dynamic   dynamic.Interface
	cluster   *kbappsv1.Cluster
	ValMap    map[string]interface{}

	UpdatableFlags
	*action.PatchOptions
}

func NewUpdateOptions(f cmdutil.Factory, streams genericiooptions.IOStreams) *UpdateOptions {
	o := &UpdateOptions{PatchOptions: action.NewPatchOptions(f, streams, types.ClusterGVR())}
	o.PatchOptions.OutputOperation = func(didPatch bool) string {
		if didPatch {
			return "updated"
		}
		return "updated (no change)"
	}
	o.ValMap = make(map[string]interface{})
	return o
}

func NewUpdateCmd(f cmdutil.Factory, streams genericiooptions.IOStreams) *cobra.Command {

	o := NewUpdateOptions(f, streams)

	cmd := &cobra.Command{
		Use:               "update NAME",
		Short:             "Update the cluster settings, such as enable or disable monitor or log.",
		Example:           clusterUpdateExample,
		ValidArgsFunction: util.ResourceNameCompletionFunc(f, o.GVR),
		Run: func(cmd *cobra.Command, args []string) {
			util.CheckErr(o.CmdComplete(cmd, args))
			util.CheckErr(o.Exec())
		},
	}
	o.UpdatableFlags.addFlags(cmd)
	o.PatchOptions.AddFlags(cmd)

	return cmd
}

func (f *UpdatableFlags) addFlags(cmd *cobra.Command) {
	cmd.Flags().BoolVar(&f.DisableExporter, "disable-exporter", true, "Enable or disable monitoring")
	cmd.Flags().StringVar(&f.TerminationPolicy, "termination-policy", "Delete", "Termination policy, one of: (DoNotTerminate, Delete, WipeOut)")
	cmd.Flags().StringSliceVar(&f.TolerationsRaw, "tolerations", nil, `Tolerations for cluster, such as "key=value:effect, key:effect", for example '"engineType=mongo:NoSchedule", "diskType:NoSchedule"'`)
	cmd.Flags().BoolVar(&f.BackupEnabled, "backup-enabled", false, "Specify whether enabled automated backup")
	cmd.Flags().StringVar(&f.BackupRetentionPeriod, "backup-retention-period", "1d", "a time string ending with the 'd'|'D'|'h'|'H' character to describe how long the Backup should be retained")
	cmd.Flags().StringVar(&f.BackupMethod, "backup-method", "", "the backup method, view it by \"kbcli cd describe <cluster-definition>\", if not specified, the default backup method will be to take snapshots of the volume")
	cmd.Flags().StringVar(&f.BackupCronExpression, "backup-cron-expression", "", "the cron expression for schedule, the timezone is in UTC. see https://en.wikipedia.org/wiki/Cron.")
	cmd.Flags().Int64Var(&f.BackupStartingDeadlineMinutes, "backup-starting-deadline-minutes", 0, "the deadline in minutes for starting the backup job if it misses its scheduled time for any reason")
	cmd.Flags().StringVar(&f.BackupRepoName, "backup-repo-name", "", "the backup repository name")
	cmd.Flags().BoolVar(&f.BackupPITREnabled, "pitr-enabled", false, "Specify whether enabled point in time recovery")
	cmd.Flags().StringVar(&f.RuntimeClassName, "runtime-class-name", "", "Specifies runtimeClassName for all Pods managed by this Cluster.")
	util.CheckErr(cmd.RegisterFlagCompletionFunc(
		"termination-policy",
		func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			return []string{
				"DoNotTerminate\tprevents deletion of the Cluster",
				"Delete\tdeletes all runtime resources belong to the Cluster.",
				"WipeOut\tdeletes all Cluster resources, including volume snapshots and backups in external storage.",
			}, cobra.ShellCompDirectiveNoFileComp
		}))
}

func (o *UpdateOptions) CmdComplete(cmd *cobra.Command, args []string) error {

	o.Names = args

	// record the flags that been set by user
	var flags []*pflag.Flag
	cmd.Flags().Visit(func(flag *pflag.Flag) {
		flags = append(flags, flag)
	})

	// nothing to do
	if len(flags) == 0 {
		return nil
	}

	for _, flag := range flags {
		v := flag.Value
		var val interface{}
		switch v.Type() {
		case "string":
			val = v.String()
		case "stringArray", "stringSlice":
			val = v.(pflag.SliceValue).GetSlice()
		case "stringToString":
			valMap := make(map[string]interface{}, 0)
			vStr := strings.Trim(v.String(), "[]")
			if len(vStr) > 0 {
				r := csv.NewReader(strings.NewReader(vStr))
				ss, err := r.Read()
				if err != nil {
					return err
				}
				for _, pair := range ss {
					kv := strings.SplitN(pair, "=", 2)
					if len(kv) != 2 {
						return fmt.Errorf("%s must be formatted as key=value", pair)
					}
					valMap[kv[0]] = kv[1]
				}
			}
			val = valMap
		default:
			val = v.String()
		}
		o.ValMap[flag.Name] = val
	}

	if err := o.PatchOptions.CmdComplete(cmd); err != nil {
		return err
	}

	return nil
}

func (o *UpdateOptions) Validate() error {
	if len(o.Names) == 0 {
		return makeMissingClusterNameErr()
	}
	if len(o.Names) > 1 {
		return fmt.Errorf("only support to update one cluster")
	}
	return nil
}

func (o *UpdateOptions) Exec() error {
	if err := o.Validate(); err != nil {
		return err
	}
	if err := o.Complete(); err != nil {
		return err
	}
	if err := o.Run(); err != nil {
		return err
	}
	return nil
}

func (o *UpdateOptions) Complete() error {
	var err error
	if o.namespace, _, err = o.Factory.ToRawKubeConfigLoader().Namespace(); err != nil {
		return err
	}
	if o.dynamic, err = o.Factory.DynamicClient(); err != nil {
		return err
	}
	return o.buildPatch()
}

func (o *UpdateOptions) buildPatch() error {
	var err error
	type buildFn func(obj map[string]interface{}, val interface{}, field string) error

	buildFlagObj := func(obj map[string]interface{}, val interface{}, field string) error {
		return unstructured.SetNestedField(obj, val, field)
	}

	buildTolObj := func(obj map[string]interface{}, val interface{}, field string) error {
		tolerations, err := util.BuildTolerations(o.TolerationsRaw)
		if err != nil {
			return err
		}
		return unstructured.SetNestedField(obj, tolerations, field)
	}

	buildComps := func(obj map[string]interface{}, val interface{}, field string) error {
		v, ok := val.(string)
		if !ok {
			return fmt.Errorf("val is not a string")
		}
		return o.buildComponents(field, v)
	}

	buildBackup := func(obj map[string]interface{}, val interface{}, field string) error {
		v, ok := val.(string)
		if !ok {
			return fmt.Errorf("val is not a string")
		}
		return o.buildBackup(field, v)
	}

	spec := map[string]interface{}{}
	// affinity := map[string]interface{}{}
	schedulingPolicy := map[string]interface{}{}
	type filedObj struct {
		field string
		obj   map[string]interface{}
		fn    buildFn
	}

	flagFieldMapping := map[string]*filedObj{
		"termination-policy": {field: "terminationPolicy", obj: spec, fn: buildFlagObj},
		// tolerations
		"tolerations": {field: "tolerations", obj: schedulingPolicy, fn: buildTolObj},

		// monitor and logs
		"disable-exporter": {field: "disableExporter", obj: nil, fn: buildComps},

		// backup config
		"backup-enabled":                   {field: "enabled", obj: nil, fn: buildBackup},
		"backup-retention-period":          {field: "retentionPeriod", obj: nil, fn: buildBackup},
		"backup-method":                    {field: "method", obj: nil, fn: buildBackup},
		"backup-cron-expression":           {field: "cronExpression", obj: nil, fn: buildBackup},
		"backup-starting-deadline-minutes": {field: "startingDeadlineMinutes", obj: nil, fn: buildBackup},
		"backup-repo-name":                 {field: "repoName", obj: nil, fn: buildBackup},
		"pitr-enabled":                     {field: "pitrEnabled", obj: nil, fn: buildBackup},
		"runtime-class-name":               {field: "runtimeClassName", obj: spec, fn: buildFlagObj},
	}

	for name, val := range o.ValMap {
		if f, ok := flagFieldMapping[name]; ok {
			if err = f.fn(f.obj, val, f.field); err != nil {
				return err
			}
		}
	}

	if len(schedulingPolicy) > 0 {
		if err = unstructured.SetNestedField(spec, schedulingPolicy, "schedulingPolicy"); err != nil {
			return err
		}
	}

	if o.cluster != nil {
		// if update the backup config, the backup method must have value
		if o.cluster.Spec.Backup != nil {
			backupPolicyListObj, err := o.dynamic.Resource(types.BackupPolicyGVR()).Namespace(o.namespace).List(context.Background(), metav1.ListOptions{
				LabelSelector: fmt.Sprintf("%s=%s", constant.AppInstanceLabelKey, o.cluster.Name),
			})
			if err != nil {
				return err
			}
			backupPolicyList := &dpv1alpha1.BackupPolicyList{}
			if err := runtime.DefaultUnstructuredConverter.FromUnstructured(backupPolicyListObj.UnstructuredContent(), backupPolicyList); err != nil {
				return err
			}

			defaultBackupMethod, backupMethodMap := utils.GetBackupMethodsFromBackupPolicy(backupPolicyList, "")
			if err != nil {
				return err
			}
			if o.cluster.Spec.Backup.Method == "" {
				o.cluster.Spec.Backup.Method = defaultBackupMethod
			}
			if _, ok := backupMethodMap[o.cluster.Spec.Backup.Method]; !ok {
				return fmt.Errorf("backup method %s is not supported, please view the supported backup methods by `kbcli cd describe %s`", o.cluster.Spec.Backup.Method, o.cluster.Spec.ClusterDef)
			}
		}

		data, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&o.cluster.Spec)
		if err != nil {
			return err
		}

		if err = unstructured.SetNestedField(spec, data["componentSpecs"], "componentSpecs"); err != nil {
			return err
		}

		if err = unstructured.SetNestedField(spec, data["backup"], "backup"); err != nil {
			return err
		}
	}

	obj := unstructured.Unstructured{
		Object: map[string]interface{}{
			"spec": spec,
		},
	}
	bytes, err := obj.MarshalJSON()
	if err != nil {
		return err
	}
	o.Patch = string(bytes)
	return nil
}

func (o *UpdateOptions) buildComponents(field string, val string) error {
	if o.cluster == nil {
		c, err := cluster.GetClusterByName(o.dynamic, o.Names[0], o.namespace)
		if err != nil {
			return err
		}
		o.cluster = c
	}

	switch field {
	case "disableExporter":
		return o.updateMonitor(val)
	default:
		return nil
	}
}

func (o *UpdateOptions) buildBackup(field string, val string) error {
	if o.cluster == nil {
		c, err := cluster.GetClusterByName(o.dynamic, o.Names[0], o.namespace)
		if err != nil {
			return err
		}
		o.cluster = c
	}
	if o.cluster.Spec.Backup == nil {
		o.cluster.Spec.Backup = &kbappsv1.ClusterBackup{}
	}

	switch field {
	case "enabled":
		return o.updateBackupEnabled(val)
	case "retentionPeriod":
		return o.updateBackupRetentionPeriod(val)
	case "method":
		return o.updateBackupMethod(val)
	case "cronExpression":
		return o.updateBackupCronExpression(val)
	case "startingDeadlineMinutes":
		return o.updateBackupStartingDeadlineMinutes(val)
	case "repoName":
		return o.updateBackupRepoName(val)
	case "pitrEnabled":
		return o.updateBackupPitrEnabled(val)
	default:
		return nil
	}
}

func (o *UpdateOptions) updateMonitor(val string) error {
	disableExporter, err := strconv.ParseBool(val)
	if err != nil {
		return err
	}

	for i := range o.cluster.Spec.ComponentSpecs {
		o.cluster.Spec.ComponentSpecs[i].DisableExporter = cfgutil.ToPointer(disableExporter)
	}
	for i := range o.cluster.Spec.Shardings {
		o.cluster.Spec.Shardings[i].Template.DisableExporter = cfgutil.ToPointer(disableExporter)
	}
	return nil
}

func (o *UpdateOptions) updateBackupEnabled(val string) error {
	boolVal, err := strconv.ParseBool(val)
	if err != nil {
		return err
	}
	o.cluster.Spec.Backup.Enabled = &boolVal
	return nil
}

func (o *UpdateOptions) updateBackupRetentionPeriod(val string) error {
	// if val is empty, do nothing
	if len(val) == 0 {
		return nil
	}

	// judge whether val end with the 'd'|'h' character
	lastChar := val[len(val)-1]
	if lastChar != 'd' && lastChar != 'h' {
		return fmt.Errorf("invalid retention period: %s, only support d|h", val)
	}

	o.cluster.Spec.Backup.RetentionPeriod = dpv1alpha1.RetentionPeriod(val)
	return nil
}

func (o *UpdateOptions) updateBackupMethod(val string) error {
	// TODO(ldm): validate backup method are defined in the backup policy.
	o.cluster.Spec.Backup.Method = val
	return nil
}

func (o *UpdateOptions) updateBackupCronExpression(val string) error {
	// judge whether val is a valid cron expression
	if _, err := cron.ParseStandard(val); err != nil {
		return fmt.Errorf("invalid cron expression: %s, please see https://en.wikipedia.org/wiki/Cron", val)
	}

	o.cluster.Spec.Backup.CronExpression = val
	return nil
}

func (o *UpdateOptions) updateBackupStartingDeadlineMinutes(val string) error {
	intVal, err := strconv.ParseInt(val, 10, 64)
	if err != nil {
		return err
	}
	o.cluster.Spec.Backup.StartingDeadlineMinutes = &intVal
	return nil
}

func (o *UpdateOptions) updateBackupRepoName(val string) error {
	o.cluster.Spec.Backup.RepoName = val
	return nil
}

func (o *UpdateOptions) updateBackupPitrEnabled(val string) error {
	boolVal, err := strconv.ParseBool(val)
	if err != nil {
		return err
	}
	o.cluster.Spec.Backup.PITREnabled = &boolVal
	return nil
}
