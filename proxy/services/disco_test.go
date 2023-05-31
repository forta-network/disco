package services

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"testing"
	"time"

	storagedriver "github.com/distribution/distribution/v3/registry/storage/driver"
	mock_multidriver "github.com/forta-network/disco/drivers/multidriver/mocks"
	"github.com/forta-network/disco/interfaces"
	mock_interfaces "github.com/forta-network/disco/interfaces/mocks"
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
	ipfsNode   *mock_interfaces.MockIPFSFilesAPI
	driver     *mock_multidriver.MockMultiDriver

	disco *Disco

	suite.Suite
}

// SetupTest sets up the test.
func (s *Suite) SetupTest() {
	s.ctx = context.Background()
	s.r = require.New(s.T())
	ctrl := gomock.NewController(s.T())
	s.ipfsClient = mock_interfaces.NewMockIPFSClient(ctrl)
	s.ipfsNode = mock_interfaces.NewMockIPFSFilesAPI(ctrl)
	s.ipfsClient.EXPECT().GetClientFor(gomock.Any(), gomock.Any()).Return(s.ipfsNode, nil).AnyTimes()
	s.driver = mock_multidriver.NewMockMultiDriver(ctrl)
	s.disco = &Disco{
		getIpfsClient: func() interfaces.IPFSClient {
			return s.ipfsClient
		},
		getDriver: func() storagedriver.StorageDriver {
			return s.driver
		},
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

type fileInfo struct {
	path  string
	size  int64
	isDir bool
}

func (fi *fileInfo) Path() string {
	return fi.path
}

func (fi *fileInfo) Size() int64 {
	return fi.size
}

func (fi *fileInfo) ModTime() time.Time {
	return time.Time{}
}

func (fi *fileInfo) IsDir() bool {
	return fi.isDir
}

func (s *Suite) TestMakeGlobalRepo() {
	// Given that a repo was pushed successfully
	// When the repo is intended to be made global automatically
	// Then it should find out that there is no repo with digest as the name yet
	s.driver.EXPECT().Stat(gomock.Any(), makeRepoPath(testManifestDigest)).Return(&fileInfo{
		path:  makeRepoPath(testManifestDigest),
		size:  0,
		isDir: false,
	}, nil)
	// And it should find the manifest digest
	s.driver.EXPECT().Reader(gomock.Any(), makeBlobPath(testManifestDigest), int64(0)).Return(io.NopCloser(bytes.NewBufferString(testManifest)), nil)
	// And replicate each blob and the uploaded repository in primary
	s.driver.EXPECT().ReplicateInPrimary(gomock.Any()).Times(3) // manifest, config and layer
	s.driver.EXPECT().ReplicateInPrimary(makeRepoPath("myrepo"))

	// And find the manifest link for the upload
	s.ipfsClient.EXPECT().FilesRead(s.ctx, registryBase+"/repositories/myrepo/_manifests/tags/latest/current/link").
		Return(ioutil.NopCloser(bytes.NewBuffer([]byte("sha256:"+testManifestDigest))), nil)
	// s.ipfsClient.EXPECT().FilesStat(s.ctx, registryBase+"/repositories/"+testManifestDigest).
	// 	Return(&ipfsapi.FilesStatObject{CumulativeSize: 0}, nil)
	// And find the manifest blob from the repo
	s.ipfsClient.EXPECT().FilesRead(s.ctx, registryBase+"/blobs/sha256/"+testManifestDigest[:2]+"/"+testManifestDigest+"/data").
		Return(ioutil.NopCloser(bytes.NewBufferString(testManifest)), nil)
	// And find the CIDs for all of the blobs
	s.ipfsClient.EXPECT().FilesStat(s.ctx, registryBase+"/blobs/sha256/"+testLayerDigest[:2]+"/"+testLayerDigest+"/data").
		Return(&ipfsapi.FilesStatObject{Hash: testLayerCid}, nil)
	s.ipfsClient.EXPECT().FilesStat(s.ctx, registryBase+"/blobs/sha256/"+testConfigDigest[:2]+"/"+testConfigDigest+"/data").
		Return(&ipfsapi.FilesStatObject{Hash: testConfigFileCid}, nil)
	s.ipfsClient.EXPECT().FilesStat(s.ctx, registryBase+"/blobs/sha256/"+testManifestDigest[:2]+"/"+testManifestDigest+"/data").
		Return(&ipfsapi.FilesStatObject{Hash: testManifestCid}, nil)
	// And write a Disco file
	s.ipfsClient.EXPECT().FilesWrite(s.ctx, registryBase+"/repositories/myrepo/disco.json", (*bufferMatcher)(bytes.NewBufferString(testDiscoFile)), gomock.Any()).
		Return(nil)

	// And get the CID for the repo and duplicate with the base32 CID v1
	s.ipfsClient.EXPECT().FilesStat(s.ctx, registryBase+"/repositories/myrepo").
		Return(&ipfsapi.FilesStatObject{Hash: testCidv0}, nil)
	s.ipfsNode.EXPECT().FilesCp(s.ctx, makeRepoPath("myrepo"), makeRepoPath(testCidv1)).
		Return(nil)
	// And duplicate the repo with digest name
	s.ipfsNode.EXPECT().FilesMkdir(s.ctx, repositoriesBase, gomock.Any()).Return(nil)
	s.ipfsNode.EXPECT().FilesRm(s.ctx, makeRepoPath(testManifestDigest), true).Return(nil)
	s.ipfsNode.EXPECT().FilesCp(s.ctx, fmt.Sprintf("/ipfs/%s", testCidv0), makeRepoPath(testManifestDigest)).
		Return(nil)
	// And copy the "latest" tag as CID in the digest repo
	s.ipfsClient.EXPECT().FilesCp(s.ctx, registryBase+"/repositories/"+testManifestDigest+"/_manifests/tags/latest",
		registryBase+"/repositories/"+testManifestDigest+"/_manifests/tags/"+testCidv1).
		Return(nil)
	// And remove the pushed repo from MFS
	s.driver.EXPECT().Delete(s.ctx, makeRepoPath("myrepo")).Return(nil)
	// And replicate the files in the secondary storage
	s.driver.EXPECT().ReplicateInSecondary(makeRepoPath(testManifestDigest)).Return(nil, nil)
	s.driver.EXPECT().ReplicateInSecondary(makeRepoPath(testCidv1)).Return(nil, nil)

	s.disco.MakeGlobalRepo(s.ctx, "myrepo")
}

func (s *Suite) TestAlreadyMadeGlobal() {
	// Given that a repo was pushed successfully
	// And made global previously
	// When the repo is inteded to be made global automatically again
	// Then it should find the manifest digest from the storage
	s.ipfsClient.EXPECT().FilesRead(s.ctx, makeRepoPath("myrepo")+"/_manifests/tags/latest/current/link").
		Return(ioutil.NopCloser(bytes.NewBuffer([]byte("sha256:"+testManifestDigest))), nil)
	// And expect that there is a repo with digest as the name
	s.driver.EXPECT().Stat(s.ctx, makeRepoPath(testManifestDigest)).
		Return(&fileInfo{
			path:  makeRepoPath(testManifestDigest),
			size:  1,
			isDir: false,
		}, nil)
	// And finally remove the pushed repo from MFS
	s.driver.EXPECT().Delete(s.ctx, makeRepoPath("myrepo")).Return(nil)

	s.disco.MakeGlobalRepo(s.ctx, "myrepo")
}

func (s *Suite) TestCloneGlobalRepo() {
	// Given that a repo was made global previously
	// When the repo is pulled with base32 CID v1
	// Then it should try to find the disco file first and not find it
	s.driver.EXPECT().Stat(gomock.Any(), makeDiscoFilePath(testCidv1)).Return(nil, storagedriver.PathNotFoundError{
		Path: makeDiscoFilePath(testCidv1),
	})
	// And clone the image repository from the ipfs network to the local ipfs node
	s.ipfsNode.EXPECT().FilesStat(gomock.Any(), makeDiscoFilePath(testCidv1)).Return(nil, errors.New("does not exist"))
	s.ipfsNode.EXPECT().FilesMkdir(gomock.Any(), repositoriesBase, gomock.Any())
	s.ipfsNode.EXPECT().FilesCp(gomock.Any(), fmt.Sprintf("/ipfs/%s", testCidv1), makeRepoPath(testCidv1))
	s.ipfsNode.EXPECT().FilesRead(gomock.Any(), makeDiscoFilePath(testCidv1)).Return(
		io.NopCloser(bytes.NewBufferString(testDiscoFile)),
		nil,
	)

	// And clone the blobs from the ipfs network to the local ipfs node

	s.ipfsNode.EXPECT().FilesStat(gomock.Any(), makeBlobPath(testManifestDigest)).Return(nil, errors.New("does not exist"))
	s.ipfsNode.EXPECT().FilesMkdir(gomock.Any(), makeBlobDirPath(testManifestDigest), gomock.Any())
	s.ipfsNode.EXPECT().FilesCp(gomock.Any(), fmt.Sprintf("/ipfs/%s", testManifestCid), makeBlobPath(testManifestDigest))

	s.ipfsNode.EXPECT().FilesStat(gomock.Any(), makeBlobPath(testConfigDigest)).Return(nil, errors.New("does not exist"))
	s.ipfsNode.EXPECT().FilesMkdir(gomock.Any(), makeBlobDirPath(testConfigDigest), gomock.Any())
	s.ipfsNode.EXPECT().FilesCp(gomock.Any(), fmt.Sprintf("/ipfs/%s", testConfigFileCid), makeBlobPath(testConfigDigest))

	s.ipfsNode.EXPECT().FilesStat(gomock.Any(), makeBlobPath(testLayerDigest)).Return(nil, errors.New("does not exist"))
	s.ipfsNode.EXPECT().FilesMkdir(gomock.Any(), makeBlobDirPath(testLayerDigest), gomock.Any())
	s.ipfsNode.EXPECT().FilesCp(gomock.Any(), fmt.Sprintf("/ipfs/%s", testLayerCid), makeBlobPath(testLayerDigest))

	// And replicate the cloned files to the secondary storage
	s.driver.EXPECT().ReplicateInSecondary(makeRepoPath(testCidv1)).Return(nil, nil)
	s.driver.EXPECT().ReplicateInSecondary(makeBlobPath(testManifestDigest)).Return(nil, nil)
	s.driver.EXPECT().ReplicateInSecondary(makeBlobPath(testConfigDigest)).Return(nil, nil)
	s.driver.EXPECT().ReplicateInSecondary(makeBlobPath(testLayerDigest)).Return(nil, nil)

	s.disco.CloneGlobalRepo(s.ctx, testCidv1)
}

func (s *Suite) TestAlreadyCloned() {
	// Given that a repo was made global previously
	// And already cloned and pulled
	// When the repo is pulled with base32 CID v1 again
	// Then it should find the disco file and abort cloning again
	s.driver.EXPECT().Stat(gomock.Any(), makeDiscoFilePath(testCidv1)).Return(&fileInfo{
		path:  makeDiscoFilePath(testCidv1),
		size:  1,
		isDir: false,
	}, nil)

	s.disco.CloneGlobalRepo(s.ctx, testCidv1)
}
