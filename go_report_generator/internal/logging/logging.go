package logging

// VerbosityLevel defines the logging verbosity.
type VerbosityLevel int

const (
	Verbose VerbosityLevel = iota
	Info
	Warning
	Error
	Off
)
