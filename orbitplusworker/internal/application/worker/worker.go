package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"orbitplusworker/internal/domain"
)

// TripDetailsRefreshWorker has no correctness, coordination, or recovery state.
// Every delivery follows the same direct path, and OrbitPlus is authoritative for
// successful, duplicate, and stale-result outcomes.
type TripDetailsRefreshWorker struct {
	config           WorkerConfig
	consumer         RabbitMQConsumer
	source           TripDetailsClient
	credentialClient OperatorCredentialClient
	orbitPlusClient  OrbitPlusClient
}

func NewTripDetailsRefreshWorker(config WorkerConfig, consumer RabbitMQConsumer, source TripDetailsClient, credentialClient OperatorCredentialClient, orbitPlusClient OrbitPlusClient) (*TripDetailsRefreshWorker, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	if consumer == nil || source == nil || credentialClient == nil || orbitPlusClient == nil {
		return nil, fmt.Errorf("%w: all injected dependencies are required", ErrInvalidConfig)
	}
	return &TripDetailsRefreshWorker{
		config:           config,
		consumer:         consumer,
		source:           source,
		credentialClient: credentialClient,
		orbitPlusClient:  orbitPlusClient,
	}, nil
}

// Handle processes one delivery. A valid message is processed once. Any terminal
// processing failure is reported to the OrbitPlus DLQ API; the RabbitMQ
// delivery is acknowledged only after the DLQ report is accepted.
func (worker *TripDetailsRefreshWorker) Handle(ctx context.Context, delivery RabbitMQDelivery) (result ExecutionResult) {
	startedAt := time.Now()
	var message domain.TripDetailsRefreshMessage
	defer func() {
		slog.Info("TripDetails refresh completed",
			"actionType", message.ActionType,
			"operator", message.OperatorCode,
			"status", result.Status,
			"orbitPlusStatus", result.OrbitPlusStatus,
			"acknowledged", result.Acknowledged,
			"failed", result.Err != nil,
			"duration", time.Since(startedAt).String(),
		)
	}()

	if err := ctx.Err(); err != nil {
		return ExecutionResult{Status: ExecutionCancelled, Err: err}
	}
	parsedMessage, err := parseTripDetailsRefreshMessage(delivery)
	if err != nil {
		return ExecutionResult{Status: ExecutionInvalidEnvelope, Err: err}
	}
	message = parsedMessage
	slog.Info("TripDetails refresh started", "actionType", message.ActionType, "operator", message.OperatorCode)

	credential, err := worker.resolveCredential(ctx, message)
	if err != nil {
		return worker.reportFailure(ctx, delivery, message, ExecutionCredentialError, "Orbit operator credential could not be resolved.")
	}
	sourceResult, err := worker.fetchTripDetails(ctx, message, credential)
	if err != nil {
		return worker.reportFailure(ctx, delivery, message, ExecutionSourceError, "Bits request failed or returned an empty response.")
	}
	orbitPlusStatus, err := worker.pushTripDetails(ctx, message, sourceResult)
	if err != nil {
		return worker.reportFailure(ctx, delivery, message, ExecutionOrbitPlusError, "OrbitPlus TripDetails request failed.")
	}
	if !orbitPlusStatus.AcknowledgementEligible() {
		return worker.reportFailure(ctx, delivery, message, ExecutionOrbitPlusOutcome, "OrbitPlus TripDetails returned a retryable response.", orbitPlusStatus)
	}

	slog.Info("RabbitMQ acknowledgement started", "actionType", message.ActionType, "operator", message.OperatorCode, "status", orbitPlusStatus)
	if err := delivery.Ack(ctx); err != nil {
		slog.Info("RabbitMQ acknowledgement completed", "actionType", message.ActionType, "operator", message.OperatorCode, "success", false)
		return worker.operationError(ExecutionAckError, err, orbitPlusStatus)
	}
	slog.Info("RabbitMQ acknowledgement completed", "actionType", message.ActionType, "operator", message.OperatorCode, "success", true)
	return ExecutionResult{Status: ExecutionAcknowledged, OrbitPlusStatus: orbitPlusStatus, Acknowledged: true}
}

func (worker *TripDetailsRefreshWorker) reportFailure(ctx context.Context, delivery RabbitMQDelivery, message domain.TripDetailsRefreshMessage, failureStatus ExecutionStatus, failureDetail string, orbitPlusStatus ...OrbitPlusStatus) ExecutionResult {
	failureMessage := fmt.Sprintf("Worker could not process the %s job: %s", message.ActionType, failureDetail)
	slog.Info("OrbitPlus DLQ report started", "actionType", message.ActionType, "operator", message.OperatorCode, "failureStatus", failureStatus)
	if err := worker.orbitPlusClient.SendTripDetailsDLQ(ctx, TripDetailsDLQRequest{
		ReferenceID:    message.ReferenceID,
		FailureMessage: failureMessage,
	}); err != nil {
		slog.Info("OrbitPlus DLQ report completed", "actionType", message.ActionType, "operator", message.OperatorCode, "success", false)
		return worker.operationError(ExecutionDLQError, err, orbitPlusStatus...)
	}
	slog.Info("OrbitPlus DLQ report completed", "actionType", message.ActionType, "operator", message.OperatorCode, "success", true)

	slog.Info("RabbitMQ acknowledgement started", "actionType", message.ActionType, "operator", message.OperatorCode, "status", "DLQ_REPORTED")
	if err := delivery.Ack(ctx); err != nil {
		slog.Info("RabbitMQ acknowledgement completed", "actionType", message.ActionType, "operator", message.OperatorCode, "success", false)
		return worker.operationError(ExecutionAckError, err, orbitPlusStatus...)
	}
	slog.Info("RabbitMQ acknowledgement completed", "actionType", message.ActionType, "operator", message.OperatorCode, "success", true)
	result := ExecutionResult{Status: ExecutionDLQReported, Acknowledged: true}
	if len(orbitPlusStatus) == 1 {
		result.OrbitPlusStatus = orbitPlusStatus[0]
	}
	return result
}

