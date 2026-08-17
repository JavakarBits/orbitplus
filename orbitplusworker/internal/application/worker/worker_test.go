package worker_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"orbitplusworker/internal/application/worker"
	"orbitplusworker/internal/domain"
)

// --- Test doubles ---

type stubDelivery struct {
	payload []byte
	acked   bool
	ackErr  error
}

func (d *stubDelivery) Payload() []byte { return append([]byte(nil), d.payload...) }
func (d *stubDelivery) Ack(_ context.Context) error {
	if d.ackErr != nil {
		return d.ackErr
	}
	d.acked = true
	return nil
}

type stubBitsClient struct {
	response worker.BitsTripDetailsResponse
	err      error
	called   bool
	request  worker.BitsTripDetailsRequest
}

func (c *stubBitsClient) FetchTripDetails(_ context.Context, req worker.BitsTripDetailsRequest) (worker.BitsTripDetailsResponse, error) {
	c.called = true
	c.request = req
	return c.response, c.err
}

type stubOrbitPlusClient struct {
	status  worker.OrbitPlusStatus
	err     error
	called  bool
	request worker.TripDetailsRefreshRequest
}

func (c *stubOrbitPlusClient) SendTripDetails(_ context.Context, req worker.TripDetailsRefreshRequest) (worker.OrbitPlusStatus, error) {
	c.called = true
	c.request = req
	return c.status, c.err
}

type stubConsumer struct {
	deliveries chan worker.RabbitMQDelivery
}

func (c *stubConsumer) Consume(_ context.Context) (<-chan worker.RabbitMQDelivery, error) {
	return c.deliveries, nil
}

// --- Helper ---

func newTestWorker(bitsClient *stubBitsClient, orbitPlusClient *stubOrbitPlusClient) *worker.TripDetailsRefreshWorker {
	consumer := &stubConsumer{deliveries: make(chan worker.RabbitMQDelivery)}
	w, err := worker.NewTripDetailsRefreshWorker(
		worker.WorkerConfig{WorkerConcurrency: 1},
		"http://localhost:8081",
		consumer,
		bitsClient,
		orbitPlusClient,
	)
	if err != nil {
		panic(fmt.Sprintf("failed to create worker: %v", err))
	}
	return w
}

func makeDelivery(msg domain.TripDetailsRefreshMessage) *stubDelivery {
	payload, _ := json.Marshal(msg)
	return &stubDelivery{payload: payload}
}

// --- Tests: Valid search flow ---

func TestHandle_ValidSearch_AcceptedAcks(t *testing.T) {
	bitsResponse := []byte(`{"status":1,"data":[{"tripCode":"T1"}]}`)
	bitsClient := &stubBitsClient{response: worker.BitsTripDetailsResponse{Body: bitsResponse}}
	orbitPlusClient := &stubOrbitPlusClient{status: worker.OrbitPlusAccepted}
	w := newTestWorker(bitsClient, orbitPlusClient)

	delivery := makeDelivery(domain.TripDetailsRefreshMessage{
		ActionType:   "search",
		OperatorCode: "OP1",
		FromCode:     "FROM",
		ToCode:       "TO",
		TripDate:     "2026-08-20",
	})

	result := w.Handle(context.Background(), delivery)

	if result.Status != worker.ExecutionAcknowledged {
		t.Fatalf("expected ACKNOWLEDGED, got %s", result.Status)
	}
	if !result.Acknowledged {
		t.Fatal("expected Acknowledged=true")
	}
	if result.OrbitPlusStatus != worker.OrbitPlusAccepted {
		t.Fatalf("expected ACCEPTED, got %s", result.OrbitPlusStatus)
	}
	if !delivery.acked {
		t.Fatal("expected delivery to be acked")
	}
	if !bitsClient.called {
		t.Fatal("expected Bits client to be called")
	}
	if !orbitPlusClient.called {
		t.Fatal("expected OrbitPlus client to be called")
	}
	// Verify raw Bits response is forwarded unchanged
	if string(orbitPlusClient.request.BitsResponse) != string(bitsResponse) {
		t.Errorf("BitsResponse mismatch: got %q, want %q", orbitPlusClient.request.BitsResponse, bitsResponse)
	}
}

