package gobbi

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/AsaiYusuke/jsonpath"
	"github.com/google/go-cmp/cmp"
)

var (
	ErrRequestError               = errors.New("error during request")
	ErrRequestFailure             = errors.New("failure during request")
	ErrUnexpectedStatus           = fmt.Errorf("%w: unexpected status", ErrRequestFailure)
	ErrNoDataHandler              = fmt.Errorf("%w: no handler for request content-type", ErrRequestError)
	ErrDataHandlerContentMismatch = fmt.Errorf("%w: data and request content-type mismatch", ErrRequestError)
	ErrStringNotFound             = fmt.Errorf("%w: string not found in body", ErrRequestFailure)
	ErrJSONPathNotMatched         = fmt.Errorf("%w: json path not matched", ErrRequestFailure)
)

type Poll struct {
	Count *int     `yaml:"count,omitempty"`
	Delay *float32 `yaml:"delay,omitempy"`
}

type Case struct {
	Name            string                   `yaml:"name,omitempty"`
	Desc            string                   `yaml:"desc,omitempty"`
	Method          string                   `yaml:"method,omitempty"`
	URL             string                   `yaml:"url,omitempty"`
	GET             string                   `yaml:"GET,omitempty"`
	POST            string                   `yaml:"POST,omitempty"`
	PUT             string                   `yaml:"PUT,omitempty"`
	DELETE          string                   `yaml:"DELETE,omitempty"`
	HEAD            string                   `yaml:"HEAD,omitempty"`
	PATCH           string                   `yaml:"PATCH,omitempty"`
	OPTIONS         string                   `yaml:"OPTIONS,omitempty"`
	Status          int                      `yaml:"status,omitempty"`
	RequestHeaders  map[string]string        `yaml:"request_headers,omitempty"`
	QueryParameters map[string][]interface{} `yaml:"query_parameters,omitempty"`
	Data            interface{}              `yaml:"data,omitempty"`
	Xfail           bool                     `yaml:"xfail,omitempty"`
	Verbose         bool                     `yaml:"verbose,omitempty"`
	Skip            string                   `yaml:"skip,omitempty"`
	CertValidated   bool                     `yaml:"cert_validated,omitempty"`
	Ssl             bool                     `yaml:"ssl,omitempty"`
	Redirects       int                      `yaml:"redirects,omitempty"`
	UsePriorTest    *bool                    `yaml:"use_prior_test,omitempty"`
	Poll            Poll                     `yaml:"poll,omitempty"`
	// TODO: Ideally these would be pluggable, as with gabbi, but it is too
	// hard to figure out how to do that, so we'll fake it for now.
	ResponseHeaders          map[string]string      `yaml:"response_headers,omitempty"`
	ResponseForbiddenHeaders []string               `yaml:"response_forbidden_headers,omitempty"`
	ResponseStrings          []string               `yaml:"response_strings,omitempty"`
	ResponseJSONPaths        map[string]interface{} `yaml:"response_json_paths,omitempty"`
	responseBody             io.ReadSeeker
	done                     bool
	prior                    *Case
}

var jsonPathConfig = jsonpath.Config{}

func init() {
	jsonPathConfig.SetAggregateFunction(`len`, func(params []interface{}) (interface{}, error) {
		return float64(len(params)), nil
	})
}

type RequestDataHandler interface {
	GetBody(c *Case) (io.Reader, error)
}

type JSONDataHandler struct{}
type NilDataHandler struct{}
type TextDataHandler struct{}

func (n *NilDataHandler) GetBody(c *Case) (io.Reader, error) {
	return nil, nil
}

func (j *JSONDataHandler) GetBody(c *Case) (io.Reader, error) {
	data, err := json.Marshal(c.Data)
	return bytes.NewReader(data), err
}

func (t *TextDataHandler) GetBody(c *Case) (io.Reader, error) {
	data, ok := c.Data.(string)
	if !ok {
		return nil, ErrDataHandlerContentMismatch
	}
	return strings.NewReader(data), nil
}

type ResponseHandler interface {
	Assert(*Case) error
}

type StringResponseHandler struct{}

func (s *StringResponseHandler) Assert(c *Case) error {
	if len(c.ResponseStrings) == 0 {
		return nil
	}

	rawBytes, err := io.ReadAll(c.GetResponseBody())
	if err != nil {
		return err
	}
	stringBody := string(rawBytes)
	bodyLength := len(stringBody)
	limit := bodyLength
	if limit > 200 {
		limit = 200
	}
	for _, check := range c.ResponseStrings {
		if !strings.Contains(stringBody, check) {
			return fmt.Errorf("%w: %s not in body: %s", ErrStringNotFound, check, stringBody[:limit])
		}
	}
	return nil
}

type JSONPathResponseHandler struct{}

func deList(i any) any {
	switch x := i.(type) {
	case []interface{}:
		if len(x) == 1 {
			return x[0]
		}
	}
	return i
}

func (j *JSONPathResponseHandler) Assert(c *Case) error {
	if len(c.ResponseJSONPaths) == 0 {
		return nil
	}
	rawBytes, err := io.ReadAll(c.GetResponseBody())
	if err != nil {
		return err
	}
	var rawJSON interface{}
	err = json.Unmarshal(rawBytes, &rawJSON)
	if err != nil {
		return err
	}
	for path, v := range c.ResponseJSONPaths {
		o, err := jsonpath.Retrieve(path, rawJSON, jsonPathConfig)
		output := deList(o)
		if err != nil {
			return err
		}
		// This switch works around numerals in JSON being weird and that it
		// is proving difficult to get a cmp.Transformer to work as expected.
		switch value := v.(type) {
		case int:
			if !cmp.Equal(float64(value), output) {
				return fmt.Errorf("%w: diff: %s", ErrJSONPathNotMatched, cmp.Diff(float64(value), output))
			}
		default:
			if !cmp.Equal(value, output) {
				return fmt.Errorf("%w: diff: %s", ErrJSONPathNotMatched, cmp.Diff(value, output))
			}
		}
	}
	return nil
}

func (c *Case) NewRequestDataHandler() (RequestDataHandler, error) {
	x := c.RequestHeaders["content-type"]
	switch {
	case x == "":
		return &NilDataHandler{}, nil
	case strings.HasPrefix(x, "application/json"):
		return &JSONDataHandler{}, nil
	case strings.HasPrefix(x, "text/plain"):
		return &TextDataHandler{}, nil
	default:
		return nil, ErrNoDataHandler
	}
}

func (c *Case) GetRequestBody() (io.Reader, error) {
	requestDataHandler, err := c.NewRequestDataHandler()
	if err != nil {
		return nil, err
	}
	return requestDataHandler.GetBody(c)
}

func (c *Case) SetResponseBody(body io.ReadSeeker) {
	c.responseBody = body
}

func (c *Case) GetResponseBody() io.ReadSeeker {
	return c.responseBody
}

func (c *Case) SetDone() {
	c.done = true
}

func (c *Case) Done() bool {
	return c.done
}

func (c *Case) GetPrior() *Case {
	return c.prior
}

func (c *Case) SetPrior(p *Case) {
	c.prior = p
}
