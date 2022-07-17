package gobbi

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

var (
	ErrRequestError               = errors.New("error during request")
	ErrRequestFailure             = errors.New("failure during request")
	ErrUnexpectedStatus           = fmt.Errorf("%w: unexpected status", ErrRequestFailure)
	ErrNoDataHandler              = fmt.Errorf("%w: no handler for request content-type", ErrRequestError)
	ErrDataHandlerContentMismatch = fmt.Errorf("%w: data and request content-type mismatch", ErrRequestError)
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
	UsePriorTest    bool                     `yaml:"use_prior_test,omitempty"`
	Poll            Poll                     `yaml:"poll,omitempty"`
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
