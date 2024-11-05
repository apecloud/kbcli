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

package componentversion

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	"k8s.io/client-go/dynamic"
	clientset "k8s.io/client-go/kubernetes"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/kubectl/pkg/util/templates"

	kbappsv1 "github.com/apecloud/kubeblocks/apis/apps/v1"

	"github.com/apecloud/kbcli/pkg/printer"
	"github.com/apecloud/kbcli/pkg/types"
	"github.com/apecloud/kbcli/pkg/util"
)

var (
	describeExample = templates.Examples(`
		# describe a specified componentversion
		kbcli componentversion describe mycomponentversion`)
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
		Short:             "Describe ComponentVersion.",
		Example:           describeExample,
		Aliases:           []string{"desc"},
		ValidArgsFunction: util.ResourceNameCompletionFunc(f, types.ComponentVersionsGVR()),
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
		return fmt.Errorf("compinent definition name should be specified")
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
		if err := o.describeCmpd(name); err != nil {
			return err
		}
	}
	return nil
}

func (o *describeOptions) describeCmpd(name string) error {
	// get component version
	cmpv := &kbappsv1.ComponentVersion{}
	if err := util.GetK8SClientObject(o.dynamic, cmpv, types.ComponentVersionsGVR(), "", name); err != nil {
		return err
	}

	if err := o.showComponentVersion(cmpv); err != nil {
		return err
	}
	return nil
}

func (o *describeOptions) showComponentVersion(cmpv *kbappsv1.ComponentVersion) error {
	printer.PrintPairStringToLine("Name", cmpv.Name, 0)

	showCompatibilityRules(cmpv.Spec.CompatibilityRules, o.Out)
	printer.PrintPairStringToLine("Status", string(cmpv.Status.Phase), 0)
	if cmpv.Status.Message != "" {
		printer.PrintPairStringToLine("Message", cmpv.Status.Message, 0)
	}
	return nil
}

func showCompatibilityRules(compatibilityRules []kbappsv1.ComponentVersionCompatibilityRule, out io.Writer) {
	if len(compatibilityRules) == 0 {
		return
	}
	fmt.Fprintf(out, "Compatibility Rules:\n")
	tbl := printer.NewTablePrinter(out)
	tbl.SetHeader("\tCOMPONENT-DEF-REGEX", "RELEASES")
	for _, rule := range compatibilityRules {
		tbl.AddRow("\t"+strings.Join(rule.CompDefs, ", "), strings.Join(rule.Releases, ", "))
	}
	tbl.Print()
	fmt.Fprint(out, "\n")
}
