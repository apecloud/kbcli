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
	"fmt"
	"strconv"
	"strings"

	"github.com/apecloud/dbctl/engines"
	"github.com/apecloud/dbctl/engines/models"
	"github.com/apecloud/dbctl/engines/register"
	appsv1alpha1 "github.com/apecloud/kubeblocks/apis/apps/v1alpha1"
	"github.com/apecloud/kubeblocks/pkg/constant"
	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/kubectl/pkg/util/templates"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/apecloud/kbcli/pkg/action"
	"github.com/apecloud/kbcli/pkg/cluster"
	"github.com/apecloud/kbcli/pkg/types"
	"github.com/apecloud/kbcli/pkg/util"
	"github.com/apecloud/kbcli/pkg/util/flags"
)

var connectExample = templates.Examples(`
		# connect to a specified cluster
		kbcli cluster connect mycluster

		# connect to a specified instance
		kbcli cluster connect -i mycluster-instance-0

		# connect to a specified component
		kbcli cluster connect mycluster --component mycomponent

		# show cli connection example, supported client: [cli, java, python, rust, php, node.js, go, .net, django] and more.
		kbcli cluster connect mycluster --client=cli`)

type componentAccount struct {
	componentName string
	secretName    string
	username      string
}

type ConnectOptions struct {
	clusterName      string
	componentName    string
	targetCluster    *appsv1alpha1.Cluster
	clientType       string
	serviceKind      string
	node             *corev1.Node
	services         []corev1.Service
	needGetReadyNode bool
	accounts         []componentAccount

	forwardSVC  string
	forwardPort string
	*action.ExecOptions
}

// NewConnectCmd returns the cmd of connecting to a cluster
func NewConnectCmd(f cmdutil.Factory, streams genericiooptions.IOStreams) *cobra.Command {
	o := &ConnectOptions{ExecOptions: action.NewExecOptions(f, streams)}
	cmd := &cobra.Command{
		Use:               "connect (NAME | -i INSTANCE-NAME)",
		Short:             "Connect to a cluster or instance.",
		Example:           connectExample,
		ValidArgsFunction: util.ResourceNameCompletionFunc(f, types.ClusterGVR()),
		Run: func(cmd *cobra.Command, args []string) {
			util.CheckErr(o.Validate(args))
			util.CheckErr(o.Complete())
			util.CheckErr(o.runShowExample())
		},
	}
	cmd.Flags().StringVarP(&o.PodName, "instance", "i", "", "The instance name to connect.")
	flags.AddComponentFlag(f, cmd, &o.componentName, "The component to connect. If not specified and no any cluster scope services, pick up the first one.")
	cmd.Flags().StringVar(&o.clientType, "client", "", "Which client connection example should be output.")
	util.CheckErr(cmd.RegisterFlagCompletionFunc("client", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		var types []string
		for _, t := range models.ClientTypes() {
			if strings.HasPrefix(t, toComplete) {
				types = append(types, t)
			}
		}
		return types, cobra.ShellCompDirectiveNoFileComp
	}))
	return cmd
}

func (o *ConnectOptions) runShowExample() error {
	// get connection info
	if err := o.getConnectionInfo(); err != nil {
		return err
	}
	if len(o.services) == 0 {
		return fmt.Errorf("cannot find any available services")
	}
	if o.needGetReadyNode {
		if err := o.getReadyNodeForNodePort(); err != nil {
			return err
		}
	}
	o.showEndpoints()
	o.showAccounts()
	o.showClientExample()
	return nil
}

func (o *ConnectOptions) Validate(args []string) error {
	if len(args) > 1 {
		return fmt.Errorf("only support to connect one cluster")
	}

	// cluster name and pod instance are mutual exclusive
	if len(o.PodName) > 0 {
		if len(args) > 0 {
			return fmt.Errorf("specify either cluster name or instance name, they are exclusive")
		}
		if len(o.componentName) > 0 {
			return fmt.Errorf("component name is valid only when cluster name is specified")
		}
	} else if len(args) == 0 {
		return fmt.Errorf("either cluster name or instance name should be specified")
	}

	// set custer name
	if len(args) > 0 {
		o.clusterName = args[0]
	}
	return nil
}

