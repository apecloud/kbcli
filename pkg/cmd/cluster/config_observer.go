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
	"fmt"

	parametersv1alpha1 "github.com/apecloud/kubeblocks/apis/parameters/v1alpha1"
	intctrlutil "github.com/apecloud/kubeblocks/pkg/controllerutil"
	"github.com/spf13/cobra"
	apiext "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/kubectl/pkg/util/templates"

	cfgcore "github.com/apecloud/kubeblocks/pkg/configuration/core"
	"github.com/apecloud/kubeblocks/pkg/configuration/openapi"
	cfgutil "github.com/apecloud/kubeblocks/pkg/configuration/util"

	"github.com/apecloud/kbcli/pkg/printer"
	"github.com/apecloud/kbcli/pkg/types"
	"github.com/apecloud/kbcli/pkg/util"
	"github.com/apecloud/kbcli/pkg/util/flags"
)

type configObserverOptions struct {
	*describeOpsOptions

	clusterName    string
	componentNames []string
	configSpecs    []string

	isExplain     bool
	truncEnum     bool
	truncDocument bool
	paramName     string

	keys []string
}

var (
	describeReconfigureExample = templates.Examples(`
		# describe a cluster, e.g. cluster name is mycluster
		kbcli cluster describe-config mycluster

		# describe a component, e.g. cluster name is mycluster, component name is mysql
		kbcli cluster describe-config mycluster --component=mysql

		# describe all configuration files.
		kbcli cluster describe-config mycluster --component=mysql --show-detail

		# describe a content of configuration file.
		kbcli cluster describe-config mycluster --component=mysql --config-file=my.cnf --show-detail`)
	explainReconfigureExample = templates.Examples(`
		# explain a cluster, e.g. cluster name is mycluster
		kbcli cluster explain-config mycluster

		# explain a specified configure template, e.g. cluster name is mycluster
		kbcli cluster explain-config mycluster --component=mysql --config-specs=mysql-3node-tpl

		# explain a specified configure template, e.g. cluster name is mycluster
		kbcli cluster explain-config mycluster --component=mysql --config-specs=mysql-3node-tpl --trunc-document=false --trunc-enum=false

		# explain a specified parameters, e.g. cluster name is mycluster
		kbcli cluster explain-config mycluster --param=sql_mode`)
)

func (r *configObserverOptions) addCommonFlags(cmd *cobra.Command, f cmdutil.Factory) {
	cmd.Flags().StringSliceVar(&r.configSpecs, "config-specs", nil, "Specify the name of the configuration template to describe. (e.g. for apecloud-mysql: --config-specs=mysql-3node-tpl)")
	flags.AddComponentsFlag(f, cmd, &r.componentNames, "Specify the name of Component to describe (e.g. for apecloud-mysql: --component=mysql). If the cluster has only one component, unset the parameter.\"")
}

func (r *configObserverOptions) complete2(args []string) error {
	if len(args) == 0 {
		return makeMissingClusterNameErr()
	}
	r.clusterName = args[0]
	return r.complete(args)
}

func (r *configObserverOptions) run(printFn func(*ReconfigureContext) error) error {
	wrapper, err := New(r.clusterName, r.namespace, r.describeOpsOptions, r.componentNames...)
	if err != nil {
		return err
	}
	for _, rctx := range wrapper.rctxMap {
		fmt.Fprintf(r.Out, "component: %s\n", rctx.CompName)
		if err := printFn(rctx); err != nil {
			return err
		}
	}
	return nil
}

func (r *configObserverOptions) printComponentConfigSpecsDescribe(rctx *ReconfigureContext) error {
	resolveParameterTemplate := func(tpl string) string {
		for _, config := range rctx.Cmpd.Spec.Configs {
			if config.Name == tpl {
				return config.TemplateRef
			}
		}
		return ""
	}

	if rctx.ConfigRender == nil || len(rctx.ConfigRender.Spec.Configs) == 0 {
		return nil
	}
	tbl := printer.NewTablePrinter(r.Out)
	printer.PrintTitle("ConfigSpecs Meta")
	tbl.SetHeader("CONFIG-SPEC-NAME", "FILE", "TEMPLATE-NAME", "COMPONENT", "CLUSTER")
	for _, info := range rctx.ConfigRender.Spec.Configs {
		tbl.AddRow(
			printer.BoldYellow(info.TemplateName),
			printer.BoldYellow(info.Name),
			printer.BoldYellow(resolveParameterTemplate(info.TemplateName)),
			rctx.CompName,
			rctx.Cluster.Name)
	}
	tbl.Print()
	return nil
}

func (r *configObserverOptions) printComponentExplainConfigure(rctx *ReconfigureContext) error {
	for _, pd := range rctx.ParametersDefs {
		if rctx.ConfigRender != nil {
			config := intctrlutil.GetComponentConfigDescription(&rctx.ConfigRender.Spec, pd.Spec.FileName)
			if config != nil {
				fmt.Println("template meta:")
				printer.PrintLineWithTabSeparator(
					printer.NewPair("  FileName", pd.Spec.FileName),
					printer.NewPair("  ConfigSpec", config.TemplateName),
					printer.NewPair("ComponentName", rctx.CompName),
					printer.NewPair("ClusterName", r.clusterName),
				)
			}
		}
		if err := r.printExplainConfigure(&pd.Spec); err != nil {
			return err
		}
	}
	return nil
}

