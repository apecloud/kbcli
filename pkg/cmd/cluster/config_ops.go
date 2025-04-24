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
	"fmt"
	"os"
	"strings"

	opsv1alpha1 "github.com/apecloud/kubeblocks/apis/operations/v1alpha1"
	parametersv1alpha1 "github.com/apecloud/kubeblocks/apis/parameters/v1alpha1"
	"github.com/apecloud/kubeblocks/pkg/client/clientset/versioned"
	cfgcm "github.com/apecloud/kubeblocks/pkg/configuration/config_manager"
	"github.com/apecloud/kubeblocks/pkg/configuration/core"
	configctrl "github.com/apecloud/kubeblocks/pkg/controller/configuration"
	"github.com/apecloud/kubeblocks/pkg/controllerutil"
	"github.com/apecloud/kubeblocks/pkg/generics"
	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/kubectl/pkg/util/templates"

	"github.com/apecloud/kbcli/pkg/printer"
	"github.com/apecloud/kbcli/pkg/types"
	"github.com/apecloud/kbcli/pkg/util"
	"github.com/apecloud/kbcli/pkg/util/prompt"
)

type configOpsOptions struct {
	*OperationsOptions

	editMode bool
	wrapper  *ReconfigureWrapper

	// config file replace
	replaceFile bool

	// Reconfiguring options
	ComponentName string
	LocalFilePath string   `json:"localFilePath"`
	Parameters    []string `json:"parameters"`

	clientSet versioned.Interface
}

var (
	createReconfigureExample = templates.Examples(`
		# update component params 
		kbcli cluster configure mycluster --components=mysql --config-spec=mysql-3node-tpl --config-file=my.cnf --set=max_connections=1000,general_log=OFF

		# if only one component, and one config spec, and one config file, simplify the searching process of configure. e.g:
		# update mysql max_connections, cluster name is mycluster
		kbcli cluster configure mycluster --set max_connections=2000
	`)
)

func (o *configOpsOptions) Complete() error {
	if o.Name == "" {
		return makeMissingClusterNameErr()
	}

	if len(o.ComponentNames) > 0 {
		o.ComponentName = o.ComponentNames[0]
	}

	if !o.editMode {
		if err := o.validateReconfigureOptions(); err != nil {
			return err
		}
	}

	client := o.clientSet
	if client == nil {
		client = GetClientFromOptionsOrDie(o.Factory)
	}
	wrapper, err := newConfigWrapper(client, o.Namespace, o.Name, o.ComponentName, o.CfgTemplateName, o.CfgFile, o.KeyValues)
	if err != nil {
		return err
	}

	o.wrapper = wrapper
	// return wrapper.AutoFillRequiredParam()
	return nil
}

func (o *configOpsOptions) validateReconfigureOptions() error {
	if o.LocalFilePath != "" && o.CfgFile == "" {
		return core.MakeError("config file is required when using --local-file")
	}
	if o.LocalFilePath != "" {
		b, err := os.ReadFile(o.LocalFilePath)
		if err != nil {
			return err
		}
		o.FileContent = string(b)
	} else {
		kvs, err := o.parseUpdatedParams()
		if err != nil {
			return err
		}
		o.KeyValues = core.FromStringPointerMap(kvs)
	}
	return nil
}

// Validate command flags or args is legal
func (o *configOpsOptions) Validate() error {
	o.CfgFile = o.wrapper.ConfigFile()
	o.CfgTemplateName = o.wrapper.ConfigSpecName()
	if len(o.ComponentNames) == 0 {
		o.ComponentNames = []string{o.wrapper.ComponentName()}
	}

	if o.editMode {
		return nil
	}

	rctx := o.wrapper.rctx
	tplObjs, err := resolveConfigTemplate(rctx, o.Dynamic)
	if err != nil {
		return err
	}

	classifyParams, err := configctrl.ClassifyComponentParameters(o.KeyValues, rctx.ParametersDefs, rctx.Cmpd.Spec.Configs, tplObjs, rctx.ConfigRender)
	if err != nil {
		return err
	}
	if err := util.ValidateParametersModified(classifyParams, rctx.ParametersDefs); err != nil {
		return err
	}

	if err := o.validateConfigParams(o.wrapper.rctx, classifyParams); err != nil {
		return err
	}

	o.printConfigureTips(classifyParams)
	return nil
}

func (o *configOpsOptions) validateConfigParams(rctx *ReconfigureContext, classifyParameters map[string]map[string]*parametersv1alpha1.ParametersInFile) error {
	if o.FileContent != "" {
		return o.confirmReconfigureWithRestart()
	}

	checkRestart := func(params map[string]*parametersv1alpha1.ParametersInFile) bool {
		for file := range params {
			match := func(pd *parametersv1alpha1.ParametersDefinition) bool {
				return pd.Spec.FileName == file && cfgcm.IsSupportReload(pd.Spec.ReloadAction)
			}
			if generics.FindFirstFunc(rctx.ParametersDefs, match) < 0 {
				return true
			}
		}
		return false
	}

	transform := func(params map[string]*parametersv1alpha1.ParametersInFile) []core.ParamPairs {
		var result []core.ParamPairs
		for file, ps := range params {
			result = append(result, core.ParamPairs{
				Key:           file,
				UpdatedParams: core.FromStringMap(ps.Parameters),
			})
		}
		return result
	}

	restart := false
	for _, parameters := range classifyParameters {
		_, err := controllerutil.MergeAndValidateConfigs(mockEmptyData(parameters), transform(parameters), rctx.ParametersDefs, rctx.ConfigRender.Spec.Configs)
		if err != nil {
			return err
		}
		if !restart {
			restart = checkRestart(parameters)
		}
	}

	if restart {
		return o.confirmReconfigureWithRestart()
	}
	return nil
}

