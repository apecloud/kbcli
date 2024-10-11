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

package chart

import (
	"fmt"

	"github.com/76creates/stickers/flexbox"
	"github.com/NimbleMarkets/ntcharts/barchart"
	"github.com/NimbleMarkets/ntcharts/canvas/runes"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/tree"
	zone "github.com/lrstanley/bubblezone"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/pointer"

	viewv1 "github.com/apecloud/kbcli/apis/view/v1"
	"github.com/apecloud/kbcli/pkg/cmd/view/chart/summary"
	"github.com/apecloud/kbcli/pkg/cmd/view/chart/timeserieslinechart"
)

// Model defines the BubbleTea Model of the ReconciliationView.

var defaultStyle = lipgloss.NewStyle().
	BorderStyle(lipgloss.NormalBorder()).
	BorderForeground(lipgloss.Color("63")) // purple

var axisStyle = lipgloss.NewStyle().
	Foreground(lipgloss.Color("3")) // yellow

var labelStyle = lipgloss.NewStyle().
	Foreground(lipgloss.Color("6")) // cyan

var blockStyle = lipgloss.NewStyle().
	Foreground(lipgloss.Color("6")) // cyan

var blockStyle2 = lipgloss.NewStyle().
	Foreground(lipgloss.Color("2")) // green

var blockStyle3 = lipgloss.NewStyle().
	Foreground(lipgloss.Color("3")) // yellow

var blockStyle4 = lipgloss.NewStyle().
	Foreground(lipgloss.Color("9")) // red

var style4 = lipgloss.NewStyle().
	Border(lipgloss.NormalBorder(), true).
	BorderForeground(lipgloss.Color("#26de81"))

var style5 = lipgloss.NewStyle().
	Border(lipgloss.NormalBorder(), true).
	BorderForeground(lipgloss.Color("#2bcbba"))

var style6 = lipgloss.NewStyle().
	Align(lipgloss.Center, lipgloss.Center)

var tsGraphLineStyle1 = lipgloss.NewStyle().
	Foreground(lipgloss.Color("4")) // blue

var tsAxisStyle = lipgloss.NewStyle().
	Foreground(lipgloss.Color("3")) // yellow

var tsLabelStyle = lipgloss.NewStyle().
	Foreground(lipgloss.Color("6")) // cyan

type Model struct {
	// base framework
	base        *flexbox.HorizontalFlexBox
	statusBar   *flexbox.FlexBox
	mainContent *flexbox.FlexBox

	// summary
	summary *summary.Model

	// latest change
	latestChange viewport.Model

	// current object tree
	objectTree *tree.Tree

	// changes
	changes timeserieslinechart.Model

	zM *zone.Manager

	view *viewv1.ReconciliationView
}

func (m *Model) Init() tea.Cmd {
	columns := []*flexbox.Column{
		m.base.NewColumn().AddCells(
			flexbox.NewCell(1, 1).SetStyle(style4),
			flexbox.NewCell(1, 2).SetStyle(style5),
		),
	}
	m.base.AddColumns(columns)

	statusRow := m.statusBar.NewRow().AddCells(
		flexbox.NewCell(1, 1).SetContent("Summary"),
		flexbox.NewCell(1, 1).SetStyle(style6).SetContent("Latest Change"),
	)
	m.statusBar.AddRows([]*flexbox.Row{statusRow})

	mainRow := m.mainContent.NewRow().AddCells(
		flexbox.NewCell(1, 1).SetContent("ObjectTree"),
		flexbox.NewCell(3, 1).SetContent("Changes"),
	)
	m.mainContent.AddRows([]*flexbox.Row{mainRow})

	return nil
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		}
	case tea.MouseMsg:
		if msg.Action == tea.MouseActionPress {
			m.summary.Update(msg)
		}
	case tea.WindowSizeMsg:
		m.base.SetWidth(msg.Width)
		m.base.SetHeight(msg.Height)
	}
	return m, nil
}