func (r *configObserverOptions) printExplainConfigure(pdSpec *parametersv1alpha1.ParametersDefinitionSpec) error {
	if pdSpec.ParametersSchema == nil {
		fmt.Printf("\n%s\n", fmt.Sprintf(notConfigSchemaPrompt, printer.BoldYellow(pdSpec.FileName)))
		return nil
	}

	schema := pdSpec.ParametersSchema.DeepCopy()
	if schema.SchemaInJSON == nil {
		if schema.CUE == "" {
			fmt.Printf("\n%s\n", fmt.Sprintf(notConfigSchemaPrompt, printer.BoldYellow(pdSpec.FileName)))
			return nil
		}
		apiSchema, err := openapi.GenerateOpenAPISchema(schema.CUE, schema.TopLevelKey)
		if err != nil {
			return cfgcore.WrapError(err, "failed to generate open api schema")
		}
		if apiSchema == nil {
			fmt.Printf("\n%s\n", cue2openAPISchemaFailedPrompt)
			return nil
		}
		schema.SchemaInJSON = apiSchema
	}
	return r.printConfigConstraint(schema.SchemaInJSON,
		cfgutil.NewSet(pdSpec.StaticParameters...),
		cfgutil.NewSet(pdSpec.DynamicParameters...),
		cfgutil.NewSet(pdSpec.ImmutableParameters...))
}

func (r *configObserverOptions) hasSpecificParam() bool {
	return len(r.paramName) != 0
}

func (r *configObserverOptions) isSpecificParam(paramName string) bool {
	return r.paramName == paramName
}

func (r *configObserverOptions) printConfigConstraint(schema *apiext.JSONSchemaProps, staticParameters, dynamicParameters, immutableParameters *cfgutil.Sets) error {
	var (
		maxDocumentLength = 100
		maxEnumLength     = 20
		spec              = schema.Properties[openapi.DefaultSchemaName]
		params            = make([]*parameterSchema, 0)
	)

	for key, property := range openapi.FlattenSchema(spec).Properties {
		if property.Type == openapi.SchemaStructType {
			continue
		}
		if r.hasSpecificParam() && !r.isSpecificParam(key) {
			continue
		}

		pt, err := generateParameterSchema(key, property)
		if err != nil {
			return err
		}
		pt.scope = "Global"
		pt.dynamic = isDynamicType(pt, staticParameters, dynamicParameters, immutableParameters)

		if r.hasSpecificParam() {
			printSingleParameterSchema(pt)
			return nil
		}
		if !r.hasSpecificParam() && r.truncDocument && len(pt.description) > maxDocumentLength {
			pt.description = pt.description[:maxDocumentLength] + "..."
		}
		params = append(params, pt)
	}

	if !r.truncEnum {
		maxEnumLength = -1
	}
	printConfigParameterSchema(params, r.Out, maxEnumLength)
	return nil
}

func isDynamicType(pt *parameterSchema, staticParameters, dynamicParameters, immutableParameters *cfgutil.Sets) bool {
	switch {
	case immutableParameters.InArray(pt.name):
		return false
	case staticParameters.InArray(pt.name):
		return false
	case dynamicParameters.InArray(pt.name):
		return true
	case dynamicParameters.Length() == 0 && staticParameters.Length() != 0:
		return true
	case dynamicParameters.Length() != 0 && staticParameters.Length() == 0:
		return false
	default:
		return false
	}
}

// NewDescribeReconfigureCmd shows details of history modifications or configuration file of reconfiguring operations
func NewDescribeReconfigureCmd(f cmdutil.Factory, streams genericiooptions.IOStreams) *cobra.Command {
	o := &configObserverOptions{
		isExplain:          false,
		describeOpsOptions: newDescribeOpsOptions(f, streams),
	}
	cmd := &cobra.Command{
		Use:               "describe-config",
		Short:             "Show details of a specific reconfiguring.",
		Aliases:           []string{"desc-config"},
		Example:           describeReconfigureExample,
		ValidArgsFunction: util.ResourceNameCompletionFunc(f, types.ClusterGVR()),
		Run: func(cmd *cobra.Command, args []string) {
			util.CheckErr(o.complete2(args))
			util.CheckErr(o.run(o.printComponentConfigSpecsDescribe))
		},
	}
	o.addCommonFlags(cmd, f)
	cmd.Flags().StringSliceVar(&o.keys, "config-file", nil, "Specify the name of the configuration file to be describe (e.g. for mysql: --config-file=my.cnf). If unset, all files.")
	return cmd
}

// NewExplainReconfigureCmd shows details of modifiable parameters.
func NewExplainReconfigureCmd(f cmdutil.Factory, streams genericiooptions.IOStreams) *cobra.Command {
	o := &configObserverOptions{
		isExplain:          true,
		truncEnum:          true,
		truncDocument:      false,
		describeOpsOptions: newDescribeOpsOptions(f, streams),
	}
	cmd := &cobra.Command{
		Use:               "explain-config",
		Short:             "List the constraint for supported configuration params.",
		Aliases:           []string{"ex-config"},
		Example:           explainReconfigureExample,
		ValidArgsFunction: util.ResourceNameCompletionFunc(f, types.ClusterGVR()),
		Run: func(cmd *cobra.Command, args []string) {
			util.CheckErr(o.complete2(args))
			util.CheckErr(o.run(o.printComponentExplainConfigure))
		},
	}
	o.addCommonFlags(cmd, f)
	cmd.Flags().BoolVar(&o.truncEnum, "trunc-enum", o.truncEnum, "If the value list length of the parameter is greater than 20, it will be truncated.")
	cmd.Flags().BoolVar(&o.truncDocument, "trunc-document", o.truncDocument, "If the document length of the parameter is greater than 100, it will be truncated.")
	cmd.Flags().StringVar(&o.paramName, "param", o.paramName, "Specify the name of parameter to be query. It clearly display the details of the parameter.")
	return cmd
}
