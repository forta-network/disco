package services

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"testing"

	mock_interfaces "github.com/forta-network/disco/proxy/services/interfaces/mocks"
	"github.com/golang/mock/gomock"
	ipfsapi "github.com/ipfs/go-ipfs-api"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

const (
	testCidv0          = "QmQahNfao3EqrFMKExRB8bedoSgot5mQJH5GDPBuMZH41r"
	testCidv1          = "bafybeibbkcck6lz37hcipp2mwtfdgstydizjq45z4fkqq4va73mp7qzutu"
	testManifestDigest = "dca71257cd2e72840a21f0323234bb2e33fea6d949fa0f21c5102146f583486b"
	testManifest       = `{
		"schemaVersion": 2,
		"mediaType": "application/vnd.docker.distribution.manifest.v2+json",
		"config": {
			 "mediaType": "application/vnd.docker.container.image.v1+json",
			 "size": 1457,
			 "digest": "sha256:69593048aa3acfee0f75f20b77acb549de2472063053f6730c4091b53f2dfb02"
		},
		"layers": [
			 {
					"mediaType": "application/vnd.docker.image.rootfs.diff.tar.gzip",
					"size": 766607,
					"digest": "sha256:b71f96345d44b237decc0c2d6c2f9ad0d17fde83dad7579608f1f0764d9686f2"
			 }
		]
 }`
	testConfigDigest  = "69593048aa3acfee0f75f20b77acb549de2472063053f6730c4091b53f2dfb02"
	testLayerDigest   = "b71f96345d44b237decc0c2d6c2f9ad0d17fde83dad7579608f1f0764d9686f2"
	testManifestCid   = "QmZFwJdqgfMKCK4by7nsTRCmQiPWJbVrvup62jjBhmgRP9"
	testConfigFileCid = "QmXjXzaQbKkz8D8T1fHy6C3JeWX7Ez6JqTsJrRyzqW1cMS"
	testLayerCid      = "QmZDpp1fytMpa7YJKR1CQcjM1vDbkA7K3giL7vTyEwjFdN"
	testDiscoFile     = `{"blobs":[{"digest":"dca71257cd2e72840a21f0323234bb2e33fea6d949fa0f21c5102146f583486b","cid":"QmZFwJdqgfMKCK4by7nsTRCmQiPWJbVrvup62jjBhmgRP9"},{"digest":"69593048aa3acfee0f75f20b77acb549de2472063053f6730c4091b53f2dfb02","cid":"QmXjXzaQbKkz8D8T1fHy6C3JeWX7Ez6JqTsJrRyzqW1cMS"},{"digest":"b71f96345d44b237decc0c2d6c2f9ad0d17fde83dad7579608f1f0764d9686f2","cid":"QmZDpp1fytMpa7YJKR1CQcjM1vDbkA7K3giL7vTyEwjFdN"}]}
`
)

// TestSuite runs the test suite.
func TestSuite(t *testing.T) {
	suite.Run(t, &Suite{})
}

// Suite is a test suite to test the agent pool.
type Suite struct {
	ctx context.Context
	r   *require.Assertions

	ipfsClient *mock_interfaces.MockIPFSClient

	disco *Disco

	suite.Suite
}

// SetupTest sets up the test.
func (s *Suite) SetupTest() {
	s.ctx = context.Background()
	s.r = require.New(s.T())
	s.ipfsClient = mock_interfaces.NewMockIPFSClient(gomock.NewController(s.T()))
	s.disco = &Disco{
		api: s.ipfsClient,
	}
}

// TestIsOnlyPullable makes sure that the methods tells us what we cannot push.
func (s *Suite) TestIsOnlyPullable() {
	s.r.True(s.disco.IsOnlyPullable(testCidv1))
	s.r.True(s.disco.IsOnlyPullable(testManifestDigest))
	s.r.False(s.disco.IsOnlyPullable("myrepo"))
}

type bufferMatcher bytes.Buffer

func (bm *bufferMatcher) Matches(x interface{}) bool {
	buf, ok := x.(*bytes.Buffer)
	if !ok {
		return false
	}
	return bm.String() == buf.String()
}

func (bm *bufferMatcher) String() string {
	return (*bytes.Buffer)(bm).String()
}

