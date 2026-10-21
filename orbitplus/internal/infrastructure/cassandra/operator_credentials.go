package cassandra

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/gocql/gocql"

	"orbitplusmaster/internal/domain"
)

const operatorCredentialsTable = "operator_credentials"

// SaveOperatorCredential upserts one operator's resolved BITS credential. It is
// written at operator-registration time after the Master resolves the
// credential from the Orbit credential service.
func (repository *QueueMetrixRepository) SaveOperatorCredential(ctx context.Context, credential domain.OperatorCredential) error {
	operatorCode := strings.TrimSpace(credential.OperatorCode)
	if operatorCode == "" {
		return fmt.Errorf("operator credential requires an operator code")
	}
	query := `INSERT INTO ` + operatorCredentialsTable + `
		(operator_code, zone_code, zone_url, bits_username, bits_api_token, active_flag, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`
	if err := repository.session.Query(query, operatorCode, credential.ZoneCode, credential.ZoneURL, credential.Username,
		credential.APIToken, credential.ActiveFlag, time.Now().UTC()).WithContext(ctx).Exec(); err != nil {
		return fmt.Errorf("save operator credential: %w", err)
	}
	return nil
}

// FindOperatorCredential returns one operator's stored BITS credential and
// whether it exists.
func (repository *QueueMetrixRepository) FindOperatorCredential(ctx context.Context, operatorCode string) (domain.OperatorCredential, bool, error) {
	operatorCode = strings.TrimSpace(operatorCode)
	if operatorCode == "" {
		return domain.OperatorCredential{}, false, fmt.Errorf("operator credential lookup requires an operator code")
	}
	var credential domain.OperatorCredential
	var activeFlag int
	err := repository.session.Query(`SELECT operator_code, zone_code, zone_url, bits_username, bits_api_token, active_flag FROM `+
		operatorCredentialsTable+` WHERE operator_code=?`, operatorCode).WithContext(ctx).
		Scan(&credential.OperatorCode, &credential.ZoneCode, &credential.ZoneURL, &credential.Username, &credential.APIToken, &activeFlag)
	if errors.Is(err, gocql.ErrNotFound) {
		return domain.OperatorCredential{}, false, nil
	}
	if err != nil {
		return domain.OperatorCredential{}, false, fmt.Errorf("find operator credential: %w", err)
	}
	credential.ActiveFlag = activeFlag
	return credential, true, nil
}
