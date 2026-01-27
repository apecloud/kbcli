/*
Copyright (C) 2022-2026 ApeCloud Co., Ltd

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

package richviewport

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var (
	borderColor = lipgloss.Color("63") // purple

	borderStyle = lipgloss.NewStyle().
			Border(lipgloss.NormalBorder(), false, true).
			BorderForeground(borderColor)

	lineStyle = lipgloss.NewStyle().
			Foreground(borderColor)

	titleStyle = func() lipgloss.Style {
		b := lipgloss.NormalBorder()
		b.Right = "├"
		return lipgloss.NewStyle().BorderStyle(b).Padding(0, 1).
			BorderForeground(borderColor)
	}()

	infoStyle = func() lipgloss.Style {
		b := lipgloss.NormalBorder()
		b.Left = "┤"
		return titleStyle.BorderStyle(b).
			BorderForeground(borderColor)
	}()
)

type Model struct {
	header   string
	content  string
	width    int
	height   int
	viewport viewport.Model
}

func (m *Model) Init() tea.Cmd {
	return nil
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var (
		cmd  tea.Cmd
		cmds []tea.Cmd
	)
	m.viewport, cmd = m.viewport.Update(msg)
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

func (m *Model) View() string {
	return lipgloss.JoinVertical(lipgloss.Top,
		m.headerView(),
		borderStyle.Render(m.viewport.View()),
		m.footerView())
}

func (m *Model) headerView() string {
	title := titleStyle.Render(m.header)
	line := strings.Repeat("─", max(0, m.width-lipgloss.Width(title)-1))
	return lipgloss.JoinHorizontal(lipgloss.Center, title, lineStyle.Render(line), lineStyle.Render(" \n┐\n│"))
}

func (m *Model) footerView() string {
	info := infoStyle.Render(fmt.Sprintf("%3.f%%", m.viewport.ScrollPercent()*100))
	line := strings.Repeat("─", max(0, m.width-lipgloss.Width(info)-1))
	return lipgloss.JoinHorizontal(lipgloss.Center, lineStyle.Render("│\n└\n "), lineStyle.Render(line), info)
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func NewViewPort(w, h int, header, content string) *Model {
	m := &Model{
		header:  header,
		content: content,
		width:   w,
		height:  h,
	}
	headerHeight := lipgloss.Height(m.headerView())
	footerHeight := lipgloss.Height(m.footerView())
	verticalMarginHeight := headerHeight + footerHeight
	m.viewport = viewport.New(m.width-2, m.height-verticalMarginHeight)
	m.viewport.SetContent(m.content)
	m.viewport.YPosition = headerHeight + 1
	return m
}
