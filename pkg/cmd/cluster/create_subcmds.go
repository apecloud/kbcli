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
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"

	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"

	"github.com/apecloud/kbcli/pkg/action"
	"github.com/apecloud/kbcli/pkg/cluster"
	"github.com/apecloud/kbcli/pkg/printer"
	"github.com/apecloud/kbcli/pkg/types"
	"github.com/apecloud/kbcli/pkg/util"
)

type objectInfo struct {
	gvr schema.GroupVersionResource
	obj *unstructured.Unstructured
}

type CreateSubCmdsOptions struct {
	// ClusterType is the type of the cluster to create.
	ClusterType cluster.ClusterType

	// values is used to render the cluster helm ChartInfo.
	Values map[string]interface{}

	// ChartInfo is the cluster chart information, used to render the command flag
	// and validate the values.
	ChartInfo *cluster.ChartInfo

	// Configuration and options for cluster affinity and tolerations
	PodAntiAffinity string `json:"podAntiAffinity"`
	// TopologyKeys if TopologyKeys is nil, add omitempty json tag, because CueLang can not covert null to list.
	TopologyKeys   []string          `json:"topologyKeys,omitempty"`
	NodeLabels     map[string]string `json:"nodeLabels,omitempty"`
	Tenancy        string            `json:"tenancy"`
	TolerationsRaw []string          `json:"-"`
	Tolerations    []corev1.Toleration

	// SkipSchemaValidation is used to skip the schema validation of the helm chart.
	SkipSchemaValidation bool `json:"-"`

	*action.CreateOptions
}

func NewSubCmdsOptions(createOptions *action.CreateOptions, t cluster.ClusterType) (*CreateSubCmdsOptions, error) {
	var err error
	o := &CreateSubCmdsOptions{
		CreateOptions:        createOptions,
		ClusterType:          t,
		SkipSchemaValidation: false,
	}

	if o.ChartInfo, err = cluster.BuildChartInfo(t); err != nil {
		return nil, err
	}
	return o, nil
}

func buildCreateSubCmds(createOptions *action.CreateOptions) []*cobra.Command {
	var cmds []*cobra.Command

	for _, t := range cluster.SupportedTypes() {
		o, err := NewSubCmdsOptions(createOptions, t)
		if err != nil {
			fmt.Fprintf(os.Stdout, "Failed add '%s' to 'create' sub command due to %s\n", t.String(), err.Error())
			cluster.ClearCharts(t)
			continue
		}

		cmd := &cobra.Command{
			Use:     t.String() + " NAME",
			Short:   fmt.Sprintf("Create a %s cluster.", t),
			Example: buildCreateSubCmdsExamples(t),
			Run: func(cmd *cobra.Command, args []string) {
				o.Args = args
				cmdutil.CheckErr(o.CreateOptions.Complete())
				cmdutil.CheckErr(o.Complete(cmd))
				cmdutil.CheckErr(o.Validate())
				cmdutil.CheckErr(o.Run())
			},
		}

		if o.ChartInfo.Alias != "" {
			cmd.Aliases = []string{o.ChartInfo.Alias}
		}

		// common flags
		cmd.Flags().BoolVar(&o.EditBeforeCreate, "edit", o.EditBeforeCreate, "Edit the API resource before creating")
		cmd.Flags().StringVar(&o.DryRun, "dry-run", "none", `Must be "client", or "server". If with client strategy, only print the object that would be sent, and no data is actually sent. If with server strategy, submit the server-side request, but no data is persistent.`)
		cmd.Flags().Lookup("dry-run").NoOptDefVal = "unchanged"
		printer.AddOutputFlagForCreate(cmd, &o.Format, false)

		// TODO: support enable logs when the api is ready.
		// TODO: support set backup config?

		// add flags from chart values.schema.json
		util.CheckErr(addCreateFlags(cmd, o.Factory, o.ChartInfo, t.String()))

		// Schedule policy
		if cmd.Flag("pod-anti-affinity") == nil {
			cmd.Flags().StringVar(&o.PodAntiAffinity, "pod-anti-affinity", "Preferred", "Pod anti-affinity type, one of: (Preferred, Required)")
			_ = cmd.Flags().SetAnnotation("pod-anti-affinity", cobra.BashCompCustom, []string{"Preferred", "Required"})
		}
		if cmd.Flag("topology-keys") == nil {
			cmd.Flags().StringArrayVar(&o.TopologyKeys, "topology-keys", nil, "Topology keys for affinity")
		}
		if cmd.Flag("node-labels") == nil {
			cmd.Flags().StringToStringVar(&o.NodeLabels, "node-labels", nil, "Node label selector")
		}
		if cmd.Flag("tolerations") == nil {
			cmd.Flags().StringSliceVar(&o.TolerationsRaw, "tolerations", nil, `Tolerations for cluster, such as "key=value:effect,key:effect", for example '"engineType=mongo:NoSchedule", "diskType:NoSchedule"'`)
		}
		if cmd.Flag("tenancy") == nil {
			cmd.Flags().StringVar(&o.Tenancy, "tenancy", "SharedNode", "Tenancy options, one of: (SharedNode, DedicatedNode)")
			_ = cmd.Flags().SetAnnotation("tenancy", cobra.BashCompCustom, []string{"SharedNode", "DedicatedNode"})
		}
		cmds = append(cmds, cmd)
	}
	return cmds
}

