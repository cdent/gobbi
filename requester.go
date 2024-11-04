package gobbi

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

const (
	// DefaultHTTPTimeout describes the default timeout used with a test case,
	// exercising the RequestWithContext handling.
	// TODO: change this.
	DefaultHTTPTimeout = 30
)

// The Requester interface is implemented by anything that can take a Case and
// make it happen.
type Requester interface {
	Do(*Case)
	ExecuteOne(*Case)
}

// BaseClient wraps the default http client with a Context and Timeout and
// provides the base from which to make more complex clients.
type BaseClient struct {
	Client  *http.Client
	Context context.Context
	Timeout time.Duration
}

// NewClient returns a new BaseClient with context and Timeout appropriately set.
func NewClient(ctx context.Context) *BaseClient {
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
	b.Context = ctx
	// TODO: Make configurable (per test case?)
	b.Timeout = DefaultHTTPTimeout * time.Second
	return &b
}

// Do executes the current Case, first checking to see if it has any priors that
// have not been executed.
func (b *BaseClient) Do(c *Case) {
	defer c.SetDone()

	if c.Done() {
		c.GetTest().Logf("returning already done from %s", c.Name)
		return
	}

	b.checkPriorTest(c)

	// Do URL replacements
	c.urlReplace()

	if !strings.HasPrefix(c.URL, "http:") && !strings.HasPrefix(c.URL, "https:") {
		c.URL = c.GetDefaultURLBase() + c.URL
	}

	c.GetTest().Logf("url for %s is %s", c.Name, c.URL)

	body, err := c.GetRequestBody()
	if err != nil {
		c.Fatalf("Error while getting request body: %v", err)
	}

	ctx, cancel := context.WithTimeout(b.Context, b.Timeout)
	defer cancel()

	rq, err := http.NewRequestWithContext(ctx, c.Method, c.URL, body)
	if err != nil {
		c.Fatalf("Error creating request: %v", err)
	}

	// Update request headers
	c.RequestHeaders = c.updateRequestHeaders(rq)
	c.dumpRequest(rq)

	resp, err := b.Client.Do(rq)
	if err != nil {
		c.Fatalf("Error making request: %v", err)
	}

	defer func() {
		if err := resp.Body.Close(); err != nil {
			c.Fatalf("Error closing response body: %v", err)
		}
	}()

	c.dumpResponse(resp)

	c.assertStatus(resp)

	// TODO: This could consume a lot of memory, but for now this is what
	// we want for being able to refer back to prior tests.
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		c.Fatalf("Error reading response body: %v", err)
	}

	seekerBody := bytes.NewReader(respBody)
	c.SetResponseBody(seekerBody)

	c.SetResponseHeader(resp.Header)

	// TODO: This returns, which we don't want, we want to continue, which means
	// we need to pass the testing harness around more.
	c.assertHandlers()

	if c.Xfail && !c.GetXFailure() {
		c.SetDone()
		c.GetTest().Fatalf("Test passed when expecting failure.")
	}
}

func (b *BaseClient) checkPriorTest(c *Case) {
	if c.UsePriorTest != nil && *c.UsePriorTest {
		prior := c.GetPrior("")
		if prior != nil && !prior.Done() {
			c.GetTest().Logf("trying to run prior %s", prior.Name)

			parent := c.GetParent()
			if parent == nil {
				c.Fatalf("unable to run prior test %s because no parent", prior.Name)
			}

			c.GetTest().Run(prior.Name, func(u *testing.T) {
				prior.SetTest(u, c.GetTest())
				b.ExecuteOne(prior)
			})
		}
	}
}

// ExecuteOne attempts to execute a single case (by calling Do) but first
// checking if it should be skipped.
func (b *BaseClient) ExecuteOne(c *Case) {
	if c.Skip != nil {
		newSkip, err := StringReplace(c, *c.Skip)
		if err != nil {
			c.Fatalf("Unable to replace strings on skip: %v", err)
		}

		c.Skip = &newSkip
	}

	if c.Skip != nil && *c.Skip != "" {
		c.GetTest().Skipf("<%s> skipping: %s", c.Name, *c.Skip)
	}

	b.Do(c)
}
