package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	flags "github.com/jessevdk/go-flags"
	datadog "github.com/zorkian/go-datadog-api"
)

const appName = "circleci-queue-to-datadog"

type options struct {
	Usernames   string `long:"usernames" description:"Comma-separated list of usernames to check queue"`
	Interval    int    `long:"interval" description:"Interval to check CircleCI queue in seconds" default:"60"`
	ShowVersion bool   `short:"v" long:"version" description:"Show version"`
}

var opts options
var targetUsernames []string

var datadogClient = datadog.NewClient(os.Getenv("DATADOG_API_KEY"), "")
var runningMetricName = "circleci.queue.running"
var notRunningMetricName = "circleci.queue.not_running"

type circleCiJob struct {
	VcsType   string `json:"vcs_type"`
	Username  string `json:"username"`
	Reponame  string `json:"reponame"`
	Branch    string `json:"branch"`
	LifeCycle string `json:"lifecycle"`
}

func main() {
	parser := flags.NewParser(&opts, flags.Default^flags.PrintErrors)
	parser.Name = appName

	if _, err := parser.Parse(); err != nil {
		if err, ok := err.(*flags.Error); ok && err.Type == flags.ErrHelp {
			fmt.Print(err)
			os.Exit(0)
		} else {
			log.Fatalf("Option error: %s", err)
		}
	}

	if opts.ShowVersion {
		fmt.Printf("%s v%s, build %s\n", appName, version, gitCommit)
		os.Exit(0)
	}

	if len(opts.Usernames) > 1 {
		targetUsernames = append(targetUsernames, strings.Split(opts.Usernames, ",")...)
	}

	for {
		go getAndSendMetrics()
		time.Sleep(time.Duration(opts.Interval) * time.Second)
	}
}

func getAndSendMetrics() {
	now := time.Now()
	if runningCounts, notRunningCounts, err := getJobCounts(); err != nil {
		log.Println(err)
	} else {
		log.Printf("running:%d\tnot_running:%d\n", runningCounts.getTotalCount(), notRunningCounts.getTotalCount())

		metrics := append(runningCounts.toMetrics(now, runningMetricName), notRunningCounts.toMetrics(now, notRunningMetricName)...)

		if err := datadogClient.PostMetrics(metrics); err != nil {
			log.Printf("failed to post metrics to Datadog: %s\n", err)
		} else {
			log.Printf("successfully sent metrics at %s to Datadog!\n", now.Format(time.RFC3339))
		}
	}
}

func getJobCounts() (*jobCounts, *jobCounts, error) {
	runningCounts := newJobCounts()
	notRunningCounts := newJobCounts()

	req, reqErr := http.NewRequest("GET", "https://circleci.com/api/v1.1/recent-builds?limit=100&circle-token="+os.Getenv("CIRCLECI_API_TOKEN"), nil)
	if reqErr != nil {
		return runningCounts, notRunningCounts, fmt.Errorf("failed to build HTTP request to CircleCI API: %s", reqErr)
	}

	req.Header.Add("Accept", "application/json")
	res, resErr := http.DefaultClient.Do(req)
	if resErr != nil {
		return runningCounts, notRunningCounts, fmt.Errorf("failed to get recent builds from CircleCI API: %s", resErr)
	}

	defer res.Body.Close()

	d := json.NewDecoder(res.Body)
	for {
		var jobs []*circleCiJob
		if err := d.Decode(&jobs); err != nil {
			if err == io.EOF {
				break
			} else {
				return runningCounts, notRunningCounts, fmt.Errorf("failed to parse response from CircleCI API: %s", err)
			}
		}

		incrJobCounts(jobs, runningCounts, notRunningCounts)
	}

	return runningCounts, notRunningCounts, nil
}

func incrJobCounts(jobs []*circleCiJob, runningCounts, notRunningCounts *jobCounts) (*jobCounts, *jobCounts) {
	for _, job := range jobs {
		if isTargetJob(job) {
			if job.LifeCycle == "running" {
				runningCounts.incr(job)
			} else if job.LifeCycle == "not_running" {
				notRunningCounts.incr(job)
			}
		}
	}

	return runningCounts, notRunningCounts
}

func isTargetJob(job *circleCiJob) bool {
	if len(targetUsernames) == 0 {
		return true
	}
	for _, targetUsername := range targetUsernames {
		if job.Username == targetUsername {
			return true
		}
	}

	return false
}
