package bitbucket

import (
	"time"
)

type Payload struct {
	Push        *Push        `json:"push"`
	Repository  *Repository  `json:"repository"`
	Actor       *User        `json:"actor"`
	Pullrequest *Pullrequest `json:"pullrequest"`
}

type Push struct {
	Changes []Changes `json:"changes"`
}

type Changes struct {
	Old   *Change `json:"old"`
	New   *Change `json:"new"`
	Links Links   `json:"links"`
}

type Change struct {
	Name  string `json:"name"`
	Links Links  `json:"links"`
	Type  string `json:"type"`
}

type Repository struct {
	Links     *Links     `json:"links,omitempty"`
	UUID      string     `json:"uuid,omitempty"`
	Project   *Project   `json:"project,omitempty"`
	FullName  string     `json:"full_name,omitempty"`
	Workspace *Workspace `json:"workspace,omitempty"`
	Name      string     `json:"name,omitempty"`
}

type Project struct {
	Name  string `json:"name,omitempty"`
	Links *Links `json:"links,omitempty"`
	Key   string `json:"key,omitempty"`
}

type Workspace struct {
	Links *Links `json:"links,omitempty"`
	UUID  string `json:"uuid,omitempty"`
	Slug  string `json:"slug,omitempty"`
}

type Links struct {
	Self Link `json:"self,omitempty"`
	HTML Link `json:"html,omitempty"`
}

type Link struct {
	Href string `json:"href,omitempty"`
}

type User struct {
	DisplayName string `json:"display_name"`
	Links       Links  `json:"links"`
	Type        string `json:"type"`
	UUID        string `json:"uuid"`
	AccountID   string `json:"account_id"`
	Nickname    string `json:"nickname"`
}

type Rendered struct {
	Title       Message `json:"title"`
	Description Message `json:"description"`
}

type Message struct {
	Type   string `json:"type"`
	Raw    string `json:"raw"`
	Markup string `json:"markup"`
	HTML   string `json:"html"`
}

type Pullrequest struct {
	CommentCount      int            `json:"comment_count"`
	TaskCount         int            `json:"task_count"`
	Type              string         `json:"type"`
	ID                int            `json:"id"`
	Title             string         `json:"title"`
	Description       string         `json:"description"`
	Rendered          Rendered       `json:"rendered"`
	State             string         `json:"state"`
	MergeCommit       any            `json:"merge_commit"`
	CloseSourceBranch bool           `json:"close_source_branch"`
	ClosedBy          any            `json:"closed_by"`
	Author            User           `json:"author"`
	Reason            string         `json:"reason"`
	CreatedOn         time.Time      `json:"created_on"`
	UpdatedOn         time.Time      `json:"updated_on"`
	Destination       PRBranch       `json:"destination"`
	Source            PRBranch       `json:"source"`
	Reviewers         []User         `json:"reviewers"`
	Participants      []Participants `json:"participants"`
	Links             Links          `json:"links"`
	Summary           Message        `json:"summary"`
}

type PRBranch struct {
	Branch     Branch     `json:"branch"`
	Commit     Commit     `json:"commit"`
	Repository Repository `json:"repository"`
}

type Branch struct {
	Name string `json:"name"`
}

type Commit struct {
	Type  string `json:"type"`
	Hash  string `json:"hash"`
	Links Links  `json:"links"`
}

type Participants struct {
	Approved       bool   `json:"approved"`
	ParticipatedOn any    `json:"participated_on"`
	Role           string `json:"role"`
	State          any    `json:"state"`
	Type           string `json:"type"`
	User           User   `json:"user"`
}

func (p Payload) GetBranch() string {
	repoBranch := ""

	if p.Pullrequest != nil {
		return p.Pullrequest.GetBranch()
	}

	if p.Push != nil {
		return p.Push.GetBranch()
	}

	return repoBranch
}

func (p Payload) GetRepo() string {
	return p.Repository.Links.HTML.Href
}

func (p Push) GetBranch() string {
	repoBranch := ""

	for _, i := range p.Changes {
		if i.Old != nil && i.Old.Type == "branch" {
			repoBranch = i.New.Name
		}

		if i.New != nil && i.New.Type == "branch" {
			repoBranch = i.New.Name
		}
	}

	return repoBranch
}

func (p Pullrequest) GetBranch() string {
	return p.Source.Branch.Name
}
