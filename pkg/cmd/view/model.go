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
	"fmt"
	"github.com/76creates/stickers/flexbox"
	"github.com/NimbleMarkets/ntcharts/canvas"
	"github.com/NimbleMarkets/ntcharts/canvas/runes"
	zone "github.com/lrstanley/bubblezone"
	corev1 "k8s.io/api/core/v1"

	"github.com/NimbleMarkets/ntcharts/barchart"
	"github.com/NimbleMarkets/ntcharts/sparkline"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/tree"

	viewv1 "github.com/apecloud/kbcli/apis/view/v1"
)

// teaModel defines the BubbleTea teaModel of the ReconciliationView.

var selectedBarData barchart.BarData

var defaultStyle = lipgloss.NewStyle().
	BorderStyle(lipgloss.NormalBorder()).
	BorderForeground(lipgloss.Color("63")) // purple

var axisStyle = lipgloss.NewStyle().
	Foreground(lipgloss.Color("3")) // yellow

var labelStyle = lipgloss.NewStyle().
	Foreground(lipgloss.Color("63")) // purple

var blockStyle = lipgloss.NewStyle().
	Foreground(lipgloss.Color("9")) // red

var blockStyle2 = lipgloss.NewStyle().
	Foreground(lipgloss.Color("2")) // green

var blockStyle3 = lipgloss.NewStyle().
	Foreground(lipgloss.Color("6")) // cyan

var blockStyle4 = lipgloss.NewStyle().
	Foreground(lipgloss.Color("3")) // yellow

var (
	style1  = lipgloss.NewStyle().Background(lipgloss.Color("#fc5c65"))
	style2  = lipgloss.NewStyle().Background(lipgloss.Color("#fd9644"))
	style3  = lipgloss.NewStyle().Background(lipgloss.Color("#fed330"))
	style4  = lipgloss.NewStyle().Background(lipgloss.Color("#26de81"))
	style5  = lipgloss.NewStyle().Background(lipgloss.Color("#2bcbba"))
	style6  = lipgloss.NewStyle().Background(lipgloss.Color("#eb3b5a"))
	style7  = lipgloss.NewStyle().Background(lipgloss.Color("#fa8231"))
	style8  = lipgloss.NewStyle().Align(lipgloss.Left, lipgloss.Top).Border(lipgloss.NormalBorder(), true).BorderBackground(lipgloss.Color("#FFFFFF"))
	style9  = lipgloss.NewStyle().Background(lipgloss.Color("#20bf6b"))
	style10 = lipgloss.NewStyle().Background(lipgloss.Color("#0fb9b1"))
	style11 = lipgloss.NewStyle().Background(lipgloss.Color("#45aaf2"))
	style12 = lipgloss.NewStyle().Background(lipgloss.Color("#4b7bec"))
	style13 = lipgloss.NewStyle().Background(lipgloss.Color("#a55eea"))
	style14 = lipgloss.NewStyle().Background(lipgloss.Color("#d1d8e0"))
	style15 = lipgloss.NewStyle().Background(lipgloss.Color("#778ca3"))
	style16 = lipgloss.NewStyle().Background(lipgloss.Color("#2d98da"))
	style17 = lipgloss.NewStyle().Background(lipgloss.Color("#3867d6"))
	style18 = lipgloss.NewStyle().Background(lipgloss.Color("#8854d0"))
	style19 = lipgloss.NewStyle().Background(lipgloss.Color("#a5b1c2"))
	style20 = lipgloss.NewStyle().Background(lipgloss.Color("#4b6584"))
)

type teaModel struct {
	view *viewv1.ReconciliationView

	base        *flexbox.HorizontalFlexBox
	statusBar   *flexbox.FlexBox
	mainContent *flexbox.FlexBox

	// summary
	summary barchart.Model
	lv      []barchart.BarData
	zM      *zone.Manager

	// latest change
	latestChange viewport.Model

	// current object tree
	objectTree *tree.Tree

	// changes
	changes sparkline.Model
}

func legend(bd barchart.BarData) (r string) {
	r = "Legend\n"
	for _, bv := range bd.Values {
		r += "\n" + bv.Style.Render(fmt.Sprintf("%c %s", runes.FullBlock, bv.Name))
	}
	return
}

