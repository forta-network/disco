package r2

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/forta-network/disco/interfaces"

	dcontext "github.com/distribution/distribution/v3/context"
	storagedriver "github.com/distribution/distribution/v3/registry/storage/driver"
	"github.com/distribution/distribution/v3/registry/storage/driver/base"
	"github.com/distribution/distribution/v3/registry/storage/driver/factory"
)

const driverName = "r2"

// minChunkSize defines the minimum multipart upload chunk size
// R2 API requires multipart upload chunks to be at least 5MB
const minChunkSize = 5 << 20

// maxChunkSize defines the maximum multipart upload chunk size allowed by R2.
const maxChunkSize = 5 << 30

const defaultChunkSize = 2 * minChunkSize

const (
	// defaultMultipartCopyChunkSize defines the default chunk size for all
	// but the last Upload Part - Copy operation of a multipart copy.
	// Empirically, 32 MB is optimal.
	defaultMultipartCopyChunkSize = 32 << 20

	// defaultMultipartCopyMaxConcurrency defines the default maximum number
	// of concurrent Upload Part - Copy operations for a multipart copy.
	defaultMultipartCopyMaxConcurrency = 100

	// defaultMultipartCopyThresholdSize defines the default object size
	// above which multipart copy will be used. (PUT Object - Copy is used
	// for objects at or below this size.)  Empirically, 32 MB is optimal.
	defaultMultipartCopyThresholdSize = 32 << 20
)

// listMax is the largest amount of objects you can request from R2 in a list call
const listMax = 1000

// DriverParameters A struct that encapsulates all of the driver parameters after all values have been set
type DriverParameters struct {
	AccessKey                   string
	SecretKey                   string
	Bucket                      string
	Region                      string
	RegionEndpoint              string
	ForcePathStyle              bool
	Secure                      bool
	SkipVerify                  bool
	ChunkSize                   int64
	MultipartCopyChunkSize      int64
	MultipartCopyMaxConcurrency int64
	MultipartCopyThresholdSize  int64
	RootDirectory               string
}

func init() {
	factory.Register(driverName, &driverFactory{})
}

// driverFactory implements the factory.StorageDriverFactory interface
type driverFactory struct{}

func (factory *driverFactory) Create(parameters map[string]interface{}) (storagedriver.StorageDriver, error) {
	return FromParameters(parameters)
}

type driver struct {
	R2                          interfaces.R2Client
	Bucket                      string
	ChunkSize                   int64
	Encrypt                     bool
	MultipartCopyChunkSize      int64
	MultipartCopyMaxConcurrency int64
	MultipartCopyThresholdSize  int64
	MultipartCombineSmallPart   bool
	RootDirectory               string
}

type baseEmbed struct {
	base.Base
}

// Driver is a storagedriver.StorageDriver implementation backed by Cloudflare's R2
// Objects are stored at absolute keys in the provided bucket.
type Driver struct {
	baseEmbed
}

