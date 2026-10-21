package worker

import "context"

// OperatorCredentialRequest identifies the operator whose Bits credential is
// resolved from OrbitService. It carries no secret material itself.
type OperatorCredentialRequest struct {
	OperatorCode string
}

// OperatorCredential is the credential material resolved for one operator. It
// is obtained from the OrbitPlus Master. ZoneURL is the operator's BITS host as
// stored by the Master; it is used as a fallback when the job message does not
// carry a zoneURL.
type OperatorCredential struct {
	OperatorCode string
	ZoneURL      string
	Username     string
	APIToken     string
}

// OperatorCredentialClient resolves per-operator Bits credentials. The worker
// obtains these from the OrbitPlus Master, which is the single component that
// talks to the Orbit credential service.
type OperatorCredentialClient interface {
	FetchOperatorCredential(ctx context.Context, request OperatorCredentialRequest) (OperatorCredential, error)
}

// CredentialInvalidator optionally lets the worker drop a cached credential so
// the next resolve re-fetches a fresh one from the Master. A credential cache
// implements this; a plain client does not need to. It is used to self-heal
// after a BITS rejection that may indicate a rotated/stale credential.
type CredentialInvalidator interface {
	Invalidate(operatorCode string)
}
