package rest

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"testing"
	"time"

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
	err := r.Read("./test/get.rest")
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
	err := r.Read("./test/get.rest")
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

func TestVariables(t *testing.T) {
	is := is.New(t)
	r := New()
	err := r.Read("./test/var.rest")
	is.NoErr(err)
	counter := 0
	methods := []string{"GET", "POST"}
	client := NewTestClient(func(r *http.Request) *http.Response {
		// Test request parameters
		is.Equal(r.URL.String(), "http://localhost:8080/")
		is.Equal(r.Method, methods[counter])
		counter++
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
	is.Equal(counter, 2)
}

func TestReadPost(t *testing.T) {
	is := is.New(t)
	r := New()
	err := r.Read("./test/post.rest")
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
	err := r.ReadConcurrent("./test/multi.rest")
	is.NoErr(err)
	counter := 0
	// urls := []string{"http://localhost/", ""}
	// methods := []string{"GET", "POST"}
	// is.Equal(len(urls), len(methods))
	client := NewTestClient(func(r *http.Request) *http.Response {
		// Test request parameters
		// is.Equal(r.URL.String(), urls[counter])
		// is.Equal(r.Method, methods[counter])
		is.Equal(r.URL.String(), "http://localhost:8080/")
		is.Equal(r.Method, "GET")
		counter++
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
	is.Equal(counter, 3)
}

func TestReadConcurrent(t *testing.T) {
	is := is.New(t)
	r := New()
	err := r.ReadConcurrent("./test/multi.rest")
	is.NoErr(err)
	counter := 0
	client := NewTestClient(func(r *http.Request) *http.Response {
		// Test request parameters
		is.Equal(r.URL.String(), "http://localhost:8080/")
		is.Equal(r.Method, "GET")
		counter++
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
	is.Equal(counter, 3)
}

func TestDelay(t *testing.T) {
	is := is.New(t)
	r := New()
	start := time.Now()
	err := r.ReadConcurrent("./test/delay.rest")
	is.NoErr(err)
	client := NewTestClient(func(r *http.Request) *http.Response {
		// Test request parameters
		is.Equal(r.URL.String(), "http://localhost:8080/")
		is.Equal(r.Method, "GET")
		is.True(time.Since(start) > time.Millisecond*999)
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

func TestMakeJavascriptRequest(t *testing.T) {
	is := is.New(t)
	r := New()
	err := r.Read("./test/post.rest")
	is.NoErr(err)
	requests, err := r.SynthisizeRequest("javascript")
	is.NoErr(err)
	js, err := ioutil.ReadFile("./test/template_request.js")
	is.NoErr(err)
	for i, c := range requests[0] {
		is.Equal(rune(js[i]), c)
	}
}
