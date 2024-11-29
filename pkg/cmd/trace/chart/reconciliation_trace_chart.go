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
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	zone "github.com/lrstanley/bubblezone"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/pointer"

	"github.com/apecloud/kbcli/pkg/cmd/trace/chart/objecttree"
	"github.com/apecloud/kbcli/pkg/cmd/trace/chart/richviewport"
	"github.com/apecloud/kbcli/pkg/cmd/trace/chart/summary"
	"github.com/apecloud/kbcli/pkg/cmd/trace/chart/timeserieslinechart"
	tracev1 "github.com/apecloud/kubeblocks/apis/trace/v1"
)

var (
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

	eventBlockStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("6")) // cyan

	columnStyle = lipgloss.NewStyle().
			Border(lipgloss.NormalBorder(), true, true, false, true).
			BorderForeground(lipgloss.Color("#2bcbba"))

	changesLineStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("4")) // blue

	summaryNLatestChangeSeparator = "    "
)

type TraceUpdateMsg struct {
	Trace *tracev1.ReconciliationTrace
}

// Model defines the BubbleTea Model of the ReconciliationTrace.
type Model struct {
	// base framework
	base *flexbox.HorizontalFlexBox

	// summary
	summary *summary.Model

	// latest change
	latestChange *richviewport.Model

	// current object tree
	objectTree *objecttree.Model

	// changes
	changes *timeserieslinechart.Model

	zoneManager *zone.Manager

	trace          *tracev1.ReconciliationTrace
	selectedChange *tracev1.ObjectChange
}

