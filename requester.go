package gobbi

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"testing"

	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
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
	// TODO: consider if retryable is something we want?
	/*
		client := retryablehttp.NewClient()
		client.RetryMax = 0 // for now
		httpClient := client.StandardClient()
		httpClient.Timeout = time.Duration(DefaultHTTPTimeout * time.Second)
	*/
	httpClient := &http.Client{}
	b.Client = httpClient
	b.makeLog("gobbi")
	return &b
}

// MakeLog creates the intial log for the application.
// TODO: if we have one of these per suite, the name should come from the suite.
func (b *BaseClient) makeLog(name string) {
	// Set up the logger
	zapLog, _ := zap.NewDevelopment()
	b.log = zapr.NewLogger(zapLog).WithName(name)
}

func (b *BaseClient) Do(c *Case) error {
	body, err := c.GetRequestBody()
	if err != nil {
		return err
	}
	// TODO: NewRequestWithContext
	rq, err := http.NewRequest(c.Method, c.URL, body)
	if err != nil {
		return err
	}

	rq.Header.Set("content-type", c.RequestHeaders["content-type"])
	rq.Header.Set("accept", c.RequestHeaders["accept"])

	resp, err := b.Client.Do(rq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	status := resp.StatusCode
	if status != c.Status {
		return fmt.Errorf("%w: expecting %d, got %d", ErrUnexpectedStatus, c.Status, status)
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	seekerBody := bytes.NewReader(respBody)

	// TODO: check all response handlers

	checkStrings := StringResponseHandler{}
	err = checkStrings.Assert(c, seekerBody)
	if err != nil {
		return err
	}

	return nil
}

func (b *BaseClient) ExecuteOne(t *testing.T, c *Case) {
	b.Log().Info("executing test", "name", c.Name, "method", c.Method, "url", c.URL)
	if c.Skip != "" {
		t.Skipf("<%s> skipping: %s", c.Name, c.Skip)
	}
	err := b.Do(c)
	if err != nil {
		t.Errorf("got unexpected error: %v", err)
	}
}

func (b *BaseClient) Log() logr.Logger {
	return b.log
}
