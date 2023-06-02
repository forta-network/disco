package multidriver

import (
	"bytes"
	"context"
	"io"
	"net/url"
	"testing"
	"time"

	storagedriver "github.com/distribution/distribution/v3/registry/storage/driver"
	"github.com/forta-network/disco/drivers/filewriter"
	mock_interfaces "github.com/forta-network/disco/interfaces/mocks"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

const (
	testPath = "/test-path"
)

type DriverTestSuite struct {
	r *require.Assertions

	primary   *mock_interfaces.MockStorageDriver
	secondary *mock_interfaces.MockStorageDriver
	driver    *driver

	suite.Suite
}

func TestDriver(t *testing.T) {
	suite.Run(t, &DriverTestSuite{})
}

func (s *DriverTestSuite) SetupTest() {
	s.r = s.Require()

	testURL, err := url.Parse("http://foo.bar")
	s.r.NoError(err)
	ctrl := gomock.NewController(s.T())
	s.primary = mock_interfaces.NewMockStorageDriver(ctrl)
	s.secondary = mock_interfaces.NewMockStorageDriver(ctrl)
	s.driver = New(testURL, s.primary, s.secondary).(*driver)
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

func (s *DriverTestSuite) TestReader() {
	s.secondary.EXPECT().Stat(gomock.Any(), testPath).Return(&fileInfo{
		size: 1,
	}, nil).Times(2)
	s.secondary.EXPECT().Reader(gomock.Any(), testPath, int64(0)).
		Return(io.NopCloser(bytes.NewBufferString("1")), nil)

	reader, err := s.driver.Reader(context.Background(), testPath, 0)
	s.r.NoError(err)
	b, err := io.ReadAll(reader)
	s.r.NoError(err)
	s.r.Equal("1", string(b))
}

func (s *DriverTestSuite) TestGetContent() {
	s.secondary.EXPECT().Stat(gomock.Any(), testPath).Return(&fileInfo{
		size: 1,
	}, nil).Times(2)
	s.secondary.EXPECT().GetContent(gomock.Any(), testPath).
		Return([]byte("1"), nil)

	b, err := s.driver.GetContent(context.Background(), testPath)
	s.r.NoError(err)
	s.r.Equal("1", string(b))
}

func (s *DriverTestSuite) TestWriter() {
	priW := &filewriter.StubWriter{}
	secW := &filewriter.StubWriter{}
	s.primary.EXPECT().Writer(gomock.Any(), testPath, true).Return(priW, nil)
	s.primary.EXPECT().Name().Return("primary")
	s.secondary.EXPECT().Writer(gomock.Any(), testPath, true).Return(secW, nil)
	s.secondary.EXPECT().Name().Return("secondary")

	writer, err := s.driver.Writer(context.Background(), testPath, true)
	s.r.NoError(err)
	n, err := writer.Write(nil)
	s.r.NoError(err)
	s.r.Equal(0, n)
}

func (s *DriverTestSuite) TestPutContent() {
	s.primary.EXPECT().PutContent(gomock.Any(), testPath, []byte("1")).Return(nil)
	s.secondary.EXPECT().PutContent(gomock.Any(), testPath, []byte("1")).Return(nil)

	err := s.driver.PutContent(context.Background(), testPath, []byte("1"))
	s.r.NoError(err)
}

func (s *DriverTestSuite) TestStat() {
	s.secondary.EXPECT().Stat(gomock.Any(), testPath).Return(&fileInfo{
		size: 1,
	}, nil).Times(2)

	info, err := s.driver.Stat(context.Background(), testPath)
	s.r.NoError(err)
	s.r.NotNil(info)
}

func (s *DriverTestSuite) TestList() {
	s.secondary.EXPECT().Stat(gomock.Any(), testPath).Return(&fileInfo{
		size: 1,
	}, nil).Times(2)
	s.secondary.EXPECT().List(gomock.Any(), testPath).Return(nil, nil)

	list, err := s.driver.List(context.Background(), testPath)
	s.r.NoError(err)
	s.r.Empty(list)
}

func (s *DriverTestSuite) TestMove() {
	s.primary.EXPECT().Move(gomock.Any(), testPath, testPath+"1").Return(nil)
	s.secondary.EXPECT().Move(gomock.Any(), testPath, testPath+"1").Return(nil)

	s.r.NoError(s.driver.Move(context.Background(), testPath, testPath+"1"))
}

func (s *DriverTestSuite) TestDelete() {
	s.primary.EXPECT().Delete(gomock.Any(), testPath).Return(nil)
	s.secondary.EXPECT().Delete(gomock.Any(), testPath).Return(nil)

	s.r.NoError(s.driver.Delete(context.Background(), testPath))
}

func (s *DriverTestSuite) TestURLFor() {
	url, err := s.driver.URLFor(context.Background(), testPath, map[string]interface{}{
		"method": "GET",
	})
	s.r.NoError(err)
	s.r.Equal("http://foo.bar/test-path", url)
}

func (s *DriverTestSuite) TestWalk() {
	s.primary.EXPECT().Walk(gomock.Any(), testPath, gomock.Any()).Return(nil)
	s.secondary.EXPECT().Walk(gomock.Any(), testPath, gomock.Any()).Return(nil)

	s.r.NoError(s.driver.Walk(context.Background(), testPath, func(fileInfo storagedriver.FileInfo) error {
		return nil
	}))
}

func (s *DriverTestSuite) TestReplicateInPrimary() {
	s.primary.EXPECT().Stat(gomock.Any(), testPath).Return(&fileInfo{
		size: 1,
	}, nil).Times(2)

	info, err := s.driver.ReplicateInPrimary(testPath)
	s.r.NoError(err)
	s.r.NotNil(info)
}

func (s *DriverTestSuite) TestReplicateInSecondary() {
	s.secondary.EXPECT().Stat(gomock.Any(), testPath).Return(&fileInfo{
		size: 1,
	}, nil).Times(2)

	info, err := s.driver.ReplicateInSecondary(testPath)
	s.r.NoError(err)
	s.r.NotNil(info)
}

func (s *DriverTestSuite) TestReplicate() {
	s.secondary.EXPECT().Stat(gomock.Any(), testPath).Return(nil, storagedriver.PathNotFoundError{})
	s.primary.EXPECT().Stat(gomock.Any(), testPath).Return(&fileInfo{
		isDir: true,
	}, nil)
	s.primary.EXPECT().Walk(gomock.Any(), testPath, gomock.Any()).Return(nil)

	info, err := s.driver.replicate(context.Background(), s.primary, s.secondary, testPath)
	s.r.NoError(err)
	s.r.Nil(info)

	s.secondary.EXPECT().Stat(gomock.Any(), testPath).Return(nil, storagedriver.PathNotFoundError{})
	s.primary.EXPECT().Stat(gomock.Any(), testPath).Return(&fileInfo{
		isDir: false,
	}, nil)
	s.primary.EXPECT().Reader(gomock.Any(), testPath, int64(0)).Return(io.NopCloser(bytes.NewBufferString("1")), nil)
	s.secondary.EXPECT().Writer(gomock.Any(), testPath, false).Return(&filewriter.StubWriter{}, nil)
	s.primary.EXPECT().Name().Return("primary")
	s.secondary.EXPECT().Name().Return("secondary")

	info, err = s.driver.replicate(context.Background(), s.primary, s.secondary, testPath)
	s.r.NoError(err)
	s.r.Nil(info)
}

func (s *DriverTestSuite) TestName() {
	s.primary.EXPECT().Name().Return("primary")
	s.secondary.EXPECT().Name().Return("secondary")

	s.r.Equal("primary+secondary", s.driver.Name())
}

func (s *DriverTestSuite) TestIs() {
	md, ok := Is(s.driver)
	s.r.True(ok)
	s.r.NotNil(md)
}
