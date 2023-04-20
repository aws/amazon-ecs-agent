// copyright amazon.com inc. or its affiliates. all rights reserved.
//
// licensed under the apache license, version 2.0 (the "license"). you may
// not use this file except in compliance with the license. a copy of the
// license is located at
//
//	http://aws.amazon.com/apache2.0/
//
// or in the "license" file accompanying this file. this file is distributed
// on an "as is" basis, without warranties or conditions of any kind, either
// express or implied. see the license for the specific language governing
// permissions and limitations under the license.
package mux

const (
	// AnythingRegEx is a regex pattern that matches anything.
	AnythingRegEx = ".*"

	// AnythingButSlashRegEx is a regex pattern that matches any string without slash.
	AnythingButSlashRegEx = "[^/]*"

	// AnythingButEmptyRegEx is a regex pattern that matches anything but an empty string.
	AnythingButEmptyRegEx = ".+"
)

// ConstructMuxVar constructs the mux var that is used in the gorilla/mux styled
// path, example: {id}, {id:[0-9]+}.
func ConstructMuxVar(name string, pattern string) string {
	if pattern == "" {
		return "{" + name + "}"
	}

	return "{" + name + ":" + pattern + "}"
}
