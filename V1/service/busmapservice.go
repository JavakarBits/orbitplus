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

func GetBusmap(tripCode, fromStationCode, toStationCode, travelDate string, operator models.Operator) models.BitsBusMap {
	/** Get Modified Date From Redis Cache */
	modifiedDateCache, _ := config.GetCache("BUSMAP_" + tripCode)
	var busMapResponse models.BitsBusMap
	if modifiedDateCache == "" {
		fmt.Println("20addBitsBusMap")
		busMapResponse = addBitsBusMap(tripCode, fromStationCode, toStationCode, travelDate, operator)
	} else {
		datetime := date_utils.ConvertDateTime(modifiedDateCache)
		/** Get Operator Routes Bleve */
		fmt.Println("26getBitsBusMap")
		resp := getBitsBusMap(tripCode, fromStationCode, toStationCode, travelDate, operator)
		busMapResponse = resp.BusMap

		/** Check Modified Date */
		if datetime.After(resp.ModifiedDate) {
			fmt.Println("32addBitsBusMap")
			busMapResponse = addBitsBusMap(tripCode, fromStationCode, toStationCode, travelDate, operator)
		}
	}
	return busMapResponse
}

func addBitsBusMap(tripCode, fromStationCode, toStationCode, travelDate string, operator models.Operator) models.BitsBusMap {
	busMap := communicator.GetBitsBusMap(tripCode, fromStationCode, toStationCode, travelDate, operator)
	now := date_utils.GetNowDBFormat()

	var bitsBusMapBleve models.BitsBusMapBleve
	bitsBusMapBleve.FromStationCode = fromStationCode
	bitsBusMapBleve.ToStationCode = toStationCode
	bitsBusMapBleve.ToStationCode = toStationCode
	bitsBusMapBleve.OperatorCode = operator.Code
	bitsBusMapBleve.TripStageCode = busMap.TripStageCode
	bitsBusMapBleve.TripCode = busMap.TripCode
	bitsBusMapBleve.TravelDate = busMap.TravelDate

	tps, _ := json.Marshal(busMap.TripStatus)
	bitsBusMapBleve.TripStatus = string(tps)

	bs, _ := json.Marshal(busMap.Bus)
	bitsBusMapBleve.Bus = string(bs)

	tx, _ := json.Marshal(busMap.Schedule)
	bitsBusMapBleve.Schedule = string(tx)

	fs, _ := json.Marshal(busMap.FromStation)
	bitsBusMapBleve.FromStation = string(fs)

	ts, _ := json.Marshal(busMap.ToStation)
	bitsBusMapBleve.ToStation = string(ts)

	op, _ := json.Marshal(busMap.Operator)
	bitsBusMapBleve.Operator = string(op)

	ctr, _ := json.Marshal(busMap.CancellationTerm)
	bitsBusMapBleve.CancellationTerm = string(ctr)

	error := cache.BitsBusMapCreateOrUpdate(bitsBusMapBleve)
	if error != nil {
		logger.ErrorLogger.Println(error.Error())
	}

	/** Update Modified Date */
	config.AddCache("BUSMAP_"+tripCode, string(now))

	return busMap
}

func getBitsBusMap(tripCode, fromStationCode, toStationCode, travelDate string, operator models.Operator) *models.BitsBusMapResponseModel {
	var searchModel models.SearchRequestModel
	searchModel.IndexName = "bitsbusmap"
	searchModel.DateField = ""

	var terms []string
	if tripCode != "" && tripCode != "NA" {
		terms = append(terms, tripCode)
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

	resp, _ := search.GetBitsBusMap(searchModel)
	return resp
}
