package cassandra

import (
	"context"
	"fmt"

	"github.com/gocql/gocql"
)

// CqlReadRepository runs read-only CQL queries against the configured keyspace
// and returns generic rows. It backs the admin "CQL query" page, which is
// restricted to SELECT statements by the application layer; this repository
// only executes whatever query it is given, so the SELECT-only guarantee must
// be enforced before calling Query.
type CqlReadRepository struct {
	session *gocql.Session
}

// NewCqlReadRepository opens a dedicated session for ad-hoc read queries.
func NewCqlReadRepository(ctx context.Context, config Config) (*CqlReadRepository, error) {
	if !validIdentifier(config.Keyspace) {
		return nil, fmt.Errorf("invalid Cassandra keyspace")
	}
	cluster := gocql.NewCluster(config.Hosts...)
	cluster.Port = config.Port
	cluster.Keyspace = config.Keyspace
	cluster.Timeout = config.Timeout
	cluster.ConnectTimeout = config.Timeout
	if config.Username != "" || config.Password != "" {
		cluster.Authenticator = gocql.PasswordAuthenticator{Username: config.Username, Password: config.Password}
	}
	session, err := cluster.CreateSession()
	if err != nil {
		return nil, fmt.Errorf("connect to Cassandra: %w", err)
	}
	return &CqlReadRepository{session: session}, nil
}

// Query runs the given CQL and returns the ordered column names and the rows as
// maps of column name to value. maxRows caps how many rows are returned so a
// large table cannot flood the caller; when the cap is hit, truncated is true.
// The caller is responsible for ensuring the statement is a read-only SELECT.
func (repository *CqlReadRepository) Query(ctx context.Context, cql string, maxRows int) (columns []string, rows []map[string]any, truncated bool, err error) {
	if repository == nil || repository.session == nil {
		return nil, nil, false, fmt.Errorf("cql read repository is not configured")
	}
	if maxRows <= 0 {
		maxRows = 1000
	}
	iter := repository.session.Query(cql).WithContext(ctx).Iter()

	columnInfos := iter.Columns()
	columns = make([]string, 0, len(columnInfos))
	for _, column := range columnInfos {
		columns = append(columns, column.Name)
	}

	rows = make([]map[string]any, 0)
	for {
		row := make(map[string]any)
		if !iter.MapScan(row) {
			break
		}
		rows = append(rows, normalizeRow(row))
		if len(rows) >= maxRows {
			truncated = true
			break
		}
	}
	if closeErr := iter.Close(); closeErr != nil {
		return nil, nil, false, fmt.Errorf("run cql query: %w", closeErr)
	}
	return columns, rows, truncated, nil
}

// Close releases the Cassandra connection.
func (repository *CqlReadRepository) Close() {
	if repository != nil && repository.session != nil {
		repository.session.Close()
	}
}

// normalizeRow converts driver-specific value types into JSON-friendly forms so
// the HTTP layer can marshal them cleanly (e.g. []byte -> string, UUID ->
// string). Values that already marshal well are left as-is.
func normalizeRow(row map[string]any) map[string]any {
	for key, value := range row {
		switch typed := value.(type) {
		case []byte:
			row[key] = string(typed)
		case gocql.UUID:
			row[key] = typed.String()
		}
	}
	return row
}
