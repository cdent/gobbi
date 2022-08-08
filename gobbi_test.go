package gobbi

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

const (
	YAMLFile1       = "testdata/suite1.yaml"
	YAMLFile2       = "testdata/methods.yaml"
	defaultBaseYAML = "testdata/base.yaml"
)

var (
	acceptableMethods = []string{
		http.MethodGet,
		http.MethodPost,
		http.MethodPut,
		http.MethodPatch,
		http.MethodDelete,
		http.MethodHead,
		http.MethodOptions,
	}
	acceptableMethodsMap = map[string]struct{}{}
)

func init() {
	for i := range acceptableMethods {
		acceptableMethodsMap[acceptableMethods[i]] = struct{}{}
	}
}

func GobbiHandler(t *testing.T) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if r := recover(); r != nil {
				panic(http.ErrAbortHandler)
			}
		}()
		method := r.Method
		// Ignore errors when parsing form
		r.ParseForm()
		if r.TLS == nil {
			r.URL.Scheme = "http"
		} else {
			r.URL.Scheme = "https"
		}
		r.URL.Host = r.Host
		urlValues := r.Form
		//pathInfo := r.RequestURI
		accept := r.Header.Get("accept")
		contentType := r.Header.Get("content-type")
		fullRequest := r.URL

		// In gabbi this raised an exception and we want to be able to
		// see/confirm that. So here we panic.
		if method == "DIE" {
			panic("test server handler asked to panic")
		}

		if accept != "" {
			w.Header().Set("content-type", accept)
		} else {
			// overly complex content-type
			w.Header().Set("content-type", "application/json; charset=utf-8; stop=no")
		}
		w.Header().Set("x-gabbi-method", method)
		w.Header().Set("x-gabbi-url", fullRequest.String())

		if _, ok := acceptableMethodsMap[method]; !ok {
			w.Header().Set("allow", strings.Join(acceptableMethods, ", "))
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		data, _ := io.ReadAll(r.Body)

		if strings.HasPrefix(method, "P") {
			w.Header().Set("location", fullRequest.String())
			if contentType == "" && len(data) != 0 {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
		}

		if strings.HasPrefix(contentType, "application/json") {
			var err error
			var x interface{}
			err = json.Unmarshal(data, &x)
			if err != nil {
				t.Logf("unable to decode request body in test server: %v", err)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			encoder := json.NewEncoder(w)
			if mappedX, ok := x.(map[string]interface{}); ok {
				for k, v := range urlValues {
					mappedX[k] = v
				}
				err = encoder.Encode(mappedX)
			} else {
				err = encoder.Encode(x)
			}
			if err != nil {
				t.Logf("unable to encode response body in test server: %v", err)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
		} else if len(urlValues) > 0 {
			encoder := json.NewEncoder(w)
			err := encoder.Encode(urlValues)
			if err != nil {
				t.Logf("unable to encode response body in test server: %v", err)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
		}
	})
}

func TestSimplestRequest(t *testing.T) {
	gc := Case{
		Name:   "simple",
		URL:    "https://burningchrome.com/",
		Method: "GET",
		Status: http.StatusOK,
		test:   t,
	}
	client := NewClient()

	client.ExecuteOne(&gc)
}

func TestSimpleSuite(t *testing.T) {
	gcs := Suite{
		Name: "suite",
		Cases: []*Case{
			&Case{
				Name:   "simple1",
				URL:    "https://burningchrome.com/",
				Method: "GET",
				Status: http.StatusOK,
				test:   t,
			},
			&Case{
				Name:   "simple2",
				URL:    "https://burningchrome.com/bang",
				Method: "GET",
				Status: http.StatusNotFound,
				test:   t,
			},
		},
	}
	gcs.Client = NewClient()
	gcs.Execute(t)
}

func TestFromYaml(t *testing.T) {
	gcs, err := NewSuiteFromYAMLFile(t, "", YAMLFile1)
	if err != nil {
		t.Fatalf("unable to create suite from yaml: %v", err)
	}
	gcs.Execute(t)
}

func TestMethodsFromYaml(t *testing.T) {
	gcs, err := NewSuiteFromYAMLFile(t, "", YAMLFile2)
	if err != nil {
		t.Fatalf("unable to create suite from yaml: %v", err)
	}
	gcs.Execute(t)
}

func TestMultiSuite(t *testing.T) {
	multi, err := NewMultiSuiteFromYAMLFiles(t, "", YAMLFile1, YAMLFile2)
	if err != nil {
		t.Fatalf("unable to create suites from yamls: %v", err)
	}
	multi.Execute(t)
}

func TestMultiWithBase(t *testing.T) {
	ts := httptest.NewServer(GobbiHandler(t))
	t.Cleanup(func() { ts.Close() })
	multi, err := NewMultiSuiteFromYAMLFiles(t, ts.URL, defaultBaseYAML)
	if err != nil {
		t.Fatalf("unable to create suites from yamls: %v", err)
	}
	multi.Execute(t)
}

// TestAllYAMLWithBase tests every yaml file in the testdata directory.
func TestAllYAMLWithBase(t *testing.T) {
	ts := httptest.NewServer(GobbiHandler(t))
	t.Cleanup(func() { ts.Close() })
	files, err := os.ReadDir("testdata")
	if err != nil {
		t.Fatal(err)
	}
	names := []string{}
	for _, f := range files {
		if !strings.HasSuffix(f.Name(), ".yaml") {
			continue
		}
		names = append(names, "testdata/"+f.Name())
	}

	os.Setenv("GABBI_TEST_URL", "takingnames")

	multi, err := NewMultiSuiteFromYAMLFiles(t, ts.URL, names...)
	if err != nil {
		t.Fatalf("unable to create suites from yamls: %v", err)
	}
	multi.Execute(t)
}

func TestResponseRegexpDoubleQuote(t *testing.T) {
	matches := responseRegexp.FindAllStringSubmatch(`$RESPONSE["$.foo.bar"]`, -1)
	argIndex := responseRegexp.SubexpIndex("argD")
	if matches[0][argIndex] != "$.foo.bar" {
		t.Errorf("unable to match, saw matches %v", matches)
	}
}

func TestResponseRegexpSingleQuote(t *testing.T) {
	matches := responseRegexp.FindAllStringSubmatch(`$RESPONSE['$.foo.bar']`, -1)
	argIndex := responseRegexp.SubexpIndex("argS")
	if matches[0][argIndex] != "$.foo.bar" {
		t.Errorf("unable to match, saw matches %v", matches)
	}
}
