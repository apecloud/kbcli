/*
Copyright (C) 2022-2024 ApeCloud Co., Ltd

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

package clusterdefinition

import (
	"context"
	"fmt"
	"strings"

	kbappsv1 "github.com/apecloud/kubeblocks/apis/apps/v1"
	"github.com/apecloud/kubeblocks/pkg/controller/component"
	"github.com/spf13/cobra"
	"github.com/stoewer/go-strcase"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	"k8s.io/client-go/dynamic"
	clientset "k8s.io/client-go/kubernetes"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/kubectl/pkg/util/templates"

	"github.com/apecloud/kbcli/pkg/cluster"
	"github.com/apecloud/kbcli/pkg/printer"
	"github.com/apecloud/kbcli/pkg/types"
	"github.com/apecloud/kbcli/pkg/util"
)

var (
	describeExample = templates.Examples(`
		# describe a specified cluster definition
		kbcli clusterdefinition describe myclusterdef`)
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
		Short:             "Describe ClusterDefinition.",
		Example:           describeExample,
		Aliases:           []string{"desc"},
		ValidArgsFunction: util.ResourceNameCompletionFunc(f, types.ClusterDefGVR()),
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
		return fmt.Errorf("cluster definition name should be specified")
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
		if err := o.describeClusterDef(name); err != nil {
			return err
		}
	}
	return nil
}

func (o *describeOptions) describeClusterDef(name string) error {
	// get cluster definition
	clusterDef := &kbappsv1.ClusterDefinition{}
	if err := util.GetK8SClientObject(o.dynamic, clusterDef, types.ClusterDefGVR(), "", name); err != nil {
		return err
	}
	if err := o.showClusterDef(clusterDef); err != nil {
		return err
	}
	return nil
}

func (o *describeOptions) showClusterDef(cd *kbappsv1.ClusterDefinition) error {
	if cd == nil {
		return nil
	}
	compDefList, err := o.dynamic.Resource(types.CompDefGVR()).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return err
	}
	cmpvList, err := o.dynamic.Resource(types.ComponentVersionsGVR()).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return err
	}
	printer.PrintPairStringToLine("Name", cd.Name)
	printer.PrintPairStringToLine("Status", string(cd.Status.Phase))
	if cd.Status.Message != "" {
		printer.PrintPairStringToLine("Message", cd.Status.Message)
	}
	printer.PrintLine("Topologies\n")
	for _, v := range cd.Spec.Topologies {
		o.showTopology(v, compDefList, cmpvList)
	}
	return nil
}

func (o *describeOptions) getComponentDefAndVersions(compDefList, cmpvList *unstructured.UnstructuredList, compMatchRegex string) (string, string) {
	var (
		compDefs []string
		releases []string
	)
	for _, v := range compDefList.Items {
		if component.PrefixOrRegexMatched(v.GetName(), compMatchRegex) {
			compDefs = append(compDefs, v.GetName())
			for _, cmpv := range cmpvList.Items {
				if _, ok := cmpv.GetLabels()[v.GetName()]; !ok {
					continue
				}
				cmpvObj := &kbappsv1.ComponentVersion{}
				_ = runtime.DefaultUnstructuredConverter.FromUnstructured(cmpv.Object, cmpvObj)
				for _, rule := range cmpvObj.Spec.CompatibilityRules {
					if cluster.CompatibleComponentDefs(rule.CompDefs, v.GetName()) {
						releases = append(releases, rule.Releases...)
					}
				}
				break
			}
		}
	}
	return strings.Join(compDefs, ", "), strings.Join(releases, ", ")
}

func (o *describeOptions) showTopology(topology kbappsv1.ClusterTopology, compDefList *unstructured.UnstructuredList, cmpvList *unstructured.UnstructuredList) {
	defaultStr := ""
	if topology.Default {
		defaultStr = "(default)"
	}
	printer.PrintPairStringToLine(fmt.Sprintf("%s%s", strcase.LowerCamelCase(topology.Name), defaultStr), "")
	// print components
	tbl := printer.NewTablePrinter(o.Out)
	tbl.SetHeader("\tCOMPONENT-NAME", "COMPONENT-DEFS", "SERVICE-VERSIONS")
	for _, v := range topology.Components {
		compDefs, versions := o.getComponentDefAndVersions(compDefList, cmpvList, v.CompDef)
		tbl.AddRow("\t"+v.Name, compDefs, versions)
	}
	tbl.Print()
	// print orders
	if topology.Orders != nil {
		printOrders := func(name string, orders []string) {
			if len(orders) > 0 {
				printer.PrintPairStringToLine(name, strings.Join(orders, "->"), 6)
			}
		}
		printer.PrintLine("\n    Orders")
		printOrders("Provision", topology.Orders.Provision)
		printOrders("Update", topology.Orders.Update)
		printOrders("Terminate", topology.Orders.Terminate)
	}
}
