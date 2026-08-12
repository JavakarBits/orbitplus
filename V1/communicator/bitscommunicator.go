package communicator

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/orbitcacheservices/logger"
	"github.com/orbitcacheservices/models"
	"github.com/orbitcacheservices/utils"
)

func GetBitsSearchResult(fromStationCode, toStationCode, tripDate string, operator models.Operator) []models.BitsSearchResult {
	client := http.Client{
		Timeout: 5 * time.Second,
	}
	response, err := client.Get("http://app.ezeebits.com/busservices/api/3.0/json/" + operator.Code + "/" + operator.Username + "/" + operator.ApiToken + "/search/" + fromStationCode + "/" + toStationCode + "/" + tripDate)
	var bitsSearchResult []models.BitsSearchResult
	if err != nil {
		fmt.Println("Timeout .. " + operator.Name)
	} else {
		responseData, err := ioutil.ReadAll(response.Body)
		if err != nil {
			logger.ErrorLogger.Println(err.Error())
		}

		bitsSearchResultResponse, err := utils.UnMarshalBinaryBitsSearchResultResponse(responseData)
		if err != nil {
			logger.ErrorLogger.Println(err.Error())
		}
		bitsSearchResult = append(bitsSearchResult, bitsSearchResultResponse.BitsSearchResult...)
	}
	return bitsSearchResult
}

func GetBitsBusMap(tripCode, fromStationCode, toStationCode, travelDate string, operator models.Operator) models.BitsBusMap {
	client := http.Client{
		Timeout: 5 * time.Second,
	}
	var busmap models.BitsBusMap
	response, err := client.Get("http://app.ezeebits.com/busservices/api/3.0/json/" + operator.Code + "/" + operator.Username + "/" + operator.ApiToken + "/busmap/" + tripCode + "/" + fromStationCode + "/" + toStationCode + "/" + travelDate)
	if err != nil {
		fmt.Println("Timeout .. " + operator.Name)
	} else {
		responseData, err := ioutil.ReadAll(response.Body)
		if err != nil {
			logger.ErrorLogger.Println(err.Error())
		}

		bitsBusMapResponse, err := utils.UnMarshalBinaryBitsBusMapResponse(responseData)
		if err != nil {
			logger.ErrorLogger.Println(err.Error())
		}
		busmap = bitsBusMapResponse.BusMap
	}
	return busmap
}
