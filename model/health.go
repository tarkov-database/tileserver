package model

var initFailure bool

// Status represents the status code of a service
type Status int

const (
	// OK status if all checks were successful
	OK Status = iota

	// Warning status if non-critical issues are discovered
	Warning

	// Failure status when critical problems are discovered
	Failure
)

// Health represents the object of the health root endpoint
type Health struct {
	OK bool `json:"ok"`
}

// GetHealth performs a self-check and returns the result
func GetHealth() *Health {
	health := &Health{
		OK: !initFailure,
	}

	return health
}

func SetInitAsFailed() {
	initFailure = true
}
