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
	"github.com/NimbleMarkets/ntcharts/canvas"
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

var (
	defaultStyle = lipgloss.NewStyle().
			BorderStyle(lipgloss.NormalBorder()).
			BorderForeground(lipgloss.Color("63")) // purple

	axisStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("3")) // yellow

	labelStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("6")) // cyan

	totalBlockStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("6")) // cyan

	addedBlockStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("2")) // green

	updatedBlockStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("3")) // yellow

	deletedBlockStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("9")) // red

	statusBarColumnStyle = lipgloss.NewStyle().
				Border(lipgloss.NormalBorder(), true).
				BorderForeground(lipgloss.Color("#26de81"))

	mainContentColumnStyle = lipgloss.NewStyle().
				Border(lipgloss.NormalBorder(), true).
				BorderForeground(lipgloss.Color("#2bcbba"))

	latestChangeCellStyle = lipgloss.NewStyle().
				Align(lipgloss.Center, lipgloss.Center)

	changesLineStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("4")) // blue

	enumeratorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("63")).
			MarginRight(1)

	rootStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("35"))

	itemStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("212"))
)

type ViewUpdateMsg struct {
	View *viewv1.ReconciliationView
}

// Model defines the BubbleTea Model of the ReconciliationView.
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
	changes *timeserieslinechart.Model

	zoneManager *zone.Manager

	view           *viewv1.ReconciliationView
	selectedChange *viewv1.ObjectChange
}

func (m *Model) Init() tea.Cmd {
	m.zoneManager = zone.New()

	m.base = flexbox.NewHorizontal(0, 0)
	columns := []*flexbox.Column{
		m.base.NewColumn().AddCells(
			flexbox.NewCell(1, 1).SetStyle(statusBarColumnStyle),
			flexbox.NewCell(1, 2).SetStyle(mainContentColumnStyle),
		),
	}
	m.base.AddColumns(columns)

	m.statusBar = flexbox.New(0, 0)
	statusRow := m.statusBar.NewRow().AddCells(
		flexbox.NewCell(1, 1).SetContent("Summary"),
		flexbox.NewCell(1, 1).SetStyle(latestChangeCellStyle).SetContent("Latest Change"),
	)
	m.statusBar.AddRows([]*flexbox.Row{statusRow})

	m.mainContent = flexbox.New(0, 0)
	mainRow := m.mainContent.NewRow().AddCells(
		flexbox.NewCell(1, 1).SetContent("ObjectTree"),
		flexbox.NewCell(1, 1).SetContent("Changes"),
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
			if m.summary != nil {
				m.summary.Update(msg)
			}
			if m.changes != nil {
				if m.zoneManager.Get(m.changes.ZoneID()).InBounds(msg) {
					m.setSelectedChange(msg)
				}
			}
		}
	case tea.WindowSizeMsg:
		m.base.SetWidth(msg.Width)
		m.base.SetHeight(msg.Height)
		m.base.ForceRecalculate()
		m.statusBar.SetWidth(m.base.GetColumn(0).GetCell(0).GetWidth())
		m.statusBar.SetHeight(m.base.GetColumn(0).GetCell(0).GetHeight())
		m.statusBar.ForceRecalculate()
		m.mainContent.SetWidth(m.base.GetColumn(0).GetCell(1).GetWidth())
		m.mainContent.SetHeight(m.base.GetColumn(0).GetCell(1).GetHeight())
		m.mainContent.ForceRecalculate()
	case ViewUpdateMsg:
		m.view = msg.View
	}
	return m, nil
}

func (m *Model) View() string {
	m.updateSummaryView()
	m.updateLatestChangeView()
	m.updateObjectTreeView()
	m.updateChangesView()

	m.updateStatusBarView()
	m.updateMainContentView()

	res := m.base.Render()
	return m.zoneManager.Scan(res)
}

func (m *Model) updateSummaryView() {
	if m.view == nil {
		return
	}
	summaryCell := m.statusBar.GetRow(0).GetCell(0)
	if summaryCell.GetHeight() <= 0 {
		return
	}
	dataSet := buildSummaryDataSet(&m.view.Status.CurrentState.Summary)
	if len(dataSet) <= 0 {
		return
	}
	m.summary = summary.New(summaryCell.GetWidth(), summaryCell.GetHeight(),
		dataSet,
		barchart.WithZoneManager(m.zoneManager),
		barchart.WithStyles(axisStyle, labelStyle))
	summaryCell.SetContent(m.summary.View())
}

func (m *Model) updateLatestChangeView() {
	if m.view == nil {
		return
	}
	change := ""
	if l := len(m.view.Status.CurrentState.Changes); l > 0 {
		change = m.view.Status.CurrentState.Changes[l-1].Description
		if m.view.Status.CurrentState.Changes[l-1].LocalDescription != nil {
			change = *m.view.Status.CurrentState.Changes[l-1].LocalDescription
		}
	}
	if m.selectedChange != nil {
		change = m.selectedChange.Description
		if m.selectedChange.LocalDescription != nil {
			change = *m.selectedChange.LocalDescription
		}
	}
	change = defaultStyle.Render(change)
	latestChangeCell := m.statusBar.GetRow(0).GetCell(1)
	latestChangeCell.SetContent(change)
}

func (m *Model) updateObjectTreeView() {
	if m.view == nil {
		return
	}
	m.objectTree = buildObjectTree(m.view.Status.CurrentState.ObjectTree)
	if m.objectTree == nil {
		return
	}
	m.mainContent.GetRow(0).GetCell(0).SetContent(m.objectTree.String())
}

