package master

import (
	"context"
	"log"
	"strings"

	"orbitplusmaster/internal/domain"
)

// OperatorCredentialResolver resolves an operator's BITS credential from the
// Orbit credential service. *OrbitCredentialClient implements it.
type OperatorCredentialResolver interface {
	FetchOperatorCredential(ctx context.Context, operatorCode string) (domain.OperatorCredential, error)
}

// OperatorCredentialStore persists and reads resolved operator credentials.
// *cassandra.QueueMetrixRepository implements it.
type OperatorCredentialStore interface {
	SaveOperatorCredential(ctx context.Context, credential domain.OperatorCredential) error
	FindOperatorCredential(ctx context.Context, operatorCode string) (domain.OperatorCredential, bool, error)
}

// CredentialBackedOperatorRegistry decorates an OperatorRegistry so that every
// successful registration also resolves the operator's BITS credential from
// Orbit and stores it. This is the single place Orbit credentials are fetched,
// so both the Orionmax inventory path and the Operators UI path get credential
// resolution without each caller doing it.
//
// Credential resolution failures are logged but do not fail registration: the
// operator still exists for routing, and a worker credential ping can surface a
// missing credential later. Secret values are never logged.
type CredentialBackedOperatorRegistry struct {
	OperatorRegistry
	resolver OperatorCredentialResolver
	store    OperatorCredentialStore
	logger   *log.Logger
}

// NewCredentialBackedOperatorRegistry wraps base so RegisterOperator also
// resolves and stores the operator's credential. If resolver or store is nil,
// base is returned unchanged so the feature is simply off.
func NewCredentialBackedOperatorRegistry(base OperatorRegistry, resolver OperatorCredentialResolver, store OperatorCredentialStore, logger *log.Logger) OperatorRegistry {
	if base == nil || resolver == nil || store == nil {
		return base
	}
	if logger == nil {
		logger = log.Default()
	}
	return &CredentialBackedOperatorRegistry{OperatorRegistry: base, resolver: resolver, store: store, logger: logger}
}

// RegisterOperator registers the operator through the wrapped registry, then
// resolves and stores its BITS credential. A credential failure is non-fatal.
func (registry *CredentialBackedOperatorRegistry) RegisterOperator(ctx context.Context, operatorCode, zoneCode string) (domain.Operator, error) {
	operator, err := registry.OperatorRegistry.RegisterOperator(ctx, operatorCode, zoneCode)
	if err != nil {
		return operator, err
	}
	registry.resolveAndStore(ctx, operator)
	return operator, nil
}

func (registry *CredentialBackedOperatorRegistry) resolveAndStore(ctx context.Context, operator domain.Operator) {
	code := strings.TrimSpace(operator.Code)
	if code == "" {
		return
	}
	// Table-first: only call Orbit when this operator has no usable credential
	// stored yet. A usable row is one that exists and already carries both BITS
	// secrets and a zone URL. This keeps Orbit from being hit on every inventory
	// event for an operator whose credential is already known.
	if existing, found, err := registry.store.FindOperatorCredential(ctx, code); err != nil {
		registry.logger.Printf("operator credential lookup failed; will resolve from Orbit: operator_code=%q error=%v", code, err)
	} else if found && credentialUsable(existing) {
		registry.logger.Printf("operator credential already stored; skipping Orbit resolve: operator_code=%q zone_url=%q", code, existing.ZoneURL)
		return
	}
	credential, err := registry.resolver.FetchOperatorCredential(ctx, code)
	if err != nil {
		registry.logger.Printf("operator credential resolve failed: operator_code=%q error=%v", code, err)
		return
	}
	// The BITS zone URL comes from the Orbit credential response (operator
	// code-wise, via data.zone.name), not from any hardcoded zone table. Fall
	// back to the operator's registered zone code only when Orbit did not
	// supply a zone code of its own.
	if strings.TrimSpace(credential.ZoneCode) == "" {
		credential.ZoneCode = operator.ZoneCode
	}
	if err := registry.store.SaveOperatorCredential(ctx, credential); err != nil {
		registry.logger.Printf("operator credential store failed: operator_code=%q error=%v", code, err)
		return
	}
	registry.logger.Printf("operator credential stored: operator_code=%q zone_code=%q zone_url=%q active_flag=%d", code, operator.ZoneCode, credential.ZoneURL, credential.ActiveFlag)
}

// credentialUsable reports whether a stored credential is complete enough to
// serve a worker without re-resolving from Orbit: both BITS secrets and the
// operator's zone URL must be present.
func credentialUsable(credential domain.OperatorCredential) bool {
	return strings.TrimSpace(credential.Username) != "" &&
		strings.TrimSpace(credential.APIToken) != "" &&
		strings.TrimSpace(credential.ZoneURL) != ""
}
