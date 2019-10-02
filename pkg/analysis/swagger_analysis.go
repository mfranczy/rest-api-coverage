package analysis

import (
	"fmt"
	"strings"

	"github.com/go-openapi/loads"
	"github.com/go-openapi/spec"

	"github.com/mfranczy/crd-rest-coverage/pkg/stats"
)

// AnalyzeSwagger initializes a stats structure based on swagger definition with total params number for each available endpoint
func AnalyzeSwagger(document *loads.Document, filter string) (*stats.Coverage, error) {
	coverage := stats.Coverage{
		Endpoints: make(map[string]map[string]*stats.Endpoint),
	}

	for _, mp := range document.Analyzer.OperationMethodPaths() {
		v := strings.Split(mp, " ")
		if len(v) != 2 {
			return nil, fmt.Errorf("Invalid method:path pair '%s'", mp)
		}
		method, path := strings.ToLower(v[0]), strings.ToLower(v[1])

		// filter requests uri
		if !strings.HasPrefix(path, filter) {
			continue
		}

		if _, ok := coverage.Endpoints[path]; !ok {
			coverage.Endpoints[path] = make(map[string]*stats.Endpoint)
		}

		if _, ok := coverage.Endpoints[path][method]; !ok {
			coverage.Endpoints[path][method] = &stats.Endpoint{
				ParamsHitsDetails: stats.ParamsHitsDetails{
					Query: make(map[string]int),
					Body:  make(map[string]int),
				},
				Path:               path,
				Method:             method,
				ExpectedUniqueHits: 1, // count endpoint calls
			}
		}

		addSwaggerParams(coverage.Endpoints[path][method], document.Analyzer.ParamsFor(method, path), document.Spec().Definitions)
	}

	return &coverage, nil
}

// addSwaggerParams adds parameters from swagger definition into coverage structure,
// parameters contain the total number value which is used to calculate coverage percentage (uniqueHits * 100 / expectedUniqueHits)
func addSwaggerParams(endpoint *stats.Endpoint, params map[string]spec.Parameter, definitions spec.Definitions) {
	for _, param := range params {
		switch param.In {
		case "body":
			if param.Schema != nil {
				endpoint.ExpectedUniqueHits += extractDefParams(param.Schema, definitions, param.Name, endpoint.ParamsHitsDetails.Body)
			} else {
				endpoint.ParamsHitsDetails.Body[param.Name] = 0
				endpoint.ExpectedUniqueHits++
			}
		case "query":
			endpoint.ParamsHitsDetails.Query[param.Name] = 0
			endpoint.ExpectedUniqueHits++
		default:
			continue
		}
	}
}

// extractDefParams returns total param numbers by following definition references
// NOTE: it does not support definitions from external files, only local
func extractDefParams(schema *spec.Schema, definitions spec.Definitions, paramPath string, params map[string]int) int {
	var tokens []string
	ptr := schema.Ref.GetPointer()
	pCnt := 0

	// TODO: replace by ptr.Get() func
	if tokens = ptr.DecodedTokens(); len(tokens) < 2 {
		return 0
	}
	refType, refName := tokens[0], tokens[1]

	if refType != "definitions" {
		return 0
	}

	def, ok := definitions[refName]
	// did not find swagger definition
	if !ok {
		return 0
	}

	// TODO: add cache to optimize params extraction
	if len(def.Properties) > 0 {
		for n, p := range def.Properties {
			path := paramPath + "." + n
			if r := p.Ref.GetPointer(); r != nil && len(r.DecodedTokens()) > 0 {
				pCnt += extractDefParams(&p, definitions, path, params)
			} else {
				params[path] = 0
				pCnt++
			}
		}
	} else {
		params[paramPath] = 0
		pCnt++
	}

	return pCnt
}

// calculateCoverage provides a total REST API and PATH:METHOD coverage number
func calculateCoverage(coverage *stats.Coverage) {
	for _, es := range coverage.Endpoints {
		for _, e := range es {
			if e.MethodCalled {
				e.UniqueHits++
			}
			// sometimes hit number is bigger than params number
			// for instance it might be caused by missing models definition
			// users have to make sure that their definitions are complete
			if e.UniqueHits > e.ExpectedUniqueHits {
				e.UniqueHits = e.ExpectedUniqueHits
			}

			if e.ExpectedUniqueHits > 0 {
				coverage.ExpectedUniqueHits += e.ExpectedUniqueHits
				coverage.UniqueHits += e.UniqueHits
				e.Percent = float64(e.UniqueHits) * 100 / float64(e.ExpectedUniqueHits)
			} else {
				e.Percent = 0
			}
		}
	}

	if coverage.ExpectedUniqueHits > 0 {
		coverage.Percent = float64(coverage.UniqueHits) * 100 / float64(coverage.ExpectedUniqueHits)
	} else {
		coverage.Percent = 0
	}
}
