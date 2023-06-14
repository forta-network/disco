package e2e_test

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path"
	"testing"
	"time"

	"github.com/forta-network/disco/cmd"
	"github.com/forta-network/disco/utils"
	ipfsapi "github.com/ipfs/go-ipfs-api"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

var (
	processStartWaitSeconds = 60
	pushImageRef            = "localhost:1970/test"

	expectedImageSha = "2197ffa9bd16c893488bc26712a9dd28826daf2abb1a1dabf554fe32615a541d"
	expectedImageCid = "bafybeicvnsxllo2cqihnyroi4ydbpvfhghrid6tfyxv2f3acoxgvg3ceoy"

	unexpectedImageCid     = "bafybeielvnt5apaxbk6chthc4dc3p6vscpx3ai4uvti7gwh253j7facsxu"
	unexpectedPullImageRef = fmt.Sprintf("localhost:1970/%s", unexpectedImageCid)

	reposPath = "/docker/registry/v2/repositories/"

	expectedSha256Repo = path.Join(reposPath, expectedImageSha)

	expectedManifestBlob = "/docker/registry/v2/blobs/sha256/21/2197ffa9bd16c893488bc26712a9dd28826daf2abb1a1dabf554fe32615a541d/data"
	expectedConfigBlob   = "/docker/registry/v2/blobs/sha256/dd/dddc7578369a0eb6d94c6eb359fb15cc807e2874fbd7e40614ed0b348c45fd2c/data"
	expectedLayerBlob    = "/docker/registry/v2/blobs/sha256/c1/c15cbdab5f8e3f3942c3461791546c53c54379cfc7746127fac6b0af00baf313/data"

	cidImageRef = path.Join("localhost:1970", expectedImageCid)
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

	os.Setenv("REGISTRY_CONFIGURATION_PATH", "./disco-e2e-config.yml")
	go cmd.Main(context.Background())
	suite.Run(t, &E2ETestSuite{})
}

func (s *E2ETestSuite) SetupTest() {
	s.r = s.Require()

	s.r.NoError(os.RemoveAll("testdir"))
	s.r.NoError(os.MkdirAll("testdir", 0777))
	s.startCleanIpfs()
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
}

func (s *E2ETestSuite) TestPushVerify() {
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
		expectedLayerBlob,
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

func (s *E2ETestSuite) TestPurgeIPFS_Pull() {
	s.r.NoError(exec.Command("docker", "push", pushImageRef).Run())

	// delete from ipfs (primary store)
	s.startCleanIpfs()

	// pull
	s.r.NoError(exec.Command("docker", "pull", cidImageRef).Run())

	// it was able to pull without needing ipfs
	_, err := s.ipfsClient1.FilesStat(context.Background(), "/docker")
	s.Error(err)
}

func (s *E2ETestSuite) TestPurgeIPFS_PushAgainPull() {
	s.r.NoError(exec.Command("docker", "push", pushImageRef).Run())

	// delete from ipfs (primary store)
	s.startCleanIpfs()

	// push again
	s.r.NoError(exec.Command("docker", "push", pushImageRef).Run())

	s.verifyFiles()

	s.r.NoError(exec.Command("docker", "pull", cidImageRef).Run())
}

func (s *E2ETestSuite) TestPurgeCache_Pull() {
	s.r.NoError(exec.Command("docker", "push", pushImageRef).Run())

	// delete from filestore (secondary store)
	s.r.NoError(os.RemoveAll("testdir/cache"))

	// pull
	s.r.NoError(exec.Command("docker", "pull", cidImageRef).Run())
}

func (s *E2ETestSuite) TestPurgeCache_PushAgainPull() {
	s.r.NoError(exec.Command("docker", "push", pushImageRef).Run())

	// delete from filestore (secondary store)
	s.r.NoError(os.RemoveAll("testdir/cache"))

	// push again
	s.r.NoError(exec.Command("docker", "push", pushImageRef).Run())

	s.verifyFiles()

	s.r.NoError(exec.Command("docker", "pull", cidImageRef).Run())
}

func (s *E2ETestSuite) TestPurgeCache_MissingCidRepo() {
	s.r.NoError(exec.Command("docker", "push", pushImageRef).Run())

	// delete the cid repo from filestore (secondary store)
	s.r.NoError(os.RemoveAll(path.Join("testdir/cache/docker/registry/v2/repositories", expectedImageCid)))

	// pull should replicate
	s.r.NoError(exec.Command("docker", "pull", cidImageRef).Run())

	s.verifyFiles()
}

func (s *E2ETestSuite) TestPullUnknown_NoClone() {
	s.r.NoError(exec.Command("docker", "push", pushImageRef).Run())

	var out bytes.Buffer
	pullCmd := exec.Command("docker", "pull", unexpectedPullImageRef)
	pullCmd.Stdout = &out
	pullCmd.Stderr = &out
	s.r.Error(pullCmd.Run())
	s.r.Contains(out.String(), "not found", out.String())
}