// FromParameters constructs a new Driver with a given parameters map
// Required parameters:
// - accesskey
// - secretkey
// - region
// - bucket
// - regionendpoint
func FromParameters(parameters map[string]interface{}) (*Driver, error) {
	// Providing no values for these is valid in case the user is authenticating
	// with an IAM on an ec2 instance (in which case the instance credentials will
	// be summoned when GetAuth is called)
	accessKey := parameters["accesskey"]
	if accessKey == nil {
		accessKey = ""
	}
	secretKey := parameters["secretkey"]
	if secretKey == nil {
		secretKey = ""
	}

	regionEndpoint := parameters["regionendpoint"]
	if regionEndpoint == nil {
		regionEndpoint = ""
	}

	forcePathStyleBool := true
	forcePathStyle := parameters["forcepathstyle"]
	switch forcePathStyle := forcePathStyle.(type) {
	case string:
		b, err := strconv.ParseBool(forcePathStyle)
		if err != nil {
			return nil, fmt.Errorf("the forcePathStyle parameter should be a boolean")
		}
		forcePathStyleBool = b
	case bool:
		forcePathStyleBool = forcePathStyle
	case nil:
		// do nothing
	default:
		return nil, fmt.Errorf("the forcePathStyle parameter should be a boolean")
	}

	regionName := parameters["region"]
	if regionName == nil || fmt.Sprint(regionName) == "" {
		return nil, fmt.Errorf("no region parameter provided")
	}
	region := fmt.Sprint(regionName)

	bucket := parameters["bucket"]
	if bucket == nil || fmt.Sprint(bucket) == "" {
		return nil, fmt.Errorf("no bucket parameter provided")
	}

	secureBool := true
	secure := parameters["secure"]
	switch secure := secure.(type) {
	case string:
		b, err := strconv.ParseBool(secure)
		if err != nil {
			return nil, fmt.Errorf("the secure parameter should be a boolean")
		}
		secureBool = b
	case bool:
		secureBool = secure
	case nil:
		// do nothing
	default:
		return nil, fmt.Errorf("the secure parameter should be a boolean")
	}

	skipVerifyBool := false
	skipVerify := parameters["skipverify"]
	switch skipVerify := skipVerify.(type) {
	case string:
		b, err := strconv.ParseBool(skipVerify)
		if err != nil {
			return nil, fmt.Errorf("the skipVerify parameter should be a boolean")
		}
		skipVerifyBool = b
	case bool:
		skipVerifyBool = skipVerify
	case nil:
		// do nothing
	default:
		return nil, fmt.Errorf("the skipVerify parameter should be a boolean")
	}

	keyID := parameters["keyid"]
	if keyID == nil {
		keyID = ""
	}

	chunkSize, err := getParameterAsInt64(parameters, "chunksize", defaultChunkSize, minChunkSize, maxChunkSize)
	if err != nil {
		return nil, err
	}

	multipartCopyChunkSize, err := getParameterAsInt64(parameters, "multipartcopychunksize", defaultMultipartCopyChunkSize, minChunkSize, maxChunkSize)
	if err != nil {
		return nil, err
	}

	multipartCopyMaxConcurrency, err := getParameterAsInt64(parameters, "multipartcopymaxconcurrency", defaultMultipartCopyMaxConcurrency, 1, math.MaxInt64)
	if err != nil {
		return nil, err
	}

	multipartCopyThresholdSize, err := getParameterAsInt64(parameters, "multipartcopythresholdsize", defaultMultipartCopyThresholdSize, 0, maxChunkSize)
	if err != nil {
		return nil, err
	}

	rootDirectory := parameters["rootdirectory"]
	if rootDirectory == nil {
		rootDirectory = ""
	}

	userAgent := parameters["useragent"]
	if userAgent == nil {
		userAgent = ""
	}

	params := DriverParameters{
		AccessKey:                   fmt.Sprint(accessKey),
		SecretKey:                   fmt.Sprint(secretKey),
		Bucket:                      fmt.Sprint(bucket),
		Region:                      region,
		RegionEndpoint:              fmt.Sprint(regionEndpoint),
		ForcePathStyle:              forcePathStyleBool,
		Secure:                      secureBool,
		SkipVerify:                  skipVerifyBool,
		ChunkSize:                   chunkSize,
		MultipartCopyChunkSize:      multipartCopyChunkSize,
		MultipartCopyMaxConcurrency: multipartCopyMaxConcurrency,
		MultipartCopyThresholdSize:  multipartCopyThresholdSize,
		RootDirectory:               fmt.Sprint(rootDirectory),
	}

	return New(params)
}

// getParameterAsInt64 converts parameters[name] to an int64 value (using
// defaultt if nil), verifies it is no smaller than min, and returns it.
func getParameterAsInt64(parameters map[string]interface{}, name string, defaultt int64, min int64, max int64) (int64, error) {
	rv := defaultt
	param := parameters[name]
	switch v := param.(type) {
	case string:
		vv, err := strconv.ParseInt(v, 0, 64)
		if err != nil {
			return 0, fmt.Errorf("%s parameter must be an integer, %v invalid", name, param)
		}
		rv = vv
	case int64:
		rv = v
	case int, uint, int32, uint32, uint64:
		rv = reflect.ValueOf(v).Convert(reflect.TypeOf(rv)).Int()
	case nil:
		// do nothing
	default:
		return 0, fmt.Errorf("invalid value for %s: %#v", name, param)
	}

	if rv < min || rv > max {
		return 0, fmt.Errorf("the %s %#v parameter should be a number between %d and %d (inclusive)", name, rv, min, max)
	}

	return rv, nil
}

func New(params DriverParameters) (*Driver, error) {
	r2Resolver := aws.EndpointResolverWithOptionsFunc(func(service, region string, options ...interface{}) (aws.Endpoint, error) {
		return aws.Endpoint{
			URL: params.RegionEndpoint,
		}, nil
	})

	cfg, err := config.LoadDefaultConfig(context.TODO(),
		config.WithEndpointResolverWithOptions(r2Resolver),
		config.WithRegion("auto"),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(params.AccessKey, params.SecretKey, "")),
	)
	if err != nil {
		return nil, err
	}

	r2Client := s3.NewFromConfig(cfg)

	d := &driver{
		R2:                          r2Client,
		Bucket:                      params.Bucket,
		ChunkSize:                   params.ChunkSize,
		MultipartCopyChunkSize:      params.MultipartCopyChunkSize,
		MultipartCopyMaxConcurrency: params.MultipartCopyMaxConcurrency,
		MultipartCopyThresholdSize:  params.MultipartCopyThresholdSize,
		MultipartCombineSmallPart:   false,
		RootDirectory:               params.RootDirectory,
	}

	return &Driver{
		baseEmbed: baseEmbed{
			Base: base.Base{
				StorageDriver: d,
			},
		},
	}, nil
}

// New constructs a new Driver with the given AWS credentials, region, encryption flag, and
// bucketName
func newFromClient(client interfaces.R2Client, params DriverParameters) (*Driver, error) {
	d := &driver{
		R2:                          client,
		Bucket:                      params.Bucket,
		ChunkSize:                   params.ChunkSize,
		MultipartCopyChunkSize:      params.MultipartCopyChunkSize,
		MultipartCopyMaxConcurrency: params.MultipartCopyMaxConcurrency,
		MultipartCopyThresholdSize:  params.MultipartCopyThresholdSize,
		MultipartCombineSmallPart:   false,
		RootDirectory:               params.RootDirectory,
	}
	return &Driver{
		baseEmbed: baseEmbed{
			Base: base.Base{
				StorageDriver: d,
			},
		},
	}, nil
}

