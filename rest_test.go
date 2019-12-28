package rest

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"testing"
	"time"

	"github.com/matryer/is"
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

func TestInvalidFile(t *testing.T) {
	is := is.New(t)
	r := New()
	valid, _ := r.IsRestFile("./test/invalid.rest")
	is.True(!valid)
}

func TestExecIndex(t *testing.T) {
	is := is.New(t)
	r := New()
	err := r.Read("./test/index.rest")
	is.NoErr(err)
	client := NewTestClient(func(r *http.Request) *http.Response {
		is.Equal(r.URL.String(), "http://localhost:8080/should/exec")
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
	_, err = r.ExecIndex(1)
	is.NoErr(err)
	_, err = r.ExecIndex(5)
	is.Equal(err.Error(), "Block 5 does not exist")
}

func TestExpect(t *testing.T) {
	is := is.New(t)
	r := New()
	err := r.Read("./test/expect.rest")
	is.NoErr(err)
	client := NewTestClient(func(r *http.Request) *http.Response {
		is.Equal(r.Method, "GET")
		url := r.URL.String()
		if url == "http://localhost:8080/user?id=123" {
			return &http.Response{
				StatusCode: 404,
				// Send response to be tested
				Body: ioutil.NopCloser(bytes.NewBufferString(`{ "error": "Unknown ID" }`)),
				// Must be set to non-nil value or it panics
				Header: make(http.Header),
			}
		}
		is.Equal(url, "http://localhost:8080/user?id=1234")
		return &http.Response{
			StatusCode: 200,
			// Send response to be tested
			Body: ioutil.NopCloser(bytes.NewBufferString(`{ "id": "1234", "name": "taybart" }`)),
			// Must be set to non-nil value or it panics
			Header: make(http.Header),
		}
	})
	r.SetClient(client)
	_, failed := r.Exec()
	is.Equal(len(failed), 0)
	// is.NoErr(err)
}
