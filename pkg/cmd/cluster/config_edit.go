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
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"

	opsv1alpha1 "github.com/apecloud/kubeblocks/apis/operations/v1alpha1"
	parametersv1alpha1 "github.com/apecloud/kubeblocks/apis/parameters/v1alpha1"
	cfgcm "github.com/apecloud/kubeblocks/pkg/configuration/config_manager"
	"github.com/apecloud/kubeblocks/pkg/configuration/core"
	"github.com/apecloud/kubeblocks/pkg/configuration/validate"
	intctrlutil "github.com/apecloud/kubeblocks/pkg/controllerutil"
	"github.com/spf13/cobra"
	"golang.org/x/exp/slices"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/kubectl/pkg/cmd/util/editor"
	"k8s.io/kubectl/pkg/util/templates"

	"github.com/apecloud/kbcli/pkg/printer"
	"github.com/apecloud/kbcli/pkg/types"
	"github.com/apecloud/kbcli/pkg/util"
	"github.com/apecloud/kbcli/pkg/util/prompt"
)

type editConfigOptions struct {
	configOpsOptions

	enableDelete bool
}

var (
	editConfigUse = "edit-config NAME [--components=component-name] [--config-spec=config-spec-name] [--config-file=config-file]"

	editConfigExample = templates.Examples(`
		# update mysql max_connections, cluster name is mycluster
		kbcli cluster edit-config mycluster
	`)
)

func (o *editConfigOptions) Run(fn func() error) error {
	wrapper := o.wrapper
	cfgEditContext := newConfigContext(o.CreateOptions, o.CreateOptions.Name, wrapper.ComponentName(), wrapper.ConfigSpecName(), wrapper.ConfigFile())
	if err := cfgEditContext.prepare(); err != nil {
		return err
	}
	reader, err := o.getReaderWrapper()
	if err != nil {
		return err
	}

	editor := editor.NewDefaultEditor([]string{
		"KUBE_EDITOR",
		"EDITOR",
	})
	if err := cfgEditContext.editConfig(editor, reader); err != nil {
		return err
	}

	diff, err := util.GetUnifiedDiffString(cfgEditContext.original, cfgEditContext.edited, "Original", "Current", 3)
	if err != nil {
		return err
	}
	if diff == "" {
		fmt.Println("Edit cancelled, no changes made.")
		return nil
	}
	util.DisplayDiffWithColor(o.CreateOptions.IOStreams.Out, diff)

	if hasSchemaForFile(wrapper.rctx, wrapper.ConfigFile()) {
		return o.runWithConfigConstraints(cfgEditContext, wrapper.rctx, fn)
	}

	yes, err := o.confirmReconfigure(fmt.Sprintf(fullRestartConfirmPrompt, printer.BoldRed(o.CfgFile)))
	if err != nil {
		return err
	}
	if !yes {
		return nil
	}

	o.HasPatch = false
	o.FileContent = cfgEditContext.getEdited()
	return fn()
}

func hasSchemaForFile(rctx *ReconfigureContext, configFile string) bool {
	if rctx.ConfigRender == nil {
		return false
	}
	return intctrlutil.GetComponentConfigDescription(&rctx.ConfigRender.Spec, configFile) != nil
}

