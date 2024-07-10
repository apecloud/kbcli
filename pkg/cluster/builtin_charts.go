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
	//go:embed charts/apecloud-mysql-cluster.tgz
	apecloudmysqlChart embed.FS
	//go:embed charts/mysql-cluster.tgz
	mysqlChart embed.FS
	//go:embed charts/postgresql-cluster.tgz
	postgresqlChart embed.FS
	//go:embed charts/kafka-cluster.tgz
	kafkaChart embed.FS
	//go:embed charts/redis-cluster.tgz
	redisChart embed.FS
	//go:embed charts/mongodb-cluster.tgz
	mongodbChart embed.FS
	//go:embed charts/llm-cluster.tgz
	llmChart embed.FS
	//go:embed charts/xinference-cluster.tgz
	xinferenceChart embed.FS
	//go:embed charts/elasticsearch-cluster.tgz
	elasticsearchChart embed.FS
)

var builtinClusterTypes = map[ClusterType]bool{}

func IsBuiltinCharts(clusterType ClusterType) bool {
	return builtinClusterTypes[clusterType]
}

// internal_chart registers embed chart

func init() {
	embedChartConfigs := map[string]*embedConfig{
		"apecloud-mysql": {
			chartFS: apecloudmysqlChart,
			name:    "apecloud-mysql-cluster.tgz",
			alias:   "",
		},
		"mysql": {
			chartFS: mysqlChart,
			name:    "mysql-cluster.tgz",
			alias:   "",
		},
		"postgresql": {
			chartFS: postgresqlChart,
			name:    "postgresql-cluster.tgz",
			alias:   "",
		},

		"kafka": {
			chartFS: kafkaChart,
			name:    "kafka-cluster.tgz",
			alias:   "",
		},

		"redis": {
			chartFS: redisChart,
			name:    "redis-cluster.tgz",
			alias:   "",
		},

		"mongodb": {
			chartFS: mongodbChart,
			name:    "mongodb-cluster.tgz",
			alias:   "",
		},

		"llm": {
			chartFS: llmChart,
			name:    "llm-cluster.tgz",
			alias:   "",
		},

		"xinference": {
			chartFS: xinferenceChart,
			name:    "xinference-cluster.tgz",
			alias:   "",
		},

		"elasticsearch": {
			chartFS: elasticsearchChart,
			name:    "elasticsearch-cluster.tgz",
			alias:   "",
		},
	}

	for k, v := range embedChartConfigs {
		if err := v.register(ClusterType(k)); err != nil {
			fmt.Println(err.Error())
		}
		builtinClusterTypes[ClusterType(k)] = true
	}
}
