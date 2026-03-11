package main

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type statusEntry struct {
	Code rune
	Path string
	Raw  string
}

type statusMsg struct {
	entries []statusEntry
	raw     string
	err     error
}

type previewMsg struct {
	title string
	body  string
}

type actionMsg struct {
	title   string
	body    string
	refresh bool
}

type model struct {
	entries []statusEntry
	cursor  int
	marked  map[string]struct{}

	previewTitle string
	previewBody  string
	statusLine   string

	width  int
	height int

	commitPrompt bool
	commitInput  textinput.Model
	commitPaths  []string

	revertConfirm bool
	revertPaths   []string
}

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "-h", "--help", "help":
			fmt.Println(cliHelp())
			return
		case "update":
			if len(os.Args) > 3 {
				fmt.Fprintf(os.Stderr, "too many arguments for update\n\n%s\n", cliHelp())
				os.Exit(2)
			}
			if len(os.Args) == 3 {
				switch os.Args[2] {
				case "-h", "--help", "help":
					fmt.Println(updateHelp())
					return
				}
			}

			version := "latest"
			if len(os.Args) == 3 {
				version = strings.TrimSpace(os.Args[2])
				if version == "" {
					version = "latest"
				}
			}

			if err := runSelfUpdate(version); err != nil {
				fmt.Fprintf(os.Stderr, "lazysvn update failed: %v\n", err)
				os.Exit(1)
			}
			return
		default:
			fmt.Fprintf(os.Stderr, "unknown argument: %s\n\n%s\n", os.Args[1], cliHelp())
			os.Exit(2)
		}
	}

	p := tea.NewProgram(initialModel(), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "lazysvn failed: %v\n", err)
		os.Exit(1)
	}
}

func initialModel() model {
	ti := textinput.New()
	ti.Placeholder = "Commit message"
	ti.Prompt = "> "
	ti.CharLimit = 200

	return model{
		marked:       map[string]struct{}{},
		previewTitle: "Help",
		previewBody:  helpText(),
		statusLine:   "Loading SVN status...",
		commitInput:  ti,
	}
}

