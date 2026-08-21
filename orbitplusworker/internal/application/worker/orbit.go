package worker

import "context"

// OperatorCredentialRequest identifies the operator whose Bits credential is
// resolved from OrbitService. It carries no secret material itself.
type OperatorCredentialRequest struct {
	OperatorCode string
}

// OperatorCredential is the credential material resolved from OrbitService.
type OperatorCredential struct {
	OperatorCode string
	Username     string
	APIToken     string
}

// OperatorCredentialClient resolves per-operator Bits credentials from OrbitService.
type OperatorCredentialClient interface {
	FetchOperatorCredential(ctx context.Context, request OperatorCredentialRequest) (OperatorCredential, error)
}
