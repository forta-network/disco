package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"strings"

	"github.com/forta-protocol/disco/proxy/services/interfaces"
	ipfsapi "github.com/ipfs/go-ipfs-api"
	log "github.com/sirupsen/logrus"
)

func (disco *Disco) digestFromLink(ctx context.Context, path string) (string, error) {
	r, err := disco.api.FilesRead(ctx, path)
	if err != nil {
		return "", err
	}
	b, err := ioutil.ReadAll((r))
	if err != nil {
		return "", err
	}
	// The digest is mentioned in sha256:<digest> format and we return only the hash.
	return string(b)[7:], nil
}

type imageManifest struct {
	Config struct {
		Digest string `json:"digest"`
	} `json:"config"`
	Layers []struct {
		Digest string `json:"digest"`
	} `json:"layers"`
}

func (disco *Disco) readManifestWithDigest(ctx context.Context, digest string) (*imageManifest, error) {
	r, err := disco.api.FilesRead(ctx, makeBlobPath(digest))
	if err != nil {
		return nil, err
	}
	var manifest imageManifest
	return &manifest, json.NewDecoder(r).Decode(&manifest)
}

func (disco *Disco) getCid(ctx context.Context, path string) (string, error) {
	stat, err := disco.api.FilesStat(ctx, path)
	if err != nil {
		return "", fmt.Errorf("failed to get cid for %s: %v", path, err)
	}
	return stat.Hash, nil
}

func (disco *Disco) getBlobCid(ctx context.Context, digest string) (string, error) {
	return disco.getCid(ctx, makeBlobPath(digest))
}

func (disco *Disco) populateBlobsDigests(ctx context.Context, manifestDigest string) ([]*blobCid, error) {
	manifest, err := disco.readManifestWithDigest(ctx, manifestDigest)
	if err != nil {
		return nil, err
	}
	configDigest := manifest.Config.Digest[7:]

	manifestCid, err := disco.getBlobCid(ctx, manifestDigest)
	if err != nil {
		return nil, err
	}
	configCid, err := disco.getBlobCid(ctx, configDigest)
	if err != nil {
		return nil, err
	}

	blobs := []*blobCid{
		{
			Digest: manifestDigest,
			Cid:    manifestCid,
		},
		{
			Digest: configDigest,
			Cid:    configCid,
		},
	}
	for _, layer := range manifest.Layers {
		layerDigest := layer.Digest[7:]
		layerCid, err := disco.getBlobCid(ctx, layerDigest)
		if err != nil {
			return nil, err
		}
		blobs = append(blobs, &blobCid{
			Digest: layerDigest,
			Cid:    layerCid,
		})
	}
	return blobs, nil
}

type blobCid struct {
	Digest string `json:"digest"`
	Cid    string `json:"cid"`
}

type discoFile struct {
	Blobs []*blobCid `json:"blobs"`
}

func (disco *Disco) writeDiscoFile(ctx context.Context, repoName string, discoFile *discoFile) error {
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(discoFile); err != nil {
		return err
	}
	if err := disco.api.FilesWrite(ctx, makeDiscoFilePath(repoName), &buf, ipfsapi.FilesWrite.Create(true)); err != nil {
		return err
	}
	return nil
}

func (disco *Disco) readDiscoFile(ctx context.Context, repoName string) (*discoFile, error) {
	nodeClient, err := disco.api.GetClientFor(ctx, makeRepoPath(repoName))
	if err != nil {
		return nil, fmt.Errorf("failed to route to provider client (before cloning global): %v", err)
	}
	hasFile, err := disco.hasFile(ctx, nodeClient, makeDiscoFilePath(repoName))
	if err != nil {
		return nil, err
	}
	if !hasFile {
		nodeClient.FilesMkdir(ctx, repositoriesBase, ipfsapi.FilesMkdir.Parents(true))
		if err := nodeClient.FilesCp(ctx, fmt.Sprintf("/ipfs/%s", repoName), makeRepoPath(repoName)); err != nil {
			return nil, fmt.Errorf("failed while copying the repo from the network: %v", err)
		}
	}
	log.WithError(err).Debugf("disco.json path: %s", makeDiscoFilePath(repoName))
	r, err := nodeClient.FilesRead(ctx, makeDiscoFilePath(repoName))
	if err != nil {
		return nil, err
	}
	var file discoFile
	if err := json.NewDecoder(r).Decode(&file); err != nil {
		return nil, fmt.Errorf("failed to decode disco file: %v", err)
	}
	return &file, nil
}

func (disco *Disco) createTagForLatest(ctx context.Context, repoName, tag string) error {
	return disco.api.FilesCp(ctx, makeTagPathFor(repoName, "latest"), makeTagPathFor(repoName, tag))
}

func (disco *Disco) hasFile(ctx context.Context, client interfaces.IPFSFilesAPI, path string) (bool, error) {
	_, err := client.FilesStat(ctx, path)
	switch {
	case err == nil:
		return true, nil

	case err != nil && strings.Contains(err.Error(), "does not exist"):
		return false, nil

	default:
		return false, fmt.Errorf("failed to check if file exists: %v", err)
	}
}
