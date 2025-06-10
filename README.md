# rx - Script Runner

A sleek TUI application that finds and runs scripts from your package.json or Makefile.

## Features

- Automatically detects package.json or Makefile in the current directory
- Lists all available npm scripts or make targets
- Real-time filtering as you type to quickly find scripts
- Clean process replacement (runs as the actual command instead of staying as rx)
- Keyboard-driven interface with intuitive navigation
- Styled UI with syntax highlighting

## Installation

To install rx globally so it can be used from any directory:

```bash
# Build the executable
go build -o rx .

# Run the init command to install rx to your PATH
./rx init

# Reload your shell configuration
source ~/.zshrc  # or ~/.bashrc, ~/.bash_profile depending on your shell
```

The `init` command will:
1. Copy the rx executable to `~/.local/bin/` (or `/usr/local/bin/` as fallback)
2. Add this directory to your PATH in your shell configuration file

## Usage

```bash
# Navigate to a directory with a package.json file
cd your-npm-project

# Run rx to see and select available npm scripts
rx
```

## Controls

- **Filtering:**
  - Just start typing to filter scripts
  - `/` or `Ctrl+F`: Focus the filter input
  - `Enter`/`Tab`/`Down`: Move from filter to list

- **Navigation:**
  - `↑`/`↓`: Navigate through scripts
  - `Enter`: Run selected script
  - `q` or `Ctrl+C`: Quit

## Build

```bash
go build -o rx .
```

## Requirements

- Go 1.21 or higher
- npm (for running the selected scripts)
