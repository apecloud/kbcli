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

package flags

import (
	"context"
	"fmt"
	"strings"

	appsv1alpha1 "github.com/apecloud/kubeblocks/apis/apps/v1alpha1"
	"github.com/apecloud/kubeblocks/pkg/constant"
	"github.com/spf13/cobra"
	flag "github.com/spf13/pflag"
	"github.com/stoewer/go-strcase"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/kube-openapi/pkg/validation/spec"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	utilcomp "k8s.io/kubectl/pkg/util/completion"

	"github.com/apecloud/kbcli/pkg/types"
	"github.com/apecloud/kbcli/pkg/util"
)

const (
	CobraInt          = "int"
	CobraSting        = "string"
	CobraBool         = "bool"
	CobraFloat64      = "float64"
	CobraStringArray  = "stringArray"
	CobraIntSlice     = "intSlice"
	CobraFloat64Slice = "float64Slice"
	CobraBoolSlice    = "boolSlice"
)

const (
	typeNull    = "null"
	typeString  = "string"
	typeNumber  = "number"
	typeInteger = "integer"
	typeBoolean = "boolean"
	typeObject  = "object"
	typeArray   = "array"
)

// AddClusterDefinitionFlag adds a flag "cluster-definition" for the cmd and stores the value of the flag
// in string p
func AddClusterDefinitionFlag(f cmdutil.Factory, cmd *cobra.Command, p *string) {
	cmd.Flags().StringVar(p, "cluster-definition", *p, "Specify cluster definition, run \"kbcli clusterdefinition list\" to show all available cluster definition")
	util.CheckErr(cmd.RegisterFlagCompletionFunc("cluster-definition",
		func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			return utilcomp.CompGetResource(f, util.GVRToString(types.ClusterDefGVR()), toComplete), cobra.ShellCompDirectiveNoFileComp
		}))
}

// BuildFlagsBySchema builds a flag.FlagSet by the given schema, convert the schema key
// to flag name, and convert the schema type to flag type.
func BuildFlagsBySchema(cmd *cobra.Command, schema *spec.Schema) error {
	if schema == nil {
		return nil
	}

	for name, prop := range schema.Properties {
		if err := buildOneFlag(cmd, name, &prop, false); err != nil {
			return err
		}
	}

	for _, name := range schema.Required {
		flagName := strcase.KebabCase(name)
		// fixme: array/object type flag will be "--servers.name", but in schema.Required will check the "--servers" flag, do not mark the array type as required in schema.json
		if schema.Properties[name].Type[0] == typeObject || schema.Properties[name].Type[0] == typeArray {
			continue
		}
		if err := cmd.MarkFlagRequired(flagName); err != nil {
			return err
		}
	}

	return nil
}

// castOrZero is a helper function to cast a value to a specific type.
// If the cast fails, the zero value will be returned.
func castOrZero[T any](v any) T {
	cv, _ := v.(T)
	return cv
}

func buildOneFlag(cmd *cobra.Command, k string, s *spec.Schema, isArray bool) error {
	name := strcase.KebabCase(k)
	if cmd.Flag(name) != nil {
		return nil
	}
	tpe := typeString
	if len(s.Type) > 0 {
		tpe = s.Type[0]
	}
	getFlags := func() *flag.FlagSet {
		if cmd.Flag(name) != nil {
			return cmd.LocalFlags()
		}
		return cmd.Flags()
	}
	if isArray {
		switch tpe {
		case typeString:
			getFlags().StringArray(name, []string{castOrZero[string](s.Default)}, buildFlagDescription(s))
		case typeInteger:
			getFlags().IntSlice(name, []int{castOrZero[int](s.Default)}, buildFlagDescription(s))
		case typeNumber:
			getFlags().Float64Slice(name, []float64{castOrZero[float64](s.Default)}, buildFlagDescription(s))
		case typeBoolean:
			getFlags().BoolSlice(name, []bool{castOrZero[bool](s.Default)}, buildFlagDescription(s))
		case typeObject:
			for subName, prop := range s.Properties {
				if err := buildOneFlag(cmd, fmt.Sprintf("%s.%s", name, subName), &prop, true); err != nil {
					return err
				}
			}
		case typeArray:
			return fmt.Errorf("unsupported build flags for object with array nested within an array")
		case typeNull:
			return nil
		default:
			return fmt.Errorf("unsupported json schema type %s", s.Type)
		}

		registerFlagCompFunc(cmd, name, s)
	} else {
		switch tpe {
		case typeString:
			getFlags().String(name, castOrZero[string](s.Default), buildFlagDescription(s))
		case typeInteger:
			getFlags().Int(name, int(castOrZero[float64](s.Default)), buildFlagDescription(s))
		case typeNumber:
			getFlags().Float64(name, castOrZero[float64](s.Default), buildFlagDescription(s))
		case typeBoolean:
			getFlags().Bool(name, castOrZero[bool](s.Default), buildFlagDescription(s))
		case typeObject:
			for subName, prop := range s.Properties {
				if err := buildOneFlag(cmd, fmt.Sprintf("%s.%s", name, subName), &prop, false); err != nil {
					return err
				}
			}
		case typeArray:
			if err := buildOneFlag(cmd, name, s.Items.Schema, true); err != nil {
				return err
			}
		case typeNull:
			return nil
		default:
			return fmt.Errorf("unsupported json schema type %s", s.Type)
		}

		registerFlagCompFunc(cmd, name, s)
	}
	// flag not support array type

	return nil
}

