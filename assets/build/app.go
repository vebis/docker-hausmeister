package main

import (
	"context"
	"io"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/events"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
)

var startTime int64
var hmUntil int
var imageHistory = make(map[string]int64)

var cli *client.Client

var enforcing bool
var deleteDangling bool

var exImageNamePrefix []string
var exImageNameSuffix []string
var exImageTagPrefix []string
var exImageTagSuffix []string
var exImageLabel []string

func getCurTimestamp() int64 {
	return int64(time.Now().Unix())
}

func getImageInspect(imageId string) (types.ImageInspect, error) {
	image, _, err := cli.ImageInspectWithRaw(context.Background(), imageId)

	return image, err
}

func getImageId(imageName string) string {
	id := ""

	image, err := getImageInspect(imageName)
	if err != nil {
		return id
	}

	id = image.ID

	log.Printf("Image name '%s' mapped to image id '%s'", imageName, id)

	return id
}

func updateImageHistory(imageId string) {
	imageHistory[imageId] = getCurTimestamp()

	return
}

func checkForRunningContainer(imageId string, all bool) bool {
	filter := filters.NewArgs()
	filter.Add("ancestor", imageId)
	options := types.ContainerListOptions{Quiet: true, All: all, Filters: filter}

	containers, err := cli.ContainerList(context.Background(), options)

	ret := true // set save default
	if err != nil {
		log.Print(err)

		return ret
	}

	if len(containers) == 0 {
		ret = false
	}

	return ret
}

func hasRunningContainers(imageId string) bool {
	return checkForRunningContainer(imageId, false)
}

func hasStoppedContainers(imageId string) bool {
	if checkForRunningContainer(imageId, false) == false && checkForRunningContainer(imageId, true) == true {
		return true
	}

	return false
}

func getRepoTags(imageId string) []string {
	var ret []string

	image, err := getImageInspect(imageId)
	if err == nil {
		ret = image.RepoTags
	}

	return ret
}

func getLabels(imageId string) map[string]string {
	ret := make(map[string]string)

	image, err := getImageInspect(imageId)
	if err == nil {
		ret = image.Config.Labels
	}

	return ret
}

func getExcludeLabel(imageId string) string {
	ret := ""

	labels := getLabels(imageId)

	if _, ok := labels["exclude"]; ok {
		ret = labels["exclude"]
	}

	return ret
}

func checkExcludeImageByPrefix(e []string, s string) bool {
	for _, v := range e {
		if strings.HasPrefix(s, v) == true {
			return true
		}
	}

	return false
}

func checkExcludeImageBySuffix(e []string, s string) bool {
	for _, v := range e {
		if strings.HasSuffix(s, v) == true {
			return true
		}
	}

	return false
}

func checkExcludeImageByValue(e []string, s string) bool {
	for _, v := range e {
		if v == s {
			return true
		}
	}

	return false
}

// checks if image is excluded -> returns true if it is
func checkExcludedImage(imageId string) bool {
	repoTags := getRepoTags(imageId)

	for _, repoTag := range repoTags {
		s := strings.Split(repoTag, ":")
		tag := s[len(s)-1]
		name := strings.Replace(repoTag, ":"+tag, "", 1)

		if checkExcludeImageByPrefix(exImageNamePrefix, name) || checkExcludeImageByPrefix(exImageTagPrefix, tag) || checkExcludeImageBySuffix(exImageNameSuffix, name) || checkExcludeImageBySuffix(exImageTagSuffix, tag) {
			return true
		}
	}

	if checkExcludeImageByValue(exImageLabel, getExcludeLabel(imageId)) {
		return true
	}

	return false
}

func rmImage(imageId string) {
	if checkExcludedImage(imageId) {
		log.Print("    Image is excluded from deletion. Skipping!")

		return
	}

	if hasRunningContainers(imageId) {
		log.Print("    Some containers are running. Skipping!")

		return
	}

	if hasStoppedContainers(imageId) && !enforcing {
		log.Print("    Some containers are stopped. Skipping!")

		return
	}

	imagedeleteresponses, err := cli.ImageRemove(context.Background(), imageId, types.ImageRemoveOptions{Force: true})
	if err != nil {
		log.Print(err)
	}

	for _, response := range imagedeleteresponses {
		if response.Deleted != "" {
			log.Printf("    Deleted   : %s", response.Deleted)
		} else {
			log.Printf("    Untagged  : %s", response.Untagged)
		}
	}

	// remove imageId from imageHistory map
	if getImageId(imageId) == "" {
		log.Print("  Image seems to be deleted")
		delete(imageHistory, imageId)
	}

	return
}

func deleteOldImages() {
	curTime := getCurTimestamp()
	log.Print("Searching for old images:")

	for imageId, last := range imageHistory {
		log.Printf("  trying image: %s", imageId)
		if last < curTime-int64(hmUntil) {
			log.Print("    Image is to old. Trying to delete.")
			rmImage(imageId)
		} else {
			log.Print("    Image is to new. Skipping.")
		}
	}

	return
}

