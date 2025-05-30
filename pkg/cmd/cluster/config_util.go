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
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"

	appsv1 "github.com/apecloud/kubeblocks/apis/apps/v1"
	parametersv1alpha1 "github.com/apecloud/kubeblocks/apis/parameters/v1alpha1"
	"github.com/apecloud/kubeblocks/pkg/generics"
	"github.com/spf13/cast"
	corev1 "k8s.io/api/core/v1"
	apiext "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/client-go/dynamic"
	"k8s.io/kubectl/pkg/cmd/util/editor"
	"sigs.k8s.io/controller-runtime/pkg/client"

	cfgcore "github.com/apecloud/kubeblocks/pkg/configuration/core"
	cfgutil "github.com/apecloud/kubeblocks/pkg/configuration/util"

	"github.com/apecloud/kbcli/pkg/action"
	"github.com/apecloud/kbcli/pkg/printer"
	"github.com/apecloud/kbcli/pkg/types"
	"github.com/apecloud/kbcli/pkg/util"
)

type configEditContext struct {
	action.CreateOptions

	clusterName    string
	componentName  string
	configSpecName string
	configKey      string

	original string
	edited   string
}

type parameterSchema struct {
	name         string
	valueType    string
	miniNum      string
	maxiNum      string
	enum         []string
	description  string
	scope        string
	dynamic      bool
	defaultValue string
}

func (c *configEditContext) getOriginal() string {
	return c.original
}

func (c *configEditContext) getEdited() string {
	return c.edited
}

func (c *configEditContext) prepare() error {
	cmObj := corev1.ConfigMap{}
	cmKey := client.ObjectKey{
		Name:      cfgcore.GetComponentCfgName(c.clusterName, c.componentName, c.configSpecName),
		Namespace: c.Namespace,
	}
	if err := util.GetResourceObjectFromGVR(types.ConfigmapGVR(), cmKey, c.Dynamic, &cmObj); err != nil {
		return err
	}

	val, ok := cmObj.Data[c.configKey]
	if !ok {
		return makeNotFoundConfigFileErr(c.configKey, c.configSpecName, cfgutil.ToSet(cmObj.Data).AsSlice())
	}

	c.original = val
	return nil
}

func (c *configEditContext) editConfig(editor editor.Editor, reader io.Reader) error {
	if reader == nil {
		reader = bytes.NewBufferString(c.original)
	}
	edited, _, err := editor.LaunchTempFile(fmt.Sprintf("%s-edit-", filepath.Base(c.configKey)), "", reader)
	if err != nil {
		return err
	}

	c.edited = string(edited)
	return nil
}

func newConfigContext(baseOptions action.CreateOptions, clusterName, componentName, configSpec, file string) *configEditContext {
	return &configEditContext{
		CreateOptions:  baseOptions,
		clusterName:    clusterName,
		componentName:  componentName,
		configSpecName: configSpec,
		configKey:      file,
	}
}

func fromKeyValuesToMap(params []cfgcore.VisualizedParam, file string) map[string]*string {
	result := make(map[string]*string)
	for _, param := range params {
		if param.Key != file {
			continue
		}
		for _, kv := range param.Parameters {
			result[kv.Key] = kv.Value
		}
	}
	return result
}

func (pt *parameterSchema) enumFormatter(maxFieldLength int) string {
	if len(pt.enum) == 0 {
		return ""
	}
	v := strings.Join(pt.enum, ",")
	if maxFieldLength > 0 && len(v) > maxFieldLength {
		v = v[:maxFieldLength] + "..."
	}
	return v
}

func (pt *parameterSchema) rangeFormatter() string {
	const (
		r          = "-"
		rangeBegin = "["
		rangeEnd   = "]"
	)

	if len(pt.maxiNum) == 0 && len(pt.miniNum) == 0 {
		return ""
	}

	v := rangeBegin
	if len(pt.miniNum) != 0 {
		v += pt.miniNum
	}
	if len(pt.maxiNum) != 0 {
		v += r
		v += pt.maxiNum
	} else if len(v) != 0 {
		v += r
	}
	v += rangeEnd
	return v
}

func getAllowedValues(pt *parameterSchema, maxFieldLength int) string {
	if len(pt.enum) != 0 {
		return pt.enumFormatter(maxFieldLength)
	}
	return pt.rangeFormatter()
}