// Implement the storagedriver.StorageDriver interface

func (d *driver) Name() string {
	return driverName
}

// GetContent retrieves the content stored at "path" as a []byte.
func (d *driver) GetContent(ctx context.Context, path string) ([]byte, error) {
	reader, err := d.Reader(ctx, path, 0)
	if err != nil {
		return nil, err
	}
	return io.ReadAll(reader)
}

// PutContent stores the []byte content at a location designated by "path".
func (d *driver) PutContent(ctx context.Context, path string, contents []byte) error {
	_, err := d.R2.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(d.Bucket),
		Key:         aws.String(d.s3Path(path)),
		ContentType: d.getContentType(),
		Body:        bytes.NewReader(contents),
	})
	return parseError(path, err)
}

// Reader retrieves an io.ReadCloser for the content stored at "path" with a
// given byte offset.
func (d *driver) Reader(ctx context.Context, path string, offset int64) (io.ReadCloser, error) {
	resp, err := d.R2.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(d.Bucket),
		Key:    aws.String(d.s3Path(path)),
		Range:  aws.String("bytes=" + strconv.FormatInt(offset, 10) + "-"),
	})
	if err != nil {
		if s3Err, ok := err.(awserr.Error); ok && s3Err.Code() == "InvalidRange" {
			return io.NopCloser(bytes.NewReader(nil)), nil
		}

		return nil, parseError(path, err)
	}
	return resp.Body, nil
}

// Writer returns a FileWriter which will store the content written to it
// at the location designated by "path" after the call to Commit.
func (d *driver) Writer(ctx context.Context, path string, appendParam bool) (storagedriver.FileWriter, error) {
	key := d.s3Path(path)
	if !appendParam {
		// TODO (brianbland): cancel other uploads at this path
		resp, err := d.R2.CreateMultipartUpload(ctx, &s3.CreateMultipartUploadInput{
			Bucket:      aws.String(d.Bucket),
			Key:         aws.String(key),
			ContentType: d.getContentType(),
		})
		if err != nil {
			return nil, err
		}
		return d.newWriter(key, *resp.UploadId, nil), nil
	}

	listMultipartUploadsInput := &s3.ListMultipartUploadsInput{
		Bucket: aws.String(d.Bucket),
		Prefix: aws.String(key),
	}
	for {
		resp, err := d.R2.ListMultipartUploads(ctx, listMultipartUploadsInput)
		if err != nil {
			return nil, parseError(path, err)
		}

		// resp.Uploads can only be empty on the first call
		// if there were no more results to return after the first call, resp.IsTruncated would have been false
		// and the loop would be exited without recalling ListMultipartUploads
		if len(resp.Uploads) == 0 {
			break
		}

		var allParts []types.Part
		for _, multi := range resp.Uploads {
			if key != *multi.Key {
				continue
			}

			partsList, err := d.R2.ListParts(ctx, &s3.ListPartsInput{
				Bucket:   aws.String(d.Bucket),
				Key:      aws.String(key),
				UploadId: multi.UploadId,
			})
			if err != nil {
				return nil, parseError(path, err)
			}
			allParts = append(allParts, partsList.Parts...)
			for *partsList.IsTruncated {
				partsList, err = d.R2.ListParts(ctx, &s3.ListPartsInput{
					Bucket:           aws.String(d.Bucket),
					Key:              aws.String(key),
					UploadId:         multi.UploadId,
					PartNumberMarker: partsList.NextPartNumberMarker,
				})
				if err != nil {
					return nil, parseError(path, err)
				}
				allParts = append(allParts, partsList.Parts...)
			}
			return d.newWriter(key, *multi.UploadId, allParts), nil
		}

		// resp.NextUploadIdMarker must have at least one element or we would have returned not found
		listMultipartUploadsInput.UploadIdMarker = resp.NextUploadIdMarker

		// from the s3 api docs, IsTruncated "specifies whether (true) or not (false) all of the results were returned"
		// if everything has been returned, break
		if resp.IsTruncated == nil || !*resp.IsTruncated {
			break
		}
	}
	return nil, storagedriver.PathNotFoundError{Path: path}
}

// Stat retrieves the FileInfo for the given path, including the current size
// in bytes and the creation time.
func (d *driver) Stat(ctx context.Context, path string) (storagedriver.FileInfo, error) {
	resp, err := d.R2.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
		Bucket:  aws.String(d.Bucket),
		Prefix:  aws.String(d.s3Path(path)),
		MaxKeys: aws.Int32(1),
	})
	if err != nil {
		return nil, err
	}

	fi := storagedriver.FileInfoFields{
		Path: path,
	}

	if len(resp.Contents) == 1 {
		if *resp.Contents[0].Key != d.s3Path(path) {
			fi.IsDir = true
		} else {
			fi.IsDir = false
			fi.Size = *resp.Contents[0].Size
			fi.ModTime = *resp.Contents[0].LastModified
		}
	} else if len(resp.CommonPrefixes) == 1 {
		fi.IsDir = true
	} else {
		return nil, storagedriver.PathNotFoundError{Path: path}
	}

	return storagedriver.FileInfoInternal{FileInfoFields: fi}, nil
}

