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

type jobCount struct {
	VcsType  string
	Username string
	Reponame string
	Branch   string
	Count    int
}

type jobCounts struct {
	jobCounts map[string]*jobCount
}

func newJobCounts() *jobCounts {
	return &jobCounts{
		jobCounts: make(map[string]*jobCount),
	}
}

func (o *jobCounts) incr(job *circleCiJob) {
	key := fmt.Sprintf("%s/%s/%s/%s", job.VcsType, job.Username, job.Reponame, job.Branch)
	if jc, ok := o.jobCounts[key]; ok {
		jc.Count++
	} else {
		o.jobCounts[key] = &jobCount{
			VcsType:  job.VcsType,
			Username: job.Username,
			Reponame: job.Reponame,
			Branch:   job.Branch,
			Count:    1,
		}
	}
}

func (o *jobCounts) toMetrics(now time.Time, metricName string) []datadog.Metric {
	metrics := make([]datadog.Metric, len(o.jobCounts))
	timestamp := float64(now.Unix())

	i := 0
	for _, jobCount := range o.jobCounts {
		count := float64(jobCount.Count)
		metrics[i] = datadog.Metric{
			Metric: &metricName,
			Points: []datadog.DataPoint{{&timestamp, &count}},
			Tags: []string{
				fmt.Sprintf("vcs_type:%s", jobCount.VcsType),
				fmt.Sprintf("username:%s", jobCount.Username),
				fmt.Sprintf("reponame:%s", jobCount.Reponame),
				fmt.Sprintf("branch:%s", jobCount.Branch),
			},
		}
		i++
	}

	return metrics
}

func (o *jobCounts) getTotalCount() int {
	cnt := 0

	for _, jobCount := range o.jobCounts {
		cnt += jobCount.Count
	}

	return cnt
}

type circleCiJob struct {
	VcsType   string `json:"vcs_type"`
	Username  string `json:"username"`
	Reponame  string `json:"reponame"`
	Branch    string `json:"branch"`
	LifeCycle string `json:"lifecycle"`
}

func main() {
	for {
		go getAndSendMetrics()
		time.Sleep(60 * time.Second)
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
		if job.LifeCycle == "running" {
			runningCounts.incr(job)
		} else if job.LifeCycle == "not_running" {
			notRunningCounts.incr(job)
		}
	}

	return runningCounts, notRunningCounts
}
