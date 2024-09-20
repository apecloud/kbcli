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
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v2"
	"helm.sh/helm/v3/pkg/chart/loader"
	"k8s.io/klog"

	"github.com/apecloud/kbcli/pkg/types"
	"github.com/apecloud/kbcli/pkg/util"
	"github.com/apecloud/kbcli/pkg/util/flags"
)

// CliClusterChartConfig is $HOME/.kbcli/cluster_types by default
var CliClusterChartConfig string

// CliChartsCacheDir is $HOME/.kbcli/charts by default
var CliChartsCacheDir string

type clusterConfig []*TypeInstance

// GlobalClusterChartConfig is kbcli global cluster chart config reference to CliClusterChartConfig
var GlobalClusterChartConfig clusterConfig
var CacheFiles []fs.DirEntry

// ReadConfigs read the config from configPath
func (c *clusterConfig) ReadConfigs(configPath string) error {
	contents, err := os.ReadFile(configPath)
	if err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		contents = []byte{}
	}
	err = yaml.Unmarshal(contents, c)
	if err != nil {
		return err
	}
	return nil
}

// WriteConfigs write current config into configPath
func (c *clusterConfig) WriteConfigs(configPath string) error {
	newConfig, err := yaml.Marshal(*c)
	if err != nil {
		return err
	}
	return os.WriteFile(configPath, newConfig, 0666)
}

// AddConfig add a new cluster type instance into current config
func (c *clusterConfig) AddConfig(add *TypeInstance) {
	*c = append(*c, add)
}

// UpdateConfig will update the existed TypeInstance in c
func (c *clusterConfig) UpdateConfig(update *TypeInstance) {
	for i, instance := range *c {
		if instance.Name == update.Name {
			(*c)[i] = update
		}
	}
}

// RemoveConfig remove a ClusterType from current config
func (c *clusterConfig) RemoveConfig(name ClusterType) bool {
	tempList := *c
	for i, chart := range tempList {
		if chart.Name == name {
			*c = append((*c)[:i], (*c)[i+1:]...)
			return true
		}
	}
	return false
}
func (c *clusterConfig) Len() int {
	return len(*c)
}

// RegisterCMD will register all cluster type instances in the config c and auto clear the register failed instances
// and rewrite config
func RegisterCMD(c clusterConfig, configPath string) {
	var needRemove []ClusterType
	for _, config := range c {
		if err := config.register(config.Name); err != nil {
			klog.V(2).Info(err.Error())
			needRemove = append(needRemove, config.Name)
		}
	}
	for _, name := range needRemove {
		c.RemoveConfig(name)
	}
	if err := c.WriteConfigs(configPath); err != nil {
		klog.V(2).Info(fmt.Sprintf("Warning: auto clear kbcli cluster chart config failed %s\n", err.Error()))
	}
}

func GetChartCacheFiles() []fs.DirEntry {
	homeDir, _ := util.GetCliHomeDir()
	dirFS := os.DirFS(homeDir)
	result, err := fs.ReadDir(dirFS, "charts")
	if err != nil {
		if os.IsNotExist(err) {
			err = os.MkdirAll(CliChartsCacheDir, 0777)
			if err != nil {
				klog.V(2).Info(fmt.Sprintf("Failed to create charts cache dir %s: %s", CliChartsCacheDir, err.Error()))
				return nil
			}
			result = []fs.DirEntry{}
		} else {
			klog.V(2).Info(fmt.Sprintf("Failed to create charts cache dir %s: %s", CliChartsCacheDir, err.Error()))
			return nil
		}
	}
	return result
}

func ClearCharts(c ClusterType) {
	// if the fail clusterType is from external config, remove the config and the related charts
	if GlobalClusterChartConfig.RemoveConfig(c) {
		if err := GlobalClusterChartConfig.WriteConfigs(CliClusterChartConfig); err != nil {
			klog.V(2).Info(fmt.Sprintf("Warning: auto clear %s config fail due to: %s\n", c, err.Error()))

		}
		if err := os.Remove(filepath.Join(CliChartsCacheDir, ClusterTypeCharts[c].getChartFileName())); err != nil {
			klog.V(2).Info(fmt.Sprintf("Warning: auto clear %s config fail due to: %s\n", c, err.Error()))
		}
		CacheFiles = GetChartCacheFiles()
	}
}

// TypeInstance reference to a cluster type instance in config and implement the cluster.chartLoader interface
type TypeInstance struct {
	Name  ClusterType `yaml:"name"`
	URL   string      `yaml:"helmChartUrl"`
	Alias string      `yaml:"alias"`
	// chartName is the filename cached locally
	ChartName string `yaml:"chartName"`
}