func (o *ConnectOptions) Complete() error {
	var err error
	if err = o.ExecOptions.Complete(); err != nil {
		return err
	}
	// opt 1. specified pod name
	// 1.1 get pod by name
	if len(o.PodName) > 0 {
		if o.Pod, err = o.Client.CoreV1().Pods(o.Namespace).Get(context.Background(), o.PodName, metav1.GetOptions{}); err != nil {
			return err
		}
		o.clusterName = cluster.GetPodClusterName(o.Pod)
	}

	if o.targetCluster, err = cluster.GetClusterByName(o.Dynamic, o.clusterName, o.Namespace); err != nil {
		return err
	}
	return nil
}

func (o *ConnectOptions) appendRealCompNamesWithSharding(realComponentNames *[]string, shardingCompName string) error {
	shardingComps, err := cluster.ListShardingComponents(o.Dynamic, o.clusterName, o.Namespace, shardingCompName)
	if err != nil {
		return err
	}
	if len(shardingComps) == 0 {
		return fmt.Errorf(`cannot find any component objects for sharding component "%s"`, shardingCompName)
	}
	for i := range shardingComps {
		*realComponentNames = append(*realComponentNames, shardingComps[i].Labels[constant.KBAppComponentLabelKey])
	}
	return nil
}

func (o *ConnectOptions) getConnectionInfo() error {
	var (
		realComponentNames []string
		componentDefName   string
		getter             = cluster.ObjectsGetter{
			Client:    o.Client,
			Dynamic:   o.Dynamic,
			Name:      o.clusterName,
			Namespace: o.Namespace,
			GetOptions: cluster.GetOptions{
				WithService: cluster.Need,
			},
		}
	)
	objs, err := getter.Get()
	if err != nil {
		return err
	}
	if o.PodName == "" && o.componentName == "" && len(o.targetCluster.Spec.Services) > 0 {
		// only get cluster connection info with cluster service
		return o.getConnectInfoWithClusterService(objs.Services)
	}

	matchSVCFunc := func(svc corev1.Service, compName string) bool {
		return compName == svc.Labels[constant.KBAppComponentLabelKey] ||
			compName == svc.Spec.Selector[constant.KBAppComponentLabelKey]
	}

	// get the component connection info.
	switch {
	case o.PodName != "":
		matchSVCFunc = func(svc corev1.Service, compName string) bool {
			return svc.Spec.Selector[constant.KBAppPodNameLabelKey] == o.PodName
		}
		realComponentNames = append(realComponentNames, o.Pod.Labels[constant.KBAppComponentLabelKey])
		componentDefName = o.Pod.Labels[constant.ComponentDefinitionLabelKey]
	case o.componentName != "":
		shardingSpec := o.targetCluster.Spec.GetShardingByName(o.componentName)
		if shardingSpec != nil {
			if err = o.appendRealCompNamesWithSharding(&realComponentNames, o.componentName); err != nil {
				return err
			}
			componentDefName = shardingSpec.Template.ComponentDef
		} else {
			compSpec := o.targetCluster.Spec.GetComponentByName(o.componentName)
			if compSpec == nil {
				return fmt.Errorf(`cannot found the component "%s" in the cluster "%s"`, o.componentName, o.clusterName)
			}
			componentDefName = compSpec.ComponentDef
			realComponentNames = append(realComponentNames, o.componentName)
		}
	default:
		// 2. get first component services
		if len(o.targetCluster.Spec.ComponentSpecs) > 0 {
			compSpec := o.targetCluster.Spec.ComponentSpecs[0]
			realComponentNames = append(realComponentNames, compSpec.Name)
			componentDefName = compSpec.ComponentDef
		} else if len(o.targetCluster.Spec.ShardingSpecs) > 0 {
			shardingSpec := o.targetCluster.Spec.ShardingSpecs[0]
			if err = o.appendRealCompNamesWithSharding(&realComponentNames, shardingSpec.Name); err != nil {
				return err
			}
			componentDefName = shardingSpec.Template.ComponentDef
		} else {
			return fmt.Errorf(`cannot found shardingSpecs or componentSpecs in cluster "%s"`, o.clusterName)
		}
	}
	for _, realCompName := range realComponentNames {
		if err = o.getConnectInfoWithPodOrCompService(objs.Services, realCompName, matchSVCFunc); err != nil {
			return err
		}
	}
	return o.getComponentAccounts(componentDefName, realComponentNames[0])
}

