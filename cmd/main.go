package main

import (
	"bytes"
	"encoding/base64"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strings"
)

const maxBufferSize = 10 * 1024 * 1024 // 10 MB

// ansiRE matches common ANSI/OSC/DCS escape sequences so we can strip them.
// Compiled once for performance.
var ansiRE = regexp.MustCompile(
	`\x1b(?:\[[0-9;?]*[ -/]*[@-~]|\][^\x07\x1b]*(?:\x07|\x1b\\)|[PX^_][^\x1b]*\x1b\\|[()][AB012]|[A-Z\\])`,
)

// stripANSI removes terminal control sequences from s.
func stripANSI(s string) string {
	return ansiRE.ReplaceAllString(s, "")
}

// detectClipboardCmd returns a clipboard helper command if available.
// It looks for wl-copy (Wayland), then xclip/xsel (X11). The bool indicates
// whether an external helper was found.
func detectClipboardCmd() (string, []string, bool) {
	// Prefer wl-copy on Wayland
	if os.Getenv("WAYLAND_DISPLAY") != "" {
		if p, err := exec.LookPath("wl-copy"); err == nil {
			return p, nil, true
		}
	}
	// X11 helpers
	if os.Getenv("DISPLAY") != "" {
		if p, err := exec.LookPath("xclip"); err == nil {
			return p, []string{"-selection", "clipboard"}, true
		}
		if p, err := exec.LookPath("xsel"); err == nil {
			return p, []string{"--clipboard", "--input"}, true
		}
	}
	// Try wl-copy anywhere as a last-ditch helper
	if p, err := exec.LookPath("wl-copy"); err == nil {
		return p, nil, true
	}
	return "", nil, false
}