// PreCheck is used by `cluster register` command
func (h *TypeInstance) PreCheck() error {

	chartInfo := &ChartInfo{}
	// load helm chart from embed tgz file
	{
		file, err := h.loadChart()
		if err != nil {
			return err
		}
		defer file.Close()
		c, err := loader.LoadArchive(file)
		if err != nil {
			if err == gzip.ErrHeader {
				return fmt.Errorf("file '%s' does not appear to be a valid chart file (details: %s)", h.getChartFileName(), err)
			}
		}
		if c == nil {
			return fmt.Errorf("failed to load cluster helm chart %s", h.getChartFileName())
		}
		chartInfo.Chart = c
	}
	if err := chartInfo.BuildClusterSchema(); err != nil {
		return err
	}
	// pre-check build sub-command flags
	if err := flags.BuildFlagsBySchema(&cobra.Command{}, chartInfo.Schema); err != nil {
		return err
	}
	err := flags.BuildFlagsBySchema(&cobra.Command{}, chartInfo.SubSchema)
	if err != nil {
		return err
	}
	return nil
}

func (h *TypeInstance) loadChart() (io.ReadCloser, error) {
	return os.Open(filepath.Join(CliChartsCacheDir, h.getChartFileName()))
}

func (h *TypeInstance) getChartFileName() string {
	return h.ChartName
}

func (h *TypeInstance) getAlias() string {
	return h.Alias
}

func (h *TypeInstance) register(subcmd ClusterType) error {
	if _, ok := ClusterTypeCharts[subcmd]; ok {
		// replace built-in cluster chart
		klog.V(2).Info(fmt.Sprintf("cluster chart of %s is replaced manully\n", subcmd.String()))
	}
	ClusterTypeCharts[subcmd] = h

	for _, f := range CacheFiles {
		if f.Name() == h.getChartFileName() {
			return nil
		}
	}
	return fmt.Errorf("can't find the %s in cache, please use 'kbcli cluster register %s --url %s' first", h.Name.String(), h.Name.String(), h.URL)
}

func (h *TypeInstance) PatchNewClusterType() error {
	if err := h.PreCheck(); err != nil {
		return fmt.Errorf("the chart of %s pre-check unsuccssful: %s", h.Name, err.Error())
	}
	isChartExist := false
	for _, item := range GlobalClusterChartConfig {
		if h.Name == item.Name {
			isChartExist = true
		}
	}
	if isChartExist {
		GlobalClusterChartConfig.UpdateConfig(h)
	} else {
		GlobalClusterChartConfig.AddConfig(h)
	}
	return GlobalClusterChartConfig.WriteConfigs(CliClusterChartConfig)
}

var StandardSchema = map[string]interface{}{
	"properties": map[string]interface{}{
		"replicas": nil,
		"cpu":      nil,
		"memory":   nil,
		"storage":  nil,
	},
}

func (h *TypeInstance) ValidateChartSchema() (bool, error) {
	file, err := h.loadChart()
	if err != nil {
		return false, err
	}
	defer file.Close()

	c, err := loader.LoadArchive(file)
	if err != nil {
		return false, err
	}

	data := c.Schema
	if len(data) == 0 {
		return false, fmt.Errorf("register cluster chart of %s failed, schema of the chart doesn't exist", h.Name)
	}

	var schema map[string]interface{}
	if err := json.Unmarshal(data, &schema); err != nil {
		return false, fmt.Errorf("register cluster chart of %s failed, error decoding JSON: %s", h.Name, err)
	}

	if err := validateSchema(schema, StandardSchema); err != nil {
		return false, err
	}

	return true, nil
}

func validateSchema(schema, standard map[string]interface{}) error {
	for key, val := range standard {
		if subStandard, ok := val.(map[string]interface{}); ok {
			subSchema, ok := schema[key].(map[string]interface{})
			if !ok {
				return fmt.Errorf("register cluster chart failed, schema missing required map key '%s'", key)
			}
			if err := validateSchema(subSchema, subStandard); err != nil {
				return err
			}
		} else {
			if _, exists := schema[key]; !exists {
				return fmt.Errorf("register cluster chart failed, schema missing required key '%s'", key)
			}
		}
	}
	return nil
}

var _ chartLoader = &TypeInstance{}

func init() {
	homeDir, _ := util.GetCliHomeDir()
	CliClusterChartConfig = filepath.Join(homeDir, types.CliClusterTypeConfigs)
	CliChartsCacheDir = filepath.Join(homeDir, types.CliChartsCache)

	err := GlobalClusterChartConfig.ReadConfigs(CliClusterChartConfig)
	if err != nil {
		fmt.Println(err.Error())
		return
	}
	// check charts cache dir
	CacheFiles = GetChartCacheFiles()
	RegisterCMD(GlobalClusterChartConfig, CliClusterChartConfig)
}
