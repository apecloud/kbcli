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
	"github.com/pterm/pterm"

	viewv1 "github.com/apecloud/kbcli/apis/view/v1"
	"github.com/apecloud/kbcli/pkg/types"
	"github.com/apecloud/kbcli/pkg/util"
	"github.com/rivo/tview"
	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	"k8s.io/client-go/dynamic"
	clientset "k8s.io/client-go/kubernetes"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/kubectl/pkg/util/templates"
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
	main := tview.NewFlex().SetDirection(tview.FlexRow)

	bars := []pterm.Bar{
		{Label: "A", Value: 10},
		{Label: "B", Value: 20},
		{Label: "C", Value: 30},
		{Label: "D", Value: 40},
		{Label: "E", Value: 50},
		{Label: "F", Value: 40},
		{Label: "G", Value: 30},
		{Label: "H", Value: 20},
		{Label: "I", Value: 10},
	}
	barText, err := pterm.DefaultBarChart.WithBars(bars).WithHeight(5).Srender()
	if err != nil {
		return err
	}
	statBox := tview.NewTextView().
		SetTextAlign(tview.AlignCenter).
		SetText(barText)
	//SetTitleAlign(tview.AlignLeft).
	//SetBorder(true)

	change := "NaN"
	if l := len(view.Status.CurrentState.Changes); l > 0 {
		change = view.Status.CurrentState.Changes[l-1].Description
	}
	latestEventBox := tview.NewBox().SetBorder(true).SetTitle(fmt.Sprintf("Latest Change: %s", change))
	statusBar := tview.NewFlex().SetDirection(tview.FlexColumn)
	statusBar.AddItem(statBox, 0, 1, false)
	statusBar.AddItem(latestEventBox, 0, 1, false)

	objectTreeBox := tview.NewBox().SetBorder(false).SetTitle("Object Tree")
	changesBox := tview.NewBox().SetBorder(false).SetTitle("Changes")
	mainContent := tview.NewFlex().SetDirection(tview.FlexColumn)
	mainContent.AddItem(objectTreeBox, 0, 1, true)
	mainContent.AddItem(changesBox, 0, 1, false)
	mainContent.SetBorder(true)

	main.AddItem(statusBar, 0, 1, false)
	main.AddItem(mainContent, 0, 1, false)

	return tview.NewApplication().SetRoot(main, true).Run()
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
