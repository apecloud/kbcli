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
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/crypto/ssh/terminal"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	"k8s.io/klog/v2"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/kubectl/pkg/util/templates"

	appsv1alpha1 "github.com/apecloud/kubeblocks/apis/apps/v1alpha1"
	"github.com/apecloud/kubeblocks/pkg/constant"
	"github.com/apecloud/kubeblocks/pkg/lorry/engines"
	"github.com/apecloud/kubeblocks/pkg/lorry/engines/models"
	"github.com/apecloud/kubeblocks/pkg/lorry/engines/register"

	"github.com/apecloud/kbcli/pkg/action"
	"github.com/apecloud/kbcli/pkg/cluster"
	"github.com/apecloud/kbcli/pkg/types"
	"github.com/apecloud/kbcli/pkg/util"
	"github.com/apecloud/kbcli/pkg/util/flags"
)

var connectExample = templates.Examples(`
		# connect to a specified cluster, default connect to the leader/primary instance
		kbcli cluster connect mycluster

		# connect to cluster as user
		kbcli cluster connect mycluster --as-user myuser

		# connect to a specified instance
		kbcli cluster connect -i mycluster-instance-0

		# connect to a specified component
		kbcli cluster connect mycluster --component mycomponent

		# show cli connection example with password mask
		kbcli cluster connect mycluster --show-example --client=cli

		# show java connection example with password mask
		kbcli cluster connect mycluster --show-example --client=java

		# show all connection examples with password mask
		kbcli cluster connect mycluster --show-example

		# show cli connection examples with real password
		kbcli cluster connect mycluster --show-example --client=cli --show-password`)

const passwordMask = "******"

// nonConnectiveEngines refer to the clusterdefinition or componentdefinition label 'app.kubernetes.io/name'
var nonConnectiveEngines = []string{
	string(models.PolarDBX),
	"starrocks",
}

type ConnectOptions struct {
	clusterName   string
	componentName string

	clientType   string
	showExample  bool
	showPassword bool
	engine       engines.ClusterCommands

	privateEndPoint bool
	svc             *corev1.Service

	component        *appsv1alpha1.ClusterComponentSpec
	componentDef     *appsv1alpha1.ClusterComponentDefinition
	targetCluster    *appsv1alpha1.Cluster
	targetClusterDef *appsv1alpha1.ClusterDefinition

	// componentDefV2 refer to the *appsv1alpha1.ComponentDefinition
	componentDefV2 *appsv1alpha1.ComponentDefinition
	// componentV2 refer to the *appsv1alpha1.Component
	componentV2 *appsv1alpha1.Component

	characterType string
	userName      string
	userPasswd    string

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
			if o.showExample {
				util.CheckErr(o.runShowExample())
			} else {
				util.CheckErr(o.Connect())
			}
		},
	}
	cmd.Flags().StringVarP(&o.PodName, "instance", "i", "", "The instance name to connect.")
	flags.AddComponentFlag(f, cmd, &o.componentName, "The component to connect. If not specified, pick up the first one.")
	cmd.Flags().BoolVar(&o.showExample, "show-example", false, "Show how to connect to cluster/instance from different clients.")
	cmd.Flags().BoolVar(&o.showPassword, "show-password", false, "Show password in example.")

	cmd.Flags().StringVar(&o.clientType, "client", "", "Which client connection example should be output, only valid if --show-example is true.")

	cmd.Flags().StringVar(&o.userName, "as-user", "", "Connect to cluster as user")

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
	info, err := o.getConnectionInfo()
	if err != nil {
		return err
	}
	// make sure engine is initialized
	if o.engine == nil {
		return fmt.Errorf("engine is not initialized yet")
	}

	// if cluster does not have public endpoints, prompts to use port-forward command and
	// connect cluster from localhost
	if o.privateEndPoint {
		fmt.Fprintf(o.Out, "# cluster %s does not have public endpoints, you can run following command and connect cluster from localhost\n"+
			"kubectl port-forward service/%s %s:%s\n\n", o.clusterName, o.svc.Name, info.Port, info.Port)
		info.Host = "127.0.0.1"
	}

	fmt.Fprint(o.Out, o.engine.ConnectExample(info, o.clientType))
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

	// validate user name and password
	if len(o.userName) > 0 {
		// read password from stdin
		fmt.Print("Password: ")
		if bytePassword, err := terminal.ReadPassword(int(os.Stdin.Fd())); err != nil {
			return err
		} else {
			o.userPasswd = string(bytePassword)
		}
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
		o.componentName = cluster.GetPodComponentName(o.Pod)
	}

	// cannot infer characterType from pod directly (neither from pod annotation nor pod label)
	// so we have to get cluster definition first to get characterType
	// opt 2. specified cluster name
	// 2.1 get cluster by name
	if o.targetCluster, err = cluster.GetClusterByName(o.Dynamic, o.clusterName, o.Namespace); err != nil {
		return err
	}
	// get cluster def
	if tempClusterDef, err := cluster.GetClusterDefByName(o.Dynamic, o.targetCluster.Spec.ClusterDefRef); err == nil {
		o.targetClusterDef = tempClusterDef
	}

	// 2.2 fill component name, use the first component by default
	if len(o.componentName) == 0 {
		o.component = &o.targetCluster.Spec.ComponentSpecs[0]
		o.componentName = o.component.Name
	} else {
		// verify component
		if o.component = o.targetCluster.Spec.GetComponentByName(o.componentName); o.component == nil {
			return fmt.Errorf("failed to get component %s. Check the list of components use: \n\tkbcli cluster list-components %s -n %s", o.componentName, o.clusterName, o.Namespace)
		}
	}

	// 2.3 get character type
	if err = o.getClusterCharacterType(); err != nil {
		return fmt.Errorf("failed to get component def form cluster definition and component def: %s", err.Error())
	}

	// 2.4 precheck if the engine type is supported
	if err = o.checkUnsupportedEngine(); err != nil {
		return err
	}

	// 2.5. get pod to connect, make sure o.clusterName, o.componentName are set before this step
	if len(o.PodName) == 0 {
		if err = o.getTargetPod(); err != nil {
			return err
		}
		if o.Pod, err = o.Client.CoreV1().Pods(o.Namespace).Get(context.TODO(), o.PodName, metav1.GetOptions{}); err != nil {
			return err
		}
	}
	return nil
}

