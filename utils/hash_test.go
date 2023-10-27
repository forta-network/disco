package utils

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

const (
	testCidv0          = "QmQahNfao3EqrFMKExRB8bedoSgot5mQJH5GDPBuMZH41r"
	testCidv1          = "bafybeibbkcck6lz37hcipp2mwtfdgstydizjq45z4fkqq4va73mp7qzutu"
	testManifestDigest = "dca71257cd2e72840a21f0323234bb2e33fea6d949fa0f21c5102146f583486b"
)

func TestToCIDv1(t *testing.T) {
	r := require.New(t)

	result, err := ToCIDv1(testCidv0)
	r.NoError(err)
	r.Equal(testCidv1, result)

	_, err = ToCIDv1("not-cid-v0")
	r.Error(err)
}

func TestIsCIDv1(t *testing.T) {
	r := require.New(t)

	r.True(IsCIDv1(testCidv1))
	r.False(IsCIDv1(testCidv0))
}

func TestIsDigestHex(t *testing.T) {
	r := require.New(t)

	r.True(IsDigestHex(testManifestDigest))
	r.False(IsDigestHex("not-sha256-digest-hex"))
}

func TestIsIPFSPath(t *testing.T) {
	r := require.New(t)

	r.True(IsIPFSPath(fmt.Sprintf("/ipfs/%s", testCidv0)))
	r.False(IsIPFSPath("/foo/bar"))
}

func TestConvertSHA256HexToCIDv1(t *testing.T) {
	r := require.New(t)

	cidStr, err := ConvertSHA256HexToCIDv1(testManifestDigest)
	r.NoError(err)
	r.Equal("bafybeig4u4jfptjookcauipqgizdjozogp7knwkj7ihsdriqefdpla2inm", cidStr)
}