// List returns a list of the objects that are direct descendants of the given path.
func (d *driver) List(ctx context.Context, opath string) ([]string, error) {
	path := opath
	if path != "/" && path[len(path)-1] != '/' {
		path = path + "/"
	}

	// This is to cover for the cases when the rootDirectory of the driver is either "" or "/".
	// In those cases, there is no root prefix to replace and we must actually add a "/" to all
	// results in order to keep them as valid paths as recognized by storagedriver.PathRegexp
	prefix := ""
	if d.s3Path("") == "" {
		prefix = "/"
	}

	resp, err := d.R2.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
		Bucket:    aws.String(d.Bucket),
		Prefix:    aws.String(d.s3Path(path)),
		Delimiter: aws.String("/"),
		MaxKeys:   aws.Int32(listMax),
	})
	if err != nil {
		return nil, parseError(opath, err)
	}

	files := []string{}
	directories := []string{}

	for {
		for _, key := range resp.Contents {
			files = append(files, strings.Replace(*key.Key, d.s3Path(""), prefix, 1))
		}

		for _, commonPrefix := range resp.CommonPrefixes {
			commonPrefix := *commonPrefix.Prefix
			directories = append(directories, strings.Replace(commonPrefix[0:len(commonPrefix)-1], d.s3Path(""), prefix, 1))
		}

		if *resp.IsTruncated {
			resp, err = d.R2.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
				Bucket:            aws.String(d.Bucket),
				Prefix:            aws.String(d.s3Path(path)),
				Delimiter:         aws.String("/"),
				MaxKeys:           aws.Int32(listMax),
				ContinuationToken: resp.NextContinuationToken,
			})
			if err != nil {
				return nil, err
			}
		} else {
			break
		}
	}

	if opath != "/" {
		if len(files) == 0 && len(directories) == 0 {
			// Treat empty response as missing directory, since we don't actually
			// have directories in s3.
			return nil, storagedriver.PathNotFoundError{Path: opath}
		}
	}

	return append(files, directories...), nil
}

// Move moves an object stored at sourcePath to destPath, removing the original
// object.
func (d *driver) Move(ctx context.Context, sourcePath string, destPath string) error {
	/* This is terrible, but aws doesn't have an actual move. */
	if err := d.copy(ctx, sourcePath, destPath); err != nil {
		return err
	}
	return d.Delete(ctx, sourcePath)
}

// copy copies an object stored at sourcePath to destPath.
func (d *driver) copy(ctx context.Context, sourcePath string, destPath string) error {
	// R2 can copy objects up to 5 GB in size with a single PUT Object - Copy
	// operation. For larger objects, the multipart upload API must be used.
	//
	// Empirically, multipart copy is fastest with 32 MB parts and is faster
	// than PUT Object - Copy for objects larger than 32 MB.

	fileInfo, err := d.Stat(ctx, sourcePath)
	if err != nil {
		return parseError(sourcePath, err)
	}

	if fileInfo.Size() <= d.MultipartCopyThresholdSize {
		_, err := d.R2.CopyObject(ctx, &s3.CopyObjectInput{
			Bucket:      aws.String(d.Bucket),
			Key:         aws.String(d.s3Path(destPath)),
			ContentType: d.getContentType(),
			CopySource:  aws.String(d.Bucket + "/" + d.s3Path(sourcePath)),
		})
		if err != nil {
			return parseError(sourcePath, err)
		}
		return nil
	}

	createResp, err := d.R2.CreateMultipartUpload(ctx, &s3.CreateMultipartUploadInput{
		Bucket:      aws.String(d.Bucket),
		Key:         aws.String(d.s3Path(destPath)),
		ContentType: d.getContentType(),
	})
	if err != nil {
		return err
	}

	numParts := (fileInfo.Size() + d.MultipartCopyChunkSize - 1) / d.MultipartCopyChunkSize
	completedParts := make([]types.CompletedPart, numParts)
	errChan := make(chan error, numParts)
	limiter := make(chan struct{}, d.MultipartCopyMaxConcurrency)

	for i := range completedParts {
		i := int64(i)
		go func() {
			limiter <- struct{}{}
			firstByte := i * d.MultipartCopyChunkSize
			lastByte := firstByte + d.MultipartCopyChunkSize - 1
			if lastByte >= fileInfo.Size() {
				lastByte = fileInfo.Size() - 1
			}
			uploadResp, err := d.R2.UploadPartCopy(ctx, &s3.UploadPartCopyInput{
				Bucket:          aws.String(d.Bucket),
				CopySource:      aws.String(d.Bucket + "/" + d.s3Path(sourcePath)),
				Key:             aws.String(d.s3Path(destPath)),
				PartNumber:      aws.Int32(int32(i + 1)),
				UploadId:        createResp.UploadId,
				CopySourceRange: aws.String(fmt.Sprintf("bytes=%d-%d", firstByte, lastByte)),
			})
			if err == nil {
				completedParts[i] = types.CompletedPart{
					ETag:       uploadResp.CopyPartResult.ETag,
					PartNumber: aws.Int32(int32(i + 1)),
				}
			}
			errChan <- err
			<-limiter
		}()
	}

	for range completedParts {
		err := <-errChan
		if err != nil {
			return err
		}
	}

	_, err = d.R2.CompleteMultipartUpload(ctx, &s3.CompleteMultipartUploadInput{
		Bucket:          aws.String(d.Bucket),
		Key:             aws.String(d.s3Path(destPath)),
		UploadId:        createResp.UploadId,
		MultipartUpload: &types.CompletedMultipartUpload{Parts: completedParts},
	})
	return err
}