func (s *Suite) TestMakeGlobalRepo() {
	// Given that a repo was pushed successfully
	// When the repo is intended to be made global automatically
	// Then it should find the manifest digest from the storage
	s.ipfsClient.EXPECT().FilesRead(s.ctx, registryBase+"/repositories/myrepo/_manifests/tags/latest/current/link").
		Return(ioutil.NopCloser(bytes.NewBuffer([]byte("sha256:"+testManifestDigest))), nil)
	// And expect that there is no repo with digest as the name yet
	s.ipfsClient.EXPECT().FilesStat(s.ctx, registryBase+"/repositories/"+testManifestDigest).
		Return(&ipfsapi.FilesStatObject{CumulativeSize: 0}, nil)
	// And find the manifest blob from the repo
	s.ipfsClient.EXPECT().FilesRead(s.ctx, registryBase+"/blobs/sha256/"+testManifestDigest[:2]+"/"+testManifestDigest+"/data").
		Return(ioutil.NopCloser(bytes.NewBufferString(testManifest)), nil)
	// And find the CIDs for all of the blobs
	s.ipfsClient.EXPECT().FilesStat(s.ctx, registryBase+"/blobs/sha256/"+testManifestDigest[:2]+"/"+testManifestDigest+"/data").
		Return(&ipfsapi.FilesStatObject{Hash: testManifestCid}, nil)
	s.ipfsClient.EXPECT().FilesStat(s.ctx, registryBase+"/blobs/sha256/"+testConfigDigest[:2]+"/"+testConfigDigest+"/data").
		Return(&ipfsapi.FilesStatObject{Hash: testConfigFileCid}, nil)
	s.ipfsClient.EXPECT().FilesStat(s.ctx, registryBase+"/blobs/sha256/"+testLayerDigest[:2]+"/"+testLayerDigest+"/data").
		Return(&ipfsapi.FilesStatObject{Hash: testLayerCid}, nil)
	// And write a Disco file
	s.ipfsClient.EXPECT().FilesWrite(s.ctx, registryBase+"/repositories/myrepo/disco.json", (*bufferMatcher)(bytes.NewBufferString(testDiscoFile)), gomock.Any()).
		Return(nil)
	// And duplicate the repo with digest name
	s.ipfsClient.EXPECT().FilesCp(s.ctx, registryBase+"/repositories/myrepo", registryBase+"/repositories/"+testManifestDigest).
		Return(nil)
	// And get the CID for the repo and duplicate with the base32 CID v1
	s.ipfsClient.EXPECT().FilesStat(s.ctx, registryBase+"/repositories/myrepo").
		Return(&ipfsapi.FilesStatObject{Hash: testCidv0}, nil)
	s.ipfsClient.EXPECT().FilesCp(s.ctx, registryBase+"/repositories/myrepo", registryBase+"/repositories/"+testCidv1).
		Return(nil)
	// And copy the "latest" tag as CID in the digest repo
	s.ipfsClient.EXPECT().FilesCp(s.ctx, registryBase+"/repositories/"+testManifestDigest+"/_manifests/tags/latest",
		registryBase+"/repositories/"+testManifestDigest+"/_manifests/tags/"+testCidv1).
		Return(nil)
	// And finally remove the pushed repo from MFS
	s.ipfsClient.EXPECT().FilesRm(s.ctx, registryBase+"/repositories/myrepo", true).Return(nil)

	s.disco.MakeGlobalRepo(s.ctx, "myrepo")
}

func (s *Suite) TestAlreadyMadeGlobal() {
	// Given that a repo was pushed successfully
	// And made global previously
	// When the repo is inteded to be made global automatically again
	// Then it should find the manifest digest from the storage
	s.ipfsClient.EXPECT().FilesRead(s.ctx, fmt.Sprintf(registryBase+"/repositories/myrepo/_manifests/tags/latest/current/link")).
		Return(ioutil.NopCloser(bytes.NewBuffer([]byte("sha256:"+testManifestDigest))), nil)
	// And expect that there is no repo with digest as the name yet
	s.ipfsClient.EXPECT().FilesStat(s.ctx, fmt.Sprintf(registryBase+"/repositories/"+testManifestDigest)).
		Return(&ipfsapi.FilesStatObject{CumulativeSize: 10101010}, nil)
	// And finally remove the pushed repo from MFS
	s.ipfsClient.EXPECT().FilesRm(s.ctx, registryBase+"/repositories/myrepo", true).Return(nil)

	s.disco.MakeGlobalRepo(s.ctx, "myrepo")
}

