package panopticon

// APIError denotes an error that occurred during processing of a JSON API request.
type APIError struct {
	Message, Extra string
	Recoverable    bool
}

// APIResponse encapsulates a JSON object, with optional error indicator.
type APIResponse struct {
	Error    *APIError   `json:",omitEmpty"`
	Artifact interface{} `json:",omitEmpty"`
}
