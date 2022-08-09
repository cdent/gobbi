package gobbi

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"runtime"
	"strings"
	"testing"
)

const (
	fileForDataPrefix = "<@"
)

// TODO: Maybe Test instead of Request? Not sure what I was thinking...
var (
	ErrTestError                   = errors.New("error during request")
	ErrTestFailure                 = errors.New("failure during request")
	ErrUnexpectedStatus            = fmt.Errorf("%w: unexpected status", ErrTestFailure)
	ErrNoDataHandler               = fmt.Errorf("%w: no handler for request content-type", ErrTestError)
	ErrDataHandlerContentMismatch  = fmt.Errorf("%w: data and request content-type mismatch", ErrTestError)
	ErrStringNotFound              = fmt.Errorf("%w: string not found in body", ErrTestFailure)
	ErrJSONPathNotMatched          = fmt.Errorf("%w: json path not matched", ErrTestFailure)
	ErrNoPriorTest                 = fmt.Errorf("%w: no prior test", ErrTestError)
	ErrHeaderNotPresent            = fmt.Errorf("%w: missing header", ErrTestFailure)
	ErrHeaderValueMismatch         = fmt.Errorf("%w: header value mismatch", ErrTestFailure)
	ErrEnvironmentVariableNotFound = fmt.Errorf("%w: environment variable not found", ErrTestError)
)

type Poll struct {
	Count *int     `yaml:"count,omitempty"`
	Delay *float32 `yaml:"delay,omitempy"`
}

func (c *Case) Errorf(format string, args ...any) {
	_, fileName, lineNumber, _ := runtime.Caller(1)
	baseName := path.Base(fileName)
	format = fmt.Sprintf("%s:%d: %s", baseName, lineNumber, format)
	if !c.Xfail {
		c.GetTest().Errorf(format, args...)
	} else {
		s := fmt.Sprintf(format, args...)
		c.SetXFailure()
		c.GetTest().Logf("ignoring error in xfail: %s", s)
	}
}

func (c *Case) Fatalf(format string, args ...any) {
	_, fileName, lineNumber, _ := runtime.Caller(1)
	baseName := path.Base(fileName)
	format = fmt.Sprintf("%s:%d: %s", baseName, lineNumber, format)
	if !c.Xfail {
		c.GetTest().Fatalf(format, args...)
	} else {
		s := fmt.Sprintf(format, args...)
		c.SetXFailure()
		c.GetTest().Skipf("skipping in xfail after: %s", s)
	}
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
	Redirects       int                      `yaml:"redirects,omitempty"`
	UsePriorTest    *bool                    `yaml:"use_prior_test,omitempty"`
	Poll            Poll                     `yaml:"poll,omitempty"`
	// SSL is ignored but we parse it for compatibility with gabbi.
	SSL *bool `yaml:"ssl,omitempty"`
	// TODO: Ideally these would be pluggable, as with gabbi, but it is too
	// hard to figure out how to do that, so we'll fake it for now.
	ResponseHeaders          map[string]string      `yaml:"response_headers,omitempty"`
	ResponseForbiddenHeaders []string               `yaml:"response_forbidden_headers,omitempty"`
	ResponseStrings          []string               `yaml:"response_strings,omitempty"`
	ResponseJSONPaths        map[string]interface{} `yaml:"response_json_paths,omitempty"`
	responseBody             io.ReadSeeker
	responseHeader           http.Header
	done                     bool
	prior                    *Case
	suiteFileName            string
	test                     *testing.T
	parent                   *testing.T
	defaultURLBase           string
	xfailure                 bool
}

func (c *Case) NewRequestDataHandler() (RequestDataHandler, error) {
	x := c.RequestHeaders["content-type"]
	// TODO: use Accepts() for these!
	switch {
	case x == "":
		switch c.Data.(type) {
		case string:
			return &TextDataHandler{}, nil
		default:
			return &NilDataHandler{}, nil
		}
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

func (c *Case) SetResponseHeader(h http.Header) {
	c.responseHeader = h
}

func (c *Case) GetResponseHeader() http.Header {
	return c.responseHeader
}

// Open a data file for reading.
// TODO: sandbox the dir!
func (c *Case) ReadFileForData(fileName string) (io.Reader, error) {
	fileName = strings.TrimPrefix(fileName, fileForDataPrefix)
	dir := path.Dir(c.suiteFileName)
	targetFile := path.Join(dir, fileName)
	return os.Open(targetFile)
}

func (c *Case) ParsedURL() *url.URL {
	// Ignore the error because we can't be here without a valid url.
	u, _ := url.Parse(c.URL)
	return u
}

func (c *Case) SetDone() {
	c.done = true
}

func (c *Case) Done() bool {
	return c.done
}

func (c *Case) GetPrior(caseName string) *Case {
	prior := c.prior
	if caseName == "" {
		return prior
	}
	if prior.Name == caseName {
		return prior
	}
	return prior.GetPrior(caseName)
}

func (c *Case) SetPrior(p *Case) {
	c.prior = p
}

func (c *Case) SetSuiteFileName(fileName string) {
	c.suiteFileName = fileName
}

func (c *Case) SetTest(t *testing.T, parent *testing.T) {
	c.test = t
	c.parent = parent
}

func (c *Case) GetTest() *testing.T {
	return c.test
}

func (c *Case) GetParent() *testing.T {
	return c.parent
}

func (c *Case) SetXFailure() {
	c.xfailure = true
}

func (c *Case) GetXFailure() bool {
	return c.xfailure
}

func (c *Case) SetDefaultURLBase(s string) {
	c.defaultURLBase = s
}

func (c *Case) GetDefaultURLBase() string {
	return c.defaultURLBase
}
