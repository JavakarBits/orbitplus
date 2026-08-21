package worker

import "errors"

// RetryableError marks a safe, transient failure that may be retried by the
// Worker. Its text must not contain credentials, secrets, or request URLs.
type RetryableError struct {
	message string
}

func (err *RetryableError) Error() string { return err.message }

// NewRetryableError creates a transient error without preserving potentially
// sensitive low-level transport details.
func NewRetryableError(message string) error {
	return &RetryableError{message: message}
}

// IsRetryableError reports whether an error is safe for a bounded retry.
func IsRetryableError(err error) bool {
	var retryableError *RetryableError
	return errors.As(err, &retryableError)
}
