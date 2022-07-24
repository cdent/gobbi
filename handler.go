package gobbi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"

	"github.com/AsaiYusuke/jsonpath"
	"github.com/google/go-cmp/cmp"
)

const (
	historyRegexpString  = `(?:\$HISTORY\[(?:"(?P<caseD>.+?)"|'(?P<caseS>.+?)')]\.)??`
	responseRegexpString = `\$RESPONSE(:(?P<cast>\w+))?\[(?:"(?P<argD>.+?)"|'(?P<argS>.+?)')\]`
	headersRegexpString  = `\$HEADERS(:(?P<cast>\w+))?\[(?:"(?P<argD>.+?)"|'(?P<argS>.+?)')\]`
	environRegexpString  = `\$ENVIRON(:(?P<cast>\w+))?\[(?:"(?P<argD>.+?)"|'(?P<argS>.+?)')\]`
	locationRegexpString = `\$LOCATION`
)

var (
	jsonPathConfig  = jsonpath.Config{}
	responseRegexp  *regexp.Regexp
	locationRegexp  *regexp.Regexp
	headersRegexp   *regexp.Regexp
	environRegexp   *regexp.Regexp
	caseDIndex      int
	caseSIndex      int
	argDIndex       int
	argSIndex       int
	stringReplacers []StringReplacer
)

func init() {
	jsonPathConfig.SetAggregateFunction(`len`, func(params []interface{}) (interface{}, error) {
		return float64(len(params)), nil
	})
	responseRegexp = regexp.MustCompile(historyRegexpString + responseRegexpString)
	locationRegexp = regexp.MustCompile(historyRegexpString + locationRegexpString)
	headersRegexp = regexp.MustCompile(historyRegexpString + headersRegexpString)
	// $HISTORY is meaningless for $ENVIRON, but we use it for consistent subexp
	// index.
	environRegexp = regexp.MustCompile(historyRegexpString + environRegexpString)
	caseDIndex = responseRegexp.SubexpIndex("caseD")
	caseSIndex = responseRegexp.SubexpIndex("caseS")
	argDIndex = responseRegexp.SubexpIndex("argD")
	argSIndex = responseRegexp.SubexpIndex("argS")
	stringReplacers = []StringReplacer{
		&LocationReplacer{},
		&HeadersReplacer{},
		&EnvironReplacer{},
	}

}

type StringReplacer interface {
	Replace(c *Case, in string) (string, error)
}

func makeStringReplaceFunc(replacements []string) func(string) string {
	return (func(string) string {
		out := replacements[0]
		replacements = replacements[1:]
		return out
	})
}

type LocationReplacer struct{}
type HeadersReplacer struct{}
type EnvironReplacer struct{}

func (l *LocationReplacer) Replace(c *Case, in string) (string, error) {
	matches := locationRegexp.FindAllStringSubmatch(in, -1)
	if len(matches) == 0 {
		return in, nil
	}
	replacements := make([]string, len(matches))

	for i := range matches {
		caseName := matches[i][caseDIndex]
		if len(caseName) == 0 {
			caseName = matches[i][caseSIndex]
		}
		prior := c.GetPrior(caseName)
		if prior == nil {
			return "", ErrNoPriorTest
		}
		replacements[i] = prior.URL
	}

	replacer := makeStringReplaceFunc(replacements)
	in = locationRegexp.ReplaceAllStringFunc(in, replacer)
	return in, nil
}

func (e *EnvironReplacer) Replace(c *Case, in string) (string, error) {
	matches := environRegexp.FindAllStringSubmatch(in, -1)
	if len(matches) == 0 {
		return in, nil
	}
	replacements := make([]string, len(matches))

	for i := range matches {
		argValue := matches[i][argDIndex]
		if len(argValue) == 0 {
			argValue = matches[i][argSIndex]
		}
		if value, ok := os.LookupEnv(argValue); !ok {
			return "", fmt.Errorf("%w: %s", ErrEnvironmentVariableNotFound, argValue)
		} else {
			replacements[i] = value
		}
	}
	replacer := makeStringReplaceFunc(replacements)
	in = environRegexp.ReplaceAllStringFunc(in, replacer)
	return in, nil
}