func (s *Suite) TestCloneGlobalRepo() {
	// Given that a repo was made global previously
	// When the repo is pulled with base32 CID v1
	// Then it should try to find the manifest digest first
	s.ipfsClient.EXPECT().FilesRead(s.ctx, registryBase+"/repositories/"+testCidv1+"/_manifests/tags/latest/current/link").
		Return(nil, errors.New("file not found error"))
	// And try to create the repositories base dir in MFS and ignore errors
	s.ipfsClient.EXPECT().FilesMkdir(s.ctx, registryBase+"/repositories", gomock.Any()).
		Return(errors.New("already exists error"))
	// And try to copy the repo from IPFS network
	s.ipfsClient.EXPECT().FilesCp(s.ctx, "/ipfs/"+testCidv1, registryBase+"/repositories/"+testCidv1).
		Return(nil)
	// And try to read the manifest digest again
	s.ipfsClient.EXPECT().FilesRead(s.ctx, registryBase+"/repositories/"+testCidv1+"/_manifests/tags/latest/current/link").
		Return(ioutil.NopCloser(bytes.NewBuffer([]byte("sha256:"+testManifestDigest))), nil)
	// And read the Disco file and copy all of the blobs from the IPFS network
	s.ipfsClient.EXPECT().FilesRead(s.ctx, registryBase+"/repositories/"+testCidv1+"/disco.json").
		Return(ioutil.NopCloser(bytes.NewBuffer([]byte(testDiscoFile))), nil)
	s.ipfsClient.EXPECT().FilesMkdir(s.ctx, registryBase+"/blobs/sha256/"+testManifestDigest[:2]+"/"+testManifestDigest, gomock.Any()).Return(nil)
	s.ipfsClient.EXPECT().FilesCp(s.ctx, "/ipfs/"+testManifestCid, registryBase+"/blobs/sha256/"+testManifestDigest[:2]+"/"+testManifestDigest+"/data").Return(nil)
	s.ipfsClient.EXPECT().FilesMkdir(s.ctx, registryBase+"/blobs/sha256/"+testConfigDigest[:2]+"/"+testConfigDigest, gomock.Any()).Return(nil)
	s.ipfsClient.EXPECT().FilesCp(s.ctx, "/ipfs/"+testConfigFileCid, registryBase+"/blobs/sha256/"+testConfigDigest[:2]+"/"+testConfigDigest+"/data").Return(nil)
	s.ipfsClient.EXPECT().FilesMkdir(s.ctx, registryBase+"/blobs/sha256/"+testLayerDigest[:2]+"/"+testLayerDigest, gomock.Any()).Return(nil)
	s.ipfsClient.EXPECT().FilesCp(s.ctx, "/ipfs/"+testLayerCid, registryBase+"/blobs/sha256/"+testLayerDigest[:2]+"/"+testLayerDigest+"/data").Return(nil)
	// And copy the repo with digest name
	s.ipfsClient.EXPECT().FilesCp(s.ctx, registryBase+"/repositories/"+testCidv1, registryBase+"/repositories/"+testManifestDigest).
		Return(nil)
	// And copy the "latest" tag as CID in the digest repo
	s.ipfsClient.EXPECT().FilesCp(s.ctx, registryBase+"/repositories/"+testManifestDigest+"/_manifests/tags/latest",
		registryBase+"/repositories/"+testManifestDigest+"/_manifests/tags/"+testCidv1).
		Return(nil)

	s.disco.CloneGlobalRepo(s.ctx, testCidv1)
}

func (s *Suite) TestAlreadyCloned() {
	// Given that a repo was made global previously
	// And already cloned and pulled
	// When the repo is pulled with base32 CID v1 again
	// Then it should try to find the manifest digest and be done when there are no errors
	s.ipfsClient.EXPECT().FilesRead(s.ctx, registryBase+"/repositories/"+testCidv1+"/_manifests/tags/latest/current/link").
		Return(ioutil.NopCloser(bytes.NewBuffer([]byte("sha256:"+testManifestDigest))), nil)

	s.disco.CloneGlobalRepo(s.ctx, testCidv1)
}
