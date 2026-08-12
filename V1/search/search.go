package search

import (
	"encoding/csv"
	"fmt"
	"log"
	"net/http"
	"os"
	"path"
	"time"

	"github.com/blevesearch/bleve/v2"
	"github.com/orbitcacheservices/errors"
	"github.com/orbitcacheservices/models"
	"github.com/orbitcacheservices/utils"
	"github.com/orbitcacheservices/utils/date_utils"
)

var (
	OperatorRoutesIndex   bleve.Index
	BitsSearchResultIndex bleve.Index
	BitsBusMapIndex       bleve.Index
)

func init() {
	log.Println("Index creating...")
	CreateIndex()
}

func getIndex(indexName string) bleve.Index {
	var index bleve.Index
	switch indexName {
	case "operatorroute":
		index = OperatorRoutesIndex
	case "bitssearchresult":
		index = BitsSearchResultIndex
	case "bitsbusmap":
		index = BitsBusMapIndex
	}
	return index
}

func CreateIndex() {
	var err error
	OperatorRoutesIndex, err = bleve.Open("data/orbit.route.bleve")
	if err == bleve.ErrorIndexPathDoesNotExist {
		fmt.Println(fmt.Sprintf("Creating operator route new index %s ... ", "data/orbit.route.bleve"))
		// create a mapping
		indexMapping, err := buildRouteIndexMapping()
		if err != nil {
			log.Fatal(err)
		}
		OperatorRoutesIndex, err = bleve.New("data/orbit.route.bleve", indexMapping)
		if err != nil {
			fmt.Println("Failed operator route index field mapping", err)
		}
	}
	BitsSearchResultIndex, err = bleve.Open("data/bits.search.bleve")
	if err == bleve.ErrorIndexPathDoesNotExist {
		fmt.Println(fmt.Sprintf("Creating bits search new index %s ... ", "data/bits.search.bleve"))
		// create a mapping
		indexMapping, err := buildRouteIndexMapping()
		if err != nil {
			log.Fatal(err)
		}
		BitsSearchResultIndex, err = bleve.New("data/bits.search.bleve", indexMapping)
		if err != nil {
			fmt.Println("Failed bits search index field mapping", err)
		}
	}
	BitsBusMapIndex, err = bleve.Open("data/bits.busmap.bleve")
	if err == bleve.ErrorIndexPathDoesNotExist {
		fmt.Println(fmt.Sprintf("Creating bits busmap new index %s ... ", "data/bits.busmap.bleve"))
		// create a mapping
		indexMapping, err := buildRouteIndexMapping()
		if err != nil {
			log.Fatal(err)
		}
		BitsBusMapIndex, err = bleve.New("data/bits.busmap.bleve", indexMapping)
		if err != nil {
			fmt.Println("Failed bits busmap index field mapping", err)
		}
	}
}

