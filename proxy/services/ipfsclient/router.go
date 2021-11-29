package ipfsclient

import (
	"crypto/md5"
	"fmt"
	"math/big"
	"strings"
)

// Router routes the content path to a node index
type Router struct {
	nodeCount *big.Int
}

// NewRouter creates a new content router.
func NewRouter(nodeCount int) *Router {
	return &Router{
		nodeCount: big.NewInt(int64(nodeCount)),
	}
}

// RouteContent suggests a node index by consuming the content path.
// There are three types of main content on distribution server storage to
// load-balance/multiplex:
//  - .../repositories/*
//  - .../blobs/*
//  - .../uploads/* (original path from distribution server: .../repositories/<repo>/_uploads/*)
func (router *Router) RouteContent(path string) (string, int, error) {
	segments := strings.Split(path[1:], "/") // exclude leading slash
	if len(segments) < 5 {
		return "", 0, pathErr(path, "has less than 5 segments")
	}
	if segments[0] != "docker" || segments[1] != "registry" || segments[2] != "v2" {
		return "", 0, pathErr(path, "has invalid first 3 segments")
	}

	// strip /docker/registry/v2
	segments = segments[3:]

	var id string
	switch segments[0] {
	case "repositories", "uploads": // repository name, upload UUID
		id = segments[1]

	case "blobs": // blob hash after the bucket dir e.g. .../sha256/a8/a8b19f...
		id = segments[3]

	default:
		return "", 0, pathErr(path, "has invalid content kind segment")
	}

	hash := md5.Sum([]byte(id))
	hashNum := new(big.Int).SetBytes(hash[:])
	remainder := new(big.Int).Mod(hashNum, router.nodeCount)
	return id, int(remainder.Int64()), nil
}

func pathErr(path, reason string, args ...interface{}) error {
	return fmt.Errorf("path %s %s", args...)
}
