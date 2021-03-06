package gobbi

import (
	"fmt"
	"net/http"
	"os"
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
	Cases  []*Case
}

type MultiSuite struct {
	Suites []*Suite
}

func NewMultiSuiteFromYAMLFiles(t *testing.T, defaultURLBase string, fileNames ...string) (*MultiSuite, error) {
	multi := MultiSuite{}
	multi.Suites = make([]*Suite, len(fileNames))
	for i, name := range fileNames {
		suite, err := NewSuiteFromYAMLFile(t, defaultURLBase, name)
		if err != nil {
			return nil, fmt.Errorf("%w: with file %s", err, name)
		}
		multi.Suites[i] = suite
	}
	return &multi, nil
}

func NewSuiteFromYAMLFile(t *testing.T, defaultURLBase, fileName string) (*Suite, error) {
	data, err := os.Open(fileName)
	defer data.Close()
	if err != nil {
		return nil, err
	}
	sy := SuiteYAML{}
	dec := yaml.NewDecoder(data)
	dec.KnownFields(true)
	err = dec.Decode(&sy)
	if err != nil {
		return nil, err
	}

	defaultBytes, err := yaml.Marshal(sy.Defaults)
	if err != nil {
		return nil, err
	}

	var prior *Case
	processedCases := make([]*Case, len(sy.Tests))
	for i, _ := range sy.Tests {
		yamlTest := sy.Tests[i]
		sc, err := makeCaseFromYAML(t, yamlTest, defaultBytes, prior)
		if err != nil {
			return nil, err
		}
		sc.SetDefaultURLBase(defaultURLBase)
		sc.SetSuiteFileName(fileName)
		prior = sc
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

// Execute a single Suite, in series.
func (s *Suite) Execute(t *testing.T) {
	for _, c := range s.Cases {
		c := c
		t.Run(c.Name, func(u *testing.T) {
			// Reset test reference so nesting works as expected.
			c.SetTest(u, t)
			s.Client.ExecuteOne(c)
		})
	}
}

// Execute a MultiSuite in parallel.
func (m *MultiSuite) Execute(t *testing.T) {
	for _, s := range m.Suites {
		s := s
		t.Run(s.Name, func(u *testing.T) {
			u.Parallel()
			s.Execute(u)
		})
	}
}

// TODO: process for fixtures
func makeCaseFromYAML(t *testing.T, src Case, defaultBytes []byte, prior *Case) (*Case, error) {
	newCase := &Case{}
	err := yaml.Unmarshal(defaultBytes, newCase)
	if err != nil {
		return newCase, err
	}
	// Set default defaults! (where zero value is insufficient)
	if newCase.Status == 0 {
		newCase.Status = http.StatusOK
	}
	if newCase.UsePriorTest == nil {
		newCase.UsePriorTest = ptrBool(true)
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
	newCase.SetPrior(prior)
	newCase.SetTest(t, nil)

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
	case newCase.Method == "":
		newCase.Method = http.MethodGet
	}

	return newCase, nil
}

func ptrBool(b bool) *bool {
	return &b
}
