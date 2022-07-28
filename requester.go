package gobbi

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"strings"
	"testing"
)

const (
	// TODO: change this
	DefaultHTTPTimeout = 30
)

type Requester interface {
	Do(*Case)
	ExecuteOne(*Case)
}

type BaseClient struct {
	Client *http.Client
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
	return &b
}

func (b *BaseClient) Do(c *Case) {
	defer c.SetDone()
	if c.Done() {
		c.GetTest().Logf("returning already done from %s", c.Name)
		return
	} else if c.UsePriorTest != nil && *c.UsePriorTest {
		prior := c.GetPrior("")
		if prior != nil && !prior.Done() {
			c.GetTest().Logf("trying to run prior %s", prior.Name)
			parent := c.GetParent()
			if parent == nil {
				c.Fatalf("unable to run prior test %s because no parent", prior.Name)
			}
			parent.Run(prior.Name, func(u *testing.T) {
				prior.SetTest(u, parent)
				b.ExecuteOne(prior)
			})
		}
	}
	// Do URL replacements
	url, err := StringReplace(c, c.URL)
	if err != nil {
		c.Errorf("StringReplace failed: %v", err)
	}
	c.URL = url

	if !strings.HasPrefix(c.URL, "http:") && !strings.HasPrefix(c.URL, "https:") {
		c.URL = c.GetDefaultURLBase() + c.URL
	}

	body, err := c.GetRequestBody()
	if err != nil {
		c.Fatalf("Error while getting request body: %v", err)
	}
	// TODO: NewRequestWithContext
	rq, err := http.NewRequest(c.Method, c.URL, body)
	if err != nil {
		c.Fatalf("Error creating request: %v", err)
	}

	rq.Header.Set("content-type", c.RequestHeaders["content-type"])
	rq.Header.Set("accept", c.RequestHeaders["accept"])

	var verboseOutput string
	if c.Verbose {
		// TODO: Test for textual content-type header to set body true or false.
		dump, err := httputil.DumpRequestOut(rq, true)
		if err != nil {
			c.GetTest().Logf("unable to dump request: %v", err)
		}
		verboseOutput = "> " + strings.ReplaceAll(string(dump), "\n", "\n> ")
	}

	resp, err := b.Client.Do(rq)
	if err != nil {
		c.Fatalf("Error making request: %v", err)
	}
	defer resp.Body.Close()

	if c.Verbose {
		// TODO: Test for textual content-type header to set body true or false.
		dump, err := httputil.DumpResponse(resp, true)
		if err != nil {
			c.GetTest().Logf("unable to dump response: %v", err)
		}
		verboseOutput += "\n\n< " + strings.ReplaceAll(string(dump), "\n", "\n< ")
		fmt.Printf("%s\n", verboseOutput)
	}

	status := resp.StatusCode
	if status != c.Status {
		c.Errorf("Expecting status %d, got %d", c.Status, status)
	}

	// TODO: This could consume a lot of memory, but for now this is what
	// we want for being able to refer back to prior tests.
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		c.Fatalf("Error reading response body: %v", err)
	}
	seekerBody := bytes.NewReader(respBody)
	c.SetResponseBody(seekerBody)

	c.SetResponseHeader(resp.Header)

	// TODO: check all response handlers

	rh := []ResponseHandler{
		&StringResponseHandler{},
		&JSONPathResponseHandler{},
		&HeaderResponseHandler{},
	}

	// TODO: This returns, which we don't want, we want to continue, which means
	// we need to pass the testing harness around more.
	for _, handler := range rh {
		// Wind body to start in case it is not there.
		_, err = c.GetResponseBody().Seek(0, io.SeekStart)
		if err != nil {
			c.Fatalf("Unable to seek response body to start: %v", err)
		}

		handler := handler
		handler.Assert(c)
	}

	if c.Xfail && !c.GetXFailure() {
		c.SetDone()
		c.GetTest().Fatalf("Test passed when expecting failure.")
	}
}

func (b *BaseClient) ExecuteOne(c *Case) {
	if c.Skip != nil && *c.Skip != "" {
		c.GetTest().Skipf("<%s> skipping: %s", c.Name, *c.Skip)
	}
	b.Do(c)
}
