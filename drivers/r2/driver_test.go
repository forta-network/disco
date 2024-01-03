package r2

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	storagedriver "github.com/distribution/distribution/v3/registry/storage/driver"
	mock_interfaces "github.com/forta-network/disco/interfaces/mocks"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

const (
	testPath = "/test-path"
)

type DriverTestSuite struct {
	r *require.Assertions

	r2Client *mock_interfaces.MockR2Client
	driver   storagedriver.StorageDriver

	suite.Suite
}

func TestDriver(t *testing.T) {
	suite.Run(t, &DriverTestSuite{})
}

func (s *DriverTestSuite) SetupTest() {
	s.r = s.Require()

	ctrl := gomock.NewController(s.T())
	s.r2Client = mock_interfaces.NewMockR2Client(ctrl)
	params := DriverParameters{ChunkSize: minChunkSize}

	var err error
	s.driver, err = newFromClient(s.r2Client, params)
	assert.NoError(s.T(), err)
}

func (s *DriverTestSuite) TestReader() {
	output := &s3.GetObjectOutput{
		Body: io.NopCloser(bytes.NewReader([]byte("1"))),
	}
	input := &s3.GetObjectInput{
		Bucket: aws.String(""),
		Key:    aws.String("test-path"),
		Range:  aws.String("bytes=0-"),
	}
	s.r2Client.EXPECT().
		GetObject(gomock.Any(), input).
		Return(output, nil)

	reader, err := s.driver.Reader(context.Background(), testPath, 0)
	s.r.NoError(err)
	b, err := io.ReadAll(reader)
	s.r.NoError(err)
	s.r.Equal("1", string(b))
}

func (s *DriverTestSuite) TestGetContent() {
	output := &s3.GetObjectOutput{
		Body: io.NopCloser(bytes.NewReader([]byte("1"))),
	}
	input := &s3.GetObjectInput{
		Bucket: aws.String(""),
		Key:    aws.String("test-path"),
		Range:  aws.String("bytes=0-"),
	}
	s.r2Client.EXPECT().
		GetObject(gomock.Any(), input).
		Return(output, nil)

	b, err := s.driver.GetContent(context.Background(), testPath)
	s.r.NoError(err)
	s.r.Equal("1", string(b))
}

func (s *DriverTestSuite) TestWriter() {
	testUploadID := "test-upload-id"

	// Mock ListMultipartUploads
	lmuOutput := &s3.ListMultipartUploadsOutput{
		Uploads: []types.MultipartUpload{
			{
				Key:      aws.String("test-path"),
				UploadId: aws.String(testUploadID),
			},
		},
	}
	s.r2Client.EXPECT().ListMultipartUploads(gomock.Any(), gomock.Any()).Return(lmuOutput, nil)

	// Mock ListParts
	listPartsOutput := &s3.ListPartsOutput{
		Parts:       []types.Part{},
		IsTruncated: aws.Bool(false),
	}
	s.r2Client.EXPECT().ListParts(gomock.Any(), gomock.Any()).Return(listPartsOutput, nil)

	// Get writer
	writer, err := s.driver.Writer(context.Background(), testPath, true)
	s.r.NoError(err)

	// Write data
	data := []byte("test data")
	n, err := writer.Write(data)
	s.r.NoError(err)
	s.r.Equal(len(data), n)

	// Mock UploadPart
	uploadPartOutput := &s3.UploadPartOutput{}
	s.r2Client.EXPECT().UploadPart(gomock.Any(), gomock.Any()).Return(uploadPartOutput, nil)

	// Mock CompleteMultipartUpload
	completeMultipartUploadOutput := &s3.CompleteMultipartUploadOutput{}
	s.r2Client.EXPECT().CompleteMultipartUpload(gomock.Any(), gomock.Any()).Return(completeMultipartUploadOutput, nil)

	// Commit and Close
	s.r.NoError(writer.Commit())
	s.r.NoError(writer.Close())
}
