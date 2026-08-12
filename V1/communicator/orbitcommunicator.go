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

func GetOperatorRoutes() []models.OperatorRoute {
	client := http.Client{
		Timeout: 5 * time.Second,
	}
	response, err := client.Get("http://app.busticketagent.com/orbitservices/api/2.0/json/hxxjfehp79q69nzp/ezeeinfo/operator/route")
	var operatorRoutes []models.OperatorRoute
	if err != nil {
		fmt.Println("Operator Route Timeout .. ")
	} else {
		responseData, err := ioutil.ReadAll(response.Body)
		if err != nil {
			logger.ErrorLogger.Println(err.Error())
		}

		operatorRouteResponse, err := utils.UnMarshalBinaryOrbitOperatorRouteResponse(responseData)
		if err != nil {
			logger.ErrorLogger.Println(err.Error())
		}

		operatorRoutes = append(operatorRoutes, operatorRouteResponse.OperatorRoute...)
	}
	return operatorRoutes
}
