package main

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/orbitcacheservices/config"
	"github.com/orbitcacheservices/logger"
	"github.com/orbitcacheservices/models"
	"github.com/orbitcacheservices/search"
	"github.com/orbitcacheservices/service"

	"github.com/gorilla/mux"
)

func main() {
	router := mux.NewRouter().StrictSlash(true)
	// router.Use(verifyAccessToken)
	router.HandleFunc("/orbitcacheservices", welcomePage)
	router.HandleFunc("/orbitcacheservices/operator/routes", getOrbitOperatorRoutes).Methods("GET")
	router.HandleFunc("/orbitcacheservices/search/{fromCode}/{toCode}/{tripDate}", getBitsSearchResult).Methods("GET")
	router.HandleFunc("/orbitcacheservices/{operatorCode}/{username}/{apiToken}/busmap/{tripCode}/{fromCode}/{toCode}/{travelDate}", getBitsBusMap).Methods("GET")

	/**  Redis */
	router.HandleFunc("/orbitcacheservices/redis/search/{fromCode}/{toCode}/{tripDate}", getSearchCache).Methods("GET")
	router.HandleFunc("/orbitcacheservices/redis/operator/routes", getOperatorRoutesCache).Methods("GET")
	router.HandleFunc("/orbitcacheservices/redis/busmap/{tripCode}", getBusMapCache).Methods("GET")

	router.HandleFunc("/orbitcacheservices/redis/search/{fromCode}/{toCode}/{tripDate}/remove", removeSearchCache).Methods("POST")
	router.HandleFunc("/orbitcacheservices/redis/operator/routes/remove", removeOperatorRouteCache).Methods("POST")
	router.HandleFunc("/orbitcacheservices/redis/busmap/{tripCode}/remove", removeBusMapCache).Methods("POST")

	http.ListenAndServe(":8090", router)
}

func welcomePage(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "Welcome!")
}

func getOrbitOperatorRoutes(w http.ResponseWriter, r *http.Request) {
	var searchModel models.SearchRequestModel
	json.NewDecoder(r.Body).Decode(&searchModel)

	resp, error := search.GetOrbitOperatorRoutes(searchModel)
	if error != nil {
		logger.ErrorLogger.Println(error.Error())
		json.NewEncoder(w).Encode(models.Failure(error.Status(), error.Message()))
	}
	json.NewEncoder(w).Encode(models.Success(resp))
}

func getBitsSearchResult(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	fromStationCode := vars["fromCode"]
	toStationCode := vars["toCode"]
	tripDate := vars["tripDate"]
	searchResults := service.GetBitsSearchResult(fromStationCode, toStationCode, tripDate)
	json.NewEncoder(w).Encode(models.Success(searchResults))
}

func getBitsBusMap(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	tripCode := vars["tripCode"]
	fromStationCode := vars["fromCode"]
	toStationCode := vars["toCode"]
	travelDate := vars["travelDate"]

	var operator models.Operator
	operator.Code = vars["operatorCode"]
	operator.Username = vars["username"]
	operator.ApiToken = vars["apiToken"]

	resp := service.GetBusmap(tripCode, fromStationCode, toStationCode, travelDate, operator)

	json.NewEncoder(w).Encode(models.Success(resp))
}

func removeSearchCache(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	fromStationCode := vars["fromCode"]
	toStationCode := vars["toCode"]
	tripDate := vars["tripDate"]

	var cacheKey string
	if fromStationCode != "" && fromStationCode != "NA" && toStationCode != "" && toStationCode != "NA" && tripDate != "" && tripDate != "NA" {
		cacheKey = fmt.Sprintf("%s_%s_%s_%s", "SEARCH", fromStationCode, toStationCode, tripDate)
		config.RemoveCache(cacheKey)
	} else {
		cacheKey = fmt.Sprintf("%s_", "SEARCH")
		config.RemoveCachePrefix(cacheKey)
	}
}

func removeOperatorRouteCache(w http.ResponseWriter, r *http.Request) {
	config.RemoveCache("OPERATOR_ROUTE")
}

