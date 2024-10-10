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

package cluster

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"text/template"

	kbappsv1 "github.com/apecloud/kubeblocks/apis/apps/v1"
	opsv1alpha1 "github.com/apecloud/kubeblocks/apis/operations/v1alpha1"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/robfig/cron/v3"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	"k8s.io/client-go/dynamic"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/kubectl/pkg/util/templates"

	appsv1alpha1 "github.com/apecloud/kubeblocks/apis/apps/v1alpha1"
	appsv1beta1 "github.com/apecloud/kubeblocks/apis/apps/v1beta1"
	dpv1alpha1 "github.com/apecloud/kubeblocks/apis/dataprotection/v1alpha1"
	cfgcore "github.com/apecloud/kubeblocks/pkg/configuration/core"
	cfgutil "github.com/apecloud/kubeblocks/pkg/configuration/util"
	"github.com/apecloud/kubeblocks/pkg/constant"
	"github.com/apecloud/kubeblocks/pkg/controller/configuration"
	"github.com/apecloud/kubeblocks/pkg/dataprotection/utils"
	"github.com/apecloud/kubeblocks/pkg/gotemplate"

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

    # enable all logs
	kbcli cluster update mycluster --enable-all-logs=true

    # update cluster topology keys and affinity
	kbcli cluster update mycluster --topology-keys=kubernetes.io/hostname --pod-anti-affinity=Required

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
	affinity := map[string]interface{}{}
	type filedObj struct {
		field string
		obj   map[string]interface{}
		fn    buildFn
	}

	flagFieldMapping := map[string]*filedObj{
		"termination-policy": {field: "terminationPolicy", obj: spec, fn: buildFlagObj},
		"pod-anti-affinity":  {field: "podAntiAffinity", obj: affinity, fn: buildFlagObj},
		"topology-keys":      {field: "topologyKeys", obj: affinity, fn: buildFlagObj},
		"node-labels":        {field: "nodeLabels", obj: affinity, fn: buildFlagObj},
		"tenancy":            {field: "tenancy", obj: affinity, fn: buildFlagObj},

		// tolerations
		"tolerations": {field: "tolerations", obj: spec, fn: buildTolObj},

		// monitor and logs
		"disable-exporter": {field: "disableExporter", obj: nil, fn: buildComps},
		"enable-all-logs":  {field: "enable-all-logs", obj: nil, fn: buildComps},

		// backup config
		"backup-enabled":                   {field: "enabled", obj: nil, fn: buildBackup},
		"backup-retention-period":          {field: "retentionPeriod", obj: nil, fn: buildBackup},
		"backup-method":                    {field: "method", obj: nil, fn: buildBackup},
		"backup-cron-expression":           {field: "cronExpression", obj: nil, fn: buildBackup},
		"backup-starting-deadline-minutes": {field: "startingDeadlineMinutes", obj: nil, fn: buildBackup},
		"backup-repo-name":                 {field: "repoName", obj: nil, fn: buildBackup},
		"pitr-enabled":                     {field: "pitrEnabled", obj: nil, fn: buildBackup},
	}

	for name, val := range o.ValMap {
		if f, ok := flagFieldMapping[name]; ok {
			if err = f.fn(f.obj, val, f.field); err != nil {
				return err
			}
		}
	}

	if len(affinity) > 0 {
		if err = unstructured.SetNestedField(spec, affinity, "affinity"); err != nil {
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
	case "enable-all-logs":
		return o.updateEnabledLog(val)
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

func (o *UpdateOptions) updateEnabledLog(val string) error {
	boolVal, err := strconv.ParseBool(val)
	if err != nil {
		return err
	}

	// update --enabled-all-logs=false for all components
	if !boolVal {
		// TODO: replace with new api
		/*	for index := range o.cluster.Spec.ComponentSpecs {
			o.cluster.Spec.ComponentSpecs[index].EnabledLogs = nil
		}*/
		return nil
	}

	// update --enabled-all-logs=true for all components
	cd, err := cluster.GetClusterDefByName(o.dynamic, o.cluster.Spec.ClusterDef)
	if err != nil {
		return err
	}
	// TODO: replace with new api
	// set --enabled-all-logs at cluster components
	// setEnableAllLogs(o.cluster, cd)
	if err = o.reconfigureLogVariables(o.cluster, cd); err != nil {
		return errors.Wrap(err, "failed to reconfigure log variables of target cluster")
	}
	return nil
}

const logsBlockName = "logsBlock"
const logsTemplateName = "template-logs-block"
const topTPLLogsObject = "component"
const defaultSectionName = "default"

// reconfigureLogVariables reconfigures the log variables of cluster
func (o *UpdateOptions) reconfigureLogVariables(c *kbappsv1.Cluster, cd *kbappsv1.ClusterDefinition) error {
	var (
		err        error
		configSpec *appsv1alpha1.ComponentConfigSpec
		logValue   *gotemplate.TplValues
	)

	createReconfigureOps := func(compSpec kbappsv1.ClusterComponentSpec, configSpec *appsv1alpha1.ComponentConfigSpec, logValue *gotemplate.TplValues) error {
		var (
			buf             bytes.Buffer
			keyName         string
			configTemplate  *corev1.ConfigMap
			formatter       *appsv1beta1.FileFormatConfig
			logTPL          *template.Template
			logVariables    map[string]string
			unstructuredObj *unstructured.Unstructured
		)

		if configTemplate, formatter, err = findConfigTemplateInfo(o.dynamic, configSpec); err != nil {
			return err
		}
		if keyName, logTPL, err = findLogsBlockTPL(configTemplate.Data); err != nil {
			return err
		}
		if logTPL == nil {
			return nil
		}
		if err = logTPL.Execute(&buf, logValue); err != nil {
			return err
		}
		// TODO: very hack logic for ini config file
		formatter.FormatterAction = appsv1beta1.FormatterAction{IniConfig: &appsv1beta1.IniConfig{SectionName: defaultSectionName}}
		if logVariables, err = cfgcore.TransformConfigFileToKeyValueMap(keyName, formatter, buf.Bytes()); err != nil {
			return err
		}
		// build OpsRequest and apply this OpsRequest
		opsRequest := buildLogsReconfiguringOps(c.Name, c.Namespace, compSpec.Name, configSpec.Name, keyName, logVariables)
		if unstructuredObj, err = util.ConvertObjToUnstructured(opsRequest); err != nil {
			return err
		}
		return util.CreateResourceIfAbsent(o.dynamic, types.OpsGVR(), c.Namespace, unstructuredObj)
	}

	for _, compSpec := range c.Spec.ComponentSpecs {
		if configSpec, err = findFirstConfigSpec(o.dynamic, compSpec.Name, compSpec.ComponentDef); err != nil {
			return err
		}
		if logValue, err = buildLogsTPLValues(&compSpec); err != nil {
			return err
		}
		if err = createReconfigureOps(compSpec, configSpec, logValue); err != nil {
			return err
		}
	}
	return nil
}

func findFirstConfigSpec(
	cli dynamic.Interface,
	compName string,
	compDefName string) (*appsv1alpha1.ComponentConfigSpec, error) {
	compDef, err := util.GetComponentDefByName(cli, compDefName)
	if err != nil {
		return nil, err
	}
	configSpecs, err := util.GetValidConfigSpecs(true, util.ToV1ComponentConfigSpecs(compDef.Spec.Configs))
	if err != nil {
		return nil, err
	}
	if len(configSpecs) == 0 {
		return nil, errors.Errorf("no config templates for component %s", compName)
	}
	return &configSpecs[0], nil
}

func findConfigTemplateInfo(dynamic dynamic.Interface, configSpec *appsv1alpha1.ComponentConfigSpec) (*corev1.ConfigMap, *appsv1beta1.FileFormatConfig, error) {
	if configSpec == nil {
		return nil, nil, errors.New("configTemplateSpec is nil")
	}
	configTemplate, err := cluster.GetConfigMapByName(dynamic, configSpec.Namespace, configSpec.TemplateRef)
	if err != nil {
		return nil, nil, err
	}
	configConstraint, err := cluster.GetConfigConstraintByName(dynamic, configSpec.ConfigConstraintRef)
	if err != nil {
		return nil, nil, err
	}
	return configTemplate, configConstraint.Spec.FileFormatConfig, nil
}

func newConfigTemplateEngine() *template.Template {
	customizedFuncMap := configuration.BuiltInCustomFunctions(nil, nil, nil)
	engine := gotemplate.NewTplEngine(nil, customizedFuncMap, logsTemplateName, nil, context.TODO())
	return engine.GetTplEngine()
}

func findLogsBlockTPL(confData map[string]string) (string, *template.Template, error) {
	engine := newConfigTemplateEngine()
	for key, value := range confData {
		if !strings.Contains(value, logsBlockName) {
			continue
		}
		tpl, err := engine.Parse(value)
		if err != nil {
			return key, nil, err
		}
		logTPL := tpl.Lookup(logsBlockName)
		// find target logs template
		if logTPL != nil {
			return key, logTPL, nil
		}
		return "", nil, errors.New("no logs config template found")
	}
	return "", nil, nil
}

func buildLogsTPLValues(compSpec *kbappsv1.ClusterComponentSpec) (*gotemplate.TplValues, error) {
	compMap := map[string]interface{}{}
	bytesData, err := json.Marshal(compSpec)
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(bytesData, &compMap)
	if err != nil {
		return nil, err
	}
	value := gotemplate.TplValues{
		topTPLLogsObject: compMap,
	}
	return &value, nil
}

func buildLogsReconfiguringOps(clusterName, namespace, compName, configName, keyName string, variables map[string]string) *opsv1alpha1.OpsRequest {
	opsName := fmt.Sprintf("%s-%s", "logs-reconfigure", uuid.NewString())
	opsRequest := util.NewOpsRequestForReconfiguring(opsName, namespace, clusterName)
	parameterPairs := make([]opsv1alpha1.ParameterPair, 0, len(variables))
	for key, value := range variables {
		v := value
		parameterPairs = append(parameterPairs, opsv1alpha1.ParameterPair{
			Key:   key,
			Value: &v,
		})
	}
	var keys []opsv1alpha1.ParameterConfig
	keys = append(keys, opsv1alpha1.ParameterConfig{
		Key:        keyName,
		Parameters: parameterPairs,
	})
	var configurations []opsv1alpha1.ConfigurationItem
	configurations = append(configurations, opsv1alpha1.ConfigurationItem{
		Keys: keys,
		Name: configName,
	})
	reconfigure := opsRequest.Spec.Reconfigures[0]
	reconfigure.ComponentName = compName
	reconfigure.Configurations = append(reconfigure.Configurations, configurations...)
	return opsRequest
}

func (o *UpdateOptions) updateMonitor(val string) error {
	disableExporter, err := strconv.ParseBool(val)
	if err != nil {
		return err
	}

	for i := range o.cluster.Spec.ComponentSpecs {
		o.cluster.Spec.ComponentSpecs[i].DisableExporter = cfgutil.ToPointer(disableExporter)
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