func GetBitsSearchResult(m models.SearchRequestModel) (*models.BitsSearchResponseModel, errors.RestErrors) {
	q := bleve.NewBooleanQuery()

	for _, v := range m.Terms {
		mq := bleve.NewMatchQuery(v)
		q.AddMust(mq)
	}
	if len(m.PharseQueries) > 0 {
		for _, v := range m.PharseQueries {
			mpq := bleve.NewMatchPhraseQuery(v)
			q.AddMust(mpq)
		}
	}

	req := bleve.NewSearchRequest(q)

	if len(m.Fields) > 0 {
		req.Fields = m.Fields
	} else {
		req.Fields = []string{"*"}
	}
	if len(m.SortBy) > 0 {
		req.SortBy(m.SortBy)
	}
	for _, f := range m.Facets {
		req.AddFacet(f, bleve.NewFacetRequest(f, 10))
	}
	req.From = m.From
	req.Size = m.Size
	if m.Size == 0 {
		req.Size = 150
	}

	index := getIndex(m.IndexName)
	fmt.Println("index name", m.IndexName, index.Name())
	res, err := index.Search(req)
	if err != nil {
		fmt.Println("Failed while execute the query", err)
		return nil, errors.NewInternalServerError("Failed while execute the query", err)
	}
	rm := models.BitsSearchResponseModel{}
	resultRow := make([]map[string]interface{}, 0)

	rm.Fields = res.Request.Fields
	// resS, _ := json.Marshal(res)
	// fmt.Println("search res", string(resS))
	rm.Total = res.Total
	rm.Took = res.Took
	rm.Status = *res.Status
	for _, rv := range res.Hits {
		resultRow = append(resultRow, rv.Fields)
	}

	var searchResults []models.BitsSearchResult
	var dateTime time.Time
	for _, rr := range resultRow {
		tripCodes := rr["tripCode"].([]interface{})
		travelDate := rr["travelDate"].([]interface{})
		modifiedDate := rr["modifiedDate"].([]interface{})
		tripStageCode := rr["tripStageCode"].([]interface{})
		travelTime := rr["travelTime"].([]interface{})
		closeTime := rr["closeTime"].([]interface{})

		amenities := rr["amenities"].([]interface{})
		operator := rr["operator"].([]interface{})
		schedule := rr["schedule"].([]interface{})
		tripStatus := rr["tripStatus"].([]interface{})
		bus := rr["bus"].([]interface{})
		activities := rr["activities"].([]interface{})
		fromStation := rr["fromStation"].([]interface{})
		toStation := rr["toStation"].([]interface{})
		stageFare := rr["stageFare"].([]interface{})
		cancellationTerm := rr["cancellationTerm"].([]interface{})

		tripCount := len(tripCodes)
		for i := 0; i < tripCount; i++ {
			var searchResult models.BitsSearchResult

			searchResult.TripCode = fmt.Sprint(tripCodes[i])
			searchResult.TravelDate = fmt.Sprint(travelDate[i])
			searchResult.TripStageCode = fmt.Sprint(tripStageCode[i])
			searchResult.TravelTime = fmt.Sprint(travelTime[i])
			searchResult.CloseTime = fmt.Sprint(closeTime[i])

			datetime := date_utils.ConvertDateTime(fmt.Sprint(modifiedDate[i]))
			if i == 0 || dateTime.After(datetime) {
				dateTime = datetime
			}

			amen, _ := utils.UnMarshalBinaryArrayBase([]byte(fmt.Sprint(amenities[i])))
			searchResult.Amenities = amen

			op, _ := utils.UnMarshalBinaryOperator([]byte(fmt.Sprint(operator[i])))
			searchResult.Operator = op

			tx, _ := utils.UnMarshalBinarySchedule([]byte(fmt.Sprint(schedule[i])))
			searchResult.Schedule = tx

			tps, _ := utils.UnMarshalBinaryBase([]byte(fmt.Sprint(tripStatus[i])))
			searchResult.TripStatus = tps

			bs, _ := utils.UnMarshalBinaryBus([]byte(fmt.Sprint(bus[i])))
			searchResult.Bus = bs

			act, _ := utils.UnMarshalBinaryArrayBase([]byte(fmt.Sprint(activities[i])))
			searchResult.Activities = act

			fms, _ := utils.UnMarshalBinaryStation([]byte(fmt.Sprint(fromStation[i])))
			searchResult.FromStation = fms

			tms, _ := utils.UnMarshalBinaryStation([]byte(fmt.Sprint(toStation[i])))
			searchResult.ToStation = tms

			fr, _ := utils.UnMarshalBinaryStageFares([]byte(fmt.Sprint(stageFare[i])))
			searchResult.StageFare = fr

			ctm, _ := utils.UnMarshalBinaryCancellationTerm([]byte(fmt.Sprint(cancellationTerm[i])))
			searchResult.CancellationTerm = ctm

			searchResults = append(searchResults, searchResult)
		}
	}
	rm.SearchResults = searchResults
	rm.ModifiedDate = dateTime

	if len(res.Facets) > 0 {
		rm.Facets = make(map[string][]models.TermFacet)
	}
	for k, v := range res.Facets {
		terms := make([]models.TermFacet, 0)
		for _, fv := range v.Terms {
			terms = append(terms, models.TermFacet{Term: fv.Term, Count: fv.Count})
		}
		rm.Facets[k] = terms
	}
	//resbyte, err := json.Marshal(rm)
	if err != nil {
		fmt.Println("Failed while marshal final search result", err)
		return nil, errors.NewRestErrors("Failed while final search result", http.StatusInternalServerError, err.Error(), nil)
	}
	//header
	if len(resultRow) == 0 {
		return nil, nil
	}
	fieldPos := make(map[string]int)
	if rm.Fields[0] != "*" {
		fmt.Println("read from fields list")
		for idx, fn := range rm.Fields {
			fieldPos[fn] = idx
		}
	} else {
		idx := 0
		for f := range resultRow[0] {
			fieldPos[f] = idx
			idx = idx + 1
		}
	}

	// generateCSV(&m, fieldPos, resultRow)
	return &rm, nil
}

