package gobbi

import (
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
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
	responseRegexp   *regexp.Regexp
	locationRegexp   *regexp.Regexp
	headersRegexp    *regexp.Regexp
	environRegexp    *regexp.Regexp
	urlRegexp        *regexp.Regexp
	stringReplacers  []StringReplacer
	responseHandlers []ResponseHandler
	requestHandlers  map[string]RequestDataHandler
)

func init() {
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
	jr := &JSONHandler{}
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
	responseHandlers = []ResponseHandler{
		&StringResponseHandler{},
		jr,
		&HeaderResponseHandler{},
	}
	requestHandlers = map[string]RequestDataHandler{
		"text":   &TextDataHandler{},
		"json":   jr,
		"nil":    &NilDataHandler{},
		"binary": &BinaryDataHandler{},
	}
}

type StringReplacer interface {
	Replace(*Case, string) (string, error)
	Resolve(*Case, string, string) (string, error)
	GetRegExp() *regexp.Regexp
}

type BaseStringReplacer struct {
	regExp *regexp.Regexp
}

func (br *BaseStringReplacer) Resolve(prior *Case, in, cast string) (string, error) {
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
	castIndex := regExp.SubexpIndex("cast")

	for i := range matches {
		var prior *Case
		var argValue string
		var cast string
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
		if castIndex >= 0 {
			cast = matches[i][castIndex]
		}
		rValue, err := rpl.Resolve(prior, argValue, cast)
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

func (l *LocationReplacer) Resolve(prior *Case, argValue, cast string) (string, error) {
	return prior.GetResponseHeader().Get("location"), nil
}

func (l *LocationReplacer) Replace(c *Case, in string) (string, error) {
	return baseReplace(l, c, in)
}

func (u *URLReplacer) Resolve(prior *Case, argValue, cast string) (string, error) {
	return prior.URL, nil
}

func (u *URLReplacer) Replace(c *Case, in string) (string, error) {
	return baseReplace(u, c, in)
}

func (e *EnvironReplacer) Resolve(prior *Case, argValue, cast string) (string, error) {
	if value, ok := os.LookupEnv(argValue); !ok {
		return "", fmt.Errorf("%w: %s", ErrEnvironmentVariableNotFound, argValue)
	} else {
		return value, nil
	}
}

func (e *EnvironReplacer) Replace(c *Case, in string) (string, error) {
	return baseReplace(e, c, in)
}

func (h *HeadersReplacer) Resolve(prior *Case, argValue, cast string) (string, error) {
	return prior.GetResponseHeader().Get(argValue), nil
}

func (h *HeadersReplacer) Replace(c *Case, in string) (string, error) {
	return baseReplace(h, c, in)
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

type NilDataHandler struct{}
type TextDataHandler struct{}
type BinaryDataHandler struct{}

func (n *NilDataHandler) GetBody(c *Case) (io.Reader, error) {
	return nil, nil
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