func (o *ConnectOptions) getConnectInfoWithPodOrCompService(services *corev1.ServiceList, realCompName string, match func(svc corev1.Service, realCompName string) bool) error {
	needGetHeadlessSVC := true
	for i := range services.Items {
		svc := services.Items[i]
		if match(svc, realCompName) {
			needGetHeadlessSVC = false
			o.appendService(svc)
		}
	}
	if needGetHeadlessSVC {
		return o.getCompHeadlessSVC(realCompName)
	}
	return nil
}

func (o *ConnectOptions) appendService(svc corev1.Service) {
	o.services = append(o.services, svc)
	if svc.Spec.Type == corev1.ServiceTypeNodePort {
		o.needGetReadyNode = true
	}
}

func (o *ConnectOptions) getCompHeadlessSVC(realCompName string) error {
	headlessSVC := &corev1.Service{}
	headlessSVCName := constant.GenerateDefaultComponentHeadlessServiceName(o.clusterName, realCompName)
	if err := util.GetResourceObjectFromGVR(types.ServiceGVR(), client.ObjectKey{Namespace: o.Namespace, Name: headlessSVCName}, o.Dynamic, headlessSVC); client.IgnoreNotFound(err) != nil {
		return err
	}
	if headlessSVC.Name != "" {
		o.services = append(o.services, *headlessSVC)
	}
	return nil
}

func (o *ConnectOptions) getConnectInfoWithClusterService(services *corev1.ServiceList) error {
	// key: compName value: isSharding
	componentMap := map[string]bool{}
	for _, v := range o.targetCluster.Spec.Services {
		svcName := constant.GenerateClusterServiceName(o.clusterName, v.ServiceName)
		for i := range services.Items {
			svc := services.Items[i]
			if svc.Name != svcName {
				continue
			}
			o.appendService(svc)
			if v.ComponentSelector != "" {
				componentMap[v.ComponentSelector] = false
			} else if v.ShardingSelector != "" {
				componentMap[v.ShardingSelector] = true
			}
			break
		}
	}
	for compName, isSharding := range componentMap {
		var (
			compDefName  string
			realCompName string
		)
		if isSharding {
			shardingSpec := o.targetCluster.Spec.GetShardingByName(compName)
			if shardingSpec == nil {
				continue
			}
			shardingComps, err := cluster.ListShardingComponents(o.Dynamic, o.clusterName, o.Namespace, compName)
			if err != nil {
				return err
			}
			if len(shardingComps) == 0 {
				return fmt.Errorf(`cannot find any component objects for sharding component "%s"`, compName)
			}
			realCompName = shardingComps[0].Labels[constant.KBAppComponentLabelKey]
			compDefName = shardingSpec.Template.ComponentDef
		} else {
			compSpec := o.targetCluster.Spec.GetComponentByName(compName)
			if compSpec == nil {
				continue
			}
			realCompName = compName
			compDefName = compSpec.ComponentDef
		}
		if err := o.getComponentAccounts(compDefName, realCompName); err != nil {
			return err
		}
	}
	return nil
}

func (o *ConnectOptions) getComponentAccounts(componentDefName, realCompName string) error {
	compDef, err := cluster.GetComponentDefByName(o.Dynamic, componentDefName)
	if err != nil {
		return err
	}
	o.serviceKind = compDef.Spec.ServiceKind
	for _, v := range compDef.Spec.SystemAccounts {
		if !v.InitAccount {
			continue
		}
		o.accounts = append(o.accounts, componentAccount{
			componentName: realCompName,
			secretName:    constant.GenerateAccountSecretName(o.clusterName, realCompName, v.Name),
			username:      v.Name,
		})
	}
	return nil
}