func (o *editConfigOptions) runWithConfigConstraints(cfgEditContext *configEditContext, rctx *ReconfigureContext, fn func() error) error {
	oldVersion := map[string]string{
		o.CfgFile: cfgEditContext.getOriginal(),
	}
	newVersion := map[string]string{
		o.CfgFile: cfgEditContext.getEdited(),
	}

	configPatch, fileUpdated, err := core.CreateConfigPatch(oldVersion, newVersion, rctx.ConfigRender.Spec, true)
	if err != nil {
		return err
	}
	if !fileUpdated && !configPatch.IsModify {
		fmt.Println("No parameters changes made.")
		return nil
	}

	fmt.Fprintf(o.CreateOptions.Out, "Config patch(updated parameters): \n%s\n\n", string(configPatch.UpdateConfig[o.CfgFile]))
	if !o.enableDelete {
		if err := core.ValidateConfigPatch(configPatch, rctx.ConfigRender.Spec); err != nil {
			return err
		}
	}

	params := core.GenerateVisualizedParamsList(configPatch, rctx.ConfigRender.Spec.Configs)
	o.KeyValues = fromKeyValuesToMap(params, o.CfgFile)
	// check immutable parameters
	if err = util.ValidateParametersModified2(sets.KeySet(fromKeyValuesToMap(params, o.CfgFile)), rctx.ParametersDefs, o.CfgFile); err != nil {
		return err
	}

	var config *parametersv1alpha1.ComponentConfigDescription
	if config = intctrlutil.GetComponentConfigDescription(&rctx.ConfigRender.Spec, o.CfgFile); config == nil {
		return fn()
	}
	var pd *parametersv1alpha1.ParametersDefinition
	for _, paramsDef := range rctx.ParametersDefs {
		if paramsDef.Spec.FileName == o.CfgFile {
			pd = paramsDef
			break
		}
	}

	confirmPrompt, err := generateReconfiguringPrompt(fileUpdated, configPatch, pd, o.CfgFile, config.FileFormatConfig)
	if err != nil {
		return err
	}
	yes, err := o.confirmReconfigure(confirmPrompt)
	if err != nil {
		return err
	}
	if !yes {
		return nil
	}

	if pd != nil && pd.Spec.ParametersSchema != nil {
		if err = validate.NewConfigValidator(pd.Spec.ParametersSchema, config.FileFormatConfig).Validate(cfgEditContext.getEdited()); err != nil {
			return core.WrapError(err, "failed to validate edited config")
		}
	}
	return fn()
}

func generateReconfiguringPrompt(fileUpdated bool, configPatch *core.ConfigPatchInfo, pd *parametersv1alpha1.ParametersDefinition, fileName string, config *parametersv1alpha1.FileFormatConfig) (string, error) {
	if fileUpdated || pd == nil {
		return restartConfirmPrompt, nil
	}

	dynamicUpdated, err := core.IsUpdateDynamicParameters(config, &pd.Spec, configPatch)
	if err != nil {
		return "", nil
	}

	confirmPrompt := confirmApplyReconfigurePrompt
	if !dynamicUpdated || !cfgcm.IsSupportReload(pd.Spec.ReloadAction) {
		confirmPrompt = restartConfirmPrompt
	}
	return confirmPrompt, nil
}

func (o *editConfigOptions) confirmReconfigure(promptStr string) (bool, error) {
	const yesStr = "yes"
	const noStr = "no"

	confirmStr := []string{yesStr, noStr}
	printer.Warning(o.CreateOptions.Out, "%s", promptStr)
	input, err := prompt.NewPrompt("Please type [Yes/No] to confirm:",
		func(input string) error {
			if !slices.Contains(confirmStr, strings.ToLower(input)) {
				return fmt.Errorf("typed \"%s\" does not match \"%s\"", input, confirmStr)
			}
			return nil
		}, o.CreateOptions.In).Run()
	if err != nil {
		return false, err
	}
	return strings.ToLower(input) == yesStr, nil
}

func (o *editConfigOptions) getReaderWrapper() (io.Reader, error) {
	var reader io.Reader
	if o.replaceFile && o.LocalFilePath != "" {
		b, err := os.ReadFile(o.LocalFilePath)
		if err != nil {
			return nil, err
		}
		reader = bytes.NewReader(b)
	}
	return reader, nil
}

// NewEditConfigureCmd shows the difference between two configuration version.
func NewEditConfigureCmd(f cmdutil.Factory, streams genericiooptions.IOStreams) *cobra.Command {
	o := &editConfigOptions{
		configOpsOptions: configOpsOptions{
			editMode:          true,
			OperationsOptions: newBaseOperationsOptions(f, streams, opsv1alpha1.ReconfiguringType, true),
		}}

	cmd := &cobra.Command{
		Use:               editConfigUse,
		Short:             "Edit the config file of the component.",
		Example:           editConfigExample,
		ValidArgsFunction: util.ResourceNameCompletionFunc(f, types.ClusterGVR()),
		Run: func(cmd *cobra.Command, args []string) {
			o.CreateOptions.Args = args
			cmdutil.CheckErr(o.CreateOptions.Complete())
			util.CheckErr(o.Complete())
			util.CheckErr(o.Validate())
			util.CheckErr(o.Run(o.CreateOptions.Run))
		},
	}
	o.buildReconfigureCommonFlags(cmd, f)
	cmd.Flags().BoolVar(&o.enableDelete, "enable-delete", false, "Boolean flag to enable delete configuration. Default with false.")
	return cmd
}
