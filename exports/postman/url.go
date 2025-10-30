package postman

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
)

// URL is a struct that contains an URL in a "broken-down way".
// Raw contains the complete URL.
type URL struct {
	version   version
	Raw       string        `json:"raw"`
	Protocol  string        `json:"protocol,omitempty"`
	Host      []string      `json:"host,omitempty"`
	Path      []string      `json:"path,omitempty"`
	Port      string        `json:"port,omitempty"`
	Query     []*QueryParam `json:"query,omitempty"`
	Hash      string        `json:"hash,omitempty"`
	Variables []*Variable   `json:"variable,omitempty" mapstructure:"variable"`
}

// mURL is used for marshalling/unmarshalling.
type mURL URL

func MustURL(in string) *URL {

	u, err := url.Parse(in)
	if err != nil {
		panic(err)
	}
	return &URL{
		Raw:      in,
		Protocol: u.Scheme,
		Host:     strings.Split(u.Hostname(), "."),
		Port:     u.Port(),
		Path:     strings.Split(u.Path, "/"),
	}
}
func NewURL(in string) (*URL, error) {

	u, err := url.Parse(in)
	if err != nil {
		return nil, err
	}

	return &URL{
		Raw:      in,
		Protocol: u.Scheme,
		Host:     strings.Split(u.Hostname(), "."),
		Port:     u.Port(),
		Path:     strings.Split(u.Path, "/"),
	}, nil
}

type QueryParam struct {
	Key         string  `json:"key"`
	Value       string  `json:"value"`
	Description *string `json:"description"`
}

// String returns the raw version of the URL.
func (u URL) String() string {
	return u.Raw
}

func (u *URL) setVersion(v version) {
	u.version = v
}

// MarshalJSON returns the JSON encoding of an URL.
// It encodes the URL as a string if it does not contain any variable.
// In case it contains any variable, it gets encoded as an object.
func (u URL) MarshalJSON() ([]byte, error) {

	// Postman Collection are always objects in v2.1.0 but can be strings in v2.0.0
	if u.version == V200 && u.Variables == nil {
		return fmt.Appendf(nil, "\"%s\"", u.Raw), nil
	}

	return json.Marshal(mURL{
		Raw:       u.Raw,
		Protocol:  u.Protocol,
		Host:      u.Host,
		Path:      u.Path,
		Port:      u.Port,
		Query:     u.Query,
		Hash:      u.Hash,
		Variables: u.Variables,
	})
}

// UnmarshalJSON parses the JSON-encoded data and create an URL from it.
// An URL can be created from an object or a string.
// If a string, the value is assumed to be the Raw attribute of the URL.
func (u *URL) UnmarshalJSON(b []byte) (err error) {
	switch b[0] {
	case '"':
		u.Raw = string(b[1 : len(b)-1])
	case '{':
		tmp := (*mURL)(u)
		err = json.Unmarshal(b, &tmp)
	default:
		err = errors.New("unsupported type")
	}

	return
}