func (m *Model) updateChangesView() {
	if m.view == nil {
		return
	}
	if m.objectTree == nil {
		return
	}

	changesCell := m.mainContent.GetRow(0).GetCell(1)
	if changesCell.GetWidth() <= 0 {
		return
	}

	depthMap := make(map[corev1.ObjectReference]float64)
	depth := buildDepthMap(m.view.Status.CurrentState.ObjectTree, 0, depthMap)
	minYValue := 0.0
	maxYValue := float64(len(depthMap))
	objectTreeHeight := lipgloss.Height(m.objectTree.String())
	changesChart := timeserieslinechart.New(changesCell.GetWidth()-2, objectTreeHeight+2, timeserieslinechart.WithZoneManager(m.zoneManager))
	changesChart.AxisStyle = axisStyle
	changesChart.LabelStyle = labelStyle
	changesChart.XLabelFormatter = timeserieslinechart.HourTimeLabelFormatter()
	changesChart.YLabelFormatter = func(i int, f float64) string {
		return ""
	}
	changesChart.UpdateHandler = timeserieslinechart.SecondUpdateHandler(1)
	changesChart.SetYRange(minYValue, maxYValue)
	changesChart.SetViewYRange(minYValue, maxYValue)
	changesChart.SetStyle(changesLineStyle)
	changesChart.SetZoneManager(m.zoneManager)
	m.changes = &changesChart
	for _, change := range m.view.Status.CurrentState.Changes {
		objRef := normalizeObjectRef(&change.ObjectReference)
		m.changes.Push(timeserieslinechart.TimePoint{Time: change.Timestamp.Time, Value: depth - depthMap[*objRef] + 1})
	}
	m.changes.DrawBraille()
	m.mainContent.GetRow(0).GetCell(1).SetContent(m.changes.View())
}

func (m *Model) updateStatusBarView() {
	m.base.GetColumn(0).GetCell(0).SetContent(m.statusBar.Render())
}

func (m *Model) updateMainContentView() {
	m.base.GetColumn(0).GetCell(1).SetContent(m.mainContent.Render())
}

func (m *Model) setSelectedChange(msg tea.MouseMsg) {
	m.selectedChange = nil
	x, y := m.zoneManager.Get(m.changes.ZoneID()).Pos(msg)
	point := m.changes.TimePointFromPoint(canvas.Point{X: x, Y: y})
	if point == nil {
		return
	}
	depthMap := make(map[corev1.ObjectReference]float64)
	depth := buildDepthMap(m.view.Status.CurrentState.ObjectTree, 0, depthMap)
	for i := range m.view.Status.CurrentState.Changes {
		change := &m.view.Status.CurrentState.Changes[i]
		if change.Timestamp.Time.Unix() != point.Time.Unix() {
			continue
		}
		objRef := normalizeObjectRef(&change.ObjectReference)
		if depthMap[*objRef] == (depth + 1 - point.Value) {
			m.selectedChange = change
			break
		}
	}
}

func buildSummaryDataSet(summary *viewv1.ObjectTreeDiffSummary) []barchart.BarData {
	var dataSet []barchart.BarData
	for i := range summary.ObjectSummaries {
		n := normalizeObjectSummary(&summary.ObjectSummaries[i])
		d := barchart.BarData{
			Label: n.ObjectType.Kind,
			Values: []barchart.BarValue{
				{Name: "Total", Value: float64(n.Total), Style: totalBlockStyle},
				{Name: "Added", Value: float64(*n.ChangeSummary.Added), Style: addedBlockStyle},
				{Name: "Updated", Value: float64(*n.ChangeSummary.Updated), Style: updatedBlockStyle},
				{Name: "Deleted", Value: float64(*n.ChangeSummary.Deleted), Style: deletedBlockStyle},
			},
		}
		dataSet = append(dataSet, d)
	}
	return dataSet
}

func normalizeObjectRef(ref *corev1.ObjectReference) *corev1.ObjectReference {
	objRef := *ref
	objRef.UID = ""
	objRef.ResourceVersion = ""
	return &objRef
}

func normalizeObjectSummary(s *viewv1.ObjectSummary) *viewv1.ObjectSummary {
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
	objRef := normalizeObjectRef(&objectTree.Primary)
	depthMap[*objRef] = depth
	for _, secondary := range objectTree.Secondaries {
		depth = buildDepthMap(secondary, depth+1, depthMap)
	}
	return depth
}

func buildObjectTree(objectTree *viewv1.ObjectTreeNode) *tree.Tree {
	if objectTree == nil {
		return nil
	}
	formatNode := func(reference *corev1.ObjectReference) string {
		return fmt.Sprintf("%s/%s", reference.Kind, reference.Name)
	}
	treeNode := tree.New()
	treeNode.Root(formatNode(&objectTree.Primary)).EnumeratorStyle(enumeratorStyle).RootStyle(rootStyle).ItemStyle(itemStyle)
	for _, secondary := range objectTree.Secondaries {
		child := buildObjectTree(secondary)
		child.Root(formatNode(&secondary.Primary)).EnumeratorStyle(enumeratorStyle).RootStyle(rootStyle).ItemStyle(itemStyle)
		treeNode.Child(child)
	}
	return treeNode
}

func NewReconciliationViewChart() *Model {
	return &Model{}
}