func GetBitsBusMap(m models.SearchRequestModel) (*models.BitsBusMapResponseModel, errors.RestErrors) {
	q := bleve.NewBooleanQuery()

	for _, v := range m.Terms {
		mq := bleve.NewMatchQuery(v)
		q.AddMust(mq)
	}
	if len(m.PharseQueries) > 0 {
		for _, v := range m.PharseQueries {
			mpq := bleve.NewMatchPhraseQuery(v)
			q.AddMust(mpq)
		}
	}

	req := bleve.NewSearchRequest(q)

	if len(m.Fields) > 0 {
		req.Fields = m.Fields
	} else {
		req.Fields = []string{"*"}
	}
	if len(m.SortBy) > 0 {
		req.SortBy(m.SortBy)
	}
	for _, f := range m.Facets {
		req.AddFacet(f, bleve.NewFacetRequest(f, 10))
	}
	req.From = m.From
	req.Size = m.Size
	if m.Size == 0 {
		req.Size = 150
	}

	index := getIndex(m.IndexName)
	fmt.Println("index name", m.IndexName, index.Name())
	res, err := index.Search(req)
	if err != nil {
		fmt.Println("Failed while execute the query", err)
		return nil, errors.NewInternalServerError("Failed while execute the query", err)
	}
	rm := models.BitsBusMapResponseModel{}
	resultRow := make([]map[string]interface{}, 0)

	rm.Fields = res.Request.Fields
	// resS, _ := json.Marshal(res)
	// fmt.Println("search res", string(resS))
	rm.Total = res.Total
	rm.Took = res.Took
	rm.Status = *res.Status
	for _, rv := range res.Hits {
		resultRow = append(resultRow, rv.Fields)
	}

	var busMap models.BitsBusMap
	for _, rr := range resultRow {
		busMap.TripCode = fmt.Sprint(rr["tripCode"])
		busMap.TravelDate = fmt.Sprint(rr["travelDate"])
		busMap.TripStageCode = fmt.Sprint(rr["tripStageCode"])
		rm.ModifiedDate = date_utils.ConvertDateTime(fmt.Sprint(rr["modifiedDate"]))

		tps, _ := utils.UnMarshalBinaryBase([]byte(fmt.Sprint(rr["tripStatus"])))
		busMap.TripStatus = tps

		bs, _ := utils.UnMarshalBinaryBus([]byte(fmt.Sprint(rr["bus"])))
		busMap.Bus = bs

		tx, _ := utils.UnMarshalBinarySchedule([]byte(fmt.Sprint(rr["schedule"])))
		busMap.Schedule = tx

		fms, _ := utils.UnMarshalBinaryStation([]byte(fmt.Sprint(rr["fromStation"])))
		busMap.FromStation = fms

		tms, _ := utils.UnMarshalBinaryStation([]byte(fmt.Sprint(rr["toStation"])))
		busMap.ToStation = tms

		op, _ := utils.UnMarshalBinaryOperator([]byte(fmt.Sprint(rr["operator"])))
		busMap.Operator = op

		ctm, _ := utils.UnMarshalBinaryCancellationTerm([]byte(fmt.Sprint(rr["cancellationTerm"])))
		busMap.CancellationTerm = ctm

		break
	}
	rm.BusMap = busMap

	if len(res.Facets) > 0 {
		rm.Facets = make(map[string][]models.TermFacet)
	}
	for k, v := range res.Facets {
		terms := make([]models.TermFacet, 0)
		for _, fv := range v.Terms {
			terms = append(terms, models.TermFacet{Term: fv.Term, Count: fv.Count})
		}
		rm.Facets[k] = terms
	}
	//resbyte, err := json.Marshal(rm)
	if err != nil {
		fmt.Println("Failed while marshal final search result", err)
		return nil, errors.NewRestErrors("Failed while final search result", http.StatusInternalServerError, err.Error(), nil)
	}
	//header
	if len(resultRow) == 0 {
		return nil, nil
	}
	fieldPos := make(map[string]int)
	if rm.Fields[0] != "*" {
		fmt.Println("read from fields list")
		for idx, fn := range rm.Fields {
			fieldPos[fn] = idx
		}
	} else {
		idx := 0
		for f := range resultRow[0] {
			fieldPos[f] = idx
			idx = idx + 1
		}
	}

	// generateCSV(&m, fieldPos, resultRow)
	return &rm, nil
}

