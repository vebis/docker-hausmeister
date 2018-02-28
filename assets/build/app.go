package main

import (
    "context"
    "os"
    "time"
    "log"
    "strconv"
    "io"

    "github.com/docker/docker/api/types"
    "github.com/docker/docker/api/types/events"
    "github.com/docker/docker/api/types/filters"
    "github.com/docker/docker/client"
)

var start_time int64
var hm_until int
var image_history = make(map[string]int64)

var cli *client.Client

var enforcing bool
var delete_dangling bool

func getCurTimestamp() int64 {
    return int64(time.Now().Unix())
}

func getImageId(image_name string) string {
    id := ""

    image, bytes, err := cli.ImageInspectWithRaw(context.Background(), image_name)
    if err != nil || len(bytes) == 0 {
        return id
    }

    id = image.ID

    log.Printf("Image name '%s' mapped to image id '%s'", image_name, id)

    return id
}

func updateImageHistory(image_id string) {
    image_history[image_id] = getCurTimestamp()

    return
}

func checkForRunningContainer(image_id string, all bool) bool {
    filter := filters.NewArgs()
    filter.Add("ancestor", image_id)
    options := types.ContainerListOptions{Quiet: true, All: all, Filters : filter }

    containers, err := cli.ContainerList(context.Background(), options)

    ret := true // save default
    if err != nil {
        log.Print(err)

        return ret
    }

    if len(containers) == 0 {
        ret = false
    }

    return ret
}

func hasRunningContainers(image_id string) bool {
    return checkForRunningContainer(image_id, false)
}

func hasStoppedContainers(image_id string) bool {
    if checkForRunningContainer(image_id, false) == false && checkForRunningContainer(image_id, true) == true {
	return true
    } else {
        return false
    }
}

func rmImage(image_id string) {
    if hasRunningContainers(image_id) {
        log.Print("    Some containers are running. Skipping!")

        return
    }

    if hasStoppedContainers(image_id) && !enforcing {
        log.Print("    Some containers are stopped. Skipping!")

        return
    }

    imagedeleteresponses, err := cli.ImageRemove(context.Background(), image_id, types.ImageRemoveOptions{Force:true})
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

    // remove image_id from image_history map
    if getImageId(image_id) == "" {
        log.Print("  Image seems to be deleted")
        delete(image_history, image_id)
    }

    return
}

func deleteOldImages() {
    cur_time := getCurTimestamp()
    log.Print("Searching for old images:")

    for image_id, last := range image_history {
        log.Printf("  trying image: %s", image_id)
        if last < cur_time-int64(hm_until) {
            log.Print("    Image is to old, deleting.")
            rmImage(image_id)
        } else {
            log.Print("    Image is to new. Skipping.")
        }
    }

    return
}

func deleteGrandFatheredImages() {
    log.Print("Trying to delete images without received start event")
    var image_id string

    images, err := cli.ImageList(context.Background(), types.ImageListOptions{})
    if err != nil {
        log.Print(err)

        return
    }

    for _, image := range images {
        image_id = image.ID
        log.Print("  trying image: ", image_id)
        if _, ok := image_history[image_id]; ok {
            log.Print("    Image is not grandfathered. Skipping!")

            continue
        } else {
            rmImage(image_id)
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

func handleEvent(image_name string) {
    log.Printf("Handle event for image '%s'",  image_name)

    image_id := getImageId(image_name)

    if image_id == "" {
       log.Print("Could not find image id")

       return
    }

    updateImageHistory(image_id)

    deleteOldImages()

    if delete_dangling {
        deleteDanglingImages()
    }

    if getCurTimestamp()>start_time+int64(hm_until) && enforcing {
        deleteGrandFatheredImages()
    }
}

func main() {
    start_time = getCurTimestamp()

    // set defaults
    hm_until = 7*24*60*60 // 1 week
    hm_delete_dangling := 1
    hm_enforcing := 0
    docker_socket := "/var/run/docker.sock"

    log.Print("Starting docker hausmeister ...")

    log.Print("Checking environment variables")

    if os.Getenv("HM_UNTIL") == "" {
        log.Print("No HM_UNTIL defined, using default")
    } else {
        t_hm_until, err := strconv.Atoi(os.Getenv("HM_UNTIL"))

        if (err == nil) {
            hm_until = t_hm_until
        }
    }

    if os.Getenv("HM_DELETE_DANGLING") == "" {
        log.Print("No HM_DELETE_DANGLING defined")
    } else {
        t_hm_delete_dangling, err := strconv.Atoi(os.Getenv("HM_DELETE_DANGLING"))

        if (err == nil) {
            hm_delete_dangling = t_hm_delete_dangling
        }
    }

    if hm_delete_dangling == 1 {
        delete_dangling = true
    } else {
        delete_dangling = false
    }

    if os.Getenv("HM_ENFORCING") == "" {
        log.Print("No HM_ENFORCING defined")
    } else {
        t_hm_enforcing, err := strconv.Atoi(os.Getenv("HM_ENFORCING"))

        if (err == nil) {
            hm_enforcing = t_hm_enforcing
        }
    }

    if hm_enforcing == 1 {
        enforcing = true
    } else {
        enforcing = false
    }

    log.Print("Configuration:")
    log.Print(" HM_UNTIL           : ", hm_until)
    log.Print(" HM_DELETE_DANGLING : ", hm_delete_dangling)
    log.Print(" HM_ENFORCING       : ", hm_enforcing)

    if _, err := os.Stat(docker_socket); os.IsNotExist(err) {
        log.Fatal("Docker socket does not exist at ", docker_socket)
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

    loop :
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
