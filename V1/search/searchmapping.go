package search

import (
	"github.com/blevesearch/bleve/analysis/lang/en"
	"github.com/blevesearch/bleve/v2"
	"github.com/blevesearch/bleve/v2/analysis/analyzer/keyword"

	"github.com/blevesearch/bleve/v2/mapping"
)

func buildSearchIndexMapping() (mapping.IndexMapping, error) {
	// a generic reusable mapping for english text
	// englishTextFieldMapping := bleve.NewTextFieldMapping()
	// englishTextFieldMapping.Analyzer = en.AnalyzerName
	// dtFieldMapping := bleve.NewDateTimeFieldMapping()
	//dtFieldMapping.DateFormat =
	// a generic reusable mapping for keyword text
	keywordFieldMapping := bleve.NewTextFieldMapping()
	keywordFieldMapping.Analyzer = keyword.Name
	// keywordFieldMapping.IncludeTermVectors = false
	// keywordFieldMapping.IncludeInAll = false
	// singleFieldMapping := bleve.NewTextFieldMapping()
	// singleFieldMapping.Analyzer = keyword.Name

	searchMapping := bleve.NewDocumentMapping()
	// searchMapping.AddFieldMappingsAt("fromStationCode", keywordFieldMapping)
	// searchMapping.AddFieldMappingsAt("toStationCode", keywordFieldMapping)
	// searchMapping.AddFieldMappingsAt("tripDate", dtFieldMapping)

	indexMapping := bleve.NewIndexMapping()
	indexMapping.DefaultMapping.Dynamic = true
	indexMapping.DefaultMapping.Enabled = true
	indexMapping.AddDocumentMapping("search", searchMapping)

	// indexMapping.TypeField = "type"
	indexMapping.DefaultAnalyzer = "en"
	return indexMapping, nil
}

func buildEventIndexMapping() (mapping.IndexMapping, error) {
	// a generic reusable mapping for english text
	englishTextFieldMapping := bleve.NewTextFieldMapping()
	dtFieldMapping := bleve.NewDateTimeFieldMapping()

	englishTextFieldMapping.Analyzer = en.AnalyzerName

	// a generic reusable mapping for keyword text
	keywordFieldMapping := bleve.NewTextFieldMapping()
	keywordFieldMapping.Analyzer = keyword.Name
	//keywordFieldMapping.IncludeTermVectors = false

	productMapping := bleve.NewDocumentMapping()

	productMapping.AddFieldMappingsAt("createdAt", dtFieldMapping)
	productMapping.AddFieldMappingsAt("updatedAt", dtFieldMapping)
	productMapping.AddFieldMappingsAt("startAt", dtFieldMapping)
	indexMapping := bleve.NewIndexMapping()
	indexMapping.AddDocumentMapping("event", productMapping)
	// indexMapping.DefaultMapping.Dynamic = false
	// indexMapping.DefaultMapping.Enabled = false
	//	err := indexMapping.AddCustomAnalyzer("exactMatchIgnoreCase",
	// map[string]interface{}{
	// "type": custom_analyzer.Name,
	// "tokenizer": Single
	// "token_filters": []string{
	// lower_case_filter.Name,
	// },
	// })
	// if err != nil {
	// log.Fatal(err)
	// }

	// indexMapping.TypeField = "type"
	// indexMapping.DefaultAnalyzer = "en"
	return indexMapping, nil

}

func buildRouteIndexMapping() (mapping.IndexMapping, error) {
	// a generic reusable mapping for english text
	englishTextFieldMapping := bleve.NewTextFieldMapping()
	dtFieldMapping := bleve.NewDateTimeFieldMapping()

	englishTextFieldMapping.Analyzer = en.AnalyzerName

	// a generic reusable mapping for keyword text
	keywordFieldMapping := bleve.NewTextFieldMapping()
	keywordFieldMapping.Analyzer = keyword.Name
	//keywordFieldMapping.IncludeTermVectors = false

	productMapping := bleve.NewDocumentMapping()

	productMapping.AddFieldMappingsAt("createdAt", dtFieldMapping)
	productMapping.AddFieldMappingsAt("updatedAt", dtFieldMapping)
	productMapping.AddFieldMappingsAt("startAt", dtFieldMapping)
	indexMapping := bleve.NewIndexMapping()
	indexMapping.AddDocumentMapping("event", productMapping)
	// indexMapping.DefaultMapping.Dynamic = false
	// indexMapping.DefaultMapping.Enabled = false
	//	err := indexMapping.AddCustomAnalyzer("exactMatchIgnoreCase",
	// map[string]interface{}{
	// "type": custom_analyzer.Name,
	// "tokenizer": Single
	// "token_filters": []string{
	// lower_case_filter.Name,
	// },
	// })
	// if err != nil {
	// log.Fatal(err)
	// }

	// indexMapping.TypeField = "type"
	// indexMapping.DefaultAnalyzer = "en"
	return indexMapping, nil

}