// --- Tests: Valid busmap flow ---

func TestHandle_ValidBusmap_AcceptedAcks(t *testing.T) {
	bitsResponse := []byte(`{"status":1,"data":[{"seatLayout":[]}]}`)
	bitsClient := &stubBitsClient{response: worker.BitsTripDetailsResponse{Body: bitsResponse}}
	orbitPlusClient := &stubOrbitPlusClient{status: worker.OrbitPlusAccepted}
	w := newTestWorker(bitsClient, orbitPlusClient)

	delivery := makeDelivery(domain.TripDetailsRefreshMessage{
		ActionType:      "busmap",
		OperatorCode:    "OP1",
		TripCode:        "TRIP123",
		FromStationCode: "STN_FROM",
		ToStationCode:   "STN_TO",
		TravelDate:      "2026-08-20",
	})

	result := w.Handle(context.Background(), delivery)

	if result.Status != worker.ExecutionAcknowledged {
		t.Fatalf("expected ACKNOWLEDGED, got %s", result.Status)
	}
	if !delivery.acked {
		t.Fatal("expected delivery to be acked")
	}
	if string(orbitPlusClient.request.BitsResponse) != string(bitsResponse) {
		t.Errorf("BitsResponse mismatch: got %q, want %q", orbitPlusClient.request.BitsResponse, bitsResponse)
	}
}

// --- Tests: Valid searchbusmap flow ---

func TestHandle_ValidSearchbusmap_AcceptedAcks(t *testing.T) {
	bitsResponse := []byte(`{"status":1,"data":[{"combined":true}]}`)
	bitsClient := &stubBitsClient{response: worker.BitsTripDetailsResponse{Body: bitsResponse}}
	orbitPlusClient := &stubOrbitPlusClient{status: worker.OrbitPlusAccepted}
	w := newTestWorker(bitsClient, orbitPlusClient)

	delivery := makeDelivery(domain.TripDetailsRefreshMessage{
		ActionType:   "searchbusmap",
		OperatorCode: "OP1",
		FromCode:     "FROM",
		ToCode:       "TO",
		TripDate:     "2026-08-20",
	})

	result := w.Handle(context.Background(), delivery)

	if result.Status != worker.ExecutionAcknowledged {
		t.Fatalf("expected ACKNOWLEDGED, got %s", result.Status)
	}
	if !delivery.acked {
		t.Fatal("expected delivery to be acked")
	}
	if string(orbitPlusClient.request.BitsResponse) != string(bitsResponse) {
		t.Errorf("BitsResponse mismatch: got %q, want %q", orbitPlusClient.request.BitsResponse, bitsResponse)
	}
}

// --- Tests: Invalid messages do not call Bits or OrbitPlus ---

func TestHandle_InvalidJSON_NoBitsOrOrbitPlusCall(t *testing.T) {
	bitsClient := &stubBitsClient{}
	orbitPlusClient := &stubOrbitPlusClient{}
	w := newTestWorker(bitsClient, orbitPlusClient)

	delivery := &stubDelivery{payload: []byte(`{not valid json`)}
	result := w.Handle(context.Background(), delivery)

	if result.Status != worker.ExecutionInvalidEnvelope {
		t.Fatalf("expected INVALID_ENVELOPE, got %s", result.Status)
	}
	if bitsClient.called {
		t.Fatal("Bits client should not be called for invalid JSON")
	}
	if orbitPlusClient.called {
		t.Fatal("OrbitPlus client should not be called for invalid JSON")
	}
	if delivery.acked {
		t.Fatal("delivery should not be acked for invalid JSON")
	}
}

