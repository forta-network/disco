# disco
![coverage](https://img.shields.io/badge/coverage-73.3%25-brightgreen)

![mirror ball](https://upload.wikimedia.org/wikipedia/commons/2/29/Disco_ball4.jpg)

[OCI Distribution Specification](https://github.com/opencontainers/distribution-spec/blob/main/spec.md) conformant & Docker compatible decentralized and **dis**tributed **co**ntainer registry, based on the open source registry implementation: https://github.com/distribution/distribution

This can be hosted or run locally (e.g. as a daemon) to interact with the global registry.

### Example flow

Make port 4001 accessible from the rest of the world. This is needed for IPFS p2p communication.

Start the IPFS daemon:
```
$ ipfs init
$ ipfs daemon
```

Start Disco:
```
$ make run
```

Interact with Disco:
```
$ docker pull busybox:latest
$ docker tag busybox:latest localhost:1970/busybox
$ docker push localhost:1970/busybox
Using default tag: latest
The push refers to repository [localhost:1970/busybox]
5b8c72934dfc: Pushed 
latest: digest: sha256:dca71257cd2e72840a21f0323234bb2e33fea6d949fa0f21c5102146f583486b size: 527

$ docker pull -a localhost:1970/dca71257cd2e72840a21f0323234bb2e33fea6d949fa0f21c5102146f583486b
bafybeibbkcck6lz37hcipp2mwtfdgstydizjq45z4fkqq4va73mp7qzutu: Pulling from dca71257cd2e72840a21f0323234bb2e33fea6d949fa0f21c5102146f583486b
Digest: sha256:dca71257cd2e72840a21f0323234bb2e33fea6d949fa0f21c5102146f583486b
latest: Pulling from dca71257cd2e72840a21f0323234bb2e33fea6d949fa0f21c5102146f583486b
Digest: sha256:dca71257cd2e72840a21f0323234bb2e33fea6d949fa0f21c5102146f583486b
Status: Downloaded newer image for localhost:1970/dca71257cd2e72840a21f0323234bb2e33fea6d949fa0f21c5102146f583486b
localhost:1970/dca71257cd2e72840a21f0323234bb2e33fea6d949fa0f21c5102146f583486b
```

Now `bafybeibbkcck6lz37hcipp2mwtfdgstydizjq45z4fkqq4va73mp7qzutu` is a global identifier for `busybox:latest` image. In another machine, start IPFS and Disco and interact with it by doing:

```
$ docker pull localhost:1970/bafybeibbkcck6lz37hcipp2mwtfdgstydizjq45z4fkqq4va73mp7qzutu
Using default tag: latest
latest: Pulling from bafybeibbkcck6lz37hcipp2mwtfdgstydizjq45z4fkqq4va73mp7qzutu
b71f96345d44: Pull complete 
Digest: sha256:dca71257cd2e72840a21f0323234bb2e33fea6d949fa0f21c5102146f583486b
Status: Downloaded newer image for localhost:1970/bafybeibbkcck6lz37hcipp2mwtfdgstydizjq45z4fkqq4va73mp7qzutu:latest
localhost:1970/bafybeibbkcck6lz37hcipp2mwtfdgstydizjq45z4fkqq4va73mp7qzutu:latest
```