// Connect creates connection string and connects to cluster
func (o *ConnectOptions) Connect() error {
	var err error

	if o.engine, err = register.NewClusterCommands(o.characterType); err != nil {
		return err
	}

	var authInfo *engines.AuthInfo
	if len(o.userName) > 0 {
		authInfo = &engines.AuthInfo{}
		authInfo.UserName = o.userName
		authInfo.UserPasswd = o.userPasswd
	} else if authInfo, err = o.getAuthInfo(); err != nil {
		// use default authInfo defined in lorry.engine
		klog.V(1).ErrorS(err, "kbcli connect failed to get getAuthInfo")
		authInfo = nil
	}

	o.ExecOptions.ContainerName = o.engine.Container()
	o.ExecOptions.Command = o.engine.ConnectCommand(authInfo)
	if klog.V(1).Enabled() {
		fmt.Fprintf(o.Out, "connect with cmd: %s", o.ExecOptions.Command)
	}
	return o.ExecOptions.Run()
}

func (o *ConnectOptions) getAuthInfo() (*engines.AuthInfo, error) {
	getter := cluster.ObjectsGetter{
		Client:    o.Client,
		Dynamic:   o.Dynamic,
		Name:      o.clusterName,
		Namespace: o.Namespace,
		GetOptions: cluster.GetOptions{
			WithClusterDef: cluster.Maybe,
			WithService:    cluster.Need,
			WithSecret:     cluster.Need,
			WithCompDef:    cluster.Maybe,
			WithComp:       cluster.Maybe,
		},
	}

	objs, err := getter.Get()
	if err != nil {
		return nil, err
	}
	if o.componentDefV2 != nil {
		o.componentV2 = getClusterCompByCompDef(objs.Components, o.componentDefV2.Name)
	}
	if user, passwd, err := getUserAndPassword(objs.ClusterDef, o.componentDefV2, o.componentV2, objs.Secrets); err != nil {
		return nil, err
	} else {
		return &engines.AuthInfo{
			UserName:   user,
			UserPasswd: passwd,
		}, nil
	}
}