func TestHandle_MissingRequiredField_NoBitsOrOrbitPlusCall(t *testing.T) {
	bitsClient := &stubBitsClient{}
	orbitPlusClient := &stubOrbitPlusClient{}
	w := newTestWorker(bitsClient, orbitPlusClient)

	// search missing fromCode
	delivery := makeDelivery(domain.TripDetailsRefreshMessage{
		ActionType:   "search",
		OperatorCode: "OP1",
		ToCode:       "TO",
		TripDate:     "2026-08-20",
	})
	result := w.Handle(context.Background(), delivery)

	if result.Status != worker.ExecutionInvalidEnvelope {
		t.Fatalf("expected INVALID_ENVELOPE, got %s", result.Status)
	}
	if bitsClient.called {
		t.Fatal("Bits client should not be called for missing required field")
	}
	if orbitPlusClient.called {
		t.Fatal("OrbitPlus client should not be called for missing required field")
	}
	if delivery.acked {
		t.Fatal("delivery should not be acked for missing required field")
	}
}

func TestHandle_UnsupportedAction_NoBitsOrOrbitPlusCall(t *testing.T) {
	bitsClient := &stubBitsClient{}
	orbitPlusClient := &stubOrbitPlusClient{}
	w := newTestWorker(bitsClient, orbitPlusClient)

	delivery := makeDelivery(domain.TripDetailsRefreshMessage{
		ActionType:   "unsupported_action",
		OperatorCode: "OP1",
	})
	result := w.Handle(context.Background(), delivery)

	if result.Status != worker.ExecutionInvalidEnvelope {
		t.Fatalf("expected INVALID_ENVELOPE, got %s", result.Status)
	}
	if bitsClient.called {
		t.Fatal("Bits client should not be called for unsupported action")
	}
	if orbitPlusClient.called {
		t.Fatal("OrbitPlus client should not be called for unsupported action")
	}
	if delivery.acked {
		t.Fatal("delivery should not be acked for unsupported action")
	}
}

// --- Tests: Bits failure leaves unacknowledged ---

func TestHandle_BitsError_LeavesUnacknowledged(t *testing.T) {
	bitsClient := &stubBitsClient{err: fmt.Errorf("Bits returned HTTP 500")}
	orbitPlusClient := &stubOrbitPlusClient{}
	w := newTestWorker(bitsClient, orbitPlusClient)

	delivery := makeDelivery(domain.TripDetailsRefreshMessage{
		ActionType:   "search",
		OperatorCode: "OP1",
		FromCode:     "FROM",
		ToCode:       "TO",
		TripDate:     "2026-08-20",
	})
	result := w.Handle(context.Background(), delivery)

	if result.Status != worker.ExecutionSourceError {
		t.Fatalf("expected SOURCE_ERROR, got %s", result.Status)
	}
	if orbitPlusClient.called {
		t.Fatal("OrbitPlus client should not be called after Bits failure")
	}
	if delivery.acked {
		t.Fatal("delivery should not be acked after Bits failure")
	}
}

func TestHandle_BitsEmptyResponse_LeavesUnacknowledged(t *testing.T) {
	bitsClient := &stubBitsClient{response: worker.BitsTripDetailsResponse{Body: []byte{}}}
	orbitPlusClient := &stubOrbitPlusClient{}
	w := newTestWorker(bitsClient, orbitPlusClient)

	delivery := makeDelivery(domain.TripDetailsRefreshMessage{
		ActionType:   "search",
		OperatorCode: "OP1",
		FromCode:     "FROM",
		ToCode:       "TO",
		TripDate:     "2026-08-20",
	})
	result := w.Handle(context.Background(), delivery)

	if result.Status != worker.ExecutionSourceError {
		t.Fatalf("expected SOURCE_ERROR, got %s", result.Status)
	}
	if orbitPlusClient.called {
		t.Fatal("OrbitPlus client should not be called after empty Bits response")
	}
	if delivery.acked {
		t.Fatal("delivery should not be acked after empty Bits response")
	}
}

