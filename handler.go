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
	historyRegexpString  = `(?:\$HISTORY\[(?:\\?"(?P<caseD>.+?)\\?"|'(?P<caseS>.+?)')]\.)??`
	responseRegexpString = `\$RESPONSE(:(?P<cast>\w+))?\[(?:\\?"(?P<argD>.+?)\\?"|'(?P<argS>.+?)')\]`
	headersRegexpString  = `\$HEADERS(:(?P<cast>\w+))?\[(?:\\?"(?P<argD>.+?)\\?"|'(?P<argS>.+?)')\]`
	environRegexpString  = `\$ENVIRON(:(?P<cast>\w+))?\[(?:\\?"(?P<argD>.+?)\\?"|'(?P<argS>.+?)')\]`
	locationRegexpString = `\$LOCATION`
	urlRegexpString      = `\$URL`
)

var (
	jsonPathConfig  = jsonpath.Config{}
	responseRegexp  *regexp.Regexp
	locationRegexp  *regexp.Regexp
	headersRegexp   *regexp.Regexp
	environRegexp   *regexp.Regexp
	urlRegexp       *regexp.Regexp
	stringReplacers []StringReplacer
)

func init() {
	jsonPathConfig.SetAggregateFunction(`len`, func(params []interface{}) (interface{}, error) {
		p := deList(params)
		switch x := p.(type) {
		case []interface{}:
			return float64(len(x)), nil
		case string:
			return float64(len(x)), nil
		case map[string]interface{}:
			return float64(len(x)), nil
		default:
			return float64(0), nil
		}
	})
	responseRegexp = regexp.MustCompile(historyRegexpString + responseRegexpString)
	locationRegexp = regexp.MustCompile(historyRegexpString + locationRegexpString)
	headersRegexp = regexp.MustCompile(historyRegexpString + headersRegexpString)
	environRegexp = regexp.MustCompile(environRegexpString)
	urlRegexp = regexp.MustCompile(historyRegexpString + urlRegexpString)
	lr := &LocationReplacer{}
	lr.regExp = locationRegexp
	hr := &HeadersReplacer{}
	hr.regExp = headersRegexp
	er := &EnvironReplacer{}
	er.regExp = environRegexp
	jr := &JSONPathStringReplacer{}
	jr.regExp = responseRegexp
	ur := &URLReplacer{}
	ur.regExp = urlRegexp
	sr := &SchemeReplacer{}
	nr := &NetlocReplacer{}
	lu := &LastURLReplacer{}
	stringReplacers = []StringReplacer{
		sr,
		nr,
		lu,
		ur,
		lr,
		hr,
		er,
		jr,
	}

}

type StringReplacer interface {
	Replace(*Case, string) (string, error)
	Resolve(*Case, string) (string, error)
	GetRegExp() *regexp.Regexp
}

type BaseStringReplacer struct {
	regExp *regexp.Regexp
}

func (br *BaseStringReplacer) Resolve(prior *Case, in string) (string, error) {
	return in, nil
}

func (br *BaseStringReplacer) GetRegExp() *regexp.Regexp {
	return br.regExp
}

func makeStringReplaceFunc(replacements []string) func(string) string {
	return (func(string) string {
		out := replacements[0]
		replacements = replacements[1:]
		return out
	})
}

type SchemeReplacer struct {
	BaseStringReplacer
}

type NetlocReplacer struct {
	BaseStringReplacer
}

type LastURLReplacer struct {
	BaseStringReplacer
}

type LocationReplacer struct {
	BaseStringReplacer
}

type HeadersReplacer struct {
	BaseStringReplacer
}

type EnvironReplacer struct {
	BaseStringReplacer
}

type JSONPathStringReplacer struct {
	BaseStringReplacer
}

type URLReplacer struct {
	BaseStringReplacer
}

