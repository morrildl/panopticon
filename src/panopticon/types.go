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

// MediaKind describes the origination of a piece of media. That is, was it collected routinely by a
// periodic uploader, was it synthesized from other media, was it proactively pinned by a user, etc.
type MediaKind string

// enum constants for MediaKind
const (
	MediaCollected MediaKind = "collected"
	MediaMotion              = "motion"
	MediaPinned              = "pinned"
	MediaGenerated           = "generated"
	MediaUnknown             = ""
)

// AllKinds is simply a list of all legitimate MediaKind values, intended for use in `range`
// statements, etc.
var AllKinds = []MediaKind{MediaCollected, MediaMotion, MediaPinned, MediaGenerated}

// AspectRatio enumerates all acceptable aspect ratios for camera images. It's used to format the UI properly for a given camera.
type AspectRatio string

// Enum constats for AspectRatio
const (
	Aspect16x9 AspectRatio = "16x9"
	Aspect4x3              = "4x3"
)
