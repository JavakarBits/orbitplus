package domain_test

import (
	"testing"

	"orbitplusworker/internal/domain"
)

func TestValidate_RequiresOperatorCodeAndActionType(t *testing.T) {
	tests := []struct {
		name    string
		message domain.TripDetailsRefreshMessage
		wantErr string
	}{
		{
			name:    "missing actionType",
			message: domain.TripDetailsRefreshMessage{OperatorCode: "OP1"},
			wantErr: "missing actionType",
		},
		{
			name:    "whitespace-only actionType",
			message: domain.TripDetailsRefreshMessage{OperatorCode: "OP1", ActionType: "   "},
			wantErr: "missing actionType",
		},
		{
			name:    "missing operatorCode",
			message: domain.TripDetailsRefreshMessage{ActionType: "search"},
			wantErr: "missing operatorCode",
		},
		{
			name:    "whitespace-only operatorCode",
			message: domain.TripDetailsRefreshMessage{ActionType: "search", OperatorCode: "  "},
			wantErr: "missing operatorCode",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.message.Validate()
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if err.Error() != tt.wantErr {
				t.Errorf("got %q, want %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestValidate_UnsupportedAction(t *testing.T) {
	message := domain.TripDetailsRefreshMessage{
		OperatorCode: "OP1",
		ActionType:   "unknown_action",
	}
	err := message.Validate()
	if err == nil {
		t.Fatal("expected error for unsupported actionType")
	}
	want := "unsupported actionType: unknown_action"
	if err.Error() != want {
		t.Errorf("got %q, want %q", err.Error(), want)
	}
}

func TestValidate_SearchRequiresFields(t *testing.T) {
	base := domain.TripDetailsRefreshMessage{
		OperatorCode: "OP1",
		ActionType:   "search",
		FromCode:     "FROM",
		ToCode:       "TO",
		TripDate:     "2026-08-20",
	}

	t.Run("valid search", func(t *testing.T) {
		if err := base.Validate(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	tests := []struct {
		name    string
		mutate  func(m *domain.TripDetailsRefreshMessage)
		wantErr string
	}{
		{
			name:    "missing fromCode",
			mutate:  func(m *domain.TripDetailsRefreshMessage) { m.FromCode = "" },
			wantErr: "missing fromCode for actionType: search",
		},
		{
			name:    "missing toCode",
			mutate:  func(m *domain.TripDetailsRefreshMessage) { m.ToCode = "" },
			wantErr: "missing toCode for actionType: search",
		},
		{
			name:    "missing tripDate",
			mutate:  func(m *domain.TripDetailsRefreshMessage) { m.TripDate = "" },
			wantErr: "missing tripDate for actionType: search",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := base
			tt.mutate(&msg)
			err := msg.Validate()
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if err.Error() != tt.wantErr {
				t.Errorf("got %q, want %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestValidate_SearchbusmapRequiresFields(t *testing.T) {
	base := domain.TripDetailsRefreshMessage{
		OperatorCode: "OP1",
		ActionType:   "searchbusmap",
		FromCode:     "FROM",
		ToCode:       "TO",
		TripDate:     "2026-08-20",
	}

	t.Run("valid searchbusmap", func(t *testing.T) {
		if err := base.Validate(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	tests := []struct {
		name    string
		mutate  func(m *domain.TripDetailsRefreshMessage)
		wantErr string
	}{
		{
			name:    "missing fromCode",
			mutate:  func(m *domain.TripDetailsRefreshMessage) { m.FromCode = "" },
			wantErr: "missing fromCode for actionType: searchbusmap",
		},
		{
			name:    "missing toCode",
			mutate:  func(m *domain.TripDetailsRefreshMessage) { m.ToCode = "" },
			wantErr: "missing toCode for actionType: searchbusmap",
		},
		{
			name:    "missing tripDate",
			mutate:  func(m *domain.TripDetailsRefreshMessage) { m.TripDate = "" },
			wantErr: "missing tripDate for actionType: searchbusmap",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := base
			tt.mutate(&msg)
			err := msg.Validate()
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if err.Error() != tt.wantErr {
				t.Errorf("got %q, want %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestValidate_BusmapRequiresFields(t *testing.T) {
	base := domain.TripDetailsRefreshMessage{
		OperatorCode:    "OP1",
		ActionType:      "busmap",
		TripCode:        "TRIP123",
		FromStationCode: "STN_FROM",
		ToStationCode:   "STN_TO",
		TravelDate:      "2026-08-20",
	}

	t.Run("valid busmap", func(t *testing.T) {
		if err := base.Validate(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	tests := []struct {
		name    string
		mutate  func(m *domain.TripDetailsRefreshMessage)
		wantErr string
	}{
		{
			name:    "missing tripCode",
			mutate:  func(m *domain.TripDetailsRefreshMessage) { m.TripCode = "" },
			wantErr: "missing tripCode for actionType: busmap",
		},
		{
			name:    "missing fromStationCode",
			mutate:  func(m *domain.TripDetailsRefreshMessage) { m.FromStationCode = "" },
			wantErr: "missing fromStationCode for actionType: busmap",
		},
		{
			name:    "missing toStationCode",
			mutate:  func(m *domain.TripDetailsRefreshMessage) { m.ToStationCode = "" },
			wantErr: "missing toStationCode for actionType: busmap",
		},
		{
			name:    "missing travelDate",
			mutate:  func(m *domain.TripDetailsRefreshMessage) { m.TravelDate = "" },
			wantErr: "missing travelDate for actionType: busmap",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := base
			tt.mutate(&msg)
			err := msg.Validate()
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if err.Error() != tt.wantErr {
				t.Errorf("got %q, want %q", err.Error(), tt.wantErr)
			}
		})
	}
}
