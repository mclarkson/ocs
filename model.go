package main

import (
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

var bgColor = tcell.NewRGBColor(25, 34, 48)

type model struct {
	table            *tview.Table
	app              *tview.Application
	sessions         Sessions
	filteredSessions Sessions
	selectedCommand  string
	selectedIndex    int
	shouldRefresh    bool
	showHelp         bool
	helpModal        *tview.Modal
	deleteModal      *tview.Modal
	header           *tview.TextView
	flex             *tview.Flex
	visible          [4]bool
	currentWidth     int
	dir              string
	dirOverridden    bool
	searchInput      *tview.InputField
	searchActive     bool
	searchQuery      string
	sessionToDelete  *Session
}

func populateTable(table *tview.Table, sessions Sessions, visible [4]bool, width int) {
	numVisible := 0
	for _, v := range visible {
		if v {
			numVisible++
		}
	}
	if numVisible == 0 {
		return
	}
	colWidths := [4]int{}
	baseWidth := width
	if visible[0] {
		colWidths[0] = 30 // fixed width for ID
		baseWidth -= colWidths[0]
	}
	if visible[3] {
		colWidths[3] = baseWidth / 5 // smaller ratio for Created
		baseWidth -= colWidths[3]
	}
	numOthers := 0
	for i := 1; i < 3; i++ {
		if visible[i] {
			numOthers++
		}
	}
	if numOthers > 0 {
		for i := 1; i < 3; i++ {
			if visible[i] {
				colWidths[i] = baseWidth / numOthers
			}
		}
	}
	// Adjust the last other column to fill baseWidth
	if numOthers > 0 {
		totalOthers := 0
		for i := 1; i < 3; i++ {
			if visible[i] {
				totalOthers += colWidths[i]
			}
		}
		lastOther := -1
		for i := 2; i >= 1; i-- {
			if visible[i] {
				lastOther = i
				break
			}
		}
		if lastOther != -1 {
			colWidths[lastOther] += baseWidth - totalOthers
		}
	}

	home, _ := os.UserHomeDir()
	colIndex := 0
	headers := []string{"ID", "Title", "Directory", "Created"}
	for i, vis := range visible {
		if vis {
			cell := tview.NewTableCell(headers[i]).SetTextColor(tcell.ColorYellow).SetSelectable(false)
			if i == 3 { // Created
				cell.SetAlign(tview.AlignRight)
			}
			table.SetCell(0, colIndex, cell)
			colIndex++
		}
	}
	// Add faint line under headers
	colIndex = 0
	for i, vis := range visible {
		if vis {
			line := strings.Repeat("─", colWidths[i])
			table.SetCell(1, colIndex, tview.NewTableCell(line).SetTextColor(tcell.ColorGray).SetSelectable(false))
			colIndex++
		}
	}
	for i, s := range sessions {
		createdTime := getCreatedTime(s.Time.Created)
		colIndex = 0
		if visible[0] {
			idText := truncateAtEnd(s.ID, colWidths[0])
			cell := tview.NewTableCell(idText).SetSelectable(true)
			if _, err := os.Stat(s.Directory); os.IsNotExist(err) {
				cell.SetTextColor(tcell.ColorRed)
			}
			table.SetCell(i+2, colIndex, cell)
			colIndex++
		}
		if visible[1] {
			titleText := truncateAtEnd(s.Title, colWidths[1])
			cell := tview.NewTableCell(titleText).SetSelectable(true)
			if _, err := os.Stat(s.Directory); os.IsNotExist(err) {
				cell.SetTextColor(tcell.ColorRed)
			}
			table.SetCell(i+2, colIndex, cell)
			colIndex++
		}
		if visible[2] {
			dir := s.Directory
			if strings.HasPrefix(dir, home) {
				dir = "~" + dir[len(home):]
			}
			dirText := truncateInMiddle(dir, colWidths[2])
			cell := tview.NewTableCell(dirText).SetSelectable(true)
			if _, err := os.Stat(s.Directory); os.IsNotExist(err) {
				cell.SetTextColor(tcell.ColorRed)
			}
			table.SetCell(i+2, colIndex, cell)
			colIndex++
		}
		if visible[3] {
			createdText := truncateAtEndCut(createdTime, colWidths[3])
			cell := tview.NewTableCell(createdText).SetSelectable(true).SetAlign(tview.AlignRight)
			if _, err := os.Stat(s.Directory); os.IsNotExist(err) {
				cell.SetTextColor(tcell.ColorRed)
			}
			table.SetCell(i+2, colIndex, cell)
		}
	}
}

func newModel(dir string, dirOverridden bool, sessions Sessions, cursor int, lastSearch string) model {
	width := 80 // default initial width

	header := tview.NewTextView().SetScrollable(false)
	header.SetBackgroundColor(bgColor)
	header.SetTextColor(tcell.ColorWhite)
	header.SetText("OpenCode Session browser                  Press ? for help")
	blank := tview.NewTextView()
	blank.SetBackgroundColor(bgColor)
	blank.SetText("")

	searchInput := tview.NewInputField().
		SetLabel("Search: ").
		SetFieldBackgroundColor(tcell.NewRGBColor(40, 50, 70)).
		SetLabelColor(tcell.ColorYellow)
	searchInput.SetBackgroundColor(bgColor)

	// Restore previous search query if any
	if lastSearch != "" {
		searchInput.SetText(lastSearch)
	}

	table := tview.NewTable().SetBorders(false).SetFixed(2, 0).SetSelectable(true, false)
	table.SetBackgroundColor(bgColor)
	app := tview.NewApplication()

	flex := tview.NewFlex().SetDirection(tview.FlexRow)
	flex.SetBackgroundColor(bgColor)
	flex.AddItem(header, 1, 0, false)
	flex.AddItem(blank, 1, 0, false)
	flex.AddItem(searchInput, 1, 0, false)
	flex.AddItem(table, 0, 1, true)

	helpModal := tview.NewModal().
		SetText("Help:\nEnter: Open selected session\nv: View selected session\nn: New opencode\nr: Refresh\n/: Fuzzy search (title & directory)\nDel/x: Delete selected session\nq: Quit\n?: Help\nEsc: Quit\nCtrl+D: Quit\ni: Toggle ID\nt: Toggle Title\nd: Toggle Directory\nc: Toggle Created").
		AddButtons([]string{"OK"}).
		SetDoneFunc(func(buttonIndex int, buttonLabel string) {
			app.SetRoot(flex, true).SetFocus(table)
		})

	deleteModal := tview.NewModal().
		SetText("Delete this session?").
		AddButtons([]string{"Delete", "Cancel"}).
		SetDoneFunc(func(buttonIndex int, buttonLabel string) {
			app.SetRoot(flex, true).SetFocus(table)
		})

	visible := [4]bool{false, true, true, true}

	// Apply search filter if there's a lastSearch
	filteredSessions := sessions
	if lastSearch != "" {
		filteredSessions = filterSessions(sessions, lastSearch)
	}

	populateTable(table, filteredSessions, visible, width)

	if cursor >= 0 && cursor < len(sessions) {
		table.Select(cursor+2, 0)
	}

	return model{
		table:            table,
		app:              app,
		sessions:         sessions,
		filteredSessions: filteredSessions,
		selectedIndex:    -1,
		shouldRefresh:    false,
		showHelp:         false,
		helpModal:        helpModal,
		deleteModal:      deleteModal,
		header:           header,
		flex:             flex,
		visible:          visible,
		currentWidth:     width,
		dir:              dir,
		dirOverridden:    dirOverridden,
		searchInput:      searchInput,
		searchActive:     false,
		searchQuery:      lastSearch,
		sessionToDelete:  nil,
	}
}

func truncateAtEnd(s string, maxLen int) string {
	if maxLen <= 0 {
		return ""
	}
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 1 {
		return "…"
	}
	return s[:maxLen-1] + "…"
}

func truncateAtEndCut(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}

func truncateInMiddle(s string, maxLen int) string {
	if maxLen <= 0 {
		return ""
	}
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 1 {
		return "…"
	}
	half := (maxLen - 1) / 2
	return s[:half] + "…" + s[len(s)-half:]
}

func getCreatedTime(created interface{}) string {
	if created == nil {
		return ""
	}
	switch v := created.(type) {
	case float64:
		return time.UnixMilli(int64(v)).Format("2006-01-02 15:04")
	case string:
		if i, err := strconv.ParseInt(v, 10, 64); err == nil {
			return time.UnixMilli(i).Format("2006-01-02 15:04")
		}
	}
	return ""
}

// fuzzyMatch checks if query matches text using fuzzy matching
// Returns true if all characters in query appear in text in order (case-insensitive)
func fuzzyMatch(text, query string) bool {
	if query == "" {
		return true
	}
	textLower := strings.ToLower(text)
	queryLower := strings.ToLower(query)

	textPos := 0
	queryPos := 0

	for queryPos < len(queryLower) && textPos < len(textLower) {
		if textLower[textPos] == queryLower[queryPos] {
			queryPos++
		}
		textPos++
	}

	return queryPos == len(queryLower)
}

func filterSessions(sessions Sessions, query string) Sessions {
	if query == "" {
		return sessions
	}
	var filtered Sessions
	for _, s := range sessions {
		// Search in both title and directory
		if fuzzyMatch(s.Title, query) || fuzzyMatch(s.Directory, query) {
			filtered = append(filtered, s)
		}
	}
	return filtered
}
