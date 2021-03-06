package rest

import (
	"bytes"
	"encoding/json"
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
		Transport: fn,
	}
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
	_, err = r.Exec()
	is.NoErr(err)
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
	_, err = r.Exec()
	is.NoErr(err)
	is.Equal(counter, 2)
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
	_, err = r.Exec()
	is.NoErr(err)
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
	_, err = r.Exec()
	is.NoErr(err)
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
	_, err = r.Exec()
	is.NoErr(err)
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
	_, err = r.Exec()
	is.NoErr(err)
}

func TestRuntimeVariables(t *testing.T) {
	is := is.New(t)
	r := New()
	err := r.Read("./test/runtime.rest")
	is.NoErr(err)

	loginCalled := false
	accountCalled := false
	client := NewTestClient(func(r *http.Request) *http.Response {
		switch r.URL.Path {
		case "/login":
			t.Log("login")
			var j map[string]string
			err = json.NewDecoder(r.Body).Decode(&j)
			is.NoErr(err)

			is.Equal(j["username"], "test")
			is.Equal(j["password"], "password")

			loginCalled = true

			// Test request parameters
			return &http.Response{
				StatusCode: 200,
				// Send response to be tested
				Body: ioutil.NopCloser(bytes.NewBufferString(`{"auth_token": "test"}`)),
				// Must be set to non-nil value or it panics
				Header: make(http.Header),
			}
		case "/account":
			t.Log("account")
			if r.Header.Get("Authorization") != "Bearer test" {
				t.Fatal("auth_token was not present during second call")
			}
			accountCalled = true
			return &http.Response{
				StatusCode: 200,
				// Must be set to non-nil value or it panics
				Header: make(http.Header),
			}
		default:
			t.Fatal("Unknown url called")
			return nil
		}
	})

	r.SetClient(client)
	_, err = r.Exec()
	is.NoErr(err)

	is.True(loginCalled)
	is.True(accountCalled)
}