func (o *ConnectOptions) getTargetPod() error {
	// make sure cluster name and component name are set
	if len(o.clusterName) == 0 {
		return fmt.Errorf("cluster name is not set yet")
	}
	if len(o.componentName) == 0 {
		return fmt.Errorf("component name is not set yet")
	}

	// get instances for given cluster name and component name
	infos := cluster.GetSimpleInstanceInfosForComponent(o.Dynamic, o.clusterName, o.componentName, o.Namespace)
	if len(infos) == 0 || infos[0].Name == constant.ComponentStatusDefaultPodName {
		return fmt.Errorf("failed to find the instance to connect, please check cluster status")
	}

	o.PodName = infos[0].Name

	// print instance info that we connect
	if len(infos) == 1 {
		fmt.Fprintf(o.Out, "Connect to instance %s\n", o.PodName)
		return nil
	}

	// output all instance infos
	var nameRoles = make([]string, len(infos))
	for i, info := range infos {
		if len(info.Role) == 0 {
			nameRoles[i] = info.Name
		} else {
			nameRoles[i] = fmt.Sprintf("%s(%s)", info.Name, info.Role)
		}
	}
	fmt.Fprintf(o.Out, "Connect to instance %s: out of %s\n", o.PodName, strings.Join(nameRoles, ", "))
	return nil
}

func (o *ConnectOptions) getConnectionInfo() (*engines.ConnectionInfo, error) {
	// make sure component and componentDef are set before this step
	if o.component == nil && o.componentDef == nil {
		return nil, fmt.Errorf("failed to get component or component def")
	}

	info := &engines.ConnectionInfo{}
	getter := cluster.ObjectsGetter{
		Client:    o.Client,
		Dynamic:   o.Dynamic,
		Name:      o.clusterName,
		Namespace: o.Namespace,
		GetOptions: cluster.GetOptions{
			WithClusterDef: cluster.Maybe,
			WithService:    cluster.Need,
			WithSecret:     cluster.Need,
			WithCompDef:    cluster.Maybe,
			WithComp:       cluster.Maybe,
		},
	}

	objs, err := getter.Get()
	if err != nil {
		return nil, err
	}

	info.ClusterName = o.clusterName
	info.ComponentName = o.componentName
	info.HeadlessEndpoint = getOneHeadlessEndpoint(objs.ClusterDef, objs.Secrets)
	// get username and password
	if o.componentDefV2 != nil {
		o.componentV2 = getClusterCompByCompDef(objs.Components, o.componentDefV2.Name)
	}
	if info.User, info.Password, err = getUserAndPassword(objs.ClusterDef, o.componentDefV2, o.componentV2, objs.Secrets); err != nil {
		return nil, err
	}
	if !o.showPassword {
		info.Password = passwordMask
	}
	// get host and port, use external endpoints first, if external endpoints are empty,
	// use internal endpoints

	// TODO: now the primary component is the first component, that may not be correct,
	// maybe show all components connection info in the future.
	internalSvcs, externalSvcs := cluster.GetComponentServices(objs.Services, o.component)
	switch {
	case len(externalSvcs) > 0:
		// cluster has public endpoints
		o.svc = externalSvcs[0]
		info.Host = cluster.GetExternalAddr(o.svc)
		info.Port = fmt.Sprintf("%d", o.svc.Spec.Ports[0].Port)
	case len(internalSvcs) > 0:
		// cluster does not have public endpoints
		o.svc = internalSvcs[0]
		info.Host = o.svc.Spec.ClusterIP
		info.Port = fmt.Sprintf("%d", o.svc.Spec.Ports[0].Port)
		o.privateEndPoint = true
	default:
		// find no endpoints
		return nil, fmt.Errorf("failed to find any cluster endpoints")
	}
	if o.engine, err = register.NewClusterCommands(o.characterType); err != nil {
		return nil, err
	}

	return info, nil
}

