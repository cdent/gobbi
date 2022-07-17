package gobbi

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	"github.com/hashicorp/go-retryablehttp"
	"go.uber.org/zap"
)

const (
	// TODO: change this
	DefaultHTTPTimeout = 30
)

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

	b.Log().Info("making request", "method", c.Method, "url", c.URL)

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

func (b *BaseClient) Log() logr.Logger {
	return b.log
}