func (m *Model) View() string {
	tree.Root("Cluster").Child()
	m.base.ForceRecalculate()
	m.statusBar.SetWidth(m.base.GetColumn(0).GetCell(0).GetWidth())
	m.statusBar.SetHeight(m.base.GetColumn(0).GetCell(0).GetHeight())
	summaryCell := m.statusBar.GetRow(0).GetCell(0)
	summaryCell.SetMinWidth(50)
	m.statusBar.ForceRecalculate()

	if summaryCell.GetHeight() > 0 {
		var dataSet []barchart.BarData
		for i := range m.view.Status.CurrentState.Summary.ObjectSummaries {
			n := normalize(&m.view.Status.CurrentState.Summary.ObjectSummaries[i])
			d := barchart.BarData{
				Label: n.ObjectType.Kind,
				Values: []barchart.BarValue{
					{Name: "Total", Value: float64(n.Total), Style: blockStyle},
					{Name: "Added", Value: float64(*n.ChangeSummary.Added), Style: blockStyle2},
					{Name: "Updated", Value: float64(*n.ChangeSummary.Updated), Style: blockStyle3},
					{Name: "Deleted", Value: float64(*n.ChangeSummary.Deleted), Style: blockStyle4},
				},
			}
			dataSet = append(dataSet, d)
		}
		m.summary = summary.New(summaryCell.GetWidth(), summaryCell.GetHeight(),
			dataSet,
			barchart.WithZoneManager(m.zM),
			barchart.WithStyles(axisStyle, labelStyle))
		summaryCell.SetContent(m.summary.View())
	}

	change := ""
	if l := len(m.view.Status.CurrentState.Changes); l > 0 {
		change = m.view.Status.CurrentState.Changes[l-1].Description
	}
	change = defaultStyle.Render(change)
	m.statusBar.GetRow(0).GetCell(1).SetContent(change)

	m.objectTree = buildObjectTree(m.view.Status.CurrentState.ObjectTree)

	m.mainContent.SetWidth(m.base.GetColumn(0).GetCell(1).GetWidth())
	m.mainContent.SetHeight(m.base.GetColumn(0).GetCell(1).GetHeight())
	m.mainContent.ForceRecalculate()

	m.mainContent.GetRow(0).GetCell(0).SetContent(m.objectTree.String())

	if m.mainContent.GetRow(0).GetCell(1).GetWidth() > 0 {
		minYValue := 0.0
		maxYValue := 100.0
		changesChart := timeserieslinechart.New(m.mainContent.GetRow(0).GetCell(1).GetWidth()-2, m.mainContent.GetHeight()-2)
		changesChart.AxisStyle = tsAxisStyle
		changesChart.LabelStyle = tsLabelStyle
		changesChart.XLabelFormatter = timeserieslinechart.HourTimeLabelFormatter()
		changesChart.UpdateHandler = timeserieslinechart.SecondUpdateHandler(1)
		changesChart.SetYRange(minYValue, maxYValue)     // set expected Y values (values can be less or greater than what is displayed)
		changesChart.SetViewYRange(minYValue, maxYValue) // setting display Y values will fail unless set expected Y values first
		changesChart.SetStyle(tsGraphLineStyle1)
		changesChart.SetLineStyle(runes.ThinLineStyle) // ThinLineStyle replaces default linechart arcline rune style
		changesChart.SetZoneManager(m.zM)
		m.changes = changesChart
		depthMap := make(map[corev1.ObjectReference]float64)
		buildDepthMap(m.view.Status.CurrentState.ObjectTree, 0, depthMap)
		for _, change := range m.view.Status.CurrentState.Changes {
			objRef := change.ObjectReference
			objRef.UID = ""
			objRef.ResourceVersion = ""
			m.changes.Push(timeserieslinechart.TimePoint{change.Timestamp.Time, depthMap[objRef]})
		}
		m.changes.DrawBraille()
		m.mainContent.GetRow(0).GetCell(1).SetContent(m.changes.View())
	}

	m.base.GetColumn(0).GetCell(0).SetContent(m.statusBar.Render())
	m.base.GetColumn(0).GetCell(1).SetContent(m.mainContent.Render())
	res := m.base.Render()

	return m.zM.Scan(res) // call zone Manager.Scan() at root Model
}

func normalize(s *viewv1.ObjectSummary) *viewv1.ObjectSummary {
	if s == nil {
		return nil
	}
	if s.ChangeSummary == nil {
		s.ChangeSummary = &viewv1.ObjectChangeSummary{}
	}
	if s.ChangeSummary.Added == nil {
		s.ChangeSummary.Added = pointer.Int32(0)
	}
	if s.ChangeSummary.Updated == nil {
		s.ChangeSummary.Updated = pointer.Int32(0)
	}
	if s.ChangeSummary.Deleted == nil {
		s.ChangeSummary.Deleted = pointer.Int32(0)
	}
	return s
}

func buildDepthMap(objectTree *viewv1.ObjectTreeNode, depth float64, depthMap map[corev1.ObjectReference]float64) float64 {
	if objectTree == nil {
		return depth
	}
	objRef := objectTree.Primary
	objRef.UID = ""
	objRef.ResourceVersion = ""
	depthMap[objRef] = depth
	for _, secondary := range objectTree.Secondaries {
		depth = buildDepthMap(secondary, depth+1, depthMap)
	}
	return depth
}

var (
	enumeratorStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("63")).MarginRight(1)
	rootStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("35"))
	itemStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("212"))
)

func buildObjectTree(objectTree *viewv1.ObjectTreeNode) *tree.Tree {
	if objectTree == nil {
		return nil
	}
	formatNode := func(reference *corev1.ObjectReference) string {
		return fmt.Sprintf("%s/%s", reference.Kind, reference.Name)
	}
	tree := tree.New()
	tree.Root(formatNode(&objectTree.Primary)).EnumeratorStyle(enumeratorStyle).RootStyle(rootStyle).ItemStyle(itemStyle)
	for _, secondary := range objectTree.Secondaries {
		child := buildObjectTree(secondary)
		child.Root(formatNode(&secondary.Primary)).EnumeratorStyle(enumeratorStyle).RootStyle(rootStyle).ItemStyle(itemStyle)
		tree.Child(child)
	}
	return tree
}

func NewReconciliationViewChart(view *viewv1.ReconciliationView) *Model {

	// all barcharts contain the same values
	// different options are displayed on the screen and below
	// and first barchart has axis and label styles
	return &Model{
		view:        view,
		zM:          zone.New(),
		base:        flexbox.NewHorizontal(0, 0),
		statusBar:   flexbox.New(0, 0),
		mainContent: flexbox.New(0, 0),
	}
}
