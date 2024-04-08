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

package bench

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/kubectl/pkg/util/templates"

	"github.com/apecloud/kubebench/api/v1alpha1"

	"github.com/apecloud/kbcli/pkg/cluster"
	"github.com/apecloud/kbcli/pkg/types"
)

var (
	tpcdsDriverMap = map[string]string{
		"mysql":      "mysql",
		"postgresql": "postgresql",
	}
	tpcdsSupportedDrivers = []string{"mysql", "postgresql"}
)

type TpcdsOptions struct {
	BenchBaseOptions

	Size   int
	UseKey bool
}

var tpcdsExample = templates.Examples(`
	# tpcds on a cluster, that will exec for all steps, cleanup, prepare and run
	kbcli bench tpcds mytest --cluster mycluster --user xxx --password xxx --database mydb

	# tpcds on a cluster, but with cpu and memory limits set
	kbcli bench tpcds mytest --cluster mycluster --user xxx --password xxx --database mydb --limit-cpu 1 --limit-memory 1Gi

	# tpcds on a cluster with 10GB data
	kbcli bench tpcds mytest --cluster mycluster --user xxx --password xxx --database mydb --size 10
`)

func NewTpcdsCmd(f cmdutil.Factory, streams genericiooptions.IOStreams) *cobra.Command {
	o := &TpcdsOptions{
		BenchBaseOptions: BenchBaseOptions{
			IOStreams: streams,
			factory:   f,
		},
	}

	cmd := &cobra.Command{
		Use:     "tpcds [Step] [Benchmark]",
		Short:   "Run TPC-DS benchmark",
		Example: tpcdsExample,
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(o.Complete(args))
			cmdutil.CheckErr(o.Validate())
			cmdutil.CheckErr(o.Run())
		},
	}

	o.AddFlags(cmd)
	cmd.Flags().IntVar(&o.Size, "size", 1, "specify the scale factor of the benchmark, 1 means 1GB data")
	cmd.Flags().BoolVar(&o.UseKey, "use-key", false, "specify whether to create pk and fk, it will take extra time to create the keys")

	return cmd
}

func (o *TpcdsOptions) Complete(args []string) error {
	var err error
	var driver string
	var host string
	var port int

	if err := o.BenchBaseOptions.BaseComplete(); err != nil {
		return err
	}

	o.Step, o.name = parseStepAndName(args, "tpcds")

	o.namespace, _, err = o.factory.ToRawKubeConfigLoader().Namespace()
	if err != nil {
		return err
	}

	if o.dynamic, err = o.factory.DynamicClient(); err != nil {
		return err
	}

	if o.client, err = o.factory.KubernetesClientSet(); err != nil {
		return err
	}

	if o.ClusterName != "" {
		clusterGetter := cluster.ObjectsGetter{
			Client:    o.client,
			Dynamic:   o.dynamic,
			Name:      o.ClusterName,
			Namespace: o.namespace,
			GetOptions: cluster.GetOptions{
				WithClusterDef:     cluster.Maybe,
				WithService:        cluster.Need,
				WithPod:            cluster.Need,
				WithEvent:          cluster.Need,
				WithPVC:            cluster.Need,
				WithDataProtection: cluster.Need,
			},
		}
		if o.ClusterObjects, err = clusterGetter.Get(); err != nil {
			return err
		}
		driver, host, port, err = getDriverAndHostAndPort(o.Cluster, o.Services)
		if err != nil {
			return err
		}
	}

	// don't overwrite the driver if it's already set
	if v, ok := tpcdsDriverMap[driver]; ok && o.Driver == "" {
		o.Driver = v
	}

	// don't overwrite the host and port if they are already set
	if o.Host == "" && o.Port == 0 {
		o.Host = host
		o.Port = port
	}

	return nil
}

func (o *TpcdsOptions) Validate() error {
	if err := o.BenchBaseOptions.BaseValidate(); err != nil {
		return err
	}

	var supported bool
	for _, v := range tpcdsDriverMap {
		if o.Driver == v {
			supported = true
			break
		}
	}
	if !supported {
		return fmt.Errorf("tpcds now only supports drivers in [%s], current cluster driver is %s",
			strings.Join(tpcdsSupportedDrivers, ","), o.Driver)
	}

	if o.User == "" {
		return fmt.Errorf("user is required")
	}

	return nil
}

func (o *TpcdsOptions) Run() error {
	tpcds := v1alpha1.Tpcds{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Tpcds",
			APIVersion: types.TpcdsGVR().GroupVersion().String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      o.name,
			Namespace: o.namespace,
		},
		Spec: v1alpha1.TpcdsSpec{
			BenchCommon: v1alpha1.BenchCommon{
				ExtraArgs:   o.ExtraArgs,
				Step:        o.Step,
				Tolerations: o.Tolerations,
				Target: v1alpha1.Target{
					Driver:   o.Driver,
					Host:     o.Host,
					Port:     o.Port,
					User:     o.User,
					Password: o.Password,
					Database: o.Database,
				},
			},
			Size:   o.Size,
			UseKey: o.UseKey,
		},
	}

	// set cpu and memory if specified
	setCPUAndMemory(&tpcds.Spec.BenchCommon, o.RequestCPU, o.RequestMemory, o.LimitCPU, o.LimitMemory)

	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{},
	}
	data, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&tpcds)
	if err != nil {
		return err
	}
	obj.SetUnstructuredContent(data)

	obj, err = o.dynamic.Resource(types.TpcdsGVR()).Namespace(o.namespace).Create(context.TODO(), obj, metav1.CreateOptions{})
	if err != nil {
		return err
	}

	fmt.Fprintf(o.Out, "%s %s created\n", obj.GetKind(), obj.GetName())
	return nil
}
