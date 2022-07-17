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
	b.Log().Info("checking prior", "name", c.Name, "prior", c.UsePriorTest)
	if c.Done() {
		return nil
	} else if c.UsePriorTest != nil && *c.UsePriorTest {
		prior := c.GetPrior()
		if prior != nil {
			err := b.Do(prior)
			if err != nil {
				return err
			}
		}
	}
	b.Log().Info("doing test", "name", c.Name, "method", c.Method, "url", c.URL)
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

	// TODO: This could consume a lot of memory, but for now this is what
	// we want for being able to refer back to prior tests.
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	seekerBody := bytes.NewReader(respBody)
	c.SetResponseBody(seekerBody)

	// TODO: check all response handlers

	// Wind body to start in case it is not there.
	_, err = c.GetResponseBody().Seek(0, io.SeekStart)
	if err != nil {
		return err
	}

	checkStrings := StringResponseHandler{}
	err = checkStrings.Assert(c)
	if err != nil {
		return err
	}

	c.SetDone()

	return nil
}

func (b *BaseClient) ExecuteOne(t *testing.T, c *Case) {
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
