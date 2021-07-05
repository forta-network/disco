package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"

	ipfsapi "github.com/ipfs/go-ipfs-api"
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

func (disco *Disco) populateBlobsDigests(ctx context.Context, manifestDigest string) (map[string]string, error) {
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

	blobs := map[string]string{
		manifestDigest: manifestCid,
		configDigest:   configCid,
	}
	for _, layer := range manifest.Layers {
		layerDigest := layer.Digest[7:]
		layerCid, err := disco.getBlobCid(ctx, layerDigest)
		if err != nil {
			return nil, err
		}
		blobs[layerDigest] = layerCid
	}
	return blobs, nil
}

type discoFile struct {
	Blobs map[string]string `json:"blobs"`
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
	r, err := disco.api.FilesRead(ctx, makeDiscoFilePath(repoName))
	if err != nil {
		return nil, err
	}
	var file discoFile
	return &file, json.NewDecoder(r).Decode(&file)
}

func (disco *Disco) createTagForLatest(ctx context.Context, repoName, tag string) error {
	return disco.api.FilesCp(ctx, makeTagPathFor(repoName, "latest"), makeTagPathFor(repoName, tag))
}
