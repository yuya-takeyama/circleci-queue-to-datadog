package main

import "testing"

func TestJobCountsIncrOnce(t *testing.T) {
	jobCounts := newJobCounts()
	job := createCircleCIJobWithLifeCycle("running")
	jobCounts.incr(job)

	expectedLen := 1
	actualLen := len(jobCounts.jobCounts)
	if actualLen != expectedLen {
		t.Errorf("incr() result is wrong: expected: %d, actual: %d", expectedLen, actualLen)
	}

	key := "github/yuya-takeyama/jr/master"
	jobCount := jobCounts.jobCounts[key]
	expectedJobCount := 1
	actualJobCount := jobCount.Count
	if actualJobCount != expectedJobCount {
		t.Errorf("incr() result is wrong: Count of %s is wrong: expected: %d, actual: %d", key, expectedJobCount, actualJobCount)
	}
}

func TestJobCountsIncrTwice(t *testing.T) {
	jobCounts := newJobCounts()
	job := createCircleCIJobWithLifeCycle("running")
	jobCounts.incr(job)
	jobCounts.incr(job)

	expectedLen := 1
	actualLen := len(jobCounts.jobCounts)
	if actualLen != expectedLen {
		t.Errorf("incr() result is wrong: expected: %d, actual: %d", expectedLen, actualLen)
	}

	key := "github/yuya-takeyama/jr/master"
	jobCount := jobCounts.jobCounts[key]
	expectedJobCount := 2
	actualJobCount := jobCount.Count
	if actualJobCount != expectedJobCount {
		t.Errorf("incr() result is wrong: Count of %s is wrong: expected: %d, actual: %d", key, expectedJobCount, actualJobCount)
	}
}

func createCircleCIJobWithLifeCycle(lifecycle string) *circleCiJob {
	return &circleCiJob{
		VcsType:   "github",
		Username:  "yuya-takeyama",
		Reponame:  "jr",
		Branch:    "master",
		LifeCycle: lifecycle,
	}
}
