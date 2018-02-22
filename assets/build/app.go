package main

import (
    "os"
    "time"
    "log"
    "strconv"
    "bufio"
    "io"
    "os/exec"
    "encoding/json"
    "strings"
)

var start_time int64
var hm_until int
var hm_delete_dangling int
var hm_enforcing int
var image_history = make(map[string]int64)
var docker_socket string

type Event struct {
    From string `json:"from"`
}

type Image struct {
    ID string   `json:"ID"`
}

func getCurTimestamp() int64 {
    return int64(time.Now().Unix())
}

func cleanJson(s string) string {
    return strings.Replace(s, "'", "", -1)
}

func getImageId(s string) string {
    id := ""

    cmd := exec.Command("docker", "image", "ls", s, "--format", "'{{json .}}'")
    cmd.Stderr = os.Stderr
    stdout, err := cmd.StdoutPipe()
    if nil != err {
        log.Fatalf("Error obtaining stdout: %s", err.Error())
    }
    reader := bufio.NewReader(stdout)
    go func(reader io.Reader) {
        scanner := bufio.NewScanner(reader)
        for scanner.Scan() {
            data := Image{}
            err := json.Unmarshal([]byte(cleanJson(scanner.Text())), &data)

            if err != nil {
                log.Print(err.Error())

                return
            }
            log.Print(" found id of image: ", data.ID)

            id = data.ID
        }
    }(reader)

    if err := cmd.Start(); nil != err {
        log.Fatalf("Error starting program: %s, %s", cmd.Path, err.Error())
    }
    cmd.Wait()

    log.Printf("[getImageId] image name '%s' mapped to image id '%s'", s, id)
    return id
}

func updateImageHistory(image_id string) {
    image_history[image_id] = getCurTimestamp()

    return
}

func checkForRunningContainer(image_id string, all bool) bool {
    var container_ids []string
    s := []string{"ancestor=", image_id}
    cmd := exec.Command("docker", "ps", "-q", "--filter", strings.Join(s,""))
    if all == true {
        cmd = exec.Command("docker", "ps", "-a", "-q", "--filter", strings.Join(s,""))
    }
    cmd.Stderr = os.Stderr
    stdout, err := cmd.StdoutPipe()
    if nil != err {
        log.Fatalf("Error obtaining stdout: %s", err.Error())
    }
    reader := bufio.NewReader(stdout)
    go func(reader io.Reader) {
        scanner := bufio.NewScanner(reader)
        for scanner.Scan() {
            container_ids = append(container_ids, scanner.Text())
        }
    }(reader)

    if err := cmd.Start(); nil != err {
        log.Fatalf("Error starting program: %s, %s", cmd.Path, err.Error())
    }
    cmd.Wait()

    var ret bool
    if len(container_ids) > 0 {
        ret = true
    } else {
        ret = false
    }

/*  DEBUG
    log.Printf("[checkForRunningContainer] result for image id '%s' and flag all %s => %s", image_id, all, ret)
*/
    return ret
}

func rmImage(image_id string) {
    // check if not in use
    if checkForRunningContainer(image_id, true) {
        if hm_enforcing == 0 {
            log.Print("Do nothing. Some containers are running or stopped!")

            return
        } else {
            log.Print("Some containers are running or stopped.")
        }
    } else {
        log.Print("Found no running containers. Proceeding...")
    }

    // if in use by stopped container decide on HM_ENFORCING
    if checkForRunningContainer(image_id, false) {
        log.Print("Do nothing. Some containers are running!")

        return
    }

    // delete image
    log.Print("trying to delete image now")
    cmd := exec.Command("docker", "image", "rm", "-f", image_id)
    cmd.Stderr = os.Stderr
    stdout, err := cmd.StdoutPipe()
    if nil != err {
        log.Fatalf("Error obtaining stdout: %s", err.Error())
    }
    reader := bufio.NewReader(stdout)
    go func(reader io.Reader) {
        scanner := bufio.NewScanner(reader)
        for scanner.Scan() {
            log.Print(scanner.Text())
        }
    }(reader)

    if err := cmd.Start(); nil != err {
        log.Fatalf("Error starting program: %s, %s", cmd.Path, err.Error())
    }
    cmd.Wait()

    // remove image_id from image_history map
    if getImageId(image_id) == "" {
        log.Print("Image seems to be deleted")
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
            log.Printf("Image %s is to old, deleting", image_id)
            rmImage(image_id)
        }
    }

    return
}

