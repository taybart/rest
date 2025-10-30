package postman

import (
	"encoding/json"
	"errors"
	"fmt"
)

// A Request represents an HTTP request.
type Request struct {
	URL         *URL      `json:"url"`
	Auth        *Auth     `json:"auth,omitempty"`
	Proxy       any       `json:"proxy,omitempty"`
	Certificate any       `json:"certificate,omitempty"`
	Method      Method    `json:"method"`
	Description any       `json:"description,omitempty"`
	Header      []*Header `json:"header,omitempty"`
	Body        *Body     `json:"body,omitempty"`
}

// mRequest is used for marshalling/unmarshalling.
type mRequest Request

// MarshalJSON returns the JSON encoding of a Request.
// If the Request only contains an URL with the Get HTTP method, it is returned as a string.
func (r Request) MarshalJSON() ([]byte, error) {
	if r.Auth == nil && r.Proxy == nil && r.Certificate == nil && r.Description == nil && r.Header == nil && r.Body == nil && r.Method == Get {
		return fmt.Appendf(nil, "\"%s\"", r.URL), nil
	}

	return json.Marshal(mRequest(r))
}

// UnmarshalJSON parses the JSON-encoded data and create a Request from it.
// A Request can be created from an object or a string.
// If a string, the string is assumed to be the request URL and the method is assumed to be 'GET'.
func (r *Request) UnmarshalJSON(b []byte) (err error) {
	switch b[0] {
	case '"':
		r.Method = Get
		r.URL = &URL{
			Raw: string(string(b[1 : len(b)-1])),
		}
	case '{':
		tmp := (*mRequest)(r)
		err = json.Unmarshal(b, &tmp)
	default:
		err = errors.New("unsupported type")
	}

	return
}
