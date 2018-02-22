# docker-hausmeister
A janitor service for docker to delete old unused images. This is especially useful, but not limited to cluster systems like Kubernetes.

# Getting started

HM_UNTIL - time in seconds until images are deleted, default 604800 (1 week)
HM_ENFORCING - enforces deletion of images with currently stopped containers associated (TODO: and deletes images which have been never seen during execution of the program), default 0
HM_DELETE_DANGLING - delete dangling images, default 1

run as a container (don't forget to mount the docker socket into the container) or native

# Author

Stephan Kirsten
