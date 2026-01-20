package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"

	"github.com/gdamore/tcell/v2"
)

func parseFlags() (string, bool, bool) {
	dir := flag.String("dir", os.Getenv("HOME")+"/.local/share/opencode/storage/session", "directory to scan for session JSON files")
	var debug bool
	flag.BoolVar(&debug, "debug", false, "enable debug output")
	flag.Parse()
	var dirOverridden bool
	flag.Visit(func(f *flag.Flag) {
		if f.Name == "dir" {
			dirOverridden = true
		}
	})
	return *dir, debug, dirOverridden
}

func checkDirectory(dir string) {
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		log.Fatalf("Directory does not exist: %s", dir)
	}
}

func loadSessions(dir string) (Sessions, error) {
	var sessions Sessions
	err := sessions.Scan(dir)
	if err != nil {
		return nil, err
	}
	sessions.Sort()
	return sessions, nil
}

func logSessions(sessions Sessions, debug bool) {
	if debug {
		log.Printf("Found %d sessions", len(sessions))
		if len(sessions) > 0 {
			log.Printf("First session: ID=%s Title=%s Dir=%s Created=%v", sessions[0].ID, sessions[0].Title, sessions[0].Directory, sessions[0].Time.Created)
		}
	}
}

func runProgram(dir string, dirOverridden bool, sessions Sessions, lastCursor int, lastSearch string) model {
	m := newModel(dir, dirOverridden, sessions, lastCursor, lastSearch)

	// Set up search input change handler
	m.searchInput.SetChangedFunc(func(text string) {
		m.searchQuery = text
		m.filteredSessions = filterSessions(m.sessions, text)
		m.table.Clear()
		populateTable(m.table, m.filteredSessions, m.visible, m.currentWidth)
		if len(m.filteredSessions) > 0 {
			m.table.Select(2, 0) // Select first result
		}
	})

	// Set up search input done handler (Enter or Esc)
	m.searchInput.SetDoneFunc(func(key tcell.Key) {
		if key == tcell.KeyEnter || key == tcell.KeyEscape {
			m.searchActive = false
			m.app.SetFocus(m.table)
		}
	})

	// Set up delete modal handler
	m.deleteModal.SetDoneFunc(func(buttonIndex int, buttonLabel string) {
		if buttonIndex == 0 && m.sessionToDelete != nil { // "Delete" button
			if err := DeleteSession(m.dir, m.sessionToDelete.ID); err != nil {
				log.Printf("Error deleting session: %v", err)
			}
			m.shouldRefresh = true
			m.app.Stop()
		} else {
			// "Cancel" button or closed
			m.sessionToDelete = nil
			m.app.SetRoot(m.flex, true).SetFocus(m.table)
		}
	})

	m.app.SetBeforeDrawFunc(func(screen tcell.Screen) bool {
		width, _ := screen.Size()
		if width == 0 {
			return false
		}
		m.currentWidth = width
		m.table.Clear()
		populateTable(m.table, m.filteredSessions, m.visible, width)
		// Update header
		left := "OpenCode Session browser"
		right := "Press ? for help"
		padLen := width - len(left) - len(right)
		if padLen > 0 {
			m.header.SetText(left + strings.Repeat(" ", padLen) + right)
		} else {
			m.header.SetText(left + " " + right)
		}
		return false
	})

	// Set up input capture
	m.table.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyEnter:
			row, _ := m.table.GetSelection()
			if row > 1 && row-2 < len(m.filteredSessions) {
				selectedSession := m.filteredSessions[row-2]
				m.selectedCommand = fmt.Sprintf("cd '%s' ; opencode -s %s", selectedSession.Directory, selectedSession.ID)
				// Find original index in m.sessions
				for i, s := range m.sessions {
					if s.ID == selectedSession.ID {
						m.selectedIndex = i
						break
					}
				}
				m.app.Stop()
			}
		case tcell.KeyEscape:
			m.app.Stop()
		case tcell.KeyCtrlD:
			m.app.Stop()
		case tcell.KeyDelete:
			row, _ := m.table.GetSelection()
			if row > 1 && row-2 < len(m.filteredSessions) {
				selectedSession := m.filteredSessions[row-2]
				m.sessionToDelete = &selectedSession
				m.deleteModal.SetText(fmt.Sprintf("Delete session?\n\nTitle: %s\nID: %s\n\nThis will permanently delete the session file and data.", selectedSession.Title, selectedSession.ID))
				m.app.SetRoot(m.deleteModal, false).SetFocus(m.deleteModal)
			}
		}
		switch event.Rune() {
		case 'q', 'Q':
			m.app.Stop()
		case 'r', 'R':
			m.shouldRefresh = true
			m.app.Stop()
		case 'n', 'N':
			m.selectedCommand = "opencode"
			m.app.Stop()
		case '/':
			m.searchActive = true
			m.app.SetFocus(m.searchInput)
		case 'i', 'I':
			m.visible[0] = !m.visible[0]
			m.table.Clear()
			populateTable(m.table, m.filteredSessions, m.visible, m.currentWidth)
		case 't', 'T':
			m.visible[1] = !m.visible[1]
			m.table.Clear()
			populateTable(m.table, m.filteredSessions, m.visible, m.currentWidth)
		case 'd', 'D':
			m.visible[2] = !m.visible[2]
			m.table.Clear()
			populateTable(m.table, m.filteredSessions, m.visible, m.currentWidth)
		case 'c', 'C':
			m.visible[3] = !m.visible[3]
			m.table.Clear()
			populateTable(m.table, m.filteredSessions, m.visible, m.currentWidth)
		case '?':
			m.app.SetRoot(m.helpModal, false).SetFocus(m.helpModal)
		case 'x', 'X':
			row, _ := m.table.GetSelection()
			if row > 1 && row-2 < len(m.filteredSessions) {
				selectedSession := m.filteredSessions[row-2]
				m.sessionToDelete = &selectedSession
				m.deleteModal.SetText(fmt.Sprintf("Delete session?\n\nTitle: %s\nID: %s\n\nThis will permanently delete the session file and data.", selectedSession.Title, selectedSession.ID))
				m.app.SetRoot(m.deleteModal, false).SetFocus(m.deleteModal)
			}
		case 'v', 'V':
			row, _ := m.table.GetSelection()
			if row > 1 && row-2 < len(m.filteredSessions) {
				selectedSession := m.filteredSessions[row-2]
				if _, err := exec.LookPath("python3"); err == nil {
					if _, err := exec.LookPath("ocs_messages.py"); err == nil {
						pager := "more"
						if _, err := exec.LookPath("glow"); err == nil {
							pager = "glow -p"
						} else if _, err := exec.LookPath("less"); err == nil {
							pager = "less"
						}
						if m.dirOverridden {
							dirForD := strings.TrimSuffix(m.dir, "/session")
							m.selectedCommand = fmt.Sprintf("ocs_messages.py -d '%s' %s | %s", dirForD, selectedSession.ID, pager)
						} else {
							m.selectedCommand = fmt.Sprintf("ocs_messages.py %s | %s", selectedSession.ID, pager)
						}
						// Find original index in m.sessions
						for i, s := range m.sessions {
							if s.ID == selectedSession.ID {
								m.selectedIndex = i
								break
							}
						}
						m.app.Stop()
					}
				}
			}
		}
		return event
	})

	if err := m.app.SetRoot(m.flex, true).Run(); err != nil {
		log.Fatalf("Error running app: %v", err)
	}
	return m
}