func baseReplace(rpl StringReplacer, c *Case, in string) (string, error) {
	regExp := rpl.GetRegExp()
	matches := regExp.FindAllStringSubmatch(in, -1)
	if len(matches) == 0 {
		return in, nil
	}
	replacements := make([]string, len(matches))

	caseDIndex := regExp.SubexpIndex("caseD")
	caseSIndex := regExp.SubexpIndex("caseS")
	argDIndex := regExp.SubexpIndex("argD")
	argSIndex := regExp.SubexpIndex("argS")

	for i := range matches {
		var prior *Case
		var argValue string
		if caseDIndex >= 0 && caseSIndex >= 0 {
			caseName := matches[i][caseDIndex]
			if len(caseName) == 0 {
				caseName = matches[i][caseSIndex]
			}
			prior = c.GetPrior(caseName)
			if prior == nil {
				return "", ErrNoPriorTest
			}
		}
		if argDIndex >= 0 && argSIndex >= 0 {
			argValue = matches[i][argDIndex]
			if len(argValue) == 0 {
				argValue = matches[i][argSIndex]
			}
		}
		rValue, err := rpl.Resolve(prior, argValue)
		if err != nil {
			return "", err
		}
		replacements[i] = rValue
	}

	replacer := makeStringReplaceFunc(replacements)
	in = rpl.GetRegExp().ReplaceAllStringFunc(in, replacer)
	return in, nil
}

func (s *SchemeReplacer) Replace(c *Case, in string) (string, error) {
	return strings.ReplaceAll(in, "$SCHEME", c.ParsedURL().Scheme), nil
}

func (n *NetlocReplacer) Replace(c *Case, in string) (string, error) {
	return strings.ReplaceAll(in, "$NETLOC", c.ParsedURL().Host), nil
}

func (n *LastURLReplacer) Replace(c *Case, in string) (string, error) {
	prior := c.GetPrior("")
	if prior == nil {
		return in, nil
	}
	return strings.ReplaceAll(in, "$LAST_URL", prior.URL), nil
}

func (l *LocationReplacer) Resolve(prior *Case, argValue string) (string, error) {
	return prior.GetResponseHeader().Get("location"), nil
}

func (l *LocationReplacer) Replace(c *Case, in string) (string, error) {
	return baseReplace(l, c, in)
}

func (u *URLReplacer) Resolve(prior *Case, argValue string) (string, error) {
	return prior.URL, nil
}

func (u *URLReplacer) Replace(c *Case, in string) (string, error) {
	return baseReplace(u, c, in)
}

func (e *EnvironReplacer) Resolve(prior *Case, argValue string) (string, error) {
	if value, ok := os.LookupEnv(argValue); !ok {
		return "", fmt.Errorf("%w: %s", ErrEnvironmentVariableNotFound, argValue)
	} else {
		return value, nil
	}
}

func (e *EnvironReplacer) Replace(c *Case, in string) (string, error) {
	return baseReplace(e, c, in)
}

func (h *HeadersReplacer) Resolve(prior *Case, argValue string) (string, error) {
	return prior.GetResponseHeader().Get(argValue), nil
}

func (h *HeadersReplacer) Replace(c *Case, in string) (string, error) {
	return baseReplace(h, c, in)
}

func (j *JSONPathStringReplacer) Resolve(prior *Case, argValue string) (string, error) {
	jpr := &JSONPathResponseHandler{}
	_, err := prior.GetResponseBody().Seek(0, io.SeekStart)
	if err != nil {
		return "", err
	}
	rawJSON, err := jpr.ReadJSONReponse(prior)
	if err != nil {
		return "", err
	}
	o, err := jsonpath.Retrieve(string(argValue), rawJSON, jsonPathConfig)
	if err != nil {
		return "", err
	}
	output := deList(o)
	switch x := output.(type) {
	case string:
		return x, nil
	default:
		resp, err := json.Marshal(output)
		return string(resp), err
	}
}

func (j *JSONPathStringReplacer) Replace(c *Case, in string) (string, error) {
	return baseReplace(j, c, in)
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
		stringData, err := StringReplace(c, stringData)
		if err != nil {
			return nil, err
		}
		return strings.NewReader(stringData), nil
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

	// TODO: this was moved locally to avoid conflicts, but now needs to be
	// incorporated into interface handling.
	caseDIndex := responseRegexp.SubexpIndex("caseD")
	caseSIndex := responseRegexp.SubexpIndex("caseS")
	argDIndex := responseRegexp.SubexpIndex("argD")
	argSIndex := responseRegexp.SubexpIndex("argS")

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
	Accepts(*Case) bool
	Assert(*Case)
}

type BaseResponseHandler struct{}

