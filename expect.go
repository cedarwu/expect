// Copyright 2021 cedar, cedarty@gmail.com.

// Package expect is a Go version of the classic TCL Expect.
package expect

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/creack/pty"
	"golang.org/x/term"
)

// DefaultTimeout is the default expect timeout.
const DefaultTimeout = 60 * time.Second

type Expect struct {
	// pty holds the pseudo-terminal tty
	pty *os.File
	// cmd contains the cmd information for the spawned process
	cmd *exec.Cmd
	// timeout contains the default timeout for a spawned command
	timeout time.Duration
	// reader is internal reader of output from spawned process
	reader *os.File
	// scanner scans output from reader
	scanner *bufio.Scanner
	// writer write to stdin
	writer *bufio.Writer
	// oldState holds the old state of terminal
	oldState *term.State
	// signalCh receive certain signals from system
	signalCh chan os.Signal
}

// Spawn starts a process.
func Spawn(command string, timeout time.Duration) (*Expect, error) {
	if len(command) == 0 {
		return nil, errors.New("invalid command")
	}
	if timeout < 1 {
		timeout = DefaultTimeout
	}

	commands := strings.Fields(command)
	cmd := exec.Command(commands[0], commands[1:]...)

	// Start the command with a pty
	ptmx, err := pty.Start(cmd)
	if err != nil {
		return nil, err
	}

	e := &Expect{
		pty:     ptmx,
		cmd:     cmd,
		timeout: timeout,
	}

	e.signalCh = make(chan os.Signal, 1)
	signal.Notify(e.signalCh, syscall.SIGWINCH)
	go func() {
		for sig := range e.signalCh {
			switch sig {
			// handle pty size
			case syscall.SIGWINCH:
				if err := pty.InheritSize(os.Stdin, ptmx); err != nil {
					log.Fatalf("error resizing pty: %s", err)
				}

			default:
				log.Fatalf("Unknown signal: %v", sig)
				_ = e.Wait()
				os.Exit(-1)
			}
		}
	}()
	e.signalCh <- syscall.SIGWINCH // Initial resize

	// Set stdin in raw mode
	e.oldState, err = term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		return nil, err
	}

	var pipeWriter *os.File
	e.reader, pipeWriter, err = os.Pipe()
	if err != nil {
		return nil, err
	}
	e.scanner = bufio.NewScanner(e.reader)

	// Copy pty output to stdout and internal reader for expect
	go func() {
		writer := io.MultiWriter(os.Stdout, pipeWriter)
		_, _ = io.Copy(writer, ptmx)
		pipeWriter.Close()
	}()

	// Copy stdin to the pty
	go func() {
		_, _ = io.Copy(ptmx, os.Stdin)
	}()

	return e, nil
}

// String implements the stringer interface.
func (e *Expect) String() string {
	res := fmt.Sprintf("%p: ", e)
	if e.pty != nil {
		res += fmt.Sprintf("pty: %s ", e.pty.Name())
	}
	if e.cmd != nil {
		res += fmt.Sprintf("cmd: %s(%d) ", e.cmd.Path, e.cmd.Process.Pid)
	}
	return res
}

// Write writes bytes b to stdin.
func (e *Expect) Write(b []byte) (int, error) {
	// log.Printf("console write: %q", b)
	return e.pty.Write(b)
}

// Send writes string s to stdin.
func (e *Expect) Send(s string) (int, error) {
	// log.Printf("console write: %v", s)
	return e.pty.WriteString(s)
}

// SendLine writes string s with newline to stdin.
func (e *Expect) SendLine(s string) (int, error) {
	// log.Printf("console write: %v", s)
	return e.pty.WriteString(s + "\n")
}

// Expect reads spawned processes output looking for pattern.
// Zero timeout means expect forever.
// Negative timeout means Default timeout.
func (e *Expect) Expect(pattern string, timeout time.Duration) (bool, error) {
	matched, err := e.ExpectAny(pattern, nil, timeout)
	return matched != "", err
}

// ExpectRe is similar to Expect, using regexp as match condition.
func (e *Expect) ExpectRe(re *regexp.Regexp, timeout time.Duration) (string, error) {
	return e.ExpectAny("", re, timeout)
}

// ExpectAny is similar to Expect, match string pattern or regexp re.
func (e *Expect) ExpectAny(pattern string, re *regexp.Regexp, timeout time.Duration) (string, error) {
	if timeout < 0 {
		timeout = e.timeout
	}
	e.reader.SetReadDeadline(time.Now().Add(timeout))

	for e.scanner.Scan() {
		text := e.scanner.Text()
		if len(pattern) > 0 {
			if strings.Contains(text, pattern) {
				return pattern, nil
			}
		}
		if re != nil {
			matched := re.FindString(text)
			if len(matched) > 0 {
				return matched, nil
			}
		}
	}

	// did not found the expected output
	e.Wait()

	if e.scanner.Err() != nil {
		return "", e.scanner.Err()
	}

	// cmd exit
	return "", errors.New("command exit")
}

// Interact gives control of the child process to the interactive user (the human at the keyboard).
func (e *Expect) Interact() error {
	err := e.reader.Close()
	if err != nil {
		return err
	}

	_, _ = io.Copy(os.Stdout, e.pty)
	return nil
}

// Wait wait for process finish and do clean jobs.
// Wait should be the last call to Expect.
func (e *Expect) Wait() error {

	_ = e.cmd.Wait()

	err := e.pty.Close()
	if err != nil {
		return err
	}

	signal.Stop(e.signalCh)
	close(e.signalCh)

	// restore terminal state before
	err = term.Restore(int(os.Stdin.Fd()), e.oldState)
	if err != nil {
		return err
	}

	return nil
}
