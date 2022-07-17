package gobbi

import (
	"net/http"
	"testing"
)

const YAMLFile1 = "testdata/suite1.yaml"
const YAMLFile2 = "testdata/methods.yaml"

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
		Name: ptrStr("suite"),
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
	gcs, err := NewSuiteFromYAMLFile(YAMLFile1)
	if err != nil {
		t.Fatalf("unable to create suite from yaml: %v", err)
	}
	gcs.Execute(t)
}

func TestMethodsFromYaml(t *testing.T) {
	gcs, err := NewSuiteFromYAMLFile(YAMLFile2)
	if err != nil {
		t.Fatalf("unable to create suite from yaml: %v", err)
	}
	gcs.Execute(t)
}