func (m model) Init() tea.Cmd {
	return fetchStatusCmd()
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case statusMsg:
		if msg.err != nil {
			m.statusLine = "Failed to run svn status."
			m.previewTitle = "SVN Error"
			m.previewBody = formatSVNResult([]string{"status"}, msg.raw, msg.err)
			return m, nil
		}

		m.entries = msg.entries
		m.cleanupMarks()
		if len(m.entries) == 0 {
			m.cursor = 0
			m.statusLine = "Working copy clean."
		} else {
			if m.cursor >= len(m.entries) {
				m.cursor = len(m.entries) - 1
			}
			m.statusLine = fmt.Sprintf("%d change(s).", len(m.entries))
		}
		return m, nil

	case previewMsg:
		m.previewTitle = msg.title
		m.previewBody = msg.body
		return m, nil

	case actionMsg:
		m.previewTitle = msg.title
		m.previewBody = msg.body
		if strings.Contains(strings.ToLower(msg.body), "error") {
			m.statusLine = "Action failed."
		} else {
			m.statusLine = "Action completed."
		}
		if msg.refresh {
			return m, fetchStatusCmd()
		}
		return m, nil

	case tea.KeyMsg:
		if m.commitPrompt {
			switch msg.String() {
			case "esc":
				m.commitPrompt = false
				m.commitInput.Blur()
				m.statusLine = "Commit canceled."
				return m, nil
			case "enter":
				message := strings.TrimSpace(m.commitInput.Value())
				if message == "" {
					m.statusLine = "Commit message cannot be empty."
					return m, nil
				}
				paths := append([]string(nil), m.commitPaths...)
				m.commitPrompt = false
				m.commitInput.Blur()
				m.statusLine = "Running svn commit..."
				return m, commitCmd(paths, message)
			default:
				var cmd tea.Cmd
				m.commitInput, cmd = m.commitInput.Update(msg)
				return m, cmd
			}
		}

		if m.revertConfirm {
			switch msg.String() {
			case "y":
				paths := append([]string(nil), m.revertPaths...)
				m.revertConfirm = false
				m.statusLine = "Running svn revert..."
				return m, revertCmd(paths)
			case "n", "esc":
				m.revertConfirm = false
				m.statusLine = "Revert canceled."
				return m, nil
			}
			return m, nil
		}

		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "up", "k":
			m.moveCursor(-1)
			return m, m.maybeFollowDiff()
		case "down", "j":
			m.moveCursor(1)
			return m, m.maybeFollowDiff()
		case "g", "home":
			m.cursor = 0
			return m, m.maybeFollowDiff()
		case "G", "end":
			if len(m.entries) > 0 {
				m.cursor = len(m.entries) - 1
			}
			return m, m.maybeFollowDiff()
		case " ":
			m.toggleMark()
			return m, nil
		case "r":
			m.statusLine = "Refreshing..."
			return m, fetchStatusCmd()
		case "enter", "d":
			path := m.currentPath()
			if path == "" {
				m.statusLine = "No file selected."
				return m, nil
			}
			m.statusLine = "Loading diff..."
			return m, diffCmd(path)
		case "l":
			path := m.currentPath()
			if path == "" {
				m.statusLine = "No file selected."
				return m, nil
			}
			m.statusLine = "Loading log..."
			return m, logCmd(path)
		case "u":
			m.statusLine = "Running svn update..."
			return m, updateCmd()
		case "a":
			targets := m.addTargets()
			if len(targets) == 0 {
				m.statusLine = "Select unversioned file(s) with status '?'."
				return m, nil
			}
			m.statusLine = "Running svn add..."
			return m, addCmd(targets)
		case "v":
			targets := m.selectedTargets()
			if len(targets) == 0 {
				m.statusLine = "No file selected."
				return m, nil
			}
			m.revertConfirm = true
			m.revertPaths = targets
			return m, nil
		case "c":
			m.commitPaths = m.commitTargets()
			m.commitPrompt = true
			m.commitInput.SetValue("")
			m.commitInput.Focus()
			m.statusLine = fmt.Sprintf("Commit %d path(s). Enter commit message.", len(m.commitPaths))
			return m, nil
		case "h", "?":
			m.previewTitle = "Help"
			m.previewBody = helpText()
			return m, nil
		}
	}

	return m, nil
}

func (m model) View() string {
	if m.width == 0 || m.height == 0 {
		return "Loading..."
	}

	header := lipgloss.NewStyle().Bold(true).Render("LazySVN - SVN TUI")
	bodyH := m.height - 4
	if bodyH < 6 {
		bodyH = 6
	}

	frameW := paneStyle().GetHorizontalFrameSize()
	frameH := paneStyle().GetVerticalFrameSize()
	contentW := max(8, m.width-frameW)
	body := ""

	// Use a stacked layout in narrower terminals to avoid right-edge clipping.
	if m.width < 110 {
		topOuterH := bodyH * 45 / 100
		if topOuterH < 4 {
			topOuterH = bodyH / 2
		}
		bottomOuterH := bodyH - topOuterH
		top := m.renderChangesPane(contentW, max(3, topOuterH-frameH))
		bottom := m.renderPreviewPane(contentW, max(3, bottomOuterH-frameH))
		body = lipgloss.JoinVertical(lipgloss.Left, top, bottom)
	} else {
		leftTotalW := m.width * 45 / 100
		if leftTotalW < 40 {
			leftTotalW = m.width / 2
		}
		rightTotalW := m.width - leftTotalW
		left := m.renderChangesPane(max(8, leftTotalW-frameW), max(3, bodyH-frameH))
		right := m.renderPreviewPane(max(8, rightTotalW-frameW), max(3, bodyH-frameH))
		body = lipgloss.JoinHorizontal(lipgloss.Top, left, right)
	}

	footer := m.renderFooter(m.width)
	return strings.Join([]string{header, body, footer}, "\n")
}

