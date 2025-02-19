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

package objecttree

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/tree"
	zone "github.com/lrstanley/bubblezone"
	corev1 "k8s.io/api/core/v1"

	tracev1 "github.com/apecloud/kubeblocks/apis/trace/v1"
)

var (
	enumeratorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("63")). // purple
			MarginRight(1)

	workloadStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("35"))

	noneWorkloadStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("212"))

	selectedBackgroundColor = lipgloss.Color("4")
)

type Model struct {
	tree        *tree.Tree
	zoneID      string
	zoneManager *zone.Manager

	data *tracev1.ObjectTreeNode

	selected *int
}

func (m *Model) Init() tea.Cmd {
	return nil
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if mm, ok := msg.(tea.MouseMsg); ok && mm.Action == tea.MouseActionPress {
		if m.zoneManager == nil {
			return m, nil
		}
		if m.zoneManager.Get(m.zoneID).InBounds(mm) {
			m.setSelectedObject(mm)
		}
	}
	return m, nil
}

func (m *Model) View() string {
	m.tree = buildObjectTree(m.data)
	if m.tree == nil {
		return ""
	}

	depthObjectMap := make(map[int]string)
	buildDepth2ObjectMap(m.data, 0, depthObjectMap)

	setBackgroundColor := func(style lipgloss.Style, nodeValue string) lipgloss.Style {
		if m.selected != nil {
			if obj, ok := depthObjectMap[*m.selected]; ok && nodeValue == obj {
				return style.Background(selectedBackgroundColor)
			}
		}
		return style
	}

	rootStyle := setBackgroundColor(workloadStyle, m.tree.Value())
	m.tree.RootStyle(rootStyle)
	m.tree.EnumeratorStyle(enumeratorStyle)

	isWorkload := func(node string) bool {
		if strings.Contains(node, "Cluster") ||
			strings.Contains(node, "Component") ||
			strings.Contains(node, "InstanceSet") ||
			strings.Contains(node, "Pod") {
			return true
		}
		return false
	}

	m.tree.ItemStyleFunc(func(children tree.Children, i int) lipgloss.Style {
		style := noneWorkloadStyle
		if isWorkload(children.At(i).Value()) {
			style = workloadStyle
		}
		return setBackgroundColor(style, children.At(i).Value())
	})

	return m.zoneManager.Mark(m.zoneID, m.tree.String())
}

func (m *Model) SetData(data *tracev1.ObjectTreeNode) {
	m.data = data
}

func (m *Model) GetSelected() int {
	if m.selected == nil {
		return -1
	}
	return *m.selected
}

func (m *Model) setSelectedObject(mm tea.MouseMsg) {
	_, y := m.zoneManager.Get(m.zoneID).Pos(mm)
	if y >= 0 {
		m.selected = &y
	}
}

func buildDepth2ObjectMap(objectTree *tracev1.ObjectTreeNode, depth int, depthObjectMap map[int]string) int {
	if objectTree == nil {
		return depth
	}
	obj := formatNode(&objectTree.Primary)
	depthObjectMap[depth] = obj
	for _, secondary := range objectTree.Secondaries {
		depth = buildDepth2ObjectMap(secondary, depth+1, depthObjectMap)
	}
	return depth
}

func formatNode(reference *corev1.ObjectReference) string {
	return fmt.Sprintf("%s/%s", reference.Kind, reference.Name)
}

func buildObjectTree(objectTree *tracev1.ObjectTreeNode) *tree.Tree {
	if objectTree == nil {
		return nil
	}

	treeNode := tree.New()
	treeNode.Root(formatNode(&objectTree.Primary))
	for _, secondary := range objectTree.Secondaries {
		child := buildObjectTree(secondary)
		treeNode.Child(child)
	}
	return treeNode
}

func NewTree(data *tracev1.ObjectTreeNode, manager *zone.Manager) *Model {
	return &Model{
		data:        data,
		zoneManager: manager,
		zoneID:      manager.NewPrefix(),
	}
}
