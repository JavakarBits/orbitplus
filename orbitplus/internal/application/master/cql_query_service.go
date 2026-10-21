package master

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

var (
	// ErrCqlNotConfigured means no Cassandra read session is available.
	ErrCqlNotConfigured = errors.New("cql query is not configured")
	// ErrCqlEmpty means the submitted query was blank.
	ErrCqlEmpty = errors.New("cql query is empty")
	// ErrCqlNotSelect means the submitted query is not a single read-only SELECT.
	ErrCqlNotSelect = errors.New("only a single SELECT statement is allowed")
)

// cqlMaxRows caps how many rows a single query returns to the browser.
const cqlMaxRows = 1000

// CqlReader runs a validated read-only CQL query and returns generic rows.
// *cassandra.CqlReadRepository implements it.
type CqlReader interface {
	Query(ctx context.Context, cql string, maxRows int) (columns []string, rows []map[string]any, truncated bool, err error)
}

// CqlQueryResult is the outcome of one admin CQL query.
type CqlQueryResult struct {
	Columns   []string
	Rows      []map[string]any
	Truncated bool
	MaxRows   int
}

// CqlQueryService runs ad-hoc, read-only CQL for the admin UI. It enforces that
// only a single SELECT statement runs, so the page can never mutate or drop
// data regardless of what is typed.
type CqlQueryService struct {
	reader CqlReader
}

// NewCqlQueryService constructs the service. A nil reader disables the feature,
// so every call reports ErrCqlNotConfigured.
func NewCqlQueryService(reader CqlReader) *CqlQueryService {
	return &CqlQueryService{reader: reader}
}

// Run validates that cql is a single SELECT and executes it, returning at most
// cqlMaxRows rows.
func (service *CqlQueryService) Run(ctx context.Context, cql string) (CqlQueryResult, error) {
	if service == nil || service.reader == nil {
		return CqlQueryResult{}, ErrCqlNotConfigured
	}
	normalized, err := validateSelectOnly(cql)
	if err != nil {
		return CqlQueryResult{}, err
	}
	columns, rows, truncated, err := service.reader.Query(ctx, normalized, cqlMaxRows)
	if err != nil {
		return CqlQueryResult{}, err
	}
	return CqlQueryResult{Columns: columns, Rows: rows, Truncated: truncated, MaxRows: cqlMaxRows}, nil
}

// disallowedKeywords are statement kinds that must never run from the query
// page. They are matched as whole words anywhere in the (comment-stripped)
// statement, so an attempt to smuggle a write past the SELECT prefix is still
// rejected.
var disallowedKeywords = []string{
	"insert", "update", "delete", "drop", "truncate", "alter",
	"create", "grant", "revoke", "use", "begin", "apply",
}

// validateSelectOnly returns the cleaned query if it is exactly one read-only
// SELECT statement, or an error explaining why it was rejected. It strips line
// and block comments, rejects multiple statements, requires a SELECT prefix,
// and rejects any DML/DDL keyword.
func validateSelectOnly(cql string) (string, error) {
	stripped := stripCQLComments(cql)
	trimmed := strings.TrimSpace(stripped)
	trimmed = strings.TrimRight(trimmed, ";")
	trimmed = strings.TrimSpace(trimmed)
	if trimmed == "" {
		return "", ErrCqlEmpty
	}
	// Reject multiple statements: after trimming a single trailing ';', no inner
	// ';' may remain.
	if strings.Contains(trimmed, ";") {
		return "", ErrCqlNotSelect
	}
	lower := strings.ToLower(trimmed)
	if !strings.HasPrefix(lower, "select") {
		return "", ErrCqlNotSelect
	}
	for _, keyword := range disallowedKeywords {
		if containsWord(lower, keyword) {
			return "", fmt.Errorf("%w: %q is not permitted", ErrCqlNotSelect, keyword)
		}
	}
	return trimmed, nil
}

// stripCQLComments removes -- line comments and /* */ block comments so a
// write statement cannot hide behind a comment.
func stripCQLComments(cql string) string {
	var builder strings.Builder
	runes := []rune(cql)
	for index := 0; index < len(runes); index++ {
		// Line comment: -- to end of line.
		if runes[index] == '-' && index+1 < len(runes) && runes[index+1] == '-' {
			for index < len(runes) && runes[index] != '\n' {
				index++
			}
			if index < len(runes) {
				builder.WriteRune('\n')
			}
			continue
		}
		// Block comment: /* ... */
		if runes[index] == '/' && index+1 < len(runes) && runes[index+1] == '*' {
			index += 2
			for index+1 < len(runes) && !(runes[index] == '*' && runes[index+1] == '/') {
				index++
			}
			index++ // skip closing '/'
			builder.WriteRune(' ')
			continue
		}
		builder.WriteRune(runes[index])
	}
	return builder.String()
}

// containsWord reports whether word appears in text as a whole word (not as a
// substring of a larger identifier).
func containsWord(text, word string) bool {
	for index := strings.Index(text, word); index >= 0; {
		before := index == 0 || !isIdentifierRune(rune(text[index-1]))
		afterIndex := index + len(word)
		after := afterIndex >= len(text) || !isIdentifierRune(rune(text[afterIndex]))
		if before && after {
			return true
		}
		next := strings.Index(text[index+1:], word)
		if next < 0 {
			return false
		}
		index = index + 1 + next
	}
	return false
}

func isIdentifierRune(character rune) bool {
	return character == '_' ||
		(character >= 'a' && character <= 'z') ||
		(character >= 'A' && character <= 'Z') ||
		(character >= '0' && character <= '9')
}
