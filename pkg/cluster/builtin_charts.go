/*
Copyright (C) 2022-2023 ApeCloud Co., Ltd

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
	"embed"
	"fmt"
	"io"
)

// embedConfig is the interface for the go embed chart
type embedConfig struct {
	chartFS embed.FS
	// chart file name, include the extension
	name string
	// chart alias, this alias will be used as the command alias
	alias string
}

var _ chartLoader = &embedConfig{}

func (e *embedConfig) register(subcmd ClusterType) error {
	if _, ok := ClusterTypeCharts[subcmd]; ok {
		return fmt.Errorf("cluster type %s already registered", subcmd)
	}
	ClusterTypeCharts[subcmd] = e
	return nil
}

func (e *embedConfig) getAlias() string {
	return e.alias
}

func (e *embedConfig) loadChart() (io.ReadCloser, error) {
	return e.chartFS.Open(fmt.Sprintf("charts/%s", e.name))
}

func (e *embedConfig) getChartFileName() string {
	return e.name
}

var (
	// run `make generate` to generate this embed file
	// must add all: to include files start with _, such as _helpers.tpl
	//go:embed all:charts
	defaultChart embed.FS
)

var builtinClusterTypes = []ClusterType{
	"mysql",
	"postgresql",
	"kafka",
	"redis",
	"mongodb",
	"llm",
	"xinference",
}

var builtinChartMap = map[ClusterType]string{
	"mysql": "apecloud-mysql",
}

func IsbuiltinCharts(chart string) bool {
	for _, c := range builtinClusterTypes {
		if chart == string(c) {
			return true
		}
	}
	return false
}

func getChartName(t ClusterType) string {
	name, ok := builtinChartMap[t]
	if !ok {
		// if not found in map, return original name
		return string(t)
	}
	return name
}

// internal_chart registers embed chart

func init() {
}