func printSingleParameterSchema(pt *parameterSchema) {
	printer.PrintTitle("Configure Constraint")
	// print column "PARAMETER NAME", "RANGE", "ENUM", "SCOPE", "TYPE", "DESCRIPTION"
	printer.PrintPairStringToLine("Parameter Name", pt.name)
	printer.PrintPairStringToLine("Allowed Values", getAllowedValues(pt, -1))
	printer.PrintPairStringToLine("Scope", pt.scope)
	printer.PrintPairStringToLine("Dynamic", cast.ToString(pt.dynamic))
	printer.PrintPairStringToLine("Type", pt.valueType)
	printer.PrintPairStringToLine("Description", pt.description)
}

// printConfigParameterSchema prints the conditions of resource.
func printConfigParameterSchema(paramTemplates []*parameterSchema, out io.Writer, maxFieldLength int) {
	if len(paramTemplates) == 0 {
		return
	}

	sort.SliceStable(paramTemplates, func(i, j int) bool {
		x1 := paramTemplates[i]
		x2 := paramTemplates[j]
		return strings.Compare(x1.name, x2.name) < 0
	})

	tbl := printer.NewTablePrinter(out)
	tbl.SetStyle(printer.TerminalStyle)
	printer.PrintTitle("Parameter Explain")
	tbl.SetHeader("PARAMETER NAME", "ALLOWED VALUES", "DEFAULT VALUE", "SCOPE", "DYNAMIC", "TYPE", "DESCRIPTION")
	for _, pt := range paramTemplates {
		tbl.AddRow(pt.name, getAllowedValues(pt, maxFieldLength), pt.defaultValue, pt.scope, cast.ToString(pt.dynamic), pt.valueType, pt.description)
	}
	tbl.Print()
}

func generateParameterSchema(paramName string, property apiext.JSONSchemaProps) (*parameterSchema, error) {
	toString := func(v interface{}) (string, error) {
		b, err := json.Marshal(v)
		if err != nil {
			return "", err
		}
		return string(b), nil
	}
	pt := &parameterSchema{
		name:        paramName,
		valueType:   property.Type,
		description: strings.TrimSpace(property.Description),
	}
	if property.Minimum != nil {
		b, err := toString(property.Minimum)
		if err != nil {
			return nil, err
		}
		pt.miniNum = b
	}
	if property.Default != nil {
		b, err := toString(property.Default)
		if err != nil {
			return nil, err
		}
		pt.defaultValue = b
	}
	if property.Format != "" {
		pt.valueType = property.Format
	}
	if property.Maximum != nil {
		b, err := toString(property.Maximum)
		if err != nil {
			return nil, err
		}
		pt.maxiNum = b
	}
	if property.Enum != nil {
		pt.enum = make([]string, len(property.Enum))
		for i, v := range property.Enum {
			b, err := toString(v)
			if err != nil {
				return nil, err
			}
			pt.enum[i] = b
		}
	}
	return pt, nil
}

func getComponentNames(cluster *appsv1.Cluster) []string {
	var components []string
	for _, component := range cluster.Spec.ComponentSpecs {
		components = append(components, component.Name)
	}
	for _, component := range cluster.Spec.Shardings {
		components = append(components, component.Name)
	}
	return components
}

func resolveConfigTemplate(rctx *ReconfigureContext, dynamic dynamic.Interface) (map[string]*corev1.ConfigMap, error) {
	tpls := generics.Map(rctx.ConfigRender.Spec.Configs, func(parameter parametersv1alpha1.ComponentConfigDescription) string {
		return parameter.TemplateName
	})
	tplObjs := make(map[string]*corev1.ConfigMap, len(tpls))
	for _, tpl := range tpls {
		if _, ok := tplObjs[tpl]; ok {
			continue
		}
		index := generics.FindFirstFunc(rctx.Cmpd.Spec.Configs, func(spec appsv1.ComponentFileTemplate) bool {
			return spec.Name == tpl
		})
		if index < 0 {
			return nil, makeConfigSpecNotExistErr(rctx.Cluster.Name, rctx.CompName, tpl)
		}
		var cm = &corev1.ConfigMap{}
		tplMeta := rctx.Cmpd.Spec.Configs[index]
		key := client.ObjectKey{Namespace: tplMeta.Namespace, Name: tplMeta.Template}
		if err := util.GetResourceObjectFromGVR(types.ConfigmapGVR(), key, dynamic, cm); err != nil {
			return nil, err
		}
		tplObjs[tpl] = cm
	}

	return tplObjs, nil
}
