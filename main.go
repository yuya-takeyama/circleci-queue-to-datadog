package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	datadog "github.com/zorkian/go-datadog-api"
)

var datadogClient = datadog.NewClient(os.Getenv("DATADOG_API_KEY"), "")
var runningMetricName = "circleci.queue.running"
var notRunningMetricName = "circleci.queue.not_running"

func main() {
	for {
		go getAndSendMetrics()
		time.Sleep(15 * time.Second)
	}
}

func getAndSendMetrics() {
	now := time.Now()
	timestamp := float64(now.Unix())
	if running, notRunning, err := getQueueNumbers(); err != nil {
		log.Println(err)
	} else {
		log.Printf("running:%d\tnot_running:%d\n", int(running), int(notRunning))

		metrics := []datadog.Metric{
			datadog.Metric{
				Metric: &runningMetricName,
				Points: []datadog.DataPoint{{&timestamp, &running}},
			},
			datadog.Metric{
				Metric: &notRunningMetricName,
				Points: []datadog.DataPoint{{&timestamp, &notRunning}},
			},
		}
		if err := datadogClient.PostMetrics(metrics); err != nil {
			log.Printf("failed to post metrics to Datadog: %s\n", err)
		} else {
			log.Printf("successfully sent metrics at %s to Datadog!\n", now.Format(time.RFC3339))
		}
	}
}

func getQueueNumbers() (float64, float64, error) {
	req, reqErr := http.NewRequest("GET", "https://circleci.com/api/v1.1/recent-builds?limit=100&circle-token="+os.Getenv("CIRCLECI_API_TOKEN"), nil)
	if reqErr != nil {
		return -1, -1, fmt.Errorf("failed to build HTTP request to CircleCI API: %s", reqErr)
	}

	req.Header.Add("Accept", "application/json")
	res, resErr := http.DefaultClient.Do(req)
	if resErr != nil {
		return -1, -1, fmt.Errorf("failed to get recent builds from CircleCI API: %s", resErr)
	}

	defer res.Body.Close()

	var running float64
	var notRunning float64

	d := json.NewDecoder(res.Body)
	for {
		var j []map[string]interface{}
		if err := d.Decode(&j); err != nil {
			if err == io.EOF {
				break
			} else {
				return -1, -1, fmt.Errorf("failed to parse response from CircleCI API: %s", err)
			}
		}

		for _, job := range j {
			if job["lifecycle"] == "running" {
				running++
			} else if job["lifecycle"] == "not_running" {
				notRunning++
			}

		}
	}

	return running, notRunning, nil
}
