package panopticon

var AuthError = &APIResponse{Error: &APIError{Message: "You are not logged in.", Extra: "Please log in to use this application.", Recoverable: true}}

var internalError = &APIResponse{Error: &APIError{Message: "The server encountered an error.", Extra: "", Recoverable: false}}
var appError = &APIResponse{Error: &APIError{Message: "There was a server error in the application.", Extra: "", Recoverable: false}}
var clientError = &APIResponse{Error: &APIError{Message: "There was a client error in the application.", Extra: "", Recoverable: false}}
var missingImage = &APIResponse{Error: &APIError{Message: "An image is unexpectedly missing.", Extra: "Try reloading the page.", Recoverable: true}}
var noSuchCamera = &APIResponse{Error: &APIError{Message: "That camera is unknown.", Extra: "Try a different camera.", Recoverable: true}}
