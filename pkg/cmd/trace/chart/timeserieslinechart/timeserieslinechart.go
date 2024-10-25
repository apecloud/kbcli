// ntcharts - Copyright (c) 2024 Neomantra Corp.
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

// Package timeserieslinechart implements a linechart that draws lines
// for time series data points
package timeserieslinechart

// https://en.wikipedia.org/wiki/Moving_average

import (
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/NimbleMarkets/ntcharts/canvas"
	"github.com/NimbleMarkets/ntcharts/canvas/buffer"
	"github.com/NimbleMarkets/ntcharts/canvas/graph"
	"github.com/NimbleMarkets/ntcharts/canvas/runes"
	"github.com/NimbleMarkets/ntcharts/linechart"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const DefaultDataSetName = "default"

func DateTimeLabelFormatter() linechart.LabelFormatter {
	var yearLabel string
	return func(i int, v float64) string {
		if i == 0 { // reset year labeling if redisplaying values
			yearLabel = ""
		}
		t := time.Unix(int64(v), 0).UTC()
		monthDay := t.Format("01/02")
		year := t.Format("'06")
		if yearLabel != year { // apply year label if first time seeing year
			yearLabel = year
			return fmt.Sprintf("%s %s", yearLabel, monthDay)
		} else {
			return monthDay
		}
	}
}

func HourTimeLabelFormatter() linechart.LabelFormatter {
	return func(i int, v float64) string {
		t := time.Unix(int64(v), 0).UTC()
		return t.Format("15:04:05")
	}
}

type TimePoint struct {
	Time  time.Time
	Value float64
}

// cAverage tracks cumulative average
type cAverage struct {
	Avg   float64
	Count float64
}

// Add adds a float64 to current cumulative average
func (a *cAverage) Add(f float64) float64 {
	a.Count += 1
	a.Avg += (f - a.Avg) / a.Count
	return a.Avg
}

type dataSet struct {
	LineStyle runes.LineStyle // type of line runes to draw
	Style     lipgloss.Style

	lastTime time.Time // last seen time value

	// stores TimePoints as FloatPoint64{X:time.Time, Y: value}
	// time.Time will be converted to seconds since epoch.
	// both time and value will be scaled to fit the graphing area
	tBuf *buffer.Float64PointScaleBuffer
}

// Model contains state of a timeserieslinechart with an embedded linechart.Model
// The X axis contains time.Time values and the Y axis contains float64 values.
// A data set consists of a sequence TimePoints in chronological order.
// If multiple TimePoints map to the same column, then average value the time points
// will be used as the Y value of the column.
// The X axis contains a time range and the Y axis contains a numeric value range.
// Uses linechart Model UpdateHandler() for processing keyboard and mouse messages.
type Model struct {
	linechart.Model
	dLineStyle runes.LineStyle     // default data set LineStyletype
	dStyle     lipgloss.Style      // default data set Style
	dSets      map[string]*dataSet // maps names to data sets

	pointToDataMap map[canvas.Point]*TimePoint // canvas point to time point map
}

// New returns a timeserieslinechart Model initialized from
// width, height, Y value range and various options.
// By default, the chart will set time.Now() as the minimum time,
// enable auto set X and Y value ranges,
// and only allow moving viewport on X axis.
func New(w, h int, opts ...Option) Model {
	m := Model{
		Model: linechart.New(w, h, 0.0, float64(time.Now().Unix()), 0, 1,
			linechart.WithXYSteps(4, 2),
			linechart.WithXLabelFormatter(DateTimeLabelFormatter()),
			linechart.WithAutoXYRange(),                        // automatically adjust value ranges
			linechart.WithUpdateHandler(DateUpdateHandler(1))), // only scroll on X axis, increments by 1 day
		dLineStyle: runes.ArcLineStyle,
		dStyle:     lipgloss.NewStyle(),
		dSets:      make(map[string]*dataSet),
	}
	for _, opt := range opts {
		opt(&m)
	}
	m.UpdateGraphSizes()
	m.rescaleData()
	if _, ok := m.dSets[DefaultDataSetName]; !ok {
		m.dSets[DefaultDataSetName] = m.newDataSet()
	}
	return m
}

// newDataSet returns a new initialize *dataSet.
func (m *Model) newDataSet() *dataSet {
	xs := float64(m.GraphWidth()) / (m.ViewMaxX() - m.ViewMinX()) // x scale factor
	ys := float64(m.Origin().Y) / (m.ViewMaxY() - m.ViewMinY())   // y scale factor
	offset := canvas.Float64Point{X: m.ViewMinX(), Y: m.ViewMinY()}
	scale := canvas.Float64Point{X: xs, Y: ys}
	return &dataSet{
		LineStyle: m.dLineStyle,
		Style:     m.dStyle,
		tBuf:      buffer.NewFloat64PointScaleBuffer(offset, scale),
	}
}

// rescaleData will reinitialize time chunks and
// map time points into graph columns for display
func (m *Model) rescaleData() {
	// rescale time points buffer
	xs := float64(m.GraphWidth()) / (m.ViewMaxX() - m.ViewMinX()) // x scale factor
	ys := float64(m.Origin().Y) / (m.ViewMaxY() - m.ViewMinY())   // y scale factor
	offset := canvas.Float64Point{X: m.ViewMinX(), Y: m.ViewMinY()}
	scale := canvas.Float64Point{X: xs, Y: ys}
	for _, ds := range m.dSets {
		if ds.tBuf.Offset() != offset {
			ds.tBuf.SetOffset(offset)
		}
		if ds.tBuf.Scale() != scale {
			ds.tBuf.SetScale(scale)
		}
	}
}

// ClearAllData will reset stored data values in all data sets.
func (m *Model) ClearAllData() {
	for _, ds := range m.dSets {
		ds.tBuf.Clear()
	}
	m.dSets[DefaultDataSetName] = m.newDataSet()
}

// ClearDataSet will erase stored data set given by name string.
func (m *Model) ClearDataSet(n string) {
	if ds, ok := m.dSets[n]; ok {
		ds.tBuf.Clear()
	}
}

// SetTimeRange updates the minimum and maximum expected time values.
// Existing data will be rescaled.
func (m *Model) SetTimeRange(min, max time.Time) {
	m.Model.SetXRange(float64(min.Unix()), float64(max.Unix()))
	m.rescaleData()
}

// SetYRange updates the minimum and maximum expected Y values.
// Existing data will be rescaled.
func (m *Model) SetYRange(min, max float64) {
	m.Model.SetYRange(min, max)
	m.rescaleData()
}

// SetViewTimeRange updates the displayed minimum and maximum time values.
// Existing data will be rescaled.
func (m *Model) SetViewTimeRange(min, max time.Time) {
	m.Model.SetViewXRange(float64(min.Unix()), float64(max.Unix()))
	m.rescaleData()
}

// SetViewYRange updates the displayed minimum and maximum Y values.
// Existing data will be rescaled.
func (m *Model) SetViewYRange(min, max float64) {
	m.Model.SetViewYRange(min, max)
	m.rescaleData()
}

// SetViewTimeAndYRange updates the displayed minimum and maximum time and Y values.
// Existing data will be rescaled.
func (m *Model) SetViewTimeAndYRange(minX, maxX time.Time, minY, maxY float64) {
	m.Model.SetViewXRange(float64(minX.Unix()), float64(maxX.Unix()))
	m.Model.SetViewYRange(minY, maxY)
	m.rescaleData()
}

// Resize will change timeserieslinechart display width and height.
// Existing data will be rescaled.
func (m *Model) Resize(w, h int) {
	m.Model.Resize(w, h)
	m.rescaleData()
}

// SetLineStyle will set the default line styles of data sets.
func (m *Model) SetLineStyle(ls runes.LineStyle) {
	m.dLineStyle = ls
	m.SetDataSetLineStyle(DefaultDataSetName, ls)
}

// SetStyle will set the default lipgloss styles of data sets.
func (m *Model) SetStyle(s lipgloss.Style) {
	m.dStyle = s
	m.SetDataSetStyle(DefaultDataSetName, s)
}

// SetDataSetLineStyle will set the line style of the given data set by name string.
func (m *Model) SetDataSetLineStyle(n string, ls runes.LineStyle) {
	if _, ok := m.dSets[n]; !ok {
		m.dSets[n] = m.newDataSet()
	}
	ds := m.dSets[n]
	ds.LineStyle = ls
}

// SetDataSetStyle will set the lipgloss style of the given data set by name string.
func (m *Model) SetDataSetStyle(n string, s lipgloss.Style) {
	if _, ok := m.dSets[n]; !ok {
		m.dSets[n] = m.newDataSet()
	}
	ds := m.dSets[n]
	ds.Style = s
}

// Push will push a TimePoint data value to the default data set
// to be displayed with Draw.
func (m *Model) Push(t TimePoint) {
	m.PushDataSet(DefaultDataSetName, t)
}

// Push will push a TimePoint data value to a data set
// to be displayed with Draw. Using given data set by name string.
func (m *Model) PushDataSet(n string, t TimePoint) {
	f := canvas.Float64Point{X: float64(t.Time.Unix()), Y: t.Value}
	// auto adjust x and y ranges if enabled
	if m.AutoAdjustRange(f) {
		m.UpdateGraphSizes()
		m.rescaleData()
	}
	if _, ok := m.dSets[n]; !ok {
		m.dSets[n] = m.newDataSet()
	}
	ds := m.dSets[n]
	ds.tBuf.Push(f)
}

// Draw will draw lines runes displayed from right to left
// of the graphing area of the canvas. Uses default data set.
func (m *Model) Draw() {
	m.DrawDataSets([]string{DefaultDataSetName})
}

// DrawAll will draw lines runes for all data sets
// from right to left of the graphing area of the canvas.
func (m *Model) DrawAll() {
	names := make([]string, 0, len(m.dSets))
	for n, ds := range m.dSets {
		if ds.tBuf.Length() > 0 {
			names = append(names, n)
		}
	}
	sort.Strings(names)
	m.DrawDataSets(names)
}

// DrawDataSets will draw lines runes from right to left
// of the graphing area of the canvas for each data set given
// by name strings.
func (m *Model) DrawDataSets(names []string) {
	if len(names) == 0 {
		return
	}
	m.Clear()
	m.DrawXYAxisAndLabel()
	for _, n := range names {
		if ds, ok := m.dSets[n]; ok {
			dataPoints := ds.tBuf.ReadAll()
			dataLen := len(dataPoints)
			if dataLen == 0 {
				return
			}
			// get sequence of line values for graphing
			seqY := m.getLineSequence(dataPoints)
			// convert to canvas coordinates and avoid drawing below X axis
			yCoords := canvas.CanvasYCoordinates(m.Origin().Y, seqY)
			if m.XStep() > 0 {
				for i, v := range yCoords {
					if v > m.Origin().Y {
						yCoords[i] = m.Origin().Y
					}
				}
			}
			startX := m.Canvas.Width() - len(yCoords)
			graph.DrawLineSequence(&m.Canvas,
				(startX == m.Origin().X),
				startX,
				yCoords,
				ds.LineStyle,
				ds.Style)
		}
	}
}

// DrawRect will draw rect dots. Uses default data set.
func (m *Model) DrawRect() {
	m.DrawRectDataSets([]string{DefaultDataSetName})
}

// DrawRectAll will draw rect dots for all data sets.
func (m *Model) DrawRectAll() {
	names := make([]string, 0, len(m.dSets))
	for n, ds := range m.dSets {
		if ds.tBuf.Length() > 0 {
			names = append(names, n)
		}
	}
	sort.Strings(names)
	m.DrawRectDataSets(names)
}

// DrawRectDataSets will draw each point in data set as a rect.
func (m *Model) DrawRectDataSets(names []string) {
	if len(names) == 0 {
		return
	}
	m.Clear()
	m.DrawXYAxisAndLabel()
	m.pointToDataMap = make(map[canvas.Point]*TimePoint)
	for _, n := range names {
		if ds, ok := m.dSets[n]; ok {
			dataPoints := ds.tBuf.ReadAll()
			dataLen := len(dataPoints)
			if dataLen == 0 {
				return
			}
			for i := 0; i < dataLen; i++ {
				fp := dataPoints[i]
				p := canvas.CanvasPointFromFloat64Point(m.Origin(), fp)
				// get all rune patterns for braille grid
				// and draw them on to the canvas
				startX := 0
				if m.YStep() > 0 {
					startX = m.Origin().X + 1
				}
				p.X += startX
				m.Canvas.SetCell(p, canvas.NewCellWithStyle(runes.FullBlock, m.dStyle))

				raw := ds.tBuf.AtRaw(i)
				m.pointToDataMap[p] = &TimePoint{Time: time.Unix(int64(raw.X), 0), Value: raw.Y}
			}
		}
	}
}

// DrawBraille will draw braille runes displayed from right to left
// of the graphing area of the canvas. Uses default data set.
func (m *Model) DrawBraille() {
	m.DrawBrailleDataSets([]string{DefaultDataSetName})
}

// DrawBrailleAll will draw braille runes for all data sets
// from right to left of the graphing area of the canvas.
func (m *Model) DrawBrailleAll() {
	names := make([]string, 0, len(m.dSets))
	for n, ds := range m.dSets {
		if ds.tBuf.Length() > 0 {
			names = append(names, n)
		}
	}
	sort.Strings(names)
	m.DrawBrailleDataSets(names)
}

// DrawBrailleDataSets will draw braille runes from right to left
// of the graphing area of the canvas for each data set given
// by name strings.
func (m *Model) DrawBrailleDataSets(names []string) {
	if len(names) == 0 {
		return
	}
	m.Clear()
	m.DrawXYAxisAndLabel()
	m.pointToDataMap = make(map[canvas.Point]*TimePoint)
	for _, n := range names {
		if ds, ok := m.dSets[n]; ok {
			dataPoints := ds.tBuf.ReadAll()
			dataLen := len(dataPoints)
			if dataLen == 0 {
				return
			}
			// draw lines from each point to the next point
			bGrid := graph.NewBrailleGrid(m.GraphWidth(), m.GraphHeight(),
				0, float64(m.GraphWidth()), // X values already scaled to graph
				0, float64(m.GraphHeight())) // Y values already scaled to graph
			for i := 0; i < dataLen; i++ {
				p := dataPoints[i]
				// ignore points that will not be displayed
				bothBeforeMin := p.X < 0
				bothAfterMax := p.X > float64(m.GraphWidth())
				if bothBeforeMin || bothAfterMax {
					continue
				}
				// get braille grid points from two Float64Point data points
				gp := bGrid.GridPoint(p)
				bGrid.Set(gp)

				// cache the mapping relationship
				pos := canvas.Point{X: gp.X / m.XStep(), Y: gp.Y / m.YStep()}
				raw := ds.tBuf.AtRaw(i)
				m.pointToDataMap[pos] = &TimePoint{Time: time.Unix(int64(raw.X), 0), Value: raw.Y}
			}

			// get all rune patterns for braille grid
			// and draw them on to the canvas
			startX := 0
			if m.YStep() > 0 {
				startX = m.Origin().X + 1
			}
			patterns := bGrid.BraillePatterns()
			for i, xb := range patterns {
				for j, r := range xb {
					if r != runes.BrailleBlockOffset {
						patterns[i][j] = runes.FullBlock
					}
				}
			}
			DrawBraillePatterns(&m.Canvas,
				canvas.Point{X: startX, Y: 0}, patterns, ds.Style)
		}
	}
}

// DrawColumnRune draws a braille rune on to the canvas at given (X,Y) coordinates with given style.
// The function checks for existing braille runes already on the canvas and
// will draw a new braille pattern with the dot patterns of both the existing and given runes.
// Does nothing if given rune is Null or is not a braille rune.
func DrawBrailleRune(m *canvas.Model, p canvas.Point, r rune, s lipgloss.Style) {
	if r == runes.Null {
		return
	}
	cr := m.Cell(p).Rune
	if cr == 0 { // set rune if nothing exists on canvas
		m.SetCell(p, canvas.NewCellWithStyle(r, s))
		return
	}
	m.SetCell(p, canvas.NewCellWithStyle(r, s))
}

// DrawBraillePatterns draws braille runes from a [][]rune representing a 2D grid of
// Braille Pattern runes.  The runes will be drawn onto the canvas from starting from top
// left of the grid to the bottom right of the grid starting at the given canvas Point.
// Given style will be applied to all runes drawn.
// This function can be used with the output [][]rune from PatternDotsGrid.BraillePatterns().
func DrawBraillePatterns(m *canvas.Model, p canvas.Point, b [][]rune, s lipgloss.Style) {
	for y, row := range b {
		for x, r := range row {
			if r != runes.BrailleBlockOffset {
				DrawBrailleRune(m, p.Add(canvas.Point{X: x, Y: y}), r, s)
			}
		}
	}
}

// getLineSequence returns a sequence of Y values
// to draw line runes from a given set of scaled []FloatPoint64.
func (m *Model) getLineSequence(points []canvas.Float64Point) []int {
	width := m.Width() - m.Origin().X // line runes can draw on axes
	if width <= 0 {
		return []int{}
	}
	dataLen := len(points)
	// each index of the bucket corresponds to a graph column.
	// each index value is the average of data point values
	// that is mapped to that graph column.
	buckets := make([]cAverage, width, width)
	for i := 0; i < dataLen; i++ {
		j := i + 1
		if j >= dataLen {
			j = i
		}
		p1 := canvas.NewPointFromFloat64Point(points[i])
		p2 := canvas.NewPointFromFloat64Point(points[j])
		// ignore points that will not be displayed on the graph
		bothBeforeMin := (p1.X < 0 && p2.X < 0)
		bothAfterMax := (p1.X > m.GraphWidth() && p2.X > m.GraphWidth())
		if bothBeforeMin || bothAfterMax {
			continue
		}
		// place all points between two points
		// that approximates a line into buckets
		points := graph.GetLinePoints(p1, p2)
		for _, p := range points {
			if (p.X >= 0) && (p.X) < width {
				buckets[p.X].Add(float64(p.Y))
			}
		}
	}
	// populate sequence of Y values to for drawing
	r := make([]int, width, width)
	for i, v := range buckets {
		r[i] = int(math.Round(v.Avg))
	}
	return r
}

// Update processes bubbletea Msg to by invoking
// UpdateHandlerFunc callback if linechart is focused.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	if !m.Focused() {
		return m, nil
	}
	m.UpdateHandler(&m.Model, msg)
	m.rescaleData()
	return m, nil
}

func (m *Model) TimePointFromPoint(point canvas.Point) *TimePoint {
	if m.pointToDataMap == nil {
		return nil
	}
	return m.pointToDataMap[point]
}

func (m *Model) HighlightLine(y int, color lipgloss.Color) {
	if y < 0 || y >= m.Origin().Y {
		return
	}
	startX := 0
	if m.YStep() > 0 {
		startX = m.Origin().X + 1
	}
	for i := 0; i < m.GraphWidth(); i++ {
		p := canvas.Point{X: startX + i, Y: y}
		cell := m.Canvas.Cell(p)
		m.Canvas.SetCell(p, canvas.NewCellWithStyle(cell.Rune, cell.Style.Background(color)))
	}
}