func (h *HeadersReplacer) Replace(c *Case, in string) (string, error) {
	matches := headersRegexp.FindAllStringSubmatch(in, -1)
	if len(matches) == 0 {
		return in, nil
	}
	replacements := make([]string, len(matches))

	for i := range matches {
		caseName := matches[i][caseDIndex]
		if len(caseName) == 0 {
			caseName = matches[i][caseSIndex]
		}
		prior := c.GetPrior(caseName)
		if prior == nil {
			return "", ErrNoPriorTest
		}
		argValue := matches[i][argDIndex]
		if len(argValue) == 0 {
			argValue = matches[i][argSIndex]
		}
		replacements[i] = prior.GetResponseHeader().Get(argValue)
	}

	replacer := makeStringReplaceFunc(replacements)
	in = headersRegexp.ReplaceAllStringFunc(in, replacer)
	return in, nil
}

func StringReplace(c *Case, in string) (string, error) {
	for _, replacer := range stringReplacers {
		var err error
		in, err = replacer.Replace(c, in)
		if err != nil {
			return in, err
		}
	}
	return in, nil

}

type RequestDataHandler interface {
	GetBody(c *Case) (io.Reader, error)
}

type JSONDataHandler struct{}
type NilDataHandler struct{}
type TextDataHandler struct{}
type BinaryDataHandler struct{}

func (n *NilDataHandler) GetBody(c *Case) (io.Reader, error) {
	return nil, nil
}

func (j *JSONDataHandler) GetBody(c *Case) (io.Reader, error) {
	if stringData, ok := c.Data.(string); ok {
		if strings.HasPrefix(stringData, fileForDataPrefix) {
			return c.ReadFileForData(stringData)
		}
	}
	data, err := json.Marshal(c.Data)
	if err != nil {
		return nil, err
	}
	data = j.Replacer(c, data)
	return bytes.NewReader(data), err
}

func (j *JSONDataHandler) Replacer(c *Case, data []byte) []byte {
	matches := responseRegexp.FindAllSubmatch(data, -1)
	if len(matches) == 0 {
		return data
	}
	replacements := make([][]byte, len(matches))
	// TODO: need a log!

	for i := range matches {
		caseName := matches[i][caseDIndex]
		if len(caseName) == 0 {
			caseName = matches[i][caseSIndex]
		}
		argValue := matches[i][argDIndex]
		if len(argValue) == 0 {
			argValue = matches[i][argSIndex]
		}
		repl, err := j.ResolveReplacer(c, caseName, argValue)
		if err != nil {
			// TODO: something
		}
		replacements[i] = repl
	}

	replacer := func(i []byte) []byte {
		out := replacements[0]
		replacements = replacements[1:]
		return out
	}
	replacedData := responseRegexp.ReplaceAllFunc(data, replacer)
	return replacedData
}

func (j *JSONDataHandler) ResolveReplacer(c *Case, caseName []byte, argvalue []byte) ([]byte, error) {
	var resp []byte
	prior := c.GetPrior(string(caseName))
	if prior == nil {
		return resp, ErrNoPriorTest
	}
	jpr := &JSONPathResponseHandler{}
	rawJSON, err := jpr.ReadJSONReponse(prior)
	if err != nil {
		return resp, err
	}
	o, err := jsonpath.Retrieve(string(argvalue), rawJSON, jsonPathConfig)
	if err != nil {
		return resp, err
	}
	output := deList(o)
	switch x := output.(type) {
	case string:
		// Avoid quoting strings
		return []byte(x), nil
	default:
		resp, err = json.Marshal(output)
		return resp, err
	}
}

func (t *TextDataHandler) GetBody(c *Case) (io.Reader, error) {
	data, ok := c.Data.(string)
	if !ok {
		return nil, ErrDataHandlerContentMismatch
	}
	if strings.HasPrefix(data, fileForDataPrefix) {
		return c.ReadFileForData(data)
	}
	return strings.NewReader(data), nil
}

func (t *BinaryDataHandler) GetBody(c *Case) (io.Reader, error) {
	if stringData, ok := c.Data.(string); ok {
		if strings.HasPrefix(stringData, fileForDataPrefix) {
			return c.ReadFileForData(stringData)
		}
	}
	return nil, ErrDataHandlerContentMismatch
}

