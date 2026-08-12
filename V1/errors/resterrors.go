package errors

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
)

type RestErrors interface {
	Message() string
	Status() int
	Error() string
	Causes() []interface{}
}

type restErrors struct {
	ErrMessage string        `json:"message"`
	ErrStatus  int           `json:"status"`
	ErrError   string        `json:"error"`
	ErrCauses  []interface{} `json:"causes"`
}

func (e restErrors) Error() string {
	return fmt.Sprintf("message: %s - status: %d - error: %s - causes: %v",
		e.ErrMessage, e.ErrStatus, e.ErrError, e.ErrCauses)
}

func (e restErrors) Message() string {
	return e.ErrMessage
}

func (e restErrors) Status() int {
	return e.ErrStatus
}

func (e restErrors) Causes() []interface{} {
	return e.ErrCauses
}

func NewRestErrors(message string, status int, err string, causes []interface{}) RestErrors {
	return restErrors{
		ErrMessage: message,
		ErrStatus:  status,
		ErrError:   err,
		ErrCauses:  causes,
	}
}

func NewRestErrorsFromBytes(bytes []byte) (RestErrors, error) {
	var apiErr restErrors
	if err := json.Unmarshal(bytes, &apiErr); err != nil {
		return nil, errors.New("invalid json")
	}
	return apiErr, nil
}

func NewBadRequestError(message string) RestErrors {
	return restErrors{
		ErrMessage: message,
		ErrStatus:  http.StatusBadRequest,
		ErrError:   "bad_request",
	}
}

func NewNotFoundError(message string) RestErrors {
	return restErrors{
		ErrMessage: message,
		ErrStatus:  http.StatusNotFound,
		ErrError:   "not_found",
	}
}

func NewUnauthorizedError(message string) RestErrors {
	return restErrors{
		ErrMessage: message,
		ErrStatus:  http.StatusUnauthorized,
		ErrError:   "unauthorized",
	}
}

func NewInternalServerError(message string, err error) RestErrors {
	result := restErrors{
		ErrMessage: message,
		ErrStatus:  http.StatusInternalServerError,
		ErrError:   "internal_server_error",
	}
	if err != nil {
		result.ErrCauses = append(result.ErrCauses, err.Error())
	}
	return result
}
func NewMissingPrimayKey(message string) RestErrors {
	result := restErrors{
		ErrMessage: message,
		ErrStatus:  http.StatusInternalServerError,
		ErrError:   "Missing field Id",
	}
	return result
}
