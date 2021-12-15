package services

import (
	"context"
	"fmt"
	"strings"

	"github.com/forta-protocol/disco/deps"
	"github.com/forta-protocol/disco/proxy/services/interfaces"
	"github.com/forta-protocol/disco/utils"
	ipfsapi "github.com/ipfs/go-ipfs-api"
	log "github.com/sirupsen/logrus"
)

// Disco service allows us to do Disco things on top of the
// Distribution server.
type Disco struct {
	api interfaces.IPFSClient
}

// NewDiscoService creates a new Disco service.
func NewDiscoService() *Disco {
	client := deps.Get()
	return &Disco{
		api: client,
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
//  2. Duplicate the repo by using the manifest digest as the repo name so we make <digest>:latest possible.
//  3. Duplicate the repo by using the base32-encoded IPFS CID of repo dir as the repo name so it becomes very easy to address the repo.
//  4. Tag the repo in step 2 with the name in step 3 like <digest>:<CID> so it becomes easy to discover the CID from the digest.
//  5. Remove the repo which was created before step 1 so we allow no special names for repositories.
//
// The images should be accessible from any Disco which speaks to an IPFS node connected to the
// network. Duplicating repositories in IPFS MFS with different names shouldn't cause
// IPFS to duplicate the actual files.
//
// After pushing <disco_host>:1970/myrepo, it transforms the storage from:
// 	/myrepo - QmWhatever1
//    ..
//      /tags
//        /latest
//
// to:
//  /<cidv1(QmWhatever2)> - QmWhatever2
//    disco.json
//    ..
//      /tags
//        /latest
//  /<digest> - QmWhatever3
//    disco.json
//    ..
//      /tags
//        /latest
//        /<cidv1(QmWhatever2)>
func (disco *Disco) MakeGlobalRepo(ctx context.Context, repoName string) error {
	// Step #5
	defer disco.api.FilesRm(ctx, makeRepoPath(repoName), true)

	// Step #1
	manifestDigest, err := disco.digestFromLink(ctx, makeManifestLinkPath(repoName))
	if err != nil {
		return fmt.Errorf("failed to read the digest from the link: %v", err)
	}
	stat, err := disco.api.FilesStat(ctx, makeRepoPath(manifestDigest))
	if err == nil && stat.CumulativeSize > 0 {
		log.Info("already made globally accessible - skipping")
		return nil
	}

	blobs, err := disco.populateBlobsDigests(ctx, manifestDigest)
	if err != nil {
		return fmt.Errorf("failed to populate blobs: %v", err)
	}
	if err := disco.writeDiscoFile(ctx, repoName, &discoFile{
		Blobs: blobs,
	}); err != nil {
		return fmt.Errorf("failed to write the disco file: %v", err)
	}

	// Step #2
	nodeClient, err := disco.api.GetClientFor(ctx, makeRepoPath(repoName))
	if err != nil {
		return fmt.Errorf("failed to find client for original repo provider (before copying): %v", err)
	}
	repoCid, err := disco.getCid(ctx, makeRepoPath(repoName))
	if err != nil {
		return fmt.Errorf("failed while getting the repo cid: %v", err)
	}
	repoCidV1 := utils.ToCIDv1(repoCid)
	err = nodeClient.FilesCp(ctx, makeRepoPath(repoName), makeRepoPath(repoCidV1))
	if err != nil && !strings.Contains(err.Error(), "already has entry") {
		return fmt.Errorf("failed while duplicating with base32 cid: %v", err)
	}

	// Step #3
	// make blob digest hex multiplexing logic work
	nodeClient, err = disco.api.GetClientFor(ctx, makeRepoPath(manifestDigest))
	if err != nil {
		return fmt.Errorf("failed to find client for destination repo provider (before copying digest-name repo): %v", err)
	}
	nodeClient.FilesMkdir(ctx, repositoriesBase, ipfsapi.FilesMkdir.Parents(true))
	if err := nodeClient.FilesCp(ctx, fmt.Sprintf("/ipfs/%s", repoCid), makeRepoPath(manifestDigest)); err != nil {
		return fmt.Errorf("failed while duplicating with digest: %v", err)
	}

	// Step #4
	if err := disco.createTagForLatest(ctx, manifestDigest, repoCidV1); err != nil {
		return fmt.Errorf("failed to create tag for latest")
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
// The end result in the IPFS node's MFS should look like the one from MakeGlobalRepo and all CIDs should match.
func (disco *Disco) CloneGlobalRepo(ctx context.Context, repoName string) error {
	// Step #1
	if !utils.IsCIDv1(repoName) {
		log.WithField("repository", repoName).Debugf("not a cidv1 name - not attempting to clone from ipfs")
		return nil
	}

	// Step #2 and #3
	file, err := disco.readDiscoFile(ctx, repoName)
	if err != nil {
		return fmt.Errorf("failed to read the disco file: %v", err)
	}
	for _, blobCid := range file.Blobs {
		// get the client without the provider: causes blobs to be replicated after increasing the amountof IPFS nodes
		blobNodeClient, err := disco.api.GetClientFor(ctx, makeBlobPath(blobCid.Digest))
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
		blobNodeClient.FilesMkdir(ctx, makeBlobDirPath(blobCid.Digest), ipfsapi.FilesMkdir.Parents(true))
		if err := blobNodeClient.FilesCp(ctx, fmt.Sprintf("/ipfs/%s", blobCid.Cid), makeBlobPath(blobCid.Digest)); err != nil {
			return fmt.Errorf("failed while copying blob %s (%s) from the network: %v", blobCid.Digest, blobCid.Cid, err)
		}
	}

	return nil
}
