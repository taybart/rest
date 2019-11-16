package rest

import (
	"io/ioutil"
	"testing"

	"github.com/matryer/is"
	"github.com/taybart/log"
)

func TestMain(m *testing.M) {
	log.SetLevel(log.DEBUG)
	m.Run()
}

func TestReadGet(t *testing.T) {
	is := is.New(t)
	r := New()
	err := r.Read("./test/get_test.rest")
	is.NoErr(err)
	r.Exec()
}

func TestHasComment(t *testing.T) {
	is := is.New(t)
	r := New()
	err := r.Read("./test/get_comment_test.rest")
	is.NoErr(err)
	r.Exec()
}

func TestReadPost(t *testing.T) {
	is := is.New(t)
	r := New()
	err := r.Read("./test/post_test.rest")
	is.NoErr(err)
	r.Exec()
}

func TestReadMulti(t *testing.T) {
	is := is.New(t)
	r := New()
	err := r.ReadConcurrent("./test/multi_test.rest")
	is.NoErr(err)
	r.Exec()
}

func TestMakeJavascriptRequest(t *testing.T) {
	is := is.New(t)
	r := New()
	err := r.Read("./test/post_test.rest")
	is.NoErr(err)
	requests, err := r.SynthisizeRequest("javascript")
	is.NoErr(err)
	js, err := ioutil.ReadFile("./test/template_request.js")
	is.NoErr(err)
	is.Equal(requests[0], string(js))
}
