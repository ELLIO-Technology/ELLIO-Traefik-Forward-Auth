package api

// PermanentError indicates an unrecoverable error that should not be retried
type PermanentError struct {
	StatusCode int
	Message    string
}

func (e *PermanentError) Error() string {
	return "permanent error: " + e.Message
}

// IsPermanentError checks if an error is a permanent error
func IsPermanentError(err error) bool {
	_, ok := err.(*PermanentError)
	return ok
}
