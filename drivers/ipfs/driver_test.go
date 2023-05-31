package ipfs

import (
	"bytes"
	"context"
	"io"
	"testing"

	storagedriver "github.com/distribution/distribution/v3/registry/storage/driver"
	mock_interfaces "github.com/forta-network/disco/interfaces/mocks"
	"github.com/golang/mock/gomock"
	ipfsapi "github.com/ipfs/go-ipfs-api"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

const (
	testPath = "/test-path"
)

type DriverTestSuite struct {
	r *require.Assertions

	ipfsClient *mock_interfaces.MockIPFSClient
	driver     storagedriver.StorageDriver

	suite.Suite
}

func TestDriver(t *testing.T) {
	suite.Run(t, &DriverTestSuite{})
}

func (s *DriverTestSuite) SetupTest() {
	s.r = s.Require()

	ctrl := gomock.NewController(s.T())
	s.ipfsClient = mock_interfaces.NewMockIPFSClient(ctrl)
	s.driver = New(s.ipfsClient)
}

func (s *DriverTestSuite) TestReader() {
	s.ipfsClient.EXPECT().FilesRead(gomock.Any(), testPath, gomock.Any()).
		Return(io.NopCloser(bytes.NewBufferString("1")), nil)

	reader, err := s.driver.Reader(context.Background(), testPath, 0)
	s.r.NoError(err)
	b, err := io.ReadAll(reader)
	s.r.NoError(err)
	s.r.Equal("1", string(b))
}

func (s *DriverTestSuite) TestGetContent() {
	s.ipfsClient.EXPECT().FilesRead(gomock.Any(), testPath, gomock.Any()).
		Return(io.NopCloser(bytes.NewBufferString("1")), nil)

	b, err := s.driver.GetContent(context.Background(), testPath)
	s.r.NoError(err)
	s.r.Equal("1", string(b))
}

type readerMatcher struct {
}

// Matches returns whether x is a match.
func (rm *readerMatcher) Matches(x interface{}) bool {
	b := make([]byte, 1)
	x.(*io.PipeReader).Read(b)
	return true
}

// String describes what the matcher matches.
func (rm *readerMatcher) String() string {
	return ""
}

func (s *DriverTestSuite) TestWriter() {
	s.ipfsClient.EXPECT().FilesStat(gomock.Any(), testPath, gomock.Any()).Return(&ipfsapi.FilesStatObject{
		Size: 0,
	}, nil)
	s.ipfsClient.EXPECT().FilesWrite(gomock.Any(), testPath, &readerMatcher{}, gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil)

	writer, err := s.driver.Writer(context.Background(), testPath, true)
	n, err := writer.Write([]byte("1"))
	s.r.NoError(writer.Commit())
	s.r.NoError(writer.Close())
	s.r.NoError(err)
	s.r.Equal(1, n)

	s.r.NoError(err)
}

func (s *DriverTestSuite) TestPutContent() {
	s.ipfsClient.EXPECT().FilesWrite(gomock.Any(), testPath, gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil)

	err := s.driver.PutContent(context.Background(), testPath, []byte("1"))
	s.r.NoError(err)
}

func (s *DriverTestSuite) TestStat() {
	ipfsStat := &ipfsapi.FilesStatObject{}
	s.ipfsClient.EXPECT().FilesStat(gomock.Any(), testPath).Return(ipfsStat, nil)

	stat, err := s.driver.Stat(context.Background(), testPath)
	s.r.NoError(err)
	s.r.Equal(ipfsStat, stat.(*fileInfo).FilesStatObject)
}

func (s *DriverTestSuite) TestList() {
	s.ipfsClient.EXPECT().FilesLs(gomock.Any(), testPath).Return(nil, nil)

	list, err := s.driver.List(context.Background(), testPath)
	s.r.NoError(err)
	s.r.Empty(list)
}

func (s *DriverTestSuite) TestMove() {
	s.ipfsClient.EXPECT().FilesMkdir(gomock.Any(), gomock.Any(), gomock.Any())
	s.ipfsClient.EXPECT().FilesMv(gomock.Any(), testPath, testPath+"1")

	s.r.NoError(s.driver.Move(context.Background(), testPath, testPath+"1"))
}

func (s *DriverTestSuite) TestDelete() {
	s.ipfsClient.EXPECT().FilesRm(gomock.Any(), testPath, true)

	s.r.NoError(s.driver.Delete(context.Background(), testPath))
}

func (s *DriverTestSuite) TestURLFor() {
	url, err := s.driver.URLFor(context.Background(), testPath, nil)
	s.r.Error(err)
	s.r.Empty(url)
}

func (s *DriverTestSuite) TestWalk() {
	ipfsStat := &ipfsapi.FilesStatObject{}
	s.ipfsClient.EXPECT().FilesStat(gomock.Any(), testPath).Return(ipfsStat, nil).AnyTimes()
	s.ipfsClient.EXPECT().FilesLs(gomock.Any(), gomock.Any()).Return(nil, nil)

	s.driver.Walk(context.Background(), testPath, func(fileInfo storagedriver.FileInfo) error {
		return nil
	})
}
