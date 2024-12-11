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
	"io"
	"sort"
	"strings"

	corev1 "k8s.io/api/core/v1"

	"github.com/apecloud/kbcli/pkg/printer"
	"github.com/apecloud/kbcli/pkg/util"
)

type PrintType string

const (
	PrintClusters   PrintType = "clusters"
	PrintWide       PrintType = "wide"
	PrintInstances  PrintType = "instances"
	PrintComponents PrintType = "components"
	PrintEvents     PrintType = "events"
	PrintLabels     PrintType = "label"
)

type PrinterOptions struct {
	ShowLabels   bool
	StatusFilter string
}

type tblInfo struct {
	header     []interface{}
	addRow     func(tbl *printer.TablePrinter, objs *ClusterObjects, opt *PrinterOptions) [][]interface{}
	getOptions GetOptions
}

var mapTblInfo = map[PrintType]tblInfo{
	PrintClusters: {
		header: []interface{}{"NAME", "NAMESPACE", "CLUSTER-DEFINITION", "TERMINATION-POLICY", "STATUS", "CREATED-TIME"},
		addRow: func(tbl *printer.TablePrinter, objs *ClusterObjects, opt *PrinterOptions) [][]interface{} {
			c := objs.GetClusterInfo()
			info := []interface{}{c.Name, c.Namespace, c.ClusterDefinition, c.TerminationPolicy, c.Status, c.CreatedTime}
			if opt.ShowLabels {
				info = append(info, c.Labels)
			}
			return [][]interface{}{info}
		},
		getOptions: GetOptions{},
	},
	PrintWide: {
		header: []interface{}{"NAME", "NAMESPACE", "CLUSTER-DEFINITION", "TERMINATION-POLICY", "STATUS", "INTERNAL-ENDPOINTS", "EXTERNAL-ENDPOINTS", "CREATED-TIME"},
		addRow: func(tbl *printer.TablePrinter, objs *ClusterObjects, opt *PrinterOptions) [][]interface{} {
			c := objs.GetClusterInfo()
			info := []interface{}{c.Name, c.Namespace, c.ClusterDefinition, c.TerminationPolicy, c.Status, c.InternalEP, c.ExternalEP, c.CreatedTime}
			if opt.ShowLabels {
				info = append(info, c.Labels)
			}
			return [][]interface{}{info}
		},
		getOptions: GetOptions{WithClusterDef: Maybe, WithService: Need, WithPod: Need},
	},
	PrintInstances: {
		header:     []interface{}{"NAME", "NAMESPACE", "CLUSTER", "COMPONENT", "STATUS", "ROLE", "ACCESSMODE", "AZ", "CPU(REQUEST/LIMIT)", "MEMORY(REQUEST/LIMIT)", "STORAGE", "NODE", "CREATED-TIME"},
		addRow:     AddInstanceRow,
		getOptions: GetOptions{WithClusterDef: Maybe, WithPod: Need},
	},
	PrintComponents: {
		header:     []interface{}{"NAME", "NAMESPACE", "CLUSTER", "TYPE", "IMAGE"},
		addRow:     AddComponentRow,
		getOptions: GetOptions{WithClusterDef: Maybe, WithPod: Need},
	},
	PrintEvents: {
		header:     []interface{}{"NAMESPACE", "TIME", "TYPE", "REASON", "OBJECT", "MESSAGE"},
		addRow:     AddEventRow,
		getOptions: GetOptions{WithClusterDef: Maybe, WithPod: Need, WithEvent: Need},
	},
	PrintLabels: {
		header:     []interface{}{"NAME", "NAMESPACE"},
		addRow:     AddLabelRow,
		getOptions: GetOptions{},
	},
}

// Printer prints cluster info
type Printer struct {
	tbl     *printer.TablePrinter
	opt     *PrinterOptions
	tblInfo tblInfo
	pt      PrintType
	rows    [][]interface{}
}

func NewPrinter(out io.Writer, printType PrintType, opt *PrinterOptions) *Printer {
	p := &Printer{tbl: printer.NewTablePrinter(out), pt: printType}
	p.tblInfo = mapTblInfo[printType]

	if opt == nil {
		opt = &PrinterOptions{}
	}
	p.opt = opt

	if opt.ShowLabels {
		p.tblInfo.header = append(p.tblInfo.header, "LABELS")
	}

	p.tbl.SetHeader(p.tblInfo.header...)
	return p
}