// Delete recursively deletes all objects stored at "path" and its subpaths.
// We must be careful since R2 does not guarantee read after delete consistency
func (d *driver) Delete(ctx context.Context, path string) error {
	s3Objects := make([]types.ObjectIdentifier, 0, listMax)
	s3Path := d.s3Path(path)
	listObjectsInput := &s3.ListObjectsV2Input{
		Bucket: aws.String(d.Bucket),
		Prefix: aws.String(s3Path),
	}

	for {
		// list all the objects
		resp, err := d.R2.ListObjectsV2(ctx, listObjectsInput)

		// resp.Contents can only be empty on the first call
		// if there were no more results to return after the first call, resp.IsTruncated would have been false
		// and the loop would exit without recalling ListObjects
		if err != nil || len(resp.Contents) == 0 {
			return storagedriver.PathNotFoundError{Path: path}
		}

		for _, key := range resp.Contents {
			// Skip if we encounter a key that is not a subpath (so that deleting "/a" does not delete "/ab").
			if len(*key.Key) > len(s3Path) && (*key.Key)[len(s3Path)] != '/' {
				continue
			}
			s3Objects = append(s3Objects, types.ObjectIdentifier{
				Key: key.Key,
			})
		}

		// Delete objects only if the list is not empty, otherwise R2 API returns a cryptic error
		if len(s3Objects) > 0 {
			// Kept for sanity, might apply to Cloudflare R2 as well.
			// NOTE: according to AWS docs https://docs.aws.amazon.com/AmazonS3/latest/API/API_ListObjectsV2.html
			// by default the response returns up to 1,000 key names. The response _might_ contain fewer keys but it will never contain more.
			// 10000 keys is coincidentally (?) also the max number of keys that can be deleted in a single Delete operation, so we'll just smack
			// Delete here straight away and reset the object slice when successful.
			resp, err := d.R2.DeleteObjects(ctx, &s3.DeleteObjectsInput{
				Bucket: aws.String(d.Bucket),
				Delete: &types.Delete{
					Objects: s3Objects,
					Quiet:   aws.Bool(false),
				},
			})
			if err != nil {
				return err
			}

			if len(resp.Errors) > 0 {
				// NOTE: AWS SDK s3.Error does not implement error interface which
				// is pretty intensely sad, so we have to do away with this for now.
				errs := make([]error, 0, len(resp.Errors))
				for _, err := range resp.Errors {
					errs = append(errs, errors.New(*err.Message))
				}
				return storagedriver.Error{
					DriverName: driverName,
					// Errs:       errs,
				}
			}
		}
		// NOTE: we don't want to reallocate
		// the slice so we simply "reset" it
		s3Objects = s3Objects[:0]

		// resp.Contents must have at least one element or we would have returned not found
		listObjectsInput.StartAfter = resp.Contents[len(resp.Contents)-1].Key

		// from the s3 api docs, IsTruncated "specifies whether (true) or not (false) all of the results were returned"
		// if everything has been returned, break
		if resp.IsTruncated == nil || !*resp.IsTruncated {
			break
		}
	}

	return nil
}

// URLFor returns a URL which may be used to retrieve the content stored at the given path.
// May return an UnsupportedMethodErr in certain StorageDriver implementations.
func (d *driver) URLFor(_ context.Context, _ string, options map[string]interface{}) (string, error) {
	methodString := http.MethodGet
	method, ok := options["method"]
	if ok {
		methodString, ok = method.(string)
		if !ok || (methodString != http.MethodGet && methodString != http.MethodHead) {
			return "", storagedriver.ErrUnsupportedMethod{}
		}
	}

	expiresIn := 20 * time.Minute
	expires, ok := options["expiry"]
	if ok {
		et, ok := expires.(time.Time)
		if ok {
			expiresIn = time.Until(et)
		}
	}

	var req *request.Request

	switch methodString {
	case http.MethodGet:
		// req, _ = d.R2.GetObject(ctx, &s3.GetObjectInput{
		// 	Bucket: aws.String(d.Bucket),
		// 	Key:    aws.String(d.s3Path(path)),
		// })
	case http.MethodHead:
		// req, _ = d.R2.HeadObjectRequest(&s3.HeadObjectInput{
		// 	Bucket: aws.String(d.Bucket),
		// 	Key:    aws.String(d.s3Path(path)),
		// })
	default:
		panic("unreachable")
	}

	return req.Presign(expiresIn)
}