// --- Tests: OrbitPlus failure leaves unacknowledged ---

func TestHandle_OrbitPlusError_LeavesUnacknowledged(t *testing.T) {
	bitsClient := &stubBitsClient{response: worker.BitsTripDetailsResponse{Body: []byte(`{"data":1}`)}}
	orbitPlusClient := &stubOrbitPlusClient{err: fmt.Errorf("OrbitPlus request: connection refused")}
	w := newTestWorker(bitsClient, orbitPlusClient)

	delivery := makeDelivery(domain.TripDetailsRefreshMessage{
		ActionType:   "search",
		OperatorCode: "OP1",
		FromCode:     "FROM",
		ToCode:       "TO",
		TripDate:     "2026-08-20",
	})
	result := w.Handle(context.Background(), delivery)

	if result.Status != worker.ExecutionOrbitPlusError {
		t.Fatalf("expected ORBITPLUS_ERROR, got %s", result.Status)
	}
	if delivery.acked {
		t.Fatal("delivery should not be acked after OrbitPlus error")
	}
}

func TestHandle_OrbitPlusRetryable_LeavesUnacknowledged(t *testing.T) {
	bitsClient := &stubBitsClient{response: worker.BitsTripDetailsResponse{Body: []byte(`{"data":1}`)}}
	orbitPlusClient := &stubOrbitPlusClient{status: worker.OrbitPlusRetryable}
	w := newTestWorker(bitsClient, orbitPlusClient)

	delivery := makeDelivery(domain.TripDetailsRefreshMessage{
		ActionType:   "search",
		OperatorCode: "OP1",
		FromCode:     "FROM",
		ToCode:       "TO",
		TripDate:     "2026-08-20",
	})
	result := w.Handle(context.Background(), delivery)

	if result.Status != worker.ExecutionOrbitPlusOutcome {
		t.Fatalf("expected ORBITPLUS_OUTCOME, got %s", result.Status)
	}
	if result.OrbitPlusStatus != worker.OrbitPlusRetryable {
		t.Fatalf("expected RETRYABLE, got %s", result.OrbitPlusStatus)
	}
	if delivery.acked {
		t.Fatal("delivery should not be acked for retryable outcome")
	}
}

// --- Tests: ACK failure leaves unacknowledged ---

func TestHandle_AckError_LeavesUnacknowledged(t *testing.T) {
	bitsClient := &stubBitsClient{response: worker.BitsTripDetailsResponse{Body: []byte(`{"data":1}`)}}
	orbitPlusClient := &stubOrbitPlusClient{status: worker.OrbitPlusAccepted}
	w := newTestWorker(bitsClient, orbitPlusClient)

	delivery := makeDelivery(domain.TripDetailsRefreshMessage{
		ActionType:   "search",
		OperatorCode: "OP1",
		FromCode:     "FROM",
		ToCode:       "TO",
		TripDate:     "2026-08-20",
	})
	delivery.ackErr = fmt.Errorf("channel closed")

	result := w.Handle(context.Background(), delivery)

	if result.Status != worker.ExecutionAckError {
		t.Fatalf("expected ACK_ERROR, got %s", result.Status)
	}
	if result.Acknowledged {
		t.Fatal("Acknowledged should be false on ack error")
	}
}

// --- Tests: Cancelled context ---

