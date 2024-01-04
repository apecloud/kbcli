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

package bench

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/kubectl/pkg/util/templates"

	"github.com/apecloud/kbcli/pkg/cluster"
	"github.com/apecloud/kbcli/pkg/types"
	"github.com/apecloud/kubebench/api/v1alpha1"
)

const (
	redisBenchDriver = "redis"
)

var redisBenchExample = templates.Examples(`
	# redis-benchmark run on a cluster
	kbcli bench redis-benchmark mytest --cluster rediscluster --clients 50 --requests 10000 --password xxx

	# redis-benchmark run on a cluster, but with cpu and memory limits set
	kbcli bench redis-benchmark mytest --cluster rediscluster --clients 50 --requests 10000 --cpu 1 --memory 1Gi --password xxx

	# redis-benchmark run on a cluster, just test set/get
	kbcli bench redis-benchmark mytest --cluster rediscluster --clients 50 --requests 10000 --tests set,get --password xxx

	# redis-benchmark run on a cluster, just test set/get with key space
	kbcli bench redis-benchmark mytest --cluster rediscluster --clients 50 --requests 10000 --tests set,get --key-space 100000 --password xxx

	# redis-benchmark run on a cluster, with pipeline
	kbcli bench redis-benchmark mytest --cluster rediscluster --clients 50 --requests 10000 --pipeline 10 --password xxx

	# redis-benchmark run on a cluster, with csv output
	kbcli bench redis-benchmark mytest --cluster rediscluster --clients 50 --requests 10000 --quiet false --extra-args "--csv" --password xxx
`)

type RedisBenchOptions struct {
	Clients  []int  // clients provides a list of client counts to run redis-benchmark with multiple times.
	Requests int    // total number of requests
	DataSize int    // data size of set/get value in bytes
	KeySpace int    // use random keys for SET/GET/INCR, random values for SADD
	Tests    string // only run the comma separated list of tests. The test names are the same as the ones produced as output.
	Pipeline int    // pipelining num requests. Default 1 (no pipeline).
	Quiet    bool   // quiet mode. Just show query/sec values

	BenchBaseOptions
}

func NewRedisBenchmarkCmd(f cmdutil.Factory, streams genericiooptions.IOStreams) *cobra.Command {
	o := &RedisBenchOptions{
		BenchBaseOptions: BenchBaseOptions{
			IOStreams: streams,
			factory:   f,
		},
	}

	cmd := &cobra.Command{
		Use:     "redis-benchmark",
		Short:   "Run redis-benchmark on a cluster",
		Example: redisBenchExample,
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(o.Complete(args))
			cmdutil.CheckErr(o.Validate())
			cmdutil.CheckErr(o.Run())
		},
	}

	o.BenchBaseOptions.AddFlags(cmd)
	cmd.Flags().IntSliceVar(&o.Clients, "clients", []int{50}, "number of parallel connections")
	cmd.Flags().IntVar(&o.Requests, "requests", 10000, "total number of requests")
	cmd.Flags().IntVar(&o.DataSize, "data-size", 3, "data size of set/get value in bytes")
	cmd.Flags().IntVar(&o.KeySpace, "key-space", 0, "use random keys for SET/GET/INCR, random values for SADD")
	cmd.Flags().StringVar(&o.Tests, "tests", "", "only run the comma separated list of tests. The test names are the same as the ones produced as output.")
	cmd.Flags().IntVar(&o.Pipeline, "pipeline", 1, "pipelining num requests. Default 1 (no pipeline).")
	cmd.Flags().BoolVar(&o.Quiet, "quiet", true, "quiet mode. Just show query/sec values")

	return cmd
}

func (o *RedisBenchOptions) Complete(args []string) error {
	var err error
	var driver string
	var host string
	var port int

	if err = o.BenchBaseOptions.BaseComplete(); err != nil {
		return err
	}

	o.Step, o.name = parseStepAndName(args, "redis-benchmark")

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

	if o.Driver == "" {
		o.Driver = driver
	}

	if o.Host == "" && o.Port == 0 {
		o.Host = host
		o.Port = port
	}

	return nil
}

func (o *RedisBenchOptions) Validate() error {
	if err := o.BaseValidate(); err != nil {
		return err
	}

	if o.Driver != redisBenchDriver {
		return fmt.Errorf("redis-benchmark only supports driver in [%s], current cluster driver is %s", redisBenchDriver, o.Driver)
	}

	return nil
}

func (o *RedisBenchOptions) Run() error {
	redisBench := v1alpha1.RedisBench{
		TypeMeta: metav1.TypeMeta{
			Kind:       "RedisBench",
			APIVersion: types.RedisBenchGVR().GroupVersion().String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      o.name,
			Namespace: o.namespace,
		},
		Spec: v1alpha1.RedisBenchSpec{
			Clients:  o.Clients,
			Requests: o.Requests,
			DataSize: o.DataSize,
			Pipeline: o.Pipeline,
			Quiet:    o.Quiet,
			BenchCommon: v1alpha1.BenchCommon{
				Tolerations: o.Tolerations,
				ExtraArgs:   o.ExtraArgs,
				Step:        o.Step,
				Target: v1alpha1.Target{
					Host: o.Host,
					Port: o.Port,
				},
			},
		},
	}

	if o.KeySpace != 0 {
		redisBench.Spec.KeySpace = &o.KeySpace
	}
	if o.Tests != "" {
		redisBench.Spec.Tests = o.Tests
	}
	if o.Password != "" {
		redisBench.Spec.Target.Password = o.Password
	}

	// set cpu and memory if specified
	setCpuAndMemory(&redisBench.Spec.BenchCommon, o.Cpu, o.Memory)

	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{},
	}
	data, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&redisBench)
	if err != nil {
		return err
	}
	obj.SetUnstructuredContent(data)

	obj, err = o.dynamic.Resource(types.RedisBenchGVR()).Namespace(o.namespace).Create(context.TODO(), obj, metav1.CreateOptions{})
	if err != nil {
		return err
	}

	fmt.Fprintf(o.Out, "%s %s created\n", obj.GetKind(), obj.GetName())

	return nil
}
