package jenkins

type apiJobList struct {
	Jobs []apiJob `json:"jobs"`
}

type apiJob struct {
	Class string `json:"_class"`
	Name  string `json:"name"`
}

type apiBuildList struct {
	Builds []apiBuild `json:"builds"`
}

type apiBuild struct {
	Number     int              `json:"number"`
	Result     string           `json:"result"`
	Timestamp  int64            `json:"timestamp"`
	Duration   int64            `json:"duration"`
	InProgress bool             `json:"inProgress"`
	Actions    []apiBuildAction `json:"actions"`
}

type apiBuildAction struct {
	Class             string       `json:"_class"`
	LastBuiltRevision *apiRevision `json:"lastBuiltRevision,omitempty"`
}

type apiRevision struct {
	SHA1   string      `json:"SHA1"`
	Branch []apiBranch `json:"branch"`
}

type apiBranch struct {
	Name string `json:"name"`
}

type apiWFDescribe struct {
	Stages []apiStage `json:"stages"`
}

type apiStage struct {
	Name            string `json:"name"`
	Status          string `json:"status"`
	StartTimeMillis int64  `json:"startTimeMillis"`
	DurationMillis  int64  `json:"durationMillis"`
}
