package gobbi

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
)

const (
	fileForDataPrefix = "<@"
)

// TODO: Maybe Test instead of Request? Not sure what I was thinking...
var (
	ErrTestError                  = errors.New("error during request")
	ErrTestFailure                = errors.New("failure during request")
	ErrUnexpectedStatus           = fmt.Errorf("%w: unexpected status", ErrTestFailure)
	ErrNoDataHandler              = fmt.Errorf("%w: no handler for request content-type", ErrTestError)
	ErrDataHandlerContentMismatch = fmt.Errorf("%w: data and request content-type mismatch", ErrTestError)
	ErrStringNotFound             = fmt.Errorf("%w: string not found in body", ErrTestFailure)
	ErrJSONPathNotMatched         = fmt.Errorf("%w: json path not matched", ErrTestFailure)
	ErrNoPriorTest                = fmt.Errorf("%w: no prior test", ErrTestError)
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
	Skip            *string                  `yaml:"skip,omitempty"`
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
	suiteFileName            string
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
		return &BinaryDataHandler{}, nil
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

// Open a data file for reading.
// TODO: sandbox the dir!
func (c *Case) ReadFileForData(fileName string) (io.Reader, error) {
	fileName = strings.TrimPrefix(fileName, fileForDataPrefix)
	dir := path.Dir(c.suiteFileName)
	targetFile := path.Join(dir, fileName)
	return os.Open(targetFile)
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

func (c *Case) SetSuiteFileName(fileName string) {
	c.suiteFileName = fileName
}
