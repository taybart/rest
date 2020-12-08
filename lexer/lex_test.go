package lexer

import (
	"bufio"
	"os"
	"testing"

	"github.com/matryer/is"
)

func parse(fn string) (err error) {
  file, err := os.Open(fn)
  if err != nil {
    return
  }
  defer file.Close()
  scanner := bufio.NewScanner(file)
  lex := New(
    false, // concurrent
  )
  reqs, vars, err := lex.Parse(scanner)
  if err != nil {
    return
  }
  for _, req := range reqs {
    _, err = BuildRequest(req, vars)
    if err != nil {
      return
    }
  }
  return
}

func TestLexFiles(t *testing.T) {
  files := []struct {
    name string
    fn   string
    res  bool
  }{
    {name: "get", fn: "../test/get.rest", res: true},
    {name: "var", fn: "../test/var.rest", res: true},
    {name: "post", fn: "../test/post.rest", res: true},
    {name: "multi", fn: "../test/multi.rest", res: true},
    {name: "delay", fn: "../test/delay.rest", res: true},
    {name: "expect", fn: "../test/expect.rest", res: true},
    {name: "skip", fn: "../test/skip.rest", res: true},
    {name: "runtime", fn: "../test/runtime.rest", res: true},
    {name: "invalid", fn: "../test/invalid.rest", res: false}, // TODO add individual failures
  }
  for _, tt := range files {
    t.Run(tt.name, func(t *testing.T) {
      is := is.New(t)
      err := parse(tt.fn)
      if tt.res {
        is.NoErr(err)
      } else {
        is.True(err != nil)
      }

    })
  }
}
