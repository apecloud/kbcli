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
	"github.com/76creates/stickers/flexbox"
	zone "github.com/lrstanley/bubblezone"

	"github.com/NimbleMarkets/ntcharts/barchart"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	"k8s.io/client-go/dynamic"
	clientset "k8s.io/client-go/kubernetes"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/kubectl/pkg/util/templates"

	viewv1 "github.com/apecloud/kbcli/apis/view/v1"
	"github.com/apecloud/kbcli/pkg/types"
	"github.com/apecloud/kbcli/pkg/util"
)

var (
	watchExamples = templates.Examples(`
		# watch a view
		kbcli view watch pg-cluster-view`)
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
	o := &watchOptions{factory: f, streams: streams, gvr: types.ViewGVR()}
	if err := o.complete(args); err != nil {
		return err
	}
	// get view object
	ctx := context.TODO()
	obj, err := o.dynamic.Resource(o.gvr).Namespace(o.namespace).Get(ctx, o.name, metav1.GetOptions{})
	if err != nil {
		return err
	}
	view := &viewv1.ReconciliationView{}
	if err = runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, view); err != nil {
		return err
	}
	return renderView(view)
}

func renderView(view *viewv1.ReconciliationView) error {
	v1 := barchart.BarData{
		Label: "Pod",
		Values: []barchart.BarValue{
			{Name: "Initial", Value: 21.2, Style: blockStyle},
			{Name: "Added", Value: 10.1, Style: blockStyle2},
			{Name: "Updated", Value: 6.5, Style: blockStyle3},
			{Name: "Deleted", Value: 7.7, Style: blockStyle4},
		},
	}
	v2 := barchart.BarData{
		Label: "PVC",
		Values: []barchart.BarValue{
			{Name: "Initial", Value: 15.1, Style: blockStyle},
			{Name: "Added", Value: 15.1, Style: blockStyle2},
			{Name: "Updated", Value: 3.3, Style: blockStyle3},
			{Name: "Deleted", Value: 7.7, Style: blockStyle4},
		},
	}
	v3 := barchart.BarData{
		Label: "ITS",
		Values: []barchart.BarValue{
			{Name: "Initial", Value: 13.6, Style: blockStyle},
			{Name: "Added", Value: 14.1, Style: blockStyle2},
			{Name: "Updated", Value: 4.4, Style: blockStyle3},
			{Name: "Deleted", Value: 4.4, Style: blockStyle4},
		},
	}
	v4 := barchart.BarData{
		Label: "SVC",
		Values: []barchart.BarValue{
			{Name: "Initial", Value: 13.1, Style: blockStyle},
			{Name: "Added", Value: 11.1, Style: blockStyle2},
			{Name: "Updated", Value: 10.9, Style: blockStyle3},
			{Name: "Deleted", Value: 9.8, Style: blockStyle4},
		},
	}
	values := []barchart.BarData{v1, v2, v3, v4}

	// create new bubblezone Manager to enable mouse support to zoom in and out of chart
	zoneManager := zone.New()

	// all barcharts contain the same values
	// different options are displayed on the screen and below
	// and first barchart has axis and label styles
	m := &teaModel{
		view:        view,
		lv:          values,
		zM:          zoneManager,
		base:        flexbox.NewHorizontal(0, 0),
		statusBar:   flexbox.New(0, 0),
		mainContent: flexbox.New(0, 0),
	}

	_, err := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion()).Run()
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