// getUserAndPassword gets cluster user and password from secrets
// TODO:@shanshanying, should use admin user and password. Use root credential for now.
func getUserAndPassword(clusterDef *appsv1alpha1.ClusterDefinition, compDef *appsv1alpha1.ComponentDefinition, comp *appsv1alpha1.Component, secrets *corev1.SecretList) (string, string, error) {
	var (
		user, password = "", ""
		err            error
	)

	if len(secrets.Items) == 0 {
		return user, password, fmt.Errorf("failed to find the cluster username and password")
	}

	getPasswordKey := func(connectionCredential map[string]string) string {
		for k := range connectionCredential {
			if strings.Contains(k, "password") {
				return k
			}
		}
		return "password"
	}

	getSecretVal := func(secret *corev1.Secret, key string) (string, error) {
		val, ok := secret.Data[key]
		if !ok {
			return "", fmt.Errorf("failed to find the cluster %s", key)
		}
		return string(val), nil
	}

	// now, we only use the first secret
	var secret *corev1.Secret
	for i, s := range secrets.Items {
		if strings.Contains(s.Name, "conn-credential") {
			secret = &secrets.Items[i]
			break
		}
	}
	// 0.8 API connect
	if secret == nil && compDef != nil && comp != nil {
		for _, account := range compDef.Spec.SystemAccounts {
			if !account.InitAccount {
				continue
			}
			for i, s := range secrets.Items {
				if s.Name == fmt.Sprintf("%s-account-%s", comp.Name, account.Name) {
					secret = &secrets.Items[i]
					break
				}
			}
			if secret != nil {
				break
			}
		}
		if secret == nil {
			return "", "", fmt.Errorf("failed to get the username and password by cluster component definition")
		}
		user, err = getSecretVal(secret, "username")
		if err != nil {
			return user, password, err
		}
		password, err = getSecretVal(secret, "password")
		return user, password, err
	}
	user, err = getSecretVal(secret, "username")
	if err != nil {
		return user, password, err
	}

	passwordKey := getPasswordKey(clusterDef.Spec.ConnectionCredential)
	password, err = getSecretVal(secret, passwordKey)
	return user, password, err
}

// getOneHeadlessEndpoint gets cluster headlessEndpoint from secrets
func getOneHeadlessEndpoint(clusterDef *appsv1alpha1.ClusterDefinition, secrets *corev1.SecretList) string {
	if len(secrets.Items) == 0 {
		return ""
	}
	val, ok := secrets.Items[0].Data["headlessEndpoint"]
	if !ok {
		return ""
	}
	return string(val)
}

// getClusterCharacterType will get the cluster character type compatible with 0.7
// If both componentDefRef and componentDef are provided, the componentDef will take precedence over componentDefRef.
func (o *ConnectOptions) getClusterCharacterType() error {
	if o.component.ComponentDef != "" {
		o.componentDefV2 = &appsv1alpha1.ComponentDefinition{}
		if err := util.GetK8SClientObject(o.Dynamic, o.componentDefV2, types.CompDefGVR(), "", o.component.ComponentDef); err != nil {
			return err
		}
		o.characterType = o.componentDefV2.Spec.ServiceKind
		return nil
	}
	o.componentDef = o.targetClusterDef.GetComponentDefByName(o.component.ComponentDefRef)
	if o.componentDef == nil {
		return fmt.Errorf("failed to get component def :%s", o.component.ComponentDefRef)
	}
	o.characterType = o.componentDef.CharacterType
	return nil
}

func (o *ConnectOptions) checkUnsupportedEngine() error {
	var engineType string
	if o.componentDefV2 != nil {
		engineType = o.componentDefV2.Labels[types.AddonNameLabelKey]
	}
	if o.targetClusterDef != nil && engineType == "" {
		engineType = o.targetClusterDef.Labels[types.AddonNameLabelKey]
	}
	if engineType == "" {
		return fmt.Errorf("fail to get the target engine type")
	}
	for i := range nonConnectiveEngines {
		if engineType == nonConnectiveEngines[i] {
			return fmt.Errorf("unsupported engine type:%s", o.characterType)
		}
	}
	return nil
}

func getClusterCompByCompDef(comps []*appsv1alpha1.Component, compDefName string) *appsv1alpha1.Component {
	// get 0.8 API component
	if len(comps) == 0 {
		return nil
	}
	for i, comp := range comps {
		labels := comp.Labels
		if labels == nil {
			continue
		}
		if name := labels[constant.ComponentDefinitionLabelKey]; name == compDefName {
			return comps[i]
		}
	}

	return nil
}
