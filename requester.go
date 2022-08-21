package gobbi

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
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

func (b *BaseClient) updateQueryString(c *Case, u string) (string, error) {
	additionalValues := c.QueryParameters
	if len(additionalValues) == 0 {
		// No changes required, return early
		return u, nil
	}
	parsedURL, err := url.Parse(u)
	if err != nil {
		return u, err
	}
	currentValues := parsedURL.Query()
	for k, v := range additionalValues {
		switch x := v.(type) {
		case []interface{}:
			s := make([]string, len(x))
			for i, item := range x {
				s[i] = scalarToString(item)
			}
			currentValues[k] = s
		default:
			currentValues[k] = []string{scalarToString(x)}
		}
	}
	for k, vList := range currentValues {
		for i, v := range vList {
			newV, err := StringReplace(c, v)
			if err != nil {
				c.Errorf("unable to string replace query parameter %s: %v", k, err)
				continue
			}
			currentValues[k][i] = newV
		}
	}

	parsedURL.RawQuery = currentValues.Encode()
	return parsedURL.String(), nil
}

func scalarToString(v any) string {
	var sValue string
	switch x := v.(type) {
	case string:
		sValue = x
	case int:
		sValue = strconv.Itoa(x)
	case float64:
		sValue = strconv.FormatFloat(x, 'G', -1, 64)
	}
	return sValue
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
			c.GetTest().Run(prior.Name, func(u *testing.T) {
				prior.SetTest(u, c.GetTest())
				b.ExecuteOne(prior)
			})
		}
	}

	// Do URL replacements
	url, err := StringReplace(c, c.URL)
	if err != nil {
		c.Errorf("StringReplace failed: %v", err)
	}
	updatedURL, err := b.updateQueryString(c, url)
	if err != nil {
		c.Errorf("error updating query string: %v", err)
	}
	c.URL = updatedURL

	if !strings.HasPrefix(c.URL, "http:") && !strings.HasPrefix(c.URL, "https:") {
		c.URL = c.GetDefaultURLBase() + c.URL
	}

	c.GetTest().Logf("url for %s is %s", c.Name, c.URL)

	body, err := c.GetRequestBody()
	if err != nil {
		c.Fatalf("Error while getting request body: %v", err)
	}
	// TODO: NewRequestWithContext
	rq, err := http.NewRequest(c.Method, c.URL, body)
	if err != nil {
		c.Fatalf("Error creating request: %v", err)
	}

	// Update request headers
	updatedHeaders := map[string]string{}
	for k, v := range c.RequestHeaders {
		newK, err := StringReplace(c, k)
		if err != nil {
			c.Errorf("StringReplace for header %s failed: %v", k, err)
			updatedHeaders[k] = v
			continue
		}
		newV, err := StringReplace(c, v)
		if err != nil {
			c.Errorf("StringReplace for header value %s failed: %v", v, err)
			updatedHeaders[newK] = v
			continue
		}
		rq.Header.Set(newK, newV)
		updatedHeaders[newK] = newV
	}
	c.RequestHeaders = updatedHeaders

	if c.Verbose {
		// TODO: Test for textual content-type header to set body true or false.
		dump, err := httputil.DumpRequestOut(rq, true)
		if err != nil {
			c.GetTest().Logf("unable to dump request: %v", err)
		}
		fmt.Printf("%s\n", strings.ReplaceAll(string(dump), "\n", "\n> "))
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
		fmt.Printf("\n\n< %s", strings.ReplaceAll(string(dump), "\n", "\n< "))
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

	// TODO: This returns, which we don't want, we want to continue, which means
	// we need to pass the testing harness around more.
	for _, handler := range responseHandlers {
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