// Walk traverses a filesystem defined within driver, starting
// from the given path, calling f on each file
func (d *driver) Walk(ctx context.Context, from string, f storagedriver.WalkFn) error {
	path := from
	if !strings.HasSuffix(path, "/") {
		path = path + "/"
	}

	prefix := ""
	if d.s3Path("") == "" {
		prefix = "/"
	}

	var objectCount int64
	if err := d.doWalk(ctx, &objectCount, d.s3Path(path), prefix, f); err != nil {
		return err
	}

	// R2 doesn't have the concept of empty directories, so it'll return path not found if there are no objects
	if objectCount == 0 {
		return storagedriver.PathNotFoundError{Path: from}
	}

	return nil
}

func (d *driver) doWalk(parentCtx context.Context, objectCount *int64, path, prefix string, f storagedriver.WalkFn) error {
	var (
		retError error
		// the most recent directory walked for de-duping
		prevDir string
		// the most recent skip directory to avoid walking over undesirable files
		prevSkipDir string
	)
	prevDir = strings.Replace(path, d.s3Path(""), prefix, 1)

	listObjectsInput := &s3.ListObjectsV2Input{
		Bucket:  aws.String(d.Bucket),
		Prefix:  aws.String(path),
		MaxKeys: aws.Int32(listMax),
	}

	ctx, done := dcontext.WithTrace(parentCtx)
	defer done("s3aws.ListObjectsV2Pages(%s)", path)

	// When the "delimiter" argument is omitted, the R2 list API will list all objects in the bucket
	// recursively, omitting directory paths. Objects are listed in sorted, depth-first order so we
	// can infer all the directories by comparing each object path to the last one we saw.
	// See: https://docs.aws.amazon.com/AmazonS3/latest/userguide/ListingKeysUsingAPIs.html

	// With files returned in sorted depth-first order, directories are inferred in the same order.
	// ErrSkipDir is handled by explicitly skipping over any files under the skipped directory. This may be sub-optimal
	// for extreme edge cases but for the general use case in a registry, this is orders of magnitude
	// faster than a more explicit recursive implementation.
	// TODO: broken
	objects, listObjectErr := d.R2.ListObjectsV2(ctx, listObjectsInput, nil)
	walkInfos := make([]storagedriver.FileInfoInternal, 0, len(objects.Contents))

	for _, file := range objects.Contents {
		filePath := strings.Replace(*file.Key, d.s3Path(""), prefix, 1)

		// get a list of all inferred directories between the previous directory and this file
		dirs := directoryDiff(prevDir, filePath)
		if len(dirs) > 0 {
			for _, dir := range dirs {
				walkInfos = append(walkInfos, storagedriver.FileInfoInternal{
					FileInfoFields: storagedriver.FileInfoFields{
						IsDir: true,
						Path:  dir,
					},
				})
				prevDir = dir
			}
		}

		walkInfos = append(walkInfos, storagedriver.FileInfoInternal{
			FileInfoFields: storagedriver.FileInfoFields{
				IsDir:   false,
				Size:    *file.Size,
				ModTime: *file.LastModified,
				Path:    filePath,
			},
		})
	}

	for _, walkInfo := range walkInfos {
		// skip any results under the last skip directory
		if prevSkipDir != "" && strings.HasPrefix(walkInfo.Path(), prevSkipDir) {
			continue
		}

		err := f(walkInfo)
		*objectCount++

		if err != nil {
			if errors.Is(err, storagedriver.ErrSkipDir) {
				if walkInfo.IsDir() {
					prevSkipDir = walkInfo.Path()
					continue
				}
				// is file, stop gracefully
				return err
			}
			retError = err
			return err
		}
	}

	if retError != nil {
		return retError
	}

	if listObjectErr != nil {
		return listObjectErr
	}

	return nil
}

// directoryDiff finds all directories that are not in common between
// the previous and current paths in sorted order.
//
// # Examples
//
//	directoryDiff("/path/to/folder", "/path/to/folder/folder/file")
//	// => [ "/path/to/folder/folder" ]
//
//	directoryDiff("/path/to/folder/folder1", "/path/to/folder/folder2/file")
//	// => [ "/path/to/folder/folder2" ]
//
//	directoryDiff("/path/to/folder/folder1/file", "/path/to/folder/folder2/file")
//	// => [ "/path/to/folder/folder2" ]
//
//	directoryDiff("/path/to/folder/folder1/file", "/path/to/folder/folder2/folder1/file")
//	// => [ "/path/to/folder/folder2", "/path/to/folder/folder2/folder1" ]
//
//	directoryDiff("/", "/path/to/folder/folder/file")
//	// => [ "/path", "/path/to", "/path/to/folder", "/path/to/folder/folder" ]
func directoryDiff(prev, current string) []string {
	var paths []string

	if prev == "" || current == "" {
		return paths
	}

	parent := current
	for {
		parent = filepath.Dir(parent)
		if parent == "/" || parent == prev || strings.HasPrefix(prev, parent) {
			break
		}
		paths = append(paths, parent)
	}
	reverse(paths)
	return paths
}

func reverse(s []string) {
	for i, j := 0, len(s)-1; i < j; i, j = i+1, j-1 {
		s[i], s[j] = s[j], s[i]
	}
}