func (o *CreateSubCmdsOptions) Complete(cmd *cobra.Command) error {
	var err error

	// if name is not specified, generate a random cluster name
	if o.Name == "" {
		o.Name, err = cluster.GenerateClusterName(o.Dynamic, o.Namespace)
		if err != nil {
			return err
		}
	}

	// get values from flags
	if cmd != nil {
		o.Values = getValuesFromFlags(cmd.LocalNonPersistentFlags())
	}

	// get all the rendered objects
	objs, err := o.getObjectsInfo()
	if err != nil {
		return err
	}

	// find the cluster object
	clusterObj, err := o.getClusterObj(objs)
	if err != nil {
		return err
	}

	// get clusterDef name
	spec, ok := clusterObj.Object["spec"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("cannot find spec in cluster object")
	}
	if o.ChartInfo.ComponentDef == nil {
		o.ChartInfo.ComponentDef = []string{}
	}
	if compSpec, ok := spec["componentSpecs"].([]interface{}); ok {
		for i := range compSpec {
			comp := compSpec[i].(map[string]interface{})
			if compDef, ok := comp["componentDef"]; ok {
				o.ChartInfo.ComponentDef = append(o.ChartInfo.ComponentDef, compDef.(string))
			}
		}
	}
	if shardingSpec, ok := spec["shardings"].([]interface{}); ok {
		for i := range shardingSpec {
			shard := shardingSpec[i].(map[string]interface{})
			if compSpec, ok := shard["template"].(map[string]interface{}); ok {
				if compDef, ok := compSpec["componentDef"]; ok {
					o.ChartInfo.ComponentDef = append(o.ChartInfo.ComponentDef, compDef.(string))
				}
			}
		}
	}
	if clusterDef, ok := spec["clusterDef"].(string); ok {
		o.ChartInfo.ClusterDef = clusterDef
	}
	if o.ChartInfo.ClusterDef == "" && len(o.ChartInfo.ComponentDef) == 0 {
		return fmt.Errorf("cannot find clusterDef in cluster spec or componentDef in componentSpecs or shardingSpecs")
	}

	// Build tolerations if raw toleration rules are configured
	if o.TolerationsRaw != nil {
		tolerationsResult, err := util.BuildTolerations(o.TolerationsRaw)
		if err != nil {
			return err
		}
		jsonData, err := json.Marshal(tolerationsResult)
		if err != nil {
			return err
		}
		err = json.Unmarshal(jsonData, &o.Tolerations)
		if err != nil {
			return err
		}
	}
	return nil
}

func (o *CreateSubCmdsOptions) Validate() error {
	matched, _ := regexp.MatchString(`^[a-z]([-a-z0-9]*[a-z0-9])?$`, o.Name)
	if !matched {
		return fmt.Errorf("cluster name must begin with a letter and can only contain lowercase letters, numbers, and '-'")
	}
	if len(o.Name) > 16 {
		return fmt.Errorf("cluster name should be less than 16 characters")
	}
	if o.Tenancy != "SharedNode" && o.Tenancy != "DedicatedNode" {
		return fmt.Errorf("tenancy must be one of: (SharedNode, DedicatedNode)")
	}
	if o.PodAntiAffinity != "Preferred" && o.PodAntiAffinity != "Required" {
		return fmt.Errorf("podAntiAffinity must be one of: (Preferred, Required)")
	}
	if o.TopologyKeys != nil {
		for _, key := range o.TopologyKeys {
			if key == "" {
				return fmt.Errorf("topologyKeys cannot be empty")
			}
		}
	}
	if o.NodeLabels != nil {
		for k, v := range o.NodeLabels {
			if k == "" || v == "" {
				return fmt.Errorf("nodeLabels cannot be empty")
			}
		}
	}
	if o.Tolerations != nil {
		for _, t := range o.Tolerations {
			if t.Key == "" || t.Operator == "" {
				return fmt.Errorf("tolerations cannot be empty")
			}
		}
	}
	return cluster.ValidateValues(o.ChartInfo, o.Values)
}

