package cassandra

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/gocql/gocql"

	"orbitplusmaster/internal/application/master"
	"orbitplusmaster/internal/domain"
)

const operatorsTable = "operators"

// RegisterOperator preserves an existing active state and records its zone code.
func (repository *QueueMetrixRepository) RegisterOperator(ctx context.Context, operatorCode, zoneCode string) (domain.Operator, error) {
	operatorCode, err := validOperatorCode(operatorCode)
	if err != nil {
		return domain.Operator{}, err
	}
	zoneCode, err = validOperatorZoneCode(zoneCode)
	if err != nil {
		return domain.Operator{}, err
	}
	operator, _, found, err := repository.operatorByCode(ctx, operatorCode)
	if err != nil {
		return domain.Operator{}, err
	}
	if found {
		return repository.updateOperatorZone(ctx, operator, zoneCode)
	}
	now := time.Now().UTC()
	query := `INSERT INTO ` + operatorsTable + ` (active_flag, operator_code, zone_code, created_at, updated_at) VALUES (1, ?, ?, ?, ?) IF NOT EXISTS`
	if err := repository.session.Query(query, operatorCode, zoneCode, now, now).WithContext(ctx).Exec(); err != nil {
		return domain.Operator{}, operatorRegistryUnavailable("register operator", err)
	}
	operator, _, found, err = repository.operatorByCode(ctx, operatorCode)
	if err != nil {
		return domain.Operator{}, err
	}
	if !found {
		return domain.Operator{}, fmt.Errorf("registered operator was not found")
	}
	return repository.updateOperatorZone(ctx, operator, zoneCode)
}

// ListActiveOperators returns only operators in the active Cassandra partition.
func (repository *QueueMetrixRepository) ListActiveOperators(ctx context.Context) ([]domain.Operator, error) {
	return repository.listOperatorsInPartition(ctx, 1)
}

// ListOperators returns all operators from the active and inactive partitions.
func (repository *QueueMetrixRepository) ListOperators(ctx context.Context) ([]domain.Operator, error) {
	active, err := repository.listOperatorsInPartition(ctx, 1)
	if err != nil {
		return nil, err
	}
	inactive, err := repository.listOperatorsInPartition(ctx, 0)
	if err != nil {
		return nil, err
	}
	operators := append(active, inactive...)
	sort.Slice(operators, func(left, right int) bool { return operators[left].Code < operators[right].Code })
	return operators, nil
}

// SetOperatorActive moves an operator between the active and inactive partitions.
func (repository *QueueMetrixRepository) SetOperatorActive(ctx context.Context, operatorCode string, active bool) (domain.Operator, error) {
	operatorCode, err := validOperatorCode(operatorCode)
	if err != nil {
		return domain.Operator{}, err
	}
	operator, createdAt, found, err := repository.operatorByCode(ctx, operatorCode)
	if err != nil {
		return domain.Operator{}, err
	}
	if !found {
		return domain.Operator{}, master.ErrOperatorNotFound
	}
	targetFlag := 0
	if active {
		targetFlag = 1
	}
	if operator.ActiveFlag == targetFlag {
		return operator, nil
	}
	now := time.Now().UTC()
	if createdAt.IsZero() {
		createdAt = now
	}
	insert := `INSERT INTO ` + operatorsTable + ` (active_flag, operator_code, zone_code, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`
	if err := repository.session.Query(insert, targetFlag, operatorCode, operator.ZoneCode, createdAt, now).WithContext(ctx).Exec(); err != nil {
		return domain.Operator{}, operatorRegistryUnavailable("set operator status", err)
	}
	deleteQuery := `DELETE FROM ` + operatorsTable + ` WHERE active_flag=? AND operator_code=?`
	if err := repository.session.Query(deleteQuery, operator.ActiveFlag, operatorCode).WithContext(ctx).Exec(); err != nil {
		return domain.Operator{}, operatorRegistryUnavailable("complete operator status change", err)
	}
	return domain.Operator{Code: operatorCode, ZoneCode: operator.ZoneCode, ActiveFlag: targetFlag}, nil
}

func (repository *QueueMetrixRepository) updateOperatorZone(ctx context.Context, operator domain.Operator, zoneCode string) (domain.Operator, error) {
	query := `UPDATE ` + operatorsTable + ` SET zone_code=?, updated_at=? WHERE active_flag=? AND operator_code=?`
	if err := repository.session.Query(query, zoneCode, time.Now().UTC(), operator.ActiveFlag, operator.Code).WithContext(ctx).Exec(); err != nil {
		return domain.Operator{}, operatorRegistryUnavailable("update operator zone", err)
	}
	operator.ZoneCode = zoneCode
	return operator, nil
}

func (repository *QueueMetrixRepository) listOperatorsInPartition(ctx context.Context, activeFlag int) ([]domain.Operator, error) {
	iter := repository.session.Query(`SELECT operator_code, zone_code FROM `+operatorsTable+` WHERE active_flag=?`, activeFlag).WithContext(ctx).Iter()
	operators := make([]domain.Operator, 0)
	for {
		var code string
		var zoneCode string
		if !iter.Scan(&code, &zoneCode) {
			break
		}
		operators = append(operators, domain.Operator{Code: code, ZoneCode: zoneCode, ActiveFlag: activeFlag})
	}
	if err := iter.Close(); err != nil {
		return nil, operatorRegistryUnavailable("list operators", err)
	}
	return operators, nil
}

func (repository *QueueMetrixRepository) operatorByCode(ctx context.Context, code string) (domain.Operator, time.Time, bool, error) {
	for _, activeFlag := range []int{1, 0} {
		var storedCode string
		var zoneCode string
		var createdAt time.Time
		err := repository.session.Query(`SELECT operator_code, zone_code, created_at FROM `+operatorsTable+` WHERE active_flag=? AND operator_code=?`, activeFlag, code).WithContext(ctx).Scan(&storedCode, &zoneCode, &createdAt)
		if err == nil {
			return domain.Operator{Code: storedCode, ZoneCode: zoneCode, ActiveFlag: activeFlag}, createdAt, true, nil
		}
		if !errors.Is(err, gocql.ErrNotFound) {
			return domain.Operator{}, time.Time{}, false, operatorRegistryUnavailable("find operator", err)
		}
	}
	return domain.Operator{}, time.Time{}, false, nil
}

func validOperatorCode(operatorCode string) (string, error) {
	operatorCode = strings.TrimSpace(operatorCode)
	if operatorCode == "" || len(operatorCode) > 128 {
		return "", master.ErrInvalidOperatorCode
	}
	return operatorCode, nil
}

func validOperatorZoneCode(zoneCode string) (string, error) {
	zoneCode, valid := master.NormalizeZoneCode(zoneCode)
	if !valid {
		return "", master.ErrInvalidOperatorZoneCode
	}
	return zoneCode, nil
}

func operatorRegistryUnavailable(operation string, err error) error {
	return fmt.Errorf("%w: %s: %v", master.ErrOperatorRegistryUnavailable, operation, err)
}
