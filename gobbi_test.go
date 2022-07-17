package gobbi

import (
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"
)

const (
	YAMLFile1       = "testdata/suite1.yaml"
	YAMLFile2       = "testdata/methods.yaml"
	defaultBaseYAML = "testdata/base.yaml"
)

func GobbiHandler() http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method := r.Method
		// Ignore errors when parsing form
		r.ParseForm()
		//urlValues := r.Form
		//pathInfo := r.RequestURI
		accept := r.Header.Get("accept")
		//contentType := r.Header.Get("content-type")
		fullRequest := r.URL

		if accept != "" {
			w.Header().Set("content-type", accept)
		} else {
			// overly complex content-type
			w.Header().Set("content-type", "application/json; charset=utf-8; stop=no")
		}
		w.Header().Set("x-gabbi-method", method)
		w.Header().Set("x-gabbi-url", fullRequest.String())
		w.WriteHeader(http.StatusOK)

		w.Write([]byte("yeah"))
	})
}

func TestSimplestRequest(t *testing.T) {
	gc := Case{
		Name:   "simple",
		URL:    "https://burningchrome.com/",
		Method: "GET",
		Status: http.StatusOK,
	}
	client := NewClient()

	client.ExecuteOne(t, &gc)
}

func TestSimpleSuite(t *testing.T) {
	gcs := Suite{
		Name: "suite",
		Cases: []Case{
			{
				Name:   "simple1",
				URL:    "https://burningchrome.com/",
				Method: "GET",
				Status: http.StatusOK,
			},
			{
				Name:   "simple2",
				URL:    "https://burningchrome.com/bang",
				Method: "GET",
				Status: http.StatusNotFound,
			},
		},
	}
	gcs.Client = NewClient()
	gcs.Execute(t)
}

func TestFromYaml(t *testing.T) {
	gcs, err := NewSuiteFromYAMLFile("", YAMLFile1)
	if err != nil {
		t.Fatalf("unable to create suite from yaml: %v", err)
	}
	gcs.Execute(t)
}

func TestMethodsFromYaml(t *testing.T) {
	gcs, err := NewSuiteFromYAMLFile("", YAMLFile2)
	if err != nil {
		t.Fatalf("unable to create suite from yaml: %v", err)
	}
	gcs.Execute(t)
}

func TestMultiSuite(t *testing.T) {
	multi, err := NewMultiSuiteFromYAMLFiles("", YAMLFile1, YAMLFile2)
	if err != nil {
		t.Fatalf("unable to create suites from yamls: %v", err)
	}
	multi.Execute(t)
}

func TestMultiWithBase(t *testing.T) {
	ts := httptest.NewServer(GobbiHandler())
	t.Cleanup(func() { ts.Close() })
	multi, err := NewMultiSuiteFromYAMLFiles(ts.URL, defaultBaseYAML)
	if err != nil {
		t.Fatalf("unable to create suites from yamls: %v", err)
	}
	multi.Execute(t)
}

// TestAllYAMLWithBase tests every yaml file in the testdata directory.
func TestAllYAMLWithBase(t *testing.T) {
	ts := httptest.NewServer(GobbiHandler())
	t.Cleanup(func() { ts.Close() })
	files, err := ioutil.ReadDir("testdata")
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, len(files))
	for i, f := range files {
		names[i] = "testdata/" + f.Name()
	}

	multi, err := NewMultiSuiteFromYAMLFiles(ts.URL, names...)
	if err != nil {
		t.Fatalf("unable to create suites from yamls: %v", err)
	}
	multi.Execute(t)
}
