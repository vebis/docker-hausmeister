# docker-hausmeister
A janitor (german: Hausmeister) service for docker to delete old unused images. This is especially useful, but not limited to cluster systems like Kubernetes.
It monitors container creation events for used images and keeps track of last usage. On every container creation it will probe for deletable images and if possibly delete them.

## Usage

### Environment variables
* HM_UNTIL - time in seconds until images are deleted, default 604800 (1 week)
* HM_ENFORCING - enforces deletion of images with currently stopped containers associated and deletes images which have been never seen during execution of the program, default 0
* HM_DELETE_DANGLING - delete dangling images, default 1
* HM_EX variables - if present images with these names/tags/labels will be spared from deletion, can be comma separated list, any combination of these variables is possible
  * HM_EX_IMAGENAMEPREFIX - image name begins with configured value
  * HM_EX_IMAGENAMESUFFIX - image name ends with configured value
  * HM_EX_IMAGETAGPREFIX - image tag begins with configured value
  * HM_EX_IMAGETAGSUFFIX - image tag ends with configured value
  * HM_EX_IMAGELABEL - image has the label "exclude" with the exact configured value

### Docker
Run as a container (don't forget to mount the docker socket into the container):
```
sudo docker run \
  -e HM_UNTIL=604800 \
  -e HM_ENFORCING=0 \
  -e HM_DELETE_DANGLING=1 \
  -v /var/run/docker.sock:/var/run/docker.sock \
  vebis/docker-hausmeister
```

### Kubernetes
Run this container as a Daemon Set:
```
kubectl apply -f docker-hausmeister.yml
```

### Native
Make sure to be member of the docker group
```
sudo HM_UNTIL=604800 HM_ENFORCING=0 HM_DELETE_DANGLING=1 ./app
```
## Important Notes
* you want to use HM_ENFORCING=1 because otherwise stopped containers will keep either unused images in the system
* if you are doing multi-stage builds, you have to run with HM_DELETE_DANGLING=1 because the all finished stages will be dangling if the next starts

## Author

Stephan Kirsten

## License

BSD 2-Clause "Simplified" License
