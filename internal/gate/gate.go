package gate

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"
)

type Case struct {
	ID             string `json:"id"`
	Title          string `json:"title"`
	Locale         string `json:"locale"`
	Input          string `json:"input"`
	RequiredSource string `json:"requiredSource"`
	RequiredText   string `json:"requiredText"`
	MaxLatencyMS   int    `json:"maxLatencyMs"`
	Critical       bool   `json:"critical"`
	HumanReview    bool   `json:"humanReview"`
}

type Suite struct {
	Name  string `json:"name"`
	Cases []Case `json:"cases"`
}

type Output struct {
	Response  json.RawMessage `json:"response"`
	LatencyMS int             `json:"latencyMs"`
}

type Version struct {
	ID      string            `json:"id"`
	Outputs map[string]Output `json:"outputs"`
}

type Check struct {
	Name   string `json:"name"`
	Passed bool   `json:"passed"`
	Detail string `json:"detail"`
}

type Result struct {
	Case           Case    `json:"case"`
	Baseline       Output  `json:"baseline"`
	Candidate      Output  `json:"candidate"`
	BaselineChecks []Check `json:"baselineChecks"`
	Checks         []Check `json:"checks"`
	Regression     bool    `json:"regression"`
	NeedsReview    bool    `json:"needsReview"`
	Review         string  `json:"review,omitempty"`
}

type Run struct {
	ID        string    `json:"id"`
	Suite     string    `json:"suite"`
	Baseline  string    `json:"baseline"`
	Candidate string    `json:"candidate"`
	CreatedAt time.Time `json:"createdAt"`
	Results   []Result  `json:"results"`
	Status    string    `json:"status"`
}

func checks(c Case, out Output) []Check {
	var response struct {
		Answer  string   `json:"answer"`
		Sources []string `json:"sources"`
	}
	valid := json.Unmarshal(out.Response, &response) == nil &&
		strings.TrimSpace(response.Answer) != "" && response.Sources != nil
	return []Check{
		{"Response schema", valid, "Requires an answer and a source list."},
		{"Policy wording", valid && strings.Contains(response.Answer, c.RequiredText), c.RequiredText},
		{"Required citation", valid && slices.Contains(response.Sources, c.RequiredSource), c.RequiredSource},
		{"Latency budget", out.LatencyMS > 0 && out.LatencyMS <= c.MaxLatencyMS,
			fmt.Sprintf("%d ms / %d ms budget", out.LatencyMS, c.MaxLatencyMS)},
	}
}

func (r *Run) UpdateStatus() {
	r.Status = "Passed"
	for _, result := range r.Results {
		if result.Review == "rejected" {
			r.Status = "Blocked"
			return
		}
		failed := slices.ContainsFunc(result.Checks, func(check Check) bool { return !check.Passed })
		if result.Case.Critical && failed {
			r.Status = "Blocked"
			return
		}
		if result.NeedsReview && result.Review == "" {
			r.Status = "Review required"
		}
		if failed && r.Status == "Passed" {
			r.Status = "Warning"
		}
	}
}

var caseIDPattern = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`)

func changed(before, after json.RawMessage) bool {
	var previous, proposed any
	if json.Unmarshal(before, &previous) != nil || json.Unmarshal(after, &proposed) != nil {
		return string(before) != string(after)
	}
	return !reflect.DeepEqual(previous, proposed)
}

func Compare(ctx context.Context, suite Suite, baseline, candidate Version) (Run, error) {
	run := Run{Suite: suite.Name, Baseline: baseline.ID, Candidate: candidate.ID,
		CreatedAt: time.Now().UTC(), Results: make([]Result, len(suite.Cases))}
	if len(suite.Cases) == 0 || len(suite.Cases) > 200 || strings.TrimSpace(suite.Name) == "" || baseline.ID == "" || candidate.ID == "" {
		return Run{}, fmt.Errorf("supply a named suite with 1–200 cases and both version IDs")
	}
	seen := make(map[string]bool)
	for _, c := range suite.Cases {
		if !caseIDPattern.MatchString(c.ID) || strings.TrimSpace(c.Title) == "" || seen[c.ID] || c.MaxLatencyMS <= 0 || c.RequiredSource == "" || c.RequiredText == "" {
			return Run{}, fmt.Errorf("invalid or duplicate case %q", c.ID)
		}
		seen[c.ID] = true
		if _, ok := baseline.Outputs[c.ID]; !ok {
			return Run{}, fmt.Errorf("baseline has no response for %s", c.ID)
		}
		if _, ok := candidate.Outputs[c.ID]; !ok {
			return Run{}, fmt.Errorf("candidate has no response for %s", c.ID)
		}
	}
	jobs := make(chan int, len(suite.Cases))
	for index := range suite.Cases {
		jobs <- index
	}
	close(jobs)
	var workers sync.WaitGroup
	for range min(4, len(suite.Cases)) {
		workers.Go(func() {
			for index := range jobs {
				if ctx.Err() != nil {
					return
				}
				c := suite.Cases[index]
				before, after := baseline.Outputs[c.ID], candidate.Outputs[c.ID]
				result := Result{Case: c, Baseline: before, Candidate: after,
					BaselineChecks: checks(c, before), Checks: checks(c, after)}
				for i, check := range result.Checks {
					result.Regression = result.Regression || (!check.Passed && result.BaselineChecks[i].Passed)
				}
				result.NeedsReview = c.HumanReview && changed(before.Response, after.Response)
				run.Results[index] = result
			}
		})
	}
	workers.Wait()
	if err := ctx.Err(); err != nil {
		return Run{}, err
	}
	run.UpdateStatus()
	return run, nil
}
