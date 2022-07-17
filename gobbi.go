package gobbi

import (
	"io/ioutil"
	"net/http"
	"path"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

type SuiteYAML struct {
	Defaults Case
	Fixtures interface{}
	Tests    []Case
}

type Suite struct {
	Name   string
	Client Requester
	File   string
	Cases  []Case
}

type MultiSuite struct {
	Suites []*Suite
}

func NewMultiSuiteFromYAMLFiles(defaultURLBase string, fileNames ...string) (*MultiSuite, error) {
	multi := MultiSuite{}
	multi.Suites = make([]*Suite, len(fileNames))
	for i, name := range fileNames {
		suite, err := NewSuiteFromYAMLFile(defaultURLBase, name)
		if err != nil {
			return nil, err
		}
		multi.Suites[i] = suite
	}
	return &multi, nil
}

func NewSuiteFromYAMLFile(defaultURLBase, fileName string) (*Suite, error) {
	data, err := ioutil.ReadFile(fileName)
	if err != nil {
		return nil, err
	}
	sy := SuiteYAML{}
	err = yaml.Unmarshal(data, &sy)
	if err != nil {
		return nil, err
	}

	processedCases := make([]Case, len(sy.Tests))
	for i, _ := range sy.Tests {
		yamlTest := sy.Tests[i]
		sc, err := makeCaseFromYAML(defaultURLBase, yamlTest, sy.Defaults)
		if err != nil {
			return nil, err
		}
		processedCases[i] = sc
	}

	name := strings.TrimSuffix(path.Base(fileName), path.Ext(fileName))

	suite := Suite{
		Name:   name,
		Cases:  processedCases,
		Client: NewClient(),
	}
	return &suite, nil
}

func (s *Suite) Execute(t *testing.T) {
	for _, c := range s.Cases {
		t.Run(c.Name, func(t *testing.T) {
			s.Client.ExecuteOne(t, &c)
		})
	}
}

func (m *MultiSuite) Execute(t *testing.T) {
	for _, s := range m.Suites {
		s := s
		t.Run(s.Name, func(t *testing.T) {
			t.Parallel()
			s.Execute(t)
		})
	}
}

// TODO: process for fixtures
func makeCaseFromYAML(defaultURLBase string, src Case, defaults Case) (Case, error) {
	newCase := defaults
	// Set default defaults! (where zero value is insufficient)
	if newCase.Status == 0 {
		newCase.Status = http.StatusOK
	}
	baseCase := src
	srcBytes, err := yaml.Marshal(baseCase)
	if err != nil {
		return newCase, err
	}
	err = yaml.Unmarshal(srcBytes, &newCase)
	if err != nil {
		return newCase, err
	}

	// At this point newCase should now src with any empty values set from
	// defaults, so now set URL and Method if GET etc are set.
	switch {
	case newCase.GET != "":
		newCase.URL = newCase.GET
		newCase.Method = http.MethodGet
	case newCase.POST != "":
		newCase.URL = newCase.POST
		newCase.Method = http.MethodPost
	case newCase.PUT != "":
		newCase.URL = newCase.PUT
		newCase.Method = http.MethodPut
	case newCase.PATCH != "":
		newCase.URL = newCase.PATCH
		newCase.Method = http.MethodPatch
	case newCase.DELETE != "":
		newCase.URL = newCase.DELETE
		newCase.Method = http.MethodDelete
	case newCase.HEAD != "":
		newCase.URL = newCase.HEAD
		newCase.Method = http.MethodHead
	case newCase.OPTIONS != "":
		newCase.URL = newCase.OPTIONS
		newCase.Method = http.MethodOptions
	}

	if !strings.HasPrefix(newCase.URL, "http:") && !strings.HasPrefix(newCase.URL, "https:") {
		newCase.URL = defaultURLBase + newCase.URL
	}

	return newCase, nil
}
