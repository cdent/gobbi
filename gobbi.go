package gobbi

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// SuiteYAML describes the top-level structure of a single YAML file.
type SuiteYAML struct {
	// Defaults apply to ever case unless overridden in the case.
	Defaults Case `yaml:"defaults"`
	// Fixtures are run per suite. TODO: Not yet implemented.
	Fixtures interface{} `yaml:"fixtures"`
	// Tests is an ordered collection of test cases.
	Tests []Case `yaml:"tests"`
}

// Suite is the internal representation of a SuiteYAML, including the HTTP
// client that will be used with that Suite.
type Suite struct {
	Name   string
	Client Requester
	File   string
	Cases  []*Case
}

// MultiSuite is a collection of Suites.
type MultiSuite struct {
	Suites []*Suite
}

// NewMultiSuiteFromYAMLFiles is a main entry point to processing a collection
// of YAML files, resulting in a MultiSuite.
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

// NewSuiteFromYAMLFile creates one suite from one YAML file.
func NewSuiteFromYAMLFile(t *testing.T, defaultURLBase, fileName string) (*Suite, error) {
	//nolint:gosec
	data, err := os.Open(fileName)
	if err != nil {
		return nil, fmt.Errorf("error in NewSuiteFromYAMLFile: %w", err)
	}

	defer func() {
		if err := data.Close(); err != nil {
			panic(err)
		}
	}()

	sy := SuiteYAML{}
	dec := yaml.NewDecoder(data)
	dec.KnownFields(true)

	err = dec.Decode(&sy)
	if err != nil {
		return nil, fmt.Errorf("error in decoding suite in NewSuiteFromYAMLFile: %w", err)
	}

	defaultBytes, err := yaml.Marshal(sy.Defaults)
	if err != nil {
		return nil, fmt.Errorf("error yaml marshaling suite defaults: %w", err)
	}

	var prior *Case
	var processedCases = make([]*Case, len(sy.Tests))

	for i := range sy.Tests {
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
func (s *Suite) Execute(ctx context.Context, t *testing.T) {
	for _, c := range s.Cases {
		t.Run(c.Name, func(u *testing.T) {
			// Reset test reference so nesting works as expected.
			c.SetTest(u, t)
			s.Client.ExecuteOne(ctx, c)
		})
	}
}

// Execute a MultiSuite in parallel.
func (m *MultiSuite) Execute(ctx context.Context, t *testing.T) {
	for _, s := range m.Suites {
		t.Run(s.Name, func(u *testing.T) {
			u.Parallel()
			s.Execute(ctx, u)
		})
	}
}

// TODO: process for fixtures.
func makeCaseFromYAML(t *testing.T, src Case, defaultBytes []byte, prior *Case) (*Case, error) {
	newCase := &Case{}

	err := yaml.Unmarshal(defaultBytes, newCase)
	if err != nil {
		return newCase, fmt.Errorf("error unmarshaling yaml to case default: %w", err)
	}

	newCase.SetDefaults()

	baseCase := src

	srcBytes, err := yaml.Marshal(baseCase)
	if err != nil {
		return newCase, fmt.Errorf("error marshaling yaml case base: %w", err)
	}

	err = yaml.Unmarshal(srcBytes, &newCase)
	if err != nil {
		return newCase, fmt.Errorf("error unmarshaling yaml case with base: %w", err)
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
