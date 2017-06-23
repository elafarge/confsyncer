Conf-Syncer: sync your config files with etcd
=============================================

[![Build Status](https://semaphoreci.com/api/v1/elafarge/confsyncer/branches/master/shields_badge.svg)](https://semaphoreci.com/elafarge/confsyncer)
[![Go Report Card](https://goreportcard.com/badge/elafarge/confsyncer)](http://goreportcard.com/report/elafarge/confsyncer)
[![Docker](https://images.microbadger.com/badges/version/elafarge/confsyncer.svg)](https://microbadger.com/images/elafarge/confsyncer)
[![License: GPL v3](https://img.shields.io/badge/License-GPL%20v3-blue.svg)](http://www.gnu.org/licenses/gpl-3.0)

Conf-Syncer is a little Go Program that helps keeping config files in sync with
a Key Value store. Only etcd v3 is supported but adding other key-value store is
just about implementing a simple interface.

Motivations
-----------

My TLS certs are fetched dynamically from Let's Encrypt and upgraded every three
months. I need to keep these certs in sync between my load-balancer instances
and using a distributed file storage was overkill (I'm running
Kubernetes clusters on top of AWS and therefore don't have access to
ReadManyWriteMany volumes... without deploying a Ceph cluster at least...).

This self-contained program can run on bare-metal or in VMs (using Docker or
not) and plays pretty well as an init-container/sidecar container for my Ingress
controllers in Kubernetes. The init-container simply pulls the config from the
key-value store and dies, the sidecar container keeps the config in sync. If you
don't need to be sure that the config files are there when the target container
starts for the first time, the sidecar container alone should be just fine).

In the longer run, I'm trying to wrap this in a Kubernetes ThirdPartyResource (a
sort of dynamic config map).

Building, Packaging and Distributing
------------------------------------

You'll need `docker` and `make` in order to build the project, nothing else
(we're using a build container), just run `make`.

You can also build a container with `make docker` and ship it to a registry of
your choice with
```shell
DOCKER_IMAGE=my.registry.io/confsyncer DOCKER_TAG=v2 make docker-push
```

Finally, if you have `docker-compose` installed, you can get a dev. environment
with one etcd container reachable on `localhost:20379` and a `syncer` container.
The command to run is `make devenv`.

Contributing
------------

Contributions are always welcome :)

A containerized development environment based on `docker-compose` is available
with `make devenv` if you don't want to deal with installing dependencies. It
runs an etcd instance that listens on `http://localhost:20379`.

Every time you bring a change to your code, rerunning `make devenv` will rebuild
what's needed and update the dev. env. (thanks to the old magic of `GNU make`).

If you want to tail your `syncer` container's log after the rebuild, simply run
`make devenv-probe`.

All `docker-compose` commands will also work ;-)

Would you update dependencies in the `glide.yaml`, run `make vendor-update` to
update them. You can handle dependencies manually with `glide`.

### KVStore fixtures

You can get some data in the etcd container by running `make populate-etcd`. It
should immediately synchronize the config with your disks.

Maintainers
-----------
* Étienne Lafarge <etienne.lafarge _at_ gmail.com>
