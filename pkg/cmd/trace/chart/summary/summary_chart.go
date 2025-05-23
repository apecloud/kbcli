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

package summary

import (
	"fmt"

	"github.com/NimbleMarkets/ntcharts/barchart"
	"github.com/NimbleMarkets/ntcharts/canvas"
	"github.com/NimbleMarkets/ntcharts/canvas/runes"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var borderStyle = lipgloss.NewStyle().
	BorderStyle(lipgloss.NormalBorder()).
	BorderForeground(lipgloss.Color("6")) // cyan

var selectedBarData barchart.BarData

type Model struct {
	summary barchart.Model
	data    []barchart.BarData
}

func (m *Model) Init() tea.Cmd {
	return nil
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if mm, ok := msg.(tea.MouseMsg); ok && mm.Action == tea.MouseActionPress {
		if m.summary.ZoneManager() == nil {
			return m, nil
		}
		if m.summary.ZoneManager().Get(m.summary.ZoneID()).InBounds(mm) {
			m.setBarData(&m.summary, mm)
		}
	}
	return m, nil
}

func (m *Model) View() string {
	m.summary.Draw()
	return lipgloss.JoinHorizontal(lipgloss.Top,
		m.summary.View(),
		borderStyle.Render(totals(m.data)),
		borderStyle.Render(legend(m.data[0])),
		borderStyle.Render(selectedData()),
	)
}

func (m *Model) setBarData(b *barchart.Model, msg tea.MouseMsg) {
	x, y := m.summary.ZoneManager().Get(b.ZoneID()).Pos(msg)
	selectedBarData = b.BarDataFromPoint(canvas.Point{X: x, Y: y})
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
		sum := bd.Values[0].Value
		r += "\n" + fmt.Sprintf("%s %d", bd.Label, int(sum))
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
		r += " " + bv.Style.Render(fmt.Sprintf("%d", int(bv.Value)))
	}
	return
}

func New(h int, dataSet []barchart.BarData, opts ...barchart.Option) *Model {
	gap := 1
	barWidth := 4
	opts = append(opts, barchart.WithDataSet(dataSet), barchart.WithBarWidth(barWidth), barchart.WithBarGap(gap))
	bar := barchart.New(len(dataSet)*(barWidth+gap), h-1, opts...)
	return &Model{
		summary: bar,
		data:    dataSet,
	}
}