// writeUsingCmd pipes content to an external clipboard helper.
// wl-copy acts as a clipboard server and never exits on its own — passing
// --paste-once makes it exit immediately after the first paste request is
// served (or right after the data is offered), which prevents goclip from
// hanging indefinitely.
func writeUsingCmd(bin string, args []string, content string) error {
	// wl-copy without any flag forks into the background and never exits,
	// so cmd.Wait() would block forever. --paste-once (-o) tells wl-copy to
	// exit as soon as the clipboard content has been served once, which is
	// the correct one-shot behaviour for a pipe tool.
	if strings.HasSuffix(bin, "wl-copy") {
		args = append([]string{"--paste-once"}, args...)
	}

	cmd := exec.Command(bin, args...)
	cmd.Env = os.Environ()

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("%s: stdin pipe: %w", bin, err)
	}

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("%s: start: %w", bin, err)
	}

	if _, err := io.WriteString(stdin, content); err != nil {
		_ = cmd.Process.Kill()
		return fmt.Errorf("%s: write stdin: %w", bin, err)
	}
	// Close stdin so the helper knows input is done.
	if err := stdin.Close(); err != nil {
		_ = cmd.Process.Kill()
		return fmt.Errorf("%s: close stdin: %w", bin, err)
	}

	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("%s failed: %v (%s)", bin, err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

// writeClipboardOSC52 attempts to copy via OSC 52 sequence written to /dev/tty.
// Many modern terminal emulators support it. This avoids external binaries.
func writeClipboardOSC52(content string) error {
	tty, err := os.OpenFile("/dev/tty", os.O_WRONLY, 0)
	if err != nil {
		return fmt.Errorf("open /dev/tty: %w", err)
	}
	defer tty.Close()

	enc := base64.StdEncoding.EncodeToString([]byte(content))
	seq := fmt.Sprintf("\x1b]52;c;%s\x07", enc) // clipboard ('c')
	_, err = io.WriteString(tty, seq)
	if err != nil {
		return fmt.Errorf("write OSC52: %w", err)
	}
	return nil
}

// writeToClipboard tries external helpers first, then falls back to OSC 52.
func writeToClipboard(content string) error {
	if bin, args, ok := detectClipboardCmd(); ok {
		if err := writeUsingCmd(bin, args, content); err == nil {
			return nil
		} // if it fails, try OSC52 as fallback
	}
	if err := writeClipboardOSC52(content); err != nil {
		return fmt.Errorf("no external clipboard helper and OSC52 failed: %w", err)
	}
	return nil
}

func writeToFile(path, content string, appendMode bool) error {
	flags := os.O_CREATE | os.O_WRONLY
	if appendMode {
		flags |= os.O_APPEND
	} else {
		flags |= os.O_TRUNC
	}
	f, err := os.OpenFile(path, flags, 0o644)
	if err != nil {
		return fmt.Errorf("open file: %w", err)
	}
	defer f.Close()

	if _, err := io.WriteString(f, content); err != nil {
		return fmt.Errorf("write file: %w", err)
	}
	return nil
}

func usageText(prog string) string {
	return fmt.Sprintf(`%s — copy piped output to the system clipboard and optionally log it.

Usage:
  some_command | %s [options]

Examples:
  ls -la | %s                     # copy stdout to clipboard
  mytool 2>&1 | %s                 # copy both stdout and stderr
  some_cmd | %s -f output.log      # save a copy to a file and copy to clipboard
  some_cmd | %s -q --no-clip       # don't print to stdout, only save to file (if -f) or nothing

Options:
`, prog, prog, prog, prog, prog, prog)
}

func main() {
	// Flags
	quiet := flag.Bool("q", false, "quiet — don't print piped input to stdout")
	strip := flag.Bool("s", true, "strip ANSI control sequences before copying")
	trim := flag.Bool("t", false, "trim leading/trailing whitespace before copying")
	notify := flag.Bool("n", false, "send a desktop notification after copying")
	logFile := flag.String("f", "", "save output to file (overwrites unless -a)")
	appendFile := flag.Bool("a", false, "append to file when used with -f")
	noClip := flag.Bool("no-clip", false, "do not copy to clipboard (useful with -f)")
	help := flag.Bool("h", false, "show help")
	flag.Parse()

	if *help {
		fmt.Print(usageText(os.Args[0]))
		flag.PrintDefaults()
		return
	}

	// Ensure there is piped input on stdin
	stat, err := os.Stdin.Stat()
	if err != nil {
		fmt.Fprintln(os.Stderr, "error: unable to stat stdin:", err)
		os.Exit(1)
	}
	if (stat.Mode() & os.ModeCharDevice) != 0 {
		fmt.Fprintln(os.Stderr, "No piped input detected. Use: some_command |", os.Args[0])
		fmt.Fprintln(os.Stderr, "Use -h for help and examples.")
		os.Exit(1)
	}

	// Read stream with a size limit to avoid OOM for very large inputs.
	var buf bytes.Buffer
	var dest io.Writer
	if *quiet {
		dest = &buf
	} else {
		dest = io.MultiWriter(os.Stdout, &buf)
	}

	limited := io.LimitReader(os.Stdin, maxBufferSize)
	if _, err := io.Copy(dest, limited); err != nil {
		fmt.Fprintln(os.Stderr, "read error:", err)
		os.Exit(1)
	}

	output := buf.String()
	if *strip {
		output = stripANSI(output)
	}
	if *trim {
		output = strings.TrimSpace(output)
	}

	if output == "" {
		// nothing to do
		if !*quiet {
			fmt.Fprintln(os.Stderr, "No content to copy.")
		}
		return
	}

	// Optional file logging
	if *logFile != "" {
		if err := writeToFile(*logFile, output, *appendFile); err != nil {
			fmt.Fprintln(os.Stderr, "file write error:", err)
			os.Exit(1)
		}
		if !*quiet {
			fmt.Fprintf(os.Stderr, "Saved %d bytes to %s\n", len(output), *logFile)
		}
	}

	// Clipboard copy
	if !*noClip {
		if err := writeToClipboard(output); err != nil {
			fmt.Fprintln(os.Stderr, "clipboard error:", err)
			fmt.Fprintln(os.Stderr, "Hint: install wl-clipboard (wl-copy) or xclip/xsel, or use a terminal that supports OSC 52.")
			os.Exit(1)
		}
		if !*quiet {
			fmt.Fprintln(os.Stderr, "Copied to clipboard.")
		}
	}

	// Desktop notification (best-effort)
	if *notify {
		_ = exec.Command("notify-send", "goclip", "Content copied to clipboard").Run()
	}
}
