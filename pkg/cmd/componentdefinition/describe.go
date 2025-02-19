/*
Copyright (C) 2022-2025 ApeCloud Co., Ltd

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

package componentdefinition

import (
	"context"
	"fmt"
	"io"

	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	"k8s.io/client-go/dynamic"
	clientset "k8s.io/client-go/kubernetes"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/kubectl/pkg/util/templates"

	kbappsv1 "github.com/apecloud/kubeblocks/apis/apps/v1"
	dpv1alpha1 "github.com/apecloud/kubeblocks/apis/dataprotection/v1alpha1"
	"github.com/apecloud/kubeblocks/pkg/controller/component"
	"github.com/apecloud/kubeblocks/pkg/dataprotection/utils/boolptr"

	"github.com/apecloud/kbcli/pkg/printer"
	"github.com/apecloud/kbcli/pkg/types"
	"github.com/apecloud/kbcli/pkg/util"
)

var (
	describeExample = templates.Examples(`
		# describe a specified component definition
		kbcli componentdefinition describe mycomponentdef`)
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
		Short:             "Describe ComponentDefinition.",
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
		return fmt.Errorf("component definition name should be specified")
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
			return fmt.Errorf("error describing componentDefinitions '%s': %w", name, err)
		}
	}
	return nil
}

func (o *describeOptions) describeCmpd(name string) error {
	// get component definition
	cmpd := &kbappsv1.ComponentDefinition{}
	if err := util.GetK8SClientObject(o.dynamic, cmpd, types.CompDefGVR(), "", name); err != nil {
		return err
	}

	if err := o.showComponentDef(cmpd); err != nil {
		return err
	}
	// get backup policy template of the component definition
	if err := o.showBackupPolicy(name); err != nil {
		return err
	}
	return nil
}

func (o *describeOptions) showComponentDef(cmpd *kbappsv1.ComponentDefinition) error {
	tbl := printer.NewTablePrinter(o.Out)
	tbl.SetHeader("NAME", "SERVICE-KIND", "SERVICE-VERSIONS", "PROVIDER", "UPDATE-STRATEGY")
	tbl.AddRow(cmpd.Name,
		cmpd.Spec.ServiceKind,
		cmpd.Spec.ServiceVersion,
		cmpd.Spec.Provider,
		*cmpd.Spec.UpdateStrategy)
	tbl.Print()
	printer.PrintLine("")
	showServices(cmpd.Spec.Services, o.Out)
	showServiceRefs(cmpd.Spec.ServiceRefDeclarations, o.Out)
	showRoles(cmpd.Spec.Roles, o.Out)
	showSystemAccounts(cmpd.Spec.SystemAccounts, o.Out)

	printer.PrintPairStringToLine("Status", string(cmpd.Status.Phase), 0)
	if cmpd.Status.Message != "" {
		printer.PrintPairStringToLine("Message", cmpd.Status.Message, 0)
	}
	return nil
}

func (o *describeOptions) showBackupPolicy(name string) error {
	backupTemplatesListObj, err := o.dynamic.Resource(types.BackupPolicyTemplateGVR()).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return err
	}
	var backupPolicyTemplates []*dpv1alpha1.BackupPolicyTemplate
	// match the backupTemplate.Spec.CompDefs with componentDef name.
	for _, item := range backupTemplatesListObj.Items {
		backupTemplate := dpv1alpha1.BackupPolicyTemplate{}
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(item.Object, &backupTemplate); err != nil {
			return err
		}
		for _, compMatchRegex := range backupTemplate.Spec.CompDefs {
			if component.PrefixOrRegexMatched(name, compMatchRegex) {
				backupPolicyTemplates = append(backupPolicyTemplates, &backupTemplate)
			}
		}
	}

	showBackupConfig(backupPolicyTemplates, o.Out)
	return nil
}

func showBackupConfig(backupPolicyTemplates []*dpv1alpha1.BackupPolicyTemplate, out io.Writer) {
	if len(backupPolicyTemplates) == 0 {
		return
	}
	fmt.Fprintf(out, "\nBackup Config:\n")
	for _, backupPolicyTemplate := range backupPolicyTemplates {
		if len(backupPolicyTemplates) > 1 {
			fmt.Fprintf(out, "  Name: %s\n", backupPolicyTemplate.Name)
		}
		tbl := printer.NewTablePrinter(out)
		tbl.SetHeader("\tBACKUP-METHOD", "ACTION-SET", "SNAPSHOT-VOLUME")
		for _, method := range backupPolicyTemplate.Spec.BackupMethods {
			snapshotVolume := "false"
			if boolptr.IsSetToTrue(method.SnapshotVolumes) {
				snapshotVolume = "true"
			}
			tbl.AddRow("\t"+method.Name, method.ActionSetName, snapshotVolume)
		}
		tbl.Print()
		fmt.Fprint(out, "\n")
	}
}

func showRoles(roles []kbappsv1.ReplicaRole, out io.Writer) {
	if len(roles) == 0 {
		return
	}
	fmt.Fprintf(out, "Roles:\n")
	tbl := printer.NewTablePrinter(out)
	tbl.SetHeader("\tNAME", "WITH-QUORUM", "UPDATE-PRIORITY")
	for _, role := range roles {
		tbl.AddRow("\t"+role.Name, role.ParticipatesInQuorum, role.UpdatePriority)
	}
	tbl.Print()
	fmt.Fprint(out, "\n")
}

func showServiceRefs(serviceRefs []kbappsv1.ServiceRefDeclaration, out io.Writer) {
	if len(serviceRefs) == 0 {
		return
	}
	fmt.Fprintf(out, "ServiceRef Declarations:\n")
	tbl := printer.NewTablePrinter(out)
	tbl.SetHeader("\tNAME", "SERVICE-KIND", "SERVICE-VERSION")
	for _, sr := range serviceRefs {
		for _, srd := range sr.ServiceRefDeclarationSpecs {
			tbl.AddRow("\t"+sr.Name, srd.ServiceKind, srd.ServiceVersion)
		}
	}
	tbl.Print()
	fmt.Fprint(out, "\n")
}

func showSystemAccounts(systemAccounts []kbappsv1.SystemAccount, out io.Writer) {
	if len(systemAccounts) == 0 {
		return
	}
	fmt.Fprintf(out, "System Accounts:\n")
	tbl := printer.NewTablePrinter(out)
	tbl.SetHeader("\tNAME", "INIT-ACCOUNT")
	for _, sa := range systemAccounts {
		tbl.AddRow("\t"+sa.Name, sa.InitAccount)
	}
	tbl.Print()
	fmt.Fprint(out, "\n")
}

func showServices(services []kbappsv1.ComponentService, out io.Writer) {
	if len(services) == 0 {
		return
	}
	fmt.Fprintf(out, "Services:\n")
	tbl := printer.NewTablePrinter(out)
	tbl.SetHeader("\tNAME", "POD-SERVICE", "ROLE-SELECTOR", "PORT(S)")
	for _, svc := range services {
		var portStr = ""
		for _, port := range svc.Spec.Ports {
			portName := port.Name
			protocol := port.Protocol
			portNum := port.Port
			targetPort := port.TargetPort.IntVal
			portStr = fmt.Sprintf("%s:%d->%d/%s", portName, portNum, targetPort, protocol)
		}
		tbl.AddRow("\t"+svc.Name, *svc.PodService, svc.RoleSelector, portStr)
	}
	tbl.Print()
	fmt.Fprint(out, "\n")
}
