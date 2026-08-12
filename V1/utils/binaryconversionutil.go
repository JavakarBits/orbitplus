package utils

import (
	"encoding/json"

	"github.com/orbitcacheservices/models"
)

func UnMarshalBinaryArraySearchRequest(data []byte) (models.SearchRequestModel, error) {
	var req models.SearchRequestModel
	err := json.Unmarshal(data, &req)
	return req, err
}

func UnMarshalBinaryArraySearchResult(data []byte) (models.OrbitTrip, error) {
	var req models.OrbitTrip
	err := json.Unmarshal(data, &req)
	return req, err
}

func UnMarshalBinaryArrayBase(data []byte) ([]models.Base, error) {
	var req []models.Base
	err := json.Unmarshal(data, &req)
	return req, err
}

func UnMarshalBinaryBase(data []byte) (models.Base, error) {
	var req models.Base
	err := json.Unmarshal(data, &req)
	return req, err
}

func UnMarshalBinaryOperator(data []byte) (models.Operator, error) {
	var req models.Operator
	err := json.Unmarshal(data, &req)
	return req, err
}

func UnMarshalBinaryFares(data []byte) ([]int, error) {
	var req []int
	err := json.Unmarshal(data, &req)
	return req, err
}

func UnMarshalBinaryStageFares(data []byte) ([]models.StageFare, error) {
	var req []models.StageFare
	err := json.Unmarshal(data, &req)
	return req, err
}

func UnMarshalBinaryCancellationTerm(data []byte) (models.CancellationTerm, error) {
	var req models.CancellationTerm
	err := json.Unmarshal(data, &req)
	return req, err
}

func UnMarshalBinaryStationPointList(data []byte) ([]models.StationPoint, error) {
	var req []models.StationPoint
	err := json.Unmarshal(data, &req)
	return req, err
}

func UnMarshalBinaryTax(data []byte) (models.Tax, error) {
	var req models.Tax
	err := json.Unmarshal(data, &req)
	return req, err
}

func UnMarshalBinarySchedule(data []byte) (models.Schedule, error) {
	var req models.Schedule
	err := json.Unmarshal(data, &req)
	return req, err
}

func UnMarshalBinaryBus(data []byte) (models.Bus, error) {
	var req models.Bus
	err := json.Unmarshal(data, &req)
	return req, err
}

func UnMarshalBinaryStation(data []byte) (models.Station, error) {
	var req models.Station
	err := json.Unmarshal(data, &req)
	return req, err
}

func UnMarshalBinaryOperatorRouteList(data []byte) ([]models.Route, error) {
	var req []models.Route
	err := json.Unmarshal(data, &req)
	return req, err
}

func UnMarshalBinaryBitsSearchResultResponse(data []byte) (models.BitsSearchResultResponse, error) {
	var req models.BitsSearchResultResponse
	err := json.Unmarshal(data, &req)
	return req, err
}

func UnMarshalBinaryBitsBusMapResponse(data []byte) (models.BitsBusMapResponse, error) {
	var req models.BitsBusMapResponse
	err := json.Unmarshal(data, &req)
	return req, err
}

func UnMarshalBinaryOperatorRoutes(data []byte) ([]models.OperatorRoute, error) {
	var req []models.OperatorRoute
	err := json.Unmarshal(data, &req)
	return req, err
}

func UnMarshalBinaryOrbitOperatorRouteResponse(data []byte) (models.OrbitOperatorRouteResponse, error) {
	var req models.OrbitOperatorRouteResponse
	err := json.Unmarshal(data, &req)
	return req, err
}

func UnMarshalBinaryRedisCache(data []byte) (models.RedisCache, error) {
	var req models.RedisCache
	err := json.Unmarshal(data, &req)
	return req, err
}