func deleteGrandFatheredImages() {
    var image_id string
    cmd := exec.Command("docker", "image", "ls", "--format", "'{{.ID}}'")
    cmd.Stderr = os.Stderr
    stdout, err := cmd.StdoutPipe()
    if nil != err {
        log.Fatalf("Error obtaining stdout: %s", err.Error())
    }
    reader := bufio.NewReader(stdout)
    go func(reader io.Reader) {
        scanner := bufio.NewScanner(reader)
        for scanner.Scan() {
            image_id = cleanJson(scanner.Text())
            log.Print("  working on image: ", image_id)
            if _, ok := image_history[image_id]; ok {
                log.Print("Image is not grandfathered. Skipping!")

                continue
            } else {
                rmImage(image_id)
            }
        }
    }(reader)

    if err := cmd.Start(); nil != err {
        log.Fatalf("Error starting program: %s, %s", cmd.Path, err.Error())
    }
    cmd.Wait()

    return
}

func deleteDanglingImages() {
    log.Print("Trying to clean dangled images")
    cmd := exec.Command("docker", "image", "prune", "--force")
    cmd.Stderr = os.Stderr
    stdout, err := cmd.StdoutPipe()
    if nil != err {
        log.Fatalf("Error obtaining stdout: %s", err.Error())
    }
    reader := bufio.NewReader(stdout)
    go func(reader io.Reader) {
        scanner := bufio.NewScanner(reader)
        for scanner.Scan() {
            log.Print(scanner.Text())
        }
    }(reader)

    if err := cmd.Start(); nil != err {
        log.Fatalf("Error starting program: %s, %s", cmd.Path, err.Error())
    }
    cmd.Wait()
}

func handleEvent(s string) {
    image_name := ""
    data := Event{}
    err := json.Unmarshal([]byte(cleanJson(s)), &data)
    log.Print(" found event for image: ", data.From)
    if err != nil {
        log.Print(err.Error())

        return
    }

    image_name = data.From

    image_id := getImageId(image_name)

    if image_id == "" {
       log.Print("Could not find image id")

       return
    }

    updateImageHistory(image_id)

    deleteOldImages()

    if hm_delete_dangling == 1 {
        deleteDanglingImages()
    }

    if getCurTimestamp()>start_time+int64(hm_until) && hm_enforcing == 1 {
        log.Print("Trying to delete images without received start event")
        deleteGrandFatheredImages()
    }
}

func main() {
    start_time = getCurTimestamp()

    // set defaults
    hm_until = 7*24*60*60 // 1 week
    hm_delete_dangling = 1
    hm_enforcing = 0
    docker_socket = "/var/run/docker.sock"

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

    if os.Getenv("HM_ENFORCING") == "" {
        log.Print("No HM_ENFORCING defined")
    } else {
        t_hm_enforcing, err := strconv.Atoi(os.Getenv("HM_ENFORCING"))

        if (err == nil) {
            hm_enforcing = t_hm_enforcing
        }
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

    cmd := exec.Command("docker", "events", "-f", "type=container", "-f", "event=create", "--format", "'{{json .}}'")
    cmd.Stderr = os.Stderr
    stdout, err := cmd.StdoutPipe()
    if nil != err {
        log.Fatalf("Error obtaining stdout: %s", err.Error())
    }
    reader := bufio.NewReader(stdout)
    go func(reader io.Reader) {
        scanner := bufio.NewScanner(reader)
        for scanner.Scan() {
            handleEvent(scanner.Text())
        }
    }(reader)

    if err := cmd.Start(); nil != err {
        log.Fatalf("Error starting program: %s, %s", cmd.Path, err.Error())
    }
    cmd.Wait()

    log.Print("Stopped docker hausmeister")
}
