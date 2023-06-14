package services

import (
	"context"
	"fmt"
	"strings"

	storagedriver "github.com/distribution/distribution/v3/registry/storage/driver"
	"github.com/forta-network/disco/deps"
	"github.com/forta-network/disco/drivers/ipfs"
	"github.com/forta-network/disco/drivers/multidriver"
	"github.com/forta-network/disco/interfaces"
	"github.com/forta-network/disco/utils"
	ipfsapi "github.com/ipfs/go-ipfs-api"
	log "github.com/sirupsen/logrus"
)

// Disco service allows us to do Disco things on top of the
// Distribution server.
type Disco struct {
	noClone       bool
	getIpfsClient getIpfsClientFunc
	getDriver     getDriverFunc
}

type getIpfsClientFunc func() interfaces.IPFSClient
type getDriverFunc func() storagedriver.StorageDriver

// NewDiscoService creates a new Disco service.
func NewDiscoService(noClone bool) *Disco {
	return &Disco{
		noClone:       noClone,
		getIpfsClient: deps.Get,
		getDriver:     ipfs.Get,
	}
}

// MakeGlobalRepo makes the repo a globally addressable one. We achieve this by
// benefiting from the content addressing and data deduplication properties of IPFS.
//
// This makes Disco a huge bucket of repositories which are identifiable only by hashes
// and each repository only has the version "latest".
//
// Steps in here are executed after Distribution server creates a repository:
//  1. Add disco.json to the repo dir so the blobs can be copied from the network at the time of "pull".
//  2. Duplicate the repo by using the base32-encoded IPFS CID of repo dir as the repo name so it becomes very easy to address the repo.
//  3. Duplicate the repo by using the manifest digest as the repo name so we make <digest>:latest possible.
//  4. Tag the repo in step 3 with the name in step 2 like <digest>:<CID> so it becomes easy to discover the CID from the digest.
//  5. Remove the repo which was created before step 1 so we allow no special names for repositories.
//
// The images should be accessible from any Disco which speaks to an IPFS node connected to the
// network. Duplicating repositories in IPFS MFS with different names shouldn't cause
// IPFS to duplicate the actual files.
//
// After pushing <disco_host>:1970/myrepo, it transforms the storage from:
//
//		/myrepo - QmWhatever1
//	   ..
//	     /tags
//	       /latest
//
// to:
//
//	/<cidv1(QmWhatever2)> - QmWhatever2
//	  disco.json
//	  ..
//	    /tags
//	      /latest
//	/<digest> - QmWhatever3
//	  disco.json
//	  ..
//	    /tags
//	      /latest
//	      /<cidv1(QmWhatever2)>
func (disco *Disco) MakeGlobalRepo(ctx context.Context, repoName string) error {
	ipfsClient := disco.getIpfsClient()
	driver := disco.getDriver()

	uploadRepoPath := makeRepoPath(repoName)

	// Step #5
	if !utils.IsCIDv1(repoName) && !utils.IsDigestHex(repoName) {
		defer func() {
			_ = driver.Delete(ctx, uploadRepoPath)
		}()
	}

	// Step #1
	manifestDigest, err := disco.digestFromLink(ctx, makeManifestLinkPath(repoName))
	if err != nil {
		return fmt.Errorf("failed to read the digest from the link: %v", err)
	}
	manifestDigestRepoPath := makeRepoPath(manifestDigest)
	stat, err := driver.Stat(ctx, manifestDigestRepoPath)
	if err == nil && stat.Size() > 0 {
		log.Info("already made globally accessible - skipping")
		return nil
	}

	// pushes can be successful after checks on the secondary driver
	// so ensure that primary storage is up-to-date and avoid false positives
	contentPaths, err := disco.populateBlobFilePaths(ctx, driver, manifestDigest)
	if err != nil {
		return fmt.Errorf("failed to populate blob file paths: %v", err)
	}
	contentPaths = append(contentPaths, uploadRepoPath)
	if err := disco.replicateInPrimary(driver, contentPaths); err != nil {
		return nil
	}

	blobs, err := disco.populateBlobsWithCids(ctx, manifestDigest)
	if err != nil {
		return fmt.Errorf("failed to populate blobs: %v", err)
	}
	if err := disco.writeDiscoFile(ctx, repoName, &discoFile{
		Blobs: blobs,
	}); err != nil {
		return fmt.Errorf("failed to write the disco file: %v", err)
	}

	// Step #2
	repoCid, err := disco.getCid(ctx, uploadRepoPath)
	if err != nil {
		return fmt.Errorf("failed while getting the repo cid: %v", err)
	}
	repoCidV1, err := utils.ToCIDv1(repoCid)
	if err != nil {
		return fmt.Errorf("failed to convert cid v0 '%s' to v1: %v", repoCid, err)
	}
	ipfsCidRepoPath := makeRepoPath(repoCidV1)
	cidRepoClient, err := ipfsClient.GetClientFor(ctx, ipfsCidRepoPath)
	if err != nil {
		return fmt.Errorf("failed to find client for cid repo (to copy after upload is done): %v", err)
	}
	_ = cidRepoClient.FilesMkdir(ctx, repositoriesBase, ipfsapi.FilesMkdir.Parents(true))
	_ = cidRepoClient.FilesRm(ctx, ipfsCidRepoPath, true)
	err = cidRepoClient.FilesCp(ctx, fmt.Sprintf("/ipfs/%s", repoCid), ipfsCidRepoPath)
	if err != nil && !strings.Contains(err.Error(), "already has entry") {
		return fmt.Errorf("failed while duplicating with base32 cid: %v", err)
	}

	// Step #3
	// make blob digest hex multiplexing logic work
	manifestRepoClient, err := ipfsClient.GetClientFor(ctx, manifestDigestRepoPath)
	if err != nil {
		return fmt.Errorf("failed to find client for destination repo provider (before copying digest-name repo): %v", err)
	}
	_ = manifestRepoClient.FilesMkdir(ctx, repositoriesBase, ipfsapi.FilesMkdir.Parents(true))
	_ = manifestRepoClient.FilesRm(ctx, manifestDigestRepoPath, true)
	if err := manifestRepoClient.FilesCp(ctx, fmt.Sprintf("/ipfs/%s", repoCid), manifestDigestRepoPath); err != nil {
		return fmt.Errorf("failed while duplicating with digest: %v", err)
	}

	// Step #4
	if err := disco.createTagForLatest(ctx, manifestDigest, repoCidV1); err != nil {
		return fmt.Errorf("failed to create tag for latest")
	}

	// replicate repo definitions in secondary (blobs are already written)
	contentPaths = []string{manifestDigestRepoPath, ipfsCidRepoPath}
	if err := disco.replicateInSecondary(driver, contentPaths); err != nil {
		return err
	}
	return nil
}

