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
	"github.com/k0kubun/pp"
	datadog "github.com/zorkian/go-datadog-api"
)

const appName = "circleci-queue-to-datadog"

type options struct {
	Usernames   string `long:"usernames" description:"Comma-separated list of usernames to check queue"`
	Interval    int    `long:"interval" description:"Interval to check CircleCI queue in seconds" default:"60"`
	Once        bool   `long:"once" description:"Exits after the first check"`
	ShowVersion bool   `short:"v" long:"version" description:"Show version"`
}

var opts options
var targetUsernames []string

var datadogClient = datadog.NewClient(os.Getenv("DATADOG_API_KEY"), "")
var runningMetricName = "circleci.queue.running"
var notRunningMetricName = "circleci.queue.not_running"

var isDebug = os.Getenv("CIRCLECI_QUEUE_TO_DATADOG_DEBUG") != ""

type circleCiJob struct {
	VcsType   string `json:"vcs_type"`
	Username  string `json:"username"`
	Reponame  string `json:"reponame"`
	Branch    string `json:"branch"`
	LifeCycle string `json:"lifecycle"`
}

func (job *circleCiJob) toKey() string {
	return fmt.Sprintf("%s/%s/%s/%s", job.VcsType, job.Username, job.Reponame, job.Branch)
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

	if opts.Once {
		if opts.Interval > 0 {
			log.Println("--interval has no effect with --once mode")
		}

		getAndSendMetrics()
		return
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
		log.Printf("running:%d\tnot_running:%d", runningCounts.getTotalCount(), notRunningCounts.getTotalCount())

		runningMetrics := runningCounts.toMetrics(now, runningMetricName)
		notRunningMetrics := notRunningCounts.toMetrics(now, notRunningMetricName)
		metrics := append(runningMetrics, notRunningMetrics...)

		if isDebug {
			fmt.Fprintln(os.Stderr, "Running:")
			pp.Fprintln(os.Stderr, runningMetrics)
			fmt.Fprintln(os.Stderr, "Not Running:")
			pp.Fprintln(os.Stderr, notRunningMetrics)
		} else {
			if err := datadogClient.PostMetrics(metrics); err != nil {
				log.Printf("failed to post metrics to Datadog: %s", err)
			} else {
				log.Printf("successfully sent metrics at %s to Datadog!", now.Format(time.RFC3339))
			}
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
				notRunningCounts.ensure(job)
			} else if job.LifeCycle == "not_running" {
				runningCounts.ensure(job)
				notRunningCounts.incr(job)
			} else {
				runningCounts.ensure(job)
				notRunningCounts.ensure(job)
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
