/*
Copyright (C) 2022-2025 ApeCloud Co., Ltd

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

package kubeblocks

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	"k8s.io/client-go/kubernetes"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/kubectl/pkg/util/templates"

	"github.com/apecloud/kbcli/pkg/printer"
	"github.com/apecloud/kbcli/pkg/spinner"
	"github.com/apecloud/kbcli/pkg/types"
	"github.com/apecloud/kbcli/pkg/util"
	"github.com/apecloud/kbcli/pkg/util/helm"
)

var showAllConfig = false
var filterConfig = ""

// keyWhiteList is a list of which kubeblocks configs are rolled out by default
var keyWhiteList = []string{
	"addonController",
	"dataProtection",
	"affinity",
	"tolerations",
}

var sensitiveValues = []string{
	"cloudProvider.accessKey",
	"cloudProvider.secretKey",
}

var backupConfigExample = templates.Examples(`
		# Enable the snapshot-controller and volume snapshot, to support snapshot backup.
		kbcli kubeblocks config --set snapshot-controller.enabled=true
	`)

var describeConfigExample = templates.Examples(`
		# Describe the KubeBlocks config.
		kbcli kubeblocks describe-config
		# Describe all the KubeBlocks configs
		kbcli kubeblocks describe-config --all
		# Describe the desired KubeBlocks configs by filter conditions
		kbcli kubeblocks describe-config --filter=addonController,affinity
`)

// NewConfigCmd creates the config command
func NewConfigCmd(f cmdutil.Factory, streams genericiooptions.IOStreams) *cobra.Command {
	o := &InstallOptions{
		Options: Options{
			IOStreams: streams,
			Wait:      true,
		},
	}

	cmd := &cobra.Command{
		Use:     "config",
		Short:   "KubeBlocks config.",
		Example: backupConfigExample,
		Args:    cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			util.CheckErr(o.Complete(f, cmd))
			util.CheckErr(configKBRelease(o))
			util.CheckErr(markKubeBlocksPodsToLoadConfigMap(o.Client))
		},
	}
	helm.AddValueOptionsFlags(cmd.Flags(), &o.ValueOpts)
	cmd.Flags().StringVarP(&o.Namespace, "namespace", "n", "", "KubeBlocks namespace")
	return cmd
}

func configKBRelease(o *InstallOptions) error {
	kbRelease, err := o.getKBRelease()
	if err != nil {
		return err
	}
	if helm.ValueOptsIsEmpty(&o.ValueOpts) {
		fmt.Fprint(o.Out, "--set should be specified.\n")
		return nil
	}
	var kbVersion string
	if kbRelease != nil && kbRelease.Chart != nil && kbRelease.Chart.Metadata != nil {
		kbVersion = kbRelease.Chart.Metadata.Version
	}
	s := spinner.New(o.Out, spinnerMsg("Config KubeBlocks "+kbVersion))
	defer s.Fail()
	o.disableHelmPreHookJob()
	// upgrade KubeBlocks chart
	if err = o.upgradeChart(); err != nil {
		return err
	}
	// successfully upgraded
	s.Success()
	return nil
}

func NewDescribeConfigCmd(f cmdutil.Factory, streams genericiooptions.IOStreams) *cobra.Command {
	o := &InstallOptions{
		Options: Options{
			IOStreams: streams,
		},
	}
	var output printer.Format
	cmd := &cobra.Command{
		Use:     "describe-config",
		Short:   "Describe KubeBlocks config.",
		Example: describeConfigExample,
		Args:    cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			util.CheckErr(o.Complete(f, cmd))
			util.CheckErr(describeConfig(o, output, getHelmValues))
		},
	}
	printer.AddOutputFlag(cmd, &output)
	cmd.Flags().StringVarP(&o.Namespace, "namespace", "n", "", "KubeBlocks namespace")
	cmd.Flags().BoolVarP(&showAllConfig, "all", "A", false, "show all kubeblocks configs value")
	cmd.Flags().StringVar(&filterConfig, "filter", "", "filter the desired kubeblocks configs, multiple filtered strings are comma separated")
	return cmd
}

// getHelmValues gets all kubeblocks values by helm and filter the addons values
func getHelmValues(release string, opt *Options) (map[string]interface{}, error) {
	if len(opt.HelmCfg.Namespace()) == 0 {
		namespace, err := util.GetKubeBlocksNamespace(opt.Client, opt.Namespace)
		if err != nil {
			return nil, err
		}
		opt.HelmCfg.SetNamespace(namespace)
	}
	values, err := helm.GetValues(release, opt.HelmCfg)
	if err != nil {
		return nil, err
	}
	// filter the addons values
	list, err := opt.Dynamic.Resource(types.AddonGVR()).List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	for _, item := range list.Items {
		delete(values, item.GetName())
	}
	// encrypted the sensitive values
	for _, key := range sensitiveValues {
		sp := strings.Split(key, ".")
		rootKey := sp[0]
		if node, ok := values[rootKey]; ok {
			encryptNodeData(values, node, sp, 0)
		}
	}
	return pruningConfigResults(values), nil
}

// encryptNodeData encrypts the specified key of helm values. will ignore the key if the type of the value is in [map, slice].
func encryptNodeData(parentNode map[string]interface{}, node interface{}, sp []string, index int) {
	switch v := node.(type) {
	case map[string]interface{}:
		// do nothing, if target node is not the leaf node
		if len(sp)-1 == index {
			return
		}
		index += 1
		encryptNodeData(v, v[sp[index]], sp, index)
	case []interface{}:
		// ignore slice ?
	default:
		// reach the leaf node, encrypt the value
		key := sp[index]
		if _, ok := parentNode[key]; ok {
			parentNode[key] = "******"
		}
	}
}

// pruningConfigResults prunes the configs results by options
func pruningConfigResults(configs map[string]interface{}) map[string]interface{} {
	if showAllConfig {
		return configs
	}
	if filterConfig != "" {
		keyWhiteList = strings.Split(filterConfig, ",")
	}
	res := make(map[string]interface{}, len(keyWhiteList))
	for _, whiteKey := range keyWhiteList {
		res[whiteKey] = configs[whiteKey]
	}
	return res
}

type fn func(release string, opt *Options) (map[string]interface{}, error)

// describeConfig outputs the configs got by the fn in specified format
func describeConfig(o *InstallOptions, format printer.Format, f fn) error {
	values, err := f(types.KubeBlocksReleaseName, &o.Options)
	if err != nil {
		return err
	}
	printer.PrintHelmValues(values, format, o.Out)
	return nil
}

// markKubeBlocksPodsToLoadConfigMap marks an annotation of the KubeBlocks pods to load the projected volumes of configmap.
// kubelet periodically requeues the Pod every 60-90 seconds, exactly the time it takes for Secret/ConfigMaps can be loaded in the config volumes.
func markKubeBlocksPodsToLoadConfigMap(client kubernetes.Interface) error {
	deploy, err := util.GetKubeBlocksDeploy(client)
	if err != nil {
		return err
	}
	if deploy == nil {
		return nil
	}
	pods, err := client.CoreV1().Pods(deploy.Namespace).List(context.Background(), metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/name=" + types.KubeBlocksChartName,
	})
	if err != nil {
		return err
	}
	for _, pod := range pods.Items {
		// mark the pod to load configmap
		if pod.Annotations == nil {
			pod.Annotations = map[string]string{}
		}
		pod.Annotations[types.ReloadConfigMapAnnotationKey] = time.Now().Format(time.RFC3339Nano)
		_, _ = client.CoreV1().Pods(deploy.Namespace).Update(context.TODO(), &pod, metav1.UpdateOptions{})
	}
	return nil
}
