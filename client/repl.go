package client

import (
	"fmt"
	"io"
	"syscall"

	"github.com/taybart/log"
	"golang.org/x/term"
)

const (
	CtrlC     byte = 3
	CtrlD     byte = 4 //EOF
	CtrlN     byte = 14
	CtrlP     byte = 16
	ESC       byte = 27
	BACKSPACE byte = 127
)

type REPL struct {
	NoSpecialCmds bool
	input         []rune   // Current input line
	history       []string // Command history
	historyIdx    int      // Current history index
	buf           [1]byte  // Single-byte buffer
	fd            int      // File descriptor
	prompt        string
}

func NewREPL(noSpecialCmds bool) *REPL {
	fd := int(syscall.Stdin)
	return &REPL{
		NoSpecialCmds: noSpecialCmds,
		input:         []rune{},
		history:       []string{},
		historyIdx:    0,
		fd:            fd,
	}
}

func (r *REPL) Loop(do func(string) error, done chan struct{}) error {
	// Save current terminal state to restore later
	oldState, err := term.GetState(r.fd)
	if err != nil {
		panic(err)
	}
	defer term.Restore(r.fd, oldState)
	// go raw
	_, err = term.MakeRaw(r.fd)
	if err != nil {
		return err
	}

	r.prompt = fmt.Sprintf("%s> %s", log.Green, log.Reset)
	fmt.Print(r.prompt)
	for {
		n, err := syscall.Read(r.fd, r.buf[:])
		if err != nil || n == 0 {
			break
		}

		b := r.buf[0]

		switch b {
		case '\r', '\n': // Enter key
			/*** We have a command ***/
			cmd := string(r.input)
			switch cmd {
			case "quit", "exit":
				if !r.NoSpecialCmds {
					close(done)
					return nil
				}
				fallthrough
			default:
				if len(cmd) != 0 {
					r.history = append(r.history, cmd)
					r.historyIdx = len(r.history)

					r.input = r.input[:0]
					if err := do(cmd); err != nil {
						fmt.Printf("\r\n%s\n\r%s",
							err, r.prompt)
						continue
					}
				}
				fmt.Printf("\n\r%s", r.prompt)
			}

		case BACKSPACE:
			if len(r.input) > 0 {
				r.input = r.input[:len(r.input)-1]
				fmt.Print("\b \b") // Erase last character visually
			}

		case CtrlP: // (previous command)
			r.PreviousCmd()
		case CtrlN: // (next command)
			r.NextCmd()
		case CtrlC, CtrlD: // SIGINT, EOF
			close(done)
			return nil
		case ESC: // ESC (terminal escape sequence)
			// Read next two bytes for arrow key sequence
			if r.readArrow() {
				continue // Skip echoing
			}

		// Handle other printable characters
		default:
			r.input = append(r.input, rune(b))
			fmt.Print(string(b))
		}
	}

	return nil
}

func (r *REPL) ReadLine(prompt string) (string, error) {
	oldState, err := term.GetState(r.fd)
	if err != nil {
		return "", err
	}
	defer term.Restore(r.fd, oldState)

	_, err = term.MakeRaw(r.fd)
	if err != nil {
		return "", err
	}

	r.prompt = prompt
	r.input = r.input[:0]
	r.historyIdx = len(r.history)
	fmt.Print(r.prompt)

	for {
		n, err := syscall.Read(r.fd, r.buf[:])
		if err != nil || n == 0 {
			return "", io.EOF
		}

		b := r.buf[0]

		switch b {
		case '\r', '\n': // Enter key
			cmd := string(r.input)
			if len(cmd) != 0 {
				r.history = append(r.history, cmd)
				r.historyIdx = len(r.history)
			}
			fmt.Print("\r\n")
			return cmd, nil

		case BACKSPACE:
			if len(r.input) > 0 {
				r.input = r.input[:len(r.input)-1]
				fmt.Print("\b \b")
			}

		case CtrlP:
			r.PreviousCmd()
		case CtrlN:
			r.NextCmd()
		case CtrlC, CtrlD:
			fmt.Print("\r\n")
			return "", io.EOF
		case ESC:
			if r.readArrow() {
				continue
			}

		default:
			r.input = append(r.input, rune(b))
			fmt.Print(string(b))
		}
	}
}

func (r *REPL) PreviousCmd() {
	if len(r.history) > 0 && r.historyIdx > 0 {
		r.historyIdx--
		r.input = []rune(r.history[r.historyIdx])
		fmt.Printf("\033[2K\r%s%s", r.prompt, string(r.input))
	}
}
func (r *REPL) NextCmd() {
	if len(r.history) > 0 && r.historyIdx < len(r.history) {
		fmt.Printf("\033[2K\r%s", r.prompt)
		if r.historyIdx == len(r.history)-1 {
			r.input = []rune{}
			return
		}
		r.historyIdx++
		r.input = []rune(r.history[r.historyIdx])
		fmt.Print(string(r.input))
	}
}

// readArrow checks for arrow key sequences and updates input from history
func (r *REPL) readArrow() bool {
	buf := make([]byte, 2)
	n, _ := syscall.Read(r.fd, buf)
	if n == 2 && buf[0] == '[' {
		switch buf[1] {
		case 'A': // Up Arrow
			r.PreviousCmd()
			return true
		case 'B': // Down Arrow
			r.NextCmd()
			return true
		}
	}
	return false
}
