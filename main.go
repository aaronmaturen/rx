package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
)

// Styles
var (
	docStyle = lipgloss.NewStyle().Margin(1, 2)
	titleStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#61AFEF")).
		Bold(true).
		MarginLeft(2)
	errorStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#E06C75")).
		Bold(true)
	successStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#98C379")).
		Bold(true)
)

// ScriptSource represents a source of scripts (package.json or Makefile)
type ScriptSource interface {
	Name() string
	GetScripts() ([]list.Item, error)
	RunScript(name string) error
}

// NPMScriptSource handles scripts from package.json
type NPMScriptSource struct {
	PackageName    string
	PackageVersion string
}

func (n *NPMScriptSource) Name() string {
	return fmt.Sprintf("%s@%s", n.PackageName, n.PackageVersion)
}

func (n *NPMScriptSource) GetScripts() ([]list.Item, error) {
	// Read package.json
	data, err := os.ReadFile("package.json")
	if err != nil {
		return nil, fmt.Errorf("error reading package.json: %w", err)
	}

	// Parse package.json
	var packageJSON struct {
		Name    string            `json:"name"`
		Version string            `json:"version"`
		Scripts map[string]string `json:"scripts"`
	}

	if err := json.Unmarshal(data, &packageJSON); err != nil {
		return nil, fmt.Errorf("error parsing package.json: %w", err)
	}

	if len(packageJSON.Scripts) == 0 {
		return nil, fmt.Errorf("no scripts found in package.json")
	}

	// Set package name and version
	n.PackageName = packageJSON.Name
	n.PackageVersion = packageJSON.Version

	// Create items for the list
	items := []list.Item{}
	for name, cmd := range packageJSON.Scripts {
		items = append(items, item{name: name, description: cmd, source: "npm"})
	}

	return items, nil
}

func (n *NPMScriptSource) RunScript(name string) error {
	// Find the path to npm executable
	npmPath, err := exec.LookPath("npm")
	if err != nil {
		return fmt.Errorf("npm not found: %w", err)
	}

	// Prepare arguments for npm run
	args := []string{"npm", "run", name}

	// Replace the current process with npm run
	return syscall.Exec(npmPath, args, os.Environ())
}

// MakefileScriptSource handles targets from Makefile
type MakefileScriptSource struct{}

func (m *MakefileScriptSource) Name() string {
	return "Makefile"
}

func (m *MakefileScriptSource) GetScripts() ([]list.Item, error) {
	// Check if Makefile exists
	if _, err := os.Stat("Makefile"); os.IsNotExist(err) {
		return nil, fmt.Errorf("Makefile not found")
	}

	// Run make -pn to get all targets
	cmd := exec.Command("make", "-pn")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("error running make -pn: %w", err)
	}

	// Parse the output to find targets
	targets := parseMakefileTargets(string(output))
	if len(targets) == 0 {
		return nil, fmt.Errorf("no targets found in Makefile")
	}

	// Create items for the list
	items := []list.Item{}
	for _, target := range targets {
		items = append(items, item{name: target, description: "make target", source: "make"})
	}

	return items, nil
}

func (m *MakefileScriptSource) RunScript(name string) error {
	// Find the path to make executable
	makePath, err := exec.LookPath("make")
	if err != nil {
		return fmt.Errorf("make not found: %w", err)
	}

	// Prepare arguments for make
	args := []string{"make", name}

	// Replace the current process with make
	return syscall.Exec(makePath, args, os.Environ())
}

// parseMakefileTargets extracts targets from make -pn output
func parseMakefileTargets(output string) []string {
	lines := strings.Split(output, "\n")
	targets := make(map[string]bool)

	for _, line := range lines {
		// Look for lines that define targets (end with :)
		if strings.Contains(line, ":") && !strings.Contains(line, "=") {
			parts := strings.Split(line, ":")
			if len(parts) > 0 {
				target := strings.TrimSpace(parts[0])
				// Filter out targets that start with . or have special characters
				if target != "" && !strings.HasPrefix(target, ".") && 
				   !strings.Contains(target, "#") && !strings.Contains(target, "%") &&
				   !strings.Contains(target, "/") {
					targets[target] = true
				}
			}
		}
	}

	// Convert map to slice
	result := []string{}
	for target := range targets {
		result = append(result, target)
	}

	return result
}

type item struct {
	name        string
	description string
	source      string // "npm" or "make"
}

func (i item) Title() string       { return i.name }
func (i item) Description() string { return i.description }
func (i item) FilterValue() string { return i.name }

type model struct {
	list           list.Model
	selected       string
	quitting       bool
	error          string
	filterInput    string
	allItems       []list.Item
	filterFocused  bool
	form           *huh.Form
	source         ScriptSource
}

func (m model) Init() tea.Cmd {
	// Initialize the form if it's nil
	if m.form == nil {
		m.form = huh.NewForm(
			huh.NewGroup(
				huh.NewInput().
					Title("Filter").
					Placeholder("Type to filter scripts...").
					Value(&m.filterInput).
					Validate(func(s string) error {
						m.filterInput = s
						m.applyFilter()
						return nil // Always valid
					}).
					Key("filter"),
			),
		).WithShowHelp(false).WithShowErrors(false)
	}

	// Initialize the form
	return m.form.Init()
}

