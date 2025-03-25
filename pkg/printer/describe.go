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

package printer

import (
	"encoding/json"
	"fmt"
	"io"
	"reflect"

	"gopkg.in/yaml.v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/apecloud/kbcli/pkg/util"
)

const NoneString = "<none>"

func PrintAllWarningEvents(events *corev1.EventList, out io.Writer) {
	objs := util.SortEventsByLastTimestamp(events, corev1.EventTypeWarning)
	title := fmt.Sprintf("\n%s Events: ", corev1.EventTypeWarning)
	if objs == nil || len(*objs) == 0 {
		fmt.Fprintln(out, title+NoneString)
		return
	}
	tbl := NewTablePrinter(out)
	fmt.Fprintln(out, title)
	tbl.SetHeader("TIME", "TYPE", "REASON", "OBJECT", "MESSAGE")
	for _, o := range *objs {
		e := o.(*corev1.Event)
		tbl.AddRow(util.GetEventTimeStr(e), e.Type, e.Reason, util.GetEventObject(e), e.Message)
	}
	tbl.Print()

}

// PrintConditions prints the conditions of resource.
func PrintConditions(conditions []metav1.Condition, out io.Writer) {
	// if the conditions are empty, return.
	if len(conditions) == 0 {
		return
	}
	tbl := NewTablePrinter(out)
	PrintTitle("Conditions")
	tbl.SetHeader("LAST-TRANSITION-TIME", "TYPE", "REASON", "STATUS", "MESSAGE")
	for _, con := range conditions {
		tbl.AddRow(util.TimeFormat(&con.LastTransitionTime), con.Type, con.Reason, con.Status, con.Message)
	}
	tbl.Print()
}

// PrintHelmValues prints the helm values file of the release in specified format, supports JSON、YAML and Table
func PrintHelmValues(configs map[string]interface{}, format Format, out io.Writer) {
	inTable := func() {
		p := NewTablePrinter(out)
		p.SetHeader("KEY", "VALUE")
		p.SortBy(1)
		for key, value := range configs {
			addRows(key, value, p, true) // to table
		}
		p.Print()
	}
	if format.IsHumanReadable() {
		inTable()
		return
	}

	var data []byte
	if format == YAML {
		data, _ = yaml.Marshal(configs)
	} else {
		data, _ = json.MarshalIndent(configs, "", "  ")
		data = append(data, '\n')
	}
	fmt.Fprint(out, string(data))
}

// addRows parses the interface value and add it to the Table
func addRows(key string, value interface{}, p *TablePrinter, ori bool) {
	if value == nil {
		p.AddRow(key, value)
		return
	}
	if reflect.TypeOf(value).Kind() == reflect.Map && ori {
		if len(value.(map[string]interface{})) == 0 {
			data, _ := json.Marshal(value)
			p.AddRow(key, string(data))
		}
		for k, v := range value.(map[string]interface{}) {
			addRows(key+"."+k, v, p, false)
		}
	} else {
		data, _ := json.Marshal(value)
		p.AddRow(key, string(data))
	}
}

func PrettyPrintObj(obj *unstructured.Unstructured) error {
	objYAML, err := yaml.Marshal(obj.Object)
	if err != nil {
		return err
	}

	// Parse YAML back into a structured map for pretty printing
	var parsedObj map[string]interface{}
	if err := yaml.Unmarshal(objYAML, &parsedObj); err != nil {
		return err
	}

	// Marshal again with indentation for pretty printing
	prettyYAML, err := yaml.Marshal(parsedObj)
	if err != nil {
		return err
	}

	fmt.Println(string(prettyYAML))
	return nil
}
