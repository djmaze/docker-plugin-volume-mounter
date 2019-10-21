# Docker Plugin Volume Mounter

[![Docker Pulls](https://img.shields.io/docker/pulls/decentralize/docker-plugin-volume-mounter.svg)](https://hub.docker.com/r/decentralize/docker-plugin-volume-mounter)
[![Build Status](https://ci.strahlungsfrei.de/api/badges/djmaze/docker-plugin-volume-mounter/status.svg)](https://ci.strahlungsfrei.de/djmaze/docker-plugin-volume-mounter)

Docker container which makes remote Docker volumes available under a well-known path. This helps with backups.

## What does it do?

This containers watches the Docker API for new volume mounts (that happens when a container is started with a volume). If the volume is not using the `local` driver, the mount directory is bind-mounted to a subfolder in `/mnt/docker-volumes`.

## What is it good for?

When using a Docker volume for mounting remote drives or directories (e.g. S3 with [Rexray](https://github.com/rexray/rexray/)), the remote directory is mounted under a non-deterministic path beyond the volume plugin's root directory. Thus, there is no clean way of taking backups from within the cluster.

When running this container, DVM will make sure there is a directory for each remote volume. Also, the directory is deterministically named after the volume name.

This allows making consistent backups also in a Swarm cluster, e.g. with [Resticker](https://github.com/djmaze/resticker).

## Usage

```bash
docker run -d --rm \
  --cap-add SYS_ADMIN \
  -v /mnt/docker-volumes:/mnt/docker-volumes:shared \
  -v /var/lib/docker/plugins:/var/lib/docker/plugins:ro \
  -v /var/run/docker.sock:/var/run/docker.sock \
  decentralize/docker-plugin-volume-mounter-starter
```

See also the provided _docker-compose.*.yml_ files for examples on how to run this on single server, in a Swarm cluster and with [Docker Socket Proxy](https://github.com/Tecnativa/docker-socket-proxy).

### Running on Docker Swarm

As the container needs `SYS_ADMIN` capabilities and Docker Swarm does not yet support running with caps, there is a starter image (`decentralize/docker-plugin-volume-mounter-starter`) which uses the Docker socket in order to run a container with the required capability. The starter container itself does not required heightened capabilities, so it can be created as a Swarm service.

See [docker-compose.swarm.yml](docker-compose.swarm.yml) on how it can used.
