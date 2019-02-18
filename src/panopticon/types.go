// Copyright © 2019 Dan Morrill
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

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