func mockEmptyData(m map[string]*parametersv1alpha1.ParametersInFile) map[string]string {
	r := make(map[string]string, len(m))
	for key := range m {
		r[key] = ""
	}
	return r
}

func (o *configOpsOptions) confirmReconfigureWithRestart() error {
	if o.AutoApprove {
		return nil
	}
	const confirmStr = "yes"
	printer.Warning(o.Out, restartConfirmPrompt)
	_, err := prompt.NewPrompt(fmt.Sprintf("Please type \"%s\" to confirm:", confirmStr),
		func(input string) error {
			if input != confirmStr {
				return fmt.Errorf("typed \"%s\" not match \"%s\"", input, confirmStr)
			}
			return nil
		}, o.In).Run()
	return err
}

func (o *configOpsOptions) parseUpdatedParams() (map[string]string, error) {
	if len(o.Parameters) == 0 && len(o.LocalFilePath) == 0 {
		return nil, core.MakeError(missingUpdatedParametersErrMessage)
	}

	keyValues := make(map[string]string)
	for _, param := range o.Parameters {
		pp := strings.Split(param, ",")
		for _, p := range pp {
			fields := strings.SplitN(p, "=", 2)
			if len(fields) != 2 {
				return nil, core.MakeError("updated parameter format: key=value")
			}
			keyValues[fields[0]] = fields[1]
		}
	}
	return keyValues, nil
}

func (o *configOpsOptions) printConfigureTips(classifyParameters map[string]map[string]*parametersv1alpha1.ParametersInFile) {
	fmt.Println("Will updated configure file meta:")
	for tpl, tplParams := range classifyParameters {
		for file := range tplParams {
			printer.PrintLineWithTabSeparator(
				printer.NewPair("  ConfigSpec", printer.BoldYellow(tpl)),
				printer.NewPair("  ConfigFile", printer.BoldYellow(file)),
				printer.NewPair("ComponentName", o.ComponentName),
				printer.NewPair("ClusterName", o.Name))
		}
	}
}

// buildReconfigureCommonFlags build common flags for reconfigure command
func (o *configOpsOptions) buildReconfigureCommonFlags(cmd *cobra.Command, f cmdutil.Factory) {
	o.addCommonFlags(cmd, f)
	cmd.Flags().StringSliceVar(&o.Parameters, "set", nil, "Specify parameters list to be updated. For more details, refer to 'kbcli cluster describe-config'.")
	cmd.Flags().StringVar(&o.CfgTemplateName, "config-spec", "", "Specify the name of the configuration template to be updated (e.g. for apecloud-mysql: --config-spec=mysql-3node-tpl). "+
		"For available templates and configs, refer to: 'kbcli cluster describe-config'.")
	cmd.Flags().StringVar(&o.CfgFile, "config-file", "", "Specify the name of the configuration file to be updated (e.g. for mysql: --config-file=my.cnf). "+
		"For available templates and configs, refer to: 'kbcli cluster describe-config'.")
	// flags.AddComponentFlag(f, cmd, &o.ComponentName, "Specify the name of Component to be updated. If the cluster has only one component, unset the parameter.")
	cmd.Flags().BoolVar(&o.ForceRestart, "force-restart", false, "Boolean flag to restart component. Default with false.")
	cmd.Flags().StringVar(&o.LocalFilePath, "local-file", "", "Specify the local configuration file to be updated.")
	cmd.Flags().BoolVar(&o.replaceFile, "replace", false, "Boolean flag to enable replacing config file. Default with false.")
}

// NewReconfigureCmd creates a Reconfiguring command
func NewReconfigureCmd(f cmdutil.Factory, streams genericiooptions.IOStreams) *cobra.Command {
	o := &configOpsOptions{
		editMode:          false,
		OperationsOptions: newBaseOperationsOptions(f, streams, opsv1alpha1.ReconfiguringType, true),
	}
	cmd := &cobra.Command{
		Use:               "configure NAME --set key=value[,key=value] [--components=component1-name,component2-name] [--config-spec=config-spec-name] [--config-file=config-file]",
		Short:             "Configure parameters with the specified components in the cluster.",
		Example:           createReconfigureExample,
		ValidArgsFunction: util.ResourceNameCompletionFunc(f, types.ClusterGVR()),
		Run: func(cmd *cobra.Command, args []string) {
			o.Args = args
			cmdutil.BehaviorOnFatal(printer.FatalWithRedColor)
			cmdutil.CheckErr(o.CreateOptions.Complete())
			cmdutil.CheckErr(o.Complete())
			cmdutil.CheckErr(o.Validate())
			cmdutil.CheckErr(o.Run())
		},
	}

	o.buildReconfigureCommonFlags(cmd, f)
	cmd.Flags().BoolVar(&o.AutoApprove, "auto-approve", false, "Skip interactive approval before reconfiguring the cluster")
	return cmd
}
