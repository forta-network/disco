package utils

import (
	"encoding/hex"
	"strings"

	"github.com/ipfs/go-cid"
	"github.com/multiformats/go-multihash"
)

// ToCIDv1 converts IPFS CIDv0 to v1.
func ToCIDv1(cidV0 string) string {
	parsed, err := multihash.FromB58String(cidV0)
	if err != nil {
		panic(err)
	}
	return cid.NewCidV1(cid.DagProtobuf, parsed).String()
}

// IsCIDv1 checks if the hash is an IPFS CIDv1 hash.
func IsCIDv1(h string) bool {
	parsed, err := cid.Parse(h)
	if err != nil {
		return false
	}
	return parsed.Version() == 1
}

// IsDigestHex checks if the digest is a 64-byte blob digest hex.
func IsDigestHex(digest string) bool {
	if len(digest) != 64 {
		return false
	}
	_, err := hex.DecodeString(digest)
	return err == nil
}

// IsIPFSPath checks if given address is an IPFS content path.
func IsIPFSPath(path string) bool {
	if path[0] != '/' {
		return false
	}
	segments := strings.Split(path[1:], "/")
	if len(segments) != 2 {
		return false
	}
	_, err := cid.Parse(path)
	return err == nil
}
