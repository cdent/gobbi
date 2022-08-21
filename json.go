package gobbi

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/AsaiYusuke/jsonpath"
	"github.com/google/go-cmp/cmp"
)

var (
	jsonPathConfig = jsonpath.Config{}
)

func init() {
	// TODO: Do processing to translate json patch functions from gabbi style
	// to gobbi style, on the fly!
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
}

type JSONHandler struct {
	BaseStringReplacer
	BaseResponseHandler
}

func (j *JSONHandler) Resolve(prior *Case, argValue, cast string) (string, error) {
	jpr := &JSONHandler{}
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

func (j *JSONHandler) Replace(c *Case, in string) (string, error) {
	return baseReplace(j, c, in)
}

// ReadJSONFromDisk, selecting a json path from it, if there is a : in the filename.
func (j *JSONHandler) ReadJSONFromDisk(c *Case, stringData string) (string, error) {
	dataPath := stringData[strings.LastIndex(stringData, ":")+1:]
	if stringData != dataPath {
		stringData = strings.Replace(stringData, ":"+dataPath, "", 1)
	}
	fh, err := c.ReadFileForData(stringData)
	if err != nil {
		return "", err
	}
	rawBytes, err := io.ReadAll(fh)
	if err != nil {
		return "", err
	}
	if stringData != dataPath {
		var v interface{}
		err = json.Unmarshal(rawBytes, &v)
		if err != nil {
			return "", err
		}
		found, err := jsonpath.Retrieve(dataPath, v, jsonPathConfig)
		if err != nil {
			return "", err
		}
		v = deList(found)
		out, err := json.Marshal(v)
		if err != nil {
			return "", err
		}
		return string(out), nil
	} else {
		return string(rawBytes), nil
	}

}

func (j *JSONHandler) GetBody(c *Case) (io.Reader, error) {
	if stringData, ok := c.Data.(string); ok {
		if strings.HasPrefix(stringData, fileForDataPrefix) {
			result, err := j.ReadJSONFromDisk(c, stringData)
			return strings.NewReader(result), err
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
	dataString, err := j.Replace(c, string(data))
	return strings.NewReader(dataString), err
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

func (*JSONHandler) Accepts(c *Case) bool {
	contentType := strings.TrimSpace(strings.Split(c.GetResponseHeader().Get("content-type"), ";")[0])
	if !strings.HasPrefix(contentType, "application/json") && !strings.HasSuffix(contentType, "+json") {
		c.Errorf("response is not JSON, must be to process JSON Path")
		return false
	}
	return true
}

func (j *JSONHandler) Assert(c *Case) {
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

func (j *JSONHandler) ReadJSONReponse(c *Case) (interface{}, error) {
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

func (j *JSONHandler) ProcessOnePath(c *Case, rawJSON interface{}, path string, v interface{}) error {
	if stringData, ok := v.(string); ok {
		if strings.HasPrefix(stringData, fileForDataPrefix) {
			jsonString, err := j.ReadJSONFromDisk(c, stringData)
			if err != nil {
				return err
			}
			c.GetTest().Logf("jsonstring is %v", jsonString)
			err = json.Unmarshal([]byte(jsonString), &v)
			if err != nil {
				return err
			}
		}
	}
	c.GetTest().Logf("path, raw, v: %v, %v, %v", path, rawJSON, v)
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