func (m *Model) Init() tea.Cmd {
	m.zoneManager = zone.New()

	m.base = flexbox.NewHorizontal(0, 0)
	columns := []*flexbox.Column{
		m.base.NewColumn().AddCells(
			flexbox.NewCell(1, 1).SetStyle(columnStyle),
			flexbox.NewCell(1, 2).SetStyle(columnStyle),
		),
	}
	m.base.AddColumns(columns)

	m.objectTree = objecttree.NewTree(nil, m.zoneManager)

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
			if m.objectTree != nil {
				m.objectTree.Update(msg)
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
	case TraceUpdateMsg:
		m.trace = msg.Trace
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

	return m.zoneManager.Scan(m.base.Render())
}

func (m *Model) updateSummaryView() {
	if m.trace == nil {
		return
	}
	dataSet := buildSummaryDataSet(&m.trace.Status.CurrentState.Summary)
	if len(dataSet) == 0 {
		return
	}
	m.summary = summary.New(m.base.GetColumn(0).GetCell(0).GetHeight(),
		dataSet,
		barchart.WithZoneManager(m.zoneManager),
		barchart.WithStyles(axisStyle, labelStyle))
}

func (m *Model) updateLatestChangeView() {
	if m.trace == nil {
		return
	}
	if m.summary == nil {
		return
	}
	formatChange := func(change *tracev1.ObjectChange) string {
		desc := change.Description
		if change.LocalDescription != nil {
			desc = *change.LocalDescription
		}
		name := types.NamespacedName{Namespace: change.ObjectReference.Namespace, Name: change.ObjectReference.Name}.String()
		return fmt.Sprintf("GVK: %s/%s\nObject: %s\nDescription: %s", change.ObjectReference.GroupVersionKind().GroupVersion(), change.ObjectReference.Kind, name, desc)
	}
	changeText := ""
	if l := len(m.trace.Status.CurrentState.Changes); l > 0 {
		changeText = formatChange(&m.trace.Status.CurrentState.Changes[l-1])
	}
	if m.selectedChange != nil {
		changeText = formatChange(m.selectedChange)
	}
	w := lipgloss.Width(m.summary.View())
	baseBorder := 1
	m.latestChange = richviewport.NewViewPort(
		m.base.GetWidth()-2*baseBorder-w-len(summaryNLatestChangeSeparator),
		m.base.GetColumn(0).GetCell(0).GetHeight()-baseBorder,
		"Latest Change",
		changeText)
}

func (m *Model) updateObjectTreeView() {
	if m.trace == nil {
		return
	}
	m.objectTree.SetData(m.trace.Status.CurrentState.ObjectTree)
}

func (m *Model) updateChangesView() {
	if m.trace == nil {
		return
	}
	if m.objectTree == nil {
		return
	}

	depthMap := make(map[corev1.ObjectReference]float64)
	depth := buildDepthMap(m.trace.Status.CurrentState.ObjectTree, 0, depthMap)
	minYValue := 0.0
	maxYValue := float64(len(depthMap))
	w, h := lipgloss.Size(m.objectTree.View())
	changesChart := timeserieslinechart.New(m.base.GetWidth()-2-w, h+2, timeserieslinechart.WithZoneManager(m.zoneManager))
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
	m.changes = &changesChart
	if len(m.trace.Status.CurrentState.Changes) > 0 {
		change := m.trace.Status.CurrentState.Changes[0]
		minX := change.Timestamp.Time.Unix()
		maxX := minX + 1
		m.changes.SetViewXRange(float64(minX), float64(maxX))
		m.changes.SetXRange(float64(minX), float64(maxX))
	}
	for _, change := range m.trace.Status.CurrentState.Changes {
		objRef := normalizeObjectRef(&change.ObjectReference)
		m.changes.Push(timeserieslinechart.TimePoint{Time: change.Timestamp.Time, Value: depth - depthMap[*objRef] + 1})
	}
	m.changes.SetDataSetStyleFunc(func(tp *timeserieslinechart.TimePoint) lipgloss.Style {
		change := m.findSelectedChange(tp)
		if change == nil {
			return lipgloss.NewStyle()
		}
		switch change.ChangeType {
		case tracev1.ObjectCreationType:
			return addedBlockStyle
		case tracev1.ObjectUpdateType:
			return updatedBlockStyle
		case tracev1.ObjectDeletionType:
			return deletedBlockStyle
		case tracev1.EventType:
			return eventBlockStyle
		}
		return lipgloss.NewStyle()
	})
	m.changes.DrawRect()
	m.changes.HighlightLine(m.objectTree.GetSelected(), lipgloss.Color("4"))
}

func (m *Model) updateStatusBarView() {
	if m.trace == nil {
		return
	}
	if m.summary == nil {
		return
	}
	if m.latestChange == nil {
		return
	}
	summary := m.summary.View()
	latestChange := m.latestChange.View()
	status := lipgloss.JoinHorizontal(lipgloss.Left, summary, summaryNLatestChangeSeparator, latestChange)
	m.base.GetColumn(0).GetCell(0).SetContent(status)
}

func (m *Model) updateMainContentView() {
	if m.trace == nil {
		return
	}
	if m.objectTree == nil {
		return
	}
	if m.changes == nil {
		return
	}
	objectTree := m.objectTree.View()
	changes := m.changes.View()
	mainContent := lipgloss.JoinHorizontal(lipgloss.Left, objectTree, changes)
	m.base.GetColumn(0).GetCell(1).SetContent(mainContent)
}

func (m *Model) setSelectedChange(msg tea.MouseMsg) {
	m.selectedChange = nil
	x, y := m.zoneManager.Get(m.changes.ZoneID()).Pos(msg)
	point := m.changes.TimePointFromPoint(canvas.Point{X: x, Y: y})
	if point == nil {
		return
	}
	change := m.findSelectedChange(point)
	if change != nil {
		m.selectedChange = change
	}
}

func (m *Model) findSelectedChange(point *timeserieslinechart.TimePoint) *tracev1.ObjectChange {
	depthMap := make(map[corev1.ObjectReference]float64)
	depth := buildDepthMap(m.trace.Status.CurrentState.ObjectTree, 0, depthMap)
	for i := range m.trace.Status.CurrentState.Changes {
		change := &m.trace.Status.CurrentState.Changes[i]
		if change.Timestamp.Time.Unix() != point.Time.Unix() {
			continue
		}
		objRef := normalizeObjectRef(&change.ObjectReference)
		if depthMap[*objRef] == (depth + 1 - point.Value) {
			return change
		}
	}
	return nil
}

func buildSummaryDataSet(summary *tracev1.ObjectTreeDiffSummary) []barchart.BarData {
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

func normalizeObjectSummary(s *tracev1.ObjectSummary) *tracev1.ObjectSummary {
	if s == nil {
		return nil
	}
	if s.ChangeSummary == nil {
		s.ChangeSummary = &tracev1.ObjectChangeSummary{}
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

func buildDepthMap(objectTree *tracev1.ObjectTreeNode, depth float64, depthMap map[corev1.ObjectReference]float64) float64 {
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

func NewReconciliationTraceChart() *Model {
	return &Model{}
}
