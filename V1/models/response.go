package models

import (
	"time"
)

type Response struct {
	Status    int         `json:"status"`
	ErrorCode int         `json:"errorCode"`
	ErrorDesc string      `json:"errorDesc"`
	Datetime  string      `json:"datetime"`
	Data      interface{} `json:"data"`
}

func Success(data interface{}) Response {
	var response Response
	response.Status = 1
	response.Datetime = time.Now().Format("2006-01-02 15:04:05")
	response.Data = data
	return response
}

func Failure(errorCode int, errorDesc string) Response {
	var response Response
	response.Status = 0
	response.Datetime = time.Now().Format("2006-01-02 15:04:05")
	response.ErrorCode = errorCode
	response.ErrorDesc = errorDesc
	return response
}
