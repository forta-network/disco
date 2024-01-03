package e2e_test

import (
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/forta-network/disco/utils"
	ipfsapi "github.com/ipfs/go-ipfs-api"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

var (
	processStartWaitSeconds = 60
	pushImageRef            = "localhost:1970/test"

	expectedImageSha = "35ff92bfc7e822eab96fe3d712164f6b547c3acffc8691b80528d334283849ab"

	expectedImageCidCacheOnly = "bafybeibv76jl7r7ielvls37d24jbmt3lkr6dvt74q2i3qbji2m2cqocjvm"

	unexpectedImageCid            = "bafybeielvnt5apaxbk6chthc4dc3p6vscpx3ai4uvti7gwh253j7facsxu"
	unexpectedPullImageRef        = fmt.Sprintf("localhost:1970/%s", unexpectedImageCid)
	expectedPullImageRefCacheOnly = fmt.Sprintf("localhost:1970/%s", expectedImageSha)

	reposPath = "/docker/registry/v2/repositories/"

	expectedSha256Repo       = path.Join(reposPath, expectedImageSha)
	expectedCidRepoCacheOnly = path.Join(reposPath, expectedImageCidCacheOnly)

	expectedManifestBlob = "/docker/registry/v2/blobs/sha256/35/35ff92bfc7e822eab96fe3d712164f6b547c3acffc8691b80528d334283849ab/data"
	expectedConfigBlob   = "/docker/registry/v2/blobs/sha256/16/165538b9f99adf71764e6e01627236bc7de03587ef8c39b621c159491466465e/data"
	expectedLayerBlob1   = "/docker/registry/v2/blobs/sha256/04/04479ea8ab2597ba1679773da48df06a9e646e3e7b67b0eb2c8c0bc6c51eb598/data"
	expectedLayerBlob2   = "/docker/registry/v2/blobs/sha256/d9/d96e79a5881296813985815a1fa73e2441e72769541b1fb32a0e14f2acf4d659/data"

	expectedCidTagCacheOnly = path.Join(reposPath, expectedImageSha, "_manifests", "tags", expectedImageCidCacheOnly)
)

type E2ETestSuite struct {
	r *require.Assertions

	ipfsClient1 *ipfsapi.Shell
	ipfsClient2 *ipfsapi.Shell

	suite.Suite
}

func TestE2E(t *testing.T) {
	if os.Getenv("E2E_TEST") != "1" {
		return
	}
	suite.Run(t, &E2ETestSuite{})
}

func (s *E2ETestSuite) SetupTest() {
	s.r = s.Require()

	s.r.NoError(os.RemoveAll("testdir"))
	s.r.NoError(os.MkdirAll("testdir", 0777))
	s.startCleanIpfs()
}

func (s *E2ETestSuite) startDisco(configPath string) {
	os.Setenv("REGISTRY_CONFIGURATION_PATH", configPath)
	discoCmd := exec.Command("./../build/disco")
	discoCmd.Stdout = os.Stdout
	discoCmd.Stderr = os.Stdout
	s.r.NoError(discoCmd.Start())
}

func (s *E2ETestSuite) startCleanIpfs() {
	_ = exec.Command("pkill", "ipfs").Run()
	s.r.NoError(os.RemoveAll("testdir/.ipfs1"))
	s.r.NoError(os.RemoveAll("testdir/.ipfs2"))

	os.Setenv("IPFS_PATH", "testdir/.ipfs1")
	s.r.NoError(exec.Command("ipfs", "init").Run())
	s.r.NoError(exec.Command("cp", "config1", "testdir/.ipfs1/config").Run())
	s.r.NoError(exec.Command("ipfs", "daemon").Start())

	s.ipfsClient1 = ipfsapi.NewShell("http://localhost:5051")
	s.ensureAvailability("ipfs", func() error {
		_, err := s.ipfsClient1.FilesLs(context.Background(), "/")
		if err != nil {
			return err
		}
		return nil
	})

	os.Setenv("IPFS_PATH", "testdir/.ipfs2")
	s.r.NoError(exec.Command("ipfs", "init").Run())
	s.r.NoError(exec.Command("cp", "config2", "testdir/.ipfs2/config").Run())
	s.r.NoError(exec.Command("ipfs", "daemon").Start())

	s.ipfsClient2 = ipfsapi.NewShell("http://localhost:5052")
	s.ensureAvailability("ipfs", func() error {
		_, err := s.ipfsClient2.FilesLs(context.Background(), "/")
		if err != nil {
			return err
		}
		return nil
	})
}

func (s *E2ETestSuite) ensureAvailability(name string, check func() error) {
	var err error
	for i := 0; i < processStartWaitSeconds; i++ {
		time.Sleep(time.Second)
		if err = check(); err == nil {
			return
		}
	}
	s.r.FailNowf("", "failed to ensure '%s' start: %v", name, err)
}

func (s *E2ETestSuite) TearDownTest() {
	_ = exec.Command("pkill", "ipfs").Run()
	_ = exec.Command("pkill", "disco").Run()
}

func (s *E2ETestSuite) TestPushVerify() {
	s.startDisco("./disco-e2e-config.yml")

	s.r.NoError(exec.Command("docker", "push", pushImageRef).Run())

	s.verifyFiles()
}

func (s *E2ETestSuite) verifyFiles() {
	// verify that the repos with sha256 and CID names exist in both stores
	// verify that the blobs exist in both stores
	for _, contentPath := range []string{
		expectedSha256Repo,

		expectedManifestBlob,
		expectedConfigBlob,
		expectedLayerBlob1,
		expectedLayerBlob2,
	} {
		ipfsInfo, err := s.ipfsClient2.FilesStat(context.Background(), contentPath)
		s.r.NoError(err, contentPath)
		s.r.Greater(ipfsInfo.CumulativeSize, uint64(0), contentPath)

		fsInfo, err := os.Stat(path.Join("testdir/cache", contentPath))
		s.r.NoError(err, contentPath)
		s.r.Greater(fsInfo.Size(), int64(0), contentPath)
	}

	repos, err := s.ipfsClient1.FilesLs(context.Background(), reposPath)
	s.r.NoError(err)
	for _, repo := range repos {
		if utils.IsCIDv1(repo.Name) {
			return // we're good
		}
	}
	repos, err = s.ipfsClient2.FilesLs(context.Background(), reposPath)
	s.r.NoError(err)
	for _, repo := range repos {
		if utils.IsCIDv1(repo.Name) {
			return // we're good
		}
	}
	s.r.FailNow("no cid repos found in ipfs")
}

func getImageCid() (foundCid string) {
	filepath.WalkDir("testdir/cache", func(currPath string, d fs.DirEntry, err error) error {
		if len(foundCid) > 0 {
			return nil
		}
		if strings.Contains(currPath, "bafybei") {
			for _, segment := range strings.Split(currPath, "/") {
				if strings.Contains(segment, "bafybei") {
					foundCid = segment
					return nil
				}
			}
		}
		return nil
	})
	return
}

func getImagePullRef(imageName string) string {
	return path.Join("localhost:1970", imageName)
}

func (s *E2ETestSuite) TestPurgeIPFS_Pull() {
	s.startDisco("./disco-e2e-config.yml")

	s.r.NoError(exec.Command("docker", "push", pushImageRef).Run())
	imageCid := getImageCid()
	imageCidPullRef := getImagePullRef(imageCid)

	// delete from ipfs (primary store)
	s.startCleanIpfs()

	// pull
	s.r.NoError(exec.Command("docker", "pull", imageCidPullRef).Run())

	// it was able to pull without needing ipfs
	_, err := s.ipfsClient1.FilesStat(context.Background(), "/docker")
	s.Error(err)
}

func (s *E2ETestSuite) TestPurgeIPFS_PushAgainPull() {
	s.startDisco("./disco-e2e-config.yml")

	s.r.NoError(exec.Command("docker", "push", pushImageRef).Run())
	imageCid := getImageCid()
	imageCidPullRef := getImagePullRef(imageCid)

	// delete from ipfs (primary store)
	s.startCleanIpfs()

	// push again
	s.r.NoError(exec.Command("docker", "push", pushImageRef).Run())

	s.verifyFiles()

	s.r.NoError(exec.Command("docker", "pull", imageCidPullRef).Run())
}

func (s *E2ETestSuite) TestPurgeCache_Pull() {
	s.startDisco("./disco-e2e-config.yml")

	s.r.NoError(exec.Command("docker", "push", pushImageRef).Run())
	imageCid := getImageCid()
	imageCidPullRef := getImagePullRef(imageCid)

	// delete from filestore (secondary store)
	s.r.NoError(os.RemoveAll("testdir/cache"))

	// pull
	s.r.NoError(exec.Command("docker", "pull", imageCidPullRef).Run())
}

func (s *E2ETestSuite) TestPurgeCache_PushAgainPull() {
	s.startDisco("./disco-e2e-config.yml")

	s.r.NoError(exec.Command("docker", "push", pushImageRef).Run())
	imageCid := getImageCid()
	imageCidPullRef := getImagePullRef(imageCid)

	// delete from filestore (secondary store)
	s.r.NoError(os.RemoveAll("testdir/cache"))

	// push again
	s.r.NoError(exec.Command("docker", "push", pushImageRef).Run())

	s.verifyFiles()

	s.r.NoError(exec.Command("docker", "pull", imageCidPullRef).Run())
}

func (s *E2ETestSuite) TestPurgeCache_MissingCidRepo() {
	s.startDisco("./disco-e2e-config.yml")

	s.r.NoError(exec.Command("docker", "push", pushImageRef).Run())
	imageCid := getImageCid()
	imageCidPullRef := getImagePullRef(imageCid)

	// delete the cid repo from filestore (secondary store)
	s.r.NoError(os.RemoveAll(path.Join("testdir/cache/docker/registry/v2/repositories", imageCid)))

	// pull should replicate
	s.r.NoError(exec.Command("docker", "pull", imageCidPullRef).Run())

	s.verifyFiles()
}

func (s *E2ETestSuite) TestPullUnknown_NoClone() {
	s.startDisco("./disco-e2e-config.yml")

	s.r.NoError(exec.Command("docker", "push", pushImageRef).Run())

	var out bytes.Buffer
	pullCmd := exec.Command("docker", "pull", unexpectedPullImageRef)
	pullCmd.Stdout = &out
	pullCmd.Stderr = &out
	s.r.Error(pullCmd.Run())
	s.r.Contains(out.String(), "not found", out.String())
}

func (s *E2ETestSuite) TestCacheOnly() {
	s.startDisco("disco-e2e-config-cache-only.yml")

	s.r.NoError(exec.Command("docker", "push", pushImageRef).Run())

	pullCmd := exec.Command("docker", "pull", "-a", expectedPullImageRefCacheOnly)
	out := bytes.NewBufferString("")
	pullCmd.Stdout = out
	s.r.NoError(pullCmd.Run())
	s.r.Contains(out.String(), expectedImageCidCacheOnly)

	// verify that file exists in the cache storage
	for _, contentPath := range []string{
		expectedSha256Repo,
		expectedCidRepoCacheOnly,

		expectedManifestBlob,
		expectedConfigBlob,
		expectedLayerBlob1,
		expectedLayerBlob2,

		expectedCidTagCacheOnly,
	} {
		fsInfo, err := os.Stat(path.Join("testdir/cache", contentPath))
		s.r.NoError(err, contentPath)
		s.r.Greater(fsInfo.Size(), int64(0), contentPath)
	}
}