func TestHandle_CancelledContext_ReturnsCancelled(t *testing.T) {
	bitsClient := &stubBitsClient{}
	orbitPlusClient := &stubOrbitPlusClient{}
	w := newTestWorker(bitsClient, orbitPlusClient)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	delivery := makeDelivery(domain.TripDetailsRefreshMessage{
		ActionType:   "search",
		OperatorCode: "OP1",
		FromCode:     "FROM",
		ToCode:       "TO",
		TripDate:     "2026-08-20",
	})
	result := w.Handle(ctx, delivery)

	if result.Status != worker.ExecutionCancelled {
		t.Fatalf("expected CANCELLED, got %s", result.Status)
	}
	if bitsClient.called {
		t.Fatal("Bits client should not be called when context is cancelled")
	}
	if delivery.acked {
		t.Fatal("delivery should not be acked when context is cancelled")
	}
}

// --- Tests: ACK only for ACCEPTED (not DUPLICATE/STALE) ---

func TestAcknowledgementEligible_OnlyAccepted(t *testing.T) {
	tests := []struct {
		status   worker.OrbitPlusStatus
		eligible bool
	}{
		{worker.OrbitPlusAccepted, true},
		{worker.OrbitPlusDuplicate, false},
		{worker.OrbitPlusStale, false},
		{worker.OrbitPlusRetryable, false},
		{worker.OrbitPlusStatus("UNKNOWN"), false},
	}
	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			if got := tt.status.AcknowledgementEligible(); got != tt.eligible {
				t.Errorf("AcknowledgementEligible() = %v, want %v", got, tt.eligible)
			}
		})
	}
}

// --- Tests: Bits response forwarded unchanged ---

func TestHandle_BitsResponseForwardedUnchanged(t *testing.T) {
	// Include special characters to verify no transformation
	bitsResponse := []byte(`{"status":1,"data":[{"fare":2999,"seatType":"LSL","special":"café & naïve <tag>"}]}`)
	bitsClient := &stubBitsClient{response: worker.BitsTripDetailsResponse{Body: bitsResponse}}
	orbitPlusClient := &stubOrbitPlusClient{status: worker.OrbitPlusAccepted}
	w := newTestWorker(bitsClient, orbitPlusClient)

	delivery := makeDelivery(domain.TripDetailsRefreshMessage{
		ActionType:   "search",
		OperatorCode: "OP1",
		FromCode:     "FROM",
		ToCode:       "TO",
		TripDate:     "2026-08-20",
	})

	w.Handle(context.Background(), delivery)

	if string(orbitPlusClient.request.BitsResponse) != string(bitsResponse) {
		t.Errorf("BitsResponse was modified: got %q, want %q", orbitPlusClient.request.BitsResponse, bitsResponse)
	}
}

// --- Tests: Message identity forwarded to OrbitPlus ---

func TestHandle_MessageIdentityForwardedToOrbitPlus(t *testing.T) {
	bitsClient := &stubBitsClient{response: worker.BitsTripDetailsResponse{Body: []byte(`{"data":1}`)}}
	orbitPlusClient := &stubOrbitPlusClient{status: worker.OrbitPlusAccepted}
	w := newTestWorker(bitsClient, orbitPlusClient)

	msg := domain.TripDetailsRefreshMessage{
		ActionType:   "busmap",
		OperatorCode: "OPERATOR_X",
		TripCode:     "TRIP_ABC",
		FromStationCode: "STN_A",
		ToStationCode:   "STN_B",
		TravelDate:      "2026-09-15",
	}
	delivery := makeDelivery(msg)

	w.Handle(context.Background(), delivery)

	if orbitPlusClient.request.Message.ActionType != msg.ActionType {
		t.Errorf("ActionType: got %q, want %q", orbitPlusClient.request.Message.ActionType, msg.ActionType)
	}
	if orbitPlusClient.request.Message.OperatorCode != msg.OperatorCode {
		t.Errorf("OperatorCode: got %q, want %q", orbitPlusClient.request.Message.OperatorCode, msg.OperatorCode)
	}
	if orbitPlusClient.request.Message.TripCode != msg.TripCode {
		t.Errorf("TripCode: got %q, want %q", orbitPlusClient.request.Message.TripCode, msg.TripCode)
	}
}
