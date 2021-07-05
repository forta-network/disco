package services

import "fmt"

const (
	registryBase     = "/docker/registry/v2"
	repositoriesBase = registryBase + "/repositories"

	discoFilePathFormat = repositoriesBase + "/%s/disco.json"

	manifestLinkPath = "/_manifests/tags/latest/current/link" // "link" is a file which contains the digest in sha256:<digest> format
	tagPathFormat    = "/_manifests/tags/%s"

	blobsBase         = registryBase + "/blobs/sha256"
	blobDirPathFormat = blobsBase + "/%s/%s"
	blobPathFormat    = blobDirPathFormat + "/data" // "data" is a file which contains the blob bytes
)

func makeRepoPath(repoName string) string {
	return repositoriesBase + "/" + repoName
}

func makeManifestLinkPath(repoName string) string {
	return makeRepoPath(repoName) + manifestLinkPath
}

func makeBlobDirPath(digest string) string {
	return fmt.Sprintf(blobDirPathFormat, digest[:2], digest)
}

func makeBlobPath(digest string) string {
	return fmt.Sprintf(blobPathFormat, digest[:2], digest)
}

func makeDiscoFilePath(repoName string) string {
	return fmt.Sprintf(discoFilePathFormat, repoName)
}

func makeTagPathFor(repoName, tag string) string {
	return fmt.Sprintf("%s/%s"+tagPathFormat, repositoriesBase, repoName, tag)
}
