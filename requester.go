package gobbi

import (
	"bytes"
	"io"
	"net/http"
	"strings"
	"testing"
)

const (
	// TODO: change this
	DefaultHTTPTimeout = 30
)

type Requester interface {
	Do(*Case)
	ExecuteOne(*testing.T, *Case)
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
	if c.Done() {
		return
	} else if c.UsePriorTest != nil && *c.UsePriorTest {
		prior := c.GetPrior("")
		if prior != nil {
			b.Do(prior)
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

	resp, err := b.Client.Do(rq)
	if err != nil {
		c.Fatalf("Error making request: %v", err)
	}
	defer resp.Body.Close()

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

	c.SetDone()

	if c.Xfail && c.GetTest().Failed() {
		c.GetTest().Skipf("Test failed as expected. Skipping counting.")
	}
}

func (b *BaseClient) ExecuteOne(t *testing.T, c *Case) {
	if c.Skip != nil && *c.Skip != "" {
		t.Skipf("<%s> skipping: %s", c.Name, *c.Skip)
	}
	b.Do(c)
}
