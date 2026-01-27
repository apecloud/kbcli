/*
Copyright (C) 2022-2026 ApeCloud Co., Ltd

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package opsdefinition

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	"k8s.io/client-go/dynamic"
	clientset "k8s.io/client-go/kubernetes"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/kubectl/pkg/util/templates"

	"github.com/apecloud/kubeblocks/apis/operations/v1alpha1"

	"github.com/apecloud/kbcli/pkg/printer"
	"github.com/apecloud/kbcli/pkg/types"
	"github.com/apecloud/kbcli/pkg/util"
)

var (
	describeExample = templates.Examples(`
		# describe a specified ops-definition
		kbcli ops-definition describe my-ops-definition`)
)

type describeOptions struct {
	factory   cmdutil.Factory
	client    clientset.Interface
	dynamic   dynamic.Interface
	namespace string

	names []string
	genericiooptions.IOStreams
}

func NewDescribeCmd(f cmdutil.Factory, streams genericiooptions.IOStreams) *cobra.Command {
	o := &describeOptions{
		factory:   f,
		IOStreams: streams,
	}
	cmd := &cobra.Command{
		Use:               "describe",
		Short:             "Describe OpsDefinition.",
		Example:           describeExample,
		Aliases:           []string{"desc"},
		ValidArgsFunction: util.ResourceNameCompletionFunc(f, types.OpsDefinitionGVR()),
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(o.complete(args))
			cmdutil.CheckErr(o.run())
		},
	}
	return cmd
}

func (o *describeOptions) complete(args []string) error {
	var err error

	if len(args) == 0 {
		return fmt.Errorf("component definition name should be specified")
	}
	o.names = args

	if o.client, err = o.factory.KubernetesClientSet(); err != nil {
		return err
	}

	if o.dynamic, err = o.factory.DynamicClient(); err != nil {
		return err
	}

	if o.namespace, _, err = o.factory.ToRawKubeConfigLoader().Namespace(); err != nil {
		return err
	}

	return nil
}

func (o *describeOptions) run() error {
	for _, name := range o.names {
		if err := o.describeOpsDefinition(name); err != nil {
			return fmt.Errorf("error describing OpsDefinition '%s': %w", name, err)
		}
	}
	return nil
}

func (o *describeOptions) describeOpsDefinition(name string) error {
	opsDef := &v1alpha1.OpsDefinition{}
	if err := util.GetK8SClientObject(o.dynamic, opsDef, types.OpsDefinitionGVR(), "", name); err != nil {
		return fmt.Errorf("failed to get OpsDefinition '%s': %w", name, err)
	}
	return o.showOpsDefinition(opsDef)
}

func (o *describeOptions) showOpsDefinition(opsDef *v1alpha1.OpsDefinition) error {
	printer.PrintPairStringToLine("Name", opsDef.Name, 0)
	showComponentInfos(&opsDef.Spec.ComponentInfos, o.Out)
	showPreConditions(&opsDef.Spec.PreConditions, o.Out)
	showActions(&opsDef.Spec.Actions, o.Out)
	showParametersSchema(opsDef.Spec.ParametersSchema, o.Out)
	showPodInfoExtractors(&opsDef.Spec.PodInfoExtractors, o.Out)
	printer.PrintPairStringToLine("Status", string(opsDef.Status.Phase), 0)
	if opsDef.Status.Message != "" {
		printer.PrintPairStringToLine("Message", opsDef.Status.Message, 0)
	}
	return nil
}

func showPreConditions(preConditions *[]v1alpha1.PreCondition, out io.Writer) {
	if len(*preConditions) == 0 {
		return
	}
	fmt.Fprintf(out, "PreConditions:\n")
	tbl := printer.NewTablePrinter(out)
	tbl.SetHeader("\tEXPRESSION", "MESSAGE")
	for _, preCondition := range *preConditions {
		tbl.AddRow("\t"+preCondition.Rule.Expression, preCondition.Rule.Message)
	}
	tbl.Print()
	fmt.Fprint(out, "\n")
}

func showComponentInfos(compInfos *[]v1alpha1.ComponentInfo, out io.Writer) {
	if len(*compInfos) == 0 {
		return
	}
	fmt.Fprintf(out, "ComponentInfos:\n")
	tbl := printer.NewTablePrinter(out)
	tbl.SetHeader("\tCOMPONENT-DEF-NAME", "ACCOUNT-NAME", "SERVICE-NAME")
	for _, compInfo := range *compInfos {
		tbl.AddRow("\t"+compInfo.ComponentDefinitionName, compInfo.AccountName, compInfo.ServiceName)
	}
	tbl.Print()
	fmt.Fprint(out, "\n")
}

func showActions(actions *[]v1alpha1.OpsAction, out io.Writer) {
	if len(*actions) == 0 {
		fmt.Fprintln(out, "No Actions defined.")
		return
	}
	fmt.Fprintf(out, "Actions:\n")
	tbl := printer.NewTablePrinter(out)
	tbl.SetHeader("\tNAME", "TYPE", "FAILURE POLICY", "PARAMETERS")

	for _, action := range *actions {
		actionType := ""
		params := strings.Join(action.Parameters, ", ")
		switch {
		case action.Workload != nil:
			actionType = "Workload"
		case action.Exec != nil:
			actionType = "Exec"
		case action.ResourceModifier != nil:
			actionType = "ResourceModifier"
		}
		if params == "" {
			params = "None"
		}

		tbl.AddRow("\t"+action.Name, actionType, string(action.FailurePolicy), params)
	}
	tbl.Print()
	fmt.Fprint(out, "\n")
}

func showParametersSchema(schema *v1alpha1.ParametersSchema, out io.Writer) {
	if schema == nil || schema.OpenAPIV3Schema == nil {
		return
	}
	fmt.Fprintf(out, "Parameters Schema:\n")
	schemaJSON, err := json.MarshalIndent(schema.OpenAPIV3Schema, "", "  ")
	if err != nil {
		fmt.Fprintf(out, "\tError: %v\n", err)
		return
	}
	fmt.Fprintf(out, "%s\n", string(schemaJSON))
}

func showPodInfoExtractors(extractors *[]v1alpha1.PodInfoExtractor, out io.Writer) {
	if len(*extractors) == 0 {
		fmt.Fprintln(out, "No Pod Info Extractors defined.")
		return
	}
	fmt.Fprintf(out, "Pod Info Extractors:\n")
	tbl := printer.NewTablePrinter(out)
	tbl.SetHeader("\tNAME", "ROLE", "SELECTION POLICY", "ENV SOURCES", "VOLUME DETAILS")

	for _, extractor := range *extractors {
		role := "None"
		if extractor.PodSelector.Role != "" {
			role = extractor.PodSelector.Role
		}
		policy := fmt.Sprintf("%v", extractor.PodSelector.MultiPodSelectionPolicy)

		envSources := formatEnvSources(extractor.Env)
		volumeDetails := formatVolumeDetails(extractor.VolumeMounts)

		tbl.AddRow("\t"+extractor.Name, role, policy, envSources, volumeDetails)
	}
	tbl.Print()
	fmt.Fprint(out, "\n")
}

// Helper function to format environment variable sources
func formatEnvSources(envVars []v1alpha1.OpsEnvVar) string {
	if len(envVars) == 0 {
		return "None"
	}
	var details []string
	for _, envVar := range envVars {
		sourceDetail := "Unknown"
		if envVar.ValueFrom.EnvVarRef != nil {
			sourceDetail = "EnvVar: " + envVar.ValueFrom.EnvVarRef.EnvName
		} else if envVar.ValueFrom.FieldRef != nil {
			sourceDetail = "FieldPath: " + envVar.ValueFrom.FieldRef.FieldPath
		}
		details = append(details, sourceDetail)
	}
	return strings.Join(details, ", ")
}

// Helper function to format volume mount details
func formatVolumeDetails(volumeMounts []corev1.VolumeMount) string {
	if len(volumeMounts) == 0 {
		return "None"
	}
	var details []string
	for _, vm := range volumeMounts {
		detail := fmt.Sprintf("%s at %s", vm.Name, vm.MountPath)
		details = append(details, detail)
	}
	return strings.Join(details, "; ")
}
