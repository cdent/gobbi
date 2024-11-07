package gobbi_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/cdent/gobbi"
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

		// Ignore but log errors when parsing form
		if err := r.ParseForm(); err != nil {
			t.Logf("error when ParseForm: %v", err)
		}

		if r.TLS == nil {
			r.URL.Scheme = "http"
		} else {
			r.URL.Scheme = "https"
		}

		r.URL.Host = r.Host
		accept := r.Header.Get("accept")
		contentType := r.Header.Get("content-type")

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
		w.Header().Set("x-gabbi-url", r.URL.String())
		// For header-key tests
		w.Header().Set("http", r.Header.Get("http"))

		if _, ok := acceptableMethodsMap[method]; !ok {
			w.Header().Set("allow", strings.Join(acceptableMethods, ", "))
			w.WriteHeader(http.StatusMethodNotAllowed)

			return
		}

		data, _ := io.ReadAll(r.Body)

		// Set location header when using POST or PUT.
		if strings.HasPrefix(method, "P") {
			w.Header().Set("location", r.URL.String())

			if contentType == "" && len(data) != 0 {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
		}

		switch {
		case strings.HasPrefix(r.RequestURI, "/jsonator"):
			jsonator(t, w, r.Form)
		case strings.HasPrefix(contentType, "application/json"):
			mirrorBody(t, w, data, r.Form)
		case len(r.Form) > 0:
			mirrorQuery(t, w, r.Form)
		default:
			// TODO: turn this off eventually as it will be too noisy, but is
			// useful while developing gobbi itself.
			t.Logf("unhandled situation in GobbiHandler: %s %s", method, r.RequestURI)
		}
	})
}

func jsonator(t *testing.T, w http.ResponseWriter, urlValues url.Values) {
	x := map[string]interface{}{}
	x[urlValues["key"][0]] = urlValues["value"][0]
	encoder := json.NewEncoder(w)

	err := encoder.Encode(x)
	if err != nil {
		t.Logf("unable to encode response body in test server: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
	}
}

func mirrorBody(t *testing.T, w http.ResponseWriter, data []byte, urlValues url.Values) {
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
}

func mirrorQuery(t *testing.T, w http.ResponseWriter, urlValues url.Values) {
	encoder := json.NewEncoder(w)

	err := encoder.Encode(urlValues)
	if err != nil {
		t.Logf("unable to encode response body in test server: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
	}
}

func TestSimplestRequest(t *testing.T) {
	t.Parallel()
	gc := gobbi.Case{
		Name:   "simple",
		URL:    "https://burningchrome.com/",
		Method: "GET",
		Status: http.StatusOK,
		Test:   t,
	}
	client := gobbi.NewClient()

	client.ExecuteOne(context.TODO(), &gc)
}

//nolint:tparallel // We want the top to be parallel, but not within the suite.
func TestSimpleSuite(t *testing.T) {
	t.Parallel()
	gcs := gobbi.Suite{
		Name: "suite",
		Cases: []*gobbi.Case{
			{
				Name:   "simple1",
				URL:    "https://burningchrome.com/",
				Method: "GET",
				Status: http.StatusOK,
				Test:   t,
			},
			{
				Name:   "simple2",
				URL:    "https://burningchrome.com/bang",
				Method: "GET",
				Status: http.StatusNotFound,
				Test:   t,
			},
		},
	}
	gcs.Client = gobbi.NewClient()
	gcs.Execute(context.TODO(), t)
}

//nolint:tparallel // We want the top to be parallel, but not within the suite.
func TestFromYaml(t *testing.T) {
	t.Parallel()

	gcs, err := gobbi.NewSuiteFromYAMLFile(t, "", YAMLFile1)
	if err != nil {
		t.Fatalf("unable to create suite from yaml: %v", err)
	}

	gcs.Execute(context.TODO(), t)
}

//nolint:tparallel // We want the top to be parallel, but not within the suite.
func TestMethodsFromYaml(t *testing.T) {
	t.Parallel()

	gcs, err := gobbi.NewSuiteFromYAMLFile(t, "", YAMLFile2)
	if err != nil {
		t.Fatalf("unable to create suite from yaml: %v", err)
	}

	gcs.Execute(context.TODO(), t)
}

func TestMultiSuite(t *testing.T) {
	t.Parallel()

	multi, err := gobbi.NewMultiSuiteFromYAMLFiles(t, "", YAMLFile1, YAMLFile2)
	if err != nil {
		t.Fatalf("unable to create suites from yamls: %v", err)
	}

	multi.Execute(context.TODO(), t)
}

func TestMultiWithBase(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(GobbiHandler(t))
	t.Cleanup(func() { ts.Close() })

	multi, err := gobbi.NewMultiSuiteFromYAMLFiles(t, ts.URL, defaultBaseYAML)
	if err != nil {
		t.Fatalf("unable to create suites from yamls: %v", err)
	}

	multi.Execute(context.TODO(), t)
}

// TestAllYAMLWithBase tests every yaml file in the testdata directory.
//
//nolint:tparallel // Because we are setting environment variables.
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

	t.Setenv("GABBI_TEST_URL", "takingnames")
	t.Setenv("ONE", "1")

	multi, err := gobbi.NewMultiSuiteFromYAMLFiles(t, ts.URL, names...)
	if err != nil {
		t.Fatalf("unable to create suites from yamls: %v", err)
	}

	multi.Execute(context.TODO(), t)
}

func TestResponseRegexpDoubleQuote(t *testing.T) {
	t.Parallel()

	matches := gobbi.ResponseRegexp.FindAllStringSubmatch(`$RESPONSE["$.foo.bar"]`, -1)
	argIndex := gobbi.ResponseRegexp.SubexpIndex("argD")

	if matches[0][argIndex] != "$.foo.bar" {
		t.Errorf("unable to match, saw matches %v", matches)
	}
}

func TestResponseRegexpSingleQuote(t *testing.T) {
	t.Parallel()

	matches := gobbi.ResponseRegexp.FindAllStringSubmatch(`$RESPONSE['$.foo.bar']`, -1)
	argIndex := gobbi.ResponseRegexp.SubexpIndex("argS")

	if matches[0][argIndex] != "$.foo.bar" {
		t.Errorf("unable to match, saw matches %v", matches)
	}
}