func buildFlagDescription(s *spec.Schema) string {
	desc := strings.Builder{}
	desc.WriteString(s.Description)

	var legalVals []string
	for _, e := range s.Enum {
		legalVals = append(legalVals, fmt.Sprintf("%v", e))
	}
	if len(legalVals) > 0 {
		desc.WriteString(fmt.Sprintf(" Legal values [%s].", strings.Join(legalVals, ", ")))
	}

	var valueRange []string
	if s.Minimum != nil {
		valueRange = append(valueRange, fmt.Sprintf("%v", *s.Minimum))
	}
	if s.Maximum != nil {
		valueRange = append(valueRange, fmt.Sprintf("%v", *s.Maximum))
	}
	if len(valueRange) > 0 {
		desc.WriteString(fmt.Sprintf(" Value range [%s].", strings.Join(valueRange, ", ")))
	}
	return desc.String()
}

func registerFlagCompFunc(cmd *cobra.Command, name string, s *spec.Schema) {
	// register the enum entry for autocompletion
	if len(s.Enum) > 0 {
		var entries []string
		for _, e := range s.Enum {
			entries = append(entries, fmt.Sprintf("%s\t", e))
		}
		_ = cmd.RegisterFlagCompletionFunc(name,
			func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
				return entries, cobra.ShellCompDirectiveNoFileComp
			})
		return
	}
}

// AddComponentFlag add flag "component" for cobra.Command and support auto complete for it
func AddComponentFlag(f cmdutil.Factory, cmd *cobra.Command, p *string, usage string) {
	cmd.Flags().StringVar(p, "component", "", usage)
	util.CheckErr(autoCompleteClusterComponent(cmd, f, "component"))
}

// AddComponentsFlag add flag "components" for cobra.Command and support auto complete for it
func AddComponentsFlag(f cmdutil.Factory, cmd *cobra.Command, p *[]string, usage string) {
	cmd.Flags().StringSliceVar(p, "components", nil, usage)
	util.CheckErr(autoCompleteClusterComponent(cmd, f, "components"))
}

func autoCompleteClusterComponent(cmd *cobra.Command, f cmdutil.Factory, flag string) error {
	autoComplete := func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		var components []string
		if len(args) == 0 {
			return components, cobra.ShellCompDirectiveNoFileComp
		}
		namespace, _, _ := f.ToRawKubeConfigLoader().Namespace()
		dynamic, _ := f.DynamicClient()
		cluster := &appsv1alpha1.Cluster{}
		_ = util.GetK8SClientObject(dynamic, cluster, types.ClusterGVR(), namespace, util.GetClusterNameFromArgsOrFlag(cmd, args))
		for _, comp := range cluster.Spec.ComponentSpecs {
			if strings.HasPrefix(comp.Name, toComplete) {
				components = append(components, comp.Name)
			}
		}
		return components, cobra.ShellCompDirectiveNoFileComp
	}
	return cmd.RegisterFlagCompletionFunc(flag, autoComplete)
}

func CompletedInstanceFlag(cmd *cobra.Command, f cmdutil.Factory, flag string) error {
	autoComplete := func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) == 0 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		clusterName := args[0]
		namespace, _, _ := f.ToRawKubeConfigLoader().Namespace()
		cli, _ := f.KubernetesClientSet()
		pods, err := cli.CoreV1().Pods(namespace).List(context.TODO(), metav1.ListOptions{
			LabelSelector: fmt.Sprintf("%s=%s", constant.AppInstanceLabelKey, clusterName),
		})
		if err != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		var podNames []string
		for _, v := range pods.Items {
			podNames = append(podNames, v.Name)
		}
		return podNames, cobra.ShellCompDirectiveNoFileComp
	}
	return cmd.RegisterFlagCompletionFunc(flag, autoComplete)
}

func CompletedInstanceTemplatesFlag(cmd *cobra.Command, f cmdutil.Factory, flag string) error {
	autoComplete := func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) == 0 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		namespace, _, _ := f.ToRawKubeConfigLoader().Namespace()
		dynamic, _ := f.DynamicClient()
		cluster := &appsv1alpha1.Cluster{}
		_ = util.GetK8SClientObject(dynamic, cluster, types.ClusterGVR(), namespace, util.GetClusterNameFromArgsOrFlag(cmd, args))
		var templateNames []string
		for _, comp := range cluster.Spec.ComponentSpecs {
			for _, insTpl := range comp.Instances {
				templateNames = append(templateNames, insTpl.Name)
			}
		}
		for _, comp := range cluster.Spec.ShardingSpecs {
			for _, insTpl := range comp.Template.Instances {
				templateNames = append(templateNames, insTpl.Name)
			}
		}
		return templateNames, cobra.ShellCompDirectiveNoFileComp
	}
	return cmd.RegisterFlagCompletionFunc(flag, autoComplete)
}
