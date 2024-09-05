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
	// unique name identifier for component object
	componentName string
	secretName    string
	username      string
}

type ConnectOptions struct {
	clusterName string
	// componentName in cluster.spec
	clusterComponentName string
	targetCluster        *appsv1alpha1.Cluster
	clientType           string
	serviceKind          string
	node                 *corev1.Node
	services             []corev1.Service
	needGetReadyNode     bool
	accounts             []componentAccount
	shardingCompMap      map[string]string

	forwardSVC  string
	forwardPort string
	*action.ExecOptions
}

// NewConnectCmd returns the cmd of connecting to a cluster
func NewConnectCmd(f cmdutil.Factory, streams genericiooptions.IOStreams) *cobra.Command {
	o := &ConnectOptions{ExecOptions: action.NewExecOptions(f, streams), shardingCompMap: map[string]string{}}
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
	flags.AddComponentFlag(f, cmd, &o.clusterComponentName, "The component to connect. If not specified and no any cluster scope services, pick up the first one.")
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
	err := o.getConnectionInfo()
	if err != nil {
		return err
	}
	if len(o.services) == 0 {
		return fmt.Errorf("cannot find any available services")
	}
	if o.needGetReadyNode {
		if o.node, err = cluster.GetReadyNodeForNodePort(o.Client); err != nil {
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
		if len(o.clusterComponentName) > 0 {
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

func (o *ConnectOptions) getConnectionInfo() error {
	var (
		componentPairs []cluster.ComponentPair
		getter         = cluster.ObjectsGetter{
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
	if o.PodName == "" && o.clusterComponentName == "" && len(o.targetCluster.Spec.Services) > 0 {
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
		componentPairs = append(componentPairs, cluster.ComponentPair{
			ComponentName:    o.Pod.Labels[constant.KBAppComponentLabelKey],
			ComponentDefName: o.Pod.Labels[constant.ComponentDefinitionLabelKey],
			ShardingName:     o.Pod.Labels[constant.KBAppShardingNameLabelKey],
		})
	case o.clusterComponentName != "":
		compSpec, isSharding := cluster.GetCompSpecAndCheckSharding(o.targetCluster, o.clusterComponentName)
		if compSpec == nil {
			return fmt.Errorf(`cannot found the component "%s" in the cluster "%s"`, o.clusterComponentName, o.clusterName)
		}
		if isSharding {
			if componentPairs, err = cluster.GetShardingComponentPairs(o.Dynamic, o.targetCluster, appsv1alpha1.ShardingSpec{
				Name:     o.clusterComponentName,
				Template: *compSpec,
			}); err != nil {
				return err
			}
		} else {
			componentPairs = append(componentPairs, cluster.ComponentPair{
				ComponentName:    compSpec.Name,
				ComponentDefName: compSpec.ComponentDef,
			})
		}
	default:
		// 2. get all component services
		componentPairs, err = cluster.GetClusterComponentPairs(o.Dynamic, o.targetCluster)
		if err != nil {
			return err
		}
	}
	for _, compPair := range componentPairs {
		if err = o.getConnectInfoWithCompService(objs.Services, compPair.ComponentName, matchSVCFunc); err != nil {
			return err
		}
		if err = o.getComponentAccounts(compPair.ComponentDefName, compPair.ComponentName); err != nil {
			return err
		}
	}
	o.setShardingCompMap(componentPairs)
	return nil
}

func (o *ConnectOptions) setShardingCompMap(componentPairs []cluster.ComponentPair) {
	for _, v := range componentPairs {
		if v.ShardingName != "" {
			o.shardingCompMap[v.ComponentName] = v.ShardingName
		}
	}
}

func (o *ConnectOptions) getConnectInfoWithCompService(services *corev1.ServiceList, componentName string, match func(svc corev1.Service, componentName string) bool) error {
	needGetHeadlessSVC := true
	for i := range services.Items {
		svc := services.Items[i]
		if match(svc, componentName) {
			needGetHeadlessSVC = false
			o.appendService(svc)
		}
	}
	if needGetHeadlessSVC {
		return o.getCompHeadlessSVC(componentName)
	}
	return nil
}

func (o *ConnectOptions) appendService(svc corev1.Service) {
	o.services = append(o.services, svc)
	if svc.Spec.Type == corev1.ServiceTypeNodePort {
		o.needGetReadyNode = true
	}
}

func (o *ConnectOptions) getCompHeadlessSVC(componentName string) error {
	headlessSVC := &corev1.Service{}
	headlessSVCName := constant.GenerateDefaultComponentHeadlessServiceName(o.clusterName, componentName)
	if err := util.GetResourceObjectFromGVR(types.ServiceGVR(), client.ObjectKey{Namespace: o.Namespace, Name: headlessSVCName}, o.Dynamic, headlessSVC); err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil
		}
		return err
	}
	if headlessSVC.Name != "" {
		o.services = append(o.services, *headlessSVC)
	}
	return nil
}

func (o *ConnectOptions) getConnectInfoWithClusterService(services *corev1.ServiceList) error {
	// key: compName value: isSharding
	clusterComponentMap := map[string]bool{}
	for _, v := range o.targetCluster.Spec.Services {
		svcName := constant.GenerateClusterServiceName(o.clusterName, v.ServiceName)
		for i := range services.Items {
			svc := services.Items[i]
			if svc.Name != svcName {
				continue
			}
			o.appendService(svc)
			if v.ComponentSelector != "" {
				clusterComponentMap[v.ComponentSelector] = false
			} else if v.ShardingSelector != "" {
				clusterComponentMap[v.ShardingSelector] = true
			}
			break
		}
	}
	for clusterCompName, isSharding := range clusterComponentMap {
		var (
			compDefName   string
			componentName string
		)
		if isSharding {
			shardingSpec := o.targetCluster.Spec.GetShardingByName(clusterCompName)
			if shardingSpec == nil {
				continue
			}
			shardingComps, err := cluster.ListShardingComponents(o.Dynamic, o.clusterName, o.Namespace, clusterCompName)
			if err != nil {
				return err
			}
			if len(shardingComps) == 0 {
				return fmt.Errorf(`cannot find any component objects for sharding component "%s"`, clusterCompName)
			}
			componentName = shardingComps[0].Labels[constant.KBAppComponentLabelKey]
			compDefName = shardingSpec.Template.ComponentDef
		} else {
			compSpec := o.targetCluster.Spec.GetComponentByName(clusterCompName)
			if compSpec == nil {
				continue
			}
			componentName = clusterCompName
			compDefName = compSpec.ComponentDef
		}
		if err := o.getComponentAccounts(compDefName, componentName); err != nil {
			return err
		}
	}
	return nil
}

func (o *ConnectOptions) getComponentAccounts(componentDefName, componentName string) error {
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
			componentName: componentName,
			secretName:    constant.GenerateAccountSecretName(o.clusterName, componentName, v.Name),
			username:      v.Name,
		})
	}
	return nil
}

func (o *ConnectOptions) showEndpoints() {
	tbl := newTbl(o.Out, "", "COMPONENT", "SERVICE-NAME", "TYPE", "PORT", "INTERNAL", "EXTERNAL")
	for _, svc := range o.services {
		var ports []string
		compName := svc.Annotations[constant.KBAppComponentLabelKey]
		if compName == "" {
			compName = svc.Spec.Selector[constant.KBAppComponentLabelKey]
		}
		if shardingName, ok := o.shardingCompMap[compName]; ok {
			compName = cluster.BuildShardingComponentName(shardingName, compName)
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
		if svc.Spec.Type == corev1.ServiceTypeNodePort && o.node != nil {
			nodeInternal, nodeExternal := cluster.GetEndpointsFromNode(o.node)
			for _, p := range svc.Spec.Ports {
				ports = append(ports, fmt.Sprintf("%s(nodePort: %s)",
					strconv.Itoa(int(p.Port)), strconv.Itoa(int(p.NodePort))))
			}
			if nodeExternal != "" {
				external = nodeExternal
			} else {
				external = nodeInternal
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
		compName := account.componentName
		if shardingName, ok := o.shardingCompMap[compName]; ok {
			compName = cluster.BuildShardingComponentName(shardingName, compName)
		}
		tbl.AddRow(compName, account.secretName, account.username, "<password>")
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
