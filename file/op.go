package file

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"

	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"
)

// opCache holds secrets read with op::read for the life of the process, so a
// reference is only read once per run (locals are evaluated on every parse,
// and servers decode per request). Failures aren't cached, so a locked vault
// can be unlocked and retried.
var opCache = struct {
	sync.Mutex
	entries map[string]*opEntry
}{entries: map[string]*opEntry{}}

type opEntry struct {
	done chan struct{}
	val  string
	err  error
}

// opRead reads a secret reference with `op read` (from account, if it isn't
// empty), concurrent reads of the same reference share one call
func opRead(ref, account string) (string, error) {
	key := account + "\x00" + ref
	opCache.Lock()
	if e, ok := opCache.entries[key]; ok {
		opCache.Unlock()
		<-e.done
		return e.val, e.err
	}
	e := &opEntry{done: make(chan struct{})}
	opCache.entries[key] = e
	opCache.Unlock()

	e.val, e.err = runOpRead(ref, account)
	if e.err != nil {
		opCache.Lock()
		delete(opCache.entries, key)
		opCache.Unlock()
	}
	close(e.done)
	return e.val, e.err
}

func runOpRead(ref, account string) (string, error) {
	bin, err := exec.LookPath("op")
	if err != nil {
		return "", errors.New("op::read needs the 1Password CLI (op) on PATH: https://developer.1password.com/docs/cli")
	}
	var stdout, stderr bytes.Buffer
	args := []string{"read", "--no-newline", ref}
	if account != "" {
		args = append(args, "--account", account)
	}
	cmd := exec.Command(bin, args...)
	// op may ask to sign in when the desktop app integration is off
	cmd.Stdin = os.Stdin
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("op read %s: %s", ref, msg)
	}
	return stdout.String(), nil
}

// makeOpReadFunc is op::read("op://vault/item/field" [, account]), account is
// anything `op --account` takes (sign-in address, account ID or email),
// without it op uses OP_ACCOUNT or its default
func makeOpReadFunc() function.Function {
	return function.New(&function.Spec{
		Params: []function.Parameter{
			{
				Name:        "reference",
				Type:        cty.String,
				AllowMarked: true,
			},
		},
		VarParam: &function.Parameter{
			Name:        "account",
			Type:        cty.String,
			AllowMarked: true,
		},
		Type: function.StaticReturnType(cty.String),
		Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
			if len(args) > 2 {
				return cty.NilVal, fmt.Errorf("op::read takes at most 2 arguments (reference, account), got %d", len(args))
			}
			var account string
			if len(args) == 2 {
				acc, _ := args[1].Unmark()
				account = acc.AsString()
			}
			refArg, _ := args[0].Unmark()
			ref := refArg.AsString()
			if !strings.HasPrefix(ref, "op://") {
				return cty.NilVal, fmt.Errorf("op::read takes a secret reference (op://vault/item/field), got %q", ref)
			}
			val, err := opRead(ref, account)
			if err != nil {
				return cty.NilVal, err
			}
			return cty.StringVal(val), nil
		},
	})
}
