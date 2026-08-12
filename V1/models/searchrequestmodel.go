package models

type SearchRequestModel struct {
	DateField     string   `json:"dtField"`
	Terms         []string `json:"terms"`
	Fields        []string `json:"fields"`        //Fields that are generated out put
	Facets        []string `json:"facets"`        //list of facet name that needs to be provided
	PharseQueries []string `json:"pharseQueries"` // used to build additional query filter.
	IndexName     string   `json:"indexName"`     //index that needs to be searched
	LocalStorage  bool     `json:"localStorage"`  //if it is set as tru, then will generate csv file and stored under local server
	From          int      `json:"from"`
	Size          int      `json:"size"`
	SortBy        []string `json:"sortBy"`
}