type ResponseHandler interface {
	Assert(*Case) error
}

type HeaderResponseHandler struct{}

func (h *HeaderResponseHandler) Assert(c *Case) error {
	if len(c.ResponseHeaders) == 0 {
		return nil
	}

	headers := c.GetResponseHeader()

	for k, v := range c.ResponseHeaders {
		headerValue := headers.Get(k)
		if headerValue == "" {
			return fmt.Errorf("%w: %s", ErrHeaderNotPresent, k)
		}
		if headerValue != h.Replacer(c, v) {
			// TODO: stop using errors, use t.Testing funcs
			return fmt.Errorf("%w: expecting %s, got %s", ErrHeaderValueMismatch, v, headerValue)
		}
	}

	return nil
}

// TODO: Dispatch to generic replacer!
func (h *HeaderResponseHandler) Replacer(c *Case, v string) string {
	v = strings.ReplaceAll(v, "$SCHEME", c.ParsedURL().Scheme)
	v = strings.ReplaceAll(v, "$NETLOC", c.ParsedURL().Host)
	return v
}

type StringResponseHandler struct{}

func (s *StringResponseHandler) Assert(c *Case) error {
	if len(c.ResponseStrings) == 0 {
		return nil
	}

	rawBytes, err := io.ReadAll(c.GetResponseBody())
	if err != nil {
		return err
	}
	stringBody := string(rawBytes)
	bodyLength := len(stringBody)
	limit := bodyLength
	if limit > 200 {
		limit = 200
	}
	for _, check := range c.ResponseStrings {
		if !strings.Contains(stringBody, check) {
			return fmt.Errorf("%w: %s not in body: %s", ErrStringNotFound, check, stringBody[:limit])
		}
	}
	return nil
}

type JSONPathResponseHandler struct{}

func deList(i any) any {
	switch x := i.(type) {
	case []interface{}:
		if len(x) == 1 {
			return x[0]
		}
	}
	return i
}

func (j *JSONPathResponseHandler) Assert(c *Case) error {
	if len(c.ResponseJSONPaths) == 0 {
		return nil
	}
	rawJSON, err := j.ReadJSONReponse(c)
	if err != nil {
		return err
	}
	for path, v := range c.ResponseJSONPaths {
		err := j.ProcessOnePath(c, rawJSON, path, v)
		if err != nil {
			return err
		}
	}
	return nil
}

func (j *JSONPathResponseHandler) ReadJSONReponse(c *Case) (interface{}, error) {
	var rawJSON interface{}
	rawBytes, err := io.ReadAll(c.GetResponseBody())
	if err != nil {
		return rawJSON, err
	}
	err = json.Unmarshal(rawBytes, &rawJSON)
	if err != nil {
		return rawJSON, err
	}
	return rawJSON, nil
}

func (j *JSONPathResponseHandler) ProcessOnePath(c *Case, rawJSON interface{}, path string, v interface{}) error {
	if stringData, ok := v.(string); ok {
		if strings.HasPrefix(stringData, fileForDataPrefix) {
			// Read JSON from disk
			fh, err := c.ReadFileForData(stringData)
			if err != nil {
				return err
			}
			rawBytes, err := io.ReadAll(fh)
			if err != nil {
				return err
			}
			err = json.Unmarshal(rawBytes, &v)
			if err != nil {
				return err
			}
		}
	}
	o, err := jsonpath.Retrieve(path, rawJSON, jsonPathConfig)
	output := deList(o)
	if err != nil {
		return err
	}
	// This switch works around numerals in JSON being weird and that it
	// is proving difficult to get a cmp.Transformer to work as expected.
	switch value := v.(type) {
	case int:
		if !cmp.Equal(float64(value), output) {
			return fmt.Errorf("%w: diff: %s", ErrJSONPathNotMatched, cmp.Diff(float64(value), output))
		}
	default:
		if !cmp.Equal(value, output) {
			return fmt.Errorf("%w: diff: %s", ErrJSONPathNotMatched, cmp.Diff(value, output))
		}
	}
	return nil
}
