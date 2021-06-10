# disco

![mirror ball](https://upload.wikimedia.org/wikipedia/commons/2/29/Disco_ball4.jpg)

(PoC) (WIP)

[OCI Distribution Specification](https://github.com/opencontainers/distribution-spec/blob/main/spec.md) compliant decentralized and **dis**tributed **co**ntainer registry, based on the open source registry implementation: https://github.com/distribution/distribution

This can be run in a load-balanced private backend or locally (e.g. as a daemon) to interact with the same _registry_.

The project is at the PoC & discussion phase.

## Features

- [x] Using IPFS as the storage (IPFS driver implementation)
- [ ] Decentralized access control (via an Ethereum smart contract)

## FAQ

### Why do we need this?

This is for addressing the concern about the centralized Docker Hub. The goal is to add freedom and decentralization to access and management of container images. Please see [this issue](https://github.com/OpenZeppelin/fortify-node/issues/1) for the original discussion.

### How is this not centralized?

There are two main parameters:

- IPFS API URL
- Access control smart contract address

Anyone who uses the same parameters and runs the server anywhere should be interacting with the same registry. If either of the parameters are different than other users' parameters (e.g. private IPFS, different contract), then the registry is a different one.

### Why not upload to IPFS directly?

A few reasons:

- Supporting the same distribution rules & standards with Docker Hub and others
- Supporting the `docker` CLI out of the box and not needing a new complex tool
- Easier development: using the open source registry implementation as the base
- Flexibility: developing new access controllers and drivers to include more features