func (p *Printer) AddRow(objs *ClusterObjects) {
	lines := p.tblInfo.addRow(p.tbl, objs, p.opt)
	p.rows = append(p.rows, lines...)
}

func (p *Printer) Print() {
	if p.pt == PrintClusters || p.pt == PrintWide {
		p.filterByStatus()
		p.sortRows()
	}

	for _, row := range p.rows {
		p.tbl.AddRow(row...)
	}

	p.tbl.Print()
}

func (p *Printer) GetterOptions() GetOptions {
	return p.tblInfo.getOptions
}

func (p *Printer) filterByStatus() {
	if p.opt.StatusFilter == "" {
		return
	}

	statusIndex := 4

	var filtered [][]interface{}
	for _, r := range p.rows {
		statusVal, _ := r[statusIndex].(string)
		if strings.EqualFold(statusVal, p.opt.StatusFilter) {
			filtered = append(filtered, r)
		}
	}
	p.rows = filtered
}

// sortRows Sort By namespace(1), clusterDef(2), status(4), name(0)
func (p *Printer) sortRows() {
	// for PrintClusters 和 PrintWide
	// NAME(0), NAMESPACE(1), CLUSTER-DEFINITION(2), STATUS(4)
	sort.Slice(p.rows, func(i, j int) bool {
		ri, rj := p.rows[i], p.rows[j]

		nsI, _ := ri[1].(string)
		nsJ, _ := rj[1].(string)
		if nsI != nsJ {
			return nsI < nsJ
		}

		cdI, _ := ri[2].(string)
		cdJ, _ := rj[2].(string)
		if cdI != cdJ {
			return cdI < cdJ
		}

		statusI, _ := ri[4].(string)
		statusJ, _ := rj[4].(string)
		if statusI != statusJ {
			return compareStatus(statusI, statusJ)
		}

		nameI, _ := ri[0].(string)
		nameJ, _ := rj[0].(string)
		return nameI < nameJ
	})
}

// compareStatus compares statuses based on the desired order
func compareStatus(status1, status2 string) bool {
	statusOrder := map[string]int{
		"Creating": 1,
		"Running":  2,
		"Updating": 3,
		"Stopping": 4,
		"Stopped":  5,
		"Deleting": 6,
		"Failed":   7,
		"Abnormal": 8,
	}

	order1, ok1 := statusOrder[status1]
	order2, ok2 := statusOrder[status2]

	// unknown is the last
	if !ok1 && !ok2 {
		return status1 < status2
	}
	if !ok1 {
		return false
	}
	if !ok2 {
		return true
	}

	return order1 < order2
}

func AddLabelRow(tbl *printer.TablePrinter, objs *ClusterObjects, opt *PrinterOptions) [][]interface{} {
	c := objs.GetClusterInfo()
	info := []interface{}{c.Name, c.Namespace}
	if opt.ShowLabels {
		labels := strings.ReplaceAll(c.Labels, ",", "\n")
		info = append(info, labels)
	}
	return [][]interface{}{info}
}

func AddComponentRow(tbl *printer.TablePrinter, objs *ClusterObjects, opt *PrinterOptions) [][]interface{} {
	components := objs.GetComponentInfo()
	var rows [][]interface{}
	for _, c := range components {
		row := []interface{}{c.Name, c.NameSpace, c.Cluster, c.ComponentDef, c.Image}
		rows = append(rows, row)
	}
	return rows
}

func AddInstanceRow(tbl *printer.TablePrinter, objs *ClusterObjects, opt *PrinterOptions) [][]interface{} {
	instances := objs.GetInstanceInfo()
	var rows [][]interface{}
	for _, instance := range instances {
		row := []interface{}{
			instance.Name, instance.Namespace, instance.Cluster, instance.Component,
			instance.Status, instance.Role, instance.AccessMode,
			instance.AZ, instance.CPU, instance.Memory,
			BuildStorageSize(instance.Storage), instance.Node, instance.CreatedTime,
		}
		rows = append(rows, row)
	}
	return rows
}

func AddEventRow(tbl *printer.TablePrinter, objs *ClusterObjects, opt *PrinterOptions) [][]interface{} {
	events := util.SortEventsByLastTimestamp(objs.Events, "")
	var rows [][]interface{}
	for _, event := range *events {
		e := event.(*corev1.Event)
		row := []interface{}{e.Namespace, util.GetEventTimeStr(e), e.Type, e.Reason, util.GetEventObject(e), e.Message}
		rows = append(rows, row)
	}
	return rows
}