func (o *CreateSubCmdsOptions) Run() error {

	objs, err := o.getObjectsInfo()
	if err != nil {
		return err
	}

	getClusterObj := func() (*unstructured.Unstructured, error) {
		for _, obj := range objs {
			if obj.gvr == types.ClusterGVR() {
				return obj.obj, nil
			}
		}
		return nil, fmt.Errorf("failed to find cluster object from manifests rendered from %s chart", o.ClusterType)
	}

	clusterObj, err := getClusterObj()
	if err != nil {
		return err
	}

	spec, _ := clusterObj.Object["spec"].(map[string]interface{})
	if compSpec, ok := spec["componentSpecs"].([]interface{}); ok {
		for i := range compSpec {
			comp := compSpec[i].(map[string]interface{})
			if err := o.applySchedulingPolicyToComponent(comp); err != nil {
				return err
			}
			compSpec[i] = comp
		}
	}

	if shardingSpec, ok := spec["shardings"].([]interface{}); ok {
		for i := range shardingSpec {
			shard := shardingSpec[i].(map[string]interface{})
			if compSpec, ok := shard["template"].(map[string]interface{}); ok {
				if err := o.applySchedulingPolicyToComponent(compSpec); err != nil {
					return err
				}
				shard["template"] = compSpec
			}
		}
	}

	// only edits the cluster object, other dependency objects are created directly
	if o.EditBeforeCreate {
		customEdit := action.NewCustomEditOptions(o.Factory, o.IOStreams, "create")
		if err = customEdit.Run(clusterObj); err != nil {
			return err
		}
	}

	dryRun, err := o.GetDryRunStrategy()
	if err != nil {
		return err
	}

	// create cluster and dependency resources
	for _, obj := range objs {
		isCluster := obj.gvr == types.ClusterGVR()
		resObj := obj.obj

		if dryRun != action.DryRunClient {
			createOptions := metav1.CreateOptions{}
			if dryRun == action.DryRunServer {
				createOptions.DryRun = []string{metav1.DryRunAll}
			}

			// create resource
			resObj, err = o.Dynamic.Resource(obj.gvr).Namespace(o.Namespace).Create(context.TODO(), resObj, createOptions)
			if err != nil {
				return err
			}

			// only output cluster resource
			if dryRun != action.DryRunServer && isCluster {
				if o.Quiet {
					continue
				}
				if o.CustomOutPut != nil {
					o.CustomOutPut(o.CreateOptions)
				}
				fmt.Fprintf(o.Out, "%s %s created\n", resObj.GetKind(), resObj.GetName())
				continue
			}
		}

		if len(objs) > 1 {
			fmt.Fprintf(o.Out, "---\n")
		}

		p, err := o.ToPrinter(nil, false)
		if err != nil {
			return err
		}

		if err = p.PrintObj(resObj, o.Out); err != nil {
			return err
		}
	}
	return nil
}

// getObjectsInfo returns all objects in helm charts along with their GVK information.
func (o *CreateSubCmdsOptions) getObjectsInfo() ([]*objectInfo, error) {
	// move values that belong to sub chart to sub map
	values := buildHelmValues(o.ChartInfo, o.Values)

	// get Kubernetes version
	kubeVersion, err := util.GetK8sVersion(o.Client.Discovery())
	if err != nil || kubeVersion == "" {
		return nil, fmt.Errorf("failed to get Kubernetes version %v", err)
	}

	// get cluster manifests
	manifests, err := cluster.GetManifests(o.ChartInfo.Chart, o.SkipSchemaValidation, o.Namespace, o.Name, kubeVersion, values)
	if err != nil {
		return nil, err
	}

	// get objects to be created from manifests
	return getObjectsInfo(o.Factory, manifests)
}

func (o *CreateSubCmdsOptions) getClusterObj(objs []*objectInfo) (*unstructured.Unstructured, error) {
	for _, obj := range objs {
		fmt.Println(obj.gvr.String())
		if obj.gvr == types.ClusterGVR() {
			return obj.obj, nil
		}
	}
	return nil, fmt.Errorf("failed to find cluster object from manifests rendered from %s chart", o.ClusterType)
}

func (o *CreateSubCmdsOptions) applySchedulingPolicyToComponent(item map[string]interface{}) error {
	if compName, ok := item["name"]; ok {
		schedulingPolicy, needSet := util.BuildSchedulingPolicy(o.Tenancy, o.Name, compName.(string), o.Tolerations, o.NodeLabels, o.PodAntiAffinity, o.TopologyKeys)
		if needSet {
			converted, err := runtime.DefaultUnstructuredConverter.ToUnstructured(schedulingPolicy)
			if err != nil {
				return err
			}
			_ = unstructured.SetNestedField(item, converted, "schedulingPolicy")
		}
	}
	return nil
}
