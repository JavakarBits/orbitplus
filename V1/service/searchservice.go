package service

import (
	"encoding/json"
	"fmt"

	"github.com/orbitcacheservices/cache"
	"github.com/orbitcacheservices/communicator"
	"github.com/orbitcacheservices/config"
	"github.com/orbitcacheservices/logger"
	"github.com/orbitcacheservices/models"
	"github.com/orbitcacheservices/search"
	"github.com/orbitcacheservices/utils/date_utils"
)

func GetBitsSearchResult(fromStationCode, toStationCode, tripDate string) []models.BitsSearchResult {
	cacheKey := fmt.Sprintf("%s_%s_%s_%s", "SEARCH", fromStationCode, toStationCode, tripDate)
	/** Get Modified Date From Redis Cache */
	modifiedDateCache, _ := config.GetCache(cacheKey)
	var bitsSearchResult []models.BitsSearchResult
	if modifiedDateCache == "" {
		fmt.Println("22getOperatorBitsSearchResult")
		bitsSearchResult = getOperatorBitsSearchResult(fromStationCode, toStationCode, tripDate)
	} else {
		datetime := date_utils.ConvertDateTime(modifiedDateCache)
		/** Get Search Result Bleve */
		fmt.Println("27getBitsSearchResult")
		bitsResponse := getBitsSearchResult(fromStationCode, toStationCode, tripDate)

		/** Check Modified Date */
		if datetime.After(bitsResponse.ModifiedDate) {
			fmt.Println("32getOperatorBitsSearchResult")
			bitsSearchResult = getOperatorBitsSearchResult(fromStationCode, toStationCode, tripDate)
		} else {
			bitsSearchResult = bitsResponse.SearchResults
		}
	}
	return bitsSearchResult
}

func getOperatorBitsSearchResult(fromStationCode, toStationCode, tripDate string) []models.BitsSearchResult {
	operators := GetRouteOperators(fromStationCode, toStationCode)

	var bitsSearchResult []models.BitsSearchResult
	channel := make(chan searchStatus)
	for _, operator := range operators {
		go searchBitsSearchResult(fromStationCode, toStationCode, tripDate, operator, channel)
	}

	result := make([]searchStatus, len(operators))
	for i, _ := range result {
		result[i] = <-channel
		if result[i].status {
			bitsSearchResult = append(bitsSearchResult, result[i].searchResults...)
			fmt.Println(result[i].operatorCode, result[i].operatorName, " search success !!")
		} else {
			// fmt.Println(result[i].operatorCode, result[i].operatorName, " search is down !!")
		}
	}

	now := date_utils.GetNowDBFormat()

	if len(bitsSearchResult) > 0 {
		var tripBleve models.BitsTripBleve
		tripBleve.FromStationCode = fromStationCode
		tripBleve.ToStationCode = toStationCode
		tripBleve.TripDate = tripDate
		cacheKey := fmt.Sprintf("%s_%s_%s_%s", "SEARCH", fromStationCode, toStationCode, tripDate)

		var searchResults []models.BitsSearchResultBleve
		for _, sr := range bitsSearchResult {
			var searchResult models.BitsSearchResultBleve

			searchResult.TripCode = sr.TripCode
			searchResult.TripStageCode = sr.TripStageCode
			searchResult.TravelDate = sr.TravelDate
			searchResult.DisplayName = sr.DisplayName
			searchResult.TravelTime = sr.TravelTime
			searchResult.CloseTime = sr.CloseTime

			a, _ := json.Marshal(sr.Amenities)
			searchResult.Amenities = string(a)

			op, _ := json.Marshal(sr.Operator)
			searchResult.Operator = string(op)

			tx, _ := json.Marshal(sr.Schedule)
			searchResult.Schedule = string(tx)

			tps, _ := json.Marshal(sr.TripStatus)
			searchResult.TripStatus = string(tps)

			bs, _ := json.Marshal(sr.Bus)
			searchResult.Bus = string(bs)

			fr, _ := json.Marshal(sr.Activities)
			searchResult.Activities = string(fr)

			fs, _ := json.Marshal(sr.FromStation)
			searchResult.FromStation = string(fs)

			ts, _ := json.Marshal(sr.ToStation)
			searchResult.ToStation = string(ts)

			st, _ := json.Marshal(sr.StageFare)
			searchResult.StageFare = string(st)

			ctr, _ := json.Marshal(sr.CancellationTerm)
			searchResult.CancellationTerm = string(ctr)

			searchResults = append(searchResults, searchResult)
		}
		tripBleve.SearchResults = searchResults

		error := cache.BitsTripCreateOrUpdate(tripBleve)
		if error != nil {
			logger.ErrorLogger.Println(error.Error())
		}

		/** Update Modified Date */
		config.AddCache(cacheKey, string(now))
	}
	return bitsSearchResult
}

func searchBitsSearchResult(fromStationCode, toStationCode, tripDate string, operator models.Operator, channel chan searchStatus) {
	bitsSearchResult := communicator.GetBitsSearchResult(fromStationCode, toStationCode, tripDate, operator)
	if len(bitsSearchResult) > 0 {
		channel <- searchStatus{bitsSearchResult, operator.Code, operator.Name, true}
	} else {
		channel <- searchStatus{bitsSearchResult, operator.Code, operator.Name, false}
	}
}

// func searchBitsSearchResult1(fromStationCode, toStationCode, tripDate string, operator models.Operator) {
// 	communicator.GetBitsSearchResult(fromStationCode, toStationCode, tripDate, operator)
// }

func getBitsSearchResult(fromStationCode, toStationCode, tripDate string) *models.BitsSearchResponseModel {
	var searchModel models.SearchRequestModel
	searchModel.IndexName = "bitssearchresult"
	searchModel.DateField = ""

	var terms []string
	if fromStationCode != "" && fromStationCode != "NA" {
		terms = append(terms, fromStationCode)
	}
	if toStationCode != "" && toStationCode != "NA" {
		terms = append(terms, toStationCode)
	}
	if tripDate != "" && tripDate != "NA" {
		terms = append(terms, tripDate)
	}
	if len(terms) == 0 {
		terms = append(terms, "*")
	}
	searchModel.Terms = terms

	var fields []string
	fields = append(fields, "*")
	searchModel.Fields = fields

	var facets []string
	facets = append(facets, "*")
	searchModel.Facets = facets

	resp, error := search.GetBitsSearchResult(searchModel)
	if error != nil {
		logger.ErrorLogger.Println(error.Error())
	}
	return resp
}

type searchStatus struct {
	searchResults []models.BitsSearchResult
	operatorCode  string
	operatorName  string
	status        bool
}