func GetOrbitOperatorRoutes(m models.SearchRequestModel) (*models.SearchRouteResponseModel, errors.RestErrors) {
	q := bleve.NewBooleanQuery()

	for _, v := range m.Terms {
		mq := bleve.NewMatchQuery(v)
		q.AddMust(mq)
	}
	if len(m.PharseQueries) > 0 {
		for _, v := range m.PharseQueries {
			mpq := bleve.NewMatchPhraseQuery(v)
			q.AddMust(mpq)
		}
	}

	req := bleve.NewSearchRequest(q)

	if len(m.Fields) > 0 {
		req.Fields = m.Fields
	} else {
		req.Fields = []string{"*"}
	}
	if len(m.SortBy) > 0 {
		req.SortBy(m.SortBy)
	}
	for _, f := range m.Facets {
		req.AddFacet(f, bleve.NewFacetRequest(f, 10))
	}
	req.From = m.From
	req.Size = m.Size
	if m.Size == 0 {
		req.Size = 150
	}

	index := getIndex(m.IndexName)
	fmt.Println("index name", m.IndexName, index.Name())
	res, err := index.Search(req)
	if err != nil {
		fmt.Println("Failed while execute the query", err)
		return nil, errors.NewInternalServerError("Failed while execute the query", err)
	}
	rm := models.SearchRouteResponseModel{}
	resultRow := make([]map[string]interface{}, 0)

	rm.Fields = res.Request.Fields
	rm.Total = res.Total
	rm.Took = res.Took
	rm.Status = *res.Status

	for _, rv := range res.Hits {
		resultRow = append(resultRow, rv.Fields)
	}

	var routes []models.OperatorRoute
	dateTime := time.Now()
	for _, rr := range resultRow {
		var operatorRoutes models.OperatorRoute
		operator, _ := utils.UnMarshalBinaryOperator([]byte(fmt.Sprint(rr["operator"])))
		operatorRoutes.Operator = operator

		route, _ := utils.UnMarshalBinaryOperatorRouteList([]byte(fmt.Sprint(rr["routes"])))
		operatorRoutes.Routes = route

		datetime := date_utils.ConvertDateTime(fmt.Sprint(rr["modifiedDate"]))
		if dateTime.After(datetime) {
			dateTime = datetime
		}

		routes = append(routes, operatorRoutes)
	}
	rm.OperatorRoutes = routes
	rm.ModifiedDate = dateTime

	if len(res.Facets) > 0 {
		rm.Facets = make(map[string][]models.TermFacet)
	}
	for k, v := range res.Facets {
		terms := make([]models.TermFacet, 0)
		for _, fv := range v.Terms {
			terms = append(terms, models.TermFacet{Term: fv.Term, Count: fv.Count})
		}
		rm.Facets[k] = terms
	}
	if err != nil {
		fmt.Println("Failed while marshal final search result", err)
		return nil, errors.NewRestErrors("Failed while final search result", http.StatusInternalServerError, err.Error(), nil)
	}
	//header
	if len(resultRow) == 0 {
		return nil, nil
	}
	fieldPos := make(map[string]int)
	if rm.Fields[0] != "*" {
		fmt.Println("read from fields list")
		for idx, fn := range rm.Fields {
			fieldPos[fn] = idx
		}
	} else {
		idx := 0
		for f := range resultRow[0] {
			fieldPos[f] = idx
			idx = idx + 1
		}
	}

	// generateCSV(&m, fieldPos, resultRow)
	return &rm, nil
}

func getInterfaceArray(data []interface{}) []interface{} {
	if data == nil {
		var data1 []interface{}
		return data1
	}
	return data
}

func generateCSV(reqM *models.SearchRequestModel, fields map[string]int, resultRow []map[string]interface{}) {
	wd, err := os.Getwd()
	if err != nil {
		fmt.Println("Failed while get the working dir", err)
		return
	}
	dt := time.Now().UTC()
	p := fmt.Sprintf("%s/csvs/%s/%d%d%d", wd, reqM.IndexName, dt.Year(), dt.Month(), dt.Day())

	if _, err := os.Stat(p); os.IsNotExist(err) {
		//fmt.Println("generateCSV", err)
		if err := os.MkdirAll(p, os.ModePerm); err != nil {
			fmt.Println("Failed while create file", err)
			return
		}
	}
	fn := fmt.Sprintf("search_%d_%d_%d%d%d%d.csv", dt.Year(), dt.Month(), dt.Day(), dt.Hour(), dt.Minute(), dt.Second())
	//fullFileName:=fmt.Sprintf("%s/%s", path, fn)
	file, err := os.Create(path.Join(p, fn))
	if err != nil {
		fmt.Println(fmt.Sprintf("Failed creating file %s", path.Join(p, fn)), err)
	}
	defer file.Close()
	if err != nil {
		fmt.Println("Failed while create csv files", err)
	}
	// 2. Initialize the writer
	writer := csv.NewWriter(file)

	csvData := make([][]string, 0)

	l := len(fields)
	row := make([]string, l)
	for k, v := range fields {
		//	fmt.Println("field order", l, k, v)
		row[v] = k
	}

	//fmt.Println("print header", row)
	csvData = append(csvData, row)
	//product data
	for _, r := range resultRow {
		row = make([]string, len(fields))
		for f, val := range r {
			idx, isFound := fields[f]
			if isFound {
				//fmt.Println("col value", f, val, idx)
				row[idx] = fmt.Sprintf("%v", val)
			}
		}
		//fmt.Println("row len", len(row))
		csvData = append(csvData, row)
	}
	// 3. Write all the records
	err = writer.WriteAll(csvData) // returns error
	if err != nil {
		fmt.Println("An error encountered ::", err)
	}
}
