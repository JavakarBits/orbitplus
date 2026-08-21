package master

import (
	"context"
	"fmt"

	"orbitplusmaster/internal/domain"
)

// TripDetailsMetadataRepository stores Search metadata in Cassandra.
type TripDetailsMetadataRepository interface {
	SaveRouteMetadata(ctx context.Context, metadata domain.TripDetailsStageMetadata) error
	SaveScheduleMetadata(ctx context.Context, metadata domain.TripDetailsStageMetadata) error
}

func (persistence *TripDetailsStorage) storeSearchMetadata(ctx context.Context, operatorCode, tripCode, tripStageCode, tripDate, fromStationCode, toStationCode string, entry map[string]any, index int) error {
	metadata := domain.TripDetailsStageMetadata{
		OperatorCode:    operatorCode,
		TripCode:        tripCode,
		ScheduleCode:    scheduleCodeFromEntry(entry),
		TripDate:        tripDate,
		FromStationCode: fromStationCode,
		ToStationCode:   toStationCode,
		TripStageCode:   tripStageCode,
		UpdatedAt:       persistence.now().UTC(),
	}
	for _, record := range stageMetadataRecords(metadata, entry) {
		if err := persistence.metadata.SaveRouteMetadata(ctx, record); err != nil {
			return fmt.Errorf("store TripDetails route metadata: %w", err)
		}
	}
	if metadata.ScheduleCode != "" {
		if err := persistence.metadata.SaveScheduleMetadata(ctx, metadata); err != nil {
			return fmt.Errorf("store TripDetails schedule metadata: %w", err)
		}
	}
	persistence.logger.Printf("TripDetails Cassandra metadata write succeeded entry=%d tripStageCode=%q scheduleCode=%q", index, tripStageCode, metadata.ScheduleCode)
	return nil
}

func scheduleCodeFromEntry(entry map[string]any) string {
	for _, key := range []string{"scheduleCode", "schedulecode"} {
		if value := stringField(entry, key); value != "" {
			return value
		}
	}
	return nestedStringField(entry, "schedule", "code")
}

func stageMetadataRecords(base domain.TripDetailsStageMetadata, entry map[string]any) []domain.TripDetailsStageMetadata {
	records := []domain.TripDetailsStageMetadata{base}
	seen := map[string]struct{}{base.FromStationCode + "\x00" + base.ToStationCode: {}}
	stations := stationCodesFromValue(entry)
	for fromIndex, fromCode := range stations {
		for _, toCode := range stations[fromIndex+1:] {
			key := fromCode + "\x00" + toCode
			if fromCode == "" || toCode == "" {
				continue
			}
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}
			record := base
			record.FromStationCode = fromCode
			record.ToStationCode = toCode
			records = append(records, record)
		}
	}
	return records
}
