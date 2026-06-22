package api

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"codeberg.org/aeforged/dalikamata/internal/domain/query"
)

const (
	defaultSize  = 100
	filterPrefix = "filter."
)

var rangeSuffixes = map[string]bool{"gte": true, "lte": true, "gt": true, "lt": true}

// validFields maps each entity to its known field names, used to reject
// unknown filter/sort fields before they reach the domain layer.
var validFields = map[query.Entity]map[string]struct{}{
	query.EntityRepo: {
		query.RepoID:   {},
		query.RepoName: {},
	},
	query.EntityCommit: {
		query.CommitSHA:       {},
		query.CommitRepoID:    {},
		query.CommitAuthor:    {},
		query.CommitTimestamp: {},
	},
	query.EntityPullRequest: {
		query.PRRepoID:           {},
		query.PRAuthor:           {},
		query.PRState:            {},
		query.PRCreatedAt:        {},
		query.PRUpdatedAt:        {},
		query.PRName:             {},
		query.PRTitle:            {},
		query.PRDescription:      {},
		query.PRID:               {},
		query.PRCycleTimeSeconds: {},
	},
	query.EntityWorkflow: {
		query.WorkflowID:   {},
		query.WorkflowName: {},
	},
	query.EntityWorkflowRun: {
		query.RunID:            {},
		query.RunWorkflowID:    {},
		query.RunNumber:        {},
		query.RunStatus:        {},
		query.RunBranch:        {},
		query.RunCommitSHA:     {},
		query.RunStartedAt:     {},
		query.RunDuration:      {},
		query.RunWorkflowName:  {},
		query.RunComponentName: {},
		query.RunTeamName:      {},
	},
	query.EntityWorkflowTask: {
		query.TaskWorkflowRunID: {},
		query.TaskOrder:         {},
		query.TaskName:          {},
		query.TaskStatus:        {},
		query.TaskStartedAt:     {},
		query.TaskDuration:      {},
		query.TaskWorkflowID:    {},
		query.TaskWorkflowName:  {},
		query.TaskComponentName: {},
		query.TaskTeamName:      {},
		query.TaskBranch:        {},
	},
	query.EntityTeam: {
		query.TeamName: {},
	},
	query.EntityComponent: {
		query.ComponentName:     {},
		query.ComponentTeamName: {},
	},
}

// parseQueryParams translates URL query parameters into a query.Query for the
// given entity. Unknown filter fields return an error; unknown sort fields are
// passed through (the domain layer ignores them silently).
func parseQueryParams(params url.Values, entity query.Entity) (query.Query, error) {
	q := query.Query{Entity: entity, Size: defaultSize}

	if s := params.Get("size"); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil {
			return q, fmt.Errorf("size: %w", err)
		}
		q.Size = n
	}
	if s := params.Get("from"); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil {
			return q, fmt.Errorf("from: %w", err)
		}
		q.From = n
	}
	if s := params.Get("sort"); s != "" {
		q.Sort = parseSortFields(s)
	}

	// Collect filter params grouped by type.
	type rangeBounds struct{ r query.Range }
	termVals := make(map[string][]string)
	rangeVals := make(map[string]*rangeBounds)
	existsSet := make(map[string]struct{})

	known := validFields[entity]

	for key, vals := range params {
		if !strings.HasPrefix(key, filterPrefix) {
			continue
		}
		rest := key[len(filterPrefix):]

		if idx := strings.LastIndex(rest, "."); idx >= 0 {
			base := rest[:idx]
			suffix := rest[idx+1:]

			if suffix == "exists" {
				if err := checkField(base, known); err != nil {
					return q, err
				}
				existsSet[base] = struct{}{}
				continue
			}
			if rangeSuffixes[suffix] {
				if err := checkField(base, known); err != nil {
					return q, err
				}
				if len(vals) == 0 {
					continue
				}
				v := inferValue(vals[0])
				if rangeVals[base] == nil {
					rangeVals[base] = &rangeBounds{}
				}
				switch suffix {
				case "gte":
					rangeVals[base].r.GTE = &v
				case "lte":
					rangeVals[base].r.LTE = &v
				case "gt":
					rangeVals[base].r.GT = &v
				case "lt":
					rangeVals[base].r.LT = &v
				}
				continue
			}
		}

		// Plain term / terms filter.
		if err := checkField(rest, known); err != nil {
			return q, err
		}
		termVals[rest] = append(termVals[rest], vals...)
	}

	// Build filter clauses and combine under OpBool/Must when there are multiple.
	var clauses []query.Filter

	for field, vals := range termVals {
		if len(vals) == 1 {
			v := inferValue(vals[0])
			clauses = append(clauses, query.Filter{Op: query.OpTerm, Field: field, Value: &v})
		} else {
			qvals := make([]query.Value, len(vals))
			for i, s := range vals {
				qvals[i] = inferValue(s)
			}
			clauses = append(clauses, query.Filter{Op: query.OpTerms, Field: field, Values: qvals})
		}
	}
	for field, rb := range rangeVals {
		r := rb.r
		clauses = append(clauses, query.Filter{Op: query.OpRange, Field: field, Range: &r})
	}
	for field := range existsSet {
		clauses = append(clauses, query.Filter{Op: query.OpExists, Field: field})
	}

	switch len(clauses) {
	case 0:
	case 1:
		q.Filter = &clauses[0]
	default:
		q.Filter = &query.Filter{Op: query.OpBool, Must: clauses}
	}

	return q, nil
}

// parseSortFields parses a comma-separated sort string (e.g. "-started_at,task_order")
// into SortField slices. A leading "-" means descending order.
func parseSortFields(s string) []query.SortField {
	parts := strings.Split(s, ",")
	fields := make([]query.SortField, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		order := query.SortAsc
		if strings.HasPrefix(p, "-") {
			order = query.SortDesc
			p = p[1:]
		}
		fields = append(fields, query.SortField{Field: p, Order: order})
	}
	return fields
}

// inferValue converts a raw URL string to the most specific Value type:
// RFC3339 time → float64 → int64 → string.
func inferValue(s string) query.Value {
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return query.TimeValue(t)
	}
	if n, err := strconv.ParseInt(s, 10, 64); err == nil {
		return query.IntValue(n)
	}
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return query.FloatValue(f)
	}
	return query.StringValue(s)
}

func checkField(field string, known map[string]struct{}) error {
	if _, ok := known[field]; !ok {
		return fmt.Errorf("unknown field %q", field)
	}
	return nil
}
