package services

import (
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"net/http"

	"github.com/OpenZeppelin/disco/config"
	"github.com/ipfs/go-cid"
	ipfsapi "github.com/ipfs/go-ipfs-api"
	"github.com/multiformats/go-multihash"
)

// IPFSClient makes requests to an IPFS node.
type IPFSClient interface {
	FilesRead(ctx context.Context, path string, options ...ipfsapi.FilesOpt) (io.ReadCloser, error)
	FilesWrite(ctx context.Context, path string, data io.Reader, options ...ipfsapi.FilesOpt) error
	FilesRm(ctx context.Context, path string, force bool) error
	FilesCp(ctx context.Context, src string, dest string) error
	FilesStat(ctx context.Context, path string, options ...ipfsapi.FilesOpt) (*ipfsapi.FilesStatObject, error)
	FilesMkdir(ctx context.Context, path string, options ...ipfsapi.FilesOpt) error
}

// Disco service allows us to do Disco things on top of the
// Distribution server.
type Disco struct {
	api IPFSClient
}

// NewDiscoService creates a new Disco service.
func NewDiscoService() *Disco {
	ipfsURL, ok := config.DistributionConfig.Storage["ipfs"]["url"]
	if !ok {
		panic("no IPFS URL specified")
	}
	api := ipfsapi.NewShellWithClient(ipfsURL.(string), http.DefaultClient)
	return &Disco{
		api: api,
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
		log.Println("already made globally accessible - skipping")
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
	if err := disco.api.FilesCp(ctx, makeRepoPath(repoName), makeRepoPath(manifestDigest)); err != nil {
		return fmt.Errorf("failed while duplicating with digest: %v", err)
	}

	// Step #3
	repoCid, err := disco.getCid(ctx, makeRepoPath(repoName))
	if err != nil {
		return fmt.Errorf("failed while getting the repo cid: %v", err)
	}
	repoCidV1 := toCidv1(repoCid)
	if err := disco.api.FilesCp(ctx, makeRepoPath(repoName), makeRepoPath(repoCidV1)); err != nil {
		return fmt.Errorf("failed while duplicating with base32 cid: %v", err)
	}

	// Step #4
	if err := disco.createTagForLatest(ctx, manifestDigest, repoCidV1); err != nil {
		return fmt.Errorf("failed to create tag for latest")
	}

	return nil
}

func toCidv1(fileCid string) string {
	parsed, err := multihash.FromB58String(fileCid)
	if err != nil {
		panic(err)
	}
	return cid.NewCidV1(cid.DagProtobuf, parsed).String()
}

func isCidv1(fileCid string) bool {
	parsed, err := cid.Parse(fileCid)
	if err != nil {
		return false
	}
	return parsed.Version() == 1
}

func isDigestHex(digest string) bool {
	if len(digest) != 64 {
		return false
	}
	_, err := hex.DecodeString(digest)
	return err == nil
}

// IsOnlyPullable tells if the repo is only pullable.
func (disco *Disco) IsOnlyPullable(repoName string) bool {
	return isCidv1(repoName) || isDigestHex(repoName)
}

// CloneGlobalRepo clones the repo from IPFS network to the IPFS node.
// Steps in here are executed before Distribution server tries to locate a repository:
//  1. Check if the repo name is base32 CID v1. If not, leave the rest to the Distribution server.
//  2. Try to find the manifest for the repo. If it exists, leave the rest to the Distribution server.
//  3. Copy the repo files from IPFS network to the IPFS node's MFS.
//  4. Use disco.json inside the repo files to copy the blobs over the network.
//  5. Duplicate the repo by using the manifest digest as the repo name. (MakeGlobalRepo step #2)
//  6. Tag the duplicated repo like <digest>:<CID>. (MakeGlobalRepo step #4)
// The end result in the IPFS node's MFS should look like the one from MakeGlobalRepo and all CIDs should match.
func (disco *Disco) CloneGlobalRepo(ctx context.Context, repoName string) error {
	// Step #1
	if !isCidv1(repoName) {
		return nil
	}

	// Step #2
	manifestDigest, err := disco.digestFromLink(ctx, makeManifestLinkPath(repoName))
	if err == nil {
		return nil
	}

	// Step #3
	disco.api.FilesMkdir(ctx, repositoriesBase, ipfsapi.FilesMkdir.Parents(true))
	if err := disco.api.FilesCp(ctx, fmt.Sprintf("/ipfs/%s", repoName), makeRepoPath(repoName)); err != nil {
		return fmt.Errorf("failed while copying the repo from the network: %v", err)
	}
	manifestDigest, err = disco.digestFromLink(ctx, makeManifestLinkPath(repoName))
	if err != nil {
		return fmt.Errorf("failed to get the manifest digest from the copied repo: %v", err)
	}

	// Step #4
	file, err := disco.readDiscoFile(ctx, repoName)
	if err != nil {
		return fmt.Errorf("failed to read the disco file: %v", err)
	}
	for _, blobCid := range file.Blobs {
		disco.api.FilesMkdir(ctx, makeBlobDirPath(blobCid.Digest), ipfsapi.FilesMkdir.Parents(true))
		if err := disco.api.FilesCp(ctx, fmt.Sprintf("/ipfs/%s", blobCid.Cid), makeBlobPath(blobCid.Digest)); err != nil {
			return fmt.Errorf("failed while copying blob %s (%s) from the network: %v", blobCid.Digest, blobCid.Cid, err)
		}
	}

	// Step #5
	if err := disco.api.FilesCp(ctx, makeRepoPath(repoName), makeRepoPath(manifestDigest)); err != nil {
		return fmt.Errorf("failed while duplicating with digest: %v", err)
	}

	// Step #6
	if err := disco.createTagForLatest(ctx, manifestDigest, repoName); err != nil {
		return fmt.Errorf("failed to create tag for latest")
	}

	return nil
}