// IsOnlyPullable tells if the repo is name of a pullable-only repo name.
func (disco *Disco) IsOnlyPullable(repoName string) bool {
	return utils.IsCIDv1(repoName) || utils.IsDigestHex(repoName)
}

// CloneGlobalRepo clones the repo from IPFS network to the IPFS node.
// Steps in here are executed before Distribution server tries to locate a repository:
//  1. Check if the repo name is base32 CID v1. If not, leave the rest to the Distribution server.
//  2. Copy the repo files from IPFS network to the IPFS node's MFS.
//  3. Use disco.json inside the repo files to copy the blobs over the network.
//
// The end result in the IPFS node's MFS should look like the one from MakeGlobalRepo and all CIDs should match.
func (disco *Disco) CloneGlobalRepo(ctx context.Context, repoName string) error {
	// Step #1
	if !utils.IsCIDv1(repoName) {
		log.WithField("repository", repoName).Debugf("not a cidv1 name - not attempting to clone from ipfs")
		return nil
	}

	ipfsClient := disco.getIpfsClient()
	driver := disco.getDriver()

	stat, err := driver.Stat(ctx, makeDiscoFilePath(repoName))
	switch err.(type) {
	case nil:
		if !stat.IsDir() && stat.Size() > 0 {
			log.WithField("repository", repoName).Debug("found in storage - not attempting to clone from ipfs")
			return nil
		}

	case storagedriver.PathNotFoundError:
		log.WithField("repository", repoName).Info("not found in secondary - replicating from primary before pull")
		err = disco.tryReplicateInSecondary(ctx, makeRepoPath(repoName))
		if err == nil {
			return nil
		}
		log.WithField("repository", repoName).WithError(err).Warn("failed to replicate in secondary before pull")
		// continue cloning

	default:
		return fmt.Errorf("failed to check disco file using the driver: %v", err)
	}

	if disco.noClone {
		return nil
	}

	// Step #2 and #3
	file, err := disco.readDiscoFile(ctx, repoName)
	if err != nil {
		return fmt.Errorf("failed to read the disco file: %v", err)
	}
	for _, blobCid := range file.Blobs {
		// get the client without the provider: causes blobs to be replicated after increasing the amountof IPFS nodes
		blobNodeClient, err := ipfsClient.GetClientFor(ctx, makeBlobPath(blobCid.Digest))
		if err != nil {
			return fmt.Errorf("failed to get blob node client: %v", err)
		}
		hasFile, err := disco.hasFile(ctx, blobNodeClient, makeBlobPath(blobCid.Digest))
		if err != nil {
			return fmt.Errorf("failed to check if blob exists: %v", err)
		}
		if hasFile {
			continue
		}
		_ = blobNodeClient.FilesMkdir(ctx, makeBlobDirPath(blobCid.Digest), ipfsapi.FilesMkdir.Parents(true))
		if err := blobNodeClient.FilesCp(ctx, fmt.Sprintf("/ipfs/%s", blobCid.Cid), makeBlobPath(blobCid.Digest)); err != nil {
			return fmt.Errorf("failed while copying blob %s (%s) from the network: %v", blobCid.Digest, blobCid.Cid, err)
		}
	}

	// replicate repo definitions and blobs in secondary
	contentPaths := []string{makeRepoPath(repoName)}
	for _, blob := range file.Blobs {
		contentPaths = append(contentPaths, makeBlobPath(blob.Digest))
	}
	return disco.replicateInSecondary(driver, contentPaths)
}

func (disco *Disco) tryReplicateInSecondary(ctx context.Context, contentPath string) error {
	multiDriver, ok := multidriver.Is(disco.getDriver())
	if !ok {
		return nil
	}
	_, err := multiDriver.ReplicateInSecondary(contentPath)
	return err
}

func (disco *Disco) replicateInSecondary(driver storagedriver.StorageDriver, contentPaths []string) error {
	multiDriver, ok := multidriver.Is(driver)
	if !ok {
		return nil
	}
	for _, contentPath := range contentPaths {
		_, err := multiDriver.ReplicateInSecondary(contentPath)
		if err != nil {
			return fmt.Errorf("failed to replicate '%s' in secondary: %v", contentPath, err)
		}
	}

	return nil
}

func (disco *Disco) replicateInPrimary(driver storagedriver.StorageDriver, contentPaths []string) error {
	multiDriver, ok := multidriver.Is(driver)
	if !ok {
		return nil
	}
	for _, contentPath := range contentPaths {
		_, err := multiDriver.ReplicateInPrimary(contentPath)
		if err != nil {
			return fmt.Errorf("failed to replicate '%s' in primary: %v", contentPath, err)
		}
	}

	return nil
}