// applyFilter filters the list items based on the filter input
func (m *model) applyFilter() {
	if m.filterInput == "" {
		// If filter is empty, show all items
		m.list.SetItems(m.allItems)
		return
	}

	// Filter items based on the filter input
	filtered := []list.Item{}
	for _, item := range m.allItems {
		if strings.Contains(strings.ToLower(item.FilterValue()), strings.ToLower(m.filterInput)) {
			filtered = append(filtered, item)
		}
	}

	m.list.SetItems(filtered)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		if m.filterFocused {
			// When filter is focused, handle special keys
			switch msg.String() {
			case "ctrl+c", "esc":
				m.quitting = true
				return m, tea.Quit
			case "enter", "tab", "down":
				// Move focus to the list
				m.filterFocused = false
				return m, nil
			default:
				// Handle input in the form
				var formCmd tea.Cmd
				formModel, formCmd := m.form.Update(msg)
				m.form = formModel.(*huh.Form)
				
				// Apply filter after form update
				m.applyFilter()
				return m, formCmd
			}
		} else {
			// When list is focused
			switch msg.String() {
			case "ctrl+c", "q":
				m.quitting = true
				return m, tea.Quit
			case "/", "ctrl+f":
				// Switch focus to filter
				m.filterFocused = true
				return m, nil
			case "enter":
				i, ok := m.list.SelectedItem().(item)
				if ok {
					m.selected = i.name
					return m, tea.Quit
				}
			default:
				// Pass key to list
				m.list, cmd = m.list.Update(msg)
				cmds = append(cmds, cmd)
			}
		}

	case tea.WindowSizeMsg:
		h, v := docStyle.GetFrameSize()
		m.list.SetSize(msg.Width-h, msg.Height-v-3) // Reserve space for filter input
		
		if m.form != nil {
			var formCmd tea.Cmd
			formModel, formCmd := m.form.Update(msg)
			m.form = formModel.(*huh.Form)
			cmds = append(cmds, formCmd)
		}
	}

	return m, tea.Batch(cmds...)
}

func (m model) View() string {
	if m.quitting {
		return successStyle.Render("Bye!\n")
	}
	
	if m.error != "" {
		return errorStyle.Render(m.error + "\n")
	}
	
	if m.selected != "" {
		// Use the source name for the running message
		var cmdName string
		if _, ok := m.source.(*NPMScriptSource); ok {
			cmdName = "npm run"
		} else {
			cmdName = "make"
		}
		return successStyle.Render(fmt.Sprintf("Running: %s %s\n", cmdName, m.selected))
	}
	
	// Combine filter input and list view
	var filterView string
	if m.form != nil {
		filterView = m.form.View()
	} else {
		// If form is nil, create a simple filter input display
		filterStyle := lipgloss.NewStyle().Padding(0, 1).Border(lipgloss.NormalBorder(), false, false, true, false).BorderForeground(lipgloss.Color("#61AFEF"))
		filterView = filterStyle.Render("Filter: " + m.filterInput)
	}
	
	listView := m.list.View()
	
	// Add keyboard help
	helpText := "\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("#5C6370")).Render(
		"/ or ctrl+f: filter • ↑/↓: navigate • enter: run script • q: quit",
	)
	
	return docStyle.Render(filterView + "\n" + listView + helpText)
}

// getDefaultInstallPath returns the default installation path for rx
func getDefaultInstallPath() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "/usr/local/bin"
	}
	return filepath.Join(homeDir, ".local", "bin")
}

// getShellConfigPath returns the path to the shell config file
func getShellConfigPath() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	
	// Check for common shell config files
	shellConfigs := []string{
		filepath.Join(homeDir, ".zshrc"),
		filepath.Join(homeDir, ".bashrc"),
		filepath.Join(homeDir, ".bash_profile"),
	}
	
	for _, config := range shellConfigs {
		if _, err := os.Stat(config); err == nil {
			return config
		}
	}
	
	// Default to .zshrc if none found
	return filepath.Join(homeDir, ".zshrc")
}

// installRx installs rx to the specified path
func installRx(installPath string) error {
	// Get the path to the current executable
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}
	
	// Create the install directory if it doesn't exist
	if err := os.MkdirAll(installPath, 0755); err != nil {
		return fmt.Errorf("failed to create install directory: %w", err)
	}
	
	// Copy the executable to the install path
	destPath := filepath.Join(installPath, "rx")
	src, err := os.Open(exePath)
	if err != nil {
		return fmt.Errorf("failed to open source file: %w", err)
	}
	defer src.Close()
	
	dst, err := os.OpenFile(destPath, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0755)
	if err != nil {
		return fmt.Errorf("failed to create destination file: %w", err)
	}
	defer dst.Close()
	
	if _, err := io.Copy(dst, src); err != nil {
		return fmt.Errorf("failed to copy file: %w", err)
	}
	
	return nil
}