func paneStyle() lipgloss.Style {
	return lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		Padding(0, 1)
}

func (m model) renderChangesPane(contentWidth, contentHeight int) string {
	border := paneStyle().
		Width(contentWidth).
		Height(contentHeight)

	innerWidth := max(8, contentWidth)
	rows := max(1, contentHeight)

	lines := []string{"Changes"}
	if len(m.entries) == 0 {
		lines = append(lines, "Working copy clean.")
		return border.Render(strings.Join(lines, "\n"))
	}

	start := m.scrollStart(rows - 1)
	end := min(len(m.entries), start+rows-1)
	for i := start; i < end; i++ {
		entry := m.entries[i]
		cursor := " "
		if i == m.cursor {
			cursor = ">"
		}
		mark := " "
		if _, ok := m.marked[entry.Path]; ok {
			mark = "*"
		}
		code := statusCodeStyle(entry.Code).Render(string(entry.Code))
		line := fmt.Sprintf("%s%s [%s] %s", cursor, mark, code, entry.Path)
		lines = append(lines, truncate(line, innerWidth))
	}

	return border.Render(strings.Join(lines, "\n"))
}

func (m model) renderPreviewPane(contentWidth, contentHeight int) string {
	border := paneStyle().
		Width(contentWidth).
		Height(contentHeight)

	innerWidth := max(8, contentWidth)
	rows := max(1, contentHeight)

	lines := []string{m.previewTitle}
	for _, line := range strings.Split(strings.ReplaceAll(m.previewBody, "\r\n", "\n"), "\n") {
		lines = append(lines, truncate(line, innerWidth))
	}
	if len(lines) > rows {
		lines = lines[:rows]
	}

	return border.Render(strings.Join(lines, "\n"))
}

func (m model) renderFooter(width int) string {
	help := "j/k move  space mark  d diff  l log  c commit  a add  v revert  u update  r refresh  q quit"
	if m.commitPrompt {
		text := truncate(fmt.Sprintf("Commit (%d path): %s", len(m.commitPaths), m.commitInput.View()), max(8, width))
		return lipgloss.NewStyle().Foreground(lipgloss.Color("11")).
			Render(text)
	}
	if m.revertConfirm {
		text := truncate(fmt.Sprintf("Revert %d path(s)? Press y or n.", len(m.revertPaths)), max(8, width))
		return lipgloss.NewStyle().Foreground(lipgloss.Color("11")).
			Render(text)
	}
	text := truncate(fmt.Sprintf("%s | %s", help, m.statusLine), max(8, width))
	return lipgloss.NewStyle().Foreground(lipgloss.Color("8")).
		Render(text)
}

func (m *model) moveCursor(delta int) {
	if len(m.entries) == 0 {
		m.cursor = 0
		return
	}
	m.cursor += delta
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.cursor >= len(m.entries) {
		m.cursor = len(m.entries) - 1
	}
}

func (m *model) toggleMark() {
	path := m.currentPath()
	if path == "" {
		return
	}
	if _, ok := m.marked[path]; ok {
		delete(m.marked, path)
		return
	}
	m.marked[path] = struct{}{}
}

func (m model) currentPath() string {
	if len(m.entries) == 0 || m.cursor < 0 || m.cursor >= len(m.entries) {
		return ""
	}
	return m.entries[m.cursor].Path
}

func (m *model) cleanupMarks() {
	if len(m.marked) == 0 {
		return
	}
	active := map[string]struct{}{}
	for _, e := range m.entries {
		active[e.Path] = struct{}{}
	}
	for p := range m.marked {
		if _, ok := active[p]; !ok {
			delete(m.marked, p)
		}
	}
}

func (m model) selectedTargets() []string {
	if len(m.marked) > 0 {
		targets := make([]string, 0, len(m.marked))
		for p := range m.marked {
			targets = append(targets, p)
		}
		sort.Strings(targets)
		return targets
	}
	if p := m.currentPath(); p != "" {
		return []string{p}
	}
	return nil
}