func parseTripDetailsRefreshMessage(delivery RabbitMQDelivery) (domain.TripDetailsRefreshMessage, error) {
	var message domain.TripDetailsRefreshMessage
	if err := json.Unmarshal(delivery.Payload(), &message); err != nil {
		return domain.TripDetailsRefreshMessage{}, fmt.Errorf("decode TripDetails refresh message: %w", err)
	}
	if err := message.Validate(); err != nil {
		return domain.TripDetailsRefreshMessage{}, err
	}
	return message, nil
}

// resolveCredential fetches the operator's Bits credential from OrbitService.
// Credential values, credential-bearing URLs, and secrets are never logged.
func (worker *TripDetailsRefreshWorker) resolveCredential(ctx context.Context, message domain.TripDetailsRefreshMessage) (BitsOperatorCredential, error) {
	slog.Info("Orbit operator credential request started", "actionType", message.ActionType, "operator", message.OperatorCode)
	operatorCredential, err := worker.credentialClient.FetchOperatorCredential(ctx, OperatorCredentialRequest{OperatorCode: message.OperatorCode})
	if err != nil {
		slog.Info("Orbit operator credential request completed", "actionType", message.ActionType, "operator", message.OperatorCode, "success", false)
		return BitsOperatorCredential{}, err
	}
	slog.Info("Orbit operator credential request completed", "actionType", message.ActionType, "operator", message.OperatorCode, "success", true)

	credential := BitsOperatorCredential{
		OperatorCode: message.OperatorCode,
		Username:     operatorCredential.Username,
		APIToken:     operatorCredential.APIToken,
		BaseURL:      message.ZoneURL,
	}
	if err := credential.Validate(message.OperatorCode); err != nil {
		return BitsOperatorCredential{}, err
	}
	return credential, nil
}

func (worker *TripDetailsRefreshWorker) fetchTripDetails(ctx context.Context, message domain.TripDetailsRefreshMessage, credential BitsOperatorCredential) (BitsTripDetailsResponse, error) {
	slog.Info("Bits TripDetails request started", "actionType", message.ActionType, "operator", message.OperatorCode)
	sourceResult, err := worker.source.FetchTripDetails(ctx, BitsTripDetailsRequest{Message: message, Credential: credential})
	if err != nil {
		slog.Info("Bits TripDetails request completed", "actionType", message.ActionType, "operator", message.OperatorCode, "success", false)
		return BitsTripDetailsResponse{}, err
	}
	slog.Info("Bits TripDetails response received", "actionType", message.ActionType, "operator", message.OperatorCode, "responseBody", string(sourceResult.Body))
	if len(sourceResult.Body) == 0 {
		slog.Info("Bits TripDetails request completed", "actionType", message.ActionType, "operator", message.OperatorCode, "success", false)
		return BitsTripDetailsResponse{}, fmt.Errorf("Bits response is empty")
	}
	slog.Info("Bits TripDetails request completed", "actionType", message.ActionType, "operator", message.OperatorCode, "success", true)
	return sourceResult, nil
}

func (worker *TripDetailsRefreshWorker) pushTripDetails(ctx context.Context, message domain.TripDetailsRefreshMessage, sourceResult BitsTripDetailsResponse) (OrbitPlusStatus, error) {
	slog.Info("OrbitPlus TripDetails push started", "actionType", message.ActionType, "operator", message.OperatorCode)
	orbitPlusStatus, err := worker.orbitPlusClient.SendTripDetails(ctx, TripDetailsRefreshRequest{
		Message:      message,
		BitsResponse: append([]byte(nil), sourceResult.Body...),
	})
	if err != nil {
		slog.Info("OrbitPlus TripDetails push completed", "actionType", message.ActionType, "operator", message.OperatorCode, "success", false)
		return "", err
	}
	slog.Info("OrbitPlus TripDetails push completed", "actionType", message.ActionType, "operator", message.OperatorCode, "success", true, "status", orbitPlusStatus)
	return orbitPlusStatus, nil
}

func (worker *TripDetailsRefreshWorker) operationError(status ExecutionStatus, err error, orbitPlusStatus ...OrbitPlusStatus) ExecutionResult {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return ExecutionResult{Status: ExecutionCancelled, Err: err}
	}
	result := ExecutionResult{Status: status, Err: err}
	if len(orbitPlusStatus) == 1 {
		result.OrbitPlusStatus = orbitPlusStatus[0]
	}
	return result
}

// Run handles deliveries with a bounded process-local goroutine pool. The pool
// caps only this Worker's concurrent fetch/submit operations; RabbitMQ controls
// redelivery and DLQ behavior, and OrbitPlus remains the cross-instance authority.
func (worker *TripDetailsRefreshWorker) Run(ctx context.Context) error {
	deliveries, err := worker.consumer.Consume(ctx)
	if err != nil {
		return err
	}

	var workerGroup sync.WaitGroup
	workerGroup.Add(worker.config.WorkerConcurrency)
	for range worker.config.WorkerConcurrency {
		go func() {
			defer workerGroup.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case delivery, open := <-deliveries:
					if !open {
						return
					}
					worker.Handle(ctx, delivery)
				}
			}
		}()
	}
	workerGroup.Wait()
	return ctx.Err()
}
