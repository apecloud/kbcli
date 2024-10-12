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

package view

import (
	"context"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	"k8s.io/client-go/dynamic"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/kubectl/pkg/util/templates"

	viewv1 "github.com/apecloud/kbcli/apis/view/v1"
	"github.com/apecloud/kbcli/pkg/cmd/view/chart"
	"github.com/apecloud/kbcli/pkg/types"
	"github.com/apecloud/kbcli/pkg/util"
)

var (
	watchExamples = templates.Examples(`
		# watch a view
		kbcli view watch pg-cluster-view`)

	program *tea.Program
)

func newWatchCmd(f cmdutil.Factory, streams genericiooptions.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "watch view-name",
		Short:   "watch a view.",
		Aliases: []string{"w"},
		Example: watchExamples,
		Run: func(cmd *cobra.Command, args []string) {
			util.CheckErr(watch(f, streams, args))
		},
	}
	return cmd
}

func watch(f cmdutil.Factory, streams genericiooptions.IOStreams, args []string) error {
	go doWatch(f, streams, args)
	return renderView()
}

func doWatch(f cmdutil.Factory, streams genericiooptions.IOStreams, args []string) {
	o := &watchOptions{factory: f, streams: streams, gvr: types.ViewGVR()}
	if err := o.complete(args); err != nil {
		klog.Fatal("failed to init clientset", err)
	}
	ctx := context.TODO()
	watcher, err := o.dynamic.Resource(o.gvr).Namespace(o.namespace).Watch(ctx, metav1.ListOptions{})
	if err != nil {
		klog.Fatal("failed to watch view", err)
	}
	for event := range watcher.ResultChan() {
		obj, ok := event.Object.(*unstructured.Unstructured)
		if !ok {
			continue
		}
		view := &viewv1.ReconciliationView{}
		if err = runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, view); err != nil {
			klog.Fatal("failed to convert view object", err)
		}
		if view.Name == args[0] && program != nil {
			program.Send(chart.ViewUpdateMsg{View: view})
		}
	}
}

func renderView() error {
	m := chart.NewReconciliationViewChart()
	program = tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())
	_, err := program.Run()
	return err
}

type watchOptions struct {
	factory   cmdutil.Factory
	client    clientset.Interface
	dynamic   dynamic.Interface
	namespace string

	gvr  schema.GroupVersionResource
	name string

	streams genericiooptions.IOStreams
}

func (o *watchOptions) complete(args []string) error {
	var err error

	if len(args) == 0 {
		return fmt.Errorf("a view name is required")
	}
	o.name = args[0]
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