func removeBusMapCache(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	tripCode := vars["tripCode"]

	if tripCode != "" && tripCode != "NA" {
		config.RemoveCache("BUSMAP_" + tripCode)
	} else {
		config.RemoveCachePrefix("BUSMAP_")
	}
}

func getSearchCache(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	fromStationCode := vars["fromCode"]
	toStationCode := vars["toCode"]
	tripDate := vars["tripDate"]

	var cacheKey string
	if fromStationCode != "" && fromStationCode != "NA" && toStationCode != "" && toStationCode != "NA" && tripDate != "" && tripDate != "NA" {
		cacheKey = fmt.Sprintf("%s_%s_%s_%s", "SEARCH", fromStationCode, toStationCode, tripDate)
	} else {
		cacheKey = fmt.Sprintf("%s_", "SEARCH")
	}

	var cacheDatas []models.RedisCache
	var err error

	iter, err := config.GetAllKeys(cacheKey)
	if err != nil {
		json.NewEncoder(w).Encode(models.Failure(500, "Unknown Exception"))
		return
	}
	for iter.Next() {
		var cacheData models.RedisCache
		cacheData.Key = iter.Val()
		data, err := config.GetCache(cacheData.Key)
		cacheData.Data = data
		if err != nil {
			json.NewEncoder(w).Encode(models.Failure(500, "Unable To Provide Data"))
			return
		}

		cacheDatas = append(cacheDatas, cacheData)
	}
	if err := iter.Err(); err != nil {
		json.NewEncoder(w).Encode(models.Failure(500, "Unknown Exception"))
		return
	}
	json.NewEncoder(w).Encode(cacheDatas)
}

func getOperatorRoutesCache(w http.ResponseWriter, r *http.Request) {
	var cacheDatas []models.RedisCache
	var err error

	iter, err := config.GetAllKeys("OPERATOR_ROUTE")
	if err != nil {
		json.NewEncoder(w).Encode(models.Failure(500, "Unknown Exception"))
		return
	}
	for iter.Next() {
		var cacheData models.RedisCache
		cacheData.Key = iter.Val()
		data, err := config.GetCache(cacheData.Key)
		cacheData.Data = data
		if err != nil {
			json.NewEncoder(w).Encode(models.Failure(500, "Unable To Provide Data"))
			return
		}

		cacheDatas = append(cacheDatas, cacheData)
	}
	if err := iter.Err(); err != nil {
		json.NewEncoder(w).Encode(models.Failure(500, "Unknown Exception"))
		return
	}
	json.NewEncoder(w).Encode(cacheDatas)
}

func getBusMapCache(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	tripCode := vars["tripCode"]
	var cacheKey string
	if tripCode != "" && tripCode != "NA" {
		cacheKey = "BUSMAP_" + tripCode
	} else {
		cacheKey = "BUSMAP_"
	}

	var cacheDatas []models.RedisCache
	var err error

	iter, err := config.GetAllKeys(cacheKey)
	if err != nil {
		json.NewEncoder(w).Encode(models.Failure(500, "Unknown Exception"))
		return
	}
	for iter.Next() {
		var cacheData models.RedisCache
		cacheData.Key = iter.Val()
		data, err := config.GetCache(cacheData.Key)
		cacheData.Data = data
		if err != nil {
			json.NewEncoder(w).Encode(models.Failure(500, "Unable To Provide Data"))
			return
		}

		cacheDatas = append(cacheDatas, cacheData)
	}
	if err := iter.Err(); err != nil {
		json.NewEncoder(w).Encode(models.Failure(500, "Unknown Exception"))
		return
	}
	json.NewEncoder(w).Encode(cacheDatas)
}

// func verifyAccessToken(next http.Handler) http.Handler {
// 	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
// 		var header = r.Header.Get("x-access-token")
// 		header = strings.TrimSpace(header)

// 		if header != "hxxjfehp79q69nzp" {
// 			w.WriteHeader(http.StatusForbidden)
// 			json.NewEncoder(w).Encode(entity.Failure(int(consts.Unauthorized), consts.Unauthorized.Error()))
// 			return
// 		}
// 		next.SeindexNameseHTTP(w, r)
// 	})
// }