// updateShellConfig adds the install path to PATH if not already there
func updateShellConfig(shellConfigPath, installPath string) error {
	// Read the current shell config
	content, err := os.ReadFile(shellConfigPath)
	if err != nil {
		return fmt.Errorf("failed to read shell config: %w", err)
	}
	
	// Check if the path is already in the config
	pathLine := fmt.Sprintf("export PATH=\"$PATH:%s\"", installPath)
	if strings.Contains(string(content), installPath) {
		return nil // Path already in config
	}
	
	// Append the path to the config
	f, err := os.OpenFile(shellConfigPath, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open shell config: %w", err)
	}
	defer f.Close()
	
	if _, err := f.WriteString("\n# Added by rx init\n" + pathLine + "\n"); err != nil {
		return fmt.Errorf("failed to update shell config: %w", err)
	}
	
	return nil
}

// handleInit handles the init command
func handleInit() {
	installPath := getDefaultInstallPath()
	shellConfigPath := getShellConfigPath()
	
	fmt.Println(successStyle.Render("Installing rx to " + installPath))
	
	if err := installRx(installPath); err != nil {
		fmt.Println(errorStyle.Render(fmt.Sprintf("Error installing rx: %v", err)))
		return
	}
	
	if shellConfigPath != "" {
		fmt.Println(successStyle.Render("Updating shell config at " + shellConfigPath))
		if err := updateShellConfig(shellConfigPath, installPath); err != nil {
			fmt.Println(errorStyle.Render(fmt.Sprintf("Error updating shell config: %v", err)))
			return
		}
	}
	
	fmt.Println(successStyle.Render("\nrx has been installed successfully!"))
	fmt.Println("To use rx from any directory, restart your terminal or run:")
	fmt.Printf("  source %s\n", shellConfigPath)
}

// findScriptSource tries to find a suitable script source in the current directory
func findScriptSource() (ScriptSource, []list.Item, error) {
	// Try package.json first
	if _, err := os.Stat("package.json"); err == nil {
		if _, err := exec.LookPath("npm"); err == nil {
			npmSource := &NPMScriptSource{}
			items, err := npmSource.GetScripts()
			if err == nil {
				return npmSource, items, nil
			}
		}
	}

	// Try Makefile next
	if _, err := os.Stat("Makefile"); err == nil {
		if _, err := exec.LookPath("make"); err == nil {
			makeSource := &MakefileScriptSource{}
			items, err := makeSource.GetScripts()
			if err == nil {
				return makeSource, items, nil
			}
		}
	}

	return nil, nil, fmt.Errorf("no valid script source found")
}

func main() {
	// Check if this is the init command
	if len(os.Args) > 1 && os.Args[1] == "init" {
		handleInit()
		return
	}
	
	// Find a suitable script source
	source, items, err := findScriptSource()
	if err != nil {
		fmt.Println(errorStyle.Render(fmt.Sprintf("Error: %v", err)))
		fmt.Println("No package.json or Makefile found in the current directory.")
		return
	}

	// Setup list with custom styling
	delegate := list.NewDefaultDelegate()
	delegate.Styles.SelectedTitle = delegate.Styles.SelectedTitle.Foreground(lipgloss.Color("#61AFEF")).Bold(true)
	delegate.Styles.SelectedDesc = delegate.Styles.SelectedDesc.Foreground(lipgloss.Color("#98C379"))

	l := list.New(items, delegate, 0, 0)
	l.Title = fmt.Sprintf("Available scripts from %s", source.Name())
	l.SetShowStatusBar(false)
	l.Styles.Title = titleStyle

	// Initialize our model
	m := model{
		list:          l,
		allItems:      items,
		filterFocused: true,
		source:        source,
	}

	// Create the filter form with Huh
	m.form = huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Filter").
				Placeholder("Type to filter scripts...").
				Value(&m.filterInput).
				Validate(func(s string) error {
					m.filterInput = s
					m.applyFilter()
					return nil // Always valid
				}).
				Key("filter"),
		),
	).WithShowHelp(false).WithShowErrors(false)

	// Start the Bubble Tea program
	p := tea.NewProgram(m, tea.WithAltScreen())
	finalModel, err := p.Run()
	if err != nil {
		fmt.Println(errorStyle.Render(fmt.Sprintf("Error running program: %v", err)))
		return
	}

	// If a script was selected, run it
	if m, ok := finalModel.(model); ok && m.selected != "" {
		// Verify the selected item is valid
		_, ok := m.list.SelectedItem().(item)
		if !ok {
			fmt.Println(errorStyle.Render("Error: Could not determine script source"))
			return
		}
		
		// Run the script using the appropriate source
		fmt.Println(successStyle.Render(fmt.Sprintf("Running: %s", m.selected)))
		
		err := m.source.RunScript(m.selected)
		if err != nil {
			fmt.Println(errorStyle.Render(fmt.Sprintf("Error executing script: %v", err)))
		}
		// If syscall.Exec succeeds, the program will be replaced, so we won't reach here
	}
}
