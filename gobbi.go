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
	Count int
	Delay float32
}

type Case struct {
	Name            string
	Desc            string
	Method          string
	URL             string `yaml:"url"`
	GET             string `yaml:"GET"`
	POST            string `yaml:"POST"`
	PUT             string `yaml:"PUT"`
	DELETE          string `yaml:"DELETE"`
	HEAD            string `yaml:"HEAD"`
	PATCH           string `yaml:"PATCH"`
	OPTIONS         string `yaml:"OPTIONS"`
	Status          int
	RequestHeaders  map[string]string
	QueryParameters map[string][]interface{}
	Data            interface{}
	Xfail           bool
	Verbose         bool
	Skip            string
	CertValidated   bool
	Ssl             bool
	Redirects       int
	UsePriorTest    bool
	Poll            Poll
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
	Name   string
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
	// TODO: process for fixtures and defaults
	name := strings.TrimSuffix(path.Base(fileName), path.Ext(fileName))

	suite := Suite{
		Name:   name,
		Cases:  sy.Tests,
		Client: NewClient(),
	}
	return &suite, nil
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

	b.Log().Info("making request", "method", c.Method, "url", c.URL, "body", c.GetBody())

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
	t.Run(s.Name, func(t *testing.T) {
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
