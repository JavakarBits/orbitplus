package models

import (
	"time"

	"github.com/blevesearch/bleve/v2"
)

type OrbitSearchResponseModel struct {
	Fields        []string               `json:"fields"`
	Facets        map[string][]TermFacet `json:"facetResult"`
	Total         uint64                 `json:"total"`
	Took          time.Duration          `json:"took"`
	Status        bleve.SearchStatus     `json:"status"`
	SearchResults []OrbitSearchResult    `json:"searchResults"`
}

type SearchRouteResponseModel struct {
	Fields         []string               `json:"fields"`
	Facets         map[string][]TermFacet `json:"facetResult"`
	Total          uint64                 `json:"total"`
	Took           time.Duration          `json:"took"`
	Status         bleve.SearchStatus     `json:"status"`
	OperatorRoutes []OperatorRoute        `json:"searchResults"`
	ModifiedDate   time.Time              `json:"modifiedDate"`
}

type BitsSearchResponseModel struct {
	Fields        []string               `json:"fields"`
	Facets        map[string][]TermFacet `json:"facetResult"`
	Total         uint64                 `json:"total"`
	Took          time.Duration          `json:"took"`
	Status        bleve.SearchStatus     `json:"status"`
	SearchResults []BitsSearchResult     `json:"searchResults"`
	ModifiedDate  time.Time              `json:"modifiedDate"`
}

type BitsBusMapResponseModel struct {
	Fields       []string               `json:"fields"`
	Facets       map[string][]TermFacet `json:"facetResult"`
	Total        uint64                 `json:"total"`
	Took         time.Duration          `json:"took"`
	Status       bleve.SearchStatus     `json:"status"`
	BusMap       BitsBusMap             `json:"busMap"`
	ModifiedDate time.Time              `json:"modifiedDate"`
}

type TermFacet struct {
	Term  string `json:"term"`
	Count int    `json:"count"`
}