func (m model) commitTargets() []string {
	targets := m.selectedTargets()
	if len(targets) > 0 {
		return targets
	}
	return []string{"."}
}

func (m model) addTargets() []string {
	if len(m.marked) > 0 {
		targets := make([]string, 0, len(m.marked))
		for _, e := range m.entries {
			if e.Code == '?' {
				if _, ok := m.marked[e.Path]; ok {
					targets = append(targets, e.Path)
				}
			}
		}
		return targets
	}

	if len(m.entries) > 0 {
		entry := m.entries[m.cursor]
		if entry.Code == '?' {
			return []string{entry.Path}
		}
	}
	return nil
}

func (m model) maybeFollowDiff() tea.Cmd {
	if strings.HasPrefix(m.previewTitle, "Diff:") {
		if p := m.currentPath(); p != "" {
			return diffCmd(p)
		}
	}
	return nil
}

func (m model) scrollStart(rows int) int {
	if len(m.entries) <= rows {
		return 0
	}
	start := m.cursor - rows/2
	if start < 0 {
		start = 0
	}
	maxStart := len(m.entries) - rows
	if start > maxStart {
		start = maxStart
	}
	return start
}

func fetchStatusCmd() tea.Cmd {
	return func() tea.Msg {
		out, err := runSVN("status")
		if err != nil {
			return statusMsg{raw: out, err: err}
		}
		return statusMsg{entries: parseStatus(out), raw: out}
	}
}

func diffCmd(path string) tea.Cmd {
	return func() tea.Msg {
		args := []string{"diff", path}
		out, err := runSVN(args...)
		return previewMsg{
			title: "Diff: " + path,
			body:  formatSVNResult(args, out, err),
		}
	}
}

func logCmd(path string) tea.Cmd {
	return func() tea.Msg {
		args := []string{"log", "-l", "20", path}
		out, err := runSVN(args...)
		return previewMsg{
			title: "Log: " + path,
			body:  formatSVNResult(args, out, err),
		}
	}
}

func updateCmd() tea.Cmd {
	return func() tea.Msg {
		args := []string{"update"}
		out, err := runSVN(args...)
		return actionMsg{
			title:   "Update",
			body:    formatSVNResult(args, out, err),
			refresh: true,
		}
	}
}

func addCmd(paths []string) tea.Cmd {
	return func() tea.Msg {
		args := append([]string{"add", "--parents"}, paths...)
		out, err := runSVN(args...)
		return actionMsg{
			title:   "Add",
			body:    formatSVNResult(args, out, err),
			refresh: true,
		}
	}
}

func revertCmd(paths []string) tea.Cmd {
	return func() tea.Msg {
		args := append([]string{"revert"}, paths...)
		out, err := runSVN(args...)
		return actionMsg{
			title:   "Revert",
			body:    formatSVNResult(args, out, err),
			refresh: true,
		}
	}
}

func commitCmd(paths []string, message string) tea.Cmd {
	return func() tea.Msg {
		args := []string{"commit", "-m", message}
		args = append(args, paths...)
		out, err := runSVN(args...)
		return actionMsg{
			title:   "Commit",
			body:    formatSVNResult(args, out, err),
			refresh: true,
		}
	}
}

func parseStatus(out string) []statusEntry {
	var entries []statusEntry
	for _, raw := range strings.Split(strings.ReplaceAll(out, "\r\n", "\n"), "\n") {
		line := strings.TrimRight(raw, " ")
		if line == "" {
			continue
		}
		if len(line) < 2 {
			continue
		}
		code := rune(line[0])
		path := ""
		if len(line) >= 9 {
			path = strings.TrimSpace(line[8:])
		}
		if path == "" {
			parts := strings.Fields(line)
			if len(parts) < 2 {
				continue
			}
			path = strings.Join(parts[1:], " ")
		}
		entries = append(entries, statusEntry{
			Code: code,
			Path: path,
			Raw:  line,
		})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })
	return entries
}