func (b *BaseResponseHandler) Accepts(c *Case) bool {
	return true
}

type HeaderResponseHandler struct {
	BaseResponseHandler
}

func (h *HeaderResponseHandler) Assert(c *Case) {
	if len(c.ResponseHeaders) == 0 {
		return
	}

	if !h.Accepts(c) {
		return
	}

	headers := c.GetResponseHeader()

	for k, v := range c.ResponseHeaders {
		var headerName string
		var headerValue string
		var err error
		headerName, err = StringReplace(c, k)
		if err != nil {
			c.Errorf("unable to replace response header name: %s, %v", k, err)
			headerName = k
		}

		hv := headers.Get(headerName)
		if hv == "" {
			c.Errorf("Expected header %s not present", headerName)
			continue
		}
		headerValue, err = StringReplace(c, v)
		if err != nil {
			c.Errorf("unable to replace response header value: %s, %v", v, err)
			headerValue = v
		}
		if hv != headerValue {
			c.Errorf("For header %s expecting value %s, got %s", headerName, headerValue, hv)
		}
	}
}

func (h *HeaderResponseHandler) Replacer(c *Case, v string) string {
	result, _ := StringReplace(c, v)
	// TODO: ignoring errors for now
	return result
}

type StringResponseHandler struct {
	BaseResponseHandler
}

func (s *StringResponseHandler) Assert(c *Case) {
	if len(c.ResponseStrings) == 0 {
		return
	}

	if !s.Accepts(c) {
		return
	}

	rawBytes, err := io.ReadAll(c.GetResponseBody())
	if err != nil {
		c.Fatalf("Unable to read response body for strings: %v", err)
	}
	stringBody := string(rawBytes)
	bodyLength := len(stringBody)
	limit := bodyLength
	if limit > 200 {
		limit = 200
	}
	for _, check := range c.ResponseStrings {
		check, err := StringReplace(c, check)
		if err != nil {
			c.Errorf("unable to process response string check: %s", check)
		}
		if !strings.Contains(stringBody, check) {
			c.Errorf("<%s> not in body: %s", check, stringBody[:limit])
		}
	}
}

type JSONPathResponseHandler struct {
	BaseResponseHandler
}

func deList(i any) any {
	switch x := i.(type) {
	case []interface{}:
		if len(x) == 1 {
			return x[0]
		}
	}
	return i
}

func (*JSONPathResponseHandler) Accepts(c *Case) bool {
	contentType := strings.TrimSpace(strings.Split(c.GetResponseHeader().Get("content-type"), ";")[0])
	if !strings.HasPrefix(contentType, "application/json") && !strings.HasSuffix(contentType, "+json") {
		c.Errorf("response is not JSON, must be to process JSON Path")
		return false
	}
	return true
}

func (j *JSONPathResponseHandler) Assert(c *Case) {
	if len(c.ResponseJSONPaths) == 0 {
		return
	}

	if !j.Accepts(c) {
		return
	}

	rawJSON, err := j.ReadJSONReponse(c)
	if err != nil {
		c.Fatalf("Unable to read JSON from body: %v", err)
	}

	// Dump ResponseJSONPaths to JSON, make it a string, do StringReplace,
	// assign it back.
	pathData, err := json.Marshal(c.ResponseJSONPaths)
	if err != nil {
		c.Fatalf("Unable to process JSON Paths: %v", err)
	}
	processedData, err := StringReplace(c, string(pathData))
	if err != nil {
		c.Fatalf("Unable to string replace JSON Paths: %v", err)
	}
	err = json.Unmarshal([]byte(processedData), &c.ResponseJSONPaths)
	if err != nil {
		c.Fatalf("Unable to unmarshal JSON Paths: %v", err)
	}

	for path, v := range c.ResponseJSONPaths {
		err := j.ProcessOnePath(c, rawJSON, path, v)
		if err != nil {
			c.Errorf("%v", err)
		}
	}
}

func (j *JSONPathResponseHandler) ReadJSONReponse(c *Case) (interface{}, error) {
	var rawJSON interface{}
	_, err := c.GetResponseBody().Seek(0, io.SeekStart)
	if err != nil {
		return rawJSON, err
	}
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
	if err != nil {
		return err
	}
	output := deList(o)
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