func handleCommand(selectedCommand string, selectedIndex int) int {
	fmt.Println(selectedCommand)
	cmd := exec.Command("/bin/bash", "-c", selectedCommand)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	if err := cmd.Run(); err != nil {
		log.Printf("Command failed: %v", err)
	}
	return selectedIndex
}

func main() {
	dir, debug, dirOverridden := parseFlags()
	if debug {
		log.Printf("Scanning dir: %s", dir)
	}
	checkDirectory(dir)
	sessions, err := loadSessions(dir)
	if err != nil {
		log.Fatalf("Error loading sessions: %v", err)
	}
	logSessions(sessions, debug)

	lastCursor := -1
	lastSearch := ""
	for {
		if debug {
			log.Printf("Setting cursor to %d", lastCursor)
		}
		finalModel := runProgram(dir, dirOverridden, sessions, lastCursor, lastSearch)
		if finalModel.shouldRefresh {
			sessions, err = loadSessions(dir)
			if err != nil {
				log.Fatalf("Error reloading sessions: %v", err)
			}
			lastCursor = -1
			// Preserve search query when refreshing
			lastSearch = finalModel.searchQuery
		} else if finalModel.selectedCommand != "" {
			lastCursor = handleCommand(finalModel.selectedCommand, finalModel.selectedIndex)
			lastSearch = ""
		} else {
			break
		}
	}
}
