# docker-hausmeister
A janitor (german: Hausmeister) service for docker to delete old unused images. This is especially useful, but not limited to cluster systems like Kubernetes.
It monitors container creation events for used images and keeps track of last usage. On every container creation it will probe for deletable images and if possibly delete them.

## Getting started

* HM_UNTIL - time in seconds until images are deleted, default 604800 (1 week)
* HM_ENFORCING - enforces deletion of images with currently stopped containers associated and deletes images which have been never seen during execution of the program, default 0
* HM_DELETE_DANGLING - delete dangling images, default 1

## Usage

### Docker
Run as a container (don't forget to mount the docker socket into the container):
```
sudo docker run -v /var/run/docker.sock:/var/run/docker.sock vebis/docker-hausmeister
```

### Kubernetes
Run this container as a Daemon Set:
```
kubectl apply -f docker-hausmeister.yml
```

### Native
Make sure to be in the docker group
```
sudo HM_UNTIL=604800 HM_ENFORCING=0 HM_DELETE_DANGLING=1 ./app
```

## Author

Stephan Kirsten

## License

BSD 2-Clause "Simplified" License