func (o *ConnectOptions) getReadyNodeForNodePort() error {
	nodes, err := o.Client.CoreV1().Nodes().List(context.TODO(), metav1.ListOptions{
		Limit: 10,
	})
	if err != nil {
		return err
	}
	for _, node := range nodes.Items {
		var nodeIsReady bool
		for _, con := range node.Status.Conditions {
			if con.Type == corev1.NodeReady {
				nodeIsReady = con.Status == corev1.ConditionTrue
				break
			}
		}
		if nodeIsReady {
			o.node = &node
			break
		}
	}
	o.node = &nodes.Items[0]
	return nil
}

func (o *ConnectOptions) getEndpointsFromNode() (string, string) {
	var (
		internal string
		external string
	)
	for _, add := range o.node.Status.Addresses {
		if add.Type == corev1.NodeInternalDNS || add.Type == corev1.NodeInternalIP {
			internal = add.Address
		}
		if add.Type == corev1.NodeExternalDNS || add.Type == corev1.NodeExternalIP {
			external = add.Address
		}
	}
	return internal, external
}

func (o *ConnectOptions) showEndpoints() {
	tbl := newTbl(o.Out, "", "COMPONENT", "SERVICE-NAME", "TYPE", "PORT", "INTERNAL", "EXTERNAL")
	for _, svc := range o.services {
		var ports []string
		compName := svc.Annotations[constant.KBAppComponentLabelKey]
		if compName == "" {
			compName = svc.Spec.Selector[constant.KBAppComponentLabelKey]
		}
		internal := fmt.Sprintf("%s.%s.svc.cluster.local", svc.Name, svc.Namespace)
		if svc.Spec.ClusterIP == corev1.ClusterIPNone {
			podName := o.PodName
			if o.PodName == "" {
				podName = "<podName>"
			}
			internal = fmt.Sprintf("%s.%s.%s.svc.cluster.local", podName, svc.Name, svc.Namespace)
		}
		external := cluster.GetExternalAddr(&svc)
		if svc.Spec.Type == corev1.ServiceTypeNodePort {
			internal, external = o.getEndpointsFromNode()
			for _, p := range svc.Spec.Ports {
				ports = append(ports, fmt.Sprintf("%s(nodePort: %s)",
					strconv.Itoa(int(p.Port)), strconv.Itoa(int(p.NodePort))))
			}
		} else {
			for _, p := range svc.Spec.Ports {
				ports = append(ports, strconv.Itoa(int(p.Port)))
			}
		}
		if o.forwardPort == "" {
			o.forwardPort = strconv.Itoa(int(svc.Spec.Ports[0].Port))
		}
		if o.forwardSVC == "" {
			if svc.Spec.ClusterIP != corev1.ClusterIPNone {
				o.forwardSVC = fmt.Sprintf("service/%s", svc.Name)
			} else {
				o.forwardSVC = o.PodName
			}
		}
		tbl.AddRow(compName, svc.Name, svc.Spec.Type, strings.Join(ports, ","), internal, external)
	}
	fmt.Fprintf(o.Out, "# you can use the following command to forward the service port to your local machine for testing the connection, using 127.0.0.1 as the host IP.\n"+
		"\tkubectl port-forward -n %s %s %s:%s\n\n", o.Namespace, o.forwardSVC, o.forwardPort, o.forwardPort)
	fmt.Fprintln(o.Out, "Endpoints:")
	tbl.Print()
}

func (o *ConnectOptions) showAccounts() {
	tbl := newTbl(o.Out, "\nAccount Secrets:", "COMPONENT", "SECRET-NAME", "USERNAME", "PASSWORD-KEY")
	for _, account := range o.accounts {
		tbl.AddRow(account.componentName, account.secretName, account.username, "<password>")
	}
	tbl.Print()
}

func (o *ConnectOptions) showClientExample() {

	engine, err := register.NewClusterCommands(o.serviceKind)
	if err != nil {
		return
	}
	fmt.Fprint(o.Out, "\n========= connection example =========\n")
	fmt.Fprint(o.Out, engine.ConnectExample(&engines.ConnectionInfo{
		Host:     "<HOST>",
		Port:     "<PORT>",
		User:     "<USERNAME>",
		Password: "<PASSWORD>",
	}, o.clientType))
}
