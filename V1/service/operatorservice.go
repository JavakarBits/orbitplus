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

func GetOperators(fromStationCode, toStationCode string) []models.Operator {
	/** Get Modified Date From Redis Cache */
	modifiedDateCache, _ := config.GetCache("OPERATOR_ROUTE")
	var operators []models.Operator
	if modifiedDateCache == "" {
		operators = addOrbitOperatorRoutes(fromStationCode, toStationCode)
	} else {
		datetime := date_utils.ConvertDateTime(modifiedDateCache)
		/** Get Operator Routes Bleve */
		resp := searchOperators(fromStationCode, toStationCode)

		/** Check Modified Date */
		if datetime.After(resp.ModifiedDate) {
			operators = addOrbitOperatorRoutes(fromStationCode, toStationCode)
		} else {
			for _, operatorRoute := range resp.OperatorRoutes {
				var routeExist bool
				for _, route := range operatorRoute.Routes {
					if route.FromStation.Code == fromStationCode && route.ToStation.Code == toStationCode {
						routeExist = true
						break
					}
				}
				if routeExist {
					operators = append(operators, operatorRoute.Operator)
				}
			}
		}
	}
	return operators
}

func GetRouteOperators(fromStationCode, toStationCode string) []models.Operator {
	/** Get Modified Date From Redis Cache */
	modifiedDateCache, _ := config.GetCache("OPERATOR_ROUTE")
	var operators []models.Operator
	if modifiedDateCache == "" {
		fmt.Println("53addOrbitOperatorRoutes")
		operators = addOrbitOperatorRoutes(fromStationCode, toStationCode)
	} else {
		fmt.Println("56searchOperators")
		datetime := date_utils.ConvertDateTime(modifiedDateCache)
		/** Get Operator Routes Bleve */
		resp := searchOperators(fromStationCode, toStationCode)

		/** Check Modified Date */
		if datetime.After(resp.ModifiedDate) {
			fmt.Println("63addOrbitOperatorRoutes")
			operators = addOrbitOperatorRoutes(fromStationCode, toStationCode)
		} else {
			for _, operatorRoute := range resp.OperatorRoutes {
				var routeExist bool
				for _, route := range operatorRoute.Routes {
					if route.FromStation.Code == fromStationCode && route.ToStation.Code == toStationCode {
						routeExist = true
						break
					}
				}
				if routeExist {
					operators = append(operators, operatorRoute.Operator)
				}
			}
		}
	}
	return operators
}

func addOrbitOperatorRoutes(fromStationCode, toStationCode string) []models.Operator {
	operatorRoutes := communicator.GetOperatorRoutes()

	now := date_utils.GetNowDBFormat()

	for _, or := range operatorRoutes {
		var operatorRouteBleve models.OperatorRouteBleve

		op, _ := json.Marshal(or.Operator)
		operatorRouteBleve.Operator = string(op)

		operatorRouteBleve.OperatorCode = or.Operator.Code
		operatorRouteBleve.RouteKeys = or.RouteKeys

		rs, _ := json.Marshal(or.Routes)
		operatorRouteBleve.Routes = string(rs)

		error := cache.OperatorRoutesCreateOrUpdate(operatorRouteBleve)

		if error != nil {
			logger.ErrorLogger.Println(error.Error())
		}

		/** Update Modified Date */
		config.AddCache("OPERATOR_ROUTE", string(now))
	}

	var operators []models.Operator
	for _, operatorRoute := range operatorRoutes {
		var routeExist bool
		for _, route := range operatorRoute.Routes {
			if route.FromStation.Code == fromStationCode && route.ToStation.Code == toStationCode {
				routeExist = true
				break
			}
		}
		if routeExist {
			operators = append(operators, operatorRoute.Operator)
		}
	}

	return operators
}

func searchOperators(fromStationCode, toStationCode string) *models.SearchRouteResponseModel {
	var searchModel models.SearchRequestModel
	searchModel.IndexName = "operatorroute"
	searchModel.DateField = ""

	var terms []string
	if fromStationCode != "" && fromStationCode != "NA" {
		terms = append(terms, fromStationCode)
	}
	if toStationCode != "" && toStationCode != "NA" {
		terms = append(terms, toStationCode)
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

	resp, error := search.GetOrbitOperatorRoutes(searchModel)
	if error != nil {
		logger.ErrorLogger.Println(error.Error())
	}

	return resp
}
