package drivers

import "strings"

// FixUploadPath rewrites .../repository/<name>/_uploads to .../uploads to make things easier.
func FixUploadPath(path string) string {
	if !strings.Contains(path, "/_uploads") {
		return path
	}
	newPath := "/docker/registry/v2/uploads"
	var append bool
	for _, segment := range strings.Split(path, "/") {
		if append {
			newPath += "/" + segment
		}
		if segment == "_uploads" {
			append = true
		}
	}
	return newPath
}
