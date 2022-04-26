package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	ec2IMDSUrl        = "http://169.254.169.254"
	ecsTMDSV2Url      = "http://169.254.170.2/v2/metadata"
	httpClientTimeout = 10 * time.Second
	serverTitle       = "SERVER_TITLE"
	serverPort        = "SERVER_PORT"
	otherServerPort   = "OTHER_SERVER_PORT"
	scDeps            = "SC_DEPS"
)

var action string

func main() {
	action = os.Getenv(serverTitle)
	port, err := strconv.Atoi(os.Getenv(serverPort))
	if err != nil {
		log.Printf("%s server got error when parsing port from env %v. Assigning 8080\n", action, err)
		port = 8080
	}
	otherPort, err := strconv.Atoi(os.Getenv(otherServerPort))
	if err != nil {
		log.Printf("%s server got error when parsing other port from env %v. Assigning 9090\n", action, err)
		otherPort = 9090
	}
	log.Printf("%s:%d server starting up...\n", action, port)
	err = runInitialTests()
	if err != nil {
		log.Printf("%s server got error when running initial tests: %v\n", action, err)
	}
	wg := new(sync.WaitGroup)
	wg.Add(2)
	go func() {
		defer wg.Done()
		server := createServer(port, serverHandlerForSC)
		log.Println(server.ListenAndServe())
	}()

	go func() {
		defer wg.Done()
		server := createServer(otherPort, otherNonSC)
		log.Println(server.ListenAndServe())
	}()
	wg.Wait()
	log.Printf("%s server done", action)
	os.Exit(0)
}

func createServer(port int, handlerFunc func(w http.ResponseWriter, req *http.Request)) *http.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/", handlerFunc)
	return &http.Server{
		Addr:    fmt.Sprintf(":%v", port),
		Handler: mux,
	}
}

func otherNonSC(w http.ResponseWriter, req *http.Request) {
	log.Printf("request headers: %v\n", req.Header)
	fmt.Fprint(w, "Hello from a Non-SC service")
}
func serverHandlerForSC(w http.ResponseWriter, req *http.Request) {
	log.Printf("request headers: %v\n", req.Header)
	resp := fmt.Sprintf("Hello from %s service\n", action)
	depsStr := os.Getenv(scDeps)
	if len(depsStr) > 0 {
		deps := strings.Split(depsStr, ",")
		for _, dep := range deps {
			depElems := strings.Split(dep, ":")
			if len(depElems) != 3 {
				log.Printf("WARNING - dependency incorrectly formatted: %s\n", dep)
				continue
			}
			depUrl := "http://" + depElems[0] + ":" + depElems[2]
			depRes, err := http.Get(depUrl)
			if err != nil {
				log.Printf("ERROR - unable to call dependency: %s (%v)\n", depUrl, err)
				resp = fmt.Sprintf("%s\tUnable to call dependency: %s (%v)\n", resp, depUrl, err)
				continue
			}
			body, err := ioutil.ReadAll(depRes.Body)
			if err != nil {
				log.Printf("ERROR - unable to read dependency response: %s (%v)\n", depUrl, err)
				resp = fmt.Sprintf("%s\tUnable to read dependency response: %s (%v)\n", resp, depUrl, err)
				continue
			}
			resp = fmt.Sprintf("%s\tResponse from \"%s\": %s\n", resp, depUrl, body)
		}
	}
	fmt.Fprint(w, resp)
}

func runInitialTests() error {
	region, err := getRegionFromIMDS()
	if err != nil {
		log.Printf("error getting region from IMDS due to error: %v. setting to us-west-2 and continue checking the rest\n", err)
		region = "us-west-2"
	} else {
		log.Printf("got region from imds: %s\n", region)
	}

	md, err := getTaskMetadata()
	if err != nil {
		return err
	}

	log.Printf("got task metadata: %s\n", md)
	return nil
}

func getRegionFromIMDS() (string, error) {
	log.Println("getting region from imds")
	httpClient := &http.Client{
		Timeout: httpClientTimeout,
	}
	return getIMDSV1(httpClient, "/latest/meta-data/placement/region")
}

func getIMDSV1(httpClient *http.Client, mdSuffix string) (string, error) {
	regionUrl := fmt.Sprintf("%s%s", ec2IMDSUrl, mdSuffix)
	req, err := http.NewRequest("GET", regionUrl, nil)
	if err != nil {
		return "", fmt.Errorf("unable to create get region request: %w", err)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("unable to get region from imds: %w", err)
	}
	defer resp.Body.Close()

	region, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("unable to read region from response: %w", err)
	}

	return string(region), nil
}

func getTaskMetadata() (string, error) {
	log.Println("verifying task metadata")

	resp, err := http.Get(ecsTMDSV2Url)
	if err != nil {
		return "", fmt.Errorf("unable to make request to task metadata endpoint: %w", err)
	}

	md, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("unable to read metadata from response: %w", err)
	}

	return string(md), nil
}
