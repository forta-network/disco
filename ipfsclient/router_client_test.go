package ipfsclient

import (
	"bytes"
	"context"
	"io"
	"testing"

	mock_interfaces "github.com/forta-network/disco/interfaces/mocks"
	"github.com/golang/mock/gomock"
	ipfsapi "github.com/ipfs/go-ipfs-api"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

const (
	testPath1   = "/docker/registry/v2/repositories/aa"
	testPath2   = "/docker/registry/v2/repositories/ac"
	testCid     = "QmQahNfao3EqrFMKExRB8bedoSgot5mQJH5GDPBuMZH41r"
	testCidPath = "/ipfs/QmQahNfao3EqrFMKExRB8bedoSgot5mQJH5GDPBuMZH41r"
)

type RouterTestSuite struct {
	r *require.Assertions

	ipfsClient1  *mock_interfaces.MockIPFSFilesAPI
	ipfsClient2  *mock_interfaces.MockIPFSFilesAPI
	routerClient *RouterClient

	suite.Suite
}

func TestRouterTestSuite(t *testing.T) {
	suite.Run(t, &RouterTestSuite{})
}

func (s *RouterTestSuite) SetupTest() {
	s.r = s.Require()

	ctrl := gomock.NewController(s.T())
	s.ipfsClient1 = mock_interfaces.NewMockIPFSFilesAPI(ctrl)
	s.ipfsClient2 = mock_interfaces.NewMockIPFSFilesAPI(ctrl)
	s.routerClient = &RouterClient{
		router: NewRouter(2),
		nodes: []*ipfsNode{
			{
				client: s.ipfsClient1,
			},
			{
				client: s.ipfsClient2,
			},
		},
	}
}

func (s *RouterTestSuite) TestGetClientFor() {
	client, err := s.routerClient.GetClientFor(context.Background(), testPath1)
	s.r.NoError(err)
	s.r.Equal(s.ipfsClient1, client)
}

func (s *RouterTestSuite) TestFilesRead() {
	s.ipfsClient1.EXPECT().FilesRead(gomock.Any(), testPath1).Return(io.NopCloser(bytes.NewBufferString("")), nil)

	r, err := s.routerClient.FilesRead(context.Background(), testPath1)
	s.r.NoError(err)
	s.r.NotNil(r)
}

func (s *RouterTestSuite) TestFilesWrite() {
	s.ipfsClient1.EXPECT().FilesWrite(gomock.Any(), testPath1, gomock.Any()).Return(nil)

	s.r.NoError(s.routerClient.FilesWrite(context.Background(), testPath1, bytes.NewBufferString("")))
}

func (s *RouterTestSuite) TestFilesRm() {
	s.ipfsClient1.EXPECT().FilesRm(gomock.Any(), testPath1, true)

	s.r.NoError(s.routerClient.FilesRm(context.Background(), testPath1, true))
}

func (s *RouterTestSuite) TestFilesCp() {
	s.ipfsClient1.EXPECT().FilesStat(gomock.Any(), testPath1).Return(&ipfsapi.FilesStatObject{
		Hash: testCid,
	}, nil)
	s.ipfsClient1.EXPECT().FilesCp(gomock.Any(), testCidPath, testPath1)

	s.r.NoError(s.routerClient.FilesCp(context.Background(), testPath1, testPath1))
}

func (s *RouterTestSuite) TestFilesStat() {
	s.ipfsClient1.EXPECT().FilesStat(gomock.Any(), testPath1).Return(&ipfsapi.FilesStatObject{}, nil)

	stat, err := s.routerClient.FilesStat(context.Background(), testPath1)
	s.r.NoError(err)
	s.r.NotNil(stat)
}

func (s *RouterTestSuite) TestFilesMkdir() {
	s.ipfsClient1.EXPECT().FilesMkdir(gomock.Any(), testPath1)

	s.r.NoError(s.routerClient.FilesMkdir(context.Background(), testPath1))
}

func (s *RouterTestSuite) TestFilesLs() {
	s.ipfsClient1.EXPECT().FilesLs(gomock.Any(), testPath1).Return([]*ipfsapi.MfsLsEntry{}, nil)

	list, err := s.routerClient.FilesLs(context.Background(), testPath1)
	s.r.NoError(err)
	s.r.NotNil(list)
}

func (s *RouterTestSuite) TestFilesMv() {
	// delete from second
	s.ipfsClient2.EXPECT().FilesRm(gomock.Any(), testPath2, true)
	// find the cid from first
	s.ipfsClient1.EXPECT().FilesStat(gomock.Any(), testPath1).Return(&ipfsapi.FilesStatObject{
		Hash: testCid,
	}, nil)
	// copy to second using cid path
	s.ipfsClient2.EXPECT().FilesCp(gomock.Any(), testCidPath, testPath2)
	// delete from first
	s.ipfsClient1.EXPECT().FilesRm(gomock.Any(), testPath1, true)

	s.r.NoError(s.routerClient.FilesMv(context.Background(), testPath1, testPath2))
}
