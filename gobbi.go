package gobbi

import (
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"path"
	"strings"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	"github.com/hashicorp/go-retryablehttp"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
)

const (
	// TODO: change this
	DefaultHTTPTimeout = 30
)

var (
	ErrRequestError     = errors.New("error during request")
	ErrRequestFailure   = errors.New("failure during request")
	ErrUnexpectedStatus = fmt.Errorf("%w: unexpected status", ErrRequestFailure)
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
	Verbose         bool                     `yaml:"verbose,omitempty`
	Skip            string                   `yaml:"verbose,omitempty`
	CertValidated   bool                     `yaml:"cert_validated,omitempty"`
	Ssl             bool                     `yaml:"ssl,omitempty"`
	Redirects       int                      `yaml:"redirects,omitempty"`
	UsePriorTest    bool                     `yaml:"use_prior_test,omitempty"`
	Poll            Poll                     `yaml:"poll,omitempty"`
}

type SuiteYAML struct {
	Defaults Case
	Fixtures interface{}
	Tests    []Case
}

func (c *Case) GetBody() io.Reader {
	return nil
}

type Suite struct {
	Name   *string
	Client Requester
	File   string
	Cases  []Case
}

type Requester interface {
	Do(*Case) error
	Log() logr.Logger
	ExecuteOne(*testing.T, *Case)
}

type BaseClient struct {
	Client *http.Client
	log    logr.Logger
}

func NewClient() *BaseClient {
	b := BaseClient{}
	client := retryablehttp.NewClient()
	client.RetryMax = 0 // for now
	httpClient := client.StandardClient()
	httpClient.Timeout = time.Duration(DefaultHTTPTimeout * time.Second)
	b.Client = httpClient
	b.makeLog("gobbi")
	return &b
}

func NewSuiteFromYAMLFile(fileName string) (*Suite, error) {
	data, err := ioutil.ReadFile(fileName)
	if err != nil {
		return nil, err
	}
	sy := SuiteYAML{}
	err = yaml.Unmarshal(data, &sy)
	if err != nil {
		return nil, err
	}

	fmt.Printf("defaults are %+v\n", sy.Defaults)

	// TODO: process for fixtures and defaults, method handling
	processedCases := make([]Case, len(sy.Tests))
	for i, _ := range sy.Tests {
		yamlTest := sy.Tests[i]
		sc, err := makeCaseFromYAML(yamlTest, sy.Defaults)
		if err != nil {
			return nil, err
		}
		processedCases[i] = sc
	}

	name := strings.TrimSuffix(path.Base(fileName), path.Ext(fileName))

	suite := Suite{
		Name:   &name,
		Cases:  processedCases,
		Client: NewClient(),
	}
	return &suite, nil
}

func makeCaseFromYAML(src Case, defaults Case) (Case, error) {
	newCase := defaults
	// Set default defaults! (where zero value is insufficient)
	if newCase.Status == 0 {
		newCase.Status = http.StatusOK
	}
	baseCase := src
	srcBytes, err := yaml.Marshal(baseCase)
	if err != nil {
		return newCase, err
	}
	fmt.Printf("%s\n", string(srcBytes))
	err = yaml.Unmarshal(srcBytes, &newCase)
	if err != nil {
		return newCase, err
	}

	fmt.Printf("newCase is %+v\n", newCase)

	// At this point newCase should now src with any empty values set from
	// defaults, so now set URL and Method if GET etc are set.
	switch {
	case newCase.GET != "":
		newCase.URL = newCase.GET
		newCase.Method = http.MethodGet
	case newCase.POST != "":
		newCase.URL = newCase.POST
		newCase.Method = http.MethodPost
	case newCase.PUT != "":
		newCase.URL = newCase.PUT
		newCase.Method = http.MethodPut
	case newCase.PATCH != "":
		newCase.URL = newCase.PATCH
		newCase.Method = http.MethodPatch
	case newCase.DELETE != "":
		newCase.URL = newCase.DELETE
		newCase.Method = http.MethodDelete
	case newCase.HEAD != "":
		newCase.URL = newCase.HEAD
		newCase.Method = http.MethodHead
	case newCase.OPTIONS != "":
		newCase.URL = newCase.OPTIONS
		newCase.Method = http.MethodOptions
	}

	return newCase, nil
}

// MakeLog creates the intial log for the application.
func (b *BaseClient) makeLog(name string) {
	// Set up the global logger
	zapLog, _ := zap.NewDevelopment()
	b.log = zapr.NewLogger(zapLog).WithName(name)
}

func (b *BaseClient) Do(c *Case) error {
	// TODO: NewRequestWithContext
	rq, err := http.NewRequest(c.Method, c.URL, c.GetBody())
	if err != nil {
		return err
	}

	b.Log().Info("making request", "test", c)

	resp, err := b.Client.Do(rq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	status := resp.StatusCode
	if status != c.Status {
		return fmt.Errorf("%w: expecting %d, got %d", ErrUnexpectedStatus, c.Status, status)
	}
	return nil
}

func (b *BaseClient) ExecuteOne(t *testing.T, c *Case) {
	err := b.Do(c)
	if err != nil {
		t.Errorf("got unexpected error: %v", err)
	}
}

func (s *Suite) Execute(t *testing.T) {
	t.Run(*s.Name, func(t *testing.T) {
		for _, c := range s.Cases {
			t.Run(c.Name, func(t *testing.T) {
				s.Client.ExecuteOne(t, &c)
			})
		}
	})
}

func (b *BaseClient) Log() logr.Logger {
	return b.log
}

func ptrStr(s string) *string {
	return &s
}

func ptrInt(i int) *int {
	return &i
}
