package master

import (
	"context"
	"fmt"

	"orbitplusmaster/internal/domain"
)

// TripDetailsMetadataRepository stores Search Stage lookup metadata in Cassandra.
type TripDetailsMetadataRepository interface {
	SaveStageMetadata(ctx context.Context, metadata domain.TripDetailsStageMetadata) error
}

func (persistence *TripDetailsStorage) storeSearchMetadata(ctx context.Context, operatorCode, tripCode, tripStageCode, travelDate, fromStationCode, toStationCode string, entry map[string]any, index int) error {
	metadata := domain.TripDetailsStageMetadata{
		OperatorCode:    operatorCode,
		TripCode:        tripCode,
		TravelDate:      travelDate,
		FromStationCode: fromStationCode,
		ToStationCode:   toStationCode,
		TripStageCode:   tripStageCode,
		UpdatedAt:       persistence.now().UTC(),
	}
	for _, record := range stageMetadataRecords(metadata, entry) {
		if err := persistence.metadata.SaveStageMetadata(ctx, record); err != nil {
			return fmt.Errorf("store TripDetails metadata: %w", err)
		}
	}
	persistence.logger.Printf("TripDetails Cassandra metadata write succeeded entry=%d tripStageCode=%q", index, tripStageCode)
	return nil
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