func runSVN(args ...string) (string, error) {
	cmd := exec.Command("svn", args...)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	return strings.TrimRight(buf.String(), "\n"), err
}

func formatSVNResult(args []string, out string, err error) string {
	var b strings.Builder
	b.WriteString("$ svn ")
	b.WriteString(strings.Join(args, " "))
	b.WriteString("\n\n")
	if strings.TrimSpace(out) != "" {
		b.WriteString(out)
		b.WriteString("\n")
	}
	if err != nil {
		b.WriteString("\nerror: ")
		b.WriteString(err.Error())
		b.WriteString("\n")
	}
	if strings.TrimSpace(out) == "" && err == nil {
		b.WriteString("(no output)\n")
	}
	return strings.TrimSpace(b.String())
}

func statusCodeStyle(code rune) lipgloss.Style {
	switch code {
	case 'M':
		return lipgloss.NewStyle().Foreground(lipgloss.Color("11"))
	case 'A':
		return lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	case 'D':
		return lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	case '?':
		return lipgloss.NewStyle().Foreground(lipgloss.Color("12"))
	case '!':
		return lipgloss.NewStyle().Foreground(lipgloss.Color("13"))
	case 'C':
		return lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Bold(true)
	default:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("7"))
	}
}

func helpText() string {
	return strings.TrimSpace(`
LazySVN keys:
  j/k or arrows  Move
  g / G          Top / Bottom
  space          Mark/unmark file
  d or Enter     Diff selected file
  l              Show recent log (20 entries) for selected file
  c              Commit marked files (or current file)
  a              svn add for unversioned (? files)
  v              Revert marked/current file (with confirmation)
  u              svn update
  r              Refresh status
  h or ?         Show help
  q              Quit
	`)
}

func cliHelp() string {
	return strings.TrimSpace(`
LazySVN

A LazyGit-style terminal UI for Subversion (SVN).

Usage:
  lazysvn
  lazysvn update [tag]
  lazysvn --help

Command:
  update     Self-update from hosted installer (defaults to latest)
`)
}

func updateHelp() string {
	return strings.TrimSpace(`
Usage:
  lazysvn update
  lazysvn update <tag>

Examples:
  lazysvn update
  lazysvn update v0.1.0

Env:
  INSTALL_DIR            Override install target directory
  LAZYSVN_INSTALL_URL    Override installer URL (default: https://lazysvn.sawirstudio.com/install.sh)
`)
}

func runSelfUpdate(version string) error {
	installURL := strings.TrimSpace(os.Getenv("LAZYSVN_INSTALL_URL"))
	if installURL == "" {
		installURL = "https://lazysvn.sawirstudio.com/install.sh"
	}

	req, err := http.NewRequest(http.MethodGet, installURL, nil)
	if err != nil {
		return fmt.Errorf("create installer request: %w", err)
	}
	req.Header.Set("User-Agent", "lazysvn-self-update")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("fetch installer: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("installer request failed: %s %s", resp.Status, strings.TrimSpace(string(body)))
	}

	script, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read installer: %w", err)
	}
	if len(script) == 0 {
		return fmt.Errorf("installer script is empty")
	}

	installDir := strings.TrimSpace(os.Getenv("INSTALL_DIR"))
	if installDir == "" {
		exePath, err := os.Executable()
		if err == nil {
			installDir = filepath.Dir(exePath)
		}
	}

	fmt.Printf("Updating lazysvn to %s...\n", version)
	cmd := exec.Command("sh")
	cmd.Stdin = bytes.NewReader(script)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(), "VERSION="+version)
	if strings.TrimSpace(os.Getenv("INSTALL_DIR")) == "" && installDir != "" {
		cmd.Env = append(cmd.Env, "INSTALL_DIR="+installDir)
	}

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("run installer: %w", err)
	}

	fmt.Println("Update complete.")
	return nil
}

func truncate(s string, maxLen int) string {
	if maxLen <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= maxLen {
		return s
	}
	if maxLen <= 1 {
		return "…"
	}
	return string(r[:maxLen-1]) + "…"
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