func deleteGrandFatheredImages() {
	log.Print("Trying to delete images without received start event")
	var imageId string

	images, err := cli.ImageList(context.Background(), types.ImageListOptions{})
	if err != nil {
		log.Print(err)

		return
	}

	for _, image := range images {
		imageId = image.ID
		log.Print("  trying image: ", imageId)
		if _, ok := imageHistory[imageId]; ok {
			log.Print("    Image is not grandfathered. Skipping!")

			continue
		} else {
			rmImage(imageId)
		}
	}

	return
}

func deleteDanglingImages() {
	log.Print("Trying to clean dangled images")

	prunereport, err := cli.ImagesPrune(context.Background(), filters.NewArgs())
	if err != nil {
		panic(err)
	}

	log.Print("  Space reclaimed: ", prunereport.SpaceReclaimed)
}

func handleEvent(imageName string) {
	log.Printf("Handle event for image '%s'", imageName)

	imageId := getImageId(imageName)

	if imageId == "" {
		log.Print("Could not find image id")

		return
	}

	updateImageHistory(imageId)

	deleteOldImages()

	if deleteDangling {
		deleteDanglingImages()
	}

	if getCurTimestamp() > startTime+int64(hmUntil) && enforcing {
		deleteGrandFatheredImages()
	}
}

func parseExcludeParameters() {
	if os.Getenv("HM_EX_IMAGENAMEPREFIX") != "" {
		exImageNamePrefix = strings.Split(os.Getenv("HM_EX_IMAGENAMEPREFIX"), ",")
	}
	if os.Getenv("HM_EX_IMAGENAMESUFFIX") != "" {
		exImageNameSuffix = strings.Split(os.Getenv("HM_EX_IMAGENAMESUFFIX"), ",")
	}
	if os.Getenv("HM_EX_IMAGETAGPREFIX") != "" {
		exImageTagPrefix = strings.Split(os.Getenv("HM_EX_IMAGETAGPREFIX"), ",")
	}
	if os.Getenv("HM_EX_IMAGETAGSUFFIX") != "" {
		exImageTagSuffix = strings.Split(os.Getenv("HM_EX_IMAGETAGSUFFIX"), ",")
	}
	if os.Getenv("HM_EX_IMAGELABEL") != "" {
		exImageLabel = strings.Split(os.Getenv("HM_EX_IMAGELABEL"), ",")
	}
}

func main() {
	startTime = getCurTimestamp()

	// set defaults
	hmUntil = 7 * 24 * 60 * 60 // 1 week
	hmDeleteDangling := 1
	hmEnforcing := 0
	dockerSocket := "/var/run/docker.sock"

	log.Print("Starting docker hausmeister ...")

	log.Print("Checking environment variables")

	if os.Getenv("HM_UNTIL") == "" {
		log.Print("No HM_UNTIL defined, using default")
	} else {
		tHmUntil, err := strconv.Atoi(os.Getenv("HM_UNTIL"))

		if err == nil {
			hmUntil = tHmUntil
		}
	}

	if os.Getenv("HM_DELETE_DANGLING") == "" {
		log.Print("No HM_DELETE_DANGLING defined")
	} else {
		tHmDeleteDangling, err := strconv.Atoi(os.Getenv("HM_DELETE_DANGLING"))

		if err == nil {
			hmDeleteDangling = tHmDeleteDangling
		}
	}

	if hmDeleteDangling == 1 {
		deleteDangling = true
	} else {
		deleteDangling = false
	}

	if os.Getenv("HM_ENFORCING") == "" {
		log.Print("No HM_ENFORCING defined")
	} else {
		tHmEnforcing, err := strconv.Atoi(os.Getenv("HM_ENFORCING"))

		if err == nil {
			hmEnforcing = tHmEnforcing
		}
	}

	if hmEnforcing == 1 {
		enforcing = true
	} else {
		enforcing = false
	}

	log.Print("Configuration:")
	log.Print(" HM_UNTIL           : ", hmUntil)
	log.Print(" HM_DELETE_DANGLING : ", hmDeleteDangling)
	log.Print(" HM_ENFORCING       : ", hmEnforcing)

	parseExcludeParameters()

	if _, err := os.Stat(dockerSocket); os.IsNotExist(err) {
		log.Fatal("Docker socket does not exist at ", dockerSocket)
	} else {
		log.Print("Docker socket exists")
	}

	clie, err := client.NewClientWithOpts(client.WithVersion("1.35"))
	cli = clie
	if err != nil {
		panic(err)
	}

	filter := filters.NewArgs()
	filter.Add("type", events.ContainerEventType)
	filter.Add("event", "create")
	messages, errs := cli.Events(context.Background(), types.EventsOptions{Filters: filter})

loop:
	for {
		select {
		case err := <-errs:
			if err != nil && err != io.EOF {
				panic(err)
			}

			break loop
		case e := <-messages:
			handleEvent(e.From)
		}
	}

	log.Print("Stopped docker hausmeister")
}
