/*
Copyright 2021 The Ceph-CSI Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/google/go-github/github"
	"golang.org/x/oauth2"
)

var (
	totalRequiredReviews int
	retry                int
)

type retestConfig struct {
	retryLimit          string
	requiredReviewCount string
	exemptlabel         string
	requiredlabel       string
	githubToken         string
	owner               string
	repo                string
	client              *github.Client
}

// standard input env variables in action more details at
// https://docs.github.com/en/actions/creating-actions/metadata-syntax-for-github-actions#inputs
func getConfig() *retestConfig {
	c := &retestConfig{}
	c.retryLimit = os.Getenv("INPUT_MAX-RETRY")
	c.requiredReviewCount = os.Getenv("INPUT_REQUIRED-APPROVE-COUNT")
	c.exemptlabel = os.Getenv("INPUT_EXEMPT-LABEL")
	c.requiredlabel = os.Getenv("INPUT_REQUIRED-LABEL")
	c.githubToken = os.Getenv("GITHUB_TOKEN")
	c.owner, c.repo = func() (string, string) {
		if os.Getenv("GITHUB_REPOSITORY") != "" {
			if len(strings.Split(os.Getenv("GITHUB_REPOSITORY"), "/")) == 2 {
				return strings.Split(os.Getenv("GITHUB_REPOSITORY"), "/")[0], strings.Split(os.Getenv("GITHUB_REPOSITORY"), "/")[1]
			}
		}
		return "", ""
	}()
	return c
}

// validate validates the input parameters.
func (c retestConfig) validate() error {
	if c.requiredlabel == "" {
		return errors.New("required-label is not set")
	}

	if c.githubToken == "" {
		return errors.New("GITHUB_TOKEN is not set")
	}

	if c.owner == "" || c.repo == "" {
		return errors.New("GITHUB_REPOSITORY is not set")
	}

	return nil
}

// createClient creates a new secure client.
func (c *retestConfig) createClient() {
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: c.githubToken},
	)
	tc := oauth2.NewClient(context.TODO(), ts)
	c.client = github.NewClient(tc)
}

func main() {
	var err error
	c := getConfig()
	if err = c.validate(); err != nil {
		log.Fatalf("%v", err)
	}

	retry, err = strconv.Atoi(c.retryLimit)
	if err != nil {
		log.Fatalf("max-retry %q is not valid", c.retryLimit)
	}

	totalRequiredReviews, err = strconv.Atoi(c.requiredReviewCount)
	if err != nil {
		log.Fatalf("required-review-count %q is not valid", c.requiredReviewCount)
	}

	c.createClient()

	opt := &github.PullRequestListOptions{}
	req, _, err := c.client.PullRequests.List(context.TODO(), c.owner, c.repo, opt)
	if err != nil {
		log.Fatalf("failed to list pull requests %v\n", err)
	}
	for _, re := range req {
		if (re.State != nil) && (*re.State == "open") {
			prNumber := re.GetNumber()
			log.Printf("PR with ID %d with Title %q is open\n", prNumber, re.GetTitle())
			for _, l := range re.Labels {
				// check if label is exempt
				if strings.EqualFold(c.exemptlabel, l.GetName()) {
					continue
				}
				// check if label is matching
				if !strings.EqualFold(c.requiredlabel, l.GetName()) {
					continue
				}

				// check if PR has required approvals
				if !c.checkPRRequiredApproval(prNumber) {
					continue
				}

				log.Printf("checking status for PR %d with label %s", prNumber, l.GetName())
				rs, _, err := c.client.Repositories.ListStatuses(context.TODO(), c.owner, c.repo, re.GetHead().GetSHA(), &github.ListOptions{})
				if err != nil {
					log.Printf("failed to list status %v\n", err)
					continue
				}

				statusList := filterStatusList(rs)
				failedTestFound := false
				for _, r := range statusList {
					log.Printf("found context %s with status %s\n", r.GetContext(), r.GetState())
					if contains([]string{"failed", "failure"}, r.GetState()) {
						log.Printf("found failed test %s\n", r.GetContext())
						failedTestFound = true
						// rebase the pr if it is behind the devel branch.
						if (re.MergeableState != nil) && (*re.MergeableState == "BEHIND") {
							comment := &github.IssueComment{
								Body: github.String("@mergifyio rebase"),
							}
							_, _, err := c.client.Issues.CreateComment(context.TODO(), c.owner, c.repo, prNumber, comment)
							if err != nil {
								log.Printf("failed to create comment %v\n", err)
							}
							break
						}

						// check if retest limit is reached
						msg := fmt.Sprintf("/retest %s", r.GetContext())
						ok, err := c.checkRetestLimitReached(prNumber, msg)
						if err != nil {
							log.Printf("failed to check retest limit %v\n", err)
							continue
						}
						if ok {
							log.Printf("Pull Request %d: %q reached  maximum attempt. skipping retest %v\n", prNumber, r.GetContext(), retry)
							continue
						}

						comment := &github.IssueComment{
							Body: github.String(msg),
						}
						_, _, err = c.client.Issues.CreateComment(context.TODO(), c.owner, c.repo, prNumber, comment)
						if err != nil {
							log.Printf("failed to create comment %v\n", err)
							continue
						}
						// Post comment with target URL for retesting
						msg = fmt.Sprintf("@%s %q test failed. Logs are available at [location](%s) for debugging", re.GetUser().GetLogin(), r.GetContext(), r.GetTargetURL())
						comment.Body = github.String(msg)
						_, _, err = c.client.Issues.CreateComment(context.TODO(), c.owner, c.repo, prNumber, comment)
						if err != nil {
							log.Printf("failed to create comment %v\n", err)
							continue
						}
					}
				}

				if failedTestFound {
					// comment `@Mergifyio requeue` so mergifyio adds the pr back into the queue.
					msg := "@Mergifyio requeue"
					comment := &github.IssueComment{
						Body: github.String(msg),
					}
					_, _, err = c.client.Issues.CreateComment(context.TODO(), c.owner, c.repo, prNumber, comment)
					if err != nil {
						log.Printf("failed to create comment %q: %v\n", msg, err)
					}
					// exit after adding retests to a pr once to avoid retesting multiple prs
					// at the same time.
					break
				}
			}
		}
	}
}

// checkPRRequiredApproval check PullRequest has required approvals.
func (c *retestConfig) checkPRRequiredApproval(prNumber int) bool {
	opts := github.ListOptions{PerPage: 100} // defaults to 30 reviews, too few sometimes
	rev, _, err := c.client.PullRequests.ListReviews(context.TODO(), c.owner, c.repo, prNumber, &opts)
	if err != nil {
		log.Printf("failed to list reviews %v\n", err)
		return false
	}
	approvedReviews := 0
	for _, rv := range rev {
		if rv.GetState() == "APPROVED" {
			approvedReviews += 1
		}
	}
	if !(approvedReviews >= totalRequiredReviews) {
		log.Printf("total approved reviews for PR %d are %d but required %d", prNumber, approvedReviews, totalRequiredReviews)
		return false
	}

	return true
}

// checkRetestLimitReached check if retest limit is reached.
func (c *retestConfig) checkRetestLimitReached(prNumber int, msg string) (bool, error) {
	creq, _, err := c.client.Issues.ListComments(context.TODO(), c.owner, c.repo, prNumber, &github.IssueListCommentsOptions{})
	if err != nil {
		return false, err
	}
	retestCount := 0

	for _, pc := range creq {
		if pc.GetBody() == msg {
			retestCount += 1
		}
	}
	log.Printf("found %d retries and remaining %d retries\n", retestCount, retry-retestCount)
	if retestCount >= int(retry) {
		return true, nil
	}

	return false, nil
}

// containers check if slice contains string.
func contains(s []string, e string) bool {
	for _, a := range s {
		if a == e {
			return true
		}
	}

	return false
}

// filterStatusesList returns list of unique and recently updated github RepoStatuses.
// Raw github RepoStatus list may contain duplicate and older statuses.
func filterStatusList(rawStatusList []*github.RepoStatus) []*github.RepoStatus {
	testStatus := make(map[string]*github.RepoStatus)

	for _, r := range rawStatusList {
		status, ok := testStatus[r.GetContext()]
		if !ok || r.GetUpdatedAt().After(status.GetUpdatedAt()) {
			testStatus[r.GetContext()] = r
		}
	}

	statusList := make([]*github.RepoStatus, 0)
	for _, rs := range testStatus {
		statusList = append(statusList, rs)
	}

	return statusList
}