func (d *driver) s3Path(path string) string {
	return strings.TrimLeft(strings.TrimRight(d.RootDirectory, "/")+path, "/")
}

// S3BucketKey returns the s3 bucket key for the given storage driver path.
func (d *Driver) S3BucketKey(path string) string {
	return d.StorageDriver.(*driver).s3Path(path)
}

func parseError(path string, err error) error {
	if s3Err, ok := err.(awserr.Error); ok && s3Err.Code() == "NoSuchKey" {
		return storagedriver.PathNotFoundError{Path: path}
	}

	return err
}

func (d *driver) getContentType() *string {
	return aws.String("application/octet-stream")
}

// writer attempts to upload parts to R2 in a buffered fashion where the last
// part is at least as large as the chunksize, so the multipart upload could be
// cleanly resumed in the future. This is violated if Close is called after less
// than a full chunk is written.
type writer struct {
	driver      *driver
	key         string
	uploadID    string
	parts       []types.Part
	size        int64
	readyPart   []byte
	pendingPart []byte
	closed      bool
	committed   bool
	cancelled   bool
}

func (d *driver) newWriter(key, uploadID string, parts []types.Part) storagedriver.FileWriter {
	var size int64
	for _, part := range parts {
		size += *part.Size
	}
	return &writer{
		driver:   d,
		key:      key,
		uploadID: uploadID,
		parts:    parts,
		size:     size,
	}
}

type completedParts []types.CompletedPart

func (a completedParts) Len() int           { return len(a) }
func (a completedParts) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a completedParts) Less(i, j int) bool { return *a[i].PartNumber < *a[j].PartNumber }

func (w *writer) Write(p []byte) (int, error) {
	if w.closed {
		return 0, fmt.Errorf("already closed")
	} else if w.committed {
		return 0, fmt.Errorf("already committed")
	} else if w.cancelled {
		return 0, fmt.Errorf("already cancelled")
	}

	// If the last written part is smaller than minChunkSize, we need to make a
	// new multipart upload :sadface:
	if len(w.parts) > 0 && int(*w.parts[len(w.parts)-1].Size) < minChunkSize {
		var completedUploadedParts completedParts
		for _, part := range w.parts {
			completedUploadedParts = append(completedUploadedParts, types.CompletedPart{
				ETag:       part.ETag,
				PartNumber: part.PartNumber,
			})
		}

		sort.Sort(completedUploadedParts)

		ctx := context.Background()
		_, err := w.driver.R2.CompleteMultipartUpload(ctx, &s3.CompleteMultipartUploadInput{
			Bucket:   aws.String(w.driver.Bucket),
			Key:      aws.String(w.key),
			UploadId: aws.String(w.uploadID),
			MultipartUpload: &types.CompletedMultipartUpload{
				Parts: completedUploadedParts,
			},
		})
		if err != nil {
			w.driver.R2.AbortMultipartUpload(ctx, &s3.AbortMultipartUploadInput{
				Bucket:   aws.String(w.driver.Bucket),
				Key:      aws.String(w.key),
				UploadId: aws.String(w.uploadID),
			})
			return 0, err
		}

		resp, err := w.driver.R2.CreateMultipartUpload(ctx, &s3.CreateMultipartUploadInput{
			Bucket:      aws.String(w.driver.Bucket),
			Key:         aws.String(w.key),
			ContentType: w.driver.getContentType(),
		})
		if err != nil {
			return 0, err
		}
		w.uploadID = *resp.UploadId

		// If the entire written file is smaller than minChunkSize, we need to make
		// a new part from scratch :double sad face:
		if w.size < minChunkSize {
			resp, err := w.driver.R2.GetObject(ctx, &s3.GetObjectInput{
				Bucket: aws.String(w.driver.Bucket),
				Key:    aws.String(w.key),
			})
			if err != nil {
				return 0, err
			}
			defer resp.Body.Close()
			w.parts = nil
			w.readyPart, err = io.ReadAll(resp.Body)
			if err != nil {
				return 0, err
			}
		} else {
			// Otherwise we can use the old file as the new first part
			copyPartResp, err := w.driver.R2.UploadPartCopy(ctx, &s3.UploadPartCopyInput{
				Bucket:     aws.String(w.driver.Bucket),
				CopySource: aws.String(w.driver.Bucket + "/" + w.key),
				Key:        aws.String(w.key),
				PartNumber: aws.Int32(1),
				UploadId:   resp.UploadId,
			})
			if err != nil {
				return 0, err
			}
			w.parts = []types.Part{
				{
					ETag:       copyPartResp.CopyPartResult.ETag,
					PartNumber: aws.Int32(1),
					Size:       aws.Int64(w.size),
				},
			}
		}
	}

	var n int

	for len(p) > 0 {
		// If no parts are ready to write, fill up the first part
		if neededBytes := int(w.driver.ChunkSize) - len(w.readyPart); neededBytes > 0 {
			if len(p) >= neededBytes {
				w.readyPart = append(w.readyPart, p[:neededBytes]...)
				n += neededBytes
				p = p[neededBytes:]
			} else {
				w.readyPart = append(w.readyPart, p...)
				n += len(p)
				p = nil
			}
		}

		if neededBytes := int(w.driver.ChunkSize) - len(w.pendingPart); neededBytes > 0 {
			if len(p) >= neededBytes {
				w.pendingPart = append(w.pendingPart, p[:neededBytes]...)
				n += neededBytes
				p = p[neededBytes:]
				err := w.flushPart()
				if err != nil {
					w.size += int64(n)
					return n, err
				}
			} else {
				w.pendingPart = append(w.pendingPart, p...)
				n += len(p)
				p = nil
			}
		}
	}
	w.size += int64(n)
	return n, nil
}