func totals(lv []barchart.BarData) (r string) {
	r = "Totals\n"
	for _, bd := range lv {
		var sum float64
		for _, bv := range bd.Values {
			sum += bv.Value
		}
		r += "\n" + fmt.Sprintf("%s %.01f", bd.Label, sum)
	}
	return
}
func selectedData() (r string) {
	r = "Selected\n"
	if len(selectedBarData.Values) == 0 {
		return
	}
	r += selectedBarData.Label
	for _, bv := range selectedBarData.Values {
		r += " " + bv.Style.Render(fmt.Sprintf("%.01f", bv.Value))
	}
	return
}

func (m *teaModel) setBarData(b *barchart.Model, msg tea.MouseMsg) {
	x, y := m.zM.Get(b.ZoneID()).Pos(msg)
	selectedBarData = b.BarDataFromPoint(canvas.Point{x, y})
}

func (m *teaModel) Init() tea.Cmd {
	columns := []*flexbox.Column{
		m.base.NewColumn().AddCells(
			flexbox.NewCell(1, 1).SetStyle(style4),
			flexbox.NewCell(1, 2).SetStyle(style5),
		),
	}
	m.base.AddColumns(columns)

	statusRow := m.statusBar.NewRow().AddCells(
		flexbox.NewCell(1, 1).SetStyle(style6).SetContent("Summary"),
		flexbox.NewCell(3, 1).SetStyle(style7).SetContent("Latest Change"),
	)
	m.statusBar.AddRows([]*flexbox.Row{statusRow})

	mainRow := m.mainContent.NewRow().AddCells(
		flexbox.NewCell(1, 1).SetStyle(style8).SetContent("ObjectTree"),
		flexbox.NewCell(3, 1).SetStyle(style9).SetContent("Changes"),
	)
	m.mainContent.AddRows([]*flexbox.Row{mainRow})

	return nil
}

func (m *teaModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		}
	case tea.MouseMsg:
		if msg.Action == tea.MouseActionPress {
			switch {
			case m.zM.Get(m.summary.ZoneID()).InBounds(msg):
				m.setBarData(&m.summary, msg)
			}
		}
	case tea.WindowSizeMsg:
		m.base.SetWidth(msg.Width)
		m.base.SetHeight(msg.Height)
	}
	return m, nil
}

func (m *teaModel) View() string {
	tree.Root("Cluster").Child()
	m.base.ForceRecalculate()
	m.statusBar.SetHeight(m.base.GetColumn(0).GetCell(0).GetHeight())
	m.statusBar.SetWidth(m.base.GetColumn(0).GetCell(0).GetWidth())
	summaryCell := m.statusBar.GetRow(0).GetCell(0)
	summaryCell.SetMinWidth(50)
	m.statusBar.ForceRecalculate()

	//m.summary.Resize(summaryCell.GetWidth(), summaryCell.GetHeight())
	m.summary = barchart.New(summaryCell.GetWidth()-30, summaryCell.GetHeight(),
		barchart.WithZoneManager(m.zM),
		barchart.WithDataSet(m.lv),
		barchart.WithStyles(axisStyle, labelStyle))
	m.summary.Draw()
	summary := lipgloss.JoinHorizontal(lipgloss.Top,
		defaultStyle.Render(m.summary.View()),
		lipgloss.JoinVertical(lipgloss.Left,
			lipgloss.JoinHorizontal(lipgloss.Top,
				defaultStyle.Render(totals(m.lv)),
				defaultStyle.Render(legend(m.lv[0])),
			),
			defaultStyle.Render(selectedData()),
		),
	)
	summaryCell.SetContent(summary)

	change := ""
	if l := len(m.view.Status.CurrentState.Changes); l > 0 {
		change = m.view.Status.CurrentState.Changes[l-1].Description
	}
	change = defaultStyle.Render(change)
	m.statusBar.GetRow(0).GetCell(1).SetContent(change)

	m.objectTree = buildObjectTree(m.view.Status.CurrentState.ObjectTree)

	m.mainContent.SetHeight(m.base.GetColumn(0).GetCell(1).GetHeight())
	m.mainContent.SetWidth(m.base.GetColumn(0).GetCell(1).GetWidth())
	m.mainContent.GetRow(0).GetCell(0).SetContent(m.objectTree.String())

	m.base.GetColumn(0).GetCell(0).SetContent(m.statusBar.Render())
	m.base.GetColumn(0).GetCell(1).SetContent(m.mainContent.Render())
	res := m.base.Render()

	return m.zM.Scan(res) // call zone Manager.Scan() at root teaModel
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
