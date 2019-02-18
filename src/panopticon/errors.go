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

var AuthError = &APIResponse{Error: &APIError{Message: "You are not logged in.", Extra: "Please log in to use this application.", Recoverable: true}}

var internalError = &APIResponse{Error: &APIError{Message: "The server encountered an error.", Extra: "", Recoverable: false}}
var appError = &APIResponse{Error: &APIError{Message: "There was a server error in the application.", Extra: "", Recoverable: false}}
var clientError = &APIResponse{Error: &APIError{Message: "There was a client error in the application.", Extra: "", Recoverable: false}}
var missingImage = &APIResponse{Error: &APIError{Message: "An image is unexpectedly missing.", Extra: "Try reloading the page.", Recoverable: true}}
var noSuchCamera = &APIResponse{Error: &APIError{Message: "That camera is unknown.", Extra: "Try a different camera.", Recoverable: true}}
