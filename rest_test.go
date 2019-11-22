package rest

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"testing"

	"github.com/matryer/is"
	// "github.com/taybart/log"
)

// RoundTripFunc .
type RoundTripFunc func(req *http.Request) *http.Response

// RoundTrip .
func (f RoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req), nil
}

//NewTestClient returns *http.Client with Transport replaced to avoid making real calls
func NewTestClient(fn RoundTripFunc) *http.Client {
	return &http.Client{
		Transport: RoundTripFunc(fn),
	}
}

func TestMain(m *testing.M) {
	m.Run()
}

func TestReadGet(t *testing.T) {
	is := is.New(t)
	r := New()
	err := r.Read("./test/get_test.rest")
	is.NoErr(err)

	client := NewTestClient(func(r *http.Request) *http.Response {
		// Test request parameters
		is.Equal(r.URL.String(), "http://localhost:8080/get-test")
		is.Equal(r.Method, "GET")
		return &http.Response{
			StatusCode: 200,
			// Send response to be tested
			Body: ioutil.NopCloser(bytes.NewBufferString(`OK`)),
			// Must be set to non-nil value or it panics
			Header: make(http.Header),
		}
	})
	r.SetClient(client)
	_, failed := r.Exec()
	is.Equal(len(failed), 0)
}

func TestHasComment(t *testing.T) {
	is := is.New(t)
	r := New()
	err := r.Read("./test/get_comment_test.rest")
	is.NoErr(err)
	client := NewTestClient(func(r *http.Request) *http.Response {
		// Test request parameters
		is.Equal(r.URL.String(), "http://localhost:8080/get-test")
		is.Equal(r.Method, "GET")
		return &http.Response{
			StatusCode: 200,
			// Send response to be tested
			Body: ioutil.NopCloser(bytes.NewBufferString(`OK`)),
			// Must be set to non-nil value or it panics
			Header: make(http.Header),
		}
	})
	r.SetClient(client)
	_, failed := r.Exec()
	is.Equal(len(failed), 0)
}

func TestReadPost(t *testing.T) {
	is := is.New(t)
	r := New()
	err := r.Read("./test/post_test.rest")
	is.NoErr(err)
	client := NewTestClient(func(r *http.Request) *http.Response {
		// Test request parameters
		is.Equal(r.URL.String(), "http://localhost:8080/post-test")
		is.Equal(r.Method, "POST")
		return &http.Response{
			StatusCode: 200,
			// Send response to be tested
			Body: ioutil.NopCloser(bytes.NewBufferString(`OK`)),
			// Must be set to non-nil value or it panics
			Header: make(http.Header),
		}
	})
	r.SetClient(client)
	_, failed := r.Exec()
	is.Equal(len(failed), 0)
}

func TestReadMulti(t *testing.T) {
	is := is.New(t)
	r := New()
	err := r.ReadConcurrent("./test/multi_test.rest")
	is.NoErr(err)
	// r.Exec()
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
	for i, c := range requests[0] {
		is.Equal(rune(js[i]), c)
	}
}
