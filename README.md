# goclip

A lightweight CLI tool to capture stdin and send it to the system clipboard. It solves the issue of manual text selection in the terminal and works where standard tools like wl-copy fail, including SSH sessions and TTY.

## Key Features

- Streaming: View output in real-time while it is being captured to the clipboard.
- ANSI Stripping: Automatically removes terminal escape codes (colors/formatting) for clean pasting.
- OSC 52 Support: Works over SSH and in TTY by sending escape sequences to your terminal emulator.
- Safety Limit: Hard-capped at 10MB
- Smart Detection: Automatically switches between Wayland (wl-copy), X11 (xclip/xsel), and OSC 52.

## Installation

The included Makefile handles directory detection (uses /usr/bin on Arch Linux and /usr/local/bin elsewhere).

### Bash

```bash
git clone https://github.com/youruser/goclip
cd goclip
make
sudo make install
```

## Usage

### Basic copy:

```bash
ls -la | goclip
```

### Copy with notification and whitespace trimming:

```bash
pwd | goclip -t -n
```

### Quiet mode with file logging (no terminal output):

```bash
./build.sh | goclip -q -f build.log
```

### Capture errors (stderr):

```bash
make 2>&1 | goclip
```

## Options

| Flag      | Description                                                |
|-----------|------------------------------------------------------------|
| `-q`      | Quiet mode – no output to stdout.                          |
| `-s`      | Strip ANSI codes (default: true).                          |
| `-t`      | Trim leading/trailing whitespace.                          |
| `-n`      | Send a desktop notification (requires notify-send).        |
| `-f [path]` | Save output to a specific file.                          |
| `-a`      | Append to file (used with -f).                             |
| `--no-clip` | Disable clipboard copying (useful for file-only logging). |
| `-h`      | Show help and examples.                                    |

## Requirements

- Wayland: wl-clipboard (recommended)
- X11: xclip or xsel
- SSH/TTY: A terminal emulator that supports OSC 52 (e.g., Alacritty, Foot, Kitty, Zed, or VS Code terminal).
