package bitbucket

// pagedResponse is the generic Bitbucket Server pagination envelope.
type pagedResponse[T any] struct {
	Values        []T  `json:"values"`
	IsLastPage    bool `json:"isLastPage"`
	NextPageStart int  `json:"nextPageStart"`
}

type apiRepo struct {
	Slug    string     `json:"slug"`
	Name    string     `json:"name"`
	Project apiProject `json:"project"`
}

type apiProject struct {
	Key  string `json:"key"`
	Name string `json:"name"`
}

type apiCommit struct {
	ID                 string     `json:"id"`
	Message            string     `json:"message"`
	Author             apiGitUser `json:"author"`
	AuthorTimestamp    int64      `json:"authorTimestamp"`
	Committer          apiGitUser `json:"committer"`
	CommitterTimestamp int64      `json:"committerTimestamp"`
}

type apiGitUser struct {
	Name         string `json:"name"`
	EmailAddress string `json:"emailAddress"`
}

type apiPullRequest struct {
	ID          int64              `json:"id"`
	Title       string             `json:"title"`
	Description string             `json:"description"`
	State       string             `json:"state"`
	Author      apiPRParticipant   `json:"author"`
	Reviewers   []apiPRParticipant `json:"reviewers"`
	FromRef     apiPRRef           `json:"fromRef"`
	ToRef       apiPRRef           `json:"toRef"`
	CreatedDate int64              `json:"createdDate"`
	UpdatedDate int64              `json:"updatedDate"`
}

type apiPRParticipant struct {
	User   apiPRUser `json:"user"`
	Role   string    `json:"role"`
	Status string    `json:"status"`
}

type apiPRUser struct {
	Name         string `json:"name"`
	EmailAddress string `json:"emailAddress"`
	DisplayName  string `json:"displayName"`
}

type apiPRRef struct {
	DisplayID string `json:"displayId"`
}