func (w *writer) Size() int64 {
	return w.size
}

func (w *writer) Close() error {
	if w.closed {
		return fmt.Errorf("already closed")
	}
	w.closed = true
	return w.flushPart()
}

func (w *writer) Cancel() error {
	ctx := context.Background()
	if w.closed {
		return fmt.Errorf("already closed")
	} else if w.committed {
		return fmt.Errorf("already committed")
	}
	w.cancelled = true
	_, err := w.driver.R2.AbortMultipartUpload(ctx, &s3.AbortMultipartUploadInput{
		Bucket:   aws.String(w.driver.Bucket),
		Key:      aws.String(w.key),
		UploadId: aws.String(w.uploadID),
	})
	return err
}

func (w *writer) Commit() error {
	if w.closed {
		return fmt.Errorf("already closed")
	} else if w.committed {
		return fmt.Errorf("already committed")
	} else if w.cancelled {
		return fmt.Errorf("already cancelled")
	}
	err := w.flushPart()
	if err != nil {
		return err
	}
	w.committed = true

	var completedUploadedParts completedParts
	for _, part := range w.parts {
		completedUploadedParts = append(completedUploadedParts, types.CompletedPart{
			ETag:       part.ETag,
			PartNumber: part.PartNumber,
		})
	}

	// This is an AWS S3 edge case, but we're keeping it for sanity
	//
	//
	// This is an edge case when we are trying to upload an empty chunk of data using
	// a MultiPart upload. As a result we are trying to complete the MultipartUpload
	// with an empty slice of `completedUploadedParts` which will always lead to 400
	// being returned from S3 See: https://docs.aws.amazon.com/sdk-for-go/api/service/s3/#CompletedMultipartUpload
	// Solution: we upload an empty i.e. 0 byte part as a single part and then append it
	// to the completedUploadedParts slice used to complete the Multipart upload.
	if len(w.parts) == 0 {
		ctx := context.Background()
		resp, err := w.driver.R2.UploadPart(ctx, &s3.UploadPartInput{
			Bucket:     aws.String(w.driver.Bucket),
			Key:        aws.String(w.key),
			PartNumber: aws.Int32(1),
			UploadId:   aws.String(w.uploadID),
			Body:       bytes.NewReader(nil),
		})
		if err != nil {
			return err
		}

		completedUploadedParts = append(completedUploadedParts, types.CompletedPart{
			ETag:       resp.ETag,
			PartNumber: aws.Int32(1),
		})
	}

	sort.Sort(completedUploadedParts)
	ctx := context.Background()
	_, err = w.driver.R2.CompleteMultipartUpload(ctx, &s3.CompleteMultipartUploadInput{
		Bucket:   aws.String(w.driver.Bucket),
		Key:      aws.String(w.key),
		UploadId: aws.String(w.uploadID),
		MultipartUpload: &types.CompletedMultipartUpload{
			Parts: completedUploadedParts,
		},
	})
	if err != nil {
		w.driver.R2.AbortMultipartUpload(ctx, &s3.AbortMultipartUploadInput{
			Bucket:   aws.String(w.driver.Bucket),
			Key:      aws.String(w.key),
			UploadId: aws.String(w.uploadID),
		})
		return err
	}
	return nil
}

// flushPart flushes buffers to write a part to R2.
// Only called by Write (with both buffers full) and Close/Commit (always)
func (w *writer) flushPart() error {
	if len(w.readyPart) == 0 && len(w.pendingPart) == 0 {
		// nothing to write
		return nil
	}
	if len(w.pendingPart) < int(w.driver.ChunkSize) {
		// closing with a small pending part
		// combine ready and pending to avoid writing a small part
		w.readyPart = append(w.readyPart, w.pendingPart...)
		w.pendingPart = nil
	}
	ctx := context.Background()

	partNumber := aws.Int32(int32(len(w.parts) + 1))
	resp, err := w.driver.R2.UploadPart(ctx, &s3.UploadPartInput{
		Bucket:     aws.String(w.driver.Bucket),
		Key:        aws.String(w.key),
		PartNumber: partNumber,
		UploadId:   aws.String(w.uploadID),
		Body:       bytes.NewReader(w.readyPart),
	})
	if err != nil {
		return err
	}
	w.parts = append(w.parts, types.Part{
		ETag:       resp.ETag,
		PartNumber: partNumber,
		Size:       aws.Int64(int64(len(w.readyPart))),
	})
	w.readyPart = w.pendingPart
	w.pendingPart = nil
	return w.flushPart()
}
